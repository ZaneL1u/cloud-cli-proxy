---
phase: 37-e2e-uat
plan: "03"
subsystem: cli
tags: [go, doctor, errcodes, explain, inotify, cold-promotion]

# Dependency graph
requires:
  - phase: 37-01
    provides: "MOUNT_PROMOTER_FAILED 错误码 + ColdPromoter 核心引擎"
  - phase: 37-02
    provides: "LastSessionSnapshot.PromotionCount/PromotionFailedCount 字段"
provides:
  - "MOUNT_PROMOTER_FAILED 错误码全链路注册（codes.go + mount.go + explanations.go）"
  - "doctor mount 域 4 项晋升指标 check（promoter_alive / promotion_queue_depth / promotion_total / promotion_failed_total）"
  - "explain 子进程测试 TestExplain_MountPromoterFailed_Exit0_MinLen"
affects: [37-05, 37-e2e-uat]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "doctor promotion check 本地只读文件模式：PID file + last-session.json，不在线访问 goroutine 内部状态"
    - "promotion 数据缺失时 check 优雅降级为 skip（非 warn/fail）"
    - "explain 子进程测试复用 buildOnceExplainBin + runExplainBin 模式"

key-files:
  modified:
    - "internal/cloudclaude/errcodes/codes.go - MOUNT_PROMOTER_FAILED 常量（37-01 已 ship）"
    - "internal/cloudclaude/errcodes/mount.go - MOUNT_PROMOTER_FAILED MustRegister（37-01 已 ship）"
    - "internal/cloudclaude/errcodes/explanations.go - registerExplanation >=200 中文字符（37-01 已 ship）"
    - "internal/cloudclaude/doctor/mount.go - 新增 4 项 promotion check 函数"
    - "internal/cloudclaude/doctor/doctor.go - RunDoctor mount 域追加 4 项 promotion check 调用"
    - "cmd/cloud-claude/explain_test.go - 新增 TestExplain_MountPromoterFailed_Exit0_MinLen"
  created: []

key-decisions:
  - "checkPromotionQueueDepth 使用 snap.PromotionCount 作为活跃度简易指标（Plan 02 未加 QueueDepth 字段时的退避）"
  - "PromotionCount/PromotionFailedCount 字段已在 Phase 37-01 写入 LastSessionSnapshot，本 plan 复用即可无需新加"
  - "doctor promotion check 均为本地只读（PID file + last-session.json），不走 runWithTimeout 包装"

patterns-established:
  - "Promotion check 模式：读本地文件 → 值缺失 skip / 有值 pass / 失败 warn + 关联错误码"
  - "MOUNT_PROMOTER_FAILED 作为 promotion check 的统一错误码锚点"

requirements-completed: [REQ-MOUNT-V31-14]

# Metrics
duration: 15min
completed: 2026-04-24
---

# Phase 37 Plan 03: 错误码基础设施与 Doctor 可观测 总结

**MOUNT_PROMOTER_FAILED 错误码全链路（37-01 ship）+ 4 项 promotion doctor check + explain 子进程测试**

## 执行概况

- **Duration:** ~15 min
- **Started:** 2026-04-24T04:57:00.000Z
- **Completed:** 2026-04-24T05:12:00.000Z
- **Tasks:** 3 (Task 1 已在 37-01 完成)
- **Files modified:** 3 (本 plan 新增)

## 成果

- MOUNT_PROMOTER_FAILED 错误码全链路就位：codes.go const + mount.go MustRegister (Warn) + explanations.go registerExplanation >=200 中文字符（五段模板：触发场景/根本原因/复现方式/修复路径/关联文档）
- doctor mount 域从 9 项扩大到 13 项（+4 项 promotion check）：
  - `promoter_alive`：通过 PID file + `kill -0` 检测 cold-promoter 进程存活
  - `promotion_queue_depth`：通过 last-session.json 判断晋升引擎活跃度
  - `promotion_total`：累计晋升文件计数
  - `promotion_failed_total`：晋升失败次数监控，失败时 warn 带 MOUNT_PROMOTER_FAILED
- explain 子进程测试通过：`cloud-claude explain MOUNT_PROMOTER_FAILED` exit 0 + stdout >=200 字符

## Task Commits

1. **Task 1: 注册 MOUNT_PROMOTER_FAILED 错误码 + 长说明** - `dc0c86a` (feat(37-01) — 已在 37-01 落实)
2. **Task 2: Doctor mount 域新增 4 项 promotion check** - `b9eedbf` (feat(37-03))
3. **Task 3: 追加 explain 子进程测试** - `ef4bb12` (test(37-03))

