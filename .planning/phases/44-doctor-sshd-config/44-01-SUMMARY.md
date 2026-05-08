---
phase: 44-doctor-sshd-config
plan: 01
subsystem: doctor
tags: [sshd, doctor, errcodes, forwarding, security]

# Dependency graph
requires:
  - phase: 34-doctor
    provides: "doctor 五维度框架 + errcodes 注册体系 + check.go newWarn/newPass helpers"
provides:
  - "parseSSHDForwarding + checkSSHDForwarding 两个 sshd 转发检查函数"
  - "SSH_SSHD_FORWARDING_DISABLED / SSH_SSHD_STREAM_FORWARDING_DISABLED / SSH_SSHD_GATEWAY_PORTS_OPEN 三个错误码"
  - "doctor ssh 维度已包含 sshd_forwarding 检查项"
affects: [doctor, errcodes, sshd-config]

# Tech tracking
tech-stack:
  added: []
  patterns: ["sshd -T 输出解析 + 基线比对 → newWarn/newPass 模式复用"]

key-files:
  created: []
  modified:
    - internal/cloudclaude/errcodes/codes.go
    - internal/cloudclaude/errcodes/ssh.go
    - internal/cloudclaude/errcodes/explanations.go
    - internal/cloudclaude/doctor/ssh.go
    - internal/cloudclaude/doctor/ssh_test.go
    - internal/cloudclaude/doctor/doctor.go

key-decisions:
  - "runner 错误时默认报 SSH_SSHD_FORWARDING_DISABLED（第一个发现问题），保持与 checkSSHDKeepaliveDrift 一致的错误处理模式"
  - "parseSSHDForwarding 使用 ToLower 做大小写防御性处理，sshd -T 输出全小写但做防护"

patterns-established:
  - "sshd -T 转发检查模式：runner.RunScript → parseSSHDForwarding → 三步顺序检查 → newWarn/newPass"

requirements-completed: [SSH-03]

# Metrics
duration: 2min 34s
completed: 2026-05-08
---

# Phase 44 Plan 01: doctor sshd 转发检查 Summary

**sshd_config AllowTcpForwarding / AllowStreamLocalForwarding / GatewayPorts 三项转发指令自动检查，含 3 个错误码 + 13 个单元测试**

## Performance

- **Duration:** 2 min 34s
- **Started:** 2026-05-08T08:42:51Z
- **Completed:** 2026-05-08T08:45:25Z
- **Tasks:** 2
- **Files modified:** 6

## Accomplishments

- 实现 parseSSHDForwarding 和 checkSSHDForwarding 两个函数，复用 checkSSHDKeepaliveDrift 模式
- 新增 SSH_SSHD_FORWARDING_DISABLED / SSH_SSHD_STREAM_FORWARDING_DISABLED / SSH_SSHD_GATEWAY_PORTS_OPEN 三个错误码，每条附 ≥200 字中文 explain
- 13 个单元测试覆盖正常配置 / 指令缺失 / 值为 no / 大小写 / 多问题优先级 / runner 错误等场景
- checkSSHDForwarding 已注册到 doctor ssh 维度，紧跟 checkSSHDKeepaliveDrift 之后

## Task Commits

Each task was committed atomically:

1. **Task 1: 新增 3 个错误码 + parseSSHDForwarding / checkSSHDForwarding 函数 + 单元测试** - `bc00c7f` (feat)
2. **Task 2: 注册 checkSSHDForwarding 到 doctor ssh 维度** - `484e623` (feat)

## Files Created/Modified

- `internal/cloudclaude/errcodes/codes.go` - 新增 3 个 SSH_SSHD_*_FORWARDING/GATEWAY 错误码常量
- `internal/cloudclaude/errcodes/ssh.go` - 注册 3 个 MustRegister Entry
- `internal/cloudclaude/errcodes/explanations.go` - 3 条 ≥200 字中文长说明
- `internal/cloudclaude/doctor/ssh.go` - parseSSHDForwarding + checkSSHDForwarding 两个函数
- `internal/cloudclaude/doctor/ssh_test.go` - 13 个新单元测试
- `internal/cloudclaude/doctor/doctor.go` - ssh 维度注册 sshd_forwarding 检查项

## Decisions Made

- runner 错误时默认报 `SSH_SSHD_FORWARDING_DISABLED`（第一个发现问题的错误码），与 `checkSSHDKeepaliveDrift` 的错误处理策略一致，避免在错误路径上引入新的分支
- parseSSHDForwarding 使用 `strings.ToLower` 做大小写防御性处理，sshd -T 输出全小写但做防护

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

- TDD GREEN 阶段 CaseInsensitive 测试未通过：`strings.TrimPrefix(line, "allowtcpforwarding ")` 在 line 为 `AllowTcpForwarding yes` 时不匹配。修正为使用 `strings.CutPrefix` 从原始行和 lowercase 行分别尝试前缀匹配，解决大小写混合输入的解析问题。此为 Rule 1（bug fix）自动修复。

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- doctor ssh 维度已包含 sshd 转发检查，可直接用于 Phase 44 后续 Plan 或集成验证
- 三个错误码已注册，`cloud-claude explain` 自动支持

---
*Phase: 44-doctor-sshd-config*
*Completed: 2026-05-08*

## Self-Check: PASSED

All files and commits verified: codes.go, ssh.go (errcodes), explanations.go, doctor/ssh.go, ssh_test.go, doctor.go, bc00c7f, 484e623
