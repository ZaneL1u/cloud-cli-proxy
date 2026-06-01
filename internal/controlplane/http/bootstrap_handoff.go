package http

import (
	"errors"
	"log/slog"
	nethttp "net/http"

	"database/sql"

	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

// BootstrapHandoffDependencies holds injected collaborators for the
// handoff handler.
type BootstrapHandoffDependencies struct {
	Logger *slog.Logger
	Tasks  TaskGetter
	Events EventLister
}

type bootstrapHandoffHandler struct {
	logger *slog.Logger
	tasks  TaskGetter
	events EventLister
}

// NewBootstrapHandoffHandler creates a handler for
// GET /v1/bootstrap/tasks/{taskID}/handoff.
// It returns SSH connection parameters only when the task has succeeded
// and the ssh.handoff.ready event exists (D-08).
func NewBootstrapHandoffHandler(deps BootstrapHandoffDependencies) nethttp.Handler {
	return &bootstrapHandoffHandler{
		logger: deps.Logger,
		tasks:  deps.Tasks,
		events: deps.Events,
	}
}

func (h *bootstrapHandoffHandler) ServeHTTP(w nethttp.ResponseWriter, r *nethttp.Request) {
	taskID := r.PathValue("taskID")
	if taskID == "" {
		writeBootstrapError(w, nethttp.StatusBadRequest, "invalid_request", "缺少 taskID 参数")
		return
	}

	task, err := h.tasks.GetTaskByID(r.Context(), taskID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeBootstrapError(w, nethttp.StatusNotFound, "task_not_found", "任务不存在")
			return
		}
		h.logger.Error("handoff get task failed", "task_id", taskID, "error", err)
		writeBootstrapError(w, nethttp.StatusInternalServerError, "internal_error", "查询任务失败")
		return
	}

	if task.Status == repository.TaskStatusFailed {
		entry := LookupBootstrapError(task.ErrorCode)
		writeBootstrapError(w, nethttp.StatusServiceUnavailable, task.ErrorCode, entry.Message)
		return
	}

	if task.Status != repository.TaskStatusSucceeded {
		writeBootstrapError(w, nethttp.StatusServiceUnavailable, "ssh_not_ready", "主机尚未就绪，请继续等待")
		return
	}

	meta := h.findHandoffMetadata(r, taskID)
	if meta == nil {
		writeBootstrapError(w, nethttp.StatusServiceUnavailable, "ssh_not_ready", "SSH 交接信息尚未就绪")
		return
	}

	writeJSON(w, nethttp.StatusOK, map[string]any{
		"ready":       true,
		"host":        meta["ssh_host"],
		"port":        meta["ssh_port"],
		"user":        meta["ssh_user"],
		"ssh_options": []string{"-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null"},
	})
}

func (h *bootstrapHandoffHandler) findHandoffMetadata(r *nethttp.Request, taskID string) map[string]any {
	if h.events == nil {
		return nil
	}
	events, err := h.events.ListEventsByTaskID(r.Context(), taskID, 50)
	if err != nil {
		return nil
	}
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Type == "ssh.handoff.ready" {
			return events[i].Metadata
		}
	}
	return nil
}
