package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/zanel1u/cloud-cli-proxy/internal/agentapi"
	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

// bypassReloadFakeRepo 是 worker_bypass_reload 系列测试专用的 WorkerRepo 实现。
// 它继承 fakeWorkerRepo（ssh_inject_test.go）已有的事件记录能力，并叠加
// Bypass 三件套（GetBypassSnapshotByID / UpdateBypassSnapshotStatus /
// GetLatestAppliedBypassSnapshot）的可注入返回值。这样 ssh_inject_test 的最小
// no-op 实现不会被覆盖污染，每个测试独立配置自己想要的 Bypass 行为。
type bypassReloadFakeRepo struct {
	fakeWorkerRepo

	mu sync.Mutex

	// 注入：GetBypassSnapshotByID
	snapshotByID    map[string]repository.BypassSnapshot
	snapshotByIDErr error

	// 注入：UpdateBypassSnapshotStatus
	statusUpdates []bypassStatusUpdate

	// 注入：GetLatestAppliedBypassSnapshot
	latestApplied    *repository.BypassSnapshot
	latestAppliedErr error
}

type bypassStatusUpdate struct {
	ID     string
	Status string
}

func (r *bypassReloadFakeRepo) GetBypassSnapshotByID(_ context.Context, id string) (repository.BypassSnapshot, error) {
	if r.snapshotByIDErr != nil {
		return repository.BypassSnapshot{}, r.snapshotByIDErr
	}
	if snap, ok := r.snapshotByID[id]; ok {
		return snap, nil
	}
	return repository.BypassSnapshot{}, pgx.ErrNoRows
}

func (r *bypassReloadFakeRepo) UpdateBypassSnapshotStatus(_ context.Context, id, status string) (repository.BypassSnapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.statusUpdates = append(r.statusUpdates, bypassStatusUpdate{ID: id, Status: status})
	return repository.BypassSnapshot{ID: id, AppliedStatus: status}, nil
}

func (r *bypassReloadFakeRepo) GetLatestAppliedBypassSnapshot(_ context.Context, _ string) (repository.BypassSnapshot, error) {
	if r.latestAppliedErr != nil {
		return repository.BypassSnapshot{}, r.latestAppliedErr
	}
	if r.latestApplied == nil {
		return repository.BypassSnapshot{}, pgx.ErrNoRows
	}
	return *r.latestApplied, nil
}

// withFastHealthChecks 把 sleepHook / 检查间隔调到 0，让测试不依赖真实时间。
// 返回 cleanup 由 t.Cleanup 自动调用。
func withFastHealthChecks(t *testing.T) {
	t.Helper()
	prevSleep := sleepHook
	sleepHook = func(_ time.Duration) {}
	t.Cleanup(func() { sleepHook = prevSleep })

	prevInterval := healthCheckInterval
	prevReload := singboxReloadWait
	healthCheckInterval = 0
	singboxReloadWait = 0
	t.Cleanup(func() {
		healthCheckInterval = prevInterval
		singboxReloadWait = prevReload
	})
}

// stubApplyHook / stubVerifyHook 用闭包替换包级 var，并在 t.Cleanup 还原。
type applyCall struct {
	HostID string
	CIDRs  json.RawMessage
}

func stubApplyHook(t *testing.T, fn func(ctx context.Context, hostID string, cidrsJSON, domainsJSON json.RawMessage) error) *[]applyCall {
	t.Helper()
	calls := &[]applyCall{}
	var mu sync.Mutex
	prev := applyBypassRuleSetHook
	applyBypassRuleSetHook = func(ctx context.Context, hostID string, cidrsJSON, domainsJSON json.RawMessage) error {
		mu.Lock()
		*calls = append(*calls, applyCall{HostID: hostID, CIDRs: cidrsJSON})
		mu.Unlock()
		return fn(ctx, hostID, cidrsJSON, domainsJSON)
	}
	t.Cleanup(func() { applyBypassRuleSetHook = prev })
	return calls
}

