package cloudclaude

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// syncBuffer 是 bytes.Buffer 的并发安全包装，仅供测试使用。
// 生产代码通过 BufferedStdin.echoMu 保证 localEcho 写入的并发安全；
// 测试主 goroutine 与 bs.Run goroutine 都会访问 echo buffer，需要外部同步。
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncBuffer) Snapshot() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]byte, s.buf.Len())
	copy(out, s.buf.Bytes())
	return out
}

func (s *syncBuffer) String() string { return string(s.Snapshot()) }

// --- 命名 helpers ---

func TestBuildTmuxSessionName_Default(t *testing.T) {
	got := buildTmuxSessionName("ABCDEF12-3456-7890-abcd-ef1234567890", "/workspace/proj")
	want := "claude-abcdef12"
	if got != want {
		t.Errorf("buildTmuxSessionName = %q, want %q", got, want)
	}
}

func TestBuildTmuxSessionName_AnonFallback(t *testing.T) {
	got := buildTmuxSessionName("", "/workspace/proj")
	if !strings.HasPrefix(got, "claude-anon-") {
		t.Errorf("anon fallback expected prefix 'claude-anon-'; got %q", got)
	}
	if len(got) != len("claude-anon-")+8 {
		t.Errorf("anon fallback hash 应为 8 字符；got %q (len=%d)", got, len(got))
	}
}

func TestSanitizeSessionName_IllegalChars(t *testing.T) {
	got, warned := sanitizeSessionName("claude/abc:def")
	want := "claude_abc_def"
	if got != want {
		t.Errorf("sanitizeSessionName = %q, want %q", got, want)
	}
	if !warned {
		t.Error("非法字符场景应 warned=true")
	}
}

func TestSanitizeSessionName_TooLong(t *testing.T) {
	long := strings.Repeat("a", 50)
	got, warned := sanitizeSessionName(long)
	if len(got) != 32 {
		t.Errorf("截断后长度应 = 32, got %d", len(got))
	}
	if !warned {
		t.Error("超长场景应 warned=true")
	}
}

// --- GenerateShortSessionID ---

