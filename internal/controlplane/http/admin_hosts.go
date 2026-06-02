package http

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	nethttp "net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"database/sql"

	"github.com/zanel1u/cloud-cli-proxy/internal/agentapi"
	"github.com/zanel1u/cloud-cli-proxy/internal/controlplane/credgen"
	"github.com/zanel1u/cloud-cli-proxy/internal/runtime"
	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

func expandHostMountSources(mounts repository.HostMounts) repository.HostMounts {
	return mounts
}

type AdminHostStore interface {
	ListHostsWithUsername(context.Context) ([]repository.HostWithUsername, error)
	GetHostDetail(context.Context, string) (repository.HostDetail, error)
	GetHost(context.Context, string) (repository.Host, error)
	UpsertHost(context.Context, repository.UpsertHostParams) (repository.Host, error)
	GetUser(context.Context, string) (repository.User, error)
	BindEgressIPToHost(context.Context, string, string) (repository.HostBinding, error)
	DeleteHost(context.Context, string) error
	ListHostsByUserID(context.Context, string) ([]repository.Host, error)
	ListRunningHosts(ctx context.Context) ([]repository.Host, error)
	GetHostWithClaudeAccount(ctx context.Context, hostID string) (repository.HostWithClaudeAccount, error) // Phase 33 D-22
	UpdateHostMounts(ctx context.Context, hostID string, mounts repository.HostMounts) error
	UpdateHostResources(ctx context.Context, hostID string, memoryLimitMB *int, cpuLimit *float64, pidsLimit *int) error
}

type AdminHostsHandler struct {
	logger        *slog.Logger
	store         AdminHostStore
	queue         HostActionQueuer
	events        EventRecorder
	imageLockPath string
}

func NewAdminHostsHandler(logger *slog.Logger, store AdminHostStore, queue HostActionQueuer, events EventRecorder, imageLockPath string) *AdminHostsHandler {
	return &AdminHostsHandler{logger: logger, store: store, queue: queue, events: events, imageLockPath: imageLockPath}
}

func (h *AdminHostsHandler) List() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		hosts, err := h.store.ListHostsWithUsername(r.Context())
		if err != nil {
			h.logger.Error("list hosts failed", "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "list hosts failed"})
			return
		}

		writeJSON(w, nethttp.StatusOK, map[string]any{"hosts": hosts})
	})
}

// getDockerStatuses runs `docker ps -a` once and returns a map of container
// name → status string (e.g. "running", "exited", "created").
func getDockerStatuses() map[string]string {
	cmd := exec.CommandContext(context.Background(), "docker", "ps", "-a",
		"--filter", "label=cloud-cli-proxy.managed=true",
		"--format", "{{.Names}}\t{{.State}}")
	out, err := cmd.Output()
	if err != nil {
		slog.Warn("docker ps failed", "error", err)
		return nil
	}
	result := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) == 2 {
			result[parts[0]] = parts[1]
		}
	}
	return result
}

type adminHostDetailResponse struct {
	repository.HostDetail
	ConnectionInfo       *repository.ConnectionInfo `json:"connection_info,omitempty"`
	PersistentVolumeName string                     `json:"persistent_volume_name,omitempty"` // Phase 33 D-22
}

func (h *AdminHostsHandler) Get() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		hostID := r.PathValue("hostID")
		detail, err := h.store.GetHostDetail(r.Context(), hostID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, nethttp.StatusNotFound, map[string]string{"error": "host not found"})
				return
			}
			h.logger.Error("get host detail failed", "host_id", hostID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "get host detail failed"})
			return
		}

		resp := adminHostDetailResponse{HostDetail: detail}
		// Phase 33 D-22：从 LEFT JOIN 取 persistent_volume_name，失败仅记日志不影响 detail 主路径。
		if hostWithCA, err := h.store.GetHostWithClaudeAccount(r.Context(), hostID); err == nil {
			resp.PersistentVolumeName = hostWithCA.PersistentVolumeName
		} else if !errors.Is(err, sql.ErrNoRows) {
			h.logger.Warn("get host with claude_account failed (degraded)", "host_id", hostID, "error", err)
		}
		resp.User.PasswordHash = ""
		resp.User.EntryPassword = ""
		sshTarget := detail.User.Username
		if sshTarget != "" {
			scheme := "https"
			if r.TLS == nil {
				scheme = "http"
			}
			host := r.Host
			if idx := strings.Index(host, ":"); idx != -1 {
				host = host[:idx]
			}
			baseURL := fmt.Sprintf("%s://%s", scheme, r.Host)
			vncPath := fmt.Sprintf("/v1/admin/hosts/%s/vnc/vnc.html", detail.Host.ID)
			resp.ConnectionInfo = &repository.ConnectionInfo{
				CurlCommand: fmt.Sprintf("curl -sSL %s/entry/%s | bash", baseURL, sshTarget),
				SSHCommand:  fmt.Sprintf("ssh %s@%s -p 2222", sshTarget, host),
				SSHPort:     2222,
				VNCURL:      fmt.Sprintf("%s%s", baseURL, vncPath),
			}
		}

		writeJSON(w, nethttp.StatusOK, resp)
	})
}

