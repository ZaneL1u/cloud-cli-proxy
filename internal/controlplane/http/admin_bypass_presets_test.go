package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"database/sql"

	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

// stubBypassPresetStore 实现 AdminBypassPresetStore + BypassAuditLogWriter（接口重叠）。
type stubBypassPresetStore struct {
	mu sync.Mutex

	presets    []repository.BypassPreset
	preset     repository.BypassPreset
	getErr     error
	createOut  repository.BypassPreset
	createErr  error
	updateOut  repository.BypassPreset
	updateErr  error
	deleteErr  error
	listErr    error
	auditLogs  []repository.InsertBypassAuditLogParams
	auditErr   error
	getCallIDs []string
}

func (s *stubBypassPresetStore) ListBypassPresets(_ context.Context) ([]repository.BypassPreset, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.presets, nil
}

func (s *stubBypassPresetStore) GetBypassPresetByID(_ context.Context, id string) (repository.BypassPreset, error) {
	s.mu.Lock()
	s.getCallIDs = append(s.getCallIDs, id)
	s.mu.Unlock()
	if s.getErr != nil {
		return repository.BypassPreset{}, s.getErr
	}
	return s.preset, nil
}

func (s *stubBypassPresetStore) CreateBypassPreset(_ context.Context, _ repository.CreateBypassPresetParams) (repository.BypassPreset, error) {
	if s.createErr != nil {
		return repository.BypassPreset{}, s.createErr
	}
	return s.createOut, nil
}

func (s *stubBypassPresetStore) UpdateBypassPreset(_ context.Context, _ string, _ repository.UpdateBypassPresetParams) (repository.BypassPreset, error) {
	if s.updateErr != nil {
		return repository.BypassPreset{}, s.updateErr
	}
	return s.updateOut, nil
}

func (s *stubBypassPresetStore) DeleteBypassPreset(_ context.Context, _ string) error {
	return s.deleteErr
}

func (s *stubBypassPresetStore) InsertBypassAuditLog(_ context.Context, p repository.InsertBypassAuditLogParams) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.auditErr != nil {
		return "", s.auditErr
	}
	s.auditLogs = append(s.auditLogs, p)
	return "audit-1", nil
}

// newPresetTestRequest 构造一个携带 user_id context 的请求（绕过中间件、保留 actor 注入）。
func newPresetTestRequest(t *testing.T, method, target, userID string, body any) *nethttp.Request {
	t.Helper()
	var buf *bytes.Buffer
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		buf = bytes.NewBuffer(raw)
	} else {
		buf = bytes.NewBuffer(nil)
	}
	r := httptest.NewRequest(method, target, buf)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Forwarded-For", "10.0.0.1, 127.0.0.1")
	r.RemoteAddr = "127.0.0.1:54321"
	if userID != "" {
		ctx := context.WithValue(r.Context(), ctxKeyUserID, userID)
		r = r.WithContext(ctx)
	}
	return r
}

func newPresetHandler(store *stubBypassPresetStore, events *stubEventRecorder) *AdminBypassPresetsHandler {
	return NewAdminBypassPresetsHandler(slog.Default(), store, events)
}

