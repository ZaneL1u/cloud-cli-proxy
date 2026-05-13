---
phase: 46
plan: "04"
subsystem: web/admin · bypass UI 预览-应用-回滚链路
tags: [react, vitest, bypass, preview, apply, rollback, ui]
requires:
  - 46-02-PLAN.md（控制面 preview/apply/rollback 接口）
  - 46-03-PLAN.md（BypassTab + 自定义规则表 + 预设区）
provides:
  - PreviewSheet（sing-box JSON + nft diff 双 Tab 预览）
  - ApplyProgressDialog（5 阶段步骤条 + task.progress_percent 映射）
  - RollbackConfirmDialog（host slug 严格输入二次确认）
  - useBypassSnapshots hooks（preview/apply/rollback/effective/audit-log）
  - JSONViewer / NftDiffViewer 只读展示原语
  - BypassTab 集成「查看生效预览」入口
affects:
  - web/admin/src/components/bypass/bypass-tab.tsx
  - web/admin/src/components/bypass/apply-progress-dialog.tsx（useSSE 在 open=false 时短路）
tech-stack:
  added:
    - "@radix-ui/react-progress（progress primitive）"
    - "@radix-ui/react-scroll-area（scroll-area primitive）"
    - "@radix-ui/react-tabs（tabs primitive）"
  patterns:
    - useRef 持有 setTimeout 句柄避免 useEffect cleanup 误清
    - vitest 直接 mock useTaskPolling 绕过 react-query polling 时序耦合
    - useSSE 通过空 url short-circuit 控制订阅生命周期
key-files:
  created:
    - web/admin/src/components/ui/progress.tsx
    - web/admin/src/components/ui/scroll-area.tsx
    - web/admin/src/components/ui/tabs.tsx
    - web/admin/src/hooks/use-bypass-snapshots.ts
    - web/admin/src/components/bypass/json-viewer.tsx
    - web/admin/src/components/bypass/nft-diff-viewer.tsx
    - web/admin/src/components/bypass/preview-sheet.tsx
    - web/admin/src/components/bypass/apply-progress-dialog.tsx
    - web/admin/src/components/bypass/rollback-confirm-dialog.tsx
    - web/admin/src/components/bypass/__tests__/preview-sheet.test.tsx
    - web/admin/src/components/bypass/__tests__/apply-progress-dialog.test.tsx
  modified:
    - web/admin/src/lib/api/bypass.ts（追加 preview/apply/rollback/effective/audit-log named exports）
    - web/admin/src/lib/api/types/bypass.ts（追加 BypassPreviewResponse / BypassApplyResponse / BypassRollbackResponse / BypassEffectiveResponse / BypassAuditLogResponse）
    - web/admin/src/components/bypass/bypass-tab.tsx（集成 PreviewSheet + ApplyProgressDialog + sticky 入口按钮）
decisions:
  - "JSON 渲染统一用 <pre> + JSON.stringify(2)，禁止引入 Monaco/CodeMirror（UI-SPEC 显式约束）"
  - "nft diff 用简单 unified diff 行扫描 + 颜色 token（+ emerald-400 / - red-400 / # slate-500）"
  - "5 阶段名固定中文：生成快照 / 下发到 agent / Reload 配置 / 健康检查 / 完成"
  - "task.progress_percent 4 档映射阶段（<25 dispatch / <50 reload / <75 health / ≥75 done），Phase 47 接管后真实推进"
  - "RollbackConfirmDialog 本 plan 不接入 BypassTab —— 等 SnapshotHistory 落地后由其调用"
  - "BypassTab 内嵌 ApplyProgressDialog 时通过 useSSE 空 url 短路，避免常驻 EventSource"
metrics:
  duration_min: 95
  completed_date: 2026-05-12
  tasks_completed: 5
  files_changed: 13
  commits: 5
  tests_added: 12
  tests_passing: 34
---

# Phase 46 Plan 04: 代理白名单预览-应用-回滚 UI 三件套 Summary

把 BYPASS-UI-04（生效预览 Sheet + 应用进度 Dialog）与 BYPASS-UI-05（Rollback 二次确认 Dialog）一次性落地，并在 BypassTab 顶层串起「查看生效预览 → 预览面板 → 应用进度跟踪」完整用户旅程；RollbackConfirmDialog 作为可复用独立组件先就位，等待 SnapshotHistory（Phase 47/48 plan）接入。

## 用户旅程图（Apply 主路径）

```
┌────────────┐   点击「查看生效预览」    ┌──────────────┐
│ BypassTab  │ ──────────────────────►  │ PreviewSheet │  自动调 POST /hosts/:id/bypass/preview
└────────────┘                           └──────┬───────┘
                                                │  渲染 sing-box JSON / nft diff
                                                │  顶部摘要：v{current} → v{next} · {summary}
                                                │  风险摘要：>5 主按钮变 warning
                                                ▼
                            点击「应用此配置」 / 「应用此配置（含 N 条高风险）」
                                                │  onApply(preview)
                                                ▼
┌────────────────────────┐                ┌────────────────────────┐
│ BypassTab.handleApply… │ ──────────────►│ ApplyProgressDialog    │  自动 POST /hosts/:id/bypass/apply
│  · 暂存 risky_count    │                │  · 5 阶段步骤条        │  → 拿 task_id
│  · 关 PreviewSheet     │                │  · useTaskPolling 推进 │  → succeeded 后 500ms 自动关闭
│  · 开 ApplyDialog      │                │  · 错误码中文文案      │  → toast.success
└────────────────────────┘                └────────────────────────┘
```