func (h *AdminHostsHandler) Create() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		var body struct {
			UserID        string                `json:"user_id"`
			EgressIPID    string                `json:"egress_ip_id"`
			Timezone      string                `json:"timezone"`
			MemoryLimitMB *int                  `json:"memory_limit_mb"`
			CPULimit      *float64              `json:"cpu_limit"`
			PidsLimit     *int                  `json:"pids_limit"`
			HostMounts    repository.HostMounts `json:"host_mounts"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.UserID == "" {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "user_id is required"})
			return
		}
		if body.EgressIPID == "" {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "egress_ip_id is required"})
			return
		}

		if err := validateMemoryLimit(body.MemoryLimitMB); err != nil {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := validateCPULimit(body.CPULimit); err != nil {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := validatePidsLimit(body.PidsLimit); err != nil {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		timezone := body.Timezone
		if timezone == "" {
			timezone = "America/Los_Angeles"
		}
		hostname := generateHostname()
		hostShortID := credgen.GenerateShortID()

		imageLockPath := h.imageLockPath
		if imageLockPath == "" {
			imageLockPath = runtime.DefaultImageLockPath
		}
		runtimeSpec, specErr := runtime.LoadRuntimeSpec(imageLockPath)
		if specErr != nil {
			h.logger.Error("load image.lock failed", "error", specErr)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "image.lock load failed"})
			return
		}
		if runtimeSpec.ImageName == "" {
			h.logger.Error("image.lock missing image_name")
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "image.lock missing image_name"})
			return
		}

		if _, err := h.store.GetUser(r.Context(), body.UserID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, nethttp.StatusNotFound, map[string]string{"error": "user not found"})
				return
			}
			h.logger.Error("get user failed", "user_id", body.UserID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "get user failed"})
			return
		}

		// 一用户一主机硬约束（与 0018 迁移的 partial unique index 对齐）：
		// 同一 user 已存在 status 不在 deleted/archived 的主机时拒绝创建，避免迁移期 race。
		existing, err := h.store.ListHostsByUserID(r.Context(), body.UserID)
		if err != nil {
			h.logger.Error("list hosts for active host check failed", "user_id", body.UserID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "check existing hosts failed"})
			return
		}
		for i := range existing {
			if existing[i].Status != "deleted" && existing[i].Status != "archived" {
				writeJSON(w, nethttp.StatusConflict, map[string]string{"error": "user already has an active host"})
				return
			}
		}

		const maxRetries = 5
		var host repository.Host
		for attempt := 0; attempt < maxRetries; attempt++ {
			host, err = h.store.UpsertHost(r.Context(), repository.UpsertHostParams{
				UserID:           body.UserID,
				Status:           "pending",
				ShortID:          hostShortID,
				TemplateImageRef: runtimeSpec.ImageName,
				HomeVolumeName:   "",
				SlotKey:          "primary",
				Timezone:         timezone,
				Hostname:         hostname,
				MemoryLimitMB:    resolveMemory(body.MemoryLimitMB),
				CPULimit:         resolveCPU(body.CPULimit),
				PidsLimit:        resolvePidsLimit(body.PidsLimit),
				HostMounts:       expandHostMountSources(body.HostMounts),
			})
			if err == nil {
				break
			}
			if strings.Contains(err.Error(), "short_id") && (strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate")) {
				hostShortID = credgen.GenerateShortID()
				continue
			}
			break
		}
		if err != nil {
			h.logger.Error("create host failed", "user_id", body.UserID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "create host failed"})
			return
		}

		if _, err := h.store.BindEgressIPToHost(r.Context(), host.ID, body.EgressIPID); err != nil {
			h.logger.Error("bind egress IP failed", "host_id", host.ID, "egress_ip_id", body.EgressIPID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "bind egress IP failed"})
			return
		}

		task, err := h.queue.QueueHostAction(r.Context(), host.ID, agentapi.ActionCreateHost, "admin", "")
		if err != nil {
			h.logger.Error("queue create_host failed", "host_id", host.ID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "queue create action failed"})
			return
		}

		if h.events != nil {
			hostID := host.ID
			if _, err := h.events.RecordEvent(r.Context(), repository.RecordEventParams{
				HostID:  &hostID,
				Level:   "info",
				Type:    "admin.host.create",
				Message: "管理员创建主机",
				Metadata: map[string]any{
					"operator": "admin",
					"user_id":  body.UserID,
				},
			}); err != nil {
				h.logger.Error("record event failed", "type", "admin.host.create", "error", err)
			}
		}

		writeJSON(w, nethttp.StatusAccepted, map[string]any{
			"host":    host,
			"task_id": task.ID,
			"status":  "202 Accepted",
		})
	})
}

func (h *AdminHostsHandler) Start() nethttp.Handler {
	return h.lifecycleAction(agentapi.ActionStartHost)
}

func (h *AdminHostsHandler) Stop() nethttp.Handler {
	return h.lifecycleAction(agentapi.ActionStopHost)
}

func (h *AdminHostsHandler) Rebuild() nethttp.Handler {
	return h.lifecycleAction(agentapi.ActionRebuildHost)
}

func (h *AdminHostsHandler) lifecycleAction(action agentapi.HostAction) nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		hostID := r.PathValue("hostID")

		if action == agentapi.ActionRebuildHost {
			var body struct {
				Mode string `json:"mode"`
			}
			if r.Body != nil && r.ContentLength > 0 {
				_ = json.NewDecoder(r.Body).Decode(&body)
			}
		}

		task, err := h.queue.QueueHostAction(r.Context(), hostID, action, "admin", "")
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, nethttp.StatusNotFound, map[string]string{"error": "host not found"})
				return
			}
			h.logger.Error("queue host action failed", "host_id", hostID, "action", action, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "queue action failed"})
			return
		}

		if h.events != nil {
			if _, err := h.events.RecordEvent(r.Context(), repository.RecordEventParams{
				HostID:   &hostID,
				Level:    "info",
				Type:     "admin.host.action",
				Message:  "管理员发起主机操作",
				Metadata: map[string]any{"operator": "admin", "action": string(action)},
			}); err != nil {
				h.logger.Error("record event failed", "type", "admin.host.action", "error", err)
			}
		}

		writeJSON(w, nethttp.StatusAccepted, map[string]any{
			"task_id": task.ID,
			"status":  "202 Accepted",
		})
	})
}

func (h *AdminHostsHandler) Delete() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		hostID := r.PathValue("hostID")
		force := r.URL.Query().Get("force") == "true"

		host, err := h.store.GetHost(r.Context(), hostID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, nethttp.StatusNotFound, map[string]string{"error": "host not found"})
				return
			}
			h.logger.Error("get host failed", "host_id", hostID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "get host failed"})
			return
		}

		if !force && host.Status == "running" {
			writeJSON(w, nethttp.StatusConflict, map[string]string{"error": "主机正在运行中，请先停止或使用强制删除"})
			return
		}

		containerName := "cloudproxy-" + hostID

		_ = dockerRm(containerName)

		if err := h.store.DeleteHost(r.Context(), hostID); err != nil {
			h.logger.Error("delete host failed", "host_id", hostID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "delete host failed"})
			return
		}

		if h.events != nil {
			if _, err := h.events.RecordEvent(r.Context(), repository.RecordEventParams{
				Level:   "warn",
				Type:    "admin.host.delete",
				Message: "管理员删除主机",
				Metadata: map[string]any{
					"operator": "admin",
					"host_id":  hostID,
					"force":    force,
				},
			}); err != nil {
				h.logger.Error("record event failed", "type", "admin.host.delete", "error", err)
			}
		}

		writeJSON(w, nethttp.StatusOK, map[string]string{"status": "deleted"})
	})
}

func (h *AdminHostsHandler) RestartVNC() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		hostID := r.PathValue("hostID")

		host, err := h.store.GetHost(r.Context(), hostID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, nethttp.StatusNotFound, map[string]string{"error": "host not found"})
				return
			}
			h.logger.Error("get host for vnc restart failed", "host_id", hostID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "get host failed"})
			return
		}
		if host.Status != "running" {
			writeJSON(w, nethttp.StatusConflict, map[string]string{"error": "host is not running"})
			return
		}

		containerName := "cloudproxy-" + hostID
		if err := restartContainerVNC(containerName); err != nil {
			h.logger.Error("restart vnc failed", "host_id", hostID, "container", containerName, "error", err)
			writeJSON(w, nethttp.StatusBadGateway, map[string]string{"error": "restart vnc failed"})
			return
		}

		if h.events != nil {
			if _, err := h.events.RecordEvent(r.Context(), repository.RecordEventParams{
				HostID:   &hostID,
				Level:    "info",
				Type:     "admin.host.vnc_restarted",
				Message:  "管理员重启 VNC 服务",
				Metadata: map[string]any{"operator": "admin"},
			}); err != nil {
				h.logger.Error("record event failed", "type", "admin.host.vnc_restarted", "error", err)
			}
		}

		writeJSON(w, nethttp.StatusOK, map[string]any{"status": "restarted"})
	})
}

func (h *AdminHostsHandler) ChangeRootPassword() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		hostID := r.PathValue("hostID")

		var body struct {
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Password == "" {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "password is required"})
			return
		}

		host, err := h.store.GetHost(r.Context(), hostID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, nethttp.StatusNotFound, map[string]string{"error": "host not found"})
				return
			}
			h.logger.Error("get host for root password change failed", "host_id", hostID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "get host failed"})
			return
		}
		if host.Status != "running" {
			writeJSON(w, nethttp.StatusConflict, map[string]string{"error": "host is not running"})
			return
		}

		containerName := "cloudproxy-" + hostID
		if err := syncContainerPassword(containerName, "root", body.Password); err != nil {
			h.logger.Error("change root password failed", "host_id", hostID, "error", err)
			writeJSON(w, nethttp.StatusBadGateway, map[string]string{"error": "change root password failed"})
			return
		}

		if h.events != nil {
			if _, err := h.events.RecordEvent(r.Context(), repository.RecordEventParams{
				HostID:   &hostID,
				Level:    "info",
				Type:     "admin.host.root_password_changed",
				Message:  "管理员修改容器 root 密码",
				Metadata: map[string]any{"operator": "admin"},
			}); err != nil {
				h.logger.Error("record event failed", "type", "admin.host.root_password_changed", "error", err)
			}
		}

		writeJSON(w, nethttp.StatusOK, map[string]string{"status": "ok"})
	})
}

func (h *AdminHostsHandler) GetImageInfo() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		hostID := r.PathValue("hostID")
		containerName := "cloudproxy-" + hostID

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		containerCmd := exec.CommandContext(ctx, "docker", "inspect",
			"--format", "{{.Image}}|{{.Config.Image}}|{{.Created}}", containerName)
		containerOut, err := containerCmd.Output()
		if err != nil {
			writeJSON(w, nethttp.StatusOK, map[string]any{
				"container_image_id":  "",
				"latest_image_id":     "",
				"update_available":    false,
				"container_available": false,
			})
			return
		}

		parts := strings.SplitN(strings.TrimSpace(string(containerOut)), "|", 3)
		containerImageID := parts[0]
		containerCreated := ""
		if len(parts) > 2 {
			containerCreated = parts[2]
		}

		spec, specErr := runtime.LoadRuntimeSpec(runtime.DefaultImageLockPath)
		if specErr != nil {
			writeJSON(w, nethttp.StatusOK, map[string]any{
				"container_image_id":  shortImageID(containerImageID),
				"container_created":   containerCreated,
				"latest_image_id":     "",
				"update_available":    false,
				"container_available": true,
			})
			return
		}

		latestCmd := exec.CommandContext(ctx, "docker", "inspect",
			"--format", "{{.Id}}|{{.Created}}", spec.ImageName)
		latestOut, err := latestCmd.Output()
		if err != nil {
			writeJSON(w, nethttp.StatusOK, map[string]any{
				"container_image_id":  shortImageID(containerImageID),
				"container_created":   containerCreated,
				"latest_image_id":     "",
				"latest_image_name":   spec.ImageName,
				"update_available":    false,
				"container_available": true,
			})
			return
		}

		latestParts := strings.SplitN(strings.TrimSpace(string(latestOut)), "|", 2)
		latestImageID := latestParts[0]
		latestCreated := ""
		if len(latestParts) > 1 {
			latestCreated = latestParts[1]
		}

		updateAvailable := containerImageID != latestImageID

		writeJSON(w, nethttp.StatusOK, map[string]any{
			"container_image_id":  shortImageID(containerImageID),
			"container_created":   containerCreated,
			"latest_image_id":     shortImageID(latestImageID),
			"latest_image_name":   spec.ImageName,
			"latest_created":      latestCreated,
			"update_available":    updateAvailable,
			"container_available": true,
		})
	})
}

func shortImageID(id string) string {
	id = strings.TrimPrefix(id, "sha256:")
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func (h *AdminHostsHandler) ExportConfig() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		hostID := r.PathValue("hostID")

		host, err := h.store.GetHost(r.Context(), hostID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, nethttp.StatusNotFound, map[string]string{"error": "host not found"})
				return
			}
			h.logger.Error("get host for config export failed", "host_id", hostID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "get host failed"})
			return
		}
		if host.Status != "running" {
			writeJSON(w, nethttp.StatusConflict, map[string]string{"error": "host is not running"})
			return
		}

		containerName := "cloudproxy-" + hostID
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, "docker", "exec", "-i", containerName,
			"tar", "czf", "-",
			"-C", "/workspace", ".claude", ".claude.json", ".chrome-data",
			"-C", "/var/lib/claude-persist", ".", ".cache")

		w.Header().Set("Content-Type", "application/gzip")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"host-%s-config.tar.gz\"", hostID))
		w.WriteHeader(nethttp.StatusOK)

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			h.logger.Error("docker exec stdout pipe failed", "host_id", hostID, "error", err)
			return
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			h.logger.Error("docker exec stderr pipe failed", "host_id", hostID, "error", err)
			return
		}

		if err := cmd.Start(); err != nil {
			h.logger.Error("docker exec tar start failed", "host_id", hostID, "error", err)
			return
		}

		go func() {
			stderrBytes, _ := io.ReadAll(stderr)
			if len(stderrBytes) > 0 {
				h.logger.Warn("docker exec tar stderr", "host_id", hostID, "stderr", string(stderrBytes))
			}
		}()

		if _, err := io.Copy(w, stdout); err != nil {
			h.logger.Error("copy tar output failed", "host_id", hostID, "error", err)
			return
		}

		if err := cmd.Wait(); err != nil {
			h.logger.Error("docker exec tar failed", "host_id", hostID, "error", err)
			return
		}

		if h.events != nil {
			if _, err := h.events.RecordEvent(r.Context(), repository.RecordEventParams{
				HostID:   &hostID,
				Level:    "info",
				Type:     "admin.host.config_exported",
				Message:  "管理员导出容器配置",
				Metadata: map[string]any{"operator": "admin"},
			}); err != nil {
				h.logger.Error("record event failed", "type", "admin.host.config_exported", "error", err)
			}
		}
	})
}

func (h *AdminHostsHandler) ImportConfig() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		hostID := r.PathValue("hostID")

		host, err := h.store.GetHost(r.Context(), hostID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, nethttp.StatusNotFound, map[string]string{"error": "host not found"})
				return
			}
			h.logger.Error("get host for config import failed", "host_id", hostID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "get host failed"})
			return
		}
		if host.Status != "running" {
			writeJSON(w, nethttp.StatusConflict, map[string]string{"error": "host is not running"})
			return
		}

		if err := r.ParseMultipartForm(100 << 20); err != nil {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "invalid multipart form: " + err.Error()})
			return
		}
		defer r.MultipartForm.RemoveAll()

		file, _, err := r.FormFile("file")
		if err != nil {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "file is required"})
			return
		}
		defer file.Close()

		containerName := "cloudproxy-" + hostID
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, "docker", "exec", "-i", containerName, "tar", "xzf", "-", "-C", "/")
		cmd.Stdin = file

		output, err := cmd.CombinedOutput()
		if err != nil {
			h.logger.Error("docker exec tar extract failed", "host_id", hostID, "error", err, "output", string(output))
			writeJSON(w, nethttp.StatusBadGateway, map[string]string{"error": "import failed: " + strings.TrimSpace(string(output))})
			return
		}

		if h.events != nil {
			if _, err := h.events.RecordEvent(r.Context(), repository.RecordEventParams{
				HostID:   &hostID,
				Level:    "info",
				Type:     "admin.host.config_imported",
				Message:  "管理员导入容器配置",
				Metadata: map[string]any{"operator": "admin"},
			}); err != nil {
				h.logger.Error("record event failed", "type", "admin.host.config_imported", "error", err)
			}
		}

		writeJSON(w, nethttp.StatusOK, map[string]string{"status": "ok"})
	})
}

func (h *AdminHostsHandler) GetClaudeSettings() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		hostID := r.PathValue("hostID")

		host, err := h.store.GetHost(r.Context(), hostID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, nethttp.StatusNotFound, map[string]string{"error": "host not found"})
				return
			}
			h.logger.Error("get host for claude settings failed", "host_id", hostID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "get host failed"})
			return
		}
		if host.Status != "running" {
			writeJSON(w, nethttp.StatusConflict, map[string]string{"error": "host is not running"})
			return
		}

		containerName := "cloudproxy-" + hostID
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, "docker", "exec", "-i", containerName,
			"cat", "/workspace/.claude/settings.json")
		output, err := cmd.CombinedOutput()
		if err != nil {
			writeJSON(w, nethttp.StatusOK, map[string]any{"settings": map[string]any{}})
			return
		}

		var settings json.RawMessage
		if err := json.Unmarshal(bytes.TrimSpace(output), &settings); err != nil {
			writeJSON(w, nethttp.StatusOK, map[string]any{"settings": map[string]any{}})
			return
		}

		writeJSON(w, nethttp.StatusOK, map[string]any{"settings": settings})
	})
}

func (h *AdminHostsHandler) UpdateClaudeSettings() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		hostID := r.PathValue("hostID")

		var body struct {
			Settings json.RawMessage `json:"settings"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Settings) == 0 {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "settings is required"})
			return
		}

		host, err := h.store.GetHost(r.Context(), hostID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, nethttp.StatusNotFound, map[string]string{"error": "host not found"})
				return
			}
			h.logger.Error("get host for claude settings update failed", "host_id", hostID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "get host failed"})
			return
		}
		if host.Status != "running" {
			writeJSON(w, nethttp.StatusConflict, map[string]string{"error": "host is not running"})
			return
		}

		containerName := "cloudproxy-" + hostID
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		mkdirCmd := exec.CommandContext(ctx, "docker", "exec", "-i", containerName,
			"mkdir", "-p", "/workspace/.claude")
		if out, err := mkdirCmd.CombinedOutput(); err != nil {
			h.logger.Error("mkdir .claude failed", "host_id", hostID, "error", err, "output", string(out))
			writeJSON(w, nethttp.StatusBadGateway, map[string]string{"error": "prepare directory failed"})
			return
		}

		prettySettings, _ := json.MarshalIndent(json.RawMessage(body.Settings), "", "  ")
		teeCmd := exec.CommandContext(ctx, "docker", "exec", "-i", containerName,
			"tee", "/workspace/.claude/settings.json")
		teeCmd.Stdin = bytes.NewReader(prettySettings)
		if out, err := teeCmd.CombinedOutput(); err != nil {
			h.logger.Error("write claude settings failed", "host_id", hostID, "error", err, "output", string(out))
			writeJSON(w, nethttp.StatusBadGateway, map[string]string{"error": "write settings failed"})
			return
		}

		if h.events != nil {
			if _, err := h.events.RecordEvent(r.Context(), repository.RecordEventParams{
				HostID:   &hostID,
				Level:    "info",
				Type:     "admin.host.claude_settings_updated",
				Message:  "管理员更新容器 Claude 配置",
				Metadata: map[string]any{"operator": "admin"},
			}); err != nil {
				h.logger.Error("record event failed", "type", "admin.host.claude_settings_updated", "error", err)
			}
		}

		writeJSON(w, nethttp.StatusOK, map[string]string{"status": "ok"})
	})
}

