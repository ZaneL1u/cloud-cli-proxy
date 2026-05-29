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

// ErrBypassReloadInvalidInput 表示调用方未透传 BypassSnapshotID。
// ErrBypassReloadFailed 表示健康检查耗尽 + 没有上一个 applied snapshot 可回滚。
var (
	ErrBypassReloadInvalidInput = errors.New("bypass reload: missing snapshot id")
	ErrBypassReloadFailed       = errors.New("bypass reload: health check exhausted and no applied snapshot to rollback to")
)

// 包级 var：测试可以调低让用例不靠真实时间推进。
var (
	healthCheckRetries  = 5
	healthCheckInterval = 1 * time.Second
	singboxReloadWait   = 2 * time.Second
)

// 测试注入点：默认绑定真实实现，单测替换为 fake 闭包。
var (
	applyBypassRuleSetHook = network.ApplyBypassRuleSet
	verifyBypassHook       = verifyBypassHealthyDefault
	sleepHook              = time.Sleep
)

// handleReloadHostBypass 执行 bypass reload 流程：
//  1. 校验 request.BypassSnapshotID 非空
//  2. 拉 snapshot 行
//  3. 调 ApplyBypassRuleSet 落盘 rule-set + nft 事务下发
//  4. sleep singboxReloadWait 等 sing-box 文件 watcher 热加载 rule_set
//  5. 健康检查 N 次
//  6. 成功：标 applied
//  7. 三次失败：自动 rollback 到上一个 applied snapshot
func (w *Worker) handleReloadHostBypass(ctx context.Context, request agentapi.HostActionRequest) error {
	if request.BypassSnapshotID == "" {
		return ErrBypassReloadInvalidInput
	}

	snap, err := w.repo.GetBypassSnapshotByID(ctx, request.BypassSnapshotID)
	if err != nil {
		return fmt.Errorf("get bypass snapshot %s: %w", request.BypassSnapshotID, err)
	}

	// 1. 下发新规则（nft + rule_set 文件）。失败直接进 rollback 流程。
	if applyErr := applyBypassRuleSetHook(ctx, request.HostID, snap.WhitelistCIDRsJSON, snap.WhitelistDomainsJSON); applyErr != nil {
		return w.markSnapshotFailedAndRollback(ctx, request, snap, fmt.Errorf("apply rule-set: %w", applyErr))
	}

	// 2. 等 sing-box 文件 watcher 检测 rule_set 变化并热加载。
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
// 验证 sing-box 进程存活 + tun 接口存在且 UP。
// 不做 TCP 探针（避免被 nft 规则误拦）。
func verifyBypassHealthyDefault(ctx context.Context, hostID string) error {
	containerName := containerNameForHost(hostID)

	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	script := `pidof sing-box >/dev/null 2>&1 && ip -o link show up 2>/dev/null | grep -q 'tun[0-9]' && echo OK || echo FAIL`
	checkCmd := exec.CommandContext(probeCtx, "docker", "exec", containerName, "sh", "-c", script)
	out, runErr := checkCmd.CombinedOutput()
	trimmed := strings.TrimSpace(string(out))
	if runErr != nil {
		return fmt.Errorf("bypass health: docker exec probe failed: %s: %w", trimmed, runErr)
	}
	if trimmed != "OK" {
		return fmt.Errorf("bypass health: docker exec probe non-OK: %q", trimmed)
	}
	return nil
}

// 编译期守护
var _ = json.RawMessage(nil)