## 5 阶段状态机映射表

ApplyProgressDialog 的 `stageStatuses` 由 `(errorCode, taskId, isDone, isFailed, isRunning, task.progress_percent)` 派生，5 个固定阶段 key/label：

| 阶段索引 | key      | label        | active 触发条件                                                          |
| -------- | -------- | ------------ | ------------------------------------------------------------------------ |
| 0        | snapshot | 生成快照     | 无 taskId 且 apply mutation 进行中 / errorCode → failed                  |
| 1        | dispatch | 下发到 agent | task.progress_percent ∈ [0, 25) / task failed → 此阶段 failed            |
| 2        | reload   | Reload 配置  | task.progress_percent ∈ [25, 50)                                         |
| 3        | health   | 健康检查     | task.progress_percent ∈ [50, 75)                                         |
| 4        | done     | 完成         | task.progress_percent ≥ 75 / task succeeded → 全部 done + 500ms 自动关闭 |

**状态机决策表（按优先级从上到下）：**

| 条件                  | snapshot | dispatch | reload  | health  | done    | 行为                          |
| --------------------- | -------- | -------- | ------- | ------- | ------- | ----------------------------- |
| errorCode 存在        | failed   | pending  | pending | pending | pending | apply mutation 401/5xx        |
| !taskId（mutation 中）| active   | pending  | pending | pending | pending | apply 请求未返回 task_id      |
| isDone                | done     | done     | done    | done    | done    | task.status="succeeded"       |
| isFailed              | done     | failed   | pending | pending | pending | task.status="failed"/"canceled" |
| isRunning, pct<25     | done     | active   | pending | pending | pending | dispatch 中                   |
| isRunning, pct<50     | done     | done     | active  | pending | pending | reload 中                     |
| isRunning, pct<75     | done     | done     | done    | active  | pending | 健康检查中                    |
| isRunning, pct≥75     | done     | done     | done    | done    | active  | 收尾中                        |

## Phase 47 接管点说明

当前 Phase 46 的 dispatch 流程在控制面是占位实现（任务被立即标 succeeded，前端 5 阶段会**瞬间全部 done**）。Phase 47 接管后：

1. **后端**：reload-host-bypass task 真实推进 `progress_percent`（snapshot 后 25% → dispatch 完成 50% → reload 完成 75% → health check 完成 100%）。
2. **前端无需改动**：`stageStatuses` 自动依据真实 `progress_percent` 推进，用户能感知到每个阶段时长。
3. **错误码扩展**：Phase 47 可能新增 `BYPASS_DISPATCH_TIMEOUT`、`BYPASS_HEALTH_CHECK_FAILED` 等 code，已在 `bypass-error-codes.ts` 字典统一收口，新增 code 只需追加映射，UI 自动获得中文提示。
4. **SSE 唤醒**：当前 `useSSE` 仅在 ApplyProgressDialog open 时订阅 `tasks` topic，Phase 47 后端推送的 `task.progress` 事件会通过 react-query invalidate 自然触发 polling，无需 UI 改动。

## RollbackConfirmDialog 接口契约

```tsx
interface RollbackConfirmDialogProps {
  hostId: string;
  /** host.slug 字段值，用户必须严格输入此字符串才能确认回滚（T-46-13 mitigate） */
  hostSlug: string;
  /** 目标快照 ID（要回滚到的历史 snapshot） */
  targetSnapshotId: string;
  /** 目标快照版本号（仅用于 UI 文案，对应 v{N}） */
  targetVersion: number;
  /** 当前生效版本号（对应 v{current}） */
  currentVersion: number;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** rollback 成功后回调，外层可用于触发 ApplyProgressDialog 跟踪 task */
  onSuccess?: (taskId: string) => void;
}
```

**调用示例（待 SnapshotHistory 落地后）：**

```tsx
const [rollbackTarget, setRollbackTarget] =
  useState<{ snapshotId: string; version: number } | null>(null);

<RollbackConfirmDialog
  hostId={hostId}
  hostSlug={host.slug}
  targetSnapshotId={rollbackTarget!.snapshotId}
  targetVersion={rollbackTarget!.version}
  currentVersion={effective.version}
  open={rollbackTarget !== null}
  onOpenChange={(o) => !o && setRollbackTarget(null)}
  onSuccess={(taskId) => {
    setApplyTaskId(taskId); // 复用 ApplyProgressDialog 跟踪进度
    setRollbackTarget(null);
  }}
/>
```

**安全设计（T-46-13 mitigate）：**