func stubVerifyHook(t *testing.T, fn func(ctx context.Context, hostID string) error) *int {
	t.Helper()
	count := 0
	var mu sync.Mutex
	prev := verifyBypassHook
	verifyBypassHook = func(ctx context.Context, hostID string) error {
		mu.Lock()
		count++
		mu.Unlock()
		return fn(ctx, hostID)
	}
	t.Cleanup(func() { verifyBypassHook = prev })
	return &count
}

func sampleSnapshot(id, hostID, version string) repository.BypassSnapshot {
	v := int64(1)
	if version != "" {
		// 用最简单的位运算 hash 让不同字符串映射不同 version，单测够用
		var sum int64
		for _, c := range version {
			sum = sum*31 + int64(c)
		}
		v = sum
	}
	return repository.BypassSnapshot{
		ID:                   id,
		HostID:               hostID,
		Version:              v,
		ConfigHash:           "hash-" + id,
		WhitelistCIDRsJSON:   json.RawMessage(`{"version":3,"rules":[{"ip_cidr":["10.0.0.0/8"]}]}`),
		WhitelistDomainsJSON: json.RawMessage(`{"version":3,"rules":[]}`),
		AppliedStatus:        "pending",
		Source:               "apply",
	}
}

// ===== Test 1: success path =====

// TestHandleReloadHostBypass_Success 守护 acceptance Test 1：
//   - GetBypassSnapshotByID 返回 pending snapshot
//   - applyBypassRuleSetHook 成功
//   - 健康检查首次即通过
//   - 期望 UpdateBypassSnapshotStatus(id, "applied") + RecordEvent("bypass.reload_applied")
//   - err == nil
func TestHandleReloadHostBypass_Success(t *testing.T) {
	withFastHealthChecks(t)

	snap := sampleSnapshot("snap-ok", "h-ok", "v1")
	repo := &bypassReloadFakeRepo{
		snapshotByID: map[string]repository.BypassSnapshot{snap.ID: snap},
	}
	w := NewWorker(repo, nil)

	applyCalls := stubApplyHook(t, func(_ context.Context, _ string, _, _ json.RawMessage) error { return nil })
	checkCount := stubVerifyHook(t, func(_ context.Context, _ string) error { return nil })

	req := agentapi.HostActionRequest{
		TaskID:           "t-ok",
		HostID:           "h-ok",
		Action:           agentapi.ActionReloadHostBypass,
		BypassSnapshotID: snap.ID,
	}
	if err := w.handleReloadHostBypass(context.Background(), req); err != nil {
		t.Fatalf("expected success, got err=%v", err)
	}

	if got := len(*applyCalls); got != 1 {
		t.Errorf("ApplyBypassRuleSet should be called once, got %d", got)
	}
	if *checkCount != 1 {
		t.Errorf("verify hook should be called once on first-pass success, got %d", *checkCount)
	}
	if len(repo.statusUpdates) != 1 || repo.statusUpdates[0].ID != snap.ID || repo.statusUpdates[0].Status != "applied" {
		t.Errorf("expected single UpdateBypassSnapshotStatus(%s, applied), got %+v", snap.ID, repo.statusUpdates)
	}

	if !hasEventType(repo.events, "bypass.reload_applied") {
		t.Errorf("expected bypass.reload_applied event, got %+v", repo.events)
	}
}

// ===== Test 2: auto rollback =====

