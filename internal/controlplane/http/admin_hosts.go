package http

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	nethttp "net/http"
	"os/exec"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/zanel1u/cloud-cli-proxy/internal/agentapi"
	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

type AdminHostStore interface {
	ListHostsWithUsername(context.Context) ([]repository.HostWithUsername, error)
	GetHostDetail(context.Context, string) (repository.HostDetail, error)
	GetHost(context.Context, string) (repository.Host, error)
	UpsertHost(context.Context, repository.UpsertHostParams) (repository.Host, error)
	GetUser(context.Context, string) (repository.User, error)
	BindEgressIPToHost(context.Context, string, string) (repository.HostBinding, error)
	DeleteHost(context.Context, string) error
}

type AdminHostsHandler struct {
	logger *slog.Logger
	store  AdminHostStore
	queue  HostActionQueuer
	events EventRecorder
}

func NewAdminHostsHandler(logger *slog.Logger, store AdminHostStore, queue HostActionQueuer, events EventRecorder) *AdminHostsHandler {
	return &AdminHostsHandler{logger: logger, store: store, queue: queue, events: events}
}

func (h *AdminHostsHandler) List() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		hosts, err := h.store.ListHostsWithUsername(r.Context())
		if err != nil {
			h.logger.Error("list hosts failed", "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "list hosts failed"})
			return
		}

		dockerStatuses := getDockerStatuses()
		for i := range hosts {
			containerName := "cloudproxy-" + hosts[i].ID
			if ds, ok := dockerStatuses[containerName]; ok {
				hosts[i].DockerStatus = ds
			} else {
				hosts[i].DockerStatus = "not found"
			}
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
	ConnectionInfo *repository.ConnectionInfo `json:"connection_info,omitempty"`
}

func (h *AdminHostsHandler) Get() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		hostID := r.PathValue("hostID")
		detail, err := h.store.GetHostDetail(r.Context(), hostID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeJSON(w, nethttp.StatusNotFound, map[string]string{"error": "host not found"})
				return
			}
			h.logger.Error("get host detail failed", "host_id", hostID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "get host detail failed"})
			return
		}

		resp := adminHostDetailResponse{HostDetail: detail}
		if detail.User.ShortID != "" {
			scheme := "https"
			if r.TLS == nil {
				scheme = "http"
			}
			baseURL := fmt.Sprintf("%s://%s", scheme, r.Host)
			resp.ConnectionInfo = &repository.ConnectionInfo{
				CurlCommand: fmt.Sprintf("curl -sSL %s/entry/%s | bash", baseURL, detail.User.ShortID),
				SSHCommand:  fmt.Sprintf("ssh %s@%s -p 2222", detail.User.ShortID, r.Host),
				SSHPort:     2222,
			}
		}

		writeJSON(w, nethttp.StatusOK, resp)
	})
}

func (h *AdminHostsHandler) Create() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		var body struct {
			UserID        string  `json:"user_id"`
			EgressIPID    string  `json:"egress_ip_id"`
			Timezone      string  `json:"timezone"`
			MemoryLimitMB int     `json:"memory_limit_mb"`
			CPULimit      float64 `json:"cpu_limit"`
			DiskLimitGB   int     `json:"disk_limit_gb"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.UserID == "" {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "user_id is required"})
			return
		}
		if body.EgressIPID == "" {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "egress_ip_id is required"})
			return
		}

		timezone := body.Timezone
		if timezone == "" {
			timezone = "America/Los_Angeles"
		}
		hostname := generateHostname()

		if _, err := h.store.GetUser(r.Context(), body.UserID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeJSON(w, nethttp.StatusNotFound, map[string]string{"error": "user not found"})
				return
			}
			h.logger.Error("get user failed", "user_id", body.UserID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "get user failed"})
			return
		}

		host, err := h.store.UpsertHost(r.Context(), repository.UpsertHostParams{
			UserID:           body.UserID,
			Status:           "pending",
			TemplateImageRef: "managed-user",
			HomeVolumeName:   "",
			SlotKey:          "primary",
			Timezone:         timezone,
			Hostname:         hostname,
			MemoryLimitMB:    body.MemoryLimitMB,
			CPULimit:         body.CPULimit,
			DiskLimitGB:      body.DiskLimitGB,
		})
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

		task, err := h.queue.QueueHostAction(r.Context(), host.ID, agentapi.ActionCreateHost, "admin")
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

		task, err := h.queue.QueueHostAction(r.Context(), hostID, action, "admin")
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
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
			if errors.Is(err, pgx.ErrNoRows) {
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
		gwName := "cloudproxy-gw-" + hostID
		netName := "cloudproxy-net-" + hostID

		_ = dockerRm(containerName)
		_ = dockerRm(gwName)
		_ = dockerNetworkRm(netName)

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

func dockerNetworkRm(networkName string) error {
	cmd := exec.CommandContext(context.Background(), "docker", "network", "rm", networkName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		slog.Warn("docker network rm failed (may not exist)", "network", networkName, "output", string(output), "error", err)
	}
	return err
}
