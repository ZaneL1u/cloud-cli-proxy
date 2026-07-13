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

func TestMigration0003DropsEgressIPAddressUniqueAndPreservesBindings(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "upgrade.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys=ON`); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}

	for _, filename := range []string{"0001_initial.sql", "0002_add_host_pids_limit.sql"} {
		contents, err := fs.ReadFile(migrations.FS, filename)
		if err != nil {
			t.Fatalf("read %s: %v", filename, err)
		}
		if _, err := db.ExecContext(ctx, string(contents)); err != nil {
			t.Fatalf("apply %s: %v", filename, err)
		}
	}

	if _, err := db.ExecContext(ctx, `
		CREATE TABLE schema_migrations (
			filename TEXT PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
		);
		INSERT INTO schema_migrations (filename) VALUES
			('0001_initial.sql'),
			('0002_add_host_pids_limit.sql');
		INSERT INTO users (id, username, entry_password) VALUES
			('user-1', 'alice', 'your-secret-here');
		INSERT INTO hosts (
			id, user_id, status, template_image_ref, home_volume_name, slot_key
		) VALUES (
			'host-1', 'user-1', 'running', 'managed-user:test', 'home-1', 'primary'
		);
		INSERT INTO egress_ips (id, label, ip_address, provider) VALUES
			('egress-1', 'shared-label', '0.0.0.0', 'manual');
		INSERT INTO host_egress_bindings (id, host_id, egress_ip_id) VALUES
			('binding-1', 'host-1', 'egress-1');
	`); err != nil {
		t.Fatalf("seed pre-migration data: %v", err)
	}

	if err := migrator.RunMigrations(ctx, db, migrations.FS); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	for _, statement := range []string{
		`INSERT INTO egress_ips (id, label, ip_address) VALUES ('egress-2', 'shared-label', '0.0.0.0')`,
		`INSERT INTO egress_ips (id, label, ip_address) VALUES ('egress-3', 'third', '8.8.8.8')`,
		`INSERT INTO egress_ips (id, label, ip_address) VALUES ('egress-4', 'fourth', '8.8.8.8')`,
		`INSERT INTO egress_ips (id, label, ip_address) VALUES ('egress-5', 'empty-one', '')`,
		`INSERT INTO egress_ips (id, label, ip_address) VALUES ('egress-6', 'empty-two', '')`,
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("duplicate or empty ip_address should be allowed: %v", err)
		}
	}

	var bindingCount int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM host_egress_bindings
		WHERE id = 'binding-1' AND host_id = 'host-1' AND egress_ip_id = 'egress-1'
	`).Scan(&bindingCount); err != nil {
		t.Fatalf("query preserved binding: %v", err)
	}
	if bindingCount != 1 {
		t.Fatalf("preserved binding count = %d, want 1", bindingCount)
	}

	if _, err := db.ExecContext(ctx, `DELETE FROM egress_ips WHERE id = 'egress-1'`); err == nil {
		t.Fatal("deleting a bound egress IP should still be restricted")
	}

	rows, err := db.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatalf("run foreign key check: %v", err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("foreign_key_check reported a violation after migration")
	}

	if _, err := db.ExecContext(ctx, `INSERT INTO egress_ips (id, label, ip_address) VALUES ('egress-null', 'null-ip', NULL)`); err == nil {
		t.Fatal("ip_address should remain NOT NULL")
	}

	if err := migrator.RunMigrations(ctx, db, migrations.FS); err != nil {
		t.Fatalf("rerun migrations: %v", err)
	}
}