// TestHandleReloadHostBypass_AutoRollback 守护 acceptance Test 2：
//   - 健康检查连续 3 次失败
//   - 存在 prev applied snapshot → 调 ApplyBypassRuleSet 重下发旧规则
//   - UpdateBypassSnapshotStatus(currentID, "rolled_back")
//   - RecordEvent("bypass.reload_rolled_back")
//   - Worker.Execute return nil（rollback 成功视为最终成功）
func TestHandleReloadHostBypass_AutoRollback(t *testing.T) {
	withFastHealthChecks(t)

	current := sampleSnapshot("snap-new", "h-rb", "v2")
	prev := sampleSnapshot("snap-prev", "h-rb", "v1")
	prev.AppliedStatus = "applied"

	repo := &bypassReloadFakeRepo{
		snapshotByID:  map[string]repository.BypassSnapshot{current.ID: current},
		latestApplied: &prev,
	}
	w := NewWorker(repo, nil)

	applyCalls := stubApplyHook(t, func(_ context.Context, _ string, _, _ json.RawMessage) error { return nil })
	checkCount := stubVerifyHook(t, func(_ context.Context, _ string) error {
		return errors.New("synthetic health probe failure")
	})

	req := agentapi.HostActionRequest{
		TaskID:           "t-rb",
		HostID:           "h-rb",
		Action:           agentapi.ActionReloadHostBypass,
		BypassSnapshotID: current.ID,
	}
	if err := w.handleReloadHostBypass(context.Background(), req); err != nil {
		t.Fatalf("rollback path must return nil (rollback 成功视为最终成功); got err=%v", err)
	}

	if *checkCount != healthCheckRetries {
		t.Errorf("verify hook should be exhausted %d times, got %d", healthCheckRetries, *checkCount)
	}

	if got := len(*applyCalls); got != 2 {
		t.Errorf("ApplyBypassRuleSet should be called 2x (new + rollback), got %d", got)
	}
	if !bytesEqual(t, (*applyCalls)[1].CIDRs, prev.WhitelistCIDRsJSON) {
		t.Errorf("rollback apply must use prev.WhitelistCIDRsJSON; got=%s want=%s",
			string((*applyCalls)[1].CIDRs), string(prev.WhitelistCIDRsJSON))
	}

	if len(repo.statusUpdates) != 1 || repo.statusUpdates[0].ID != current.ID || repo.statusUpdates[0].Status != "rolled_back" {
		t.Errorf("expected UpdateBypassSnapshotStatus(%s, rolled_back), got %+v", current.ID, repo.statusUpdates)
	}

	if !hasEventType(repo.events, "bypass.reload_rolled_back") {
		t.Errorf("expected bypass.reload_rolled_back event, got %+v", repo.events)
	}
}

// ===== Test 3: failed terminal =====

// TestHandleReloadHostBypass_NoApplied_FailedTerminal 守护 acceptance Test 3：
//   - 健康检查失败 + GetLatestAppliedBypassSnapshot 返回 pgx.ErrNoRows
//   - UpdateBypassSnapshotStatus(currentID, "failed")
//   - RecordEvent("bypass.reload_failed")
//   - return ErrBypassReloadFailed
func TestHandleReloadHostBypass_NoApplied_FailedTerminal(t *testing.T) {
	withFastHealthChecks(t)

	current := sampleSnapshot("snap-first", "h-first", "v1")
	repo := &bypassReloadFakeRepo{
		snapshotByID:     map[string]repository.BypassSnapshot{current.ID: current},
		latestAppliedErr: pgx.ErrNoRows,
	}
	w := NewWorker(repo, nil)

	_ = stubApplyHook(t, func(_ context.Context, _ string, _, _ json.RawMessage) error { return nil })
	_ = stubVerifyHook(t, func(_ context.Context, _ string) error {
		return errors.New("synthetic probe failure")
	})

	req := agentapi.HostActionRequest{
		TaskID:           "t-first",
		HostID:           "h-first",
		Action:           agentapi.ActionReloadHostBypass,
		BypassSnapshotID: current.ID,
	}
	err := w.handleReloadHostBypass(context.Background(), req)
	if !errors.Is(err, ErrBypassReloadFailed) {
		t.Fatalf("expected ErrBypassReloadFailed sentinel, got %v", err)
	}

	if len(repo.statusUpdates) != 1 || repo.statusUpdates[0].ID != current.ID || repo.statusUpdates[0].Status != "failed" {
		t.Errorf("expected UpdateBypassSnapshotStatus(%s, failed), got %+v", current.ID, repo.statusUpdates)
	}
	if !hasEventType(repo.events, "bypass.reload_failed") {
		t.Errorf("expected bypass.reload_failed event, got %+v", repo.events)
	}
}

