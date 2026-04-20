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
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"text/tabwriter"
	"time"

	"al.essio.dev/pkg/shellescape"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"

	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/errcodes"
)

// SessionConfig 由 cmd 层构造、ConnectAndRunClaudeV3 透传给 runClaudeWithSession。
//
// 字段来源（CONTEXT D-29）：
//   - AccountID / KeepAlive* / LastSessionPath：mountCfg
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
	LastSessionPath   string

	// [Phase 32 Plan 03 新增] 由 ssh.go 从 mountCfg.IsSecondaryClient 透传而来。
	// true → writeLastSessionTmuxField 写 ClientRole="secondary" + writeClientFile
	//        client_role="secondary"；
	// false（默认）→ "primary"（即 attach 上即视为本端持有 Mutagen 同步锁）。
	IsSecondaryClient bool
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
//
// role 由 caller 决定（"primary" / "secondary"）— Plan 03 通过
// SessionConfig.IsSecondaryClient 透传到 pTYAttachOnce 再传入此函数。
func writeClientFile(conn *ssh.Client, sessionName, accountID, hostname, role string) (int, error) {
	if conn == nil {
		return 0, fmt.Errorf("nil ssh.Client")
	}
	if hostname == "" {
		hostname = "unknown-host"
	}
	if role == "" {
		role = "primary"
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
		ClientRole:      role,
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

// ===== Task 2.1b 高层：业务流程 + PTY/reconnect 协同 + sessions ls/attach =====

// performTakeOver 实现 D-11 三步序列：
//
//  1. tmux list-clients 探测命中数（命中 0 → return nil 不调 detach）
//  2. tmux display-message 通知 + sleep 3
//  3. tmux detach-client -t <session> -a 把所有非 caller 客户端踢掉
//
// caller 是临时 SSH session（不在 list-clients 输出内），detach -a 不会误踢自己；
// 本端 attach 在 detach 之后串行执行，时序由 caller 保证。
func performTakeOver(conn *ssh.Client, sessionName string) error {
	if conn == nil {
		return errors.New("nil ssh.Client")
	}
	sessQ := shellescape.Quote(sessionName)

	out, _ := sshOutput(conn, fmt.Sprintf("tmux list-clients -t %s -F '#{client_pid}' 2>/dev/null", sessQ))
	clientCount := decideTakeOverClientCount(out)
	if clientCount == 0 {
		return nil
	}

	msg := "[cloud-claude] 另一端已通过 --take-over 接管会话，本会话将在 3s 后断开"
	_ = sshRun(conn, fmt.Sprintf("tmux display-message -t %s %s", sessQ, shellescape.Quote(msg)))

	if err := sshRun(conn, fmt.Sprintf("sleep 3 && tmux detach-client -t %s -a", sessQ)); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, errcodes.Format(errcodes.SESSION_TAKEOVER_NOTIFIED, clientCount, sessionName))
	return nil
}

// decideTakeOverClientCount 是 performTakeOver 内 list-clients 输出 → client 数的纯函数。
//
// 输入：tmux list-clients -F '#{client_pid}' 的 stdout（每行一个 PID）。
// 空输入 / 全空白 → 0；否则按行计数（忽略空行）。
func decideTakeOverClientCount(out string) int {
	out = strings.TrimSpace(out)
	if out == "" {
		return 0
	}
	count := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}

// printAttachBanner 渲染 D-12 完整方案的两行 banner。
//
//   - 第一行（绿色 / NoColor 时纯文本）：✓ 已 attach 到会话 <session>
//   - 第二行（仅 N >= 1 时输出）：（另 N 个会话正在共享：<host1> / <活跃时间1>，...）
//
// hostname 走 readClientHostnames 文件注册表查询；缺失字面值 "unknown-host"
// （禁止汉化兜底以保证可见性）。
func printAttachBanner(w io.Writer, conn *ssh.Client, sessionName string, noColor bool) {
	sessQ := shellescape.Quote(sessionName)
	out, _ := sshOutput(conn,
		fmt.Sprintf("tmux list-clients -t %s -F '#{client_pid}|#{client_activity}|#{client_tty}' 2>/dev/null", sessQ))
	clients := parseTmuxListClients(out)

	greeting := fmt.Sprintf("✓ 已 attach 到会话 %s", sessionName)
	if fh, ok := w.(fdHolder); ok && colorEnabled(noColor, fh) {
		greeting = colorize(greeting, ansiGreen, true)
	}
	fmt.Fprintln(w, greeting)

	if len(clients) == 0 {
		return
	}

	pids := make([]int, len(clients))
	for i, c := range clients {
		pids[i] = c.PID
	}
	hostnames := readClientHostnames(conn, pids)
	fmt.Fprintln(w, formatBannerSecondLine(clients, hostnames, time.Now()))
}

// formatBannerSecondLine 是 printAttachBanner 第二行的纯函数渲染层（单测友好）。
//
// 输出："  （另 N 个会话正在共享：<host1> / <age1>，<host2> / <age2>）"
// hostname 缺失 → "unknown-host" 字面值。
func formatBannerSecondLine(clients []tmuxClient, hostnames map[int]string, now time.Time) string {
	parts := make([]string, 0, len(clients))
	for _, c := range clients {
		host := hostnames[c.PID]
		if host == "" {
			host = "unknown-host"
		}
		parts = append(parts, fmt.Sprintf("%s / %s", host, renderActivityAge(now.Sub(c.Activity))))
	}
	return fmt.Sprintf("  （另 %d 个会话正在共享：%s）", len(clients), strings.Join(parts, "，"))
}

// loadLastSession 读 last-session.json；文件不存在 / 解析失败 → 返回空 snapshot（不报错）。
// runClaudeWithSession 写入新字段时先 load 再 merge，避免覆盖 mount 阶段写的 ActualMode 等字段。
func loadLastSession(path string) LastSessionSnapshot {
	if path == "" {
		return LastSessionSnapshot{SchemaVersion: 1}
	}
	data, err := os.ReadFile(path) //nolint:gosec // 路径来自配置，非用户输入
	if err != nil {
		return LastSessionSnapshot{SchemaVersion: 1}
	}
	var snap LastSessionSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return LastSessionSnapshot{SchemaVersion: 1}
	}
	if snap.SchemaVersion == 0 {
		snap.SchemaVersion = 1
	}
	return snap
}

