package repository

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// migration0019Filename 是 Phase 45 Plan 03 落地的 migration 文件名（BYPASS-DATA-01..04）。
const migration0019Filename = "0019_host_bypass_rules.sql"

// TestMigration0019_FileContent 验证 migration 0019 的核心 schema 语义：
//   - 五张表 + TEXT 主键 + TEXT DEFAULT (CURRENT_TIMESTAMP)
//   - 四个 CHECK 枚举（scope / rule_type / source / applied_status）
//   - XOR 约束（rule scope <-> host_id；binding preset_id <-> rule_id）
//   - 两条系统预设 seed（loopback / lan，含五段 CIDR）
//   - 禁止 ENUM 类型与 up 段 DROP TABLE
func TestMigration0019_FileContent(t *testing.T) {
	path := filepath.Join("..", "migrations", migration0019Filename)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 migration 失败（%s）: %v", migration0019Filename, err)
	}
	content := string(raw)

	mustContain := []string{
		"CREATE TABLE IF NOT EXISTS host_bypass_presets",
		"CREATE TABLE IF NOT EXISTS host_bypass_rules",
		"CREATE TABLE IF NOT EXISTS host_bypass_bindings",
		"CREATE TABLE IF NOT EXISTS host_bypass_snapshots",
		"CREATE TABLE IF NOT EXISTS host_bypass_audit_log",
		"hex(randomblob(16))",
		"TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)",
		"rules         TEXT NOT NULL DEFAULT '[]'",
		"CHECK (scope IN ('global', 'host'))",
		"CHECK (rule_type IN ('ip','cidr','domain','domain_suffix','domain_keyword','port'))",
		"CHECK (source IN ('admin','system'))",
		"CHECK (applied_status IN ('pending','applied','failed','rolled_back'))",
		"applied_status          TEXT NOT NULL DEFAULT 'pending'",
		"CONSTRAINT chk_bypass_rule_scope",
		"CONSTRAINT chk_bypass_binding_xor",
		"UNIQUE (host_id, config_hash)",
		"REFERENCES hosts(id) ON DELETE CASCADE",
		"REFERENCES users(id) ON DELETE SET NULL",
		"ON CONFLICT (slug) DO NOTHING",
		"'loopback'",
		"'lan'",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"100.64.0.0/10",
		"fc00::/7",
	}
	for _, token := range mustContain {
		if !strings.Contains(content, token) {
			t.Errorf("migration 0019 必须包含 %q", token)
		}
	}

	forbidden := []string{
		"CREATE TYPE", // 禁止 PG ENUM（项目历史用 TEXT + CHECK，对齐 RESEARCH §Anti-Patterns）
		"DROP TABLE",  // up 段不做 rollback；DROP 仅在文件顶部注释中作为运维参考
	}
	for _, token := range forbidden {
		// 注释里我们写了运维参考的 DROP TABLE 示意，必须排除注释行；用每行扫描更稳。
		for _, line := range strings.Split(content, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "--") {
				continue
			}
			if strings.Contains(line, token) {
				t.Errorf("migration 0019 不得在可执行语句中包含 %q（行：%s）", token, line)
			}
		}
	}
}

// TestMigration0019_SystemPresetsSeed 锁定两条系统预设的 slug / name 字面量；
// 后续 Phase 46 admin API 会以 slug 为稳定键查询，name 是用户可见文案，禁止漂移。
func TestMigration0019_SystemPresetsSeed(t *testing.T) {
	path := filepath.Join("..", "migrations", migration0019Filename)
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

// TestMigration0019_SnapshotShape 锁定 snapshot 表的关键列与索引；
// Phase 47 apply / rollback 链路会依赖 version DESC 顺序和 UNIQUE(host_id, config_hash) 做幂等。
func TestMigration0019_SnapshotShape(t *testing.T) {
	path := filepath.Join("..", "migrations", migration0019Filename)
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
