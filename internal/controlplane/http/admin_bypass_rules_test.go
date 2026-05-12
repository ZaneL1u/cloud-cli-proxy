package http

import (
	"context"
	"encoding/json"
	"log/slog"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

type stubBypassRuleStore struct {
	mu sync.Mutex

	// list 返回：根据 host_id 入参分流
	globalRules []repository.BypassRule
	hostRules   []repository.BypassRule
	listErr     error

	getByIDRule repository.BypassRule
	getByIDErr  error
	getCalls    int

	createOut   repository.BypassRule
	createErr   error
	createCount int
	createSeen  []repository.CreateBypassRuleParams

	updateOut   repository.BypassRule
	updateErr   error
	updateCount int

	deleteErr   error
	deleteCount int

	auditLogs []repository.InsertBypassAuditLogParams
}

func (s *stubBypassRuleStore) ListBypassRules(_ context.Context, hostID *string) ([]repository.BypassRule, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	if hostID == nil {
		return s.globalRules, nil
	}
	return s.hostRules, nil
}

func (s *stubBypassRuleStore) GetBypassRuleByID(_ context.Context, _ string) (repository.BypassRule, error) {
	s.mu.Lock()
	s.getCalls++
	s.mu.Unlock()
	if s.getByIDErr != nil {
		return repository.BypassRule{}, s.getByIDErr
	}
	return s.getByIDRule, nil
}

func (s *stubBypassRuleStore) CreateBypassRule(_ context.Context, p repository.CreateBypassRuleParams) (repository.BypassRule, error) {
	s.mu.Lock()
	s.createCount++
	s.createSeen = append(s.createSeen, p)
	s.mu.Unlock()
	if s.createErr != nil {
		return repository.BypassRule{}, s.createErr
	}
	out := s.createOut
	if out.IsRisky != p.IsRisky {
		out.IsRisky = p.IsRisky
	}
	return out, nil
}

func (s *stubBypassRuleStore) UpdateBypassRule(_ context.Context, _ string, _ repository.UpdateBypassRuleParams) (repository.BypassRule, error) {
	s.mu.Lock()
	s.updateCount++
	s.mu.Unlock()
	if s.updateErr != nil {
		return repository.BypassRule{}, s.updateErr
	}
	return s.updateOut, nil
}

func (s *stubBypassRuleStore) DeleteBypassRule(_ context.Context, _ string) error {
	s.mu.Lock()
	s.deleteCount++
	s.mu.Unlock()
	return s.deleteErr
}

func (s *stubBypassRuleStore) InsertBypassAuditLog(_ context.Context, p repository.InsertBypassAuditLogParams) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.auditLogs = append(s.auditLogs, p)
	return "audit-x", nil
}

// stubBypassProxyProvider 用最小实现满足 AdminBypassProxyIPProvider。
type stubBypassProxyProvider struct {
	ips []repository.EgressIP
	err error
}

func (s *stubBypassProxyProvider) ListEgressIPs(_ context.Context) ([]repository.EgressIP, error) {
	return s.ips, s.err
}

func newRuleTestRequest(t *testing.T, method, target, userID string, body any) *nethttp.Request {
	t.Helper()
	return newPresetTestRequest(t, method, target, userID, body)
}

func newRuleHandler(store *stubBypassRuleStore, proxy AdminBypassProxyIPProvider, events *stubEventRecorder) *AdminBypassRulesHandler {
	return NewAdminBypassRulesHandler(slog.Default(), store, proxy, events)
}

