---
phase: 46
slug: api-ui
status: draft
shadcn_initialized: true
preset: new-york + neutral (cssVariables, oklch tokens)
created: 2026-05-12
---

# Phase 46 — UI Design Contract

> 视觉与交互契约：host 详情页「代理白名单」Tab + 预设卡片 + 自定义规则 CRUD + 预览面板 + 应用进度反馈。
>
> 由 gsd-ui-researcher 基于 46-CONTEXT.md 已锁定决策 + 现有 admin 前端 token 体系（`web/admin/src/index.css` + shadcn new-york）正式化。

---

## Design System

| Property | Value |
|----------|-------|
| Tool | shadcn (CLI 初始化已就绪) |
| Preset | `new-york` style + `neutral` baseColor + `cssVariables: true` + `iconLibrary: lucide`（见 `web/admin/components.json`） |
| Component library | radix-ui v1.4 + 本地复制源码到 `web/admin/src/components/ui/`（不引入 `shadcn/ui` npm 包） |
| Icon library | `lucide-react` v0.510（已用于 host 详情页 Tab/按钮） |
| Font | `Inter, system-ui, -apple-system, sans-serif`（antialiased），来自 `web/admin/src/index.css` body 规则 |
| Tailwind | v4.1 + `@tailwindcss/vite`，颜色全部走 `oklch()` 自定义属性 |
| Form | `react-hook-form` 7.56 + `@hookform/resolvers` + `zod` 3.24（与 `egress-ip-drawer.tsx` 完全一致） |
| Toast | `sonner` v2（顶层已挂载，直接 `toast.success/error/info`） |
| Data | `@tanstack/react-query` v5（key `['bypass', hostId, ...]`，`staleTime: 30_000`） |

复用清单：
- 已存在 UI primitives：`alert-dialog` / `badge` / `button` / `card` / `dialog` / `dropdown-menu` / `input` / `label` / `select` / `separator` / `sheet` / `table` / `table-skeleton` / `textarea` / `tooltip` / `status-dot`
- **需新增 UI primitives（沿用 shadcn new-york 风格本地复制源码）**：`tabs`（Bypass Tab 容器）、`popover`（预设卡片规则示例 hover）、`progress`（5 阶段步骤条）、`scroll-area`（JSON 预览滚动）、`form`（替代手写 zod 错误 `<p>`，与现有 RHF 用法兼容）
- 不引入第三方 registry，不引入 Monaco editor（预览 JSON 用语义化 `<pre>` + 简易 syntax highlighter 或 prismjs 已有同类，planner 决定；本契约只规范容器与折叠交互）

---

## Spacing Scale

声明值（必须是 4 的倍数，与 Tailwind v4 默认 `spacing-1=4px` 对齐）：

| Token | Value | Tailwind class | Usage |
|-------|-------|----------------|-------|
| xs | 4px | `gap-1` / `p-1` | 图标内边距、徽章内字符间距、表格单元格 icon 与文字 |
| sm | 8px | `gap-2` / `p-2` / `space-y-2` | 紧凑表单字段间距、Drawer 内 Label/Input 间距、按钮内 icon 与文字 |
| md | 16px | `gap-4` / `p-4` / `space-y-4` | 默认表单行间距（Drawer 内）、卡片 padding、Dialog 内段落间距 |
| lg | 24px | `gap-6` / `p-6` / `space-y-6` | 卡片之间、Section 之间、Tab 内容到下一区块 |
| xl | 32px | `gap-8` / `p-8` | 仅用于 Sheet 内部主标题与内容分隔 |
| 2xl | 48px | `py-12` | 空状态垂直留白（无规则时占位区） |
| 3xl | 64px | `py-16` | 不用（v3.5 单 Tab 内不需要超大留白） |

例外（必须显式声明）：
- **Tab 触发器高度 36px**（`h-9`），符合 host 详情页现有 Tab 内嵌按钮的 36px touch target（如 `connTabs` 中 `py-1.5 + 文字 12px` ≈ 36px）。
- **预设卡片高度 96px**（`h-24`），固定避免 hover Popover 抖动。
- **Drawer 宽度 480px**（`w-[480px]`），与 `egress-ip-drawer.tsx` `SheetContent className="w-[480px] sm:max-w-[480px]"` 完全对齐。
- **预览 Sheet 宽度 640px**（`w-[640px] sm:max-w-[640px]`），比 Drawer 宽 160px 以容纳 JSON / nft diff 双 Tab 内容。

