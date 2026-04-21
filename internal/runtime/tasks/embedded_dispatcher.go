package tasks

import (
	"context"
	"log/slog"

	"github.com/zanel1u/cloud-cli-proxy/internal/agentapi"
)

type EmbeddedDispatcher struct {
	worker *Worker
}

func NewEmbeddedDispatcher(worker *Worker) *EmbeddedDispatcher {
	return &EmbeddedDispatcher{worker: worker}
}

func (d *EmbeddedDispatcher) Dispatch(ctx context.Context, request agentapi.HostActionRequest) (agentapi.HostActionResponse, error) {
	slog.Info("embedded: executing host action in-process",
		"task_id", request.TaskID,
		"host_id", request.HostID,
		"action", request.Action,
	)

	update := d.worker.Execute(ctx, request)

	if err := d.worker.UpdateTaskStatus(ctx, update); err != nil {
		slog.Error("embedded: failed to update task status", "error", err)
	}

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
