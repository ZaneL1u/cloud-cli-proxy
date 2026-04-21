package cloudclaude

import (
	"os"

	"golang.org/x/term"
)

// ANSI 颜色常量（极简实现，避免引入第三方 color 库）。
// Phase 34 Plan 02 Task 2.1：首字母大写导出，供 doctor 包跨包复用。
const (
	AnsiReset  = "\033[0m"
	AnsiRed    = "\033[31m"
	AnsiGreen  = "\033[32m"
	AnsiYellow = "\033[33m"
	AnsiCyan   = "\033[36m"
	AnsiGray   = "\033[90m" // [Phase 32 D-23] reconnect "..." / input_buffer 未确认字符
)

// fdHolder 是 ColorEnabled 唯一关心的接口：能拿到 fd 即可探测 TTY。
// os.File 实现该接口；测试时可传 nil 或自定义实现。
type fdHolder interface {
	Fd() uintptr
}

// ColorEnabled 判定是否输出 ANSI 颜色：
//   - noColor=true 直接关闭
//   - NO_COLOR 环境变量任意非空 → 关闭（遵循 https://no-color.org/）
//   - w 为 nil 或非 TTY → 关闭
//
// Phase 34 Plan 02 Task 2.1：从 lower-case colorEnabled 改名导出，doctor 包复用。
func ColorEnabled(noColor bool, w fdHolder) bool {
	if noColor {
		return false
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if w == nil {
		return false
	}
	return term.IsTerminal(int(w.Fd()))
}

// Colorize 包装文本为 ANSI 着色。enabled=false 时返回原文。
//
// Phase 34 Plan 02 Task 2.1：从 lower-case colorize 改名导出，doctor 包复用。
func Colorize(s, ansi string, enabled bool) string {
	if !enabled {
		return s
	}
	return ansi + s + AnsiReset
}