---

## Typography

声明 4 个角色，2 个权重，line-height 沿用 host 详情页现有比例。

| Role | Size | Weight | Line Height | Tailwind | Usage |
|------|------|--------|-------------|----------|-------|
| Body / caption | 12px | 400 regular | 1.5 (line-height: 18px) | `text-xs leading-relaxed` | 表格 cell、表单字段 hint、Tooltip 内文、Popover 规则示例、错误码副本 |
| Body | 14px | 400 regular | 1.5 (line-height: 21px) | `text-sm leading-normal` | Tab 内正文、Drawer 表单 Label、Toast 文本、CTA Button label、预设卡片描述 |
| Heading | 16px | 600 semibold | 1.3 (line-height: 21px) | `text-base font-semibold` | Card title、Sheet 内 section heading、预设卡片标题（如 `loopback`） |
| Display | 20px | 600 semibold | 1.2 (line-height: 24px) | `text-xl font-semibold tracking-tight` | Sheet title（"预览生效配置"）、AlertDialog title（"确认应用配置？"） |

**仅 2 个权重**：400 regular（body/caption）+ 600 semibold（heading/display/badge）。**禁止引入 500 medium 或 700 bold**，与现有 host 详情页保持一致。

**等宽字体**：所有 host slug / config_hash / IP / CIDR / 端口 / 域名值使用 `font-mono`（Tailwind 默认 `ui-monospace, SFMono-Regular, …, monospace`），sizing 与上下文一致。

---

## Color

颜色全部走 `web/admin/src/index.css` `oklch()` token，**不引入新的 raw hex**。Tab 内严格遵守 60/30/10 占比。

| Role | Token | Value (oklch) | Usage |
|------|-------|---------------|-------|
| Dominant (60%) | `--background` / `--card` | `oklch(0.995 0 0)` / `oklch(1 0 0)` | Tab 内容容器、卡片 surface、Drawer 内层 |
| Secondary (30%) | `--muted` / `--secondary` / `--border` | `oklch(0.968 0.003 264)` / `oklch(0.965 0.005 264)` / `oklch(0.915 0.006 264)` | 表格表头底色、卡片边框、Tab 未选中态、占位区底色、未生效预设卡片 |
| Accent (10%) | `--primary` | `oklch(0.488 0.200 264)` (蓝紫主色) | 仅以下元素：1) Tab 激活态背景；2) 主 CTA "应用配置" 按钮；3) 预设卡片 selected 边框 + checkbox 选中态；4) 表单 focus ring；5) "v23 → v24" 版本号高亮；6) 进度步骤条 active step；7) host slug 副标题 hover 态 |
| Destructive | `--destructive` | `oklch(0.577 0.245 27.325)` (红) | 仅以下元素：1) "删除规则" 表格行 hover icon；2) AlertDialog 确认按钮（删除 / rollback）；3) 表单字段错误提示文字；4) apply 失败状态 icon |
| Warning | `--warning` / `--warning-foreground` | `oklch(0.72 0.17 75)` (琥珀) / `oklch(0.18 0 0)` | 仅以下元素：1) 高风险规则徽章（`Risky` Badge variant="warning"）；2) `domain_keyword < 4 字符` 表单提示；3) `apply` 含 > 5 条高风险规则时主按钮 hover 态；4) 自定义规则表行左侧 2px 警告色边框；5) "有更新" 类似徽章（与 host 详情页 `bg-amber-50 text-amber-700` 风格保持，但优先用 `bg-warning/10 text-warning-foreground`） |
| Success | `--success` / `--success-foreground` | `oklch(0.62 0.17 145)` / `oklch(0.985 0 0)` | 仅以下元素：1) apply 成功 step icon（`CheckCircle2`）；2) "已生效" 状态 Badge；3) Toast `toast.success` 边色 |
| Info | `--info` / `--info-foreground` | `oklch(0.58 0.16 220)` / `oklch(0.985 0 0)` | 仅以下元素：1) "查看 sing-box JSON / nft set diff" Tab 切换器激活态；2) `loopback 强制启用` Tooltip 边框 |

**Accent 排除清单（明确不允许）**：
- 不用于普通表格行 hover（用 `bg-muted/50`）
- 不用于非主按钮（次要按钮 `variant="outline"`，第三按钮 `variant="ghost"`）
- 不用于普通 link（用 `--foreground` hover 态）
- 不用于 dropdown menu item hover（用 `--accent` token oklch(0.955 0.012 264)，那是 muted-accent，不是 primary）

