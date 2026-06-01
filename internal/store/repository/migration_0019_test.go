package repository

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMigration0019_FileContent 验证 host_bypass 五张表的核心 schema 语义。
func TestMigration0019_FileContent(t *testing.T) {
	path := filepath.Join("..", "migrations", initialMigrationFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 migration 失败（%s）: %v", initialMigrationFile, err)
	}
	content := string(raw)

	mustContain := []string{
		"CREATE TABLE IF NOT EXISTS host_bypass_presets",
		"CREATE TABLE IF NOT EXISTS host_bypass_rules",
		"CREATE TABLE IF NOT EXISTS host_bypass_bindings",
		"CREATE TABLE IF NOT EXISTS host_bypass_snapshots",
		"CREATE TABLE IF NOT EXISTS host_bypass_audit_log",
		"rules",
		"TEXT NOT NULL DEFAULT '[]'",
		"hex(randomblob(16))",
		"DATETIME NOT NULL DEFAULT (CURRENT_TIMESTAMP)",
		"CHECK (scope IN ('global', 'host'))",
		"CHECK (rule_type IN ('ip','cidr','domain','domain_suffix','domain_keyword','port'))",
		"CHECK (source IN ('admin','system'))",
		"CHECK (applied_status IN ('pending','applied','failed','rolled_back'))",
		"applied_status          TEXT NOT NULL DEFAULT 'pending'",
		"CONSTRAINT chk_bypass_rule_scope",
		"CONSTRAINT chk_bypass_binding_xor",
		"UNIQUE (host_id, config_hash)",
		"REFERENCES hosts (id) ON DELETE CASCADE",
		"'loopback'",
		"'lan'",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"100.64.0.0/10",
	}
	for _, token := range mustContain {
		if !strings.Contains(content, token) {
			t.Errorf("migration 必须包含 %q", token)
		}
	}

	forbidden := []string{
		"CREATE TYPE", // 禁止 PG ENUM
	}
	for _, token := range forbidden {
		for _, line := range strings.Split(content, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "--") {
				continue
			}
			if strings.Contains(line, token) {
				t.Errorf("migration 不得在可执行语句中包含 %q（行：%s）", token, line)
			}
		}
	}
}

// TestMigration0019_SystemPresetsSeed 锁定两条系统预设的 slug / name 字面量。
func TestMigration0019_SystemPresetsSeed(t *testing.T) {
	path := filepath.Join("..", "migrations", initialMigrationFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 migration 失败: %v", err)
	}
	content := string(raw)

	if !strings.Contains(content, "'loopback', '本机回环'") {
		t.Error("seed loopback 行必须存在且 name='本机回环'")
	}
	if !strings.Contains(content, "'lan', '局域网'") {
		t.Error("seed lan 行必须存在且 name='局域网'")
	}
}

// TestMigration0019_SnapshotShape 锁定 snapshot 表的关键列与索引。
func TestMigration0019_SnapshotShape(t *testing.T) {
	path := filepath.Join("..", "migrations", initialMigrationFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 migration 失败: %v", err)
	}
	content := string(raw)

	mustContain := []string{
		"host_bypass_snapshots",
		"version                 INTEGER NOT NULL",
		"config_hash             TEXT NOT NULL",
		"whitelist_cidrs_json    TEXT NOT NULL DEFAULT '{\"version\":3,\"rules\":[]}'",
		"whitelist_domains_json  TEXT NOT NULL DEFAULT '{\"version\":3,\"rules\":[]}'",
		"UNIQUE (host_id, config_hash)",
		"idx_bypass_snapshots_host_version",
	}
	for _, token := range mustContain {
		if !strings.Contains(content, token) {
			t.Errorf("snapshot 段缺少 %q", token)
		}
	}
}
