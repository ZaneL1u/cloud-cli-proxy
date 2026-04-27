// Package cloudclaude — OAuth credentials 过期检查（Phase 31 / REQ-F7-C）。
//
// 在 SSH 握手成功后、HotSync 启动 / sshfs mount 之前的窗口期，
// 远程 timeout 2 cat /home/claude/.claude/.credentials.json，按 D-22
// 三态分支处理：NotFound / Expired / ExpiringSoon / Valid。
//
// 失败容错（JSON 解析失败 / 字段缺失）→ 视为 NotFound（保守降级）。
package cloudclaude

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/crypto/ssh"
)

// OAuthState 是凭证状态枚举。
type OAuthState int

const (
	OAuthValid        OAuthState = iota // expiresAt - now ≥ 5min
	OAuthExpiringSoon                   // 0 < expiresAt - now < 5min
	OAuthExpired                        // expiresAt ≤ now
	OAuthNotFound                       // 文件不存在 / JSON 解析失败 / 字段缺失
)

// oauthExpiringWindow 是 ExpiringSoon 阈值（CONTEXT D-22 锁定 5min；10min 留 v3.1）。
const oauthExpiringWindow = 5 * time.Minute

// OAuthStatus 是 CheckOAuthCredentials / parseExpiresAt 的返回值。
type OAuthStatus struct {
	State           OAuthState
	ExpiresAt       time.Time // 解析失败 / NotFound 时为 zero value
	MinutesToExpire int       // 仅 ExpiringSoon 有意义
}

// CheckOAuthCredentials 在 connA 上远程 timeout 2 cat /home/claude/.claude/.credentials.json，
// 解析 claudeAiOauth.expiresAt（毫秒级 Unix timestamp）后按 D-22 三态返回。
//
// 任何远程命令错误（session 创建失败 / SSH 错误 / cat 退出非 0）都收敛到 OAuthNotFound
// （保守降级，避免阻塞 mount 路径）。claudeAccountID 仅用于错误信息渲染（不影响检查逻辑）。
func CheckOAuthCredentials(connA *ssh.Client, claudeAccountID string) (*OAuthStatus, error) {
	_ = claudeAccountID
	if connA == nil {
		return &OAuthStatus{State: OAuthNotFound}, nil
	}
	sess, err := connA.NewSession()
	if err != nil {
		return &OAuthStatus{State: OAuthNotFound}, nil
	}
	defer sess.Close()

	var stdout bytes.Buffer
	sess.Stdout = &stdout
	sess.Stderr = nil

	cmd := "timeout 2 cat /home/claude/.claude/.credentials.json 2>/dev/null"
	_ = sess.Run(cmd)

	return parseExpiresAt(stdout.String(), time.Now()), nil
}

// parseExpiresAt 是纯函数，便于单元测试覆盖三态 + JSON 解析容错。
//
// rawJSON 为远端 cat 的 stdout；now 用于测试注入「当前时间」。
//
// 边界规则（CONTEXT D-22）：
//   - expiresAt ≤ now → OAuthExpired
//   - 0 < expiresAt - now < 5min → OAuthExpiringSoon（严格小于；正好 5min 视为 Valid）
//   - expiresAt - now ≥ 5min → OAuthValid
//   - 任何 JSON 解析失败 / 字段缺失 / 空输入 → OAuthNotFound（保守降级）
func parseExpiresAt(rawJSON string, now time.Time) *OAuthStatus {
	if rawJSON == "" {
		return &OAuthStatus{State: OAuthNotFound}
	}
	var creds struct {
		Inner struct {
			ExpiresAt int64 `json:"expiresAt"`
		} `json:"claudeAiOauth"`
	}
	if err := json.Unmarshal([]byte(rawJSON), &creds); err != nil {
		return &OAuthStatus{State: OAuthNotFound}
	}
	if creds.Inner.ExpiresAt == 0 {
		return &OAuthStatus{State: OAuthNotFound}
	}

	expires := time.UnixMilli(creds.Inner.ExpiresAt)
	if !expires.After(now) {
		return &OAuthStatus{State: OAuthExpired, ExpiresAt: expires}
	}
	if expires.Sub(now) < oauthExpiringWindow {
		minutes := int(expires.Sub(now).Minutes())
		if minutes < 1 {
			minutes = 1
		}
		return &OAuthStatus{
			State:           OAuthExpiringSoon,
			ExpiresAt:       expires,
			MinutesToExpire: minutes,
		}
	}
	return &OAuthStatus{State: OAuthValid, ExpiresAt: expires}
}

var _ = fmt.Errorf