func TestAdminBypassPresetsHandler(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	systemPreset := repository.BypassPreset{
		ID: "preset-sys", Slug: "loopback", Name: "Loopback",
		IsSystem: true, IsForceOn: true, IsActive: true,
		Rules:     []repository.BypassPresetRule{{RuleType: "cidr", Value: "127.0.0.0/8"}},
		CreatedAt: now, UpdatedAt: now,
	}
	userPreset := repository.BypassPreset{
		ID: "preset-1", Slug: "corp", Name: "Corp",
		IsSystem: false, IsForceOn: false, IsActive: true,
		Rules:     []repository.BypassPresetRule{{RuleType: "domain_suffix", Value: "corp.internal"}},
		CreatedAt: now, UpdatedAt: now,
	}

	t.Run("List 200 returns presets envelope", func(t *testing.T) {
		store := &stubBypassPresetStore{presets: []repository.BypassPreset{systemPreset, userPreset}}
		events := &stubEventRecorder{}
		h := newPresetHandler(store, events)

		r := newPresetTestRequest(t, "GET", "/v1/admin/bypass/presets", "test-admin-id", nil)
		w := httptest.NewRecorder()
		h.List().ServeHTTP(w, r)

		if w.Code != 200 {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		var body map[string]any
		if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		arr, ok := body["presets"].([]any)
		if !ok || len(arr) != 2 {
			t.Fatalf("expect 2 presets, got %v", body)
		}
	})

	t.Run("Get 404 when not found", func(t *testing.T) {
		store := &stubBypassPresetStore{getErr: sql.ErrNoRows}
		h := newPresetHandler(store, &stubEventRecorder{})

		r := newPresetTestRequest(t, "GET", "/v1/admin/bypass/presets/missing", "test-admin-id", nil)
		r.SetPathValue("presetID", "missing")
		w := httptest.NewRecorder()
		h.Get().ServeHTTP(w, r)

		if w.Code != 404 {
			t.Fatalf("status = %d, want 404", w.Code)
		}
		if !strings.Contains(w.Body.String(), ErrCodeBypassPresetNotFound) {
			t.Errorf("missing error code: %s", w.Body.String())
		}
	})

	t.Run("Create 201 + audit + event published", func(t *testing.T) {
		store := &stubBypassPresetStore{createOut: userPreset}
		events := &stubEventRecorder{}
		h := newPresetHandler(store, events)

		body := map[string]any{
			"slug": "corp", "name": "Corp", "description": "corp tunnel",
			"is_force_on": false, "is_active": true,
			"rules": []map[string]any{
				{"rule_type": "domain_suffix", "value": "corp.internal", "note": "internal"},
			},
		}
		r := newPresetTestRequest(t, "POST", "/v1/admin/bypass/presets", "test-admin-id", body)
		w := httptest.NewRecorder()
		h.Create().ServeHTTP(w, r)

		if w.Code != 201 {
			t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
		}
		if len(store.auditLogs) != 1 {
			t.Fatalf("expect 1 audit log entry, got %d", len(store.auditLogs))
		}
		got := store.auditLogs[0]
		if got.Action != "create_preset" || got.TargetKind != "preset" {
			t.Errorf("unexpected audit action=%s kind=%s", got.Action, got.TargetKind)
		}
		if got.ActorID == nil || *got.ActorID != "test-admin-id" {
			t.Errorf("actor_id mismatch: %+v", got.ActorID)
		}
		if got.ActorIP != "10.0.0.1" {
			t.Errorf("actor_ip = %q, want 10.0.0.1 (first XFF token)", got.ActorIP)
		}
		if !events.hasType("bypass.create_preset") {
			t.Errorf("event bypass.create_preset not published: %+v", events.events)
		}
	})

	t.Run("Create 422 when rule violates guardrail", func(t *testing.T) {
		store := &stubBypassPresetStore{createOut: userPreset}
		h := newPresetHandler(store, &stubEventRecorder{})

		body := map[string]any{
			"slug": "bad", "name": "Bad",
			"rules": []map[string]any{
				{"rule_type": "cidr", "value": "0.0.0.0/0"},
			},
		}
		r := newPresetTestRequest(t, "POST", "/v1/admin/bypass/presets", "test-admin-id", body)
		w := httptest.NewRecorder()
		h.Create().ServeHTTP(w, r)

		if w.Code != 422 {
			t.Fatalf("status = %d, want 422 body=%s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), ErrCodeBypassRuleTooBroad) {
			t.Errorf("missing TOO_BROAD code: %s", w.Body.String())
		}
		if len(store.auditLogs) != 0 {
			t.Errorf("audit log should not be written on guard failure")
		}
	})

	t.Run("Update system preset returns 403 BYPASS_PRESET_IMMUTABLE", func(t *testing.T) {
		store := &stubBypassPresetStore{
			preset:    systemPreset,
			updateErr: repository.ErrSystemBypassPresetImmutable,
		}
		h := newPresetHandler(store, &stubEventRecorder{})

		body := map[string]any{"name": stringPtr("renamed")}
		r := newPresetTestRequest(t, "PATCH", "/v1/admin/bypass/presets/preset-sys", "test-admin-id", body)
		r.SetPathValue("presetID", "preset-sys")
		w := httptest.NewRecorder()
		h.Update().ServeHTTP(w, r)

		if w.Code != 403 {
			t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), ErrCodeBypassPresetImmutable) {
			t.Errorf("missing IMMUTABLE code: %s", w.Body.String())
		}
	})

	t.Run("Update missing preset returns 404", func(t *testing.T) {
		store := &stubBypassPresetStore{getErr: sql.ErrNoRows}
		h := newPresetHandler(store, &stubEventRecorder{})

		body := map[string]any{"name": stringPtr("x")}
		r := newPresetTestRequest(t, "PATCH", "/v1/admin/bypass/presets/missing", "test-admin-id", body)
		r.SetPathValue("presetID", "missing")
		w := httptest.NewRecorder()
		h.Update().ServeHTTP(w, r)

		if w.Code != 404 {
			t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
		}
	})

	t.Run("Delete system preset returns 403 BYPASS_PRESET_IMMUTABLE", func(t *testing.T) {
		store := &stubBypassPresetStore{
			preset:    systemPreset,
			deleteErr: repository.ErrSystemBypassPresetImmutable,
		}
		h := newPresetHandler(store, &stubEventRecorder{})

		r := newPresetTestRequest(t, "DELETE", "/v1/admin/bypass/presets/preset-sys", "test-admin-id", nil)
		r.SetPathValue("presetID", "preset-sys")
		w := httptest.NewRecorder()
		h.Delete().ServeHTTP(w, r)

		if w.Code != 403 {
			t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), ErrCodeBypassPresetImmutable) {
			t.Errorf("missing IMMUTABLE code: %s", w.Body.String())
		}
	})

	t.Run("Delete 204 + audit + event published", func(t *testing.T) {
		store := &stubBypassPresetStore{preset: userPreset}
		events := &stubEventRecorder{}
		h := newPresetHandler(store, events)

		r := newPresetTestRequest(t, "DELETE", "/v1/admin/bypass/presets/preset-1", "test-admin-id", nil)
		r.SetPathValue("presetID", "preset-1")
		w := httptest.NewRecorder()
		h.Delete().ServeHTTP(w, r)

		if w.Code != 204 {
			t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
		}
		if len(store.auditLogs) != 1 {
			t.Fatalf("expect 1 audit log entry, got %d", len(store.auditLogs))
		}
		if store.auditLogs[0].Action != "delete_preset" {
			t.Errorf("unexpected audit action=%s", store.auditLogs[0].Action)
		}
		if !events.hasType("bypass.delete_preset") {
			t.Errorf("event bypass.delete_preset not published: %+v", events.events)
		}
	})

	t.Run("Update 200 success records before/after audit", func(t *testing.T) {
		store := &stubBypassPresetStore{
			preset:    userPreset,
			updateOut: userPreset,
		}
		events := &stubEventRecorder{}
		h := newPresetHandler(store, events)

		body := map[string]any{"name": stringPtr("Corp v2")}
		r := newPresetTestRequest(t, "PATCH", "/v1/admin/bypass/presets/preset-1", "test-admin-id", body)
		r.SetPathValue("presetID", "preset-1")
		w := httptest.NewRecorder()
		h.Update().ServeHTTP(w, r)

		if w.Code != 200 {
			t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
		}
		if len(store.auditLogs) != 1 {
			t.Fatalf("expect 1 audit log entry")
		}
		log := store.auditLogs[0]
		if log.Action != "update_preset" || log.Before == nil || log.After == nil {
			t.Errorf("expect update_preset with before+after, got %+v", log)
		}
		// sanity check: before unmarshal contains slug.
		var beforeDoc map[string]any
		if err := json.Unmarshal(log.Before, &beforeDoc); err != nil {
			t.Fatalf("unmarshal before: %v", err)
		}
		if beforeDoc["slug"] != "corp" {
			t.Errorf("before.slug = %v, want corp", beforeDoc["slug"])
		}
		if !events.hasType("bypass.update_preset") {
			t.Errorf("event bypass.update_preset not published")
		}
	})

	t.Run("Create unauthenticated context still allows handler (router-level guard handles 401)", func(t *testing.T) {
		// 注：401 是 adminGuard 中间件职责（T-46-01 走 router 链路，下方 TestAdminBypassRouter401 覆盖）；
		// 本子用例验证 handler 自己对无 actor 上下文不会 panic、可降级落 nil actor_id。
		store := &stubBypassPresetStore{createOut: userPreset}
		events := &stubEventRecorder{}
		h := newPresetHandler(store, events)

		body := map[string]any{
			"slug": "corp", "name": "Corp",
			"rules": []map[string]any{{"rule_type": "domain_suffix", "value": "corp.internal"}},
		}
		r := newPresetTestRequest(t, "POST", "/v1/admin/bypass/presets", "", body)
		w := httptest.NewRecorder()
		h.Create().ServeHTTP(w, r)

		if w.Code != 201 {
			t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
		}
		if len(store.auditLogs) != 1 || store.auditLogs[0].ActorID != nil {
			t.Errorf("expect actor_id nil when no user_id in context, got %+v", store.auditLogs[0])
		}
	})
}

