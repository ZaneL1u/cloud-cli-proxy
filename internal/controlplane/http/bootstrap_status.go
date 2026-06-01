package http

import (
	"context"
	"errors"
	"log/slog"
	nethttp "net/http"

	"database/sql"

	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

type TaskGetter interface {
	GetTaskByID(context.Context, string) (repository.Task, error)
}

type EventLister interface {
	ListEventsByTaskID(context.Context, string, int) ([]repository.Event, error)
}

type BootstrapStatusDependencies struct {
	Logger *slog.Logger
	Tasks  TaskGetter
	Events EventLister
}

type bootstrapStatusHandler struct {
	logger *slog.Logger
	tasks  TaskGetter
	events EventLister
}

func NewBootstrapStatusHandler(deps BootstrapStatusDependencies) nethttp.Handler {
	return &bootstrapStatusHandler{
		logger: deps.Logger,
		tasks:  deps.Tasks,
		events: deps.Events,
	}
}

type stageMapping struct {
	code string
	text string
}

var stagesByEventType = map[string]stageMapping{
	"ssh.ready":          {code: "ssh_ready", text: "SSH 就绪"},
	"runtime.validating": {code: "runtime_validating", text: "运行时校验中"},
	"net.ready":          {code: "host_starting", text: "主机启动中"},
}

var defaultStage = stageMapping{code: "host_starting", text: "主机启动中"}

func (h *bootstrapStatusHandler) ServeHTTP(w nethttp.ResponseWriter, r *nethttp.Request) {
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
		h.logger.Error("get task failed", "task_id", taskID, "error", err)
		writeBootstrapError(w, nethttp.StatusInternalServerError, "internal_error", "查询任务失败")
		return
	}

	stage := resolveStage(r.Context(), h.events, taskID, task.Status)

	resp := map[string]any{
		"task_id":     task.ID,
		"task_status": string(task.Status),
		"stage_code":  stage.code,
		"stage_text":  stage.text,
	}

	if task.Status == repository.TaskStatusFailed {
		resp["error_code"] = task.ErrorCode
		resp["error_message"] = task.LastErrorSummary
		resp["retryable"] = false
	}

	writeJSON(w, nethttp.StatusOK, resp)
}

func resolveStage(ctx context.Context, events EventLister, taskID string, status repository.TaskStatus) stageMapping {
	if status == repository.TaskStatusPending {
		return defaultStage
	}

	if events == nil {
		return defaultStage
	}

	eventList, err := events.ListEventsByTaskID(ctx, taskID, 50)
	if err != nil || len(eventList) == 0 {
		return defaultStage
	}

	// Walk events in reverse to find the highest-priority stage reached.
	// Priority order: ssh.ready > runtime.validating > net.ready
	for i := len(eventList) - 1; i >= 0; i-- {
		if s, ok := stagesByEventType[eventList[i].Type]; ok {
			return s
		}
	}

	return defaultStage
}
