# Phase 42: Phase 39 验证补齐 - Research

**Researched:** 2026-05-08
**Domain:** Verification gap closure -- producing formal VERIFICATION.md for Phase 39
**Confidence:** HIGH

## Summary

Phase 39（本地 Dev Containers）的 3 个 plan（39-01、39-02、39-03）全部标记 complete，代码已实现并提交（commits c41735a、bdb6904、295d558），30 个单元测试全部通过，但缺少正式的 VERIFICATION.md 文件。本次 Phase 42 的目标是为 Phase 39 的 5 个需求（LOCAL-01、LOCAL-02、LOCAL-03、LOCAL-04、UX-02）补充正式验证文档，消除 v3.4 里程碑审计中 "unverified phase" 的 blocker。

这是一个纯验证/文档补齐阶段，不需要新增功能代码。所有需要验证的代码和测试已经存在。Phase 40 的手动测试（40-VERIFICATION-REPORT.md）已确认 `cloud-claude local up` 能真实启动容器并提供 SSH 连接，提供了额外的端到端验证证据。

**Primary recommendation:** 按照 Phase 38/41 的 VERIFICATION.md 格式，以 Observable Truths + Artifacts + Key Links + Requirements Coverage 的结构，系统地为 Phase 39 的每个需求提供代码级证据，生成 39-VERIFICATION.md。

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| LOCAL-01 | `cloud-claude local` 子命令支持一键启动本地容器 | `internal/local/local.go` LocalManager.Up 实现完整；`cmd/cloud-claude/local.go` cobra 注册；30 个 unit tests 通过；Phase 40 手动验证真实 Docker 启动 |
| LOCAL-02 | 本地容器支持 Dev Containers 配置 | `.devcontainer/devcontainer.json` 包含 MODE=local、forwardPorts、SYS_ADMIN；JSON 合法 |
| LOCAL-03 | 本地容器支持 sing-box 全隧道（可选配置） | `internal/local/egress.go` DetectEgressMode + ValidateEgressConfig；entrypoint.sh tun/proxy 分支；8+4 个 egress tests 通过 |
| LOCAL-04 | entrypoint 支持 `MODE=local` 分支 | `deploy/docker/managed-user/entrypoint.sh` 行 217-313：KasmVNC+v3 stages 被 `if [ "$MODE" != "local" ]` 包裹；行 315-384 sing-box 启动段 |
| UX-02 | `cloud-claude local` 支持 `down` / `status` 子命令 | `cmd/cloud-claude/local.go` runLocalDown + runLocalStatus；`internal/local/local.go` Down + Status 方法；TestDown + TestStatus 测试 |
</phase_requirements>

## Standard Stack

本阶段是验证/文档补齐阶段，不引入新库或新依赖。

### Existing Code Under Verification

| File | Lines | Purpose | Verification Status |
|------|-------|---------|---------------------|
| `internal/local/local.go` | 275 | LocalManager, Up/Down/Status, buildCreateArgs | 30 unit tests PASS |
| `internal/local/container.go` | 60 | DockerRunner interface, containerExists, inspectSSHPort, inspectContainerStatus | Unit tests PASS |
| `internal/local/egress.go` | 77 | DetectEgressMode, ValidateEgressConfig, egressMountArg | 12 unit tests PASS |
| `internal/local/password.go` | 20 | GeneratePassword (crypto/rand) | 7 test cases PASS |
| `internal/local/local_test.go` | 352 | Full unit test suite | 30/30 PASS, 0.22s |
| `cmd/cloud-claude/local.go` | 132 | cobra local subcommand group (up/down/status) | go build OK |
| `.devcontainer/devcontainer.json` | 25 | VS Code Dev Container config (MODE=local) | JSON valid |
| `deploy/docker/managed-user/entrypoint.sh` | 387 | MODE=local branch + sing-box startup | bash -n syntax OK |

## Architecture Patterns

### Verification Document Pattern (from Phase 38/41)

The VERIFICATION.md follows a specific structure established by the project:

