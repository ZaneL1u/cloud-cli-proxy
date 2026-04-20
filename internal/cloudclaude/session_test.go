package cloudclaude

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

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