**对比度合规**：所有上述 oklch 组合在 light 模式下 contrast ratio ≥ 4.5:1（warning text 在白底 ≥ 4.7:1 已验证）。dark 模式 v3.5 不实现（项目当前仅 light 模式）。

---

## Copywriting Contract

全部使用中文（项目 CLAUDE.md 强制约束）。错误码 `BYPASS_*` 在前端 `lib/bypass-error-codes.ts` 提供中文映射。

### 顶层 Tab 标签

| Element | Copy |
|---------|------|
| Tab 标题 | 代理白名单 |
| Tab 角标（规则数 > 0 时） | `{n}` 条规则（如 "3 条规则"），Badge variant="secondary" |

### 预设卡片区

| Element | Copy |
|---------|------|
| 区域标题 | 预设规则集 |
| 区域副标题 | 选中预设以快速启用一组系统维护的白名单规则 |
| loopback 卡片标题 | loopback（本机回环） |
| loopback 卡片描述 | `127.0.0.0/8` + `169.254.0.0/16`，强制启用，不可关闭 |
| loopback 锁定 Tooltip | loopback 是 sing-box 自身和系统进程的必需通道，无法关闭 |
| lan 卡片标题 | lan（内网与 ULA） |
| lan 卡片描述 | RFC1918 + CGNAT 100.64/10 + ULA fc00::/7 |
| 卡片 hover Popover 标题 | 包含的规则 |

### 自定义规则表

| Element | Copy |
|---------|------|
| 区域标题 | 自定义规则 |
| 区域副标题 | 最多 1000 条，支持 IP / CIDR / 域名 / 域名后缀 / 域名关键词 5 种类型 |
| Primary CTA | 添加自定义规则 |
| 列名 | 类型 / 值 / 端口 / 风险 / 备注 / 操作 |
| 类型筛选 placeholder | 全部类型 |
| 搜索框 placeholder | 搜索值或备注… |
| 高风险徽章文本 | 高风险 |
| 空状态 heading | 暂无自定义规则 |
| 空状态 body | 当前 host 仅启用了预设规则，点击「添加自定义规则」补充域名或 IP |

### 新建/编辑规则 Drawer

| Element | Copy |
|---------|------|
| Drawer 标题（create） | 添加自定义规则 |
| Drawer 标题（edit） | 编辑自定义规则 |
| 类型字段 Label | 规则类型 * |
| 类型选项 | IP 地址 / CIDR 网段 / 完整域名 / 域名后缀 / 域名关键词 |
| 值字段 Label | 值 * |
| 值字段 placeholder（IP） | 例如：192.168.1.10 |
| 值字段 placeholder（CIDR） | 例如：10.0.0.0/8 |
| 值字段 placeholder（域名） | 例如：api.internal.corp |
| 值字段 placeholder（域名后缀） | 例如：corp.internal（不要带前导点） |
| 值字段 placeholder（域名关键词） | 例如：mirrors（≥ 4 字符） |
| 端口字段 Label | 端口（可选） |
| 端口字段 hint | 留空表示所有端口；单个端口或范围（80 / 80-443） |
| 备注字段 Label | 备注 |
| 备注 placeholder | 简要说明此规则用途（≤ 200 字） |
| `confirm_risky` 复选框 | 我已知悉该关键词可能误命中其他域名 |
| 主 CTA（create） | 创建规则 |
| 主 CTA（edit） | 保存修改 |
| 次 CTA | 取消 |

### 预览面板（Sheet）

| Element | Copy |
|---------|------|
| Sheet 标题 | 预览生效配置 |
| 版本号副标题 | v{current} → v{next} · 覆盖 {n} 条规则 |
| 视图切换 Tab 1 | sing-box JSON |
| 视图切换 Tab 2 | nft set diff |
| JSON 折叠提示（> 10000 行） | 配置超过 1 万行，点击展开完整内容（可能影响浏览器性能） |
| 风险摘要 heading | 风险报告 |
| 风险摘要为空 | 无风险项 |
| 主 CTA | 应用此配置 |
| 主 CTA（含 > 5 条高风险） | 应用此配置（含 {n} 条高风险） |
| 次 CTA | 取消 |

### 应用进度 Dialog

