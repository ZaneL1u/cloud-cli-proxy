package migrator

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

func RunMigrations(ctx context.Context, db *sql.DB, migrationFS embed.FS) error {
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			filename TEXT PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
		)
	`); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(migrationFS, ".")
	if err != nil {
		return fmt.Errorf("discover migrations: %w", err)
	}

	var sqlFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			sqlFiles = append(sqlFiles, entry.Name())
		}
	}

	sort.Strings(sqlFiles)

	for _, filename := range sqlFiles {
		var applied bool
		if err := db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE filename = ?)`, filename).Scan(&applied); err != nil {
			return fmt.Errorf("check migration %s: %w", filename, err)
		}
		if applied {
			continue
		}

		contents, err := fs.ReadFile(migrationFS, filename)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", filename, err)
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", filename, err)
		}

		statements := strings.TrimSpace(string(contents))
		if statements != "" {
			if _, err := tx.ExecContext(ctx, statements); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("apply migration %s: %w", filename, err)
			}
		}

		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (filename) VALUES (?)`, filename); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %s: %w", filename, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", filename, err)
		}
	}

	return nil
}
