// Package cloudclaude — gitignore-style 忽略规则解析。
//
// 用于把用户工程根目录下的 .gitignore（以及可选的 .cloud-claude-ignore）喂给
// 两个路径：
//  1. 前置体积检查 localDuBytes —— 命中 ignore 的文件不计入 50MB 阈值
//  2. mutagen-defaults.yml 生成 —— 命中 ignore 的 pattern 追加到 mutagen sync
//     的 ignore 列表，mutagen 热同步不会把这些文件推到 /workspace-hot
//
// 设计目标：覆盖 gitignore 常见 95% pattern，不引入新依赖，行为可被测试锁住。
// 显式不支持的 gitignore 特性：
//   - 反斜杠转义（`\#` / `\ `）
//   - 嵌套 .gitignore 文件（只读根目录那份）
//   - 按文件 mode 匹配

package cloudclaude

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ignoreRule 是一条编译后的 gitignore 规则。
type ignoreRule struct {
	re      *regexp.Regexp
	negate  bool
	dirOnly bool
}

// IgnoreMatcher 按 gitignore 顺序规则判断路径是否被忽略。
// 顺序遍历 rules，后出现的规则覆盖前面的（gitignore 语义）。
type IgnoreMatcher struct {
	root  string
	rules []ignoreRule
}

// DefaultBinaryIgnorePatterns 是 cloud-claude 内置的"媒体/二进制"扩展名黑名单。
//
// 设计要点：
//   - 按扩展名精确匹配（gitignore pattern），不做文件大小判定，契合 mutagen yaml
//     ignore.paths 语法、不需要 per-file stat
//   - 命中文件走 sshfs 冷层按需拉取（Full 模式下 /workspace-cold 里仍可访问），
//     不会进入 mutagen hot sync 的首次全量传输
//   - 歧义扩展名（`*.bin` / `*.obj` 等）刻意不入表，避免 C 工程 target object
//     / 通用二进制 blob 被误伤；此类文件用户应在 .gitignore 自行处理
//   - 用户在 .gitignore 或 .cloud-claude-ignore 用 `!foo.png` 之类 negation 能
//     把具体文件救回到 mutagen hot 路径（gitignore 顺序覆盖语义）
//
// 如果整体禁用，设置环境变量 CLOUD_CLAUDE_NO_DEFAULT_IGNORE=1。
var DefaultBinaryIgnorePatterns = []string{
	// ===== cloud-claude 本地会话临时文件（不应进入热同步）=====
	".cloud-claude/",
	// ===== 3D 模型 =====
	"*.glb", "*.gltf", "*.fbx", "*.stl", "*.blend", "*.dae", "*.3ds", "*.ply",
	// ===== 视频 =====
	"*.mp4", "*.webm", "*.mov", "*.avi", "*.mkv", "*.flv", "*.wmv",
	"*.m4v", "*.mpg", "*.mpeg",
	// ===== 音频 =====
	"*.mp3", "*.wav", "*.flac", "*.aac", "*.ogg", "*.m4a", "*.wma", "*.opus",
	// ===== 光栅图片（SVG / ICO 是矢量或小体积，保留走 mutagen）=====
	"*.png", "*.jpg", "*.jpeg", "*.gif", "*.bmp", "*.tiff", "*.tif",
	"*.heic", "*.heif", "*.webp", "*.avif", "*.psd", "*.ai",
	// ===== 字体 =====
	"*.woff", "*.woff2", "*.ttf", "*.otf", "*.eot",
	// ===== 压缩 / 打包 =====
	"*.zip", "*.tar", "*.gz", "*.bz2", "*.xz", "*.7z", "*.rar",
	"*.tgz", "*.tbz", "*.tbz2", "*.jar", "*.war", "*.ear",
	// ===== 大型 WASM / 系统镜像 =====
	"*.wasm", "*.dmg", "*.iso", "*.img",
	// ===== 数据库文件 =====
	"*.sqlite", "*.sqlite3", "*.sqlite-journal", "*.mdb",
	// ===== CAD / 工程 =====
	"*.dxf", "*.dwg", "*.step", "*.stp", "*.igs", "*.iges",
	// ===== 办公文档（二进制，不适合 mutagen 双向同步）=====
	"*.pdf", "*.doc", "*.docx", "*.ppt", "*.pptx", "*.xls", "*.xlsx",
	// ===== 进程崩溃 core dump（claude / Node / Python 常见产物）=====
	"core", "core.*",
}

// LoadProjectIgnore 读 cwd 下的 .gitignore / .cloud-claude-ignore，
// 合并成原始 pattern 列表（去除空行和 `#` 注释后的有效行）。
// 两个文件都不存在时返回空列表（nil），不是错误。
//
// .cloud-claude-ignore 优先级在 .gitignore 之后读入，所以对同一目标的
// negation（`!xxx`）能覆盖 .gitignore 的结论。
func LoadProjectIgnore(cwd string) []string {
	var out []string
	for _, name := range []string{".gitignore", ".cloud-claude-ignore"} {
		lines, err := readIgnoreFile(filepath.Join(cwd, name))
		if err != nil {
			continue
		}
		out = append(out, lines...)
	}
	return out
}

