---
phase: quick-260513-gii-upserthost-sql
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/store/repository/queries.go
autonomous: true
requirements:
  - QUICK-UPSERTHOST-SQL-FIX
must_haves:
  truths:
    - "调用 POST /v1/admin/hosts 创建主机时不再返回 500 `{\"error\":\"create host failed\"}`"
    - "UpsertHost 的 INSERT 语句中列数与 VALUES 占位符数量一致（均为 12）"
    - "ON CONFLICT DO UPDATE SET 子句不再包含因移除 host_ports 残留的空白行"
  artifacts:
    - path: "internal/store/repository/queries.go"
      provides: "修复后的 UpsertHost SQL 语句"
      contains: "VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)"
  key_links:
    - from: "internal/store/repository/queries.go::UpsertHost"
      to: "PostgreSQL hosts 表（12 个目标列）"
      via: "QueryRow INSERT...ON CONFLICT...RETURNING"
      pattern: "INSERT INTO hosts.*VALUES \\(\\$1.*\\$12\\)"
---

<objective>
修复 `UpsertHost` SQL 语句中 INSERT 列数（12）与 VALUES 占位符数量（13）不一致导致的创建主机 500 错误。

目的：恢复 `POST /v1/admin/hosts` 接口的正常工作，让前端能再次创建主机记录。
产出：`internal/store/repository/queries.go` 中 `UpsertHost` 函数的 SQL 字符串修复。
</objective>

<execution_context>
@.claude/get-shit-done/workflows/execute-plan.md
</execution_context>

<context>
@CLAUDE.md
@.planning/STATE.md

# 修改目标文件
@internal/store/repository/queries.go

# 背景：commit 74c1502 "彻底移除端口映射特性" 删除 host_ports 列时漏掉了：
# 1. VALUES 子句末尾的 $13 占位符
# 2. DO UPDATE SET 中 `host_ports = EXCLUDED.host_ports,` 这一行留下的空白行
# Go 调用方只传 12 个参数（params.UserID ... mountsJSON），与列数一致，所以唯一不一致点就在 SQL 字符串。

<interfaces>
当前 queries.go:381-398 关键片段（修复前）：

```go
INSERT INTO hosts (user_id, status, short_id, template_image_ref, home_volume_name, slot_key, timezone, hostname, memory_limit_mb, cpu_limit, disk_limit_gb, host_mounts)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)   -- ← BUG: 多余的 $13
ON CONFLICT (user_id, slot_key)
DO UPDATE SET
    status = EXCLUDED.status,
    template_image_ref = EXCLUDED.template_image_ref,
    home_volume_name = EXCLUDED.home_volume_name,
    timezone = EXCLUDED.timezone,
    hostname = EXCLUDED.hostname,
    memory_limit_mb = EXCLUDED.memory_limit_mb,
    cpu_limit = EXCLUDED.cpu_limit,
    disk_limit_gb = EXCLUDED.disk_limit_gb,
    host_mounts = EXCLUDED.host_mounts,
                                              -- ← BUG: 空白行（74c1502 删除 host_ports = ... 后留下）
    updated_at = NOW()
```

Go 参数列表（queries.go:400-411，无需修改）：
1. params.UserID
2. params.Status
3. params.ShortID
4. params.TemplateImageRef
5. params.HomeVolumeName
6. params.SlotKey
7. params.Timezone
8. params.Hostname
9. memoryLimitMB
10. cpuLimit
11. diskLimitGB
12. mountsJSON
（共 12 个 → 与修复后的 12 个占位符对齐）
</interfaces>
</context>

<tasks>

<task type="auto">
  <name>Task 1: 修复 UpsertHost SQL 占位符数量与残留空白行</name>
  <files>internal/store/repository/queries.go</files>
  <action>
针对 `internal/store/repository/queries.go` 中 `UpsertHost` 函数做两处精确修改，**不要改动 Go 参数列表、不要改动测试**：

1. **第 382 行：删除多余的 `$13` 占位符**
   - 原文：`VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`
   - 改为：`VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`

2. **第 393-394 行：清理 `host_mounts = EXCLUDED.host_mounts,` 后面的空白残留行**
   - 当前结构：
     ```
     host_mounts = EXCLUDED.host_mounts,
     <这里有一行只含空白字符，是 74c1502 删除 host_ports = EXCLUDED.host_ports, 后遗留>
     updated_at = NOW()
     ```
   - 改为（删掉中间那行空白行，让 `host_mounts = EXCLUDED.host_mounts,` 与 `updated_at = NOW()` 紧邻）：
     ```
     host_mounts = EXCLUDED.host_mounts,
     updated_at = NOW()
     ```

约束：
- 仅修改这两处，保持函数其余逻辑（默认值处理、Scan 字段、错误处理）完全不变。
- 不要改动 RETURNING 子句、ON CONFLICT 目标、Go 参数顺序。
- 不要新增或删除测试文件；`models_test.go::TestUpsertHostParams_AllFields` 是结构体字段测试，不涉及 SQL 字符串，无需变更。

修改后用 `grep` 自检：
- `grep -n "VALUES (\\$1" internal/store/repository/queries.go` 应返回包含 `$12)` 结尾、**不含 `$13`** 的那一行。
- `grep -n "host_mounts = EXCLUDED.host_mounts" internal/store/repository/queries.go` 接下来非空一行应是 `updated_at = NOW()`。
  </action>
  <verify>
    <automated>cd /Users/zaneliu/Projects/open-source/cloud-cli-proxy-main && go vet ./internal/store/... && go build ./internal/store/... && grep -n 'VALUES (\$1' internal/store/repository/queries.go | grep -v '\$13' | grep -q '\$12)' && ! grep -A1 'host_mounts = EXCLUDED.host_mounts,' internal/store/repository/queries.go | head -2 | tail -1 | grep -qE '^[[:space:]]*$'</automated>
  </verify>
  <done>
- `internal/store/repository/queries.go` 第 382 行的 VALUES 子句仅有 12 个占位符（`$1` 到 `$12`），无 `$13`。
- `host_mounts = EXCLUDED.host_mounts,` 与 `updated_at = NOW()` 之间不再有残留空白行。
- `go vet ./internal/store/...` 通过。
- `go build ./internal/store/...` 通过。
- 函数其余部分（参数列表、Scan、错误处理）保持不变。
  </done>
</task>

</tasks>

<verification>
1. 静态检查：`go vet ./internal/store/...` 与 `go build ./internal/store/...` 均通过。
2. 结构自检：`grep` 校验 SQL 语句已修复为 12 占位符且无残留空白行。
3. 单元测试（若存在）：`go test ./internal/store/repository/... -count=1` 通过（本修复仅改 SQL 字符串，不影响现有测试）。
4. 运行时验证（手动，可选）：在本地 Postgres 启动后，调用 `POST /v1/admin/hosts` 不再返回 500；PostgreSQL 不再报 "INSERT has more expressions than target columns"。
</verification>

<success_criteria>
- `UpsertHost` 的 INSERT 列数（12）与 VALUES 占位符数（12）严格一致。
- 残留的空白行被清理，`DO UPDATE SET` 子句语法清爽。
- Go 编译与 `go vet` 通过；现有测试不受影响。
- 创建主机接口不再因 SQL 语法错误返回 500。
</success_criteria>

<output>
完成后创建总结：`.planning/quick/260513-gii-upserthost-sql/260513-gii-SUMMARY.md`，记录：
- 修改位置（queries.go:382 与 queries.go:393-394）
- 修复前后 SQL 片段对比
- 验证命令的实际输出
- 关联 commit `74c1502` 作为上下文
</output>