func (h *AdminHostsHandler) GetClaudeInfo() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		hostID := r.PathValue("hostID")

		host, err := h.store.GetHost(r.Context(), hostID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, nethttp.StatusNotFound, map[string]string{"error": "host not found"})
				return
			}
			h.logger.Error("get host for claude info failed", "host_id", hostID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "get host failed"})
			return
		}
		if host.Status != "running" {
			writeJSON(w, nethttp.StatusConflict, map[string]string{"error": "host is not running"})
			return
		}

		containerName := "cloudproxy-" + hostID
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		script := `echo '===CLAUDE_JSON===' && cat /workspace/.claude.json 2>/dev/null || echo '{}' && echo '===PROJECT_SETTINGS===' && cat /workspace/.claude/settings.json 2>/dev/null || echo '{}' && echo '===UNAME===' && uname -a 2>/dev/null || echo 'unknown' && echo '===HOSTNAME===' && hostname 2>/dev/null || echo 'unknown' && echo '===NODE===' && node --version 2>/dev/null || echo 'unknown'`
		cmd := exec.CommandContext(ctx, "docker", "exec", "-i", containerName, "bash", "-c", script)
		output, err := cmd.CombinedOutput()
		if err != nil {
			writeJSON(w, nethttp.StatusOK, map[string]any{
				"claude_json": map[string]any{},
				"uname":       "unknown",
				"hostname":    "unknown",
				"node":        "unknown",
			})
			return
		}

		raw := string(output)
		result := map[string]any{
			"uname":    "unknown",
			"hostname": "unknown",
			"node":     "unknown",
		}

		extractSection := func(marker, next string) string {
			start := strings.Index(raw, marker)
			if start == -1 {
				return ""
			}
			start += len(marker) + 1
			end := len(raw)
			if next != "" {
				if idx := strings.Index(raw[start:], next); idx >= 0 {
					end = start + idx
				}
			}
			return strings.TrimSpace(raw[start:end])
		}

		claudeJSON := extractSection("===CLAUDE_JSON===", "===PROJECT_SETTINGS===")
		var cj json.RawMessage
		if err := json.Unmarshal([]byte(claudeJSON), &cj); err == nil {
			result["claude_json"] = cj
		} else {
			result["claude_json"] = map[string]any{}
		}

		projectSettings := extractSection("===PROJECT_SETTINGS===", "===UNAME===")
		var ps json.RawMessage
		if err := json.Unmarshal([]byte(projectSettings), &ps); err == nil {
			result["project_settings"] = ps
		} else {
			result["project_settings"] = map[string]any{}
		}

		result["uname"] = extractSection("===UNAME===", "===HOSTNAME===")
		result["hostname"] = extractSection("===HOSTNAME===", "===NODE===")
		result["node"] = extractSection("===NODE===", "")

		writeJSON(w, nethttp.StatusOK, result)
	})
}

