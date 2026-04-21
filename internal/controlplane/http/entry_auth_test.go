package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	nethttp "net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

// stubEntryStore 覆盖 EntryHandler 的新老依赖：在 Phase 30 Wave 2 需要同时支持
// GetHostByShortID 返回 template_image_ref、以及 ResolveClaudeAccountIDForEntry。
type stubEntryStore struct {
	hostAuth          repository.HostSSHAuth
	hostAuthErr       error
	user              repository.User
	userErr           error
	userByShortID     repository.User
	userByShortIDErr  error
	primaryHost       repository.Host
	primaryHostErr    error
	resolveAccountID  string
	resolveAccountOK  bool
	resolveAccountErr error

	gotResolveUserID string
	gotResolveHostID string
	resolveCalled    bool
}

func (s *stubEntryStore) GetUserByShortID(_ context.Context, _ string) (repository.User, error) {
	return s.userByShortID, s.userByShortIDErr
}

func (s *stubEntryStore) GetPrimaryHostByUserID(_ context.Context, _ string) (repository.Host, error) {
	return s.primaryHost, s.primaryHostErr
}

func (s *stubEntryStore) GetHostByShortID(_ context.Context, _ string) (repository.HostSSHAuth, error) {
	return s.hostAuth, s.hostAuthErr
}

func (s *stubEntryStore) GetUser(_ context.Context, _ string) (repository.User, error) {
	return s.user, s.userErr
}

func (s *stubEntryStore) ResolveClaudeAccountIDForEntry(_ context.Context, userID, hostID string) (string, bool, error) {
	s.resolveCalled = true
	s.gotResolveUserID = userID
	s.gotResolveHostID = hostID
	return s.resolveAccountID, s.resolveAccountOK, s.resolveAccountErr
}

func mustBcrypt(t *testing.T, password string) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	return string(hash)
}

func doAuth(t *testing.T, store EntryStore, shortID, password string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	handler := NewEntryHandler(slog.Default(), store, "").Auth()
	body, _ := json.Marshal(map[string]string{"password": password})
	req := httptest.NewRequest(nethttp.MethodPost, "/v1/entry/"+shortID+"/auth", bytes.NewReader(body))
	req.Host = "gateway.example.com"
	req.SetPathValue("shortId", shortID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	resp := map[string]any{}
	if rec.Body.Len() > 0 {
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode response: %v (body=%s)", err, rec.Body.String())
		}
	}
	return rec, resp
}

// TestEntryAuth_Ready_ViaHostShortID_V3Image 覆盖 D-03/D-06/D-07/D-08：
// host short_id 路径，模板镜像 tag 为 v3.0.0 → ready 响应必须带齐扩展字段。
func TestEntryAuth_Ready_ViaHostShortID_V3Image(t *testing.T) {
	now := time.Now()
	hash := mustBcrypt(t, "correct-horse")
	store := &stubEntryStore{
		hostAuth: repository.HostSSHAuth{
			HostID:           "h1",
			HostShortID:      "host-short-1",
			EntryPassword:    "host-pwd",
			HostStatus:       "running",
			UserID:           "u1",
			UserStatus:       "active",
			Username:         "alice",
			TemplateImageRef: "ghcr.io/example/cloud-claude:v3.0.0",
		},
		user: repository.User{
			ID: "u1", Username: "alice", Status: "active", Role: "user",
			PasswordHash: hash, CreatedAt: now, UpdatedAt: now,
		},
		resolveAccountID: "claude-acct-1",
		resolveAccountOK: true,
	}
	rec, resp := doAuth(t, store, "host-short-1", "correct-horse")
	if rec.Code != nethttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if resp["status"] != "ready" {
		t.Fatalf("status field = %v, want ready", resp["status"])
	}
	if resp["image_version"] != "v3.0.0" {
		t.Errorf("image_version = %v, want v3.0.0", resp["image_version"])
	}
	if resp["supports_mergerfs"] != true {
		t.Errorf("supports_mergerfs = %v, want true", resp["supports_mergerfs"])
	}
	if resp["claude_account_id"] != "claude-acct-1" {
		t.Errorf("claude_account_id = %v, want claude-acct-1", resp["claude_account_id"])
	}
	if !store.resolveCalled {
		t.Error("expected ResolveClaudeAccountIDForEntry to be called")
	}
	if store.gotResolveUserID != "u1" || store.gotResolveHostID != "h1" {
		t.Errorf("resolver called with (user=%q host=%q), want (u1, h1)", store.gotResolveUserID, store.gotResolveHostID)
	}
}

// TestEntryAuth_Ready_ViaUserShortID_V3Image 覆盖 D-03/D-06/D-07/D-08 的用户 fallback 路径。
func TestEntryAuth_Ready_ViaUserShortID_V3Image(t *testing.T) {
	hash := mustBcrypt(t, "correct-horse")
	store := &stubEntryStore{
		hostAuthErr: pgx.ErrNoRows,
		userByShortID: repository.User{
			ID: "u-99", Username: "bob", Status: "active", Role: "user", PasswordHash: hash,
		},
		primaryHost: repository.Host{
			ID: "h-99", UserID: "u-99", Status: "running", ShortID: "h-short",
			EntryPassword: "host-pwd", TemplateImageRef: "registry.internal:5000/cloud-claude:v3.0.0",
		},
		resolveAccountID: "claude-acct-7",
		resolveAccountOK: true,
	}
	rec, resp := doAuth(t, store, "user-short", "correct-horse")
	if rec.Code != nethttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if resp["status"] != "ready" {
		t.Fatalf("status = %v, want ready", resp["status"])
	}
	if resp["image_version"] != "v3.0.0" {
		t.Errorf("image_version = %v, want v3.0.0", resp["image_version"])
	}
	if resp["supports_mergerfs"] != true {
		t.Errorf("supports_mergerfs = %v, want true", resp["supports_mergerfs"])
	}
	if resp["claude_account_id"] != "claude-acct-7" {
		t.Errorf("claude_account_id = %v, want claude-acct-7", resp["claude_account_id"])
	}
	if store.gotResolveUserID != "u-99" || store.gotResolveHostID != "h-99" {
		t.Errorf("resolver called with (user=%q host=%q), want (u-99, h-99)", store.gotResolveUserID, store.gotResolveHostID)
	}
}

