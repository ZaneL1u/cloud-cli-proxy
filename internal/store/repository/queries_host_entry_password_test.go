package repository

import (
	"strings"
	"testing"
)

// TestAllHostReadQueriesExcludeEntryPassword 锁定 0018 迁移后的不变量：
// 读 Host 的 SQL 不应再 SELECT entry_password 列（该列已迁至 users.entry_password）。
//
// 测试只断言 SQL 文本契约（字符串 strings.Contains），不连接 DB、
// 不打印任何密码明文样本（敏感字段防泄漏）。
func TestAllHostReadQueriesExcludeEntryPassword(t *testing.T) {
	queries := map[string]string{
		"getHostSQL":                  getHostSQL,
		"listHostsSQL":                listHostsSQL,
		"listHostsByUserIDSQL":        listHostsByUserIDSQL,
		"listHostsWithUsernameSQL":    listHostsWithUsernameSQL,
		"listRunningHostsSQL":         listRunningHostsSQL,
		"listRunningHostsByUserIDSQL": listRunningHostsByUserIDSQL,
	}
	for name, q := range queries {
		if strings.Contains(q, "entry_password") {
			t.Errorf("%s 不应再包含 entry_password 列（0018 迁移后该列已迁至 users）；实际 SQL:\n%s", name, q)
		}
	}
}
