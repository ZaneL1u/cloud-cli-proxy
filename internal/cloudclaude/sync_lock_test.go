package cloudclaude

import (
	"testing"
)

// TestAcquireSyncLock_AnonReturnsNoop 验证 D-19：accountID == "" 走 noop 路径。
// 不需要真实 SSH 连接（nil conn 也允许）。
func TestAcquireSyncLock_AnonReturnsNoop(t *testing.T) {
	release, err := AcquireSyncLock(nil, "")
	if err != nil {
		t.Fatalf("anon 路径不应返回错误，得 %v", err)
	}
	if release == nil {
		t.Fatal("anon 路径必须返回非 nil noop release")
	}
	release()
}

// TestAcquireSyncLock_NilConnNonAnonErrors 验证 conn==nil 且 accountID 非空时
// 显式返回错误，不 panic。
func TestAcquireSyncLock_NilConnNonAnonErrors(t *testing.T) {
	release, err := AcquireSyncLock(nil, "test-account")
	if err == nil {
		t.Fatal("nil conn + 非空 accountID 应返回错误")
	}
	if release != nil {
		t.Error("出错时 release 应为 nil")
	}
}

func TestParseLastInt_SingleLine(t *testing.T) {
	if got := parseLastInt("12345\n"); got != 12345 {
		t.Errorf("got %d, want 12345", got)
	}
}

func TestParseLastInt_MultiLineLastWins(t *testing.T) {
	in := "lock acquired\n\n9876\n"
	if got := parseLastInt(in); got != 9876 {
		t.Errorf("got %d, want 9876（多行场景应取末尾合法数字）", got)
	}
}

func TestParseLastInt_NoNumber(t *testing.T) {
	if got := parseLastInt("error: flock not found\n"); got != 0 {
		t.Errorf("got %d, want 0（无数字时返回 0）", got)
	}
}

func TestParseLastInt_EmptyInput(t *testing.T) {
	if got := parseLastInt(""); got != 0 {
		t.Errorf("got %d, want 0（空输入）", got)
	}
}

func TestParseLastInt_NegativePIDIgnored(t *testing.T) {
	if got := parseLastInt("-1\n"); got != 0 {
		t.Errorf("got %d, want 0（负数不视为合法 PID）", got)
	}
}

func TestParseLastInt_LargePID(t *testing.T) {
	if got := parseLastInt("123456\n"); got != 123456 {
		t.Errorf("got %d, want 123456", got)
	}
}

func TestParseLastInt_TrailingWhitespace(t *testing.T) {
	if got := parseLastInt("  4242  \n   \n"); got != 4242 {
		t.Errorf("got %d, want 4242（应跳过末尾空行 + 修剪两侧空白）", got)
	}
}
