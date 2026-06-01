---
phase: 58-sqlite
plan: 01
subsystem: database
tags: [sqlite, modernc, database/sql, embed, migrator, go-mod]

requires:
  - phase: none
    provides: baseline pgx/v5 migrator
provides:
  - "modernc.org/sqlite v1.51.0 直接依赖已加入 go.mod"
  - "google/uuid v1.6.0 已加入 go.mod"
  - "migrator.go 完全重写为 database/sql + embed.FS + SQLite 语法"
  - "SQLite 驱动注册 (_ modernc.org/sqlite) 在 migrator 包"
affects: [58-sqlite-plan-02, 58-sqlite-plan-03]

tech-stack:
  added: [modernc.org/sqlite v1.51.0, google/uuid v1.6.0, database/sql, embed]
  patterns: ["embed.FS 替代 filepath.Glob 文件发现", "database/sql 标准 API 替代 pgx 专有 API", "? 占位符替代 $N PG 语法"]

key-files:
  created: []
  modified:
    - go.mod
    - go.sum
    - internal/store/migrator/migrator.go

key-decisions:
  - "modernc.org/sqlite 驱动注册临时放在 migrator.go 的 blank import，Plan 02 迁移至 app.go"
  - "pgx/v5 暂留在 go.mod 中（其他文件仍引用），Plan 02/03 完成后由 go mod tidy 自动移除"
  - "google/uuid 标记为 indirect（无文件直接 import），Plan 02 重写 repository 后升为 direct"

patterns-established:
  - "embed.FS 迁移文件注入: 调用方通过 //go:embed 注入 SQL 文件，migrator 按文件名排序执行"
  - "SQLite schema_migrations: TEXT PRIMARY KEY + TEXT DEFAULT CURRENT_TIMESTAMP"

requirements-completed: [DB-01, DB-02]

duration: 18min
completed: 2026-06-01
---

# Phase 58 Plan 01: SQLite 依赖切换与迁移执行器重写 Summary

**migrator.go 从 pgxpool.Pool + filepath.Glob 重写为 database/sql + embed.FS + SQLite 语法，go.mod 新增 modernc.org/sqlite 直接依赖**

## Performance

- **Duration:** 18 min
- **Started:** 2026-06-01T06:34:00Z
- **Completed:** 2026-06-01T06:51:51Z
- **Tasks:** 2
- **Files modified:** 3

## Accomplishments
- migrator.go 完全重写：移除 pgxpool.Pool 依赖，使用 database/sql 标准库 + embed.FS 加载迁移文件
- go.mod 新增 modernc.org/sqlite v1.51.0 直接依赖和 google/uuid v1.6.0
- SQLite 语法适配：$1 占位符改为 ?，TIMESTAMPTZ 改为 TEXT，NOW() 改为 CURRENT_TIMESTAMP
- 迁移文件发现从 filepath.Glob 改为 fs.ReadDir，支持嵌入式文件系统

## Task Commits

1. **Task 1 + Task 2: 切换依赖 + 重写 migrator.go** - `13d5cbc` (feat)
   - 两个任务因 go.mod 依赖关系紧密耦合，合并为单次提交
   - migrator.go 重写是 go.mod 依赖切换的前置条件（否则 go mod tidy 会重新添加 pgx）

**Plan metadata:** (pending) (docs: complete plan)

## Files Created/Modified
- `go.mod` - 新增 modernc.org/sqlite v1.51.0 直接依赖，google/uuid v1.6.0 间接依赖
- `go.sum` - 新增 modernc.org/sqlite 及其传递依赖的哈希条目
- `internal/store/migrator/migrator.go` - 完全重写为 database/sql + embed.FS 实现

## Decisions Made
- **SQLite 驱动注册位置：** 临时放在 migrator.go 的 blank import (`_ "modernc.org/sqlite"`)，Plan 02 重写 app.go 时迁移至更合适的位置
- **pgx 暂留策略：** pgx/v5 仍在 go.mod 中，因为 queries.go、app.go 等文件仍引用它。Plan 02/03 完成全部迁移后，`go mod tidy` 会自动移除
- **google/uuid 间接依赖：** 当前无 Go 文件直接 import google/uuid，标记为 `// indirect`。Plan 02 重写 repository 层时会改为直接依赖

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Task 1 和 Task 2 合并执行**
- **Found during:** Task 1
- **Issue:** go.mod 依赖切换需要 migrator.go 先移除 pgx 引用，否则 `go mod tidy` 会重新添加 pgx 依赖
- **Fix:** 先完成 migrator.go 重写（Task 2），再运行 `go mod tidy` 清理依赖
- **Files modified:** go.mod, go.sum, internal/store/migrator/migrator.go
- **Verification:** migrator.go 无 pgx 引用，go.mod 包含 modernc.org/sqlite
- **Committed in:** 13d5cbc

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** 合并执行是必要的，因为 go.mod 和 migrator.go 存在循环依赖关系。无功能影响。

## Partial Acceptance Criteria

- ✅ go.mod 包含 modernc.org/sqlite（直接依赖）
- ✅ go.mod 包含 google/uuid（间接依赖）
- ⚠️ go.mod 仍包含 jackc/pgx/v5（因 queries.go/app.go 等文件仍引用，Plan 02/03 完成后移除）
- ✅ migrator.go 使用 database/sql + embed.FS
- ✅ migrator.go RunMigrations 签名为 `(ctx context.Context, db *sql.DB, migrationFS embed.FS) error`
- ✅ schema_migrations 使用 TEXT + CURRENT_TIMESTAMP
- ✅ 所有 SQL 占位符为 ? 而非 $1
- ✅ 使用 db.ExecContext / db.QueryRowContext / db.BeginTx

## Issues Encountered
- `go mod tidy` 会重新添加 pgx/v5：因为项目中仍有 20+ 文件引用 `github.com/jackc/pgx/v5`。这是预期行为，Plan 02/03 完成后会自动清理。

## Next Phase Readiness
- migrator.go 已完全兼容 SQLite，可直接用于 Plan 02 的迁移文件改写
- modernc.org/sqlite 驱动已注册，Plan 03 的 app.go 可直接 `sql.Open("sqlite", dsn)`
- pgx 依赖清理需等 Plan 02/03 完成 queries.go 和 app.go 重写后统一执行

---
*Phase: 58-sqlite*
*Completed: 2026-06-01*

## Self-Check: PASSED

- FOUND: internal/store/migrator/migrator.go
- FOUND: go.mod
- FOUND: 58-01-SUMMARY.md
- FOUND: commit 13d5cbc