```yaml
---
phase: <phase_id>
status: passed
verified: <ISO timestamp>
---
```

Followed by these sections:
1. **Observable Truths** — Table of machine-verifiable facts with code evidence (file:line references)
2. **Required Artifacts** — Each file created/modified, with what it provides
3. **Key Link Verification** — Cross-file wiring checks (from -> to -> via -> status)
4. **Requirements Coverage** — Each requirement ID mapped to specific evidence
5. **Test Coverage** — Unit test counts and results
6. **Human Verification Required** — Manual tests that need a running Docker environment
7. **Anti-Patterns Found** — Check for TODO/FIXME/PLACEHOLDER/stub patterns

### Key Format Rules

- Each Observable Truth must reference specific code locations (`file.go:line`)
- Evidence column must be concrete, not vague ("PASS" is not evidence; "local.go:81-146 Up method with 8-step flow" is)
- Key Links use the format: `from_file::function` -> `to_file::function` via `mechanism`
- Requirements table maps each req ID to SATISFIED with code evidence

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Docker container verification | Custom test harness | Existing `go test ./internal/local/...` + manual Docker test | Docker is not available in CI; unit tests mock it |
| JSON validation for devcontainer.json | Custom validator | `python3 -m json.tool` or `cat \| jq .` | Standard JSON validation is sufficient |
| Bash syntax checking | Custom parser | `bash -n entrypoint.sh` | Native bash check catches all syntax errors |

## Common Pitfalls

### Pitfall 1: Confusing Unit Test Evidence with Integration Evidence
**What goes wrong:** Marking a truth as VERIFIED based solely on unit tests when the truth involves real Docker interaction
**Why it happens:** Unit tests use mock DockerRunner, so they verify code logic but not Docker behavior
**How to avoid:** For truths that involve actual container startup (LOCAL-01 truth 1-3), distinguish between:
  - Code-level evidence: function exists, logic is correct (unit test)
  - Integration evidence: real Docker container starts (from Phase 40 manual test)
**Warning signs:** Claiming "container starts" when only a mock test passes

### Pitfall 2: Missing entrypoint.sh Evidence Granularity
**What goes wrong:** Vaguely stating "entrypoint supports MODE=local" without line references
**Why it happens:** The entrypoint is a long bash script with multiple conditional branches
**How to avoid:** Reference specific line ranges for each behavior:
  - MODE detection: line 217-218
  - KasmVNC skip: line 220 `if [ "$MODE" != "local" ]`
  - v3 stages skip: line 306-312 inside the same if block
  - sing-box startup: line 315-384
  - sshd exec: line 387

### Pitfall 3: Egress Config Mode Detection Not Fully Exercised
**What goes wrong:** Only testing socks/http detection without testing the tun fallback path
**Why it happens:** tun mode requires /etc/sing-box/config.json which doesn't exist in unit tests
**How to avoid:** Document that tun fallback to proxy (entrypoint.sh line 325-328) is a runtime behavior verified by code review, not unit test

### Pitfall 4: Devcontainer.json Changes Not Verified Against VS Code
**What goes wrong:** Marking devcontainer.json as VERIFIED only by JSON validity check
**Why it happens:** VS Code Dev Containers has specific parsing requirements beyond JSON validity
**How to avoid:** Note that VS Code compatibility requires manual verification (Human Verification Required section); code-level verification covers JSON structure and required fields only

## Code Examples

### Observable Truth Pattern (from Phase 38)

```markdown
| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | LocalManager.Up 启动容器并返回 SSHInfo | VERIFIED | `internal/local/local.go:81-146` Up 方法：生成密码 -> 容器名 -> 构建参数 -> docker create/start -> inspectSSHPort -> 返回 SSHInfo |
```

### Key Link Pattern (from Phase 38)

```markdown
| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| cmd/cloud-claude/local.go::runLocalUp | internal/local/local.go::Up | LocalManager.Up() 调用 | WIRED | `cmd/cloud-claude/local.go:73` mgr.Up(cmd.Context()) |
```

## State of the Art