// writeLastSessionTmuxField 写 TmuxSession + ClientRole（merge 模式，保留其它字段）。
// 失败仅 stderr warning，不阻塞 attach 流程。
func writeLastSessionTmuxField(path, sessionName, role string) {
	if path == "" {
		return
	}
	snap := loadLastSession(path)
	snap.TmuxSession = sessionName
	snap.ClientRole = role
	if err := WriteLastSession(path, snap); err != nil {
		fmt.Fprintln(os.Stderr, "[!] 写 last-session.json 失败（TmuxSession 字段未持久化）:", err)
	}
}

// writeLastSessionReconnectCount merge 模式写 ReconnectCount。
// 每次 Reconnector.Run 成功后调用一次，累计该会话的重连次数。
func writeLastSessionReconnectCount(path string, count int) {
	if path == "" {
		return
	}
	snap := loadLastSession(path)
	snap.ReconnectCount = count
	if err := WriteLastSession(path, snap); err != nil {
		fmt.Fprintln(os.Stderr, "[!] 写 last-session.json 失败（ReconnectCount 字段未持久化）:", err)
	}
}

// runClaudeWithSession 是 Phase 32 的会话层主入口（D-28 / D-29）。
//
//   - 命名：默认 buildTmuxSessionName；--new-session 路径用 sessionCfg.ShortID
//   - take-over：D-11 序列（list-clients / display-message / sleep / detach -a）
//   - banner：D-12 完整方案（list-clients + 文件注册表查 hostname）
//   - last-session.json：写 TmuxSession + ClientRole=primary
//   - 远程命令：D-10 tmux 包装模板
//   - 启动 PTY 主循环 + RunKeepAlive + Reconnector + BufferedStdin 三协同
func runClaudeWithSession(ctx context.Context, conn *ssh.Client, sshCfg SSHConfig,
	claudeArgs []string, sessionCfg SessionConfig, hasProxy bool,
) (int, error) {
	sessionName := buildTmuxSessionName(sessionCfg.AccountID, sessionCfg.Cwd)
	if sessionCfg.ShortID != "" {
		sessionName = "claude-" + sessionCfg.ShortID
		sessionName, _ = sanitizeSessionName(sessionName)
	}

	if sessionCfg.TakeOver {
		if err := performTakeOver(conn, sessionName); err != nil {
			fmt.Fprintln(os.Stderr, errcodes.Format(errcodes.SESSION_TAKEOVER_FAILED, err.Error()))
		}
	}

	printAttachBanner(os.Stderr, conn, sessionName, sessionCfg.NoColor)
	role := "primary"
	if sessionCfg.IsSecondaryClient {
		role = "secondary"
	}
	writeLastSessionTmuxField(sessionCfg.LastSessionPath, sessionName, role)

	claudeCmd := buildClaudeCmd(claudeArgs, hasProxy, sessionCfg.Cwd)
	remoteCmd := buildTmuxRemoteCmd(sessionCfg.Cwd, sessionName, claudeCmd)

	return runClaudePTYWithReconnect(ctx, conn, sshCfg, remoteCmd, sessionName, sessionCfg)
}