func TestGenerateShortSessionID_8Chars(t *testing.T) {
	for i := 0; i < 100; i++ {
		id := GenerateShortSessionID()
		if len(id) != 8 {
			t.Fatalf("第 %d 次：GenerateShortSessionID 长度 = %d, want 8 (id=%q)", i, len(id), id)
		}
		// 字符集允许 base64url：[A-Za-z0-9_-]
		for _, r := range id {
			ok := (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_'
			if !ok {
				t.Fatalf("非法字符 %q in %q", r, id)
			}
		}
	}
}

// --- parseTmuxListClients ---

func TestParseTmuxListClients_3Lines(t *testing.T) {
	out := "1234|1700000000|/dev/pts/0\n5678|1700000100|/dev/pts/1\n9999|1700000200|/dev/pts/2\n"
	got := parseTmuxListClients(out)
	if len(got) != 3 {
		t.Fatalf("期望 3 条，得 %d", len(got))
	}
	if got[0].PID != 1234 || got[1].PID != 5678 || got[2].PID != 9999 {
		t.Errorf("PID 解析错误: %+v", got)
	}
	if got[0].TTY != "/dev/pts/0" {
		t.Errorf("TTY 解析错误: %q", got[0].TTY)
	}
	if got[0].Activity.Unix() != 1700000000 {
		t.Errorf("Activity 时间错误: %v", got[0].Activity)
	}
}

func TestParseTmuxListClients_Empty(t *testing.T) {
	if got := parseTmuxListClients(""); len(got) != 0 {
		t.Errorf("空输入应返回 0 客户端，得 %d", len(got))
	}
	if got := parseTmuxListClients("\n   \n"); len(got) != 0 {
		t.Errorf("空白输入应返回 0 客户端，得 %d", len(got))
	}
}

// --- renderActivityAge ---

func TestRenderActivityAge_Thresholds(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "刚刚活跃"},
		{15 * time.Second, "刚刚活跃"},
		{29 * time.Second, "刚刚活跃"},
		{30 * time.Second, "0 分钟前活跃"},
		{5 * time.Minute, "5 分钟前活跃"},
		{59 * time.Minute, "59 分钟前活跃"},
		{1 * time.Hour, "1 小时前活跃"},
		{3 * time.Hour, "3 小时前活跃"},
	}
	for _, tc := range cases {
		got := renderActivityAge(tc.d)
		if got != tc.want {
			t.Errorf("renderActivityAge(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

// --- buildTmuxRemoteCmd ---

// 注：al.essio.dev/pkg/shellescape v1.6 仅在含特殊字符时加引号，纯 ASCII 路径
// 不会被引（POSIX 安全）。测试断言模板字面值结构 + D-10 关键词不带 quote 假设。
func TestBuildTmuxRemoteCmd_ContainsAllParts(t *testing.T) {
	got := buildTmuxRemoteCmd("/workspace/proj", "claude-abcdef12", "claude --foo")
	mustContain := []string{
		"cd /workspace/proj",
		"command -v tmux >/dev/null 2>&1",
		"exec tmux new-session -A -d -s claude-abcdef12",
		"\\; attach-session -t claude-abcdef12",
		"|| exec cd /workspace/proj && claude --foo",
	}
	for _, s := range mustContain {
		if !strings.Contains(got, s) {
			t.Errorf("buildTmuxRemoteCmd 缺少片段 %q\nfull: %s", s, got)
		}
	}
}

// 含特殊字符的 cwd 必须被 shellescape 单引号引用（防注入）。
func TestBuildTmuxRemoteCmd_SpecialCharsQuoted(t *testing.T) {
	got := buildTmuxRemoteCmd("/tmp/foo bar; rm -rf /", "claude-abc", "claude")
	if !strings.Contains(got, "'/tmp/foo bar; rm -rf /'") {
		t.Errorf("含特殊字符的 cwd 应被单引号包裹（防 shell 注入）；得: %s", got)
	}
}

// --- buildClaudeCmd ---

func TestBuildClaudeCmd_WithProxy(t *testing.T) {
	got := buildClaudeCmd([]string{"--model", "opus"}, true, "/workspace/proj")
	if !strings.Contains(got, "export PATH=/workspace/proj/.cloud-claude/bin:$PATH") {
		t.Errorf("hasProxy=true 应注入 PATH，得: %s", got)
	}
	if !strings.Contains(got, "claude --model opus") {
		t.Errorf("应含 claude args，得: %s", got)
	}
}

func TestBuildClaudeCmd_NoProxy(t *testing.T) {
	got := buildClaudeCmd([]string{"--help"}, false, "/workspace/proj")
	if strings.Contains(got, "PATH=") {
		t.Errorf("hasProxy=false 不应注入 PATH，得: %s", got)
	}
	if !strings.Contains(got, "claude --help") {
		t.Errorf("应含 claude args，得: %s", got)
	}
}

// --- parseClientRegistryDump ---

func TestParseClientRegistryDump_HappyPath(t *testing.T) {
	entry1, _ := json.Marshal(clientFileSchema{
		SchemaVersion: 1, Hostname: "alice-mbp", TmuxClientPID: 1111,
		TmuxSession: "claude-abc", AttachAtUnix: 1700000000,
		ClaudeAccountID: "uuid-1", ClientRole: "primary",
	})
	entry2, _ := json.Marshal(clientFileSchema{
		SchemaVersion: 1, Hostname: "bob-laptop", TmuxClientPID: 2222,
		TmuxSession: "claude-abc", AttachAtUnix: 1700000100,
		ClaudeAccountID: "uuid-1", ClientRole: "primary",
	})
	dump := "===1111===\n" + string(entry1) + "\n===2222===\n" + string(entry2) + "\n"
	got := parseClientRegistryDump(dump)
	if got[1111] != "alice-mbp" {
		t.Errorf("pid 1111 hostname = %q, want alice-mbp", got[1111])
	}
	if got[2222] != "bob-laptop" {
		t.Errorf("pid 2222 hostname = %q, want bob-laptop", got[2222])
	}
}

func TestParseClientRegistryDump_MissingFile(t *testing.T) {
	// pid 3333 没有对应 JSON（cat 失败 → 空段）
	dump := "===3333===\n===4444===\n{\"schema_version\":1,\"hostname\":\"x\"}\n"
	got := parseClientRegistryDump(dump)
	if _, ok := got[3333]; ok {
		t.Errorf("缺失文件的 pid 不应出现在 map 中（caller 兜底 unknown-host）")
	}
	if got[4444] != "x" {
		t.Errorf("pid 4444 应正确解析；got %+v", got)
	}
}

// ===== Task 2.1b 集成边界单测（纯函数提取，无网络依赖）=====

// --- decideTakeOverClientCount ---

func TestDecideTakeOverClientCount_Zero(t *testing.T) {
	cases := []string{"", "   ", "\n\n", "  \n  \n"}
	for _, in := range cases {
		if got := decideTakeOverClientCount(in); got != 0 {
			t.Errorf("decideTakeOverClientCount(%q) = %d, want 0", in, got)
		}
	}
}

func TestDecideTakeOverClientCount_NonZero(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"1234\n", 1},
		{"1234\n5678\n", 2},
		{"1\n2\n3\n4\n", 4},
		{"100\n\n200\n", 2}, // 空行被忽略
	}
	for _, tc := range cases {
		if got := decideTakeOverClientCount(tc.in); got != tc.want {
			t.Errorf("decideTakeOverClientCount(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// --- formatBannerSecondLine ---

func TestFormatBannerSecondLine_NoOtherClients(t *testing.T) {
	got := formatBannerSecondLine(nil, nil, time.Now())
	if !strings.Contains(got, "另 0 个会话") {
		t.Errorf("空 clients 应输出 '另 0 个'；得 %q", got)
	}
}

func TestFormatBannerSecondLine_WithRegistry(t *testing.T) {
	now := time.Now()
	clients := []tmuxClient{
		{PID: 1111, Activity: now.Add(-10 * time.Second), TTY: "/dev/pts/0"},
		{PID: 2222, Activity: now.Add(-5 * time.Minute), TTY: "/dev/pts/1"},
	}
	hostnames := map[int]string{1111: "alice-mbp", 2222: "bob-laptop"}
	got := formatBannerSecondLine(clients, hostnames, now)
	mustContain := []string{
		"另 2 个会话正在共享",
		"alice-mbp / 刚刚活跃",
		"bob-laptop / 5 分钟前活跃",
	}
	for _, s := range mustContain {
		if !strings.Contains(got, s) {
			t.Errorf("formatBannerSecondLine 缺少片段 %q\nfull: %s", s, got)
		}
	}
}

func TestFormatBannerSecondLine_MissingHostnameUnknown(t *testing.T) {
	now := time.Now()
	clients := []tmuxClient{{PID: 9999, Activity: now, TTY: "/dev/pts/3"}}
	hostnames := map[int]string{} // 空 map → 兜底 unknown-host
	got := formatBannerSecondLine(clients, hostnames, now)
	if !strings.Contains(got, "unknown-host / 刚刚活跃") {
		t.Errorf("缺失 hostname 应字面值 unknown-host；得 %q", got)
	}
}

// --- loadLastSession + writeLastSessionTmuxField ---

func TestLoadLastSession_MissingFileReturnsEmpty(t *testing.T) {
	tmp := t.TempDir() + "/no-such-file.json"
	snap := loadLastSession(tmp)
	if snap.SchemaVersion != 1 {
		t.Errorf("缺失文件应返回 SchemaVersion=1（防御写回时被强制设 0）；得 %d", snap.SchemaVersion)
	}
	if snap.TmuxSession != "" {
		t.Errorf("缺失文件不应返回任何 TmuxSession；得 %q", snap.TmuxSession)
	}
}

func TestWriteLastSessionTmuxField_PreservesExistingFields(t *testing.T) {
	tmp := t.TempDir() + "/last.json"
	// 先写一个 mount 阶段产物（含 ActualMode + ConflictCount）
	prev := LastSessionSnapshot{
		SchemaVersion:   1,
		IntendedMode:    "auto",
		ActualMode:      "full",
		ConflictCount:   3,
		ClaudeAccountID: "uuid-x",
	}
	if err := WriteLastSession(tmp, prev); err != nil {
		t.Fatalf("准备失败: %v", err)
	}

	// session 层覆盖 TmuxSession + ClientRole
	writeLastSessionTmuxField(tmp, "claude-abcdef12", "primary")

	// 读回应保留 mount 阶段字段
	got := loadLastSession(tmp)
	if got.ActualMode != "full" || got.ConflictCount != 3 || got.ClaudeAccountID != "uuid-x" {
		t.Errorf("merge 写入应保留 mount 字段；got %+v", got)
	}
	if got.TmuxSession != "claude-abcdef12" || got.ClientRole != "primary" {
		t.Errorf("session 字段未正确写入；got TmuxSession=%q ClientRole=%q",
			got.TmuxSession, got.ClientRole)
	}
}

func TestWriteLastSessionReconnectCount_MergeMode(t *testing.T) {
	tmp := t.TempDir() + "/last.json"
	writeLastSessionTmuxField(tmp, "claude-x", "primary")
	writeLastSessionReconnectCount(tmp, 5)

	got := loadLastSession(tmp)
	if got.TmuxSession != "claude-x" {
		t.Errorf("ReconnectCount 写入应保留 TmuxSession；got %q", got.TmuxSession)
	}
	if got.ReconnectCount != 5 {
		t.Errorf("ReconnectCount = %d, want 5", got.ReconnectCount)
	}
}

// TestPTYReconnect_BufferedInputFlush 验证 Phase 32 Gap #1 闭合（SC5 / REQ-F3-B）。
//
// 等价测试路径（不走真实 ssh.Client，避免 mock 完整 ssh stack）：
//  1. 手工创建 Reconnector + BufferedStdin 单例（mirror runClaudePTYWithReconnect 循环外逻辑）
//  2. state.Store(StateReconnecting) 模拟 Reconnector.Run 入口
//  3. io.Pipe 喂 "abc" 到 BufferedStdin.src
//  4. 断言 echo buffer 含 AnsiGray + 原始字符 + ringBuf 非空（"abc"）
//  5. state.Store(StateConnected) + bs.Flush() 模拟 Reconnector.Run 成功回调
//  6. 断言 pipeR 读到 "abc" + echo 含 AnsiReset + ringBuf 清空
//
// 完整 ssh.Client 拨号路径的端到端验收留 Phase 35 真机 UAT
// （32-VERIFICATION.md human_verification#1）。
func TestPTYReconnect_BufferedInputFlush(t *testing.T) {
	srcR, srcW := io.Pipe()
	defer srcW.Close()
	echo := &syncBuffer{} // 并发安全 echo writer（测试 main + bs.Run 双 goroutine 访问）

	// mirror runClaudePTYWithReconnect 外层：NewReconnector + NewBufferedStdin 共享 state。
	reconnector := NewReconnector(SSHConfig{},
		nil,
		func(c *ssh.Client) error { return nil },
		&bytes.Buffer{}, // statusWriter — 测试不调 reconnector.Run，renderStatus 不会启动
		true,            // noColor for reconnector（不影响 BufferedStdin 的 AnsiGray 输出）
	)
	// BufferedStdin noColor=false，确保 AnsiGray / AnsiReset 真实输出到 echo buffer。
	bs, pipeR := NewBufferedStdin(srcR, reconnector.StateAddr(), echo, false, reconnector.Trigger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = bs.Run(ctx) }()
	defer bs.Close()

	// Phase 1: 模拟 Reconnector.Run 入口 —— state=Reconnecting
	reconnector.StateAddr().Store(int32(StateReconnecting))

	// 喂 "abc" 到 src（异步写，因 io.Pipe 同步阻塞直到读端消费）
	go func() {
		_, _ = srcW.Write([]byte("abc"))
	}()

	// 等 bs.Run 逐字节读 + handleReconnecting 完成（buffer 累积 + echo 写入）
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		bs.ringMu.Lock()
		ringLen := len(bs.ringBuf)
		bs.ringMu.Unlock()
		if ringLen == 3 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	// 断言 1：echo buffer 含 AnsiGray（灰色未确认开头）
	echoSnap := echo.Snapshot()
	if !bytes.Contains(echoSnap, []byte(AnsiGray)) {
		t.Errorf("echo buffer 应含 AnsiGray，实际: %q", string(echoSnap))
	}

	// 断言 2：echo buffer 含原始字符 a/b/c
	for _, c := range []byte("abc") {
		if !bytes.Contains(echoSnap, []byte{c}) {
			t.Errorf("echo buffer 应含 %q，实际: %q", string(c), string(echoSnap))
		}
	}

	// 断言 3：ringBuf 非空（3 bytes "abc"）
	bs.ringMu.Lock()
	ringLen := len(bs.ringBuf)
	ringContent := string(bs.ringBuf)
	bs.ringMu.Unlock()
	if ringLen != 3 || ringContent != "abc" {
		t.Fatalf("ringBuf 应含 \"abc\"（len=3），实际 len=%d content=%q", ringLen, ringContent)
	}

	// Phase 2: 模拟 Reconnector.Run 成功回调 —— state=Connected + Flush
	reconnector.StateAddr().Store(int32(StateConnected))

	// io.Pipe Flush 会阻塞直到有读者；先把读 goroutine 起来
	readDone := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 16)
		n, _ := pipeR.Read(buf)
		readDone <- buf[:n]
	}()
	time.Sleep(50 * time.Millisecond) // 让读 goroutine 就绪

	if err := bs.Flush(); err != nil {
		t.Fatalf("Flush 失败: %v", err)
	}

	// 断言 4：pipeR 读到 "abc"（按序 / 无丢字）
	select {
	case got := <-readDone:
		if string(got) != "abc" {
			t.Errorf("pipeR 读到 %q，期望 \"abc\"", string(got))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("pipeR 未在 2s 内读到数据 —— Flush 路径未生效（Gap #1 未闭合）")
	}

	// 断言 5：Flush 后 ringBuf 清空
	bs.ringMu.Lock()
	if len(bs.ringBuf) != 0 {
		t.Errorf("Flush 后 ringBuf 应清空，实际 len=%d", len(bs.ringBuf))
	}
	bs.ringMu.Unlock()

	// 断言 6：Flush 后 echo buffer 含 AnsiReset（closeGrayIfOpen 被调用，灰色关闭）
	echoFinal := echo.Snapshot()
	if !bytes.Contains(echoFinal, []byte(AnsiReset)) {
		t.Errorf("Flush 后 echo buffer 应含 AnsiReset，实际: %q", string(echoFinal))
	}
}
