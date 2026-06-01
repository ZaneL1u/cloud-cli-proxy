---
phase: 58-sqlite
plan: 03
subsystem: persistence
tags: [sqlite, pgx-removal, database/sql, app-init, pragma]
requires:
  - 58-01 (SQLite 迁移文件 + migrator)
  - 58-02 (Repository 层 database/sql 重写)
provides:
  - App 层 SQLite 初始化 + PRAGMA WAL/foreign_keys/busy_timeout
  - 全项目 pgx 引用清除
  - HTTP handler + runtime database/sql 迁移
  - 嵌入迁移文件 (go:embed)
affects:
  - internal/controlplane/app/*
  - internal/controlplane/http/*
  - internal/runtime/runtime_service.go
  - internal/store/migrations/*
  - internal/store/repository/* (test fixes)
  - cmd/control-plane/main.go
tech-stack:
  added: ["modernc.org/sqlite (blank import in app)"]
  patterns: ["sql.Open + PRAGMA", "embed.FS for migrations", "*sql.Tx (was pgx.Tx)"]
key-files:
  created:
    - internal/store/migrations/embed.go
  modified:
    - internal/controlplane/app/app.go
    - internal/controlplane/app/seed_admin.go
    - internal/controlplane/app/app_test.go
    - internal/controlplane/app/seed_admin_test.go
    - internal/controlplane/app/testmain_test.go
    - internal/controlplane/http/admin_bindings.go
    - internal/controlplane/http/admin_bypass_bindings.go
    - internal/controlplane/http/admin_bypass_presets.go
    - internal/controlplane/http/admin_bypass_rules.go
    - internal/controlplane/http/admin_bypass_snapshots.go
    - internal/controlplane/http/admin_claude_accounts.go
    - internal/controlplane/http/admin_egress_ip_probe.go
    - internal/controlplane/http/admin_egress_ips.go
    - internal/controlplane/http/admin_hosts.go
    - internal/controlplane/http/admin_users.go
    - internal/controlplane/http/auth_handler.go
    - internal/controlplane/http/bootstrap_auth.go
    - internal/controlplane/http/bootstrap_handoff.go
    - internal/controlplane/http/bootstrap_status.go
    - internal/controlplane/http/entry.go
    - internal/controlplane/http/hosts.go
    - internal/controlplane/http/ssh_keys.go
    - internal/controlplane/http/user_hosts.go
    - internal/controlplane/http/user_password.go
    - internal/controlplane/http/admin_bindings_test.go
    - internal/controlplane/http/admin_bypass_bindings_test.go
    - internal/controlplane/http/admin_bypass_presets_test.go
    - internal/controlplane/http/admin_bypass_rules_test.go
    - internal/controlplane/http/admin_bypass_snapshots_test.go
    - internal/controlplane/http/admin_claude_accounts_test.go
    - internal/controlplane/http/admin_egress_ips_test.go
    - internal/controlplane/http/admin_hosts_test.go
    - internal/controlplane/http/admin_users_test.go
    - internal/controlplane/http/bootstrap_auth_test.go
    - internal/controlplane/http/entry_auth_test.go
    - internal/runtime/runtime_service.go
    - internal/runtime/tasks/worker_bypass_reload_test.go
    - internal/store/repository/migration_0019_test.go
    - internal/store/repository/queries_bypass_test.go
    - internal/store/repository/queries_claude_account_delete_test.go
    - internal/store/repository/queries_claude_account_volume_test.go
    - internal/store/repository/queries_contract_test.go
    - cmd/control-plane/main.go
decisions:
  - "sql.Open(\"sqlite\", DATABASE_URL) + PRAGMA journal_mode=WAL + foreign_keys=ON + busy_timeout=5000"
  - "SetMaxOpenConns(1) 单写入者，SetMaxIdleConns(1)，SetConnMaxLifetime(0)"
  - "App.db 类型从 *pgxpool.Pool 改为 *sql.DB"
  - "Config.MigrationDir 移除，迁移文件改用 internal/store/migrations/embed.go 内嵌"
  - "migrator.RunMigrations 签名改为 func(context.Context, *sql.DB, embed.FS) error"
  - "admin_claude_accounts_test.go: pgx.Tx stub 替换为内存 SQLite 真实事务"
  - "pgconn.PgError 错误检查替换为 strings.Contains(\"UNIQUE constraint failed\")"
  - "测试断言适配 SQLite 语法: is_system = FALSE → 0, NOW() → CURRENT_TIMESTAMP, 移除 FOR UPDATE"
requirements-completed: [DB-04, DB-05]
metrics:
  plan_start: "2026-06-01T07:00:00Z"
  completed_date: "2026-06-01T07:30:55Z"
  duration_minutes: 30
  task_count: 2
---

# Phase 58 Plan 03: 全项目 pgx 引用清除 + App SQLite 初始化

完成 Phase 58 最后一环：将 App 初始化改为 SQLite，清除全项目 pgx 依赖，所有 HTTP handler 和 runtime 迁移到 database/sql。

## One-liner

全项目 37 个文件的 pgx 引用清除，App 使用 sql.Open("sqlite") + PRAGMA WAL 初始化，go build ./... / go vet ./... / go test ./internal/... 全部通过。

## Task Summaries

### Task 1: app.go / seed_admin.go / queries_bypass.go 核心重写

- **Commit:** db13722
- **变更文件:** 7 files (+38 / -21)
- **内容:**
  - app.go: `pgxpool.ParseConfig` → `sql.Open("sqlite", ...)` + 三个 PRAGMA 语句 + `SetMaxOpenConns(1)`
  - app.go: `App.db` 类型从 `*pgxpool.Pool` 改为 `*sql.DB`
  - app.go: migrator 签名改为 `func(context.Context, *sql.DB, embed.FS) error`
  - seed_admin.go: `pgx.ErrNoRows` → `sql.ErrNoRows`
  - main.go: 移除 `MigrationDir` 字段
  - 新建 `internal/store/migrations/embed.go`：`//go:embed *.sql`
  - testmain_test.go: 注释更新

### Task 2: 全项目 pgx 残留清除 + 编译验证

- **Commit:** bbcffa8
- **变更文件:** 37 files (+247 / -270)
- **内容:**
  - 19 个 HTTP handler 文件：`pgx.ErrNoRows` → `sql.ErrNoRows`，导入 `database/sql`
  - `admin_claude_accounts.go`: `pgx.Tx` → `*sql.Tx`，`tx.Commit(ctx)` → `tx.Commit()`，`tx.Rollback(ctx)` → `tx.Rollback()`
  - `admin_bypass_snapshots.go`: `isUniqueViolation` 删除 `pgconn.PgError` 类型断言，改用 `strings.Contains("UNIQUE constraint failed")`
  - `runtime_service.go`: `pgx.ErrNoRows` → `sql.ErrNoRows`
  - 11 个测试文件：`pgx.ErrNoRows` → `sql.ErrNoRows`，导入替换
  - `admin_claude_accounts_test.go`: 重写为使用内存 SQLite 真实事务
  - 5 个 repository 测试文件：适配 SQLite 语法（`is_system = FALSE` → `0`，`NOW()` → `CURRENT_TIMESTAMP`，移除 `FOR UPDATE`）

## Verification Results

| 检查项 | 结果 |
|--------|------|
| `go build ./...` | PASS |
| `go build ./cmd/control-plane` | PASS |
| `go vet ./internal/...` | PASS |
| `go test ./internal/store/...` | PASS (全部通过，内存 SQLite) |
| `go test ./internal/controlplane/...` | PASS |
| `go test ./internal/...` (全部) | PASS |
| `grep jackc/pgx` (非测试源码) | 0 行（仅 `tests/e2e/helpers_linux.go` 残留，有 `//go:build e2e && linux` 约束，按计划延后适配） |
| `grep pgx.ErrNoRows` (非测试源码) | 0 行 |
| `grep pgx.Tx` (非测试源码) | 0 行 |
| `grep pgconn` (非测试源码) | 0 行 |
| `grep pgxpool` (非测试源码) | 0 行 |
| App 使用 sql.Open("sqlite") | YES |
| PRAGMA journal_mode=WAL 生效 | YES |
| PRAGMA foreign_keys=ON 生效 | YES |
| PRAGMA busy_timeout=5000 生效 | YES |
| db.SetMaxOpenConns(1) | YES |

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] embed.go 缺失（迁移文件 embed.FS 未创建）**
- **Found during:** Task 1
- **Issue:** 前序计划 (58-01, 58-02) 已将 migrator 迁移为 `embed.FS` 参数，但未创建内嵌迁移文件的 Go 文件
- **Fix:** 创建 `internal/store/migrations/embed.go`，使用 `//go:embed *.sql` 内嵌 24 个迁移文件
- **Files created:** `internal/store/migrations/embed.go`

**2. [Rule 1 - Bug] admin_claude_accounts_test.go pgx.Tx 接口不兼容**
- **Found during:** Task 2 go vet
- **Issue:** `AdminClaudeAccountStore.BeginTx` 返回类型从 `pgx.Tx` 改为 `*sql.Tx` 后，测试 stub 无法实现该接口
- **Fix:** 将测试从 pgx.Tx stub 重写为使用内存 SQLite (`sql.Open("sqlite", ":memory:")`) 创建真实事务，并通过 `repository.LockClaudeAccountForDelete` / `repository.DeleteClaudeAccountTx` 执行真实的 DB 操作
- **Files modified:** `internal/controlplane/http/admin_claude_accounts_test.go` (+145 / -145)

**3. [Rule 1 - Bug] 5 个 repository 测试断言与 SQLite 语法不兼容**
- **Found during:** Task 2 测试运行
- **Issue:** `is_system = FALSE` (PG) → `is_system = 0` (SQLite)，`NOW()` → `CURRENT_TIMESTAMP`，`FOR UPDATE` (不支持) → 移除
- **Fix:** 更新 queries_bypass_test.go, migration_0019_test.go, queries_claude_account_delete_test.go, queries_claude_account_volume_test.go, queries_contract_test.go 的断言
- **Files modified:** 5 test files

**4. [Rule 1 - Bug] isUniqueViolation 函数使用 pgconn.PgError 类型**
- **Found during:** Task 2 编译
- **Issue:** `pgconn` 包已移除，`isUniqueViolation` 需改用字符串匹配
- **Fix:** 删除 `var pgErr *pgconn.PgError` + `errors.As` 分支，改为 `strings.Contains(err.Error(), "UNIQUE constraint failed")`
- **Files modified:** `internal/controlplane/http/admin_bypass_snapshots.go`

## Threat Flags

| Flag | File | Description |
|------|------|-------------|
| threat_flag: error-matching | admin_bypass_snapshots.go | UNIQUE constraint 检测从 SQLSTATE 23505 结构化匹配降级为字符串匹配，存在同名字符串误匹配的低风险。SQLite 不提供 SQLSTATE 支持，此为可用方案。 |
| threat_flag: tx-isolation | admin_claude_accounts.go | *sql.Tx 默认隔离级别与 pgx.Tx 不同（SQLite 默认为 SERIALIZABLE vs PG 默认为 READ COMMITTED），可能影响并发删除 claude_account 时的行为。SetMaxOpenConns(1) 消除了并发写入竞争，保持语义等价。 |

## Known Stubs

无。

## Self-Check

- [x] 所有修改文件存在于 worktree
- [x] 两个 commit (db13722, bbcffa8) 存在于 git log
- [x] go build ./... 编译通过
- [x] go vet ./internal/... 无错误
- [x] go test ./internal/... 全部通过
- [x] grep jackc/pgx 非测试源码无匹配（除 e2e 带 build tag 文件）

## Self-Check: PASSED
