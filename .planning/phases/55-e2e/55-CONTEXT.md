# Phase 55: e2e 重构 + 同容器安全断言 - Context

**Gathered:** 2026-05-27
**Status:** Ready for planning
**Mode:** Auto-generated (infrastructure phase — smart discuss skip)

<domain>
## Phase Boundary

`harness.Scenario` builder 单容器化 — 删除 `gatewaySpec` + `SingBoxGateway` 访问器，合并到 `userSpec`/`User`；v3.6 全部 LEAK/KILL/MVS 用例迁移到新拓扑（user 容器即 sing-box 容器）；新增 SEC-01..03 三条同容器安全断言（用户无法 kill sing-box、读不到 config、无特权 cap）。

v3.6 cross-container 协调辅助代码（gw 启动同步、network connect 等待等）全部删除，净删 ≥ 150 行。
</domain>

<decisions>
## Implementation Decisions

### Claude's Discretion
All implementation choices are at Claude's discretion — pure infrastructure/test-migration phase. Use ROADMAP phase goal, success criteria, existing test patterns, and Phase 53/54 architecture changes to guide decisions.

Key architectural context from prior phases:
- Phase 53: managed-user 镜像内置 sing-box，entrypoint `start_singbox_or_die` + `remove_singbox_config` + `apply_nft_or_die`
- Phase 54: 控制面不再创建 gw 容器，单容器路径，`container_proxy_provider` 简化

</decisions>

<code_context>
## Existing Code Insights

### Current Scenario Builder (`tests/e2e/harness/scenario.go`)
- `gatewaySpec` + `hostSpec` + `userSpec` 三实体模型
- `SingBoxGateway()` 访问器 — v4.0 需合并到 `User()`
- `WithSingBoxGateway(...)` → 合并到 `WithUser(...)`
- `PrepareGateway` 调用 — v4.0 无 gateway，需删除

### Test Files Requiring Migration
- `tests/e2e/leak/` — 8 条 LEAK 用例（leak_01..08）
- `tests/e2e/killswitch_singbox_crash_test.go` — KILL 核心用例
- `tests/e2e/killswitch_stress/` — 4 条 KILL 压力用例
- `tests/e2e/` 根 — MVS/GoldenPath/bootstrap/egress/dns/expiry
- `tests/e2e/helpers.go` + `tests/e2e/helpers_linux.go` — fixture 辅助函数

### New Security Assertions (SEC-01..03)
- SEC-01: 用户 kill sing-box 必须失败（非 root，无权限）
- SEC-02: 用户读 config 必须失败（文件已 rm 或权限拒绝）
- SEC-03: 用户 cap 集合必须为空（`getpcaps $$` 输出空）
</code_context>

<specifics>
## Specific Ideas

No specific requirements — infrastructure phase. Refer to ROADMAP Phase 55 success criteria:

1. `harness.Scenario` builder API `.WithSingBoxGateway(...)` 合并进 `.WithUser(...)`
2. v3.6 LEAK-01..08 用例迁移后继续绿
3. v3.6 KILL-01..04 用例迁移：`docker kill <gw>` 改为 `docker exec <user> kill -9 $(pidof sing-box)`；新增断言 "PID 1 死 → 容器死 → 出网立即断"
4. v3.6 MVS-01..10 + GoldenPath 用例迁移，出口 IP / DNS / default-deny / 错误码契约保持不变
5. SEC-01 / SEC-02 / SEC-03 三条新用例独立可跑且全绿
6. 删除 v3.6 cross-container 协调辅助代码，净删 ≥ 150 行
</specifics>

<deferred>
## Deferred Ideas

None — infrastructure phase. All migration work is in-scope for Phase 55.
</deferred>
