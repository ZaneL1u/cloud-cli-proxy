package errcodes

import (
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestErrcodesRegistry(t *testing.T) {
	reg := Registry()

	// Phase 34 D-21：17 条新 + 25 条既有 = ≥ 42 条；下限放宽到 30 留余量。
	if len(reg) < 30 {
		t.Fatalf("注册表条目不足：want >= 30, got %d", len(reg))
	}

	// 命名规范：^[A-Z]+_[A-Z]+_[A-Z0-9]+(_[A-Z0-9]+)*$，允许 3+ 段。
	re := regexp.MustCompile(`^[A-Z]+_[A-Z]+_[A-Z0-9]+(_[A-Z0-9]+)*$`)

	seen := map[Code]struct{}{}
	for code, e := range reg {
		if _, dup := seen[code]; dup {
			t.Errorf("发现重复 code: %s", code)
		}
		seen[code] = struct{}{}

		if string(e.Code) != string(code) {
			t.Errorf("entry.Code (%s) 与 map key (%s) 不一致", e.Code, code)
		}

		if !re.MatchString(string(code)) {
			t.Errorf("code %q 不符合命名规范 ^[A-Z]+_[A-Z]+_[A-Z0-9]+(_[A-Z0-9]+)*$", code)
		}

		if e.Message == "" {
			t.Errorf("code %q Message 不应为空", code)
		}
		if e.NextAction == "" {
			t.Errorf("code %q NextAction 不应为空", code)
		}
		if n := utf8.RuneCountInString(e.NextAction); n > 80 {
			t.Errorf("code %q NextAction 长度 %d > 80 runes: %q", code, n, e.NextAction)
		}
	}
}

func TestFormat_Render(t *testing.T) {
	got := Format(MOUNT_HOT_SYNC_FAILED, "SFTP 连接断开")
	want := "[MOUNT_HOT_SYNC_FAILED] 热同步失败: SFTP 连接断开\n  建议: 检查当前目录可读写、远端 staging 路径权限，或回退到 sshfs-only"
	if got != want {
		t.Errorf("Format 输出不匹配模板：\nwant:\n%s\n\ngot:\n%s", want, got)
	}
}

func TestFormat_UnknownCode(t *testing.T) {
	got := Format("FAKE_CODE_X")
	if !strings.Contains(got, "(unknown code)") {
		t.Errorf("未注册 code 应输出 (unknown code)，实际: %q", got)
	}
	if !strings.Contains(got, "FAKE_CODE_X") {
		t.Errorf("未注册 code 输出应包含原 code 字面量，实际: %q", got)
	}
}

func TestLookup_Hit(t *testing.T) {
	e, ok := Lookup(NET_OAUTH_EXPIRED)
	if !ok {
		t.Fatalf("Lookup(NET_OAUTH_EXPIRED) 应命中，但返回 false")
	}
	if e.Severity != SeverityFatal {
		t.Errorf("NET_OAUTH_EXPIRED Severity want SeverityFatal, got %v", e.Severity)
	}
	if !strings.Contains(e.Message, "OAuth") {
		t.Errorf("NET_OAUTH_EXPIRED Message 应包含 OAuth，实际: %q", e.Message)
	}
}

func TestLookup_Miss(t *testing.T) {
	if _, ok := Lookup("DOES_NOT_EXIST_XX"); ok {
		t.Errorf("Lookup 未注册 code 应返回 false")
	}
}

func TestPhase41CodesRegistered(t *testing.T) {
	// Phase 41 新增 6 个错误码必须全部已注册。
	codes := []Code{
		SSH_VSCODE_SERVER_NOT_RUNNING,
		SSH_VSCODE_PORT_NOT_LISTENING,
		SSH_FORWARDING_SOCKET_MISSING,
		SSH_FORWARDING_BLOCKED,
		DISK_VSCODE_SERVER_WARN,
		DISK_VSCODE_SERVER_BLOAT,
	}
	for _, c := range codes {
		e, ok := Lookup(c)
		if !ok {
			t.Errorf("Phase 41 code %q 未注册", c)
			continue
		}
		if e.Code != c {
			t.Errorf("Lookup(%q).Code = %q, want %q", c, e.Code, c)
		}
	}
}