func TestAdminBypassRulesHandler(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	hostID := "host-1"

	globalRule := repository.BypassRule{
		ID: "rule-g1", Scope: "global", RuleType: "domain_suffix", Value: "corp.internal",
		CreatedAt: now, UpdatedAt: now,
	}
	hostRule := repository.BypassRule{
		ID: "rule-h1", Scope: "host", HostID: &hostID, RuleType: "ip", Value: "192.0.2.10",
		CreatedAt: now, UpdatedAt: now,
	}

	t.Run("List global only when no host_id", func(t *testing.T) {
		store := &stubBypassRuleStore{globalRules: []repository.BypassRule{globalRule}}
		h := newRuleHandler(store, &stubBypassProxyProvider{}, &stubEventRecorder{})
		r := newRuleTestRequest(t, "GET", "/v1/admin/bypass/rules", "admin", nil)
		w := httptest.NewRecorder()
		h.List().ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("status = %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "rule-g1") {
			t.Errorf("expect global rule in body: %s", w.Body.String())
		}
	})

	t.Run("List with host_id returns host+global", func(t *testing.T) {
		store := &stubBypassRuleStore{hostRules: []repository.BypassRule{globalRule, hostRule}}
		h := newRuleHandler(store, &stubBypassProxyProvider{}, &stubEventRecorder{})
		r := newRuleTestRequest(t, "GET", "/v1/admin/bypass/rules?host_id="+hostID, "admin", nil)
		w := httptest.NewRecorder()
		h.List().ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("status = %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "rule-h1") {
			t.Errorf("expect host rule in body")
		}
	})

	t.Run("Create global rule 201 + audit + event", func(t *testing.T) {
		store := &stubBypassRuleStore{createOut: globalRule}
		events := &stubEventRecorder{}
		h := newRuleHandler(store, &stubBypassProxyProvider{}, events)

		body := map[string]any{"scope": "global", "rule_type": "domain_suffix", "value": "corp.internal"}
		r := newRuleTestRequest(t, "POST", "/v1/admin/bypass/rules", "admin", body)
		w := httptest.NewRecorder()
		h.Create().ServeHTTP(w, r)

		if w.Code != 201 {
			t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
		}
		if store.createCount != 1 {
			t.Errorf("expect 1 create call, got %d", store.createCount)
		}
		if len(store.auditLogs) != 1 || store.auditLogs[0].Action != "create_rule" {
			t.Errorf("audit log not written: %+v", store.auditLogs)
		}
		if !events.hasType("bypass.create_rule") {
			t.Errorf("event bypass.create_rule not published")
		}
	})

	t.Run("Create host rule with port range 201", func(t *testing.T) {
		store := &stubBypassRuleStore{createOut: hostRule}
		h := newRuleHandler(store, &stubBypassProxyProvider{}, &stubEventRecorder{})

		body := map[string]any{
			"scope": "host", "host_id": hostID,
			"rule_type": "ip", "value": "192.0.2.10",
			"port": "80-443",
		}
		r := newRuleTestRequest(t, "POST", "/v1/admin/bypass/rules", "admin", body)
		w := httptest.NewRecorder()
		h.Create().ServeHTTP(w, r)

		if w.Code != 201 {
			t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
		}
	})

	t.Run("Create CIDR 0.0.0.0/0 → 422 TOO_BROAD", func(t *testing.T) {
		store := &stubBypassRuleStore{}
		h := newRuleHandler(store, &stubBypassProxyProvider{}, &stubEventRecorder{})

		body := map[string]any{"scope": "global", "rule_type": "cidr", "value": "0.0.0.0/0"}
		r := newRuleTestRequest(t, "POST", "/v1/admin/bypass/rules", "admin", body)
		w := httptest.NewRecorder()
		h.Create().ServeHTTP(w, r)

		if w.Code != 422 {
			t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), ErrCodeBypassRuleTooBroad) {
			t.Errorf("expect TOO_BROAD: %s", w.Body.String())
		}
		if store.createCount != 0 {
			t.Errorf("CreateBypassRule should NOT be called on guard failure")
		}
	})

	t.Run("Create domain .com → 422 TOO_BROAD", func(t *testing.T) {
		store := &stubBypassRuleStore{}
		h := newRuleHandler(store, &stubBypassProxyProvider{}, &stubEventRecorder{})

		body := map[string]any{"scope": "global", "rule_type": "domain", "value": ".com"}
		r := newRuleTestRequest(t, "POST", "/v1/admin/bypass/rules", "admin", body)
		w := httptest.NewRecorder()
		h.Create().ServeHTTP(w, r)

		if w.Code != 422 || !strings.Contains(w.Body.String(), ErrCodeBypassRuleTooBroad) {
			t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
		}
	})

	t.Run("Create domain_keyword abc no confirm → 400 KEYWORD_TOO_SHORT", func(t *testing.T) {
		store := &stubBypassRuleStore{}
		h := newRuleHandler(store, &stubBypassProxyProvider{}, &stubEventRecorder{})

		body := map[string]any{"scope": "global", "rule_type": "domain_keyword", "value": "abc"}
		r := newRuleTestRequest(t, "POST", "/v1/admin/bypass/rules", "admin", body)
		w := httptest.NewRecorder()
		h.Create().ServeHTTP(w, r)

		if w.Code != 400 || !strings.Contains(w.Body.String(), ErrCodeBypassKeywordTooShort) {
			t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
		}
	})

	t.Run("Create domain_keyword abc + confirm_risky → 201 + IsRisky=true + note", func(t *testing.T) {
		store := &stubBypassRuleStore{createOut: repository.BypassRule{
			ID: "rule-r1", Scope: "global", RuleType: "domain_keyword", Value: "abc",
			IsRisky: true, CreatedAt: now, UpdatedAt: now,
		}}
		events := &stubEventRecorder{}
		h := newRuleHandler(store, &stubBypassProxyProvider{}, events)

		body := map[string]any{
			"scope":         "global",
			"rule_type":     "domain_keyword",
			"value":         "abc",
			"confirm_risky": true,
		}
		r := newRuleTestRequest(t, "POST", "/v1/admin/bypass/rules", "admin", body)
		w := httptest.NewRecorder()
		h.Create().ServeHTTP(w, r)

		if w.Code != 201 {
			t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
		}
		if len(store.createSeen) != 1 || !store.createSeen[0].IsRisky {
			t.Errorf("expect IsRisky=true on CreateBypassRule params, got %+v", store.createSeen)
		}
		if len(store.auditLogs) != 1 || store.auditLogs[0].Note != "confirm_risky_accepted" {
			t.Errorf("expect audit note=confirm_risky_accepted, got %+v", store.auditLogs)
		}
		if !events.hasType("bypass.create_rule") {
			t.Errorf("event bypass.create_rule not published")
		}
	})

	t.Run("Create cidr covers proxy IP → 422 CONFLICT_PROXY", func(t *testing.T) {
		store := &stubBypassRuleStore{}
		proxy := &stubBypassProxyProvider{ips: []repository.EgressIP{{IPAddress: "203.0.113.10"}}}
		h := newRuleHandler(store, proxy, &stubEventRecorder{})

		body := map[string]any{"scope": "global", "rule_type": "cidr", "value": "203.0.113.0/24"}
		r := newRuleTestRequest(t, "POST", "/v1/admin/bypass/rules", "admin", body)
		w := httptest.NewRecorder()
		h.Create().ServeHTTP(w, r)

		if w.Code != 422 || !strings.Contains(w.Body.String(), ErrCodeBypassRuleConflictProxy) {
			t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
		}
	})

	t.Run("Create host rule when count ≥ 1000 → 422 LIMIT_EXCEEDED", func(t *testing.T) {
		// 构造 1000 条 host scope 规则
		bulk := make([]repository.BypassRule, 1000)
		for i := range bulk {
			bulk[i] = repository.BypassRule{Scope: "host", HostID: &hostID}
		}
		store := &stubBypassRuleStore{hostRules: bulk}
		h := newRuleHandler(store, &stubBypassProxyProvider{}, &stubEventRecorder{})

		body := map[string]any{
			"scope": "host", "host_id": hostID,
			"rule_type": "ip", "value": "192.0.2.20",
		}
		r := newRuleTestRequest(t, "POST", "/v1/admin/bypass/rules", "admin", body)
		w := httptest.NewRecorder()
		h.Create().ServeHTTP(w, r)

		if w.Code != 422 || !strings.Contains(w.Body.String(), ErrCodeBypassLimitExceeded) {
			t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
		}
	})

	t.Run("Update value triggers guard → 422", func(t *testing.T) {
		store := &stubBypassRuleStore{
			getByIDRule: globalRule,
			updateOut:   globalRule,
		}
		h := newRuleHandler(store, &stubBypassProxyProvider{}, &stubEventRecorder{})

		body := map[string]any{"value": ".com"} // domain_suffix .com 命中 TLD
		r := newRuleTestRequest(t, "PATCH", "/v1/admin/bypass/rules/rule-g1", "admin", body)
		r.SetPathValue("ruleID", "rule-g1")
		w := httptest.NewRecorder()
		h.Update().ServeHTTP(w, r)

		if w.Code != 422 {
			t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
		}
		if store.updateCount != 0 {
			t.Errorf("UpdateBypassRule should NOT be called on guard failure")
		}
	})

	t.Run("Update success records before via GetBypassRuleByID", func(t *testing.T) {
		updated := globalRule
		updated.Value = "corp2.internal"
		store := &stubBypassRuleStore{
			getByIDRule: globalRule,
			updateOut:   updated,
		}
		h := newRuleHandler(store, &stubBypassProxyProvider{}, &stubEventRecorder{})

		body := map[string]any{"value": "corp2.internal"}
		r := newRuleTestRequest(t, "PATCH", "/v1/admin/bypass/rules/rule-g1", "admin", body)
		r.SetPathValue("ruleID", "rule-g1")
		w := httptest.NewRecorder()
		h.Update().ServeHTTP(w, r)

		if w.Code != 200 {
			t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
		}
		if store.getCalls != 1 {
			t.Errorf("GetBypassRuleByID should be called once before update, got %d", store.getCalls)
		}
		if len(store.auditLogs) != 1 || store.auditLogs[0].Before == nil {
			t.Errorf("audit log before must NOT be nil after WARN-5 fix: %+v", store.auditLogs)
		}
		var beforeDoc map[string]any
		if err := json.Unmarshal(store.auditLogs[0].Before, &beforeDoc); err != nil {
			t.Fatalf("unmarshal before: %v", err)
		}
		if beforeDoc["value"] != "corp.internal" {
			t.Errorf("before.value = %v, want corp.internal", beforeDoc["value"])
		}
	})

	t.Run("Delete missing → 404", func(t *testing.T) {
		store := &stubBypassRuleStore{getByIDErr: pgx.ErrNoRows}
		h := newRuleHandler(store, &stubBypassProxyProvider{}, &stubEventRecorder{})

		r := newRuleTestRequest(t, "DELETE", "/v1/admin/bypass/rules/missing", "admin", nil)
		r.SetPathValue("ruleID", "missing")
		w := httptest.NewRecorder()
		h.Delete().ServeHTTP(w, r)

		if w.Code != 404 || !strings.Contains(w.Body.String(), ErrCodeBypassRuleNotFound) {
			t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
		}
	})

	t.Run("Delete success records before non-nil", func(t *testing.T) {
		store := &stubBypassRuleStore{getByIDRule: globalRule}
		events := &stubEventRecorder{}
		h := newRuleHandler(store, &stubBypassProxyProvider{}, events)

		r := newRuleTestRequest(t, "DELETE", "/v1/admin/bypass/rules/rule-g1", "admin", nil)
		r.SetPathValue("ruleID", "rule-g1")
		w := httptest.NewRecorder()
		h.Delete().ServeHTTP(w, r)

		if w.Code != 204 {
			t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
		}
		if store.deleteCount != 1 {
			t.Errorf("delete count = %d", store.deleteCount)
		}
		if len(store.auditLogs) != 1 || store.auditLogs[0].Before == nil {
			t.Errorf("audit before must be non-nil (WARN-5): %+v", store.auditLogs)
		}
		if !events.hasType("bypass.delete_rule") {
			t.Errorf("event bypass.delete_rule not published")
		}
	})

	t.Run("Validate dry-run: hits guard → 422 + no CreateBypassRule called", func(t *testing.T) {
		store := &stubBypassRuleStore{}
		h := newRuleHandler(store, &stubBypassProxyProvider{}, &stubEventRecorder{})

		body := map[string]any{"scope": "global", "rule_type": "cidr", "value": "0.0.0.0/0"}
		r := newRuleTestRequest(t, "POST", "/v1/admin/bypass/rules/validate", "admin", body)
		w := httptest.NewRecorder()
		h.Validate().ServeHTTP(w, r)

		if w.Code != 422 {
			t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
		}
		var got map[string]any
		if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got["valid"] != false || got["code"] != ErrCodeBypassRuleTooBroad {
			t.Errorf("unexpected validate response: %+v", got)
		}
		if store.createCount != 0 {
			t.Errorf("Validate must NOT call CreateBypassRule, got %d", store.createCount)
		}
		if len(store.auditLogs) != 0 {
			t.Errorf("Validate must NOT write audit log, got %d", len(store.auditLogs))
		}
	})

	t.Run("Validate dry-run: valid rule → 200 valid=true", func(t *testing.T) {
		store := &stubBypassRuleStore{}
		h := newRuleHandler(store, &stubBypassProxyProvider{}, &stubEventRecorder{})

		body := map[string]any{"scope": "global", "rule_type": "domain_suffix", "value": "corp.internal"}
		r := newRuleTestRequest(t, "POST", "/v1/admin/bypass/rules/validate", "admin", body)
		w := httptest.NewRecorder()
		h.Validate().ServeHTTP(w, r)

		if w.Code != 200 {
			t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
		}
		var got map[string]any
		if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got["valid"] != true {
			t.Errorf("expect valid=true: %+v", got)
		}
	})
}

// 编译期断言
var _ AdminBypassRuleStore = (*stubBypassRuleStore)(nil)
var _ BypassAuditLogWriter = (*stubBypassRuleStore)(nil)
var _ AdminBypassProxyIPProvider = (*stubBypassProxyProvider)(nil)
