---
phase: 46
plan: 03
subsystem: web/admin (bypass UI)
tags: [frontend, vitest, shadcn, react-hook-form]
requires: [phase-45-schema, 46-02-api-handlers (并行)]
provides:
  - bypass-tab-component
  - bypass-rule-drawer
  - preset-grid
  - custom-rules-table
  - bypass-i18n-error-codes
  - vitest-infrastructure
affects:
  - web/admin/src/routes/_dashboard/hosts/$hostId.tsx
  - web/admin/package.json (script + devDeps)
tech-stack:
  added:
    - vitest@4
    - @vitest/ui@4
    - @testing-library/react@16
    - @testing-library/jest-dom@6
    - @testing-library/user-event@14
    - jsdom@29
  patterns:
    - react-hook-form + zod (sheet 表单)
    - tanstack-query (key ['bypass', hostId, ...])
    - shadcn new-york + radix-ui v1.4
key-files:
  created:
    - web/admin/vitest.config.ts
    - web/admin/src/test/setup.ts
    - web/admin/src/test/smoke.test.ts
    - web/admin/src/lib/api/types/bypass.ts
    - web/admin/src/lib/api/bypass.ts
    - web/admin/src/hooks/use-bypass-presets.ts
    - web/admin/src/hooks/use-bypass-rules.ts
    - web/admin/src/hooks/use-bypass-bindings.ts
    - web/admin/src/lib/i18n/bypass-error-codes.ts
    - web/admin/src/lib/i18n/__tests__/bypass-error-codes.test.ts
    - web/admin/src/components/bypass/risky-keyword-confirm.tsx
    - web/admin/src/components/bypass/bypass-rule-drawer.tsx
    - web/admin/src/components/bypass/preset-card.tsx
    - web/admin/src/components/bypass/preset-grid.tsx
    - web/admin/src/components/bypass/custom-rules-table.tsx
    - web/admin/src/components/bypass/bypass-tab.tsx
    - web/admin/src/components/bypass/__tests__/bypass-rule-drawer.test.tsx
    - web/admin/src/components/bypass/__tests__/preset-grid.test.tsx
    - web/admin/src/components/bypass/__tests__/custom-rules-table.test.tsx
    - web/admin/src/components/bypass/__tests__/bypass-tab.test.tsx
  modified:
    - web/admin/package.json
    - web/admin/pnpm-lock.yaml
    - web/admin/src/routes/_dashboard/hosts/$hostId.tsx
decisions:
  - 用 Tooltip 兜底「预设卡片 hover Popover」需求，避免新增 shadcn popover primitive
  - Tab 集成走「在 host 详情页插入独立 #bypass section」最小侵入路径，
    而非把整个详情页改成 Tabs 容器；保留 URL 锚点 #bypass 直跳能力
  - zod schema 在 domain_keyword < 4 字符时不硬拦截，由 RiskyKeywordConfirm 走软确认 UX
  - 不引入 TanStack React Table v8，用原生 table 满足 v3.5 量级（< 1000 条）
  - vitest setup.ts 提前补 hasPointerCapture / scrollIntoView polyfill，
    让 Radix UI 组件在 jsdom 环境可测
metrics:
  duration: ~25 分钟
  completed: 2026-05-12
---

# Phase 46 Plan 03: 后台 UI BypassTab + 预设网格 + 自定义规则表 + Drawer + i18n Summary

完成了 host 详情页「代理白名单」Tab 的所有 UI 组件骨架（BYPASS-UI-01/02/03），并 bootstrap 了 web/admin 项目的 vitest 测试基础设施，22 个测试用例全绿。

## 交付清单

### Task 0 — vitest bootstrap
- 安装 vitest 4 + jsdom 29 + testing-library 三件套
- `vitest.config.ts` 与 `vite.config.ts` alias 对齐（`@` → `./src`）
- `src/test/setup.ts` 注入 matchMedia / ResizeObserver / hasPointerCapture /
  scrollIntoView polyfill，保证 Radix UI 组件在 jsdom 可测
- `package.json` 新增 `test:unit` script（约定通过 `pnpm test:unit -- ...` 跑）

### Task 1 — API types + client + hooks
- `src/lib/api/types/bypass.ts`：`BypassPreset` / `BypassRule` / `BypassBinding`
  等 TypeScript interface，字段全部 snake_case 与后端 JSON 对齐
- `src/lib/api/bypass.ts`：presets / rules / bindings 三组 CRUD client function
- 3 个 hook 文件：`use-bypass-presets` / `use-bypass-rules` / `use-bypass-bindings`，
  query key 统一 `['bypass', ...]`，staleTime 30 秒，mutation 成功后 invalidate

