package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

var (
	explainBinOnce sync.Once
	explainBinPath string
	explainBinErr  error
)

// buildOnceExplainBin 在当前 go test 进程内只编译一次，避免复用跨测试进程的陈旧 /tmp 二进制。
func buildOnceExplainBin(t *testing.T) string {
	t.Helper()
	explainBinOnce.Do(func() {
		tmpDir, err := os.MkdirTemp("", "cloud-claude-explain-test-*")
		if err != nil {
			explainBinErr = err
			return
		}
		explainBinPath = tmpDir + "/cloud-claude"
		cmd := exec.Command("go", "build", "-o", explainBinPath, "./")
		cmd.Dir = "."
		if out, err := cmd.CombinedOutput(); err != nil {
			explainBinErr = errors.New(err.Error() + "\n" + string(out))
		}
	})
	if explainBinErr != nil {
		t.Fatalf("编译 cloud-claude 失败: %v", explainBinErr)
	}
	return explainBinPath
}

func runExplainBin(t *testing.T, bin string, args ...string) (exitCode int, stdout, stderr string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, bin, args...)
	var outBuf, errBuf bytes.Buffer
	c.Stdout = &outBuf
	c.Stderr = &errBuf
	err := c.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), outBuf.String(), errBuf.String()
	}
	if err != nil {
		t.Logf("cloud-claude explain 执行错误: %v", err)
		return -1, outBuf.String(), errBuf.String()
	}
	return 0, outBuf.String(), errBuf.String()
}

// TestExplain_KnownCode_Exit0 — 覆盖 REQ-F8-C / ROADMAP §Phase 34 SC#8：
// cloud-claude explain MOUNT_MUTAGEN_VERSION_SKEW 必须 exit 0 + stdout 含错误码字面量 + "建议:" 子串。
func TestExplain_KnownCode_Exit0(t *testing.T) {
	bin := buildOnceExplainBin(t)
	code, stdout, stderr := runExplainBin(t, bin, "explain", "MOUNT_MUTAGEN_VERSION_SKEW")
	if code != 0 {
		t.Fatalf("known code 应 exit 0，实际 %d；stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "MOUNT_MUTAGEN_VERSION_SKEW") {
		t.Errorf("stdout 未包含错误码字面量: %q", stdout)
	}
	if !strings.Contains(stdout, "建议:") {
		t.Errorf("stdout 未包含 '建议:' 子串（Format 两段 + NextAction）: %q", stdout)
	}
	if !strings.Contains(stdout, "详细说明：") {
		t.Errorf("stdout 未包含 '详细说明：' 段（段 2 锚点）: %q", stdout)
	}
}

// TestExplain_UnknownCode_Exit4 — 覆盖 CONTEXT D-17：
// cloud-claude explain FAKE_CODE_X 必须 exit 4 + stderr 含 "未找到错误码"。
func TestExplain_UnknownCode_Exit4(t *testing.T) {
	bin := buildOnceExplainBin(t)
	code, _, stderr := runExplainBin(t, bin, "explain", "FAKE_CODE_X")
	if code != 4 {
		t.Fatalf("unknown code 应 exit 4 (exitConfigError)，实际 %d", code)
	}
	if !strings.Contains(stderr, "未找到错误码") {
		t.Errorf("stderr 未包含 '未找到错误码' 字面量: %q", stderr)
	}
	if !strings.Contains(stderr, "FAKE_CODE_X") {
		t.Errorf("stderr 未回显原输入 FAKE_CODE_X: %q", stderr)
	}
}

// TestExplain_CaseSensitive_LowerCaseUnknown — 覆盖 RESEARCH §8.4 / PITFALLS C8：
// cloud-claude explain mount_mutagen_version_skew（小写）必须 exit 4，禁止自动修正。
func TestExplain_CaseSensitive_LowerCaseUnknown(t *testing.T) {
	bin := buildOnceExplainBin(t)
	code, _, stderr := runExplainBin(t, bin, "explain", "mount_mutagen_version_skew")
	if code != 4 {
		t.Fatalf("lower-case 输入应 exit 4（禁止自动修正），实际 %d；stderr=%q", code, stderr)
	}
}

func TestExplain_MountRequireGitRepo_Exit0_MinLen(t *testing.T) {
	bin := buildOnceExplainBin(t)
	code, stdout, stderr := runExplainBin(t, bin, "explain", "MOUNT_REQUIRE_GIT_REPO")
	if code != 0 {
		t.Fatalf("known code 应 exit 0，实际 %d；stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "MOUNT_REQUIRE_GIT_REPO") {
		t.Errorf("stdout 未包含错误码字面量: %q", stdout)
	}
	if n := utf8.RuneCountInString(stdout); n < 200 {
		t.Errorf("stdout 字符数 %d < 200（D-18 要求 ExtendedExplanations ≥200 中文字符）", n)
	}
}

func TestExplain_MountOversizedFileSkipped_Exit0_MinLen(t *testing.T) {
	bin := buildOnceExplainBin(t)
	code, stdout, stderr := runExplainBin(t, bin, "explain", "MOUNT_OVERSIZED_FILE_SKIPPED")
	if code != 0 {
		t.Fatalf("known code 应 exit 0，实际 %d；stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "MOUNT_OVERSIZED_FILE_SKIPPED") {
		t.Errorf("stdout 未包含错误码字面量: %q", stdout)
	}
	if n := utf8.RuneCountInString(stdout); n < 200 {
		t.Errorf("stdout 字符数 %d < 200", n)
	}
}
