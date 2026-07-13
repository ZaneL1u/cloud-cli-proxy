# 出口 IP 唯一约束修复实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让多条代理线路可以处于未检测状态或共享同一真实出口 IP，并通过
`v4.2.19` 自动升级现有 SQLite 数据库。

**Architecture:** 使用 SQLite 表重建迁移移除 `egress_ips.ip_address` 的唯一
约束，同时临时备份并恢复引用它的绑定表。API 和前端统一用空字符串表达尚未
检测的 IP，真实地址继续由探测流程写入。

**Tech Stack:** Go 1.26、`database/sql`、`modernc.org/sqlite`、React 19、
TypeScript、Vitest、GitHub Actions。

## Global Constraints

- v1 仍为单宿主机部署，不改变容器、隧道或出口验证架构。
- 每个用户容器仍必须绑定至少一个出口资源。
- 不新增第三方运行时依赖。
- 所有仓库路径使用项目根目录相对路径。
- 发布版本固定为 `v4.2.19`。

---

### Task 1: SQLite 升级迁移

**Files:**
- Create: `internal/store/repository/migration_0003_test.go`
- Create: `internal/store/migrations/0003_drop_egress_ip_address_unique.sql`

**Interfaces:**
- Consumes: `migrator.RunMigrations(context.Context, *sql.DB, embed.FS)`
- Produces: 最终结构中 `egress_ips.ip_address TEXT NOT NULL` 且不唯一

- [x] **Step 1: 写失败的升级测试**

测试先建立仅应用 `0001`、`0002` 的旧数据库，写入用户、主机、出口和绑定，
再通过 migrator 应用 `0003`。断言重复 IP 可插入、绑定保留、外键仍生效。

- [x] **Step 2: 运行测试确认红灯**

Run: `go test ./internal/store/repository -run TestMigration0003 -count=1`

Expected: 因缺少 `0003_drop_egress_ip_address_unique.sql` 或重复 IP 唯一冲突失败。

- [x] **Step 3: 实现最小迁移**

迁移按以下顺序执行：备份 `host_egress_bindings`、删除子表、重建
`egress_ips`、复制数据、恢复子表和索引、删除备份表。

- [x] **Step 4: 运行迁移测试确认绿灯**

Run: `go test ./internal/store/repository -run TestMigration0003 -count=1`

Expected: PASS。

### Task 2: API 空 IP 契约

**Files:**
- Modify: `internal/controlplane/http/admin_egress_ips_test.go`
- Modify: `internal/controlplane/http/admin_egress_ips.go`

**Interfaces:**
- Consumes: `createEgressIPRequest.IPAddress`、`updateEgressIPRequest.IPAddress`
- Produces: 空字符串合法；非空值必须通过 `net.ParseIP`

- [x] **Step 1: 写创建空 IP 和更新非法 IP 的失败测试**

创建请求传 `ip_address: ""` 应返回 201；更新请求传 `not-an-ip` 应返回 400。

- [x] **Step 2: 运行测试确认红灯**

Run: `go test ./internal/controlplane/http -run TestAdminEgressIPsHandler -count=1`

Expected: 创建空 IP 用例得到 400，或更新非法 IP 用例得到 200。

- [x] **Step 3: 实现统一校验**

对创建和更新都执行 `strings.TrimSpace`；仅在非空时调用 `net.ParseIP`。

- [x] **Step 4: 运行 API 测试确认绿灯**

Run: `go test ./internal/controlplane/http -run TestAdminEgressIPsHandler -count=1`

Expected: PASS。

### Task 3: 前端不再生成占位 IP

**Files:**
- Create: `web/admin/src/lib/egress-ip-address.ts`
- Create: `web/admin/src/lib/__tests__/egress-ip-address.test.ts`
- Modify: `web/admin/src/components/egress-ips/egress-ip-drawer.tsx`

**Interfaces:**
- Produces: `normalizeEgressIPAddress(value: string): string`
- Consumes: 表单 `values.ip_address`

- [x] **Step 1: 写失败的纯函数测试**

断言空白输入返回空字符串，显式 IP 去除首尾空格后保持原值。

- [x] **Step 2: 运行测试确认红灯**

Run: `pnpm test:unit -- src/lib/__tests__/egress-ip-address.test.ts`

Expected: 因模块或导出不存在失败。

- [x] **Step 3: 实现函数并接入提交载荷**

`normalizeEgressIPAddress` 只执行 `trim()`；抽屉提交时直接把结果放入
`ip_address`，删除 `0.0.0.0` 回退逻辑和未使用的正则。

- [x] **Step 4: 运行前端测试确认绿灯**

Run: `pnpm test:unit -- src/lib/__tests__/egress-ip-address.test.ts`

Expected: PASS。

### Task 4: 验证与发布

**Files:**
- Modify: `.planning/debug/egress-ip-unique-collision.md`

- [x] **Step 1: 运行格式化与定向验证**

Run: `gofmt -w internal/controlplane/http/admin_egress_ips.go internal/controlplane/http/admin_egress_ips_test.go internal/store/repository/migration_0003_test.go`

Run: `go test ./internal/store/... ./internal/controlplane/http/...`

Run: `pnpm test:unit && pnpm build`

Run: `pnpm typecheck`，并与 `origin/main` 基线比较；本次不新增类型错误。

- [x] **Step 2: 运行全量 Go 验证**

Run: `go test ./...`

Expected: PASS。

- [x] **Step 3: 检查隐私、差异和迁移结构**

确认新增文件没有绝对路径、凭据或个人信息；确认迁移后
`PRAGMA foreign_key_check` 无结果。

- [ ] **Step 4: 提交并发布**

提交聚焦改动，推送修复分支，合入 `main`，在合入提交上创建并推送
`v4.2.19` 标签。

- [ ] **Step 5: 监控发布完成**

确认 Release workflow、GitHub Release、控制面镜像和 managed-user 镜像均成功。