type claudeProcess struct {
	PID            int    `json:"pid"`
	WorkDir        string `json:"work_dir"`
	ElapsedSeconds int    `json:"elapsed_seconds"`
}

func (h *AdminHostsHandler) GetClaudeStatus() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		hostID := r.PathValue("hostID")

		host, err := h.store.GetHost(r.Context(), hostID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, nethttp.StatusNotFound, map[string]string{"error": "host not found"})
				return
			}
			h.logger.Error("get host for claude status failed", "host_id", hostID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "get host failed"})
			return
		}
		if host.Status != "running" {
			writeJSON(w, nethttp.StatusConflict, map[string]string{"error": "host is not running"})
			return
		}

		containerName := "cloudproxy-" + hostID
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		script := `ps -eo pid=,etimes=,args= 2>/dev/null | grep '[c]laude ' | while read -r pid etime rest; do
  cwd=$(readlink /proc/$pid/cwd 2>/dev/null || echo "unknown")
  printf '%s|%s|%s\n' "$pid" "$etime" "$cwd"
done`
		procCmd := exec.CommandContext(ctx, "docker", "exec", "-i", containerName, "bash", "-c", script)
		procOut, _ := procCmd.CombinedOutput()

		var processes []claudeProcess
		for _, line := range strings.Split(strings.TrimSpace(string(procOut)), "\n") {
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, "|", 3)
			if len(parts) < 3 {
				continue
			}
			pid, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
			elapsed, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
			cwd := strings.TrimSpace(parts[2])
			if pid > 0 {
				processes = append(processes, claudeProcess{
					PID:            pid,
					WorkDir:        cwd,
					ElapsedSeconds: elapsed,
				})
			}
		}

		versionCmd := exec.CommandContext(ctx, "docker", "exec", "-i", containerName,
			"bash", "-c", "claude --version 2>/dev/null || echo unknown")
		versionOut, _ := versionCmd.CombinedOutput()
		version := strings.TrimSpace(string(versionOut))
		if version == "" {
			version = "unknown"
		}

		writeJSON(w, nethttp.StatusOK, map[string]any{
			"running_instances": len(processes),
			"version":           version,
			"processes":         processes,
		})
	})
}