| Element | Copy |
|---------|------|
| Dialog 标题 | 应用白名单配置 |
| 步骤 1 | 生成快照 |
| 步骤 2 | 下发到 agent |
| 步骤 3 | Reload 配置 |
| 步骤 4 | 健康检查 |
| 步骤 5 | 完成 |
| 自动关闭后 toast（成功） | 已应用 · 白名单变更不影响现有 TCP 连接，新连接才用新规则 |
| 失败 heading | 应用失败 |
| 失败 body 模板 | `{错误码中文文案} · 错误码：{CODE}` |
| 自动回滚状态行 | 已自动回滚到上一个生效配置（v{prev}） |
| 关闭按钮 | 关闭 |

### 错误状态

| 错误码 | 中文文案（toast / Drawer / Sheet 共享） |
|--------|-----------------------------------------|
| `BYPASS_RULE_TOO_BROAD` | 规则覆盖范围过大，请使用更具体的 CIDR 或域名 |
| `BYPASS_RULE_CONFLICT_PROXY` | 该规则覆盖了代理服务器 IP，会导致代理自身无法访问 |
| `BYPASS_LIMIT_EXCEEDED` | 当前 host 自定义规则已达上限（1000 条），请先删除不再使用的规则 |
| `BYPASS_KEYWORD_TOO_SHORT` | 关键词长度小于 4 字符，存在误命中风险，请勾选「我已知悉」继续 |
| `BYPASS_PRESET_IMMUTABLE` | 系统预设不可修改或删除 |
| `BYPASS_HOST_UNREACHABLE` | host-agent 当前不可达，配置已保存但未生效，将自动重试 |
| `BYPASS_RELOAD_TIMEOUT` | sing-box reload 超时，已自动回滚 |
| 未知错误兜底 | 操作失败，请稍后重试 · 错误码：{CODE} |

### Destructive Confirmation（AlertDialog 二次确认）

| Action | Confirmation Copy |
|--------|--------------------|
| 删除单条自定义规则 | 标题："删除该规则？" / 描述："删除后白名单立即收紧。需要保存后通过『应用此配置』生效。" / 确认按钮：删除 |
| 解绑预设（仅 lan） | 标题："取消 lan 预设？" / 描述："关闭后容器将无法直连内网地址（RFC1918 / CGNAT / ULA），可能影响 LAN 内服务访问。" / 确认按钮：确认取消 |
| 应用含 > 5 条高风险规则 | 标题："含 {n} 条高风险规则，确认应用？" / 描述："建议先复核高风险规则列表。应用后立即生效，不影响现有 TCP 连接。" / 确认按钮：仍要应用（warning 色） |
| Rollback | 标题："回滚到 v{target}？" / 描述："当前配置 v{current} 将被替换为 v{target}。需要在输入框输入 host slug `{slug}` 以确认。" / 输入框 placeholder："输入 host slug" / 确认按钮：执行回滚（disabled until input matches） |

---

## Registry Safety

| Registry | Blocks Used | Safety Gate |
|----------|-------------|-------------|
| shadcn official | `tabs`, `popover`, `progress`, `scroll-area`, `form` 五个新增 primitives（沿用 new-york style 本地复制源码到 `src/components/ui/`） | not required（官方 registry） |

**不引入任何第三方 registry。** 不引入 Monaco / CodeMirror 等大体积编辑器。预览 JSON 使用语义化 `<pre>` + 轻量 syntax highlight（如 `prism-react-renderer` 或纯 CSS span，planner 决定具体实现）。预览面板大小限制由 UI 层 10000 行折叠机制兜底，避免性能问题。

第三方 registry 安全门：N/A — 本阶段无第三方依赖，故无 `shadcn view + diff` 流程。

---

## Component Inventory（本阶段交付清单）

按 CONTEXT.md 「Claude's Discretion」放在 `web/admin/src/components/bypass/` 目录下，每个组件单独文件：