// ===== Test 4: missing snapshot id =====

// TestHandleReloadHostBypass_MissingSnapshotID 守护 acceptance Test 4：
//   - request.BypassSnapshotID == "" → 立即 return ErrBypassReloadInvalidInput
//   - 不应调 GetBypassSnapshotByID / ApplyBypassRuleSet / 任何健康检查
func TestHandleReloadHostBypass_MissingSnapshotID(t *testing.T) {
	withFastHealthChecks(t)

	repo := &bypassReloadFakeRepo{}
	w := NewWorker(repo, nil)

	applyCalls := stubApplyHook(t, func(_ context.Context, _ string, _, _ json.RawMessage) error { return nil })
	checkCount := stubVerifyHook(t, func(_ context.Context, _ string) error { return nil })

	req := agentapi.HostActionRequest{
		TaskID: "t-missing",
		HostID: "h-missing",
		Action: agentapi.ActionReloadHostBypass,
		// BypassSnapshotID 故意留空
	}
	err := w.handleReloadHostBypass(context.Background(), req)
	if !errors.Is(err, ErrBypassReloadInvalidInput) {
		t.Fatalf("expected ErrBypassReloadInvalidInput, got %v", err)
	}
	if len(*applyCalls) != 0 {
		t.Errorf("missing snapshot id must short-circuit before ApplyBypassRuleSet; got %d calls", len(*applyCalls))
	}
	if *checkCount != 0 {
		t.Errorf("missing snapshot id must not invoke verifyBypassHook; got %d calls", *checkCount)
	}
	if len(repo.statusUpdates) != 0 {
		t.Errorf("missing snapshot id must not write status updates; got %+v", repo.statusUpdates)
	}
}

// ===== Test 5: dispatcher integration =====

// TestExecute_Dispatch_ReloadHostBypass 守护 acceptance Test 5：
//   - Worker.Execute(request) 命中 case ActionReloadHostBypass 后调用 handleReloadHostBypass
//   - 不再写 "Phase 46 placeholder; no-op until Phase 47" 字面日志
//   - Status 成功路径返回 succeeded
func TestExecute_Dispatch_ReloadHostBypass(t *testing.T) {
	withFastHealthChecks(t)

	snap := sampleSnapshot("snap-dispatch", "h-dispatch", "v1")
	repo := &bypassReloadFakeRepo{
		snapshotByID: map[string]repository.BypassSnapshot{snap.ID: snap},
	}
	w := NewWorker(repo, nil)

	applyCalls := stubApplyHook(t, func(_ context.Context, _ string, _, _ json.RawMessage) error { return nil })
	checkCount := stubVerifyHook(t, func(_ context.Context, _ string) error { return nil })

	req := agentapi.HostActionRequest{
		TaskID:           "t-dispatch",
		HostID:           "h-dispatch",
		Action:           agentapi.ActionReloadHostBypass,
		BypassSnapshotID: snap.ID,
	}
	update := w.Execute(context.Background(), req)
	if update.Status != taskStateSucceeded {
		t.Errorf("expected succeeded, got status=%q err=%q", update.Status, update.ErrorCode)
	}

	if got := len(*applyCalls); got != 1 {
		t.Errorf("Execute should dispatch handleReloadHostBypass which calls ApplyBypassRuleSet once, got %d", got)
	}
	if *checkCount != 1 {
		t.Errorf("expected verify hook called once via Execute dispatch, got %d", *checkCount)
	}
}

// ===== helpers =====

func bytesEqual(t *testing.T, a, b json.RawMessage) bool {
	t.Helper()
	return strings.TrimSpace(string(a)) == strings.TrimSpace(string(b))
}
