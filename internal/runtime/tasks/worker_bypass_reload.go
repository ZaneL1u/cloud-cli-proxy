package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"

	"github.com/zanel1u/cloud-cli-proxy/internal/agentapi"
	"github.com/zanel1u/cloud-cli-proxy/internal/network"
	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

// ErrBypassReloadInvalidInput 表示调用方未透传 BypassSnapshotID（旧 Phase 46
// placeholder 时代靠 requestedBy hack 现已废弃；任何空 snapshot id 都视为契约违规）。
//
// ErrBypassReloadFailed 表示健康检查耗尽 + 没有上一个 applied snapshot 可回滚 →
// 终态。worker.Execute 把它映射为 errorCode="bypass_reload_failed"。
var (
	ErrBypassReloadInvalidInput = errors.New("bypass reload: missing snapshot id")
	ErrBypassReloadFailed       = errors.New("bypass reload: health check exhausted and no applied snapshot to rollback to")
)

// 包级 var：测试可以调低让用例不靠真实时间推进。
var (
	healthCheckRetries  = 3
	healthCheckInterval = 200 * time.Millisecond
	singboxReloadWait   = time.Second
)

// 测试注入点：默认绑定真实实现，单测替换为 fake 闭包。
var (
	applyBypassRuleSetHook = network.ApplyBypassRuleSet
	verifyBypassHook       = verifyBypassHealthyDefault
	sleepHook              = time.Sleep
)

// handleReloadHostBypass 执行 Phase 47 Plan 01 完整 reload 流程：
//
//  1. 校验 request.BypassSnapshotID 非空（不再兼容 Phase 46 placeholder）
//  2. 拉 snapshot 行（GetBypassSnapshotByID）
//  3. 调 ApplyBypassRuleSet 落盘 rule-set + nft -f 事务下发；任一失败 → 自动 rollback
//  4. sleep singboxReloadWait 等 sing-box 文件 watch 触发热加载
//  5. 健康检查 healthCheckRetries 次，每次失败 sleep healthCheckInterval 后重试
//  6. 成功：UpdateBypassSnapshotStatus(id, "applied") + RecordEvent("bypass.reload_applied")，return nil
//  7. 三次失败：调 markSnapshotFailedAndRollback —— 找上一个 applied → 重下发 →
//     当前 snapshot 标 rolled_back + RecordEvent("bypass.reload_rolled_back")，return nil
//     （rollback 成功视为最终成功，避免 host 状态被翻为 failed）。
//     若无上一个 applied → 标 failed + RecordEvent("bypass.reload_failed")，return ErrBypassReloadFailed。
func (w *Worker) handleReloadHostBypass(ctx context.Context, request agentapi.HostActionRequest) error {
	if request.BypassSnapshotID == "" {
		return ErrBypassReloadInvalidInput
	}

	snap, err := w.repo.GetBypassSnapshotByID(ctx, request.BypassSnapshotID)
	if err != nil {
		return fmt.Errorf("get bypass snapshot %s: %w", request.BypassSnapshotID, err)
	}

	// 1. 下发新规则。失败直接进 rollback 流程。
	if applyErr := applyBypassRuleSetHook(ctx, request.HostID, snap.WhitelistCIDRsJSON, snap.WhitelistDomainsJSON); applyErr != nil {
		return w.markSnapshotFailedAndRollback(ctx, request, snap, fmt.Errorf("apply rule-set: %w", applyErr))
	}

	// 2. 等 sing-box 内部 file watcher 触发热加载（v1.11 默认轮询窗口 ~1s）。
	sleepHook(singboxReloadWait)

	// 3. 健康检查 N 次。任一次通过即视为生效，立即标 applied 返回。
	var lastCheckErr error
	for i := 0; i < healthCheckRetries; i++ {
		if checkErr := verifyBypassHook(ctx, request.HostID); checkErr == nil {
			_, _ = w.repo.UpdateBypassSnapshotStatus(ctx, snap.ID, "applied")
			w.recordReloadEvent(ctx, request, snap, "info", "bypass.reload_applied", "bypass rule-set applied and verified", "")
			return nil
		} else {
			lastCheckErr = checkErr
			if i < healthCheckRetries-1 {
				sleepHook(healthCheckInterval)
			}
		}
	}

	// 4. 健康检查耗尽 → 自动 rollback。
	cause := fmt.Errorf("health check exhausted after %d attempts: %w", healthCheckRetries, lastCheckErr)
	return w.markSnapshotFailedAndRollback(ctx, request, snap, cause)
}

