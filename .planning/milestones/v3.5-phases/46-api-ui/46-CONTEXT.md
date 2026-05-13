# Phase 46: 控制面 API 与后台 UI - Context

**Gathered:** 2026-05-12
**Status:** Ready for planning
**Mode:** smart_discuss auto-optimized (batch recommendations，对齐 Phase 45 数据基础设施 + 现有 admin_egress_ips / egress-ip-drawer 风格)

<domain>
## Phase Boundary

让管理员能在后台对每个 host 完成「勾预设 → 加规则 → 预览 diff → 一键 apply / rollback」闭环，并以护栏校验和审计日志保证操作可追踪、可回滚、不可越权。

**本阶段交付：**
- **5 API 套件**：preset / rule 全 CRUD + validate + host-bypass 绑定 CRUD + preview / apply / rollback / effective + audit log 写入
- **5 UI 模块**：host 详情页 Bypass Tab + 预设卡片（loopback 锁定 / lan 可勾选）+ 自定义规则 CRUD + preview 面板（sing-box JSON / nft diff 切换）+ 应用进度反馈

**不在本阶段范围：**
- agent reload 链路（Phase 47）
- nft set 原子更新 + 安全不变量 CI（Phase 47）
- 用户自助配置（v3.6+）

</domain>

<decisions>
## Implementation Decisions

### API 设计风格（沿用 admin_egress_ips.go 范式）

- **Handler 模式**：每个资源一个 `Admin{Resource}Handler` struct（`logger / store / events`），构造函数 `NewAdminXXXHandler`。
- **Store 接口**：定义在 handler 文件内（如 `AdminBypassStore`），聚合需要的 Repository 方法子集 —— 解耦 handler 测试与真实 Repository。
- **JSON 验证**：每个 raw JSON 字段单独 `validateBypassRule(raw json.RawMessage) error` 函数，错误返回中文提示（与现有项目错误信息风格对齐 —— 现有 `admin_egress_ips.go` 用英文，本阶段沿用英文 error message 给 API/前端解析，但中文文案放在前端 toast）。
- **路径风格**：复数资源名 `/v1/admin/bypass/presets` / `/v1/admin/bypass/rules`（与现有 `/v1/admin/egress-ips` 风格一致）。
- **错误码**：HTTP body 含 `{"code":"BYPASS_RULE_TOO_BROAD","message":"..."}` 结构，code 命名全部 `BYPASS_*` 前缀，沿用 `cloud-claude/errcodes/` 现有的 SCREAMING_SNAKE_CASE 模式。
- **JWT 鉴权**：复用现有 `middleware.RequireJWT` 链；无新中间件。
- **Pagination**：preset/rule list 不分页（系统预设少 + 自定义规则单 host < 1000 条上限）；只在 audit_log list 加 `?limit=100&before=<created_at>` cursor。
- **幂等**：apply 用 `config_hash` 作为 unique key（已在 0019 schema 设计），重复 POST 同 hash 返回 200 + 现有 snapshot id，不重复写。

### 护栏（硬拦截）实现

- 5 条护栏触发 HTTP 422 + 错误码（验证逻辑在 handler，不在 repository）：
  - 全量绕过 (`0.0.0.0/0` / `::/0`) → `BYPASS_RULE_TOO_BROAD`
  - v4 CIDR < /16 且非 RFC1918 / CGNAT / loopback → `BYPASS_RULE_TOO_BROAD`
  - `domain_suffix` < 4 字符或 TLD（`.com` / `.net` / `.org` 等列入硬拦截列表）→ `BYPASS_RULE_TOO_BROAD`
  - 规则 CIDR 覆盖代理服务器 IP → `BYPASS_RULE_CONFLICT_PROXY`
  - host 有效规则 > 1000 → `BYPASS_LIMIT_EXCEEDED`
- `domain_keyword` < 4 字符返回 HTTP 400 + 警告字段 `risky:true`，要求请求体携带 `confirm_risky:true` 才允许保存（软拦截，UI 弹二次确认）。
- 护栏列表（TLD 黑名单 / 私有段白名单 / proxy IP 检测）放 `internal/controlplane/http/bypass_validation.go`，单独单元测试。

### 审计日志写入

- 所有写操作（create / update / delete / bind / unbind / apply / rollback）使用 Repository `InsertBypassAuditLog`，由 handler 在事务内/外按需调用。
- 字段填法：`actor_id` = JWT subject claim；`actor_ip` = `r.RemoteAddr`（如果有 trusted proxy header 设置则用 `X-Forwarded-For` 第一个 IP，沿用现有 middleware helper）；`action`、`target_kind`、`target_id` 显式；`before` / `after` JSONB 直接 `json.Marshal`；`note` 来自请求体可选字段。
- 90 天保留策略：本阶段只设计字段，不实现 retention worker（cron 留 v3.5 P1）。

### 后台 UI 风格（沿用 `egress-ips/` 路由 + shadcn + Tailwind v4 + Radix UI）

