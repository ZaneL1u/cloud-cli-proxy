package doctor

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude"
)

// 状态符号 + 纯文本 fallback（CONTEXT D-14）。
const (
	iconPass = "[✓]"
	iconWarn = "[!]"
	iconFail = "[✗]"
	iconSkip = "[~]"

	iconPassPlain = "[ok]"
	iconWarnPlain = "[warn]"
	iconFailPlain = "[fail]"
	iconSkipPlain = "[skip]"
)

// RenderText 是 text 模式的主入口（CONTEXT D-13 布局：banner → downgrade banner → 5 维度矩阵 → 汇总）。
func RenderText(r *Report, noColor bool) string {
	var b strings.Builder

	// 第一段：banner
	b.WriteString("╭─────────────────────────────────────────╮\n")
	b.WriteString("│  Cloud Claude Doctor v3.0 体检报告       │\n")
	b.WriteString("╰─────────────────────────────────────────╯\n")
	if r.CloudClaudeVer != "" {
		fmt.Fprintf(&b, "  cloud-claude: %s\n", r.CloudClaudeVer)
	}
	if r.RemoteImageVer != "" {
		fmt.Fprintf(&b, "  远端镜像:     %s\n", r.RemoteImageVer)
	}
	b.WriteString("\n")

	// 第二段：降级历史 banner（M13 第一屏锚点）
	b.WriteString(renderDowngradeBanner(r.DowngradeHistory))
	b.WriteString("\n")

	// 第三段：5 维度矩阵
	byDomain := groupByDomain(r.Checks)
	for _, dom := range []string{"network", "auth", "ssh", "mount", "disk"} {
		checks, ok := byDomain[dom]
		if !ok {
			continue
		}
		fmt.Fprintf(&b, "── %s ──\n", dom)
		for _, c := range checks {
			b.WriteString(renderCheckLine(c, noColor))
		}
		b.WriteString("\n")
	}

	// 末尾：汇总
	fmt.Fprintf(&b, "共 %d 项检查：%d pass / %d warn / %d fail / %d skip（耗时 %.2fs）\n",
		r.Summary.Total, r.Summary.Pass, r.Summary.Warn, r.Summary.Fail, r.Summary.Skip,
		float64(r.DurationMS)/1000.0)

	return b.String()
}

// renderDowngradeBanner 读 LastSessionSnapshot 输出第一屏降级历史（M13 验收锚点）。
// 输入为 nil → 输出 STATE_LAST_SESSION_MISSING 提示（**不算 fail**）。
func renderDowngradeBanner(banner *DowngradeBanner) string {
	var b strings.Builder
	b.WriteString("── 上次会话快照 ──\n")
	if banner == nil {
		b.WriteString("  [!] 未找到上次会话快照（首次运行 cloud-claude 后再 doctor 即可看到）\n")
		b.WriteString("      错误码: STATE_LAST_SESSION_MISSING\n")
		return b.String()
	}
	fmt.Fprintf(&b, "  时间戳: %s（%d 秒前）\n",
		time.Now().Add(-time.Duration(banner.SnapshotAgeSeconds)*time.Second).Format(time.RFC3339),
		banner.SnapshotAgeSeconds)
	if banner.IntendedMode != "" && banner.ActualMode != "" {
		fmt.Fprintf(&b, "  模式: 意图=%s 实际=%s\n", banner.IntendedMode, banner.ActualMode)
	}
	if banner.ClientRole != "" {
		fmt.Fprintf(&b, "  角色: %s\n", banner.ClientRole)
	}
	if banner.ConflictCount > 0 {
		fmt.Fprintf(&b, "  热同步冲突: %d 个\n", banner.ConflictCount)
	}
	if banner.ReconnectCount > 0 {
		fmt.Fprintf(&b, "  重连次数: %d\n", banner.ReconnectCount)
	}
	// M13 关键字面量：`[降级] <from> → <to> | 原因 [<reason_code>] <reason_message>`
	for _, step := range banner.DowngradeChain {
		fmt.Fprintf(&b, "  [降级] %s → %s | 原因 [%s] %s\n",
			step.From, step.To, step.ReasonCode, step.ReasonMessage)
	}
	return b.String()
}

// renderCheckLine 渲染单个 check 为一行 + 可选多行 FixApplied/FixFailed。
// 输出格式：`  [符号] name: message（建议: ... | 错误码: ...）`
func renderCheckLine(c Check, noColor bool) string {
	var b strings.Builder
	icon := pickIcon(c.Status, noColor)
	fmt.Fprintf(&b, "  %s %s: %s", icon, c.Name, c.Message)
	if c.Status == StatusWarn || c.Status == StatusFail {
		// PITFALLS M14: 所有 warn/fail 必带「建议:」子串 + 错误码
		var suffix []string
		if c.NextAction != "" {
			suffix = append(suffix, "建议: "+c.NextAction)
		}
		if c.Code != "" {
			suffix = append(suffix, "错误码: "+string(c.Code))
		}
		if len(suffix) > 0 {
			fmt.Fprintf(&b, "（%s）", strings.Join(suffix, " | "))
		}
	}
	b.WriteString("\n")
	for _, fx := range c.FixApplied {
		fmt.Fprintf(&b, "       ✓ 已修复: %s\n", fx)
	}
	for _, fx := range c.FixFailed {
		fmt.Fprintf(&b, "       ✗ 修复失败: %s\n", fx)
	}
	return b.String()
}

// pickIcon 根据 Status + noColor 选择符号。
// 注：cloudclaude.ColorEnabled 接受 (noColor, fdHolder) 两参；这里传入 os.Stdout 做 TTY 探测。
func pickIcon(s Status, noColor bool) string {
	if !cloudclaude.ColorEnabled(noColor, os.Stdout) {
		switch s {
		case StatusPass:
			return iconPassPlain
		case StatusWarn:
			return iconWarnPlain
		case StatusFail:
			return iconFailPlain
		case StatusSkip:
			return iconSkipPlain
		}
		return iconPassPlain
	}
	// 彩色版：直接返回带彩色 ANSI 的 icon
	switch s {
	case StatusPass:
		return cloudclaude.Colorize(iconPass, cloudclaude.AnsiGreen, true)
	case StatusWarn:
		return cloudclaude.Colorize(iconWarn, cloudclaude.AnsiYellow, true)
	case StatusFail:
		return cloudclaude.Colorize(iconFail, cloudclaude.AnsiRed, true)
	case StatusSkip:
		return cloudclaude.Colorize(iconSkip, cloudclaude.AnsiGray, true)
	}
	return iconPass
}

// groupByDomain 按 Check.Domain 分组。
func groupByDomain(checks []Check) map[string][]Check {
	m := make(map[string][]Check)
	for _, c := range checks {
		m[c.Domain] = append(m[c.Domain], c)
	}
	return m
}

// RenderJSON 是 --json 模式主入口：MarshalIndent 2 空格（CONTEXT Discretion §6）。
func RenderJSON(r *Report) ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}