// TestEntryAuth_Ready_NoClaudeAccount_OmitsField 覆盖 D-05 第三条：未命中时
// claude_account_id 必须省略（omitempty），其余 v3 字段仍按镜像推导返回。
func TestEntryAuth_Ready_NoClaudeAccount_OmitsField(t *testing.T) {
	hash := mustBcrypt(t, "correct-horse")
	store := &stubEntryStore{
		hostAuth: repository.HostSSHAuth{
			HostID: "h1", HostShortID: "host-short-1", EntryPassword: "p",
			HostStatus: "running", UserID: "u1", UserStatus: "active", Username: "alice",
			TemplateImageRef: "ghcr.io/example/cloud-claude:v2.0.0",
		},
		user: repository.User{ID: "u1", Status: "active", PasswordHash: hash},
		// resolveAccountOK 默认为 false，resolveAccountID 默认为 ""
	}
	rec, resp := doAuth(t, store, "host-short-1", "correct-horse")
	if rec.Code != nethttp.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if _, ok := resp["claude_account_id"]; ok {
		t.Errorf("claude_account_id must be omitted when resolver returns ok=false, got %v", resp["claude_account_id"])
	}
	if resp["image_version"] != "v2.0.0" {
		t.Errorf("image_version = %v, want v2.0.0", resp["image_version"])
	}
	if resp["supports_mergerfs"] != false {
		t.Errorf("supports_mergerfs = %v, want false for v2 image", resp["supports_mergerfs"])
	}
}

// TestEntryAuth_ResolverError_Returns500 覆盖 D-05：解析报错不能降级为 ok=false，
// 必须 fail-fast 返回 500，避免静默丢失账号维度。
func TestEntryAuth_ResolverError_Returns500(t *testing.T) {
	hash := mustBcrypt(t, "correct-horse")
	store := &stubEntryStore{
		hostAuth: repository.HostSSHAuth{
			HostID: "h1", HostShortID: "host-short-1", EntryPassword: "p",
			HostStatus: "running", UserID: "u1", UserStatus: "active", Username: "alice",
			TemplateImageRef: "ghcr.io/example/cloud-claude:v3.0.0",
		},
		user:              repository.User{ID: "u1", Status: "active", PasswordHash: hash},
		resolveAccountErr: fmt.Errorf("db down"),
	}
	rec, _ := doAuth(t, store, "host-short-1", "correct-horse")
	if rec.Code != nethttp.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 on resolver error", rec.Code)
	}
}

// TestEntryAuth_NotReady_DoesNotForceExtensionFields 覆盖 D-08：
// 非 ready 路径不强制带 v3 扩展字段；我们锁死当前行为是"完全不带"，避免旧客户端误判。
func TestEntryAuth_NotReady_DoesNotForceExtensionFields(t *testing.T) {
	hash := mustBcrypt(t, "correct-horse")
	store := &stubEntryStore{
		hostAuth: repository.HostSSHAuth{
			HostID: "h1", HostShortID: "host-short-1", EntryPassword: "p",
			HostStatus: "stopped", UserID: "u1", UserStatus: "active", Username: "alice",
			TemplateImageRef: "ghcr.io/example/cloud-claude:v3.0.0",
		},
		user: repository.User{ID: "u1", Status: "active", PasswordHash: hash},
	}
	rec, resp := doAuth(t, store, "host-short-1", "correct-horse")
	if rec.Code != nethttp.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if resp["status"] != "not_ready" {
		t.Fatalf("status = %v, want not_ready", resp["status"])
	}
	for _, key := range []string{"image_version", "supports_mergerfs", "claude_account_id"} {
		if _, ok := resp[key]; ok {
			t.Errorf("not_ready response must not carry %q, got %v", key, resp[key])
		}
	}
	if store.resolveCalled {
		t.Error("not_ready path must not call ResolveClaudeAccountIDForEntry")
	}
}

// TestEntryAuth_InvalidCredentials_NoExtensions 锁死：认证失败不得泄露任何 v3 能力字段。
func TestEntryAuth_InvalidCredentials_NoExtensions(t *testing.T) {
	hash := mustBcrypt(t, "correct-horse")
	store := &stubEntryStore{
		hostAuth: repository.HostSSHAuth{
			HostID: "h1", HostShortID: "host-short-1", EntryPassword: "p",
			HostStatus: "running", UserID: "u1", UserStatus: "active", Username: "alice",
			TemplateImageRef: "ghcr.io/example/cloud-claude:v3.0.0",
		},
		user: repository.User{ID: "u1", Status: "active", PasswordHash: hash},
	}
	rec, resp := doAuth(t, store, "host-short-1", "wrong-password")
	if rec.Code != nethttp.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	for _, key := range []string{"image_version", "supports_mergerfs", "claude_account_id"} {
		if _, ok := resp[key]; ok {
			t.Errorf("401 response must not expose %q", key)
		}
	}
}