- **Tab 集成**：host 详情页 `$hostId.tsx` 加 `tabs/bypass.tsx` 路由（Tab 顺序：Overview / Egress / Bypass / Mounts / Audit）；URL 用 `?tab=bypass` query 而非 hash（与现有 router 一致）。
- **预设卡片布局**：3 列网格（loopback / lan / 占位 disabled，未来 cn-dev 等）；每张卡片显示 slug + 名称 + 包含规则数 + 强制开启徽章；`loopback` checkbox disabled + 灰色锁图标 + tooltip "loopback 强制启用，不可关闭"；hover 显示规则示例（Popover）。
- **自定义规则表**：TanStack Table，列 = 类型 / 值 / 端口 / 风险 / 备注 / 操作；类型筛选下拉 + 全文搜索；高风险（`is_risky=true`）行左侧黄色边框 + 黄色徽章 "Risky"；新建/编辑用 Drawer（沿用 `egress-ip-drawer.tsx` 模式）；类型选择 5 选 1（IP / CIDR / 域名 / 域名后缀 / 域名关键词）+ port 字段（可选）。
- **预览面板**：右侧滑出（`Sheet` from shadcn）；两个 Tab —— "sing-box JSON" 用 Monaco editor read-only + JSON 语法高亮，"nft set diff" 用 unified diff highlighter；上方显示 "v23 → v24" 版本号和 "覆盖 X 条规则" 摘要；底部一个 "应用配置" 主按钮 + "取消" 次按钮；高风险规则数 > 0 时主按钮变黄并需要二次点击确认。
- **应用进度反馈**：底部 Toast 不够 —— 用专门的 `ApplyProgressDialog` modal，5 阶段步骤条（生成快照 / 下发 agent / reload / 健康检查 / 完成），每个阶段 spinner / check icon / error icon；总耗时 < 5s 时 dialog 自动关闭并 toast "白名单变更不影响现有 TCP 连接，新连接才用新规则"。
- **二次确认**：高风险动作（删除自定义规则、apply 含 > 5 条高风险规则的 snapshot、rollback）用 `AlertDialog` 弹窗 + 输入 host slug 确认（与 egress IP 删除 UX 对齐）。

### React Query 缓存策略

- `useBypassPresets()`、`useBypassRules(hostId)`、`useBypassEffective(hostId)`、`useBypassAuditLog(hostId)` 4 个 hook，`staleTime: 30_000`；apply / rollback 成功后 `invalidate(['bypass', hostId])` 全部失效。
- Optimistic update 仅用于规则启用/禁用 toggle，其他写操作走默认 refetch。
- error boundary 显示错误码 + 中文文案，复用 `cloud-claude/errcodes` 现有的错误码 → 中文 message 映射（如果不存在则在前端 `lib/bypass-error-codes.ts` 自建）。

### 前后端契约同步

- TypeScript 类型从 OpenAPI 不行（项目无），改为：在 `web/admin/src/lib/api/types/bypass.ts` 手写 interface，每个字段与 `internal/store/repository/models.go` 注释对齐；不引入 codegen。
- Repository struct 上加 JSON tag（`pgx` 已读 db tag，JSON tag 独立）；handler `json.NewDecoder/Encoder` 不做大小写转换，前端 interface 用 snake_case 字段名对齐 API 实际 payload。

### Claude's Discretion（Plan 阶段可自由决定）

- 5 个 API handler 是否拆 5 个 `*.go` 文件（preset / rule / binding / preview / audit_log）还是塞同一个 `admin_bypass.go` ≈ 600 行 —— 建议拆 5 个文件 + 一个共享 helper 文件 `bypass_validation.go`；planner 视情况调整
- React 组件文件粒度：放 `web/admin/src/components/bypass/` 目录下，每个组件单独文件 vs 一个大文件 —— 默认每个组件单独文件
- preview 接口的 `nft set diff` 内容如何生成 —— 由 control-plane 拼接（参考 Phase 45 rule-set placeholder + Phase 47 即将实现的 nft 规则模板），具体格式由 planner 决定
- Drawer vs Modal vs Sheet 的具体 shadcn 组件选择，由 UI-SPEC 阶段最终敲定（如果 UI-SPEC 推翻这里的建议，以 UI-SPEC 为准）
- Tab 之间状态保留策略（URL query vs context）

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets

| 资产 | 路径 | 复用方式 |
|------|------|----------|
| Handler 模板 | `internal/controlplane/http/admin_egress_ips.go` | 仿写 5 个 `Admin*` handler；同样的 Store interface + validate pattern |
| Handler 测试模板 | `internal/controlplane/http/admin_egress_ips_test.go` | 仿写 5 个 handler test 集 |
| Probe pattern | `admin_egress_ip_probe.go` | preview / apply / rollback 借鉴 probe 这种「非 CRUD action endpoint」的实现 |
| 错误码注册 | `internal/cloudclaude/errcodes/` | 新增 BYPASS_* 错误码 |
| middleware.RequireJWT | 现有 admin middleware | 直接复用 |
| Repository CRUD | `internal/store/repository/queries_bypass.go` (Phase 45) | 19 个方法 + ErrSystemBypassPresetImmutable sentinel 已就绪 |
| Tab 路由模板 | `web/admin/src/routes/_dashboard/hosts/$hostId.tsx` | 加 Bypass Tab |
| Drawer 模板 | `web/admin/src/components/egress-ips/egress-ip-drawer.tsx` | 仿写 bypass rule drawer |
| EgressIPs CRUD UI | `web/admin/src/routes/_dashboard/egress-ips/` | 仿写预设管理（admin 全局） |
| API client | `web/admin/src/lib/api/` | 加 `bypass.ts` |