func (h *AdminHostsHandler) UpdateClaude() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		hostID := r.PathValue("hostID")

		host, err := h.store.GetHost(r.Context(), hostID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, nethttp.StatusNotFound, map[string]string{"error": "host not found"})
				return
			}
			h.logger.Error("get host for claude update failed", "host_id", hostID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "get host failed"})
			return
		}
		if host.Status != "running" {
			writeJSON(w, nethttp.StatusConflict, map[string]string{"error": "host is not running"})
			return
		}

		containerName := "cloudproxy-" + hostID
		ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
		defer cancel()

		updateScript := `set -e
ARCH=$(dpkg --print-architecture)
case "${ARCH}" in
  amd64) GOARCH="x64" ;;
  arm64) GOARCH="arm64" ;;
  *) echo "unsupported arch: ${ARCH}" >&2; exit 1 ;;
esac
LATEST=$(curl -fsSL https://api.github.com/repos/anthropics/claude-code/releases/latest | jq -r .tag_name)
if [ -z "${LATEST}" ] || [ "${LATEST}" = "null" ]; then
  echo "Failed to get latest release version" >&2; exit 1
fi
URL="https://github.com/anthropics/claude-code/releases/download/${LATEST}/claude-linux-${GOARCH}.tar.gz"
echo "Downloading ${LATEST}..."
curl -fsSL -o /tmp/claude-update.tar.gz "${URL}"
tar -xzf /tmp/claude-update.tar.gz -C /tmp claude
mv /tmp/claude /usr/local/bin/claude
chmod +x /usr/local/bin/claude
rm -f /tmp/claude-update.tar.gz
claude --version
2>&1`
		cmd := exec.CommandContext(ctx, "docker", "exec", "-i", containerName, "bash", "-c", updateScript)
		output, err := cmd.CombinedOutput()
		if err != nil {
			h.logger.Error("update claude failed", "host_id", hostID, "error", err, "output", string(output))
			writeJSON(w, nethttp.StatusBadGateway, map[string]string{"error": "update claude failed: " + strings.TrimSpace(string(output))})
			return
		}

		versionCmd := exec.CommandContext(ctx, "docker", "exec", "-i", containerName,
			"bash", "-c", "claude --version 2>/dev/null || echo unknown")
		versionOut, _ := versionCmd.CombinedOutput()
		version := strings.TrimSpace(string(versionOut))
		if version == "" {
			version = "unknown"
		}

		if h.events != nil {
			if _, err := h.events.RecordEvent(r.Context(), repository.RecordEventParams{
				HostID:   &hostID,
				Level:    "info",
				Type:     "admin.host.claude_updated",
				Message:  "管理员更新容器 Claude Code",
				Metadata: map[string]any{"operator": "admin", "version": version},
			}); err != nil {
				h.logger.Error("record event failed", "type", "admin.host.claude_updated", "error", err)
			}
		}

		writeJSON(w, nethttp.StatusOK, map[string]any{"status": "ok", "version": version})
	})
}

