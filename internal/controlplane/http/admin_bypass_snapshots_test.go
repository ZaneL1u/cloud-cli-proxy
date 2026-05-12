package http

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/zanel1u/cloud-cli-proxy/internal/agentapi"
	"github.com/zanel1u/cloud-cli-proxy/internal/network"
	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

// stubBypassSnapshotStore 实现 AdminBypassSnapshotStore + BypassAuditLogWriter。
// 全部 mock state 由 mu 保护，测试可并发安全断言计数。
type stubBypassSnapshotStore struct {
	mu sync.Mutex

	// 静态返回值
	host           repository.Host
	hostErr        error
	bindings       []repository.BypassBinding
	bindingsErr    error
	presetByID     map[string]repository.BypassPreset
	rules          []repository.BypassRule
	snapshots      []repository.BypassSnapshot // 用于 ListBypassSnapshotsByHost
	snapshotByID   map[string]repository.BypassSnapshot
	latestApplied  repository.BypassSnapshot
	latestErr      error
	createOut      repository.BypassSnapshot
	createErr      error
	createErrOnce  bool // 第一次返回 createErr，第二次成功
	createCallSeen int
	auditErr       error

	// 计数器（验证 rollback 不触发 UpdateBypassSnapshotStatus）
	createCalls               []repository.CreateBypassSnapshotParams
	getSnapshotByIDCalls      []string
	updateSnapshotStatusCalls []string // 期望恒为 0
	auditLogs                 []repository.InsertBypassAuditLogParams
}

func newStubSnapStore() *stubBypassSnapshotStore {
	return &stubBypassSnapshotStore{
		presetByID:   map[string]repository.BypassPreset{},
		snapshotByID: map[string]repository.BypassSnapshot{},
	}
}

func (s *stubBypassSnapshotStore) GetHost(_ context.Context, _ string) (repository.Host, error) {
	if s.hostErr != nil {
		return repository.Host{}, s.hostErr
	}
	return s.host, nil
}

func (s *stubBypassSnapshotStore) ListBypassBindingsByHost(_ context.Context, _ string) ([]repository.BypassBinding, error) {
	if s.bindingsErr != nil {
		return nil, s.bindingsErr
	}
	return s.bindings, nil
}

func (s *stubBypassSnapshotStore) GetBypassPresetByID(_ context.Context, id string) (repository.BypassPreset, error) {
	p, ok := s.presetByID[id]
	if !ok {
		return repository.BypassPreset{}, pgx.ErrNoRows
	}
	return p, nil
}

func (s *stubBypassSnapshotStore) ListBypassRules(_ context.Context, _ *string) ([]repository.BypassRule, error) {
	return s.rules, nil
}

func (s *stubBypassSnapshotStore) ListBypassSnapshotsByHost(_ context.Context, _ string, _ int) ([]repository.BypassSnapshot, error) {
	return s.snapshots, nil
}

func (s *stubBypassSnapshotStore) CreateBypassSnapshot(_ context.Context, p repository.CreateBypassSnapshotParams) (repository.BypassSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.createCallSeen++
	s.createCalls = append(s.createCalls, p)
	if s.createErrOnce && s.createCallSeen == 1 && s.createErr != nil {
		return repository.BypassSnapshot{}, s.createErr
	}
	if !s.createErrOnce && s.createErr != nil {
		return repository.BypassSnapshot{}, s.createErr
	}
	// 默认按入参回填一个 snapshot
	out := s.createOut
	if out.ID == "" {
		out = repository.BypassSnapshot{
			ID:                   "snap-new",
			HostID:               p.HostID,
			Version:              p.Version,
			ConfigHash:           p.ConfigHash,
			WhitelistCIDRsJSON:   p.WhitelistCIDRsJSON,
			WhitelistDomainsJSON: p.WhitelistDomainsJSON,
			AppliedStatus:        "pending",
			Source:               p.Source,
		}
	}
	return out, nil
}

func (s *stubBypassSnapshotStore) GetBypassSnapshotByID(_ context.Context, id string) (repository.BypassSnapshot, error) {
	s.mu.Lock()
	s.getSnapshotByIDCalls = append(s.getSnapshotByIDCalls, id)
	s.mu.Unlock()
	snap, ok := s.snapshotByID[id]
	if !ok {
		return repository.BypassSnapshot{}, pgx.ErrNoRows
	}
	return snap, nil
}