### Established Patterns

- **后端**：标准库 `net/http` + Go 1.22 mux + JWT；Store interface 注入 handler；返回 JSON `{code, message, data}` 三段；validate 函数返回 `error`，handler 根据 errors.Is / type-switch 决定 HTTP 状态码
- **前端**：TanStack Router + TanStack Query + Radix UI + Tailwind v4；shadcn 自建组件库（不 import shadcn npm 包，本地复制源码）；i18n 中文文案直接写组件内 / 错误码 map
- **测试**：handler 单元测试用 `httptest.NewRecorder()` + table-driven；前端 vitest + @testing-library/react，组件测试用 mock Query client
- **审计事件**：写操作通过 `EventRecorder.Record(ctx, event)` 异步落库（不阻塞 handler 响应），与 audit_log 表是双轨制（事件流 + audit_log 详细 diff）

### Integration Points

- **Repository 注入**：`internal/controlplane/server.go` 构造时把 Phase 45 已就绪的 Bypass* 方法注入 5 个新 handler
- **Route registration**：`internal/controlplane/server.go::setupRoutes` 注册 `/v1/admin/bypass/*` 路由
- **前端 entry**：`web/admin/src/routeTree.gen.ts` 自动生成；只需新增 `routes/_dashboard/hosts/$hostId/bypass.tsx` 或者在现有 `$hostId.tsx` 加 Tab
- **错误码 i18n**：`web/admin/src/lib/i18n/error-codes.ts` 新增 BYPASS_* 文案
- **agent reload trigger**：apply 接口 dispatch worker task `ActionReloadHostBypass`（**Phase 47 实现**，Phase 46 只占用 action 字符串常量，dispatcher case 留 `// TODO Phase 47` 跳过实际执行 —— 或者 Phase 46 完全不发起 dispatch，apply 仅落 snapshot 状态为 `pending` 等 Phase 47 接管）

### 现状 vs 目标差异

| 维度 | 当前 | Phase 46 目标 |
|------|------|--------------|
| API | 仅 egress-ips CRUD | + 5 Bypass handler + preview/apply/rollback |
| 护栏 | 仅 egress IP validate | + Bypass 5 条硬拦截 + 1 条软拦截 |
| 审计 | EventRecorder 异步事件 | + `host_bypass_audit_log` 详细 diff 落库 |
| UI Tab | Overview / Egress / Mounts | + Bypass |
| 错误码 | EGRESS_* / SSH_* | + BYPASS_* (10+ 个) |

</code_context>

<specifics>
## Specific Ideas

- `BypassPreset` / `BypassRule` 列表 API 返回 `data: {presets / rules: []}` 顶层包裹（与 egress-ips API 风格一致），不直接返回数组
- `preview` / `apply` 接口的请求体含 `preset_ids: []string` + `rule_ids: []string` 显式列表，不传 host_id 之外的隐式状态
- `rollback` 接口幂等：传 `target_snapshot_id`，已经回滚到该 snapshot 时返回 200 + 现有结果
- `effective` 接口返回 4 段：`presets_active`、`rules_active`、`whitelist_cidrs_rendered`、`whitelist_domains_rendered`，便于前端做对账显示
- audit log 默认 `?limit=20`，最大 `?limit=200`；cursor 用 `?before=<created_at_iso>`
- 应用进度 dialog 5 步骤的阶段名固定（中文）："生成快照" / "下发到 agent" / "Reload 配置" / "健康检查" / "完成"
- `domain_keyword` 风险确认 UX：用 AlertDialog 显示 "keyword『abc』< 4 字符可能误命中其他域名（如 abc.com / abcdef.org）" + 复选框 "我确认接受风险"
- preview JSON 视图限制大小：rule-set 文件 > 10000 行时折叠成「点击展开」按钮防止浏览器卡死
- nft diff 颜色：绿色 `+` 新增 / 红色 `-` 删除 / 默认色 unchanged context（≤ 3 行）

</specifics>

<deferred>
## Deferred Ideas

- 灰度按钮「先在测试 host 验证」→ v3.5 P1（`BYPASS-CANARY`）
- 流量 dashboard / 命中统计 → v3.5 P1（`BYPASS-DASHBOARD` / `BYPASS-HIT-STATS`）
- 跨 host 批量 apply → 未来再加
- preset 编辑器（添加新预设规则）→ v3.6+（v3.5 只允许编辑非 system 预设，新增 preset 留待 P1）
- 用户自助配置 → v3.6+（`BYPASS-USER-SELF`）
- API codegen / OpenAPI schema → 不引入（保持手写 TypeScript types 简洁）
- 复合规则编辑器 / domain_regex → 已在 REQUIREMENTS.md `Out of Scope` 显式排除

</deferred>
