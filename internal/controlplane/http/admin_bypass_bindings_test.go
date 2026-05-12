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

type stubBypassBindingStore struct {
	mu sync.Mutex

	bindings  []repository.BypassBinding
	listErr   error
	createOut repository.BypassBinding
	createErr error
	createSeen []repository.CreateBypassBindingParams
	deleteErr error
	deleteCount int

	auditLogs []repository.InsertBypassAuditLogParams
}

func (s *stubBypassBindingStore) ListBypassBindingsByHost(_ context.Context, _ string) ([]repository.BypassBinding, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.bindings, nil
}

func (s *stubBypassBindingStore) CreateBypassBinding(_ context.Context, p repository.CreateBypassBindingParams) (repository.BypassBinding, error) {
	s.mu.Lock()
	s.createSeen = append(s.createSeen, p)
	s.mu.Unlock()
	if s.createErr != nil {
		return repository.BypassBinding{}, s.createErr
	}
	return s.createOut, nil
}

func (s *stubBypassBindingStore) DeleteBypassBinding(_ context.Context, _ string) error {
	s.mu.Lock()
	s.deleteCount++
	s.mu.Unlock()
	return s.deleteErr
}

func (s *stubBypassBindingStore) InsertBypassAuditLog(_ context.Context, p repository.InsertBypassAuditLogParams) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.auditLogs = append(s.auditLogs, p)
	return "audit-b", nil
}

func newBindingTestRequest(t *testing.T, method, target, userID string, body any) *nethttp.Request {
	t.Helper()
	return newPresetTestRequest(t, method, target, userID, body)
}

func newBindingsHandler(store *stubBypassBindingStore, events *stubEventRecorder) *AdminBypassBindingsHandler {
	return NewAdminBypassBindingsHandler(slog.Default(), store, events)
}