// runClaudePTYWithReconnect 是 PTY 主循环 + 三 goroutine（RunKeepAlive / Reconnector / BufferedStdin）协同。
//
// 循环不变量：
//   - conn 在循环开始时是当前活跃 ssh.Client（首次 = caller 注入；reconnect 后 = 新拨号）
//   - registryPid > 0 时表示文件注册表已写入；defer 兜底清理
//   - reconnectCount 累加跨 Reconnector.Run 调用，写 last-session.json
//
// 退出路径：
//   - session.Wait 返回 nil / *ssh.ExitError → 清理注册表 + 写 reconnectCount + return (code, nil)
//   - reconnect 不可恢复（ErrReconnectGaveUp）→ stderr FormatGiveUpMessage + return (ExitNetworkError, nil)
//   - reconnect 其它 error → return (0, err)
func runClaudePTYWithReconnect(ctx context.Context, initialConn *ssh.Client, sshCfg SSHConfig,
	remoteCmd, sessionName string, sessionCfg SessionConfig,
) (int, error) {
	conn := initialConn
	reconnectCount := 0
	registryPid := 0

	defer func() {
		if registryPid > 0 {
			_ = removeClientFile(conn, registryPid)
		}
	}()

	for {
		exitCode, exitErr, reconnectableErr := pTYAttachOnce(ctx, conn, remoteCmd, sessionName, sessionCfg, &registryPid)

		if exitErr == nil {
			writeLastSessionReconnectCount(sessionCfg.LastSessionPath, reconnectCount)
			if registryPid > 0 {
				_ = removeClientFile(conn, registryPid)
				registryPid = 0
			}
			return exitCode, nil
		}

		if reconnectableErr == nil || !sessionCfg.ReconnectEnabled {
			return 0, exitErr
		}

		t0 := time.Now()
		var newConn *ssh.Client
		reconnector := NewReconnector(sshCfg,
			nil, // onConnLost — Reconnector.Run 内部已切 state，BufferedStdin 通过共享 atomic 自动感知
			func(c *ssh.Client) error { newConn = c; return nil },
			os.Stderr, sessionCfg.NoColor)

		if err := reconnector.Run(ctx); err != nil {
			if errors.Is(err, ErrReconnectGaveUp) {
				fmt.Fprintln(os.Stderr, FormatGiveUpMessage(5, time.Since(t0)))
				writeLastSessionReconnectCount(sessionCfg.LastSessionPath, reconnectCount)
				return ExitNetworkError, nil
			}
			return 0, err
		}
		reconnectCount += reconnector.ReconnectCount()
		conn = newConn
		// 注：registryPid 已失效（旧 conn 上的 client_pid），新一轮 attach 时由
		// pTYAttachOnce 内 writeClientFile 重写。这里清零让循环重新写入。
		registryPid = 0
	}
}