### Task 2 — i18n + RiskyKeywordConfirm
- `src/lib/i18n/bypass-error-codes.ts`：8 个 `BYPASS_*` 错误码 → 中文文案
- `parseBypassError(err)` 从 `ApiError.message` 解析 JSON body 提取 code
- `RiskyKeywordConfirm` AlertDialog：「我已知悉」复选框 + 确认按钮 disabled 守护

### Task 3 — BypassRuleDrawer
- 仿 `egress-ip-drawer.tsx` 风格，Sheet 480px 宽
- zod schema 按 rule_type 切换校验：
  - `ip`：IPv4 正则
  - `cidr`：`x.x.x.x/yy` 格式
  - `domain`：完整域名（≥ 2 段，TLD 字母）
  - `domain_suffix`：不带前导点 + ≥ 4 字符
  - `domain_keyword`：不硬拦截短关键词（走 RiskyKeywordConfirm 软确认）
- edit 模式锁定规则类型选择器（type 不可变更）
- 切换类型时清空 value 字段
- 端口字段单端口或 `80-443` 范围正则校验
- 错误处理通过 `parseBypassError` 抓取 BYPASS_* 错误码 → toast

### Task 4 — PresetGrid + PresetCard + CustomRulesTable
- `preset-card.tsx`：单卡片三态机（forced-on / selected / unselected），
  loopback 强制锁 + Lock 图标 + secondary Badge「强制」，
  hover 用 Tooltip 显示样例规则（兜底 Popover 需求）
- `preset-grid.tsx`：3 列网格，不足 3 个填补「敬请期待」占位，
  bindingByPreset 映射 + create/delete binding mutation
- `custom-rules-table.tsx`：原生 `<table>` + shadcn Table primitive，
  - 列：类型 / 值 / 端口 / 风险 / 备注 / 操作
  - 类型筛选 Select + 全文搜索 Input（搜 value/note）
  - is_risky 行：`border-l-2 border-l-warning` + 「高风险」徽章
  - 删除按钮触发 AlertDialog 二次确认
  - 空状态用 `EmptyState` 组件

### Task 5 — BypassTab + host 详情页集成
- `bypass-tab.tsx`：顶层容器，含标题 + 规则数 Badge + 预设区 + 自定义规则区
- `$hostId.tsx` 在 ClaudeStatusCard 后插入 `<div id="bypass">` 锚定 section
- 支持 `#bypass` 锚点 URL 直跳

## 验证结果

| 验证项 | 命令 | 结果 |
| --- | --- | --- |
| Vitest | `pnpm test:unit -- --reporter=basic` | 22/22 passed |
| TypeScript | `pnpm typecheck` | 9 个基线错误，新增 0 个 |
| Build | `pnpm build` | 成功（817 kB） |

## 偏差与说明

### Rule 3 — vitest 阻塞性问题（自动修复）

测试 BypassRuleDrawer 时 Radix Select 在 jsdom 抛 `target.hasPointerCapture is not a function`。
追加 `setup.ts` 的 polyfill（`HTMLElement.prototype.hasPointerCapture/setPointerCapture/releasePointerCapture/scrollIntoView`）解决；
同时简化了一个测试用例（避免依赖真实的 Select 弹层交互，改成在 edit 模式锁定类型）。

### 已知 baseline TypeScript 错误（仓库基线）

仓库当前已有 9 个 TS 错误（egress-ip-drawer 的 zod resolver 推断、use-hosts.ts 的 toast 未导入等），
非本 plan 引入。本 plan 新增代码无任何新 TS 错误。

### Deferred（留给后续 plan）

- 预览面板 `PreviewSheet` / `JSONViewer` / `NftDiffViewer`：留给 46-04 或 47-x
- `ApplyProgressDialog` 5 阶段步骤条：留给 47-x（apply / agent reload 链路）
- `RollbackConfirmDialog`：留给 47-x（rollback API 就绪后）
- shadcn `tabs` / `popover` / `progress` / `scroll-area` / `form` 5 个 primitive：
  本 plan 不需要（Tooltip 兜底 Popover；表单错误用手写 `<p className="text-destructive">`），
  未来的 PreviewSheet 实现可能需要补 tabs / scroll-area

## Self-Check: PASSED

- 已创建文件存在：通过 git log 和 build 产物验证
- 每个 Task 单独 commit：db54519 / b509196 / 9feb79b / 3949838 / 1be7c2d / ee5d3aa
- 测试全绿，typecheck 无回归，build 通过
