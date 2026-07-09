package repository

import (
	"context"
	"database/sql"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/zanel1u/cloud-cli-proxy/internal/store/migrations"
	"github.com/zanel1u/cloud-cli-proxy/internal/store/migrator"
	_ "modernc.org/sqlite"
)

// TestMigration0003_EgressIPLabelUnique 验证：
// 1. foreign_keys=ON + 有绑定数据 → 0003 迁移不因 RESTRICT 失败（子表重建法）
// 2. 迁移后绑定数据完整（id/label/绑定行仍在）
// 3. 同 label 触发唯一冲突（label UNIQUE 已建立）
// 4. 同 ip_address 不同 label 允许插入（ip 已去 UNIQUE）
// 5. ip_address 可为 NULL（NOT NULL 约束已去除）
// 6. 恢复的子表 FK 仍有效（DELETE 被引用的 egress_ip 被 RESTRICT 挡）
func TestMigration0003_EgressIPLabelUnique(t *testing.T) {
	// 步骤 1：使用独立文件库，不用 :memory（PRAGMA 状态在文件库更稳定）
	dsn := "file:" + filepath.Join(t.TempDir(), "m.db")
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	// 步骤 1b：SetMaxOpenConns(1) 保证 PRAGMA 状态在唯一连接上一致
	db.SetMaxOpenConns(1)

	ctx := context.Background()

	// 步骤 2：开启 foreign_keys 并断言生效
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys=ON"); err != nil {
		t.Fatalf("PRAGMA foreign_keys=ON: %v", err)
	}
	var fkEnabled int
	if err := db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fkEnabled); err != nil {
		t.Fatalf("read PRAGMA foreign_keys: %v", err)
	}
	if fkEnabled != 1 {
		t.Fatalf("foreign_keys 未生效（==%d），测试无效", fkEnabled)
	}

	// 步骤 3：读 0001、0002 SQL 文件内容并各自 Exec 建 schema（不含 0003）
	for _, filename := range []string{"0001_initial.sql", "0002_add_host_pids_limit.sql"} {
		content, err := fs.ReadFile(migrations.FS, filename)
		if err != nil {
			t.Fatalf("read %s: %v", filename, err)
		}
		if _, err := db.ExecContext(ctx, string(content)); err != nil {
			t.Fatalf("exec %s: %v", filename, err)
		}
	}

	// 步骤 4：插入满足 FK 的最小数据链，制造 RESTRICT 场景
	// users 表：必填 entry_password（NOT NULL DEFAULT ''），role（NOT NULL DEFAULT 'user'）
	if _, err := db.ExecContext(ctx,
		`INSERT INTO users (id, username, entry_password) VALUES ('u1', 'alice', 'pw')`); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	// hosts 表：必填 user_id, template_image_ref, home_volume_name, slot_key
	if _, err := db.ExecContext(ctx,
		`INSERT INTO hosts (id, user_id, status, template_image_ref, home_volume_name, slot_key)
		 VALUES ('h1', 'u1', 'running', 'img:v1', 'vol-1', 'primary')`); err != nil {
		t.Fatalf("insert host: %v", err)
	}
	// egress_ips：旧表（ip_address NOT NULL UNIQUE），插入合法数据
	if _, err := db.ExecContext(ctx,
		`INSERT INTO egress_ips (id, label, ip_address, provider) VALUES ('e1', 'vless-10042', '0.0.0.0', 'manual')`); err != nil {
		t.Fatalf("insert egress_ip: %v", err)
	}
	// host_egress_bindings：引用 h1 → e1，制造 ON DELETE RESTRICT 场景
	if _, err := db.ExecContext(ctx,
		`INSERT INTO host_egress_bindings (id, host_id, egress_ip_id) VALUES ('b1', 'h1', 'e1')`); err != nil {
		t.Fatalf("insert host_egress_binding: %v", err)
	}

	// 验证绑定行确实存在（确保制造场景成功）
	var preCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM host_egress_bindings`).Scan(&preCount); err != nil {
		t.Fatalf("count bindings pre-migration: %v", err)
	}
	if preCount != 1 {
		t.Fatalf("迁移前应有 1 行绑定，实际 %d", preCount)
	}

	// 步骤 5：读 0003 SQL 内容并 Exec（此时有绑定数据 + FK=ON）
	sql0003, err := fs.ReadFile(migrations.FS, "0003_egress_ip_label_unique.sql")
	if err != nil {
		t.Fatalf("read 0003: %v", err)
	}
	if _, err := db.ExecContext(ctx, string(sql0003)); err != nil {
		t.Fatalf("exec 0003（子表重建法应能通过）: %v", err)
	}

	// 步骤 6：断言迁移后语义正确

	// 6a. 绑定数据仍完整
	var postCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM host_egress_bindings WHERE egress_ip_id = 'e1'`).Scan(&postCount); err != nil {
		t.Fatalf("count bindings post-migration: %v", err)
	}
	if postCount != 1 {
		t.Errorf("迁移后绑定数据应保留 1 行，实际 %d", postCount)
	}

	// 6b. label UNIQUE 生效：同 label 重复插入应报错
	_, dupErr := db.ExecContext(ctx,
		`INSERT INTO egress_ips (id, label, ip_address, provider) VALUES ('e2', 'vless-10042', '1.1.1.1', 'manual')`)
	if dupErr == nil {
		t.Error("同 label 重复插入应报唯一约束错误，但未报错（label UNIQUE 未生效）")
	}

	// 6c. ip_address 去 UNIQUE：同 ip 不同 label 允许
	if _, err := db.ExecContext(ctx,
		`INSERT INTO egress_ips (id, label, ip_address, provider) VALUES ('e3', 'socks-10080', '0.0.0.0', 'manual')`); err != nil {
		t.Errorf("同 ip_address 不同 label 应允许插入（ip 已去 UNIQUE），但报错: %v", err)
	}

	// 6d. ip_address 可为 NULL（NOT NULL 约束已去除）
	if _, err := db.ExecContext(ctx,
		`INSERT INTO egress_ips (id, label, provider) VALUES ('e4', 'reality-10443', 'manual')`); err != nil {
		t.Errorf("ip_address 为 NULL 应允许插入，但报错: %v", err)
	}

	// 6e. 恢复的 FK 仍有效：DELETE 被 RESTRICT 引用的 egress_ip 应被挡
	_, restrictErr := db.ExecContext(ctx, `DELETE FROM egress_ips WHERE id = 'e1'`)
	if restrictErr == nil {
		t.Error("DELETE 被 RESTRICT 引用的 egress_ip 应被 FK 约束阻止，但未报错（FK 恢复失败）")
	}

	// 步骤 7（可选集成层）：另建空库，用 migrator.RunMigrations 全量跑（含 0003），断言无错
	dsn2 := "file:" + filepath.Join(t.TempDir(), "m2.db")
	db2, err := sql.Open("sqlite", dsn2)
	if err != nil {
		t.Fatalf("open db2: %v", err)
	}
	defer db2.Close()
	db2.SetMaxOpenConns(1)
	if _, err := db2.ExecContext(ctx, "PRAGMA foreign_keys=ON"); err != nil {
		t.Fatalf("db2 PRAGMA foreign_keys=ON: %v", err)
	}
	if err := migrator.RunMigrations(ctx, db2, migrations.FS); err != nil {
		t.Errorf("migrator.RunMigrations（含 0003，空库）失败: %v", err)
	}
}