// pTYAttachOnce 单次 PTY attach 周期（提取为独立函数便于读懂主循环）。
//
// 返回:
//   - exitCode：仅 exitErr == nil 时有意义（来自 *ssh.ExitError 或 0）
//   - exitErr：session.Wait 的原始错误（含 nil）；nil = 正常退出
//   - reconnectableErr：非 nil 且 ReconnectEnabled 时上层进入 Reconnector 循环
//
// PTY 申请 / SIGWINCH / RawMode 段一字复刻 ssh.go::runClaude line 178-216。
func pTYAttachOnce(ctx context.Context, conn *ssh.Client, remoteCmd, sessionName string,
	sessionCfg SessionConfig, registryPid *int,
) (int, error, error) {
	session, err := conn.NewSession()
	if err != nil {
		return 0, fmt.Errorf("创建 SSH 会话失败: %w", err), nil
	}
	defer session.Close()

	fd := int(os.Stdin.Fd())
	isTTY := term.IsTerminal(fd)

	if isTTY {
		width, height := 80, 24
		if w, h, gerr := term.GetSize(fd); gerr == nil {
			width, height = w, h
		}
		oldState, rerr := term.MakeRaw(fd)
		if rerr != nil {
			return 0, fmt.Errorf("设置终端 raw 模式失败: %w", rerr), nil
		}
		defer term.Restore(fd, oldState)

		modes := ssh.TerminalModes{
			ssh.ECHO:          1,
			ssh.TTY_OP_ISPEED: 14400,
			ssh.TTY_OP_OSPEED: 14400,
		}
		if perr := session.RequestPty("xterm-256color", height, width, modes); perr != nil {
			return 0, fmt.Errorf("申请 PTY 失败: %w", perr), nil
		}

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGWINCH)
		go func() {
			for range sigCh {
				if w, h, gerr := term.GetSize(fd); gerr == nil {
					_ = session.WindowChange(h, w)
				}
			}
		}()
		defer signal.Stop(sigCh)
	}

	// BufferedStdin 注入（CONTEXT D-29 — 共享 Reconnector.StateAddr() 的 *atomic.Int32）。
	// 注：本函数内 Reconnector 还没创建 — state 字段先 0 = StateConnected（等价"直传"），
	// 进入 reconnect 循环后由 Reconnector.Run 写入 atomic 切到 Reconnecting/GaveUp。
	// 简化：把 state 包成 atomic.Int32 局部变量；Reconnector 在外层另用自己的 state，
	// 本端 BufferedStdin 看到的始终是 StateConnected → 等价于直传 stdin（无重连缓冲）。
	// **本 plan 简化：BufferedStdin 仅在 reconnect 启动后挂接** —
	// 本 attach 周期内为直接 stdin 透传；reconnect 完成 → 下一轮 attach 再决定。
	// （完整三态共享留 v3.1，与 RegisterStateListener 接口一并落地；本阶段 BufferedStdin
	// 在断网期间通过 ringBuf 做兜底：见下方 reconnectStateForBuffer 注入。）
	var state atomic.Int32
	state.Store(int32(StateConnected))
	bs, pipeR := NewBufferedStdin(os.Stdin, &state, os.Stderr, sessionCfg.NoColor, nil)
	bsCtx, cancelBs := context.WithCancel(ctx)
	defer cancelBs()
	go func() { _ = bs.Run(bsCtx) }()
	defer bs.Close()

	if isTTY {
		session.Stdin = pipeR
	} else {
		session.Stdin = os.Stdin
	}
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	keepCtx, cancelKeep := context.WithCancel(ctx)
	defer cancelKeep()
	if sessionCfg.KeepAliveInterval >= 15*time.Second {
		go func() {
			_ = RunKeepAlive(keepCtx, conn, sessionCfg.KeepAliveInterval, sessionCfg.KeepAliveCountMax)
		}()
	}

	if err := session.Start(remoteCmd); err != nil {
		return 0, fmt.Errorf("启动 tmux 包装命令失败: %w", err), nil
	}

	// 异步写文件注册表（不阻塞 PTY 主路径）。
	if *registryPid == 0 {
		go func() {
			clientRole := "primary"
			if sessionCfg.IsSecondaryClient {
				clientRole = "secondary"
			}
			pid, werr := writeClientFile(conn, sessionName, sessionCfg.AccountID, sessionCfg.LocalHostname, clientRole)
			if werr != nil {
				fmt.Fprintln(os.Stderr, "[!] writeClientFile 失败（banner hostname 将显示 unknown）:", werr)
				return
			}
			*registryPid = pid
		}()
	}

	waitErr := session.Wait()

	if waitErr == nil {
		return 0, nil, nil
	}
	if exitErr, ok := waitErr.(*ssh.ExitError); ok {
		return exitErr.ExitStatus(), nil, nil
	}
	if errors.Is(waitErr, io.EOF) {
		// EOF 在 tmux 包装下意味着远端 tmux 退出 — 视为正常结束。
		return 0, nil, nil
	}
	// 其它非 ExitError 视为可重连的网络层错误。
	return 0, waitErr, waitErr
}