## Files Created/Modified

| 文件 | 变更 | 说明 |
|------|------|------|
| `internal/cloudclaude/errcodes/codes.go` | 37-01 已 ship | MOUNT_PROMOTER_FAILED 常量（行 138） |
| `internal/cloudclaude/errcodes/mount.go` | 37-01 已 ship | MOUNT_PROMOTER_FAILED MustRegister（行 131-136） |
| `internal/cloudclaude/errcodes/explanations.go` | 37-01 已 ship | registerExplanation >=200 中文字符（行 144-148） |
| `internal/cloudclaude/doctor/mount.go` | 新增 4 函数 | checkPromoterAlive / checkPromotionQueueDepth / checkPromotionTotal / checkPromotionFailedTotal |
| `internal/cloudclaude/doctor/doctor.go` | 新增 4 调用 | mount 块 checkDefaultIgnoreLoaded 后追加 4 项 promotion check |
| `cmd/cloud-claude/explain_test.go` | 新增 1 测试 | TestExplain_MountPromoterFailed_Exit0_MinLen |

## 验证结果

| 检查项 | 状态 |
|--------|------|
| `go test ./internal/cloudclaude/errcodes/... -v` | 9/9 PASS |
| `go test ./cmd/cloud-claude/... -run TestExplain_Mount -v` | 3/3 PASS |
| `go build ./...` | PASS |
| `go vet ./internal/cloudclaude/doctor/...` | PASS |
| `bash scripts/ci-doctor-grep.sh ./cloud-claude` | M14 gate PASS |
| `grep -c "MOUNT_PROMOTER_FAILED" codes.go` | 1 |
| `grep -c "MOUNT_PROMOTER_FAILED" mount.go` | 1 |
| `grep -c checkPromoterAlive\|... doctor.go` | 4 |
| `grep -c promoter_alive\|... mount.go` | >=1 (4 类) |
| `grep -c TestExplain_MountPromoterFailed explain_test.go` | 1 |

## Decisions Made

- Task 1 因已在 Phase 37-01 中落地，本 plan 仅做验收不复做
- `checkPromotionQueueDepth` 复用 `snap.PromotionCount` 作为活跃度简易指标（Plan 02 未加 `PromotionQueueDepth` 字段时的设计退避，与 PLAN.md 推荐方案一致）
- doctor promotion check 均为本地只读（PID file + last-session.json），不走 `runWithTimeout` 包装，与 `checkRequireGitRepo` 等本地 check 同模式
- `MOUNT_PROMOTER_FAILED` 作为所有 promotion 警告的统一错误码锚点（promoter_alive 失败 + promotion_failed_total 有失败记录均走此码）

## Deviations from Plan

### 验收指标微小偏差

**1. [Plan 计数] explanations.go grep -c 输出 2 而非 1**
- **Found during:** Task 1 验收
- **Issue:** `grep -c "MOUNT_PROMOTER_FAILED" explanations.go` 输出 2（registerExplanation 调用 + 文本中自引用各命中 1 次），PLAN.md 验收标准写为 1
- **Fix:** 无需修改 — `TestAllCodesHaveExplanations` PASS（>=200 rune 断言通过），实际语义正确
- **Impact:** 无，仅验收计数预期偏差

---

**Total deviations:** 1 项（验收计数微小偏差，无代码变更）
**Impact on plan:** 无实质影响。核心验收（Go 测试 + CI gate）全部 PASS。

## Issues Encountered

- `Edit` 工具在处理 tab 缩进文件时匹配困难，改用 Python 脚本完成 doctor.go 行级插入
- Plan 02 未执行导致 `LastSessionSnapshot.PromotionCount/PromotionFailedCount` 字段存在性不确定，实际检查后发现字段已在 Phase 37-01 中写入，无需额外添加

## Known Stubs

无。4 项 promotion check 均正确读取本地文件并做 skip/pass/warn 三态判断，不存在硬编码空值或 TODO 占位符。`checkPromotionQueueDepth` 的 `PromotionCount` 替代 `QueueDepth` 是 Plan 明确记录的设计退避（PLAN.md 推荐方案），非 stub。

## Next Phase Readiness

- Doctor promotion 可观测就位，Plan 37-05（e2e UAT 脚本）可直接用 `cloud-claude doctor --json | jq '.checks[] | select(.name | startswith("promotion"))'` 做端到端验证
- explain 测试覆盖 MOUNT_PROMOTER_FAILED，子进程渲染链路完整

---
*Phase: 37-e2e-uat*
*Completed: 2026-04-24*