func (h *AdminHostsHandler) UpdateMounts() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		hostID := r.PathValue("hostID")
		var body struct {
			Mounts repository.HostMounts `json:"mounts"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		for _, m := range body.Mounts {
			if m.Source == "" || m.Target == "" {
				writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "source and target are required"})
				return
			}
			if !strings.HasPrefix(m.Source, "/") || !strings.HasPrefix(m.Target, "/") {
				writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "paths must be absolute"})
				return
			}
		}
		expandedMounts := expandHostMountSources(body.Mounts)
		if err := h.store.UpdateHostMounts(r.Context(), hostID, expandedMounts); err != nil {
			h.logger.Error("update host mounts failed", "host_id", hostID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "update mounts failed"})
			return
		}
		if h.events != nil {
			hid := hostID
			if _, err := h.events.RecordEvent(r.Context(), repository.RecordEventParams{
				HostID: &hid, Level: "info", Type: "admin.host.update_mounts",
				Message:  "管理员更新主机挂载配置",
				Metadata: map[string]any{"operator": "admin", "mount_count": len(body.Mounts)},
			}); err != nil {
				h.logger.Error("record event failed", "type", "admin.host.update_mounts", "error", err)
			}
		}
		writeJSON(w, nethttp.StatusOK, map[string]string{"status": "ok"})
	})
}

// PatchResources 处理 PATCH /v1/admin/hosts/{hostID}/resources。
func (h *AdminHostsHandler) PatchResources() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		hostID := r.PathValue("hostID")

		var body struct {
			MemoryLimitMB *int     `json:"memory_limit_mb"`
			CPULimit      *float64 `json:"cpu_limit"`
			PidsLimit     *int     `json:"pids_limit"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}

		if err := validateMemoryLimit(body.MemoryLimitMB); err != nil {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := validateCPULimit(body.CPULimit); err != nil {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := validatePidsLimit(body.PidsLimit); err != nil {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		host, err := h.store.GetHost(r.Context(), hostID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, nethttp.StatusNotFound, map[string]string{"error": "host not found"})
				return
			}
			h.logger.Error("get host for resource patch failed", "host_id", hostID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "get host failed"})
			return
		}

		if host.Status == "running" {
			if err := dockerUpdateHostResources(r.Context(), "cloudproxy-"+hostID, body.MemoryLimitMB, body.CPULimit, body.PidsLimit); err != nil {
				h.logger.Error("docker update resources failed", "host_id", hostID, "error", err)
				writeJSON(w, nethttp.StatusBadGateway, map[string]string{"error": "docker update resources failed"})
				return
			}
		}

		if err := h.store.UpdateHostResources(r.Context(), hostID,
			resolveResourceMemory(body.MemoryLimitMB),
			resolveResourceCPU(body.CPULimit),
			resolveResourcePidsLimit(body.PidsLimit),
		); err != nil {
			h.logger.Error("update host resources failed", "host_id", hostID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "update resources failed"})
			return
		}

		if h.events != nil {
			hid := hostID
			if _, err := h.events.RecordEvent(r.Context(), repository.RecordEventParams{
				HostID: &hid, Level: "info", Type: "admin.host.resources_updated",
				Message:  "管理员更新主机资源限制",
				Metadata: map[string]any{"operator": "admin"},
			}); err != nil {
				h.logger.Error("record event failed", "type", "admin.host.resources_updated", "error", err)
			}
		}

		writeJSON(w, nethttp.StatusOK, map[string]string{"status": "ok"})
	})
}