| Aspect | Current State | Notes |
|--------|---------------|-------|
| Phase 39 code | Complete (3 plans, 3 commits) | All 30 unit tests pass |
| Phase 39 verification | MISSING | No VERIFICATION.md exists |
| Phase 40 integration evidence | Partial | 40-VERIFICATION-REPORT.md confirms local up + SSH works |
| v3.4 audit status | gaps_found | 5 Phase 39 requirements marked "partial" due to missing VERIFICATION |

## Verification Architecture

> nyquist_validation is false in .planning/config.json -- skipping automated test mapping section.

### Manual Verification Commands

These commands can be run to produce evidence for the VERIFICATION.md:

```bash
# Unit tests (all should PASS)
go test ./internal/local/... -v -count=1

# Build check
go build ./cmd/cloud-claude/

# CLI help verification
go run ./cmd/cloud-claude/ local --help
go run ./cmd/cloud-claude/ local up --help
go run ./cmd/cloud-claude/ local down --help
go run ./cmd/cloud-claude/ local status --help

# entrypoint syntax check
bash -n deploy/docker/managed-user/entrypoint.sh

# devcontainer.json validation
cat .devcontainer/devcontainer.json | python3 -m json.tool

# Anti-pattern scan
grep -rn "TODO\|FIXME\|HACK\|PLACEHOLDER" internal/local/ cmd/cloud-claude/local.go
```

## Open Questions

1. **是否需要在 VERIFICATION.md 中包含 Phase 40 的集成测试证据？**
   - What we know: Phase 40 手动验证报告确认 `cloud-claude local up` 真实启动了 Docker 容器并可通过 SSH 连接
   - What's unclear: 是否可以引用 Phase 40 的证据来验证 LOCAL-01 的 "功能正确" 要求
   - Recommendation: 可以引用，但标注来源为 "Phase 40 手动验证"，因为 Phase 42 自身不执行 Docker 测试

2. **entrypoint.sh MODE=local 分支中 v3 stages 跳过的验证粒度**
   - What we know: v3 stages 函数调用在 `if [ "$MODE" != "local" ]` 块内（line 306-312），代码级证据充分
   - What's unclear: 是否需要验证 MODE=remote 行为不变（回归测试）
   - Recommendation: 在 Behavioral Spot-Checks 中列出 MODE=remote 回归验证命令，但标注为 Human Verification Required

3. **egress tun 模式的完整链路验证**
   - What we know: entrypoint.sh 检测到 tun 模式缺少 config.json 时降级为 proxy（line 325-328），代码逻辑正确
   - What's unclear: 完整 tun 模式（有 config.json + NET_ADMIN + /dev/net/tun）是否在真实环境中测试过
   - Recommendation: 在 Human Verification Required 中列出 tun 模式完整链路测试，标注需要有 egress 配置的真实环境

## Sources

### Primary (HIGH confidence)
- `.planning/phases/38-ssh-proxy-port-forwarding/38-VERIFICATION.md` — VERIFICATION.md 格式模板
- `.planning/phases/41-doctor/41-VERIFICATION.md` — 较新的 VERIFICATION.md 格式参考
- `.planning/v3.4-MILESTONE-AUDIT.md` — 审计发现的具体 gap 详情

### Secondary (MEDIUM confidence)
- Phase 39 plan files (39-01-PLAN.md, 39-02-PLAN.md, 39-03-PLAN.md) — 原始需求和 success criteria
- Phase 39 summaries (39-01-SUMMARY.md, 39-02-SUMMARY.md, 39-03-SUMMARY.md) — 实现确认

### Tertiary (LOW confidence)
- Phase 40-VERIFICATION-REPORT.md — 非标准格式，但提供了集成测试证据

## Metadata

**Confidence breakdown:**
- Standard Stack: HIGH — no new stack; verification only
- Architecture: HIGH — VERIFICATION.md format well-established from Phase 38/41
- Pitfalls: MEDIUM — gap closure verification has specific traps around evidence granularity

**Research date:** 2026-05-08
**Valid until:** 2026-06-08 (stable -- verification document format does not change)
