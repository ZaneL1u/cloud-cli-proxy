package cloudclaude

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestAuthResponse_V3Fields_RoundTrip 覆盖 Phase 30 Wave 2 D-03：
// v3 服务器响应扩展 image_version / supports_mergerfs / claude_account_id，
// 客户端结构体必须能完整读取。
func TestAuthResponse_V3Fields_RoundTrip(t *testing.T) {
	payload := `{
		"status": "ready",
		"ssh_user": "u", "ssh_pass": "p", "ssh_host": "h", "ssh_port": 2222,
		"image_version": "v3.0.0",
		"supports_mergerfs": true,
		"claude_account_id": "claude-acct-42"
	}`
	var resp AuthResponse
	if err := json.Unmarshal([]byte(payload), &resp); err != nil {
		t.Fatalf("unmarshal v3 payload: %v", err)
	}
	if resp.ImageVersion != "v3.0.0" {
		t.Errorf("ImageVersion = %q, want v3.0.0", resp.ImageVersion)
	}
	if !resp.SupportsMergerfs {
		t.Errorf("SupportsMergerfs = false, want true")
	}
	if resp.ClaudeAccountID != "claude-acct-42" {
		t.Errorf("ClaudeAccountID = %q, want claude-acct-42", resp.ClaudeAccountID)
	}
}

// TestAuthResponse_MissingV3Fields_DefaultZero 覆盖 D-08：非 v3 服务器不返回扩展字段时，
// 客户端字段默认零值即可，既有 SSH 校验路径不受影响。
func TestAuthResponse_MissingV3Fields_DefaultZero(t *testing.T) {
	payload := `{"status":"ready","ssh_user":"u","ssh_pass":"p","ssh_host":"h","ssh_port":2222}`
	var resp AuthResponse
	if err := json.Unmarshal([]byte(payload), &resp); err != nil {
		t.Fatalf("unmarshal v2 payload: %v", err)
	}
	if resp.ImageVersion != "" || resp.SupportsMergerfs || resp.ClaudeAccountID != "" {
		t.Errorf("v2 payload must leave v3 fields zero-valued, got: %+v", resp)
	}
	if resp.SSHHost != "h" || resp.SSHPort != 2222 || resp.SSHUser != "u" || resp.SSHPass != "p" {
		t.Errorf("v2 SSH fields must survive decode, got: %+v", resp)
	}
}

// TestAuthResponse_MarshalOmitempty 锁死 v3 扩展字段的 omitempty 行为：
// 仅在有意义时出现在 JSON 里，避免把"没拿到"误为"明确的 false/空串"。
func TestAuthResponse_MarshalOmitempty(t *testing.T) {
	resp := AuthResponse{
		Status: "ready", SSHUser: "u", SSHPass: "p", SSHHost: "h", SSHPort: 2222,
	}
	buf, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, key := range []string{`"image_version"`, `"supports_mergerfs"`, `"claude_account_id"`} {
		if strings.Contains(string(buf), key) {
			t.Errorf("empty value must be omitted for %s, got: %s", key, buf)
		}
	}
}

// TestAuthenticate_V3Gateway_PreservesExtensions 覆盖端到端：
// Authenticate() 针对 v3 gateway 返回的 JSON 必须把扩展字段原样透传，且不破坏既有 SSH 四元组校验（D-08）。
func TestAuthenticate_V3Gateway_PreservesExtensions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"status":"ready","ssh_user":"u","ssh_pass":"p","ssh_host":"h","ssh_port":2222,
			"image_version":"v3.0.0","supports_mergerfs":true,
			"claude_account_id":"claude-acct-42"
		}`))
	}))
	defer srv.Close()

	client := NewEntryClient(srv.URL)
	resp, err := client.Authenticate(context.Background(), "short", "pwd")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if resp.Status != "ready" {
		t.Fatalf("status = %q", resp.Status)
	}
	if resp.ImageVersion != "v3.0.0" || !resp.SupportsMergerfs || resp.ClaudeAccountID != "claude-acct-42" {
		t.Errorf("extensions lost after round-trip: %+v", resp)
	}
}

// TestAuthenticate_V2Gateway_NoExtensionsRequired 锁死向后兼容：
// 旧 gateway 只返回 v2 字段时 Authenticate() 依然成功，扩展字段保持零值（D-08）。
func TestAuthenticate_V2Gateway_NoExtensionsRequired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ready","ssh_user":"u","ssh_pass":"p","ssh_host":"h","ssh_port":2222}`))
	}))
	defer srv.Close()

	client := NewEntryClient(srv.URL)
	resp, err := client.Authenticate(context.Background(), "short", "pwd")
	if err != nil {
		t.Fatalf("Authenticate on v2 gateway: %v", err)
	}
	if resp.Status != "ready" || resp.SSHHost != "h" || resp.SSHPort != 2222 {
		t.Fatalf("v2 ready payload lost: %+v", resp)
	}
	if resp.ImageVersion != "" || resp.SupportsMergerfs || resp.ClaudeAccountID != "" {
		t.Errorf("v3 fields must be zero-valued on v2 gateway, got: %+v", resp)
	}
}
