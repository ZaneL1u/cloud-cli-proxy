package cloudclaude

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIgnoreMatcher_Basic(t *testing.T) {
	root := t.TempDir()
	patterns := []string{
		"node_modules/",
		"*.log",
		"/build",
		"core",
		"web/**/vision-models/",
		"!web/apps/editor/public/vision-models/small.glb",
	}
	m := NewIgnoreMatcher(root, patterns)

	cases := []struct {
		rel     string
		isDir   bool
		ignored bool
		desc    string
	}{
		{"node_modules", true, true, "dir-only 顶层命中"},
		{"sub/node_modules", true, true, "dir-only 子层命中"},
		{"sub/node_modules", false, false, "dir-only 对文件不匹配"},
		{"app.log", false, true, "扩展名 glob"},
		{"nested/app.log", false, true, "扩展名 glob 递归"},
		{"build", true, true, "anchored 顶层"},
		{"sub/build", true, false, "anchored 不在顶层不命中"},
		{"core", false, true, "核心文件名"},
		{"sub/core", false, true, "核心文件名递归"},
		{"web/apps/editor/public/vision-models", true, true, "中缀 ** 命中目录"},
		// 注：被忽略目录下的文件由 WalkDir SkipDir 天然过滤；matcher 对单个文件
		// 只判断文件本身是否命中规则，这里 "big.glb" 没被任何规则匹配故 false。
		{"web/apps/editor/public/vision-models/small.glb", false, false, "negation 对单文件路径本身检查"},
		{"README.md", false, false, "未命中任何规则"},
	}
	for _, tc := range cases {
		abs := filepath.Join(root, tc.rel)
		got := m.IsIgnored(abs, tc.isDir)
		if got != tc.ignored {
			t.Errorf("[%s] IsIgnored(%q, dir=%v) = %v, want %v", tc.desc, tc.rel, tc.isDir, got, tc.ignored)
		}
	}
}

func TestIgnoreMatcher_EmptyAndNil(t *testing.T) {
	// nil matcher 不 panic，全返回 false
	var m *IgnoreMatcher
	if m.IsIgnored("/any/path", false) {
		t.Error("nil matcher should return false")
	}

	// 空规则 matcher 也不命中
	m = NewIgnoreMatcher(t.TempDir(), nil)
	if m.IsIgnored("/any/path", false) {
		t.Error("empty matcher should return false")
	}
}

func TestIgnoreMatcher_OutsideRoot(t *testing.T) {
	root := t.TempDir()
	m := NewIgnoreMatcher(root, []string{"*.log"})
	// root 外的路径不应被命中（保守，避免误伤）
	if m.IsIgnored("/tmp/other/app.log", false) {
		t.Error("path outside root should not be ignored")
	}
}

func TestLoadProjectIgnore_ReadsBothFiles(t *testing.T) {
	root := t.TempDir()
	gi := "# comment\n\nnode_modules/\n*.log\n"
	cci := "!app.log\n"
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(gi), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".cloud-claude-ignore"), []byte(cci), 0o644); err != nil {
		t.Fatal(err)
	}
	patterns := LoadProjectIgnore(root)
	want := []string{"node_modules/", "*.log", "!app.log"}
	if len(patterns) != len(want) {
		t.Fatalf("got %d patterns, want %d: %v", len(patterns), len(want), patterns)
	}
	for i, w := range want {
		if patterns[i] != w {
			t.Errorf("patterns[%d] = %q, want %q", i, patterns[i], w)
		}
	}
}

func TestLoadProjectIgnore_MissingFilesOK(t *testing.T) {
	root := t.TempDir()
	patterns := LoadProjectIgnore(root)
	if len(patterns) != 0 {
		t.Errorf("missing files should yield empty; got %v", patterns)
	}
}

func TestDefaultBinaryIgnore_MatchesMediaFiles(t *testing.T) {
	root := t.TempDir()
	m := NewIgnoreMatcher(root, DefaultBinaryIgnorePatterns)

	shouldIgnore := []string{
		"web/apps/editor/public/vision-models/液晶柜.glb",
		"assets/background/1.jpg",
		"public/demo.mp4",
		"server/模板.dxf",
		"dist/release.zip",
		"storage/data.sqlite3",
		"docs/guide.pdf",
		"core",
		"core.1234",
		"bundle.wasm",
		"fonts/Inter.woff2",
	}
	for _, rel := range shouldIgnore {
		if !m.IsIgnored(filepath.Join(root, rel), false) {
			t.Errorf("expected default binary ignore to match %q", rel)
		}
	}

	shouldNotIgnore := []string{
		"main.go",
		"README.md",
		"src/app.ts",
		"icon.svg", // SVG 是文本，不在黑名单
		"favicon.ico",
		"script.js",
		"out.bin", // 刻意不入黑名单，避免 C 工程 target 误伤
		"obj/main.obj",
	}
	for _, rel := range shouldNotIgnore {
		if m.IsIgnored(filepath.Join(root, rel), false) {
			t.Errorf("default binary ignore should NOT match %q", rel)
		}
	}
}

func TestDefaultBinaryIgnore_NegationInUserIgnoreWins(t *testing.T) {
	root := t.TempDir()
	// 默认黑名单会命中 *.glb；用户写 !special.glb 应该把 special.glb 救回
	patterns := append([]string{}, DefaultBinaryIgnorePatterns...)
	patterns = append(patterns, "!special.glb")
	m := NewIgnoreMatcher(root, patterns)

	if !m.IsIgnored(filepath.Join(root, "big.glb"), false) {
		t.Error("big.glb should still be ignored by default")
	}
	if m.IsIgnored(filepath.Join(root, "special.glb"), false) {
		t.Error("special.glb should be rescued by !special.glb negation")
	}
}

func TestLoadMountIgnorePatterns_EnvDisable(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("*.log\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 默认启用：含黑名单 + 用户 gitignore
	t.Setenv("CLOUD_CLAUDE_NO_DEFAULT_IGNORE", "")
	got := LoadMountIgnorePatterns(root)
	if len(got) <= 1 {
		t.Fatalf("expected default ignore + user rules; got %v", got)
	}
	found := false
	for _, p := range got {
		if p == "*.glb" {
			found = true
			break
		}
	}
	if !found {
		t.Error("default glb pattern missing when env unset")
	}
	if got[len(got)-1] != "*.log" {
		t.Errorf("user gitignore should be appended last; got tail %q", got[len(got)-1])
	}

	// env=1 时禁用默认黑名单，仅用户 gitignore
	t.Setenv("CLOUD_CLAUDE_NO_DEFAULT_IGNORE", "1")
	got = LoadMountIgnorePatterns(root)
	if len(got) != 1 || got[0] != "*.log" {
		t.Errorf("env=1 should disable defaults; got %v", got)
	}
}

func TestIgnoreMatcher_DotStarEdgeCase(t *testing.T) {
	// `.env*` 常见于 gitignore，应该匹配 .env, .env.local 但不匹配 .environment/README
	root := t.TempDir()
	m := NewIgnoreMatcher(root, []string{".env*"})
	cases := map[string]bool{
		".env":       true,
		".env.local": true,
		"sub/.env":   true,
		"README.md":  false,
	}
	for rel, want := range cases {
		if got := m.IsIgnored(filepath.Join(root, rel), false); got != want {
			t.Errorf("IsIgnored(%q) = %v, want %v", rel, got, want)
		}
	}
}