func dockerResourcePidsLimitValue(limit int) string {
	if limit == 0 {
		return "-1"
	}
	return strconv.Itoa(limit)
}

var dockerUpdateHostResources = func(ctx context.Context, containerName string, memoryLimitMB *int, cpuLimit *float64, pidsLimit *int) error {
	args := []string{"update"}
	if memoryLimitMB != nil {
		if *memoryLimitMB == 0 {
			args = append(args, "--memory", "0")
		} else {
			args = append(args, "--memory", fmt.Sprintf("%dm", *memoryLimitMB))
		}
	}
	if cpuLimit != nil {
		if *cpuLimit == 0 {
			args = append(args, "--cpus", "0")
		} else {
			args = append(args, "--cpus", fmt.Sprintf("%.1f", *cpuLimit))
		}
	}
	if pidsLimit != nil {
		args = append(args, "--pids-limit", dockerResourcePidsLimitValue(*pidsLimit))
	}
	if len(args) == 1 {
		return nil
	}
	args = append(args, containerName)
	cmd := exec.CommandContext(ctx, "docker", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker update: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	return nil
}

// syncContainerPassword updates the Linux user password inside a running container via docker exec.

// Exposed as a package-level var so unit tests can inject a fake implementation (Phase 29.1).
var syncContainerPassword = func(containerName, user, password string) error {
	cmd := exec.CommandContext(context.Background(), "docker", "exec", "-i", containerName,
		"chpasswd")
	cmd.Stdin = strings.NewReader(fmt.Sprintf("%s:%s\n", user, password))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker exec chpasswd: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func generateHostname() string {
	const alphaNum = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	prefixes := []string{"DESKTOP-", "LAPTOP-"}

	pidx, _ := rand.Int(rand.Reader, big.NewInt(int64(len(prefixes))))
	prefix := prefixes[pidx.Int64()]

	suffix := make([]byte, 7)
	for i := range suffix {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(alphaNum))))
		suffix[i] = alphaNum[n.Int64()]
	}
	return prefix + string(suffix)
}