func (s *stubBypassSnapshotStore) GetLatestAppliedBypassSnapshot(_ context.Context, _ string) (repository.BypassSnapshot, error) {
	if s.latestErr != nil {
		return repository.BypassSnapshot{}, s.latestErr
	}
	return s.latestApplied, nil
}

func (s *stubBypassSnapshotStore) InsertBypassAuditLog(_ context.Context, p repository.InsertBypassAuditLogParams) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.auditErr != nil {
		return "", s.auditErr
	}
	s.auditLogs = append(s.auditLogs, p)
	return "audit-1", nil
}

// stubHostActionQueuer 实现 HostActionQueuer，计数 QueueHostAction 调用并捕获 payload。
// Phase 47 Plan 01 起，第 4 参是真实 requestedBy（admin / actor user-id），第 5 参 Payload
// 才是 reload_host_bypass 的 snapshot ID。Phase 46 的「借用 requestedBy 传 snapshot ID」hack 已修复。
type stubHostActionQueuer struct {
	mu        sync.Mutex
	calls     []stubHostActionCall
	queueErr  error
	returnTID string
}

type stubHostActionCall struct {
	HostID      string
	Action      agentapi.HostAction
	RequestedBy string
	Payload     string
}

func (q *stubHostActionQueuer) QueueHostAction(_ context.Context, hostID string, action agentapi.HostAction, requestedBy string, payload string) (repository.Task, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.calls = append(q.calls, stubHostActionCall{HostID: hostID, Action: action, RequestedBy: requestedBy, Payload: payload})
	if q.queueErr != nil {
		return repository.Task{}, q.queueErr
	}
	tid := q.returnTID
	if tid == "" {
		tid = "task-1"
	}
	return repository.Task{ID: tid}, nil
}

// stubEventRecorderSnap 实现 EventRecorder，记录 RecordEvent 调用。
type stubEventRecorderSnap struct {
	mu     sync.Mutex
	events []repository.RecordEventParams
}

func (e *stubEventRecorderSnap) RecordEvent(_ context.Context, p repository.RecordEventParams) (repository.Event, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, p)
	return repository.Event{}, nil
}

// ---------------------------------------------------------------------------
// 测试用例
// ---------------------------------------------------------------------------

func newSnapHandler(store *stubBypassSnapshotStore, q *stubHostActionQueuer, ev *stubEventRecorderSnap) *AdminBypassSnapshotsHandler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewAdminBypassSnapshotsHandler(logger, store, q, ev)
}

