// cmd/cloud-claude/explain.go — Phase 34 Plan 01 Task 1.7
//
// cloud-claude explain <code> 子命令：对标 rustc --explain。
// 数据源 = errcodes.Lookup + errcodes.Format + errcodes.ExtendedExplanations。
// 大小写敏感匹配（CONTEXT D-17 / RESEARCH §8.4），未注册 code exit 4 (= exitConfigError)。
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/errcodes"
)

func newExplainCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "explain <code>",
		Short:         "解释 cloud-claude 错误码（对标 rustc --explain）",
		Long:          "对给定错误码（大小写敏感）输出统一三要素 + 详细中文说明 + 修复路径。\n未注册错误码返回 exit 4。",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runExplain,
	}
	cmd.Flags().Bool("verbose", false, "输出关联的 doctor check 名（如已登记）")
	return cmd
}

func runExplain(cmd *cobra.Command, args []string) error {
	code := errcodes.Code(args[0])

	entry, ok := errcodes.Lookup(code)
	if !ok {
		fmt.Fprintf(os.Stderr, "未找到错误码 %s；运行 cloud-claude doctor 查看可用检查项\n", args[0])
		os.Exit(exitConfigError)
		return nil
	}
	fmt.Println(errcodes.Format(code))

	fmt.Println()
	fmt.Println("详细说明：")
	if exp, hasExp := errcodes.ExtendedExplanations[code]; hasExp {
		fmt.Println(exp)
	} else {
		fmt.Println("未提供详细说明，运行 cloud-claude doctor <domain> 查看相关检查项")
	}

	verbose, _ := cmd.Flags().GetBool("verbose")
	if verbose {
		fmt.Println()
		fmt.Printf("Severity: %s\n", entry.Severity)
	}
	return nil
}
