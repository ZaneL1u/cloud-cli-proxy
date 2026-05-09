package tasks

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/zanel1u/cloud-cli-proxy/internal/agentapi"
	"github.com/zanel1u/cloud-cli-proxy/internal/broadcast"
)

type EmbeddedDispatcher struct {
	worker *Worker
}

func NewEmbeddedDispatcher(worker *Worker) *EmbeddedDispatcher {
	return &EmbeddedDispatcher{worker: worker}
}

func (d *EmbeddedDispatcher) Dispatch(ctx context.Context, request agentapi.HostActionRequest) (resp agentapi.HostActionResponse, err error) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("dispatcher panic recovered",
				"task_id", request.TaskID,
				"host_id", request.HostID,
				"action", request.Action,
				"panic", r,
			)
			fallback := agentapi.TaskStatusUpdate{
				TaskID:       request.TaskID,
				Status:       taskStateFailed,
				ErrorCode:    "panic_recovered",
				ErrorMessage: fmt.Sprintf("dispatcher panic: %v", r),
			}
			_ = d.worker.UpdateTaskStatus(ctx, fallback)
			broadcast.Broadcast("tasks", "update", request.TaskID)
			resp = agentapi.HostActionResponse{Update: fallback}
			err = nil
		}
	}()

	slog.Info("embedded: executing host action in-process",
		"task_id", request.TaskID,
		"host_id", request.HostID,
		"action", request.Action,
	)

	running := agentapi.TaskStatusUpdate{
		TaskID: request.TaskID,
		Status: taskStateRunning,
	}
	if err := d.worker.UpdateTaskStatus(ctx, running); err != nil {
		slog.Error("embedded: failed to update task status to running", "error", err)
	}
	broadcast.Broadcast("tasks", "update", request.TaskID)

	update := d.worker.Execute(ctx, request)

	if uerr := d.worker.UpdateTaskStatus(ctx, update); uerr != nil {
		slog.Error("embedded: failed to update task status", "error", uerr)
	}
	broadcast.Broadcast("tasks", "update", request.TaskID)

	return agentapi.HostActionResponse{Update: update}, nil
}

// RunHostAction 让 EmbeddedDispatcher 满足 cphttp.HostActionRunner 接口，
// 这样 admin DELETE claude-accounts handler 在 embedded 模式（无 host-agent socket）
// 也能复用同一段调用代码触达 worker.removeVolumes。
//
// Phase 33：Plan 02 admin handler 设计假设有远端 *agentapi.Client，但 embedded 模式
// 没有 socket 可连，必须经此适配器路由到 in-process worker。
func (d *EmbeddedDispatcher) RunHostAction(ctx context.Context, request agentapi.HostActionRequest) (agentapi.HostActionResponse, error) {
	return d.Dispatch(ctx, request)
}