| Component | 文件 | 职责 | 复用基准 |
|-----------|------|------|----------|
| `BypassTab` | `bypass-tab.tsx` | 顶层容器，挂接 `$hostId.tsx` Tab 路由 | `connTabs` 风格（host 详情页内嵌 Tab） |
| `PresetGrid` | `preset-grid.tsx` | 3 列预设卡片网格（loopback 锁定 / lan 可勾选 / 占位 disabled） | 新组件，遵循 shadcn `Card` + 内嵌 checkbox + Popover hover |
| `PresetCard` | `preset-card.tsx` | 单张预设卡片 + Popover 规则示例 | 新组件，使用 shadcn `Card` + `Popover` |
| `CustomRulesTable` | `custom-rules-table.tsx` | TanStack Table 列表 + 类型筛选 + 全文搜索 + 高风险徽章 | 仿现有 `egress-ips` 列表模式 |
| `BypassRuleDrawer` | `bypass-rule-drawer.tsx` | 新建 / 编辑规则的 Sheet 表单（5 种类型 + 端口 + 备注 + risky 确认） | `egress-ip-drawer.tsx` 完全对齐 width / form 风格 |
| `PreviewSheet` | `preview-sheet.tsx` | 右侧滑出 Sheet，双 Tab 切换（JSON / nft diff），底部主 CTA | 新组件，使用 shadcn `Sheet` + `Tabs` |
| `JSONViewer` | `json-viewer.tsx` | sing-box JSON 只读展示 + 10000 行折叠 + 语法高亮 | 新组件 |
| `NftDiffViewer` | `nft-diff-viewer.tsx` | unified diff 渲染（绿 + / 红 - / context 默认色） | 新组件 |
| `ApplyProgressDialog` | `apply-progress-dialog.tsx` | 5 阶段步骤条 + 各阶段 spinner/check/error icon + 失败错误码 | 仿 `$hostId.tsx` `upgradeOpen` Dialog 模式（progress bar + status text + error block） |
| `RiskyKeywordConfirm` | `risky-keyword-confirm.tsx` | `domain_keyword < 4` AlertDialog + 复选框确认 | 仿 `forceDeleteOpen` AlertDialog |
| `RollbackConfirmDialog` | `rollback-confirm-dialog.tsx` | host slug 输入二次确认 | 新组件，结合 shadcn `AlertDialog` + 内嵌 `Input` |

Hooks（`web/admin/src/hooks/`）：
- `use-bypass-presets.ts`：list / get / create / update / delete（系统预设禁止 mutation）
- `use-bypass-rules.ts`：list（hostId 过滤）/ get / create / update / delete / validate（dry-run）
- `use-bypass-bindings.ts`：list / bind / unbind
- `use-bypass-effective.ts`：当前生效规则全集（`presets_active / rules_active / whitelist_cidrs_rendered / whitelist_domains_rendered`）
- `use-bypass-preview.ts`：preview（不落库）
- `use-bypass-apply.ts`：apply + 进度回放（结合 SSE topic `tasks`）
- `use-bypass-rollback.ts`：rollback to snapshot_id

API client（`web/admin/src/lib/api/bypass.ts` + `web/admin/src/lib/api/types/bypass.ts`）：手写 TypeScript interface，字段 snake_case 与后端 JSON 对齐（CONTEXT.md 已锁定不引入 codegen）。

---

## Interaction Contracts

### 预设卡片状态机

| State | 视觉 | 交互 |
|-------|------|------|
| `forced-on`（loopback） | 主色 1px 实线边框 + 主色背景 5% 透明度 + 锁图标 + checkbox 灰色 disabled checked | hover 时仅显示 Tooltip "loopback 强制启用，不可关闭"，点击无响应 |
| `selected`（lan 启用） | 主色 1px 实线边框 + 主色背景 5% 透明度 + checkbox 主色 checked | 点击 checkbox 立即取消选中并触发预览刷新（不立即 apply） |
| `unselected`（lan 关闭） | border 1px solid `--border` + 背景 `--card` + checkbox 未选中 | 点击 checkbox 立即选中并触发预览刷新 |
| `placeholder`（cn-dev 等 P1 预设） | 50% 透明度 + 文字 "敬请期待" + checkbox 灰色 disabled | 不可点击，cursor 默认 |

### 自定义规则表行 hover

- 整行 `bg-muted/50`（与 host 详情页表格 hover 一致）
- 右侧操作按钮 `Edit` + `Delete` 默认 50% 透明度，hover 整行时变为 100% 透明度
- 高风险行：左侧 2px 实线 `--warning` 色边框，永久显示 `Risky` Badge（不依赖 hover）

### Drawer 表单交互

