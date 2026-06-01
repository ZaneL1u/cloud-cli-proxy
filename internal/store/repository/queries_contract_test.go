package repository

import (
	"strings"
	"testing"
)

// TestSQLConstants_NonEmpty 确保所有包级 SQL 常量非空。
func TestSQLConstants_NonEmpty(t *testing.T) {
	queries := map[string]string{
		"getHostSQL":                           getHostSQL,
		"listHostsSQL":                         listHostsSQL,
		"listHostsByUserIDSQL":                 listHostsByUserIDSQL,
		"listHostsWithUsernameSQL":             listHostsWithUsernameSQL,
		"listRunningHostsSQL":                  listRunningHostsSQL,
		"listRunningHostsByUserIDSQL":          listRunningHostsByUserIDSQL,
		"getHostByUsernameSQL":                 getHostByUsernameSQL,
		"getHostWithClaudeAccountSQL":          getHostWithClaudeAccountSQL,
		"resolveClaudeAccountByHostSQL":        resolveClaudeAccountByHostSQL,
		"resolveClaudeAccountByUserFallbackSQL": resolveClaudeAccountByUserFallbackSQL,
		"checkClaudeAccountPersistentVolumeNameSQL": checkClaudeAccountPersistentVolumeNameSQL,
		"upsertClaudeAccountPersistentVolumeNameSQL": upsertClaudeAccountPersistentVolumeNameSQL,
		"lockClaudeAccountForDeleteSQL":        lockClaudeAccountForDeleteSQL,
		"deleteClaudeAccountSQL":               deleteClaudeAccountSQL,
	}
	for name, q := range queries {
		if strings.TrimSpace(q) == "" {
			t.Errorf("%s 是空 SQL 常量", name)
		}
	}
}

// TestHostQueries_IncludeRequiredColumns 验证所有读 Host 的 SQL 包含必要列。
// 0018 迁移后 hosts.entry_password 已删除，凭据归 users 表所有。
func TestHostQueries_IncludeRequiredColumns(t *testing.T) {
	hostReadQueries := map[string]string{
		"getHostSQL":                  getHostSQL,
		"listHostsSQL":                listHostsSQL,
		"listHostsByUserIDSQL":        listHostsByUserIDSQL,
		"listHostsWithUsernameSQL":    listHostsWithUsernameSQL,
		"listRunningHostsSQL":         listRunningHostsSQL,
		"listRunningHostsByUserIDSQL": listRunningHostsByUserIDSQL,
	}

	required := []string{
		"id", "user_id", "status", "short_id",
		"template_image_ref",
		"home_volume_name", "slot_key", "timezone", "hostname",
	}

	for name, q := range hostReadQueries {
		for _, col := range required {
			if !strings.Contains(q, col) {
				t.Errorf("%s 缺少必要列 %q", name, col)
			}
		}
		if strings.Contains(q, "entry_password") {
			t.Errorf("%s 不应再 SELECT entry_password（0018 迁移后该列已迁至 users）", name)
		}
	}
}

// TestGetHostByUsernameSQL_Shape 验证 username 查询 SQL 结构正确。
func TestGetHostByUsernameSQL_Shape(t *testing.T) {
	q := getHostByUsernameSQL
	if !strings.Contains(q, "JOIN users u") {
		t.Error("getHostByUsernameSQL 应 JOIN users 表")
	}
	if !strings.Contains(q, "u.username = ?") {
		t.Error("getHostByUsernameSQL 应按 u.username 查询")
	}
	if !strings.Contains(q, "ssh_private_key") {
		t.Error("getHostByUsernameSQL 应包含 ssh_private_key 列")
	}
	if !strings.Contains(q, "'workspace'") {
		t.Error("getHostByUsernameSQL 应硬编码 ContainerUser = 'workspace'")
	}
	if strings.Contains(q, "h.short_id = ?") {
		t.Error("getHostByUsernameSQL 不应再按 short_id 查询")
	}
	if !strings.Contains(q, "u.entry_password") {
		t.Error("getHostByUsernameSQL 应从 users 表读取 entry_password（0018 迁移后）")
	}
	if strings.Contains(q, "h.entry_password") {
		t.Error("getHostByUsernameSQL 不应再读 hosts.entry_password（0018 迁移后该列已被删除）")
	}
}

// TestListHostsWithUsernameSQL_IncludesEgressInfo 验证 listHostsWithUsernameSQL 包含出口 IP 信息。
func TestListHostsWithUsernameSQL_IncludesEgressInfo(t *testing.T) {
	q := listHostsWithUsernameSQL
	if !strings.Contains(q, "egress_ips") && !strings.Contains(q, "LEFT JOIN") {
		t.Error("listHostsWithUsernameSQL 应包含 JOIN 以获取出口 IP")
	}
	if !strings.Contains(q, "u.username") {
		t.Error("listHostsWithUsernameSQL 应包含 u.username 列")
	}
}

// TestClaudeAccountQueries_Shape 验证 ClaudeAccount 相关 SQL 结构正确。
func TestClaudeAccountQueries_Shape(t *testing.T) {
	queries := map[string][]string{
		"resolveClaudeAccountByHostSQL": {
			"claude_accounts",
		},
		"resolveClaudeAccountByUserFallbackSQL": {
			"claude_accounts",
		},
		"checkClaudeAccountPersistentVolumeNameSQL": {
			"claude_accounts",
			"persistent_volume_name",
		},
		"lockClaudeAccountForDeleteSQL": {
			"claude_accounts",
			"FOR UPDATE",
		},
		"deleteClaudeAccountSQL": {
			"DELETE FROM claude_accounts",
		},
	}

	for name, keywords := range queries {
		sqlText, ok := sqlConstants[name]
		if !ok {
			t.Fatalf("缺少 SQL 常量映射: %s", name)
		}
		for _, keyword := range keywords {
			if !strings.Contains(sqlText, keyword) {
				t.Errorf("%s 应包含 %q", name, keyword)
			}
		}
	}
}

// sqlConstants maps test names to actual SQL constants for contract tests.
var sqlConstants = map[string]string{
	"resolveClaudeAccountByHostSQL":        resolveClaudeAccountByHostSQL,
	"resolveClaudeAccountByUserFallbackSQL": resolveClaudeAccountByUserFallbackSQL,
	"checkClaudeAccountPersistentVolumeNameSQL": checkClaudeAccountPersistentVolumeNameSQL,
	"lockClaudeAccountForDeleteSQL":        lockClaudeAccountForDeleteSQL,
	"deleteClaudeAccountSQL":               deleteClaudeAccountSQL,
}

// TestGetHostSQL_NoLeakOfSensitiveFields 验证敏感字段不出现在默认查询中。
// 0018 迁移后 hosts.entry_password 列已不存在，凭据归 users 所有，仅 GetHostByUsername 返回。
func TestGetHostSQL_NoLeakOfSensitiveFields(t *testing.T) {
	q := getHostSQL
	if strings.Contains(q, "entry_password") {
		t.Error("getHostSQL 不应再 SELECT entry_password（0018 迁移后该列已迁至 users）")
	}
}

// TestRepository_New_PanicsOnNil 可选的负面测试（验证 New(nil) 行为）。
// 实际 New 不检查 nil，但调用方不应传 nil。
func TestRepository_New_DoesNotPanic(t *testing.T) {
	// New(nil) 可能不会 panic，但后续调用会 panic on nil pointer。
	// 此测试仅为文档化当前行为。
	t.Run("nil pool", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Log("New(nil) panicked (expected for documentation)")
			}
		}()
		_ = New(nil)
	})
}