func dockerRm(containerName string) error {
	cmd := exec.CommandContext(context.Background(), "docker", "rm", "-f", containerName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		slog.Warn("docker rm failed (may not exist)", "container", containerName, "output", string(output), "error", err)
	}
	return err
}

func (h *AdminHostsHandler) GetLogs() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		hostID := r.PathValue("hostID")

		_, err := h.store.GetHost(r.Context(), hostID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, nethttp.StatusNotFound, map[string]string{"error": "host not found"})
				return
			}
			h.logger.Error("get host for logs failed", "host_id", hostID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "get host failed"})
			return
		}

		tail := r.URL.Query().Get("tail")
		if tail == "" {
			tail = "100"
		}
		tailN, err := strconv.Atoi(tail)
		if err != nil || tailN < 1 {
			tailN = 100
		}
		if tailN > 500 {
			tailN = 500
		}

		containerName := "cloudproxy-" + hostID
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, "docker", "logs", "--tail", strconv.Itoa(tailN), containerName)
		output, err := cmd.CombinedOutput()

		result := map[string]any{
			"host_id":        hostID,
			"container_name": containerName,
			"tail":           tailN,
			"logs":           string(output),
		}
		if err != nil {
			result["error"] = err.Error()
			result["logs"] = string(output)
		}

		writeJSON(w, nethttp.StatusOK, result)
	})
}

func intPtr(v int) *int           { return &v }
func floatPtr(v float64) *float64 { return &v }

func validateMemoryLimit(mb *int) error {
	if mb == nil || *mb == 0 {
		return nil
	}
	if *mb < 128 {
		return fmt.Errorf("memory_limit_mb 不能小于 128 MB")
	}
	if *mb > 262144 {
		return fmt.Errorf("memory_limit_mb 不能大于 262144 MB (256 GB)")
	}
	return nil
}

func validateCPULimit(cpu *float64) error {
	if cpu == nil || *cpu == 0 {
		return nil
	}
	if *cpu < 0.1 {
		return fmt.Errorf("cpu_limit 不能小于 0.1 核")
	}
	if *cpu > 64 {
		return fmt.Errorf("cpu_limit 不能大于 64 核")
	}
	return nil
}

func validatePidsLimit(pids *int) error {
	if pids == nil || *pids == 0 {
		return nil
	}
	if *pids < 64 {
		return fmt.Errorf("pids_limit 不能小于 64")
	}
	if *pids > 131072 {
		return fmt.Errorf("pids_limit 不能大于 131072")
	}
	return nil
}

// resolveMemory 三态解析：nil → 默认值(4096) / 0 → 无限制 / >0 → 传值。
func resolveMemory(mb *int) *int {
	if mb == nil {
		def := 4096
		return &def
	}
	return mb
}

// resolveCPU 三态解析：nil → 默认值(2.0) / 0 → 无限制 / >0 → 传值。
func resolveCPU(cpu *float64) *float64 {
	if cpu == nil {
		def := 2.0
		return &def
	}
	return cpu
}

// resolvePidsLimit 三态解析：nil → 默认值(1024) / 0 → 无限制 / >0 → 传值。
func resolvePidsLimit(pids *int) *int {
	if pids == nil {
		def := 1024
		return &def
	}
	return pids
}

// resolveResourceMemory PATCH 端点的三态解析：nil=不修改，0=无限制，>0=传值。
func resolveResourceMemory(mb *int) *int {
	if mb == nil {
		return nil
	}
	return mb
}

// resolveResourceCPU PATCH 端点的三态解析：nil=不修改，0=无限制，>0=传值。
func resolveResourceCPU(cpu *float64) *float64 {
	if cpu == nil {
		return nil
	}
	return cpu
}

// resolveResourcePidsLimit PATCH 端点的三态解析：nil=不修改，0=无限制，>0=传值。
func resolveResourcePidsLimit(pids *int) *int {
	if pids == nil {
		return nil
	}
	return pids
}