// markSnapshotFailedAndRollback 在 reload 失败后试图回滚到上一个 applied snapshot。
//
// 行为契约（与 Plan 47-01 acceptance Test 2/3 对齐）：
//   - 找不到 prev applied snapshot：标 current=failed，写 bypass.reload_failed
//     event，return ErrBypassReloadFailed。worker.Execute 把它映射为 errorCode。
//   - 找得到 prev applied：重新 ApplyBypassRuleSet(prev)。即便重下发本身又失败，
//     仍把 current 标 rolled_back（防止 current 永远卡在 pending）。这种「rollback
//     also failed」case 会同时写 bypass.reload_rollback_failed event 供运维兜底。
//   - rollback 流程整体返回 nil，避免 host 状态被翻为 failed —— 旧规则继续生效。
func (w *Worker) markSnapshotFailedAndRollback(ctx context.Context, request agentapi.HostActionRequest, current repository.BypassSnapshot, cause error) error {
	prev, prevErr := w.repo.GetLatestAppliedBypassSnapshot(ctx, request.HostID)
	if prevErr != nil {
		_, _ = w.repo.UpdateBypassSnapshotStatus(ctx, current.ID, "failed")
		w.recordReloadEvent(ctx, request, current, "error", "bypass.reload_failed", "bypass reload failed and no applied snapshot to rollback to", cause.Error())
		slog.Error("bypass reload failed without rollback target",
			"host_id", request.HostID, "snapshot_id", current.ID, "cause", cause)
		return ErrBypassReloadFailed
	}

	if rbErr := applyBypassRuleSetHook(ctx, request.HostID, prev.WhitelistCIDRsJSON, prev.WhitelistDomainsJSON); rbErr != nil {
		w.recordReloadEvent(ctx, request, current, "error", "bypass.reload_rollback_failed",
			"applying previous snapshot during rollback failed", fmt.Sprintf("cause=%v; rollback_err=%v", cause, rbErr))
		slog.Error("bypass rollback re-apply failed",
			"host_id", request.HostID, "snapshot_id", current.ID, "prev_id", prev.ID, "cause", cause, "rollback_err", rbErr)
	}

	_, _ = w.repo.UpdateBypassSnapshotStatus(ctx, current.ID, "rolled_back")
	w.recordReloadEvent(ctx, request, current, "warn", "bypass.reload_rolled_back",
		"bypass reload rolled back to previous applied snapshot", cause.Error())
	slog.Warn("bypass reload rolled back",
		"host_id", request.HostID, "snapshot_id", current.ID, "prev_id", prev.ID, "cause", cause)
	return nil
}

// recordReloadEvent 统一写一条 RecordEvent。metadata 含 snapshot_id / version /
// config_hash 等审计字段，避免 RecordEvent 调用点散落 6 处分布式难追。
//   - level: "info" / "warn" / "error"
//   - eventType: 必须以 bypass.reload_* 开头，便于 SIEM 过滤
//   - msg: 人类可读的事件摘要
//   - detail: 失败 cause 摘要（applied 成功 case 传 ""）
func (w *Worker) recordReloadEvent(ctx context.Context, request agentapi.HostActionRequest, snap repository.BypassSnapshot, level, eventType, msg, detail string) {
	hostID := request.HostID
	meta := map[string]any{
		"host_id":     hostID,
		"snapshot_id": snap.ID,
		"version":     snap.Version,
		"config_hash": snap.ConfigHash,
	}
	if detail != "" {
		meta["detail"] = detail
	}
	taskID := request.TaskID
	params := repository.RecordEventParams{
		HostID:   &hostID,
		Level:    level,
		Type:     eventType,
		Message:  msg,
		Metadata: meta,
	}
	if taskID != "" {
		params.TaskID = &taskID
	}
	_, _ = w.repo.RecordEvent(ctx, params)
}

// verifyBypassHealthyDefault 默认健康检查实现：
//
// 进入 worker 容器 netns 跑一次到 RFC1918 探针 IP 的 TCP 探针（3s 超时）。
//
//	docker inspect -f '{{.State.Pid}}' cloudproxy-<hostID>
//	nsenter -t <pid> -n -- timeout 3 sh -c '
//	  exec 3<>/dev/tcp/192.168.0.1/53 2>/dev/null && echo OK
//	'
//
// 选择 192.168.0.1/53（53 端口在 sing-box hijack-dns 范围外，避免 DNS hijack
// 误判通；同时 RFC1918 段命中 ip_is_private direct route，不依赖代理后端）。
// 3s 超时硬上界防止 worker 容器卡死时 reload 流程被永远 hold。
//
// 任何非 0 退出 / nsenter 失败 / 超时都视为「健康检查不通过」并返回 error；
// 调用方据此触发重试 + rollback。
func verifyBypassHealthyDefault(ctx context.Context, hostID string) error {
	containerName := containerNameForHost(hostID)

	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	pidCmd := exec.CommandContext(probeCtx, "docker", "inspect", "-f", "{{.State.Pid}}", containerName)
	pidOut, err := pidCmd.Output()
	if err != nil {
		return fmt.Errorf("bypass health: get worker container pid: %w", err)
	}
	pid := strings.TrimSpace(string(pidOut))
	if pid == "" || pid == "0" {
		return fmt.Errorf("bypass health: worker container %s not running (pid=%q)", containerName, pid)
	}

	// 用 /dev/tcp/<host>/<port> 让 bash 直接走内核 TCP 连接，避免容器内
	// 缺 curl / nc 等工具时 false-negative。timeout 3 是 nsenter 内部第二道闸。
	script := `exec 3<>/dev/tcp/192.168.0.1/53 2>/dev/null && echo OK || echo FAIL`
	checkCmd := exec.CommandContext(probeCtx, "nsenter", "-t", pid, "-n", "--", "timeout", "3", "bash", "-c", script)
	out, runErr := checkCmd.CombinedOutput()
	trimmed := strings.TrimSpace(string(out))
	if runErr != nil {
		return fmt.Errorf("bypass health: nsenter probe failed: %s: %w", trimmed, runErr)
	}
	if trimmed != "OK" {
		return fmt.Errorf("bypass health: nsenter probe non-OK: %q", trimmed)
	}
	return nil
}

// 编译期守护：json import 在测试用 fake snapshot 时常用，避免 IDE 误删；
// network 包 import 同时由 applyBypassRuleSetHook 持有引用。
var _ = json.RawMessage(nil)
