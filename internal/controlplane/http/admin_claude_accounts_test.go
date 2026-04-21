package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/zanel1u/cloud-cli-proxy/internal/agentapi"
)

// stubTx 实现 pgx.Tx 最小接口（仅 handler 实际使用的 4 方法）；其余 panic 以提示设计偏差。
type stubTx struct {
	scanResults  []any
	queryRowErr  error
	execAffected int64
	execErr      error
	committed    bool
	rolledback   bool
}

type stubRow struct {
	results []any
	err     error
}

func (r *stubRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	for i, d := range dest {
		if i >= len(r.results) {
			return errors.New("stubRow: not enough results")
		}
		switch dd := d.(type) {
		case *string:
			*dd = r.results[i].(string)
		default:
			return errors.New("stubRow: unsupported dest type")
		}
	}
	return nil
}

func (s *stubTx) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	return &stubRow{results: s.scanResults, err: s.queryRowErr}
}
func (s *stubTx) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	if s.execErr != nil {
		return pgconn.CommandTag{}, s.execErr
	}
	return pgconn.NewCommandTag("DELETE " + itoa(s.execAffected)), nil
}
func (s *stubTx) Commit(_ context.Context) error   { s.committed = true; return nil }
func (s *stubTx) Rollback(_ context.Context) error { s.rolledback = true; return nil }

func (s *stubTx) Begin(_ context.Context) (pgx.Tx, error) { panic("stubTx.Begin not implemented") }
func (s *stubTx) CopyFrom(_ context.Context, _ pgx.Identifier, _ []string, _ pgx.CopyFromSource) (int64, error) {
	panic("stubTx.CopyFrom not implemented")
}
func (s *stubTx) SendBatch(_ context.Context, _ *pgx.Batch) pgx.BatchResults {
	panic("stubTx.SendBatch not implemented")
}
func (s *stubTx) LargeObjects() pgx.LargeObjects { panic("stubTx.LargeObjects not implemented") }
func (s *stubTx) Prepare(_ context.Context, _, _ string) (*pgconn.StatementDescription, error) {
	panic("stubTx.Prepare not implemented")
}
func (s *stubTx) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	panic("stubTx.Query not implemented")
}
func (s *stubTx) Conn() *pgx.Conn { panic("stubTx.Conn not implemented") }

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

type stubAdminClaudeAccountStore struct {
	tx       *stubTx
	beginErr error
	beginCnt int
	beginCtx context.Context
}

func (s *stubAdminClaudeAccountStore) BeginTx(ctx context.Context) (pgx.Tx, error) {
	s.beginCnt++
	s.beginCtx = ctx
	if s.beginErr != nil {
		return nil, s.beginErr
	}
	return s.tx, nil
}

func newAdminClaudeAccountsTestRouter(t *testing.T, store AdminClaudeAccountStore, events EventRecorder) (nethttp.Handler, func()) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewAdminClaudeAccountsHandler(logger, store, nil, events)
	mux := nethttp.NewServeMux()
	mux.Handle("DELETE /v1/admin/claude-accounts/{accountID}", handler.Delete())
	return mux, func() {}
}