// LoadMountIgnorePatterns 返回 mutagen 挂载流程实际使用的合并 ignore 列表：
//
//	[DefaultBinaryIgnorePatterns...] + [用户 .gitignore + .cloud-claude-ignore]
//
// 默认黑名单在前、用户规则在后，后出现规则覆盖语义使得用户写 `!specific.png`
// 能够把被黑名单命中的具体文件救回 mutagen 热同步。
//
// 环境变量 CLOUD_CLAUDE_NO_DEFAULT_IGNORE=1 时完全不启用默认黑名单，等价于
// 旧行为（仅用户 .gitignore）。用于排查黑名单误伤或需要同步特殊二进制的场景。
func LoadMountIgnorePatterns(cwd string) []string {
	var out []string
	if os.Getenv("CLOUD_CLAUDE_NO_DEFAULT_IGNORE") != "1" {
		out = append(out, DefaultBinaryIgnorePatterns...)
	}
	out = append(out, LoadProjectIgnore(cwd)...)
	return out
}

func readIgnoreFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), " \t\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out, sc.Err()
}

// NewIgnoreMatcher 编译 patterns。无效行被丢弃（不影响其他规则）。
// root 是工程根目录绝对路径，供 IsIgnored 把入参 absPath 转相对路径。
func NewIgnoreMatcher(root string, patterns []string) *IgnoreMatcher {
	m := &IgnoreMatcher{root: root}
	for _, raw := range patterns {
		if r, ok := compileIgnoreRule(raw); ok {
			m.rules = append(m.rules, r)
		}
	}
	return m
}

// IsIgnored 判断 absPath（允许是文件或目录）相对 root 是否被忽略。
// 若 absPath 不在 root 之下，返回 false（保守，不误伤外部路径）。
func (m *IgnoreMatcher) IsIgnored(absPath string, isDir bool) bool {
	if m == nil || len(m.rules) == 0 {
		return false
	}
	rel, err := filepath.Rel(m.root, absPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return false
	}
	// gitignore 用正斜杠匹配；WalkDir 在 Windows 可能给反斜杠。
	rel = filepath.ToSlash(rel)
	return m.IsIgnoredRel(rel, isDir)
}

// IsIgnoredRel 直接按相对 root 的 slash path 判断是否忽略。
// 供远端扫描、虚拟路径映射等场景复用，不要求调用方提供本地绝对路径。
func (m *IgnoreMatcher) IsIgnoredRel(rel string, isDir bool) bool {
	if m == nil || len(m.rules) == 0 {
		return false
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || rel == "" {
		return false
	}
	ignored := false
	for _, r := range m.rules {
		if r.dirOnly && !isDir {
			continue
		}
		if r.re.MatchString(rel) {
			ignored = !r.negate
		}
	}
	return ignored
}

// compileIgnoreRule 把一行 gitignore pattern 编译成 ignoreRule。
// 返回 ok=false 表示该行是空、注释或无法编译，调用方应跳过。
func compileIgnoreRule(raw string) (ignoreRule, bool) {
	line := strings.TrimRight(raw, " \t\r")
	if line == "" || strings.HasPrefix(line, "#") {
		return ignoreRule{}, false
	}

	negate := false
	if strings.HasPrefix(line, "!") {
		negate = true
		line = line[1:]
	}

	dirOnly := false
	if strings.HasSuffix(line, "/") {
		dirOnly = true
		line = strings.TrimSuffix(line, "/")
	}

	// gitignore: pattern 含 `/`（非末尾）视为锚定到 root；
	// pattern 前导 `/` 显式锚定；否则匹配任意路径深度。
	anchored := false
	if strings.HasPrefix(line, "/") {
		anchored = true
		line = strings.TrimPrefix(line, "/")
	} else if strings.Contains(line, "/") {
		anchored = true
	}

	// 转换 glob 到 regex。
	var sb strings.Builder
	if anchored {
		sb.WriteString("^")
	} else {
		sb.WriteString("(^|/)")
	}

	i := 0
	for i < len(line) {
		c := line[i]
		switch c {
		case '*':
			// `**` 匹配任意（含 `/`）；可选后缀 `/` 吞掉。
			if i+1 < len(line) && line[i+1] == '*' {
				sb.WriteString(".*")
				i += 2
				if i < len(line) && line[i] == '/' {
					i++
				}
				continue
			}
			sb.WriteString("[^/]*")
			i++
		case '?':
			sb.WriteString("[^/]")
			i++
		case '[':
			// 字符类：找到配对的 `]`，整段透传到 regex。
			end := strings.IndexByte(line[i+1:], ']')
			if end == -1 {
				sb.WriteString("\\[")
				i++
				continue
			}
			sb.WriteString(line[i : i+end+2])
			i += end + 2
		case '.', '+', '(', ')', '|', '^', '$', '{', '}', '\\':
			sb.WriteByte('\\')
			sb.WriteByte(c)
			i++
		default:
			sb.WriteByte(c)
			i++
		}
	}
	// 允许 pattern 精确匹配，也允许作为目录前缀（命中目录下所有子项）。
	sb.WriteString("(/|$)")

	re, err := regexp.Compile(sb.String())
	if err != nil {
		return ignoreRule{}, false
	}
	return ignoreRule{re: re, negate: negate, dirOnly: dirOnly}, true
}
