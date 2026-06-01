package http

import (
	"errors"
	"log/slog"
	nethttp "net/http"

	"database/sql"

	"github.com/zanel1u/cloud-cli-proxy/internal/agentapi"
)

const (
	createHostRoute  = "POST /v1/hosts/{hostID}/create"
	startHostRoute   = "POST /v1/hosts/{hostID}/start"
	stopHostRoute    = "POST /v1/hosts/{hostID}/stop"
	rebuildHostRoute = "POST /v1/hosts/{hostID}/rebuild"
)

type HostActionHandlerDependencies struct {
	Logger *slog.Logger
	Queue  HostActionQueuer
}

type HostActionsHandler struct {
	logger *slog.Logger
	queue  HostActionQueuer
}

func NewHostActionsHandler(deps HostActionHandlerDependencies) HostActionsHandler {
	return HostActionsHandler{
		logger: deps.Logger,
		queue:  deps.Queue,
	}
}

func (h HostActionsHandler) Create() nethttp.Handler {
	return h.handleLifecycleAction(createHostRoute, agentapi.ActionCreateHost)
}

func (h HostActionsHandler) Start() nethttp.Handler {
	return h.handleLifecycleAction(startHostRoute, agentapi.ActionStartHost)
}

func (h HostActionsHandler) Stop() nethttp.Handler {
	return h.handleLifecycleAction(stopHostRoute, agentapi.ActionStopHost)
}

func (h HostActionsHandler) Rebuild() nethttp.Handler {
	return h.handleLifecycleAction(rebuildHostRoute, agentapi.ActionRebuildHost)
}

func (h HostActionsHandler) handleLifecycleAction(route string, action agentapi.HostAction) nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		hostID := r.PathValue("hostID")
		if hostID == "" {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "host_id is required"})
			return
		}
		if h.queue == nil {
			writeJSON(w, nethttp.StatusServiceUnavailable, map[string]string{"error": "runtime queue unavailable"})
			return
		}

		requestedBy := r.Header.Get("X-Requested-By")
		if requestedBy == "" {
			requestedBy = "control-plane"
		}

		task, err := h.queue.QueueHostAction(r.Context(), hostID, action, requestedBy, "")
		if err != nil {
			if h.logger != nil {
				h.logger.Error("queue host action failed", "route", route, "host_id", hostID, "error", err)
			}

			status := nethttp.StatusInternalServerError
			if errors.Is(err, sql.ErrNoRows) {
				status = nethttp.StatusNotFound
			}
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, nethttp.StatusAccepted, map[string]any{
			"route":   route,
			"task_id": task.ID,
			"status":  "202 Accepted",
		})
	})
}
