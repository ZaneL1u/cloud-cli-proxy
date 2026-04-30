package cloudclaude

import (
	"fmt"
	"io"
	"strings"
)

// ProgressUI 提供极客风终端进度展示。
// 核心约束：每阶段只保留最新一行，不刷屏。
type ProgressUI struct {
	w        io.Writer
	enabled  bool
	barWidth int
}

// NewProgressUI 创建 ProgressUI。
// noColor=true 或 NO_COLOR 环境变量或 w 非 TTY 时禁用颜色与单行刷新。
func NewProgressUI(w io.Writer, noColor bool) *ProgressUI {
	enabled := false
	if fh, ok := w.(fdHolder); ok {
		enabled = ColorEnabled(noColor, fh)
	}
	return &ProgressUI{
		w:        w,
		enabled:  enabled,
		barWidth: 20,
	}
}

// Scanning 在单行实时刷新扫描进度。
// 格式：同步代码库  扫描 1,232 文件  src/utils.ts
func (p *ProgressUI) Scanning(file string, count int) {
	if !p.enabled {
		return
	}
	fmt.Fprintf(p.w, "\r\033[2K同步代码库  扫描 %s 文件  %s",
		formatInt(count), truncate(file, 28))
}

// ScanDone 结束扫描，定格最终统计。
func (p *ProgressUI) ScanDone(total int) {
	fmt.Fprintf(p.w, "\r\033[2K同步代码库  扫描完成 %s 文件\n", formatInt(total))
}

// Syncing 在单行实时刷新同步进度。
// 格式：[████████░░] 98% (1,210/1,232) file.go
func (p *ProgressUI) Syncing(done, total int, currentFile string) {
	if !p.enabled {
		return
	}
	pct := 0
	if total > 0 {
		pct = done * 100 / total
	}
	filled := pct * p.barWidth / 100
	bar := strings.Repeat("█", filled) + strings.Repeat("░", p.barWidth-filled)

	fmt.Fprintf(p.w, "\r\033[2K[%s] %3d%% (%s/%s) %s",
		bar, pct, formatInt(done), formatInt(total), truncate(currentFile, 20))
}

// SyncDone 结束同步，定格 100%。
func (p *ProgressUI) SyncDone(done, total int) {
	if p.enabled {
		bar := strings.Repeat("█", p.barWidth)
		fmt.Fprintf(p.w, "\r\033[2K[%s] 100%% (%s/%s) 同步完成\n",
			bar, formatInt(done), formatInt(total))
	} else {
		fmt.Fprintf(p.w, "  同步完成 %s 个文件\n", formatInt(done))
	}
}

// Distribution 输出 hot/cold 分布摘要（一行，不刷新）。
func (p *ProgressUI) Distribution(hotFiles, hotBytes, coldFiles, coldBytes int64) {
	total := hotFiles + coldFiles
	hotPct := int64(0)
	if total > 0 {
		hotPct = hotFiles * 100 / total
	}

	hotText := fmt.Sprintf("hot:%d%% %sf %s", hotPct, formatInt(int(hotFiles)), formatBytes(hotBytes))
	coldText := fmt.Sprintf("cold:%d%% %sf %s", 100-hotPct, formatInt(int(coldFiles)), formatBytes(coldBytes))

	if p.enabled {
		fmt.Fprintf(p.w, "  %s%s%s  %s%s%s\n",
			AnsiOrange, hotText, AnsiReset,
			AnsiBlue, coldText, AnsiReset)
	} else {
		fmt.Fprintf(p.w, "  %s  %s\n", hotText, coldText)
	}
}

// formatInt 千位分隔。
func formatInt(n int) string {
	s := fmt.Sprintf("%d", n)
	if n < 1000 {
		return s
	}
	var result []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}
	return string(result)
}

// formatBytes 人类可读字节。
func formatBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// truncate 截断字符串，保留头尾，中间用 … 连接。
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	head := maxLen/2 - 1
	tail := maxLen - head - 3
	return string(runes[:head]) + "…" + string(runes[len(runes)-tail:])
}