- 主按钮在 `input.trim() === hostSlug` 之前 `disabled`，且保持默认色。
- 一旦严格匹配，按钮变 `bg-destructive`，提示用户处于"高危确认"心智。
- 关闭 Dialog 自动清空输入框，防止跨打开会话的残留。
- 输入框 `font-mono`，便于用户从主机详情页复制 slug 字符串后精确对齐。

## Decisions Made

1. **JSON 预览不引入 Monaco/CodeMirror**：`46-UI-SPEC.md` 明确禁止。改用 `<pre>` + `JSON.stringify(value, null, 2)`；超过 10000 字符时折叠显示 + "展开完整内容" 按钮。
2. **nft diff 用极简 unified diff 渲染**：行首字符 `+ - # @` 决定颜色 token，足够审计场景使用，避免引入 react-diff-viewer 等 60kb+ 依赖。
3. **5 阶段映射用 25/50/75 整百分比硬阈值**：Phase 46 后端占位返回 0% 或 100%，硬阈值能保证占位场景 5 阶段瞬间 done 不闪 active；Phase 47 真实推进时也能按 4 档自然过渡。
4. **useRef + 单独 cleanup useEffect 持有 setTimeout**：useEffect 写在 `[isDone, autoCloseScheduled, onOpenChange]` 依赖里，状态切换会触发 cleanup 误清 timer，改成 useRef 持有 + 独立 unmount-only cleanup。
5. **vitest 直接 mock useTaskPolling**：避免与 react-query polling 时序耦合导致 flake，测试单纯验证「task 数据驱动 UI 状态机」契约。
6. **RollbackConfirmDialog 不在 BypassTab 接入**：Phase 46 不做历史快照面板，组件先就位等 SnapshotHistory 调用，不破坏 BypassTab 信息密度。
7. **useSSE 在 dialog open=false 时短路**：BypassTab 内嵌 ApplyProgressDialog 后，组件一直 mount；通过传空 url 避免常驻 EventSource 订阅 + 让 jsdom 测试环境无需 mock window.EventSource。

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] ApplyProgressDialog 自动关闭 timer 被误清**

- **Found during:** Task 4 单测验证
- **Issue:** `setTimeout` 写在 useEffect 内部并在 cleanup `clearTimeout(timer)`，但 `setAutoCloseScheduled(true)` 立即触发依赖重跑 → cleanup 在 timer fire 之前就把它清掉，500ms 自动关闭永远不发生。
- **Fix:** 拆成两个 useEffect — 一个负责 schedule timer 到 `useRef`，另一个仅 unmount-only cleanup。
- **Files modified:** `web/admin/src/components/bypass/apply-progress-dialog.tsx`
- **Commit:** 4278ac1

**2. [Rule 1 - Bug] BypassTab 内嵌 ApplyProgressDialog 触发 EventSource ReferenceError**

- **Found during:** Task 5 集成后跑 `bypass-tab.test.tsx`
- **Issue:** BypassTab mount 即 mount ApplyProgressDialog → useSSE 调用 `new EventSource(...)` 在 jsdom 环境抛 ReferenceError。
- **Fix:** ApplyProgressDialog 内部 `useSSE(open ? url : "", ...)`，空 url 时 `useSSE` 已 short-circuit。
- **Files modified:** `web/admin/src/components/bypass/apply-progress-dialog.tsx`
- **Commit:** 3a3bb28

### 测试断言修正

**3. [Rule 1 - Bug] pct=60 阶段映射断言写错**

- **Found during:** Task 4 单测首次运行
- **Issue:** 测试断言 `pct=60` 时 `reload=active`，但 60 ∈ [50, 75) 映射到 `health` 阶段。
- **Fix:** 改测试断言为 `health=active` + `reload=done`。
- **Files modified:** `web/admin/src/components/bypass/__tests__/apply-progress-dialog.test.tsx`
- **Commit:** 4278ac1

### Auth Gates

无。本 plan 全程在前端层运作，无需后端 auth 切换。

## Known Stubs

无。`RollbackConfirmDialog` 虽未接入 BypassTab 但有完整 props 契约和真实 `useRollbackBypass` mutation，不是 stub；待 SnapshotHistory 组件 import 即可立即工作。

## Commits

| #   | Commit  | 任务                                    | 文件改动 |
| --- | ------- | --------------------------------------- | -------- |
| 1   | 8868748 | API client + hooks + UI primitives      | 5        |
| 2   | d34ad60 | JSONViewer + NftDiffViewer              | 2        |
| 3   | a4de8b2 | PreviewSheet + 6 单测                   | 2        |
| 4   | 4278ac1 | ApplyProgressDialog + 6 单测            | 2        |
| 5   | 3a3bb28 | RollbackConfirmDialog + BypassTab 集成  | 3        |

## Self-Check: PASSED

- 创建文件全部存在（13 个）。
- 全部 5 个 commit 在 `git log` 中可见（8868748 / d34ad60 / a4de8b2 / 4278ac1 / 3a3bb28）。
- `pnpm test:unit -- src/components/bypass/` 34/34 PASS。
- `pnpm typecheck` 仅基线 9 个错误，**0 新增**。
- `pnpm build` 成功（仅 chunk size 历史警告，不阻塞）。