func doRequest(handler nethttp.Handler, method, target string, hostID string, body any) *httptest.ResponseRecorder {
	var rdr io.Reader
	if body != nil {
		buf, _ := json.Marshal(body)
		rdr = bytes.NewReader(buf)
	}
	req := httptest.NewRequest(method, target, rdr)
	req.SetPathValue("hostID", hostID)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestAdminBypassSnapshotsHandler(t *testing.T) {
	t.Run("preview returns 200 with rendered JSON for existing host", func(t *testing.T) {
		store := newStubSnapStore()
		store.rules = []repository.BypassRule{
			{ID: "r1", Scope: "host", HostID: strPtr("h1"), RuleType: "cidr", Value: "10.0.0.0/8"},
		}
		q := &stubHostActionQueuer{}
		ev := &stubEventRecorderSnap{}
		h := newSnapHandler(store, q, ev)

		rec := doRequest(h.Preview(), "POST", "/v1/admin/hosts/h1/bypass/preview", "h1", nil)
		if rec.Code != nethttp.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		var resp previewResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp.ConfigHash == "" {
			t.Error("expected non-empty config_hash")
		}
		if !strings.Contains(string(resp.WhitelistCIDRsRendered), "10.0.0.0/8") {
			t.Errorf("expected rendered cidrs to contain 10.0.0.0/8: %s", resp.WhitelistCIDRsRendered)
		}
	})

	t.Run("preview returns 404 when host not found", func(t *testing.T) {
		store := newStubSnapStore()
		store.hostErr = pgx.ErrNoRows
		h := newSnapHandler(store, &stubHostActionQueuer{}, &stubEventRecorderSnap{})

		rec := doRequest(h.Preview(), "POST", "/v1/admin/hosts/h1/bypass/preview", "h1", nil)
		if rec.Code != nethttp.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), ErrCodeBypassHostNotFound) {
			t.Errorf("expected error code %s in body: %s", ErrCodeBypassHostNotFound, rec.Body.String())
		}
	})

	t.Run("apply first time succeeds, writes audit + dispatches action + records event", func(t *testing.T) {
		store := newStubSnapStore()
		store.rules = []repository.BypassRule{
			{ID: "r1", Scope: "host", HostID: strPtr("h1"), RuleType: "cidr", Value: "10.0.0.0/8"},
		}
		store.latestErr = pgx.ErrNoRows // 没有 prev applied snapshot
		q := &stubHostActionQueuer{returnTID: "task-7"}
		ev := &stubEventRecorderSnap{}
		h := newSnapHandler(store, q, ev)

		rec := doRequest(h.Apply(), "POST", "/v1/admin/hosts/h1/bypass/apply", "h1", map[string]string{"note": "first apply"})
		if rec.Code != nethttp.StatusCreated {
			t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
		}
		var resp applyResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp.AppliedStatus != "pending" {
			t.Errorf("expected applied_status=pending, got %s", resp.AppliedStatus)
		}
		if resp.TaskID != "task-7" {
			t.Errorf("expected task_id=task-7, got %s", resp.TaskID)
		}
		// audit 写入了 1 行
		if len(store.auditLogs) != 1 {
			t.Errorf("expected 1 audit row, got %d", len(store.auditLogs))
		}
		if store.auditLogs[0].Action != "apply" {
			t.Errorf("expected audit action=apply, got %s", store.auditLogs[0].Action)
		}
		// QueueHostAction 被调用 1 次 + Action 正确
		if len(q.calls) != 1 {
			t.Fatalf("expected 1 queue call, got %d", len(q.calls))
		}
		if q.calls[0].Action != agentapi.ActionReloadHostBypass {
			t.Errorf("expected action=ActionReloadHostBypass, got %s", q.calls[0].Action)
		}
		// events.RecordEvent 被调用 1 次 + 类型正确
		if len(ev.events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(ev.events))
		}
		if ev.events[0].Type != "bypass.apply" {
			t.Errorf("expected event type=bypass.apply, got %s", ev.events[0].Type)
		}
	})

	t.Run("apply idempotent on unique violation", func(t *testing.T) {
		store := newStubSnapStore()
		store.rules = []repository.BypassRule{
			{ID: "r1", Scope: "host", HostID: strPtr("h1"), RuleType: "cidr", Value: "10.0.0.0/8"},
		}
		// 第一次 Create 触发 unique 冲突
		store.createErr = newFakeError("duplicate key value violates unique constraint host_bypass_snapshots_host_id_config_hash_key")
		// findSnapshotByConfigHash 走 ListBypassSnapshotsByHost：放入一条匹配 hash 的现有 snapshot
		// 我们事先渲染计算 hash —— 直接复用渲染函数。
		input := BypassRenderInput{HostID: "h1", Rules: store.rules}
		out, _ := RenderBypassConfig(input, nil)
		existing := repository.BypassSnapshot{
			ID:            "snap-existing",
			HostID:        "h1",
			Version:       3,
			ConfigHash:    out.ConfigHash,
			AppliedStatus: "applied",
			Source:        "apply",
		}
		store.snapshots = []repository.BypassSnapshot{existing}

		q := &stubHostActionQueuer{}
		ev := &stubEventRecorderSnap{}
		h := newSnapHandler(store, q, ev)

		rec := doRequest(h.Apply(), "POST", "/v1/admin/hosts/h1/bypass/apply", "h1", nil)
		if rec.Code != nethttp.StatusOK {
			t.Fatalf("expected 200 on idempotent hit, got %d body=%s", rec.Code, rec.Body.String())
		}
		var resp applyResponse
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		if resp.SnapshotID != "snap-existing" {
			t.Errorf("expected snapshot_id=snap-existing, got %s", resp.SnapshotID)
		}
		// CR-06：幂等路径需要补一次 dispatch（worker 占位是幂等的），
		// 这样前端 useTaskPolling 拿到非空 task_id 能正常推进/收敛 UI。
		// 仍然保持不写 audit、不写 events（幂等命中 = 配置未变，无需重复留痕）。
		if resp.TaskID == "" {
			t.Errorf("expected non-empty task_id on idempotent dispatch, got empty")
		}
		if len(store.auditLogs) != 0 {
			t.Errorf("expected 0 audit rows on idempotent path, got %d", len(store.auditLogs))
		}
		if len(q.calls) != 1 {
			t.Errorf("expected 1 queue call (idempotent dispatch), got %d", len(q.calls))
		}
		if len(ev.events) != 0 {
			t.Errorf("expected 0 events on idempotent path, got %d", len(ev.events))
		}
	})

	t.Run("apply config_hash equals preview config_hash for same input", func(t *testing.T) {
		store := newStubSnapStore()
		store.rules = []repository.BypassRule{
			{ID: "r1", Scope: "host", HostID: strPtr("h1"), RuleType: "cidr", Value: "10.0.0.0/8"},
		}
		store.latestErr = pgx.ErrNoRows
		h := newSnapHandler(store, &stubHostActionQueuer{}, &stubEventRecorderSnap{})

		recPrev := doRequest(h.Preview(), "POST", "/v1/admin/hosts/h1/bypass/preview", "h1", nil)
		var prevResp previewResponse
		_ = json.Unmarshal(recPrev.Body.Bytes(), &prevResp)

		recApply := doRequest(h.Apply(), "POST", "/v1/admin/hosts/h1/bypass/apply", "h1", nil)
		var applyResp applyResponse
		_ = json.Unmarshal(recApply.Body.Bytes(), &applyResp)

		if prevResp.ConfigHash == "" || applyResp.ConfigHash == "" {
			t.Fatal("expected non-empty config_hash on both responses")
		}
		if prevResp.ConfigHash != applyResp.ConfigHash {
			t.Errorf("preview/apply hash mismatch: preview=%s apply=%s", prevResp.ConfigHash, applyResp.ConfigHash)
		}
	})

	t.Run("rollback succeeds, target snapshot status untouched, audit note has prefix", func(t *testing.T) {
		store := newStubSnapStore()
		// target snapshot 存在 + host_id 匹配 + applied
		target := repository.BypassSnapshot{
			ID:                   "snap-target",
			HostID:               "h1",
			Version:              5,
			ConfigHash:           "hash-target",
			WhitelistCIDRsJSON:   json.RawMessage(`{"version":3,"rules":[]}`),
			WhitelistDomainsJSON: json.RawMessage(`{"version":3,"rules":[]}`),
			AppliedStatus:        "applied",
			Source:               "apply",
		}
		store.snapshotByID["snap-target"] = target
		// current latest applied 是别的 snapshot
		store.latestApplied = repository.BypassSnapshot{
			ID:            "snap-current",
			HostID:        "h1",
			Version:       9,
			ConfigHash:    "hash-current",
			AppliedStatus: "applied",
		}
		store.snapshots = []repository.BypassSnapshot{store.latestApplied}
		// 默认 createOut 留空 → handler 用 params 回填
		q := &stubHostActionQueuer{returnTID: "task-rb"}
		ev := &stubEventRecorderSnap{}
		h := newSnapHandler(store, q, ev)

		rec := doRequest(h.Rollback(), "POST", "/v1/admin/hosts/h1/bypass/rollback", "h1",
			map[string]string{"target_snapshot_id": "snap-target"})
		if rec.Code != nethttp.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		var resp rollbackResponse
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		if resp.RollbackTargetSnapshotID != "snap-target" {
			t.Errorf("expected rollback_target_snapshot_id=snap-target, got %s", resp.RollbackTargetSnapshotID)
		}
		if resp.AppliedStatus != "pending" {
			t.Errorf("expected new snapshot applied_status=pending, got %s", resp.AppliedStatus)
		}

		// 关键断言 1：UpdateBypassSnapshotStatus 从未被调用（WARN-4：target 状态不变）
		if len(store.updateSnapshotStatusCalls) != 0 {
			t.Errorf("WARN-4 violation: UpdateBypassSnapshotStatus called %d times, expected 0",
				len(store.updateSnapshotStatusCalls))
		}
		// 关键断言 2：CreateBypassSnapshot 调用了一次，source='rollback'
		if len(store.createCalls) != 1 {
			t.Fatalf("expected 1 create call, got %d", len(store.createCalls))
		}
		if store.createCalls[0].Source != "rollback" {
			t.Errorf("expected source=rollback, got %s", store.createCalls[0].Source)
		}
		// 关键断言 3：audit note 含 rollback_target_snapshot_id 前缀
		if len(store.auditLogs) != 1 {
			t.Fatalf("expected 1 audit row, got %d", len(store.auditLogs))
		}
		if !strings.HasPrefix(store.auditLogs[0].Note, "rollback_target_snapshot_id=") {
			t.Errorf("expected audit note to start with 'rollback_target_snapshot_id=', got %q", store.auditLogs[0].Note)
		}
		// 关键断言 4：QueueHostAction payload 是 new snapshot.ID 不是 target.ID
		if len(q.calls) != 1 {
			t.Fatalf("expected 1 queue call, got %d", len(q.calls))
		}
		if q.calls[0].Payload == "snap-target" {
			t.Error("queue payload should be new snapshot id, not target id")
		}
		// 关键断言 5：events.RecordEvent("bypass.rollback")
		if len(ev.events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(ev.events))
		}
		if ev.events[0].Type != "bypass.rollback" {
			t.Errorf("expected event type=bypass.rollback, got %s", ev.events[0].Type)
		}
	})

	t.Run("rollback returns 404 when target not found", func(t *testing.T) {
		store := newStubSnapStore()
		h := newSnapHandler(store, &stubHostActionQueuer{}, &stubEventRecorderSnap{})
		rec := doRequest(h.Rollback(), "POST", "/v1/admin/hosts/h1/bypass/rollback", "h1",
			map[string]string{"target_snapshot_id": "missing"})
		if rec.Code != nethttp.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), ErrCodeBypassSnapshotNotFound) {
			t.Errorf("expected %s in body: %s", ErrCodeBypassSnapshotNotFound, rec.Body.String())
		}
	})

	t.Run("rollback returns 404 when target host_id mismatch (cross-host)", func(t *testing.T) {
		store := newStubSnapStore()
		store.snapshotByID["snap-other"] = repository.BypassSnapshot{
			ID: "snap-other", HostID: "h2", AppliedStatus: "applied",
		}
		h := newSnapHandler(store, &stubHostActionQueuer{}, &stubEventRecorderSnap{})
		rec := doRequest(h.Rollback(), "POST", "/v1/admin/hosts/h1/bypass/rollback", "h1",
			map[string]string{"target_snapshot_id": "snap-other"})
		if rec.Code != nethttp.StatusNotFound {
			t.Fatalf("expected 404 (cross-host masked as not found), got %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("rollback returns 409 when target not applied", func(t *testing.T) {
		store := newStubSnapStore()
		store.snapshotByID["snap-pending"] = repository.BypassSnapshot{
			ID: "snap-pending", HostID: "h1", AppliedStatus: "pending",
		}
		h := newSnapHandler(store, &stubHostActionQueuer{}, &stubEventRecorderSnap{})
		rec := doRequest(h.Rollback(), "POST", "/v1/admin/hosts/h1/bypass/rollback", "h1",
			map[string]string{"target_snapshot_id": "snap-pending"})
		if rec.Code != nethttp.StatusConflict {
			t.Fatalf("expected 409, got %d body=%s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), ErrCodeBypassSnapshotConflict) {
			t.Errorf("expected %s in body: %s", ErrCodeBypassSnapshotConflict, rec.Body.String())
		}
	})

	t.Run("rollback idempotent when target == current latest applied", func(t *testing.T) {
		store := newStubSnapStore()
		store.snapshotByID["snap-x"] = repository.BypassSnapshot{
			ID: "snap-x", HostID: "h1", Version: 7, AppliedStatus: "applied",
		}
		store.latestApplied = store.snapshotByID["snap-x"]
		q := &stubHostActionQueuer{}
		ev := &stubEventRecorderSnap{}
		h := newSnapHandler(store, q, ev)

		rec := doRequest(h.Rollback(), "POST", "/v1/admin/hosts/h1/bypass/rollback", "h1",
			map[string]string{"target_snapshot_id": "snap-x"})
		if rec.Code != nethttp.StatusOK {
			t.Fatalf("expected 200 on idempotent rollback, got %d", rec.Code)
		}
		if len(store.createCalls) != 0 {
			t.Errorf("expected 0 create calls on idempotent rollback, got %d", len(store.createCalls))
		}
		if len(store.auditLogs) != 0 {
			t.Errorf("expected 0 audit rows on idempotent rollback, got %d", len(store.auditLogs))
		}
		if len(q.calls) != 0 {
			t.Errorf("expected 0 queue calls on idempotent rollback, got %d", len(q.calls))
		}
	})

	t.Run("effective returns 4 sections with cidrs/domains rendered", func(t *testing.T) {
		store := newStubSnapStore()
		// 1 preset binding + 1 rule binding
		preset := repository.BypassPreset{
			ID: "p-loop", Slug: "loopback", IsActive: true, IsForceOn: true,
			Rules: []repository.BypassPresetRule{{RuleType: "cidr", Value: "127.0.0.0/8"}},
		}
		store.presetByID["p-loop"] = preset
		globalRule := repository.BypassRule{
			ID: "rg-1", Scope: "global", RuleType: "domain_suffix", Value: "corp.internal",
		}
		store.rules = []repository.BypassRule{globalRule}
		store.bindings = []repository.BypassBinding{
			{ID: "b1", HostID: "h1", PresetID: strPtr("p-loop"), Enabled: true, Source: "admin"},
			{ID: "b2", HostID: "h1", RuleID: strPtr("rg-1"), Enabled: true, Source: "admin"},
		}
		h := newSnapHandler(store, &stubHostActionQueuer{}, &stubEventRecorderSnap{})

		rec := doRequest(h.Effective(), "GET", "/v1/admin/hosts/h1/bypass/effective", "h1", nil)
		if rec.Code != nethttp.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		var resp effectiveResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(resp.PresetsActive) != 1 {
			t.Errorf("expected 1 active preset, got %d", len(resp.PresetsActive))
		}
		if len(resp.RulesActive) != 1 {
			t.Errorf("expected 1 active rule, got %d", len(resp.RulesActive))
		}
		if !strings.Contains(string(resp.WhitelistCIDRsRendered), "127.0.0.0/8") {
			t.Errorf("expected cidrs to contain 127.0.0.0/8: %s", resp.WhitelistCIDRsRendered)
		}
		if !strings.Contains(string(resp.WhitelistDomainsRendered), "corp.internal") {
			t.Errorf("expected domains to contain corp.internal: %s", resp.WhitelistDomainsRendered)
		}
	})

	t.Run("effective returns 404 when host not found", func(t *testing.T) {
		store := newStubSnapStore()
		store.hostErr = pgx.ErrNoRows
		h := newSnapHandler(store, &stubHostActionQueuer{}, &stubEventRecorderSnap{})
		rec := doRequest(h.Effective(), "GET", "/v1/admin/hosts/h1/bypass/effective", "h1", nil)
		if rec.Code != nethttp.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})
}

// strPtr 是测试辅助：返回字符串的指针，便于在 stub 数据里写 *string 字段。
func strPtr(s string) *string { return &s }

// fakeError 在 stub 内模拟 SQL 错误信息。
type fakeError struct{ msg string }

func (e *fakeError) Error() string { return e.msg }
func newFakeError(s string) error  { return &fakeError{msg: s} }

// ---------------------------------------------------------------------------
// Phase 47 Plan 01 Task 4：GET /v1/admin/hosts/{hostID}/bypass/consistency
// ---------------------------------------------------------------------------

// withFakeConsistencyHook 替换包级 verifyConsistencyHook，t.Cleanup 自动还原。
// 直接 import internal/network 包：生产代码已经 import 它，不存在循环依赖。
func withFakeConsistencyHook(t *testing.T, fn func(ctx context.Context, hostID string) (network.ConsistencyResult, error)) {
	t.Helper()
	prev := verifyConsistencyHook
	verifyConsistencyHook = fn
	t.Cleanup(func() { verifyConsistencyHook = prev })
}

// TestConsistency_OK 守护 acceptance Test 1：mock VerifyBypassConsistency 返回 OK=true
// → 200 + JSON {ok:true, ruleset_sha256, nft_set_sha256}。
func TestConsistency_OK(t *testing.T) {
	store := newStubSnapStore()
	store.host = repository.Host{ID: "h1"}
	h := newSnapHandler(store, &stubHostActionQueuer{}, &stubEventRecorderSnap{})

	withFakeConsistencyHook(t, func(_ context.Context, _ string) (network.ConsistencyResult, error) {
		return network.ConsistencyResult{OK: true, RuleSetSHA256: "abc", NftSetSHA256: "abc"}, nil
	})

	rec := doRequest(h.Consistency(), "GET", "/v1/admin/hosts/h1/bypass/consistency", "h1", nil)
	if rec.Code != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		OK            bool   `json:"ok"`
		RuleSetSHA256 string `json:"ruleset_sha256"`
		NftSetSHA256  string `json:"nft_set_sha256"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !body.OK {
		t.Errorf("expected ok=true, got %+v", body)
	}
	if body.RuleSetSHA256 != "abc" || body.NftSetSHA256 != "abc" {
		t.Errorf("expected hash=abc/abc, got %+v", body)
	}
}

// TestConsistency_Drift 守护 acceptance Test 2：OK=false → 409 + 两 hash 字面值 + detail。
func TestConsistency_Drift(t *testing.T) {
	store := newStubSnapStore()
	store.host = repository.Host{ID: "h1"}
	h := newSnapHandler(store, &stubHostActionQueuer{}, &stubEventRecorderSnap{})

	withFakeConsistencyHook(t, func(_ context.Context, _ string) (network.ConsistencyResult, error) {
		return network.ConsistencyResult{
			OK:            false,
			RuleSetSHA256: "rs-aaa",
			NftSetSHA256:  "nft-bbb",
			Detail:        "cidr set mismatch: file=2 entries, nft=1 entries",
		}, nil
	})

	rec := doRequest(h.Consistency(), "GET", "/v1/admin/hosts/h1/bypass/consistency", "h1", nil)
	if rec.Code != nethttp.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", rec.Code, rec.Body.String())
	}
	bodyStr := rec.Body.String()
	for _, want := range []string{"rs-aaa", "nft-bbb", "cidr set mismatch"} {
		if !strings.Contains(bodyStr, want) {
			t.Errorf("expected drift body to contain %q, got %s", want, bodyStr)
		}
	}
}

// TestConsistency_AdminOnly 守护 acceptance Test 3：路由层 adminGuard 控制访问；
// 这里直接对 handler 做 doRequest 验证 handler 自身不重复鉴权也不泄露内部信息 —
// host 不存在时仍稳定返回 404 BYPASS_HOST_NOT_FOUND，确认 handler 落在路由 guard 之后。
//
// 真正的 403 路径在 router_test 里覆盖 adminGuard（本任务对 admin endpoint 不引入
// 新的鉴权逻辑，复用已有 adminGuard）。
func TestConsistency_AdminOnly(t *testing.T) {
	store := newStubSnapStore()
	store.hostErr = pgx.ErrNoRows
	h := newSnapHandler(store, &stubHostActionQueuer{}, &stubEventRecorderSnap{})

	called := false
	withFakeConsistencyHook(t, func(_ context.Context, _ string) (network.ConsistencyResult, error) {
		called = true
		return network.ConsistencyResult{OK: true}, nil
	})

	rec := doRequest(h.Consistency(), "GET", "/v1/admin/hosts/missing/bypass/consistency", "missing", nil)
	if rec.Code != nethttp.StatusNotFound {
		t.Fatalf("expected 404 BYPASS_HOST_NOT_FOUND, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "BYPASS_HOST_NOT_FOUND") {
		t.Errorf("expected error_code=BYPASS_HOST_NOT_FOUND, got body=%s", rec.Body.String())
	}
	if called {
		t.Errorf("verifyConsistencyHook must not be called when host lookup 404s")
	}
}

// TestConsistency_Timeout 守护 acceptance Test 4：mock 返回 context.DeadlineExceeded
// → 504 + JSON {error_code:"BYPASS_CONSISTENCY_TIMEOUT"}。
func TestConsistency_Timeout(t *testing.T) {
	store := newStubSnapStore()
	store.host = repository.Host{ID: "h1"}
	h := newSnapHandler(store, &stubHostActionQueuer{}, &stubEventRecorderSnap{})

	withFakeConsistencyHook(t, func(_ context.Context, _ string) (network.ConsistencyResult, error) {
		return network.ConsistencyResult{}, context.DeadlineExceeded
	})

	rec := doRequest(h.Consistency(), "GET", "/v1/admin/hosts/h1/bypass/consistency", "h1", nil)
	if rec.Code != nethttp.StatusGatewayTimeout {
		t.Fatalf("expected 504, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "BYPASS_CONSISTENCY_TIMEOUT") {
		t.Errorf("expected BYPASS_CONSISTENCY_TIMEOUT error code, got body=%s", rec.Body.String())
	}
}
