//go:build e2e

package harness

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func assertDirExists(t *testing.T, p string) {
	t.Helper()
	info, err := os.Stat(p)
	if err != nil {
		t.Fatalf("stat %s: %v", p, err)
	}
	if !info.IsDir() {
		t.Fatalf("not a dir: %s", p)
	}
}

func mustReadFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return string(b)
}

// findFirstChildDir 返回 parent 下第一个子目录的绝对路径（按 ReadDir 顺序）。
// 用于解析 Collect 写出的 <baseDir>/<sanitizedName>/<timestamp>/。
func findFirstChildDir(t *testing.T, parent string) string {
	t.Helper()
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatalf("readdir %s: %v", parent, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			return filepath.Join(parent, e.Name())
		}
	}
	t.Fatalf("no child dir under %s", parent)
	return ""
}

// TestArtifactDumper_CollectCreates5Subdirs：Collect 后 baseDir 下有
// <sanitizedName>/<timestamp>/ + 5 个子目录 + 5 份 README.md（含 Phase 52 字样）。
func TestArtifactDumper_CollectCreates5Subdirs(t *testing.T) {
	tempDir := t.TempDir()
	d := NewArtifactDumper(nil, tempDir)
	dir, err := d.Collect(context.Background(), "TestCase/sub")
	if err != nil {
		t.Fatalf("Collect err: %v", err)
	}

	// dir 是绝对路径；应当符合 <tempDir>/TestCase_sub/<timestamp>/ 结构
	rel, err := filepath.Rel(tempDir, dir)
	if err != nil {
		t.Fatalf("rel %s: %v", dir, err)
	}
	parts := strings.Split(rel, string(os.PathSeparator))
	if len(parts) != 2 {
		t.Fatalf("dir layout unexpected: rel=%q (parts=%d)", rel, len(parts))
	}
	if parts[0] != "TestCase_sub" {
		t.Fatalf("sanitized name unexpected: %q", parts[0])
	}

	for _, sub := range ArtifactSubdirs {
		subDir := filepath.Join(dir, sub)
		assertDirExists(t, subDir)

		readmePath := filepath.Join(subDir, "README.md")
		content := mustReadFile(t, readmePath)
		if !strings.Contains(content, "Phase 52") {
			t.Fatalf("README %s missing 'Phase 52' fragment: %q", subDir, content[:min(80, len(content))])
		}
	}
}

// TestArtifactDumper_CollectIsIdempotent：连续两次 Collect 同一 name，
// 第二次不报错；README 不被重复覆盖（mtime 间隔近似不变即可）。
func TestArtifactDumper_CollectIsIdempotent(t *testing.T) {
	tempDir := t.TempDir()
	d := NewArtifactDumper(nil, tempDir)

	dir1, err := d.Collect(context.Background(), "Idempotent")
	if err != nil {
		t.Fatalf("first Collect err: %v", err)
	}
	readmePath := filepath.Join(dir1, "logs", "README.md")
	info1, err := os.Stat(readmePath)
	if err != nil {
		t.Fatalf("stat readme1: %v", err)
	}

	// 第二次：Collect 时间戳粒度 1 秒，可能创建新目录或落到同一个；
	// 关键断言是「两次都不报错」+「第一份 README 仍存在且未被覆盖」。
	dir2, err := d.Collect(context.Background(), "Idempotent")
	if err != nil {
		t.Fatalf("second Collect err: %v", err)
	}
	if dir2 == "" {
		t.Fatalf("second Collect returned empty dir")
	}

	info2, err := os.Stat(readmePath)
	if err != nil {
		t.Fatalf("stat readme2: %v", err)
	}
	if !info1.ModTime().Equal(info2.ModTime()) {
		t.Fatalf("README rewritten unexpectedly: mtime1=%v mtime2=%v", info1.ModTime(), info2.ModTime())
	}
}

// TestArtifactDumper_DefaultBaseDirIsProjectRelative：未设环境变量时
// defaultBaseDir() 返回项目根相对路径（"./out/e2e-artifacts"）。
func TestArtifactDumper_DefaultBaseDirIsProjectRelative(t *testing.T) {
	t.Setenv(EnvArtifactBaseDir, "") // 清空环境变量
	got := defaultBaseDir()
	if got != DefaultArtifactBaseDir {
		t.Fatalf("defaultBaseDir = %q, want %q", got, DefaultArtifactBaseDir)
	}
	if strings.HasPrefix(got, "/") {
		t.Fatalf("default base dir must be project-relative, got absolute %q", got)
	}
}

// TestArtifactDumper_EnvOverrideRespected：环境变量覆盖默认值。
// 用 t.TempDir() 拼接子串验证（不硬编码 /Users/... 或 /home/...）。
func TestArtifactDumper_EnvOverrideRespected(t *testing.T) {
	override := filepath.Join(t.TempDir(), "e2e-override")
	t.Setenv(EnvArtifactBaseDir, override)
	got := defaultBaseDir()
	if got != override {
		t.Fatalf("defaultBaseDir env override failed: got %q want %q", got, override)
	}
}

// TestArtifactDumper_OnWaitForTimeoutWritesNoteFile：超时即 dump，
// system/wait-timeout.txt 内容含 name=cp.health 与 last_err=boom。
func TestArtifactDumper_OnWaitForTimeoutWritesNoteFile(t *testing.T) {
	tempDir := t.TempDir()
	d := NewArtifactDumper(nil, tempDir)

	if err := d.OnWaitForTimeout(context.Background(), "cp.health", errors.New("boom")); err != nil {
		t.Fatalf("OnWaitForTimeout err: %v", err)
	}

	// 找到该次 Collect 写出的目录（<tempDir>/cp.health/<timestamp>/）
	nameDir := filepath.Join(tempDir, "cp.health")
	tsDir := findFirstChildDir(t, nameDir)
	notePath := filepath.Join(tsDir, "system", "wait-timeout.txt")

	content := mustReadFile(t, notePath)
	if !strings.Contains(content, "name=cp.health") {
		t.Fatalf("wait-timeout.txt missing name fragment: %q", content)
	}
	if !strings.Contains(content, "last_err=boom") {
		t.Fatalf("wait-timeout.txt missing last_err fragment: %q", content)
	}
}

// TestBaseSuite_TearDownTestSkipsOnSuccess：用例成功时 TearDownTest 不应
// 写任何 artifact 文件到 dumper.baseDir。
//
// 这里直接操作 BaseSuite struct（不通过 testify suite.Run），用 setT helper
// 注入 testing.T。`s.T().Failed()` 在 t 没失败时返回 false。
func TestBaseSuite_TearDownTestSkipsOnSuccess(t *testing.T) {
	tempDir := t.TempDir()
	bs := &BaseSuite{}
	bs.SetT(t)
	bs.SetupSuite()

	bs.SetArtifactDumper(NewArtifactDumper(nil, tempDir))

	// t 当前没失败（也不会失败，本测试自己跑得绿），TearDownTest 应不写 disk
	bs.TearDownTest()

	entries, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("readdir tempDir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("TearDownTest 在用例成功时不应写 artifact，got entries: %v", entries)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