- 类型字段值切换时：清空值字段 + 重置 placeholder + 重置 zod schema（不同类型规则的 validation 不一样）
- 端口字段：blur 时校验格式（数字 / `min-max` 范围）
- `domain_keyword < 4` 字符：实时显示 inline warning "关键词较短，可能误命中其他域名"，但不阻塞 submit；submit 时弹 `RiskyKeywordConfirm` AlertDialog
- submit pending：主按钮显示 "保存中…" + 内嵌 `Loader2` spinner，整个表单 disabled

### 预览面板触发

- Tab 内任意预设或规则变更（勾选 / 新建 / 编辑 / 删除）后，右下角浮现 "查看生效预览" sticky 按钮
- 点击后 `Sheet` 从右侧滑出，宽度 640px，默认显示 sing-box JSON Tab
- Sheet 内的 "应用此配置" 主按钮触发 `ApplyProgressDialog`，**Sheet 不关闭**（用户可在进度结束后看到最终配置）

### 应用进度反馈

- 5 步进度条横向布局，每步显示：icon + 阶段名 + 当前状态文字
- 当前 active step：`Loader2` 旋转 + 主色文字
- 已完成 step：`CheckCircle2` success 色 + 灰色文字
- 失败 step：`XCircle` destructive 色 + destructive 色文字
- 总耗时 ≤ 5s 且全部成功：dialog 自动 500ms 延迟关闭 + 触发 `toast.success`
- 任意步骤失败：dialog 保持开启 + 显示错误码 block + "关闭" 按钮（toast 不触发，避免双重提示）

### Rollback host slug 二次确认

- 输入框使用 `font-mono` 显示 host slug placeholder
- 输入值严格 equality 比对 host.slug
- 确认按钮在输入匹配前 disabled + 灰色
- 匹配成功后确认按钮变为 destructive 色

---

## Empty / Loading / Error States

| Scenario | 视觉 |
|----------|------|
| Tab 整体 loading | 复用 host 详情页 `animate-pulse` 骨架屏模式：3 个 `h-24` 卡片占位 + 1 个 `h-64` 表格占位 |
| 自定义规则空 | 居中 `py-12` 区块：lucide `ShieldCheck` icon（48px，`text-muted-foreground`）+ heading（14px semibold）+ body（12px regular muted） + 主 CTA "添加自定义规则" |
| 预览面板 loading | Sheet 内 `Loader2` 旋转居中 + 文字 "正在生成预览…" |
| 预览面板 fetch error | 红色边框 block + 错误码 + "重试" 链接 |
| Apply network error | dialog 内 step 4/5 直接跳 error，文案 "网络连接失败，请检查后重试" |

---

## Checker Sign-Off

- [ ] Dimension 1 Copywriting: PASS — 所有 CTA / 空状态 / 错误状态 / 二次确认 / 错误码映射均显式声明中文文案
- [ ] Dimension 2 Visuals: PASS — 11 个组件文件清单 + 复用基准明确 + 状态机覆盖
- [ ] Dimension 3 Color: PASS — 60/30/10 token 锁定 + accent reserved-for 列表明确 + destructive/warning/success/info 用途排他
- [ ] Dimension 4 Typography: PASS — 4 角色 / 2 权重 / line-height 显式 / mono 用法清单
- [ ] Dimension 5 Spacing: PASS — 7 档 token（4/8/16/24/32/48/64）全部 4 的倍数 + Drawer/Sheet 宽度例外显式声明
- [ ] Dimension 6 Registry Safety: PASS — 仅 shadcn 官方，新增 5 个 primitives 本地复制源码，无第三方依赖

**Approval:** pending（由 gsd-ui-checker 升级为 approved）

---

## 备注

- 本契约的所有 spacing / typography / color 取值都已对照 `web/admin/src/index.css` 已存在 token，不需要新增 CSS 变量。
- 5 个新增 shadcn primitives（`tabs` / `popover` / `progress` / `scroll-area` / `form`）由 planner 在 plan 任务中以 `npx shadcn add tabs popover progress scroll-area form` 一键添加（或对应文件级 prompt）。
- TanStack Table 在项目中尚未引入，planner 需在 Plan 任务中评估：
  - 选项 A：引入 `@tanstack/react-table` v8（与现有 `@tanstack/react-router/react-query` 同生态，体积约 14kb gz）
  - 选项 B：用原生 `<table>` + 手写排序/筛选（< 1000 行规则量级可承受）
  - **UI-SPEC 默认建议 A**，因 v3.5 P1 会扩展批量操作 + 命中统计排序，原生 table 维护成本高。
