package doctor

import (
	"context"
	"fmt"
	"strings"

	"golang.org/x/crypto/ssh"

	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude"
	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/errcodes"
)

// loadConfig 包级 var 注入点（auth_test.go mock 无配置场景）。
var loadConfig = cloudclaude.LoadConfig

// entryAuthenticate 包级 var（mock Entry API）。签名与 EntryClient.AuthenticateAndWait 对齐但返回简化。
var entryAuthenticate = func(ctx context.Context, gateway, username, password string) (*cloudclaude.AuthResponse, error) {
	client := cloudclaude.NewEntryClient(gateway)
	return client.AuthenticateAndWait(ctx, username, password, func(string) {})
}

// checkConfigPresent — AUTH_CONFIG_MISSING (Fatal)：LoadConfig 失败即 fail。
func checkConfigPresent(ctx context.Context) (Check, *cloudclaude.Config) {
	cfg, err := loadConfig()
	if err != nil {
		c := newFail("auth", "config_present", errcodes.AUTH_CONFIG_MISSING, err.Error())
		return c, nil
	}
	return newPass("auth", "config_present", "~/.cloud-claude/config.yaml 已加载"), cfg
}

// checkEntryTokenValid — AUTH_TOKEN_EXPIRED (Warn)：调 AuthenticateAndWait dry-run，200 pass / 401 warn。
// 返回 authResp 供后续 check（auth.oauth_credentials）复用。
func checkEntryTokenValid(ctx context.Context, cfg *cloudclaude.Config) (Check, *cloudclaude.AuthResponse) {
	if cfg == nil {
		return newSkip("auth", "entry_token_valid", "未加载 config，跳过"), nil
	}
	resp, err := entryAuthenticate(ctx, cfg.Gateway, cfg.Username, cfg.Password)
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "401") || strings.Contains(msg, "认证失败") {
			return newWarn("auth", "entry_token_valid", errcodes.AUTH_TOKEN_EXPIRED, msg), nil
		}
		return newFail("auth", "entry_token_valid", errcodes.AUTH_GATEWAY_UNREACHABLE, cfg.Gateway, msg), nil
	}
	details := map[string]any{}
	if resp != nil {
		details["image_version"] = resp.ImageVersion
		details["claude_account_id"] = resp.ClaudeAccountID
	}
	return Check{
		Domain: "auth", Name: "entry_token_valid", Status: StatusPass,
		Message: fmt.Sprintf("Entry API 认证成功（image=%s）", resp.ImageVersion),
		Details: details,
	}, resp
}

// checkOAuthCredentials 复用 cloudclaude.CheckOAuthCredentials 三态 switch（CONTEXT D-19 / RESEARCH §3.2）。
// runner 参数预留：将来可改走 runner 代替 ssh.Client；目前直接传 ssh.Client（与 oauth_check.go 一致）。
func checkOAuthCredentials(ctx context.Context, conn *ssh.Client, claudeAccountID string) Check {
	if conn == nil {
		return newSkip("auth", "oauth_credentials", "未能连接远端容器，跳过")
	}
	if claudeAccountID == "" {
		return newSkip("auth", "oauth_credentials", "Entry API 未返回 claude_account_id，跳过")
	}
	status, err := cloudclaude.CheckOAuthCredentials(conn, claudeAccountID)
	if err != nil {
		return newFail("auth", "oauth_credentials", errcodes.NET_OAUTH_NOT_FOUND, err.Error())
	}
	switch status.State {
	case cloudclaude.OAuthValid:
		return newPass("auth", "oauth_credentials", fmt.Sprintf("OAuth 有效（%d 分钟后过期）", status.MinutesToExpire))
	case cloudclaude.OAuthExpiringSoon:
		return newWarn("auth", "oauth_credentials", errcodes.NET_OAUTH_EXPIRING_SOON, status.MinutesToExpire)
	case cloudclaude.OAuthExpired:
		return newFail("auth", "oauth_credentials", errcodes.NET_OAUTH_EXPIRED)
	case cloudclaude.OAuthNotFound:
		return newFail("auth", "oauth_credentials", errcodes.NET_OAUTH_NOT_FOUND, claudeAccountID)
	default:
		return newFail("auth", "oauth_credentials", errcodes.NET_OAUTH_NOT_FOUND, "unknown state")
	}
}
