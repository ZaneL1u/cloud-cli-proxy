// Package cloudclaude — Phase 32 会话层（tmux 默认包装 + 多端共享 attach）。
//
// 本文件按"基础层 / 高层"两段分布：
//
//   - 基础层（Task 2.1a）：DetectTmux / 命名 helpers / 远程命令模板 / 客户端文件
//     注册表（write/remove/read）/ tmux list-clients 解析 / 时间渲染 / 纯函数 helpers。
//
//   - 高层（Task 2.1b）：runClaudeWithSession / runClaudePTYWithReconnect /
//     runClaudePTYBare / performTakeOver / printAttachBanner /
//     RunSessionsLs / RunSessionsAttach。
//
// 设计约束：
//   - 远程命令所有插值参数必须走 shellescape.Quote / shellescape.QuoteCommand
//     （SP-03，禁止手写 '...' 引号）。
//   - 命名复用 mount_strategy.simpleHash8（同包，禁止重新发明）。
//   - 错误码全部走 errcodes.Format，禁止裸字符串。
package cloudclaude

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"al.essio.dev/pkg/shellescape"
	"golang.org/x/crypto/ssh"
)

// SessionConfig 由 cmd 层构造、ConnectAndRunClaudeV3 透传给 runClaudeWithSession。
//
// 字段来源（CONTEXT D-29）：
//   - AccountID / KeepAlive*：mountCfg
//   - ShortID / TakeOver / LocalHostname：cobra flag + os.Hostname() 注入
//   - TmuxAvailable：DetectTmux 探测结果
//   - ReconnectEnabled：默认 true，测试可关
type SessionConfig struct {
	AccountID         string
	ShortID           string
	TakeOver          bool
	TmuxAvailable     bool
	KeepAliveInterval time.Duration
	KeepAliveCountMax int
	ReconnectEnabled  bool
	NoColor           bool
	Cwd               string
	LocalHostname     string
}

// clientsRegistryDir 是容器内文件注册表目录（D-12 完整方案）。
// UID 1000 默认可写 /workspace；不污染 mergerfs/Mutagen — Phase 31 mutagen
// ignore 已包含 .cloud-claude/ 顶层匹配。
const clientsRegistryDir = "/workspace/.cloud-claude/clients"

// clientFileSchema 是注册表 JSON 的 schema_version=1 结构（D-12 修订）。
//
// ClientRole 在本 plan 始终写 "primary"（"我能 attach 上即视为本端是 primary 视角"）；
// Plan 03 在 ErrSyncLocked 路径覆写为 "secondary"。
type clientFileSchema struct {
	SchemaVersion   int    `json:"schema_version"`
	Hostname        string `json:"hostname"`
	TmuxClientPID   int    `json:"tmux_client_pid"`
	TmuxSession     string `json:"tmux_session"`
	AttachAtUnix    int64  `json:"attach_at_unix"`
	ClaudeAccountID string `json:"claude_account_id"`
	ClientRole      string `json:"client_role"`
}

// DetectTmux 远程探测 tmux 是否可用（CONTEXT D-15 / D-16 / REQ-F4-C）。
//
// 远程命令: command -v tmux >/dev/null 2>&1 && tmux -V 2>&1
//   - 成功 → (true, "tmux X.Y", "")
//   - 任何失败 → (false, "", reason) 并 **不阻塞** 启动（caller 退化到 v2.0 runClaude）。
func DetectTmux(conn *ssh.Client) (available bool, version string, reason string) {
	if conn == nil {
		return false, "", "no connection"
	}
	sess, err := conn.NewSession()
	if err != nil {
		return false, "", err.Error()
	}
	defer sess.Close()
	var buf bytes.Buffer
	sess.Stdout = &buf
	sess.Stderr = &buf
	runErr := sess.Run("command -v tmux >/dev/null 2>&1 && tmux -V 2>&1")
	if runErr != nil {
		out := strings.TrimSpace(buf.String())
		if out == "" {
			out = runErr.Error()
		}
		return false, "", out
	}
	return true, strings.TrimSpace(buf.String()), ""
}

// buildTmuxSessionName 默认 session 命名（D-07 / D-09）。
//   - 非空 accountID → "claude-<account_id_short8>"（前 8 字符小写去 "-"）
//   - 空 accountID → "claude-anon-<simpleHash8(cwd)>"（D-09 退化）
//   - 长度 > 32 / 非法字符 → sanitizeSessionName 兜底
func buildTmuxSessionName(accountID, cwd string) string {
	var raw string
	if accountID == "" {
		raw = "claude-anon-" + simpleHash8(cwd)
	} else {
		id8 := strings.ToLower(strings.ReplaceAll(accountID, "-", ""))
		if len(id8) > 8 {
			id8 = id8[:8]
		}
		raw = "claude-" + id8
	}
	sanitized, _ := sanitizeSessionName(raw)
	return sanitized
}

// buildShortIDSessionName 用于 --new-session（D-08）。
// 与默认 8-hex 命名空间正交（base64url 含 '-' / '_'）。
func buildShortIDSessionName() string {
	return "claude-" + GenerateShortSessionID()
}

