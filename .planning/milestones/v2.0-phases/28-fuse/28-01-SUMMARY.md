---
phase: 28-fuse
plan: 01
subsystem: infra
tags: [apparmor, fuse, sshfs, docker, security]

requires:
  - phase: 24-fuse
    provides: "SYS_ADMIN + /dev/fuse 容器权限和 sshfs/fuse3 镜像预装"
provides:
  - "AppArmor unconfined 安全选项解除 FUSE 挂载阻断"
  - "FUSE 兼容性自动化验证脚本（sshfs 真实挂载 + 网络策略共存 + E2E 流程）"
affects: [28-02, deploy]

tech-stack:
  added: []
  patterns: ["apparmor=unconfined 容器安全降级", "mountpoint -q 挂载就绪判据"]

key-files:
  created: [scripts/verify-fuse-compat.sh]
  modified: [internal/runtime/tasks/worker.go]

key-decisions:
  - "选择 apparmor=unconfined 而非自定义 AppArmor profile（运维复杂度 vs 安全边界已由 nftables+namespace 覆盖）"

patterns-established:
  - "verify-*.sh 结构化输出模式: [PASS]/[FAIL]/[WARN] 前缀 + 汇总计数 + 非零退出码"

requirements-completed: [SRV-04]

duration: 2min
completed: 2026-04-15
---

# Phase 28 Plan 01: FUSE 兼容性修复与验证 Summary

**worker.go 添加 apparmor=unconfined 解除 FUSE 阻断，238 行验证脚本覆盖 sshfs 真实挂载 + 网络策略共存 + E2E 流程**

## Performance

- **Duration:** 2 min
- **Started:** 2026-04-15T06:37:08Z
- **Completed:** 2026-04-15T06:39:14Z
- **Tasks:** 2
- **Files modified:** 2

## Accomplishments
- worker.go createHost() 添加 `--security-opt apparmor=unconfined`，解除 AppArmor docker-default profile 的 `deny mount` 对 FUSE 的阻断
- 创建 238 行验证脚本，覆盖宿主机安全模块检测、容器内真实 sshfs FUSE 挂载测试、网络策略共存验证、端到端流程验证四个阶段
- 验证脚本使用 mountpoint -q 判据与 mount.go waitForMount 对齐

## Task Commits

Each task was committed atomically:

1. **Task 1: worker.go 添加 AppArmor unconfined 安全选项** - `fa90fbe` (feat)
2. **Task 2: 创建 FUSE 兼容性验证脚本** - `0642c30` (feat)

## Files Created/Modified
- `internal/runtime/tasks/worker.go` - createHost() args 切片中添加 `--security-opt apparmor=unconfined`
- `scripts/verify-fuse-compat.sh` - 五阶段 FUSE 兼容性验证：安全模块检测 → sshfs 挂载 → 网络策略 → E2E → 汇总

## Decisions Made
- 选择 `apparmor=unconfined` 而非自定义 AppArmor profile：安全边界不依赖 AppArmor（nftables 默认拒绝 + 隧道出网 + namespace 隔离已覆盖），自定义 profile 增加运维复杂度。RESEARCH.md Pattern 2 保留了自定义 profile 模板供未来审计需要时启用。

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Plan 02（集成测试与自定义 AppArmor profile 模板）可基于本计划的修改继续执行
- 验证脚本需在目标 Linux 宿主机上运行以确认生产环境兼容性

## Self-Check: PASSED

---
*Phase: 28-fuse*
*Completed: 2026-04-15*