func TestAdminClaudeAccountsDelete_StrictSuccess_DBDeletedAndAuditEventEmitted(t *testing.T) {
	tx := &stubTx{
		scanResults:  []any{"acct-1", "claude-state-acct-1"},
		execAffected: 1,
	}
	store := &stubAdminClaudeAccountStore{tx: tx}
	events := &stubEventRecorder{}

	origRun := runHostAction
	runHostAction = func(ctx context.Context, client *agentapi.Client, req agentapi.HostActionRequest) (agentapi.HostActionResponse, error) {
		if req.Action != agentapi.ActionVolumeRemove {
			t.Fatalf("expected ActionVolumeRemove, got %q", req.Action)
		}
		if len(req.Volumes) != 1 || req.Volumes[0].Name != "claude-state-acct-1" {
			t.Fatalf("expected single volume claude-state-acct-1, got %+v", req.Volumes)
		}
		return agentapi.HostActionResponse{}, nil
	}
	t.Cleanup(func() { runHostAction = origRun })

	mux, _ := newAdminClaudeAccountsTestRouter(t, store, events)
	req := httptest.NewRequest(nethttp.MethodDelete, "/v1/admin/claude-accounts/acct-1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != nethttp.StatusOK {
		t.Errorf("want 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !tx.committed {
		t.Error("tx must be committed on success")
	}
	if !events.hasType("claude_account.deleted") {
		t.Error("audit event claude_account.deleted must be emitted")
	}
}

func TestAdminClaudeAccountsDelete_StrictHostAgentFailure_RollbackAnd409WithChineseMessage(t *testing.T) {
	tx := &stubTx{scanResults: []any{"acct-1", "claude-state-acct-1"}}
	store := &stubAdminClaudeAccountStore{tx: tx}
	events := &stubEventRecorder{}

	origRun := runHostAction
	runHostAction = func(ctx context.Context, client *agentapi.Client, req agentapi.HostActionRequest) (agentapi.HostActionResponse, error) {
		return agentapi.HostActionResponse{}, errors.New("volume_in_use: stuck on container_xyz")
	}
	t.Cleanup(func() { runHostAction = origRun })

	mux, _ := newAdminClaudeAccountsTestRouter(t, store, events)
	req := httptest.NewRequest(nethttp.MethodDelete, "/v1/admin/claude-accounts/acct-1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != nethttp.StatusConflict {
		t.Fatalf("want 409, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !tx.rolledback {
		t.Error("tx must be rolled back on host-agent failure")
	}
	if !events.hasType("claude_account.delete_volume_rm_failed") {
		t.Error("audit event claude_account.delete_volume_rm_failed must be emitted")
	}

	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("body must be JSON: %v", err)
	}
	errObj, _ := body["error"].(map[string]any)
	if errObj["code"] != "STATE_VOLUME_IN_USE_001" {
		t.Errorf("error.code must be STATE_VOLUME_IN_USE_001, got %v", errObj["code"])
	}
	msg, _ := errObj["message"].(string)
	if !strings.Contains(msg, "请先停止使用该账号的所有 host 后重试") {
		t.Errorf("error.message must contain Chinese guidance, got %q", msg)
	}
}

func TestAdminClaudeAccountsDelete_ForceTrue_DBDeletedEvenWhenRmFails(t *testing.T) {
	tx := &stubTx{scanResults: []any{"acct-1", "claude-state-acct-1"}, execAffected: 1}
	store := &stubAdminClaudeAccountStore{tx: tx}
	events := &stubEventRecorder{}

	origRun := runHostAction
	var capturedLabels map[string]string
	runHostAction = func(ctx context.Context, client *agentapi.Client, req agentapi.HostActionRequest) (agentapi.HostActionResponse, error) {
		capturedLabels = req.Labels
		return agentapi.HostActionResponse{}, errors.New("daemon connection refused")
	}
	t.Cleanup(func() { runHostAction = origRun })

	mux, _ := newAdminClaudeAccountsTestRouter(t, store, events)
	req := httptest.NewRequest(nethttp.MethodDelete, "/v1/admin/claude-accounts/acct-1?force=true", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != nethttp.StatusOK {
		t.Fatalf("force=true must return 200 even when rm fails, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !tx.committed {
		t.Error("tx must be committed (DB delete first) in force path")
	}
	if capturedLabels["force"] != "true" {
		t.Errorf("force label must be propagated to host-agent, got %v", capturedLabels)
	}
	if !events.hasType("claude_account.force_volume_rm_failed") {
		t.Error("audit event claude_account.force_volume_rm_failed must be emitted")
	}

	var body map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &body)
	if body["volume_rm"] != "failed" {
		t.Errorf("volume_rm must be \"failed\", got %v", body["volume_rm"])
	}
	if na, _ := body["next_action"].(string); !strings.Contains(na, "docker volume rm -f") {
		t.Errorf("next_action must hint docker volume rm -f, got %q", na)
	}
}

func TestAdminClaudeAccountsDelete_AccountNotFound_404(t *testing.T) {
	tx := &stubTx{queryRowErr: pgx.ErrNoRows}
	store := &stubAdminClaudeAccountStore{tx: tx}
	mux, _ := newAdminClaudeAccountsTestRouter(t, store, &stubEventRecorder{})
	req := httptest.NewRequest(nethttp.MethodDelete, "/v1/admin/claude-accounts/missing", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != nethttp.StatusNotFound {
		t.Errorf("want 404, got %d", rr.Code)
	}
}

func TestAdminClaudeAccountsDelete_NoVolumeName_SkipsHostAgentCall(t *testing.T) {
	tx := &stubTx{scanResults: []any{"acct-1", ""}, execAffected: 1}
	store := &stubAdminClaudeAccountStore{tx: tx}
	events := &stubEventRecorder{}
	called := false
	origRun := runHostAction
	runHostAction = func(ctx context.Context, client *agentapi.Client, req agentapi.HostActionRequest) (agentapi.HostActionResponse, error) {
		called = true
		return agentapi.HostActionResponse{}, nil
	}
	t.Cleanup(func() { runHostAction = origRun })

	mux, _ := newAdminClaudeAccountsTestRouter(t, store, events)
	req := httptest.NewRequest(nethttp.MethodDelete, "/v1/admin/claude-accounts/acct-1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != nethttp.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
	if called {
		t.Error("host-agent must NOT be called when volume_name is empty")
	}
}

func TestAdminClaudeAccountsDelete_StrictUsesTenSecondTimeout(t *testing.T) {
	tx := &stubTx{scanResults: []any{"acct-1", "claude-state-acct-1"}, execAffected: 1}
	store := &stubAdminClaudeAccountStore{tx: tx}
	origRun := runHostAction
	runHostAction = func(ctx context.Context, client *agentapi.Client, req agentapi.HostActionRequest) (agentapi.HostActionResponse, error) {
		return agentapi.HostActionResponse{}, nil
	}
	t.Cleanup(func() { runHostAction = origRun })

	mux, _ := newAdminClaudeAccountsTestRouter(t, store, &stubEventRecorder{})
	req := httptest.NewRequest(nethttp.MethodDelete, "/v1/admin/claude-accounts/acct-1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if store.beginCtx == nil {
		t.Fatal("BeginTx must be called")
	}
	deadline, ok := store.beginCtx.Deadline()
	if !ok {
		t.Fatal("strict path must have deadline")
	}
	remaining := time.Until(deadline)
	if remaining > 10*time.Second+100*time.Millisecond || remaining < 9*time.Second {
		t.Errorf("strict timeout must be ~10s, got %v", remaining)
	}
}

func TestAdminClaudeAccountsDelete_ForceUsesThirtySecondTimeout(t *testing.T) {
	tx := &stubTx{scanResults: []any{"acct-1", "claude-state-acct-1"}, execAffected: 1}
	store := &stubAdminClaudeAccountStore{tx: tx}
	origRun := runHostAction
	runHostAction = func(ctx context.Context, client *agentapi.Client, req agentapi.HostActionRequest) (agentapi.HostActionResponse, error) {
		return agentapi.HostActionResponse{}, nil
	}
	t.Cleanup(func() { runHostAction = origRun })

	mux, _ := newAdminClaudeAccountsTestRouter(t, store, &stubEventRecorder{})
	req := httptest.NewRequest(nethttp.MethodDelete, "/v1/admin/claude-accounts/acct-1?force=true", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	deadline, ok := store.beginCtx.Deadline()
	if !ok {
		t.Fatal("force path must have deadline")
	}
	remaining := time.Until(deadline)
	if remaining > 30*time.Second+100*time.Millisecond || remaining < 29*time.Second {
		t.Errorf("force timeout must be ~30s, got %v", remaining)
	}
}

func TestParseForceFlag_AcceptsTrueOneYes(t *testing.T) {
	cases := map[string]bool{"true": true, "1": true, "yes": true, "false": false, "": false, "TRUE": false}
	for s, want := range cases {
		if got := parseForceFlag(s); got != want {
			t.Errorf("parseForceFlag(%q): got %v, want %v", s, got, want)
		}
	}
}