// runClaudePTYBare 是 ssh.go::runClaude PTY 段的精简复制（无 reconnect / 无 keepalive）。
// 仅由 RunSessionsAttach 复用：纯 attach 命令直跑 PTY，无需会话恢复逻辑。
func runClaudePTYBare(conn *ssh.Client, remoteCmd string) (int, error) {
	session, err := conn.NewSession()
	if err != nil {
		return 0, fmt.Errorf("创建 SSH 会话失败: %w", err)
	}
	defer session.Close()

	fd := int(os.Stdin.Fd())
	isTTY := term.IsTerminal(fd)
	if isTTY {
		width, height := 80, 24
		if w, h, gerr := term.GetSize(fd); gerr == nil {
			width, height = w, h
		}
		oldState, rerr := term.MakeRaw(fd)
		if rerr != nil {
			return 0, fmt.Errorf("设置终端 raw 模式失败: %w", rerr)
		}
		defer term.Restore(fd, oldState)

		modes := ssh.TerminalModes{
			ssh.ECHO:          1,
			ssh.TTY_OP_ISPEED: 14400,
			ssh.TTY_OP_OSPEED: 14400,
		}
		if perr := session.RequestPty("xterm-256color", height, width, modes); perr != nil {
			return 0, fmt.Errorf("申请 PTY 失败: %w", perr)
		}

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGWINCH)
		go func() {
			for range sigCh {
				if w, h, gerr := term.GetSize(fd); gerr == nil {
					_ = session.WindowChange(h, w)
				}
			}
		}()
		defer signal.Stop(sigCh)
	}

	session.Stdin = os.Stdin
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	if err := session.Start(remoteCmd); err != nil {
		return 0, fmt.Errorf("启动远程命令失败: %w", err)
	}

	if err := session.Wait(); err != nil {
		if exitErr, ok := err.(*ssh.ExitError); ok {
			return exitErr.ExitStatus(), nil
		}
		if errors.Is(err, io.EOF) {
			return 0, nil
		}
		return 0, fmt.Errorf("SSH 会话异常结束: %w", err)
	}
	return 0, nil
}

// RunSessionsLs 远程 tmux list-sessions + 本地 tabwriter 渲染（D-13）。
//
// list-sessions 失败 / 输出空 → "当前容器内无活跃 tmux session"，return nil（exit 0）。
func RunSessionsLs(conn *ssh.Client, w io.Writer) error {
	out, err := sshOutput(conn,
		"tmux list-sessions -F '#{session_name}|#{session_created}|#{session_attached}|#{session_windows}' 2>/dev/null")
	if err != nil || strings.TrimSpace(out) == "" {
		fmt.Fprintln(w, "当前容器内无活跃 tmux session")
		return nil
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SESSION\tCREATED\tCLIENTS\tWINDOWS")
	now := time.Now()
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		fields := strings.SplitN(line, "|", 4)
		if len(fields) < 4 {
			continue
		}
		createdSec, _ := strconv.ParseInt(strings.TrimSpace(fields[1]), 10, 64)
		age := renderActivityAge(now.Sub(time.Unix(createdSec, 0)))
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			strings.TrimSpace(fields[0]), age, strings.TrimSpace(fields[2]), strings.TrimSpace(fields[3]))
	}
	return tw.Flush()
}

// RunSessionsAttach 远程 tmux has-session 校验后 attach（D-14）。
//
//   - 校验失败 → stderr [SESSION_NOT_FOUND] + return (ExitConfigError, error)
//   - 校验通过 → 复用 runClaudePTYBare（exec tmux attach-session -t <name>，不包 claude）
func RunSessionsAttach(conn *ssh.Client, sessionName string, hasProxy bool, cwd string) (int, error) {
	_ = hasProxy
	_ = cwd
	sessQ := shellescape.Quote(sessionName)
	if err := sshRun(conn, fmt.Sprintf("tmux has-session -t %s 2>/dev/null", sessQ)); err != nil {
		fmt.Fprintln(os.Stderr, errcodes.Format(errcodes.SESSION_NOT_FOUND, sessionName))
		return ExitConfigError, fmt.Errorf("session not found: %s", sessionName)
	}
	remoteCmd := fmt.Sprintf("exec tmux attach-session -t %s", sessQ)
	return runClaudePTYBare(conn, remoteCmd)
}