func TestAdminBypassBindingsHandler(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	hostID := "host-1"
	presetID := "preset-1"
	bindingPreset := repository.BypassBinding{
		ID: "bind-1", HostID: hostID, PresetID: &presetID,
		Enabled: true, Source: "admin", CreatedAt: now,
	}

	t.Run("ListByHost 200 returns bindings", func(t *testing.T) {
		store := &stubBypassBindingStore{bindings: []repository.BypassBinding{bindingPreset}}
		h := newBindingsHandler(store, &stubEventRecorder{})

		r := newBindingTestRequest(t, "GET", "/v1/admin/hosts/"+hostID+"/bypass", "admin", nil)
		r.SetPathValue("hostID", hostID)
		w := httptest.NewRecorder()
		h.ListByHost().ServeHTTP(w, r)

		if w.Code != 200 {
			t.Fatalf("status = %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "bind-1") {
			t.Errorf("body missing bind-1: %s", w.Body.String())
		}
	})

	t.Run("ListByHost missing hostID → 400", func(t *testing.T) {
		store := &stubBypassBindingStore{}
		h := newBindingsHandler(store, &stubEventRecorder{})

		r := newBindingTestRequest(t, "GET", "/v1/admin/hosts//bypass", "admin", nil)
		// 不 SetPathValue → hostID 为空
		w := httptest.NewRecorder()
		h.ListByHost().ServeHTTP(w, r)

		if w.Code != 400 {
			t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("Bind preset 201 + audit + event", func(t *testing.T) {
		store := &stubBypassBindingStore{createOut: bindingPreset}
		events := &stubEventRecorder{}
		h := newBindingsHandler(store, events)

		body := map[string]any{"preset_id": presetID, "enabled": true}
		r := newBindingTestRequest(t, "POST", "/v1/admin/hosts/"+hostID+"/bypass", "admin", body)
		r.SetPathValue("hostID", hostID)
		w := httptest.NewRecorder()
		h.Bind().ServeHTTP(w, r)

		if w.Code != 201 {
			t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
		}
		if len(store.createSeen) != 1 {
			t.Fatalf("expect 1 create call")
		}
		got := store.createSeen[0]
		if got.HostID != hostID {
			t.Errorf("host_id mismatch: %s", got.HostID)
		}
		if got.PresetID == nil || *got.PresetID != presetID {
			t.Errorf("preset_id mismatch: %+v", got.PresetID)
		}
		if got.RuleID != nil {
			t.Errorf("rule_id should be nil, got %+v", got.RuleID)
		}
		if got.Source != "admin" {
			t.Errorf("source default = %q, want admin", got.Source)
		}
		if len(store.auditLogs) != 1 || store.auditLogs[0].Action != "bind" {
			t.Errorf("audit log not written: %+v", store.auditLogs)
		}
		if !events.hasType("bypass.bind") {
			t.Errorf("event bypass.bind not published")
		}
	})

	t.Run("Bind rule 201 + audit", func(t *testing.T) {
		ruleID := "rule-1"
		store := &stubBypassBindingStore{createOut: repository.BypassBinding{
			ID: "bind-r", HostID: hostID, RuleID: &ruleID, Enabled: true, Source: "admin",
		}}
		events := &stubEventRecorder{}
		h := newBindingsHandler(store, events)

		body := map[string]any{"rule_id": ruleID}
		r := newBindingTestRequest(t, "POST", "/v1/admin/hosts/"+hostID+"/bypass", "admin", body)
		r.SetPathValue("hostID", hostID)
		w := httptest.NewRecorder()
		h.Bind().ServeHTTP(w, r)

		if w.Code != 201 {
			t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
		}
		got := store.createSeen[0]
		if got.RuleID == nil || *got.RuleID != ruleID {
			t.Errorf("rule_id mismatch")
		}
		if got.PresetID != nil {
			t.Errorf("preset_id should be nil")
		}
	})

	t.Run("Bind preset_id+rule_id both set → 400", func(t *testing.T) {
		store := &stubBypassBindingStore{}
		h := newBindingsHandler(store, &stubEventRecorder{})

		body := map[string]any{"preset_id": presetID, "rule_id": "rule-1"}
		r := newBindingTestRequest(t, "POST", "/v1/admin/hosts/"+hostID+"/bypass", "admin", body)
		r.SetPathValue("hostID", hostID)
		w := httptest.NewRecorder()
		h.Bind().ServeHTTP(w, r)

		if w.Code != 400 || !strings.Contains(w.Body.String(), ErrCodeBypassInvalidRequest) {
			t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
		}
		if len(store.createSeen) != 0 {
			t.Errorf("CreateBypassBinding should NOT be called on validation failure")
		}
	})

	t.Run("Bind preset_id+rule_id both empty → 400", func(t *testing.T) {
		store := &stubBypassBindingStore{}
		h := newBindingsHandler(store, &stubEventRecorder{})

		body := map[string]any{"enabled": true}
		r := newBindingTestRequest(t, "POST", "/v1/admin/hosts/"+hostID+"/bypass", "admin", body)
		r.SetPathValue("hostID", hostID)
		w := httptest.NewRecorder()
		h.Bind().ServeHTTP(w, r)

		if w.Code != 400 {
			t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
		}
		if len(store.createSeen) != 0 {
			t.Errorf("CreateBypassBinding should NOT be called on validation failure")
		}
	})

	t.Run("Bind missing hostID → 400", func(t *testing.T) {
		store := &stubBypassBindingStore{}
		h := newBindingsHandler(store, &stubEventRecorder{})

		body := map[string]any{"preset_id": presetID}
		r := newBindingTestRequest(t, "POST", "/v1/admin/hosts//bypass", "admin", body)
		// 不 SetPathValue → hostID 为空
		w := httptest.NewRecorder()
		h.Bind().ServeHTTP(w, r)

		if w.Code != 400 {
			t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("Unbind 204 + audit + event", func(t *testing.T) {
		store := &stubBypassBindingStore{}
		events := &stubEventRecorder{}
		h := newBindingsHandler(store, events)

		r := newBindingTestRequest(t, "DELETE", "/v1/admin/bypass/bindings/bind-1", "admin", nil)
		r.SetPathValue("bindingID", "bind-1")
		w := httptest.NewRecorder()
		h.Unbind().ServeHTTP(w, r)

		if w.Code != 204 {
			t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
		}
		if store.deleteCount != 1 {
			t.Errorf("delete count = %d, want 1", store.deleteCount)
		}
		if len(store.auditLogs) != 1 || store.auditLogs[0].Action != "unbind" {
			t.Errorf("audit log not written: %+v", store.auditLogs)
		}
		if !events.hasType("bypass.unbind") {
			t.Errorf("event bypass.unbind not published")
		}
	})

	t.Run("Unbind missing → 404", func(t *testing.T) {
		store := &stubBypassBindingStore{deleteErr: pgx.ErrNoRows}
		h := newBindingsHandler(store, &stubEventRecorder{})

		r := newBindingTestRequest(t, "DELETE", "/v1/admin/bypass/bindings/missing", "admin", nil)
		r.SetPathValue("bindingID", "missing")
		w := httptest.NewRecorder()
		h.Unbind().ServeHTTP(w, r)

		if w.Code != 404 || !strings.Contains(w.Body.String(), ErrCodeBypassBindingNotFound) {
			t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
		}
		if len(store.auditLogs) != 0 {
			t.Errorf("audit log should NOT be written on 404")
		}
	})

	t.Run("Bind with explicit source preserved", func(t *testing.T) {
		store := &stubBypassBindingStore{createOut: bindingPreset}
		h := newBindingsHandler(store, &stubEventRecorder{})

		body := map[string]any{"preset_id": presetID, "source": "system"}
		r := newBindingTestRequest(t, "POST", "/v1/admin/hosts/"+hostID+"/bypass", "admin", body)
		r.SetPathValue("hostID", hostID)
		w := httptest.NewRecorder()
		h.Bind().ServeHTTP(w, r)

		if w.Code != 201 {
			t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
		}
		if store.createSeen[0].Source != "system" {
			t.Errorf("source = %q, want system", store.createSeen[0].Source)
		}
	})

	t.Run("Bind invalid JSON → 400", func(t *testing.T) {
		store := &stubBypassBindingStore{}
		h := newBindingsHandler(store, &stubEventRecorder{})

		r := httptest.NewRequest("POST", "/v1/admin/hosts/"+hostID+"/bypass", strings.NewReader("not-json"))
		r.Header.Set("Content-Type", "application/json")
		r.SetPathValue("hostID", hostID)
		w := httptest.NewRecorder()
		h.Bind().ServeHTTP(w, r)

		if w.Code != 400 {
			t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
		}
	})

	_ = json.RawMessage{} // 静态引用，避免空 import
}

// 编译期断言
var _ AdminBypassBindingStore = (*stubBypassBindingStore)(nil)
var _ BypassAuditLogWriter = (*stubBypassBindingStore)(nil)