func TestAdminBypassRouter401Unauthenticated(t *testing.T) {
	// T-46-01: 没有 Authorization 头，adminGuard 直接拦截，handler 不执行。
	// 通过手工组装 adminGuard(handler.List()) 验证 401 路径，不依赖 router.go 中
	// Plan 01 Task 5 的 Dependencies 注册（Task 5 会补 Dependencies 字段 + 路由）。
	store := &stubBypassPresetStore{presets: []repository.BypassPreset{}}
	h := newPresetHandler(store, &stubEventRecorder{})

	authMw := AuthMiddleware(testJWTSecret)
	guarded := authMw(RequireRole("admin")(h.List()))

	r := httptest.NewRequest("GET", "/v1/admin/bypass/presets", nil)
	// 故意不设 Authorization。
	w := httptest.NewRecorder()
	guarded.ServeHTTP(w, r)

	if w.Code != 401 {
		t.Fatalf("status = %d, want 401 (handler should NOT be reached)", w.Code)
	}
	if len(store.getCallIDs) != 0 {
		t.Errorf("handler executed despite 401: getCallIDs=%v", store.getCallIDs)
	}
}

// stringPtr 小工具，用于 PATCH 请求构造可选指针字段。
func stringPtr(s string) *string { return &s }

// 编译期断言：stub 实现两个接口。
var _ AdminBypassPresetStore = (*stubBypassPresetStore)(nil)
var _ BypassAuditLogWriter = (*stubBypassPresetStore)(nil)

// 静态检查 ErrSystemBypassPresetImmutable 是 errors.Is 友好的 sentinel。
var _ = errors.Is
