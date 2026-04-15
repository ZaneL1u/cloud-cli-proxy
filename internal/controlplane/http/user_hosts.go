package http

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	nethttp "net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/zanel1u/cloud-cli-proxy/internal/agentapi"
	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

// UserHostStore defines the data access interface for user host endpoints.
type UserHostStore interface {
	ListHostsWithEgressByUserID(context.Context, string) ([]repository.UserHostSummary, error)
	GetHost(context.Context, string) (repository.Host, error)
	GetHostDetail(context.Context, string) (repository.HostDetail, error)
	GetUser(context.Context, string) (repository.User, error)
}

// UserHostsHandler handles user self-service host endpoints.
type UserHostsHandler struct {
	logger *slog.Logger
	store  UserHostStore
	queue  HostActionQueuer
	events EventRecorder
}

// NewUserHostsHandler creates a new UserHostsHandler.
func NewUserHostsHandler(logger *slog.Logger, store UserHostStore, queue HostActionQueuer, events EventRecorder) *UserHostsHandler {
	return &UserHostsHandler{logger: logger, store: store, queue: queue, events: events}
}

// List returns a handler for GET /v1/user/hosts.
func (h *UserHostsHandler) List() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		userID := UserIDFromContext(r.Context())
		if userID == "" {
			writeJSON(w, nethttp.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}

		hosts, err := h.store.ListHostsWithEgressByUserID(r.Context(), userID)
		if err != nil {
			h.logger.Error("list user hosts failed", "user_id", userID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "list hosts failed"})
			return
		}

		writeJSON(w, nethttp.StatusOK, map[string]any{"hosts": hosts})
	})
}

// Get returns a handler for GET /v1/user/hosts/{hostID}.
func (h *UserHostsHandler) Get() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		userID := UserIDFromContext(r.Context())
		role := RoleFromContext(r.Context())
		hostID := r.PathValue("hostID")

		if userID == "" {
			writeJSON(w, nethttp.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}

		// Ownership check: fetch host first
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

		// Admin skips ownership check
		if role != "admin" && host.UserID != userID {
			writeJSON(w, nethttp.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}

		// Get full detail
		detail, err := h.store.GetHostDetail(r.Context(), hostID)
		if err != nil {
			h.logger.Error("get host detail failed", "host_id", hostID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "get host detail failed"})
			return
		}

		// Build response with sensitive fields filtered
		bindings := make([]repository.UserEgressBinding, 0, len(detail.Bindings))
		for _, b := range detail.Bindings {
			bindings = append(bindings, repository.UserEgressBinding{
				IPAddress: b.EgressIP.IPAddress,
			})
		}

		resp := repository.UserHostDetail{
			ID:             detail.Host.ID,
			Hostname:       detail.Host.Hostname,
			Status:         detail.Host.Status,
			Timezone:       detail.Host.Timezone,
			CreatedAt:      detail.Host.CreatedAt,
			UpdatedAt:      detail.Host.UpdatedAt,
			EgressBindings: bindings,
		}

		if user, err := h.store.GetUser(r.Context(), userID); err == nil {
			scheme := "https"
			if r.TLS == nil {
				scheme = "http"
			}
			host := r.Host
			if idx := strings.Index(host, ":"); idx != -1 {
				host = host[:idx]
			}
			baseURL := fmt.Sprintf("%s://%s", scheme, r.Host)
			sshTarget := detail.Host.ShortID
			if sshTarget == "" {
				sshTarget = user.ShortID
			}
			if user.ShortID != "" || sshTarget != "" {
				entryID := detail.Host.ShortID
				if entryID == "" {
					entryID = user.ShortID
				}
				resp.ConnectionInfo = &repository.ConnectionInfo{
					CurlCommand: fmt.Sprintf("curl -sSL %s/entry/%s | bash", baseURL, entryID),
					SSHCommand:  fmt.Sprintf("ssh %s@%s -p 2222", sshTarget, host),
					SSHPort:     2222,
					VNCURL:      fmt.Sprintf("%s/v1/user/hosts/%s/vnc/vnc.html", baseURL, detail.Host.ID),
				}
			}
		}

		writeJSON(w, nethttp.StatusOK, resp)
	})
}

// RestartVNC returns a handler for POST /v1/user/hosts/{hostID}/vnc/restart.
func (h *UserHostsHandler) RestartVNC() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		userID := UserIDFromContext(r.Context())
		role := RoleFromContext(r.Context())
		hostID := r.PathValue("hostID")

		if userID == "" {
			writeJSON(w, nethttp.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}

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

		if role != "admin" && host.UserID != userID {
			writeJSON(w, nethttp.StatusForbidden, map[string]string{"error": "forbidden"})
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
				HostID:  &hostID,
				UserID:  &userID,
				Level:   "info",
				Type:    "user.host.vnc_restarted",
				Message: "用户重启 VNC 服务",
				Metadata: map[string]any{
					"user_id": userID,
				},
			}); err != nil {
				h.logger.Error("record event failed", "type", "user.host.vnc_restarted", "error", err)
			}
		}

		writeJSON(w, nethttp.StatusOK, map[string]any{"status": "restarted"})
	})
}

// Rebuild returns a handler for POST /v1/user/hosts/{hostID}/rebuild.
func (h *UserHostsHandler) Rebuild() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		userID := UserIDFromContext(r.Context())
		role := RoleFromContext(r.Context())
		hostID := r.PathValue("hostID")

		if userID == "" {
			writeJSON(w, nethttp.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}

		// Ownership check
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

		// Admin skips ownership check
		if role != "admin" && host.UserID != userID {
			writeJSON(w, nethttp.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}

		task, err := h.queue.QueueHostAction(r.Context(), hostID, agentapi.ActionRebuildHost, userID)
		if err != nil {
			h.logger.Error("queue rebuild failed", "host_id", hostID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "queue rebuild failed"})
			return
		}

		// Record event
		if h.events != nil {
			if _, err := h.events.RecordEvent(r.Context(), repository.RecordEventParams{
				HostID:  &hostID,
				UserID:  &userID,
				Level:   "info",
				Type:    "user.host.rebuild",
				Message: "用户发起主机重建",
				Metadata: map[string]any{
					"user_id": userID,
				},
			}); err != nil {
				h.logger.Error("record event failed", "type", "user.host.rebuild", "error", err)
			}
		}

		writeJSON(w, nethttp.StatusAccepted, map[string]any{
			"task_id": task.ID,
			"status":  "202 Accepted",
		})
	})
}
