package doctor

import (
	"context"
	"fmt"
	"testing"

	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude"
)

func TestCheckConfigPresent_Fail(t *testing.T) {
	orig := loadConfig
	loadConfig = func() (*cloudclaude.Config, error) { return nil, fmt.Errorf("~/.cloud-claude/config.yaml 不存在") }
	t.Cleanup(func() { loadConfig = orig })
	c, cfg := checkConfigPresent(context.Background())
	if c.Status != StatusFail {
		t.Errorf("应 Fail，实际 %s", c.Status)
	}
	if c.Code != "AUTH_CONFIG_MISSING" {
		t.Errorf("Code 应为 AUTH_CONFIG_MISSING，实际 %q", c.Code)
	}
	if cfg != nil {
		t.Error("失败时 cfg 必须 nil")
	}
}

func TestCheckConfigPresent_Pass(t *testing.T) {
	orig := loadConfig
	loadConfig = func() (*cloudclaude.Config, error) {
		return &cloudclaude.Config{Gateway: "https://gw.example.com", Username: "x", Password: "y"}, nil
	}
	t.Cleanup(func() { loadConfig = orig })
	c, cfg := checkConfigPresent(context.Background())
	if c.Status != StatusPass {
		t.Errorf("应 Pass，实际 %s", c.Status)
	}
	if cfg == nil {
		t.Error("成功时 cfg 必须非 nil")
	}
}

func TestCheckEntryTokenValid_401_Warn(t *testing.T) {
	orig := entryAuthenticate
	entryAuthenticate = func(ctx context.Context, gw, id, pw string) (*cloudclaude.AuthResponse, error) {
		return nil, fmt.Errorf("认证失败: HTTP 401")
	}
	t.Cleanup(func() { entryAuthenticate = orig })
	cfg := &cloudclaude.Config{Gateway: "https://gw.example.com"}
	c, resp := checkEntryTokenValid(context.Background(), cfg)
	if c.Status != StatusWarn {
		t.Errorf("401 应 Warn，实际 %s", c.Status)
	}
	if c.Code != "AUTH_TOKEN_EXPIRED" {
		t.Errorf("Code 应为 AUTH_TOKEN_EXPIRED，实际 %q", c.Code)
	}
	if resp != nil {
		t.Error("认证失败时 resp 必须 nil")
	}
}

func TestCheckEntryTokenValid_Success_Pass(t *testing.T) {
	orig := entryAuthenticate
	entryAuthenticate = func(ctx context.Context, gw, id, pw string) (*cloudclaude.AuthResponse, error) {
		return &cloudclaude.AuthResponse{ImageVersion: "v3.0.0", ClaudeAccountID: "acct-1"}, nil
	}
	t.Cleanup(func() { entryAuthenticate = orig })
	cfg := &cloudclaude.Config{Gateway: "https://gw.example.com"}
	c, resp := checkEntryTokenValid(context.Background(), cfg)
	if c.Status != StatusPass {
		t.Errorf("应 Pass，实际 %s", c.Status)
	}
	if resp == nil || resp.ClaudeAccountID != "acct-1" {
		t.Error("resp 必须完整透传")
	}
}

func TestCheckEntryTokenValid_NilCfg_Skip(t *testing.T) {
	c, _ := checkEntryTokenValid(context.Background(), nil)
	if c.Status != StatusSkip {
		t.Errorf("nil cfg 应 Skip，实际 %s", c.Status)
	}
}

func TestCheckOAuthCredentials_NilConn_Skip(t *testing.T) {
	c := checkOAuthCredentials(context.Background(), nil, "acct-1")
	if c.Status != StatusSkip {
		t.Errorf("nil conn 应 Skip，实际 %s", c.Status)
	}
}

func TestCheckOAuthCredentials_EmptyAccountID_Skip(t *testing.T) {
	// 通过 nil conn 短路（empty accountID 与 nil conn 都返回 Skip）。
	c := checkOAuthCredentials(context.Background(), nil, "")
	if c.Status != StatusSkip {
		t.Errorf("empty accountID 应 Skip，实际 %s", c.Status)
	}
}