// GenerateShortSessionID 暴露给 cmd/cloud-claude/main.go 在 --new-session 触发时调用。
// crypto/rand 6 字节 → base64url 8 字符（无填充）。
//
// 极端情况（rand.Read 失败）退化到时间戳后缀，仍保证返回 8 字符。
func GenerateShortSessionID() string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		s := strconv.FormatInt(time.Now().UnixNano(), 36)
		if len(s) >= 8 {
			return s[len(s)-8:]
		}
		return strings.Repeat("0", 8-len(s)) + s
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

// sanitizeSessionName 字符集 [a-zA-Z0-9_-]，非法字符替换 '_'；长度 > 32 截断。
// 返回 (sanitized, warned) — warned=true 时调用方可选 stderr 提示。
func sanitizeSessionName(name string) (string, bool) {
	warned := false
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
			warned = true
		}
	}
	sanitized := b.String()
	if len(sanitized) > 32 {
		sanitized = sanitized[:32]
		warned = true
	}
	return sanitized, warned
}

// buildClaudeCmd 构造 claude 调用串（PATTERNS SP-03 风格）。
// hasProxy=true 时多一段 export PATH=<binDir>:$PATH。
func buildClaudeCmd(claudeArgs []string, hasProxy bool, remoteCwd string) string {
	claudeCmd := shellescape.QuoteCommand(append([]string{"claude"}, claudeArgs...))
	if hasProxy {
		binDir := remoteCwd + "/.cloud-claude/bin"
		return fmt.Sprintf("export PATH=%s:$PATH && %s",
			shellescape.Quote(binDir), claudeCmd)
	}
	return claudeCmd
}

// buildTmuxRemoteCmd 构造 D-10 完整远程命令模板。所有参数已 shellescape。
//
//   cd <cwd_q> && command -v tmux >/dev/null 2>&1 \
//     && exec tmux new-session -A -d -s <session_q> <wrap_q> \; attach-session -t <session_q> \
//     || exec <fallback>
//
// wrapCmd = "cd <cwd_q> && <claudeCmd>"（整体 shellescape 一次后塞给 tmux new-session）。
// fallback = wrapCmd 字面值（不经过 tmux 直接 exec）。
func buildTmuxRemoteCmd(remoteCwd, sessionName, claudeCmd string) string {
	cwdQ := shellescape.Quote(remoteCwd)
	sessionQ := shellescape.Quote(sessionName)
	wrapCmd := fmt.Sprintf("cd %s && %s", cwdQ, claudeCmd)
	wrapQ := shellescape.Quote(wrapCmd)
	return fmt.Sprintf(
		"cd %s && command -v tmux >/dev/null 2>&1 && exec tmux new-session -A -d -s %s %s \\; attach-session -t %s || exec %s",
		cwdQ, sessionQ, wrapQ, sessionQ, wrapCmd,
	)
}

// sshOutput 是 mount.go::sshRun 的"取 stdout"姊妹版本（同包私有，无需修改 mount.go）。
// 失败时仍返回已收集的 CombinedOutput 内容 + 原始 err（便于 caller 记录）。
func sshOutput(conn *ssh.Client, cmd string) (string, error) {
	if conn == nil {
		return "", fmt.Errorf("nil ssh.Client")
	}
	sess, err := conn.NewSession()
	if err != nil {
		return "", err
	}
	defer sess.Close()
	out, err := sess.CombinedOutput(cmd)
	return string(out), err
}

// writeClientFile attach 成功后立即调一次（D-12 / Q1 RESOLVED）。
//
// 流程：
//  1. 远程 tmux display-message -p '#{client_pid}' 取本端 client_pid
//  2. 构造 clientFileSchema → json.Marshal
//  3. 远程 mkdir + printf '%s' <json_q> > <pid>.json
//
// 失败仅返回 (0, err)；caller 记录 warning 即可，不阻塞 attach。
func writeClientFile(conn *ssh.Client, sessionName, accountID, hostname string) (int, error) {
	if conn == nil {
		return 0, fmt.Errorf("nil ssh.Client")
	}
	if hostname == "" {
		hostname = "unknown-host"
	}

	pidOut, err := sshOutput(conn, "tmux display-message -p '#{client_pid}' 2>/dev/null")
	if err != nil {
		return 0, fmt.Errorf("tmux display-message 失败: %w", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(pidOut))
	if err != nil || pid <= 0 {
		return 0, fmt.Errorf("解析 client_pid 失败: %q", pidOut)
	}

	entry := clientFileSchema{
		SchemaVersion:   1,
		Hostname:        hostname,
		TmuxClientPID:   pid,
		TmuxSession:     sessionName,
		AttachAtUnix:    time.Now().Unix(),
		ClaudeAccountID: accountID,
		ClientRole:      "primary",
	}
	jsonBytes, err := json.Marshal(entry)
	if err != nil {
		return 0, err
	}

	writeCmd := fmt.Sprintf(
		"mkdir -p %s && printf '%%s' %s > %s/%d.json",
		shellescape.Quote(clientsRegistryDir),
		shellescape.Quote(string(jsonBytes)),
		shellescape.Quote(clientsRegistryDir),
		pid,
	)
	if err := sshRun(conn, writeCmd); err != nil {
		return pid, fmt.Errorf("写注册表失败: %w", err)
	}
	return pid, nil
}

// removeClientFile 在 runClaudeWithSession defer 退出时调用；失败忽略
// （SSH 异常断开导致 rm 未执行 → 孤儿条目下次 attach 通过 tmux list-clients 对照被动跳过）。
func removeClientFile(conn *ssh.Client, remoteTmuxClientPid int) error {
	if conn == nil || remoteTmuxClientPid <= 0 {
		return nil
	}
	rmCmd := fmt.Sprintf("rm -f %s/%d.json",
		shellescape.Quote(clientsRegistryDir), remoteTmuxClientPid)
	return sshRun(conn, rmCmd)
}

// readClientHostnames 批量读取注册表 hostname；缺失 / parse 失败 → "unknown-host"。
//
// 单 SSH session 多 cat（减少往返）：
//
//	for pid in <pids>; do echo "===<pid>==="; cat .../<pid>.json 2>/dev/null || true; done
//
// 永不返回 error；返回 map 长度 == len(otherClientPids)。
func readClientHostnames(conn *ssh.Client, otherClientPids []int) map[int]string {
	result := make(map[int]string, len(otherClientPids))
	for _, pid := range otherClientPids {
		result[pid] = "unknown-host"
	}
	if conn == nil || len(otherClientPids) == 0 {
		return result
	}

	pidsList := make([]string, len(otherClientPids))
	for i, p := range otherClientPids {
		pidsList[i] = strconv.Itoa(p)
	}
	script := fmt.Sprintf(
		`for pid in %s; do echo "===${pid}==="; cat %s/${pid}.json 2>/dev/null || true; done`,
		strings.Join(pidsList, " "),
		shellescape.Quote(clientsRegistryDir),
	)
	out, err := sshOutput(conn, script)
	if err != nil {
		return result
	}
	for pid, host := range parseClientRegistryDump(out) {
		if host != "" {
			result[pid] = host
		}
	}
	return result
}

// parseClientRegistryDump 是 readClientHostnames 的纯函数解析层（单测友好）。
//
// 输入格式（来自远程 for pid in ...; do echo "===<pid>==="; cat <pid>.json || true; done）：
//
//	===<pid1>===\n<json or empty>\n===<pid2>===\n<json or empty>\n...
//
// strings.Split(out, "===") 切完后：
//   - sections[0] 通常空字符串（首段）
//   - sections[1] / sections[3] / ... = pid 串（奇数 index）
//   - sections[2] / sections[4] / ... = json 串（偶数 index，可能为空）
//
// 返回 map[pid]hostname；解析失败 / hostname 空 → 不写入 map（caller 兜底 unknown-host）。
func parseClientRegistryDump(out string) map[int]string {
	result := map[int]string{}
	sections := strings.Split(out, "===")
	for i := 1; i+1 < len(sections); i += 2 {
		pidStr := strings.TrimSpace(sections[i])
		body := strings.TrimSpace(sections[i+1])
		pid, err := strconv.Atoi(pidStr)
		if err != nil || pid <= 0 || body == "" {
			continue
		}
		var entry clientFileSchema
		if json.Unmarshal([]byte(body), &entry) == nil && entry.Hostname != "" {
			result[pid] = entry.Hostname
		}
	}
	return result
}

// tmuxClient 是 tmux list-clients 单条解析结果。
type tmuxClient struct {
	PID      int
	Activity time.Time
	TTY      string
}

// parseTmuxListClients 解析 'pid|unix_seconds|tty' 多行输出。
// 空输入 / 字段不足的行被跳过。
func parseTmuxListClients(out string) []tmuxClient {
	out = strings.TrimSpace(out)
	if out == "" {
		return nil
	}
	var clients []tmuxClient
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "|", 3)
		if len(fields) < 3 {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSpace(fields[0]))
		if err != nil {
			continue
		}
		actSec, _ := strconv.ParseInt(strings.TrimSpace(fields[1]), 10, 64)
		clients = append(clients, tmuxClient{
			PID:      pid,
			Activity: time.Unix(actSec, 0),
			TTY:      strings.TrimSpace(fields[2]),
		})
	}
	return clients
}

// renderActivityAge 三档活跃度文案（D-12 修订）。
//   - < 30s → "刚刚活跃"
//   - < 1h  → "N 分钟前活跃"
//   - >= 1h → "N 小时前活跃"
//
// d 为负数时按 0 处理（防御 tmux 输出时间戳轻微早于本地时钟）。
func renderActivityAge(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < 30*time.Second:
		return "刚刚活跃"
	case d < time.Hour:
		return fmt.Sprintf("%d 分钟前活跃", int(d.Minutes()))
	default:
		return fmt.Sprintf("%d 小时前活跃", int(d.Hours()))
	}
}
