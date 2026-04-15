# Phase 25: cloud-claude CLI 骨架与连接 - Research

**Researched:** 2026-04-15
**Domain:** Go CLI（cobra）、YAML 配置、Entry HTTP 契约、`golang.org/x/crypto/ssh` + PTY
**Confidence:** HIGH（契约以仓库内实现为准）

<user_constraints>
## User Constraints (from 25-CONTEXT.md)

- D-01～D-12：cobra、config 路径、Entry auth、轮询、单 PTY session、中文错误与退出码、无硬编码网关默认等 — 见 `.planning/phases/25-cloud-claude-cli/25-CONTEXT.md`。
</user_constraints>

## Findings

### 控制面契约（已实现）

- `POST /v1/entry/{shortId}/auth`，body `{"password":"..."}`。
- 成功且主机 `running`：`ssh_user`、`ssh_pass`、`ssh_host`（来自请求 Host）、`ssh_port`（2222）、`status: ready`。
- 主机非 running：`status: not_ready` + `message`。
- 凭证错误 / 用户不存在：401 等 — 见 `internal/controlplane/http/entry.go`。

### 栈选择

- 与 `.planning/research/STACK.md`、`SUMMARY.md` 一致：cobra、yaml.v3、x/crypto/ssh、x/term。
- 无需引入 WebSocket；数据面为 SSH。

### 风险与注意事项

- `ssh_host` 派生自 HTTP `Host`：客户端访问控制面的 URL 与期望 SSH 目标主机名需一致（NAT/端口转发场景需在文档说明）。
- `golang.org/x/crypto` 版本应与仓库统一（STATE 已提示 Phase 25 前对齐）。

## Recommendations

1. 先实现配置读写与 `init`，再实现 HTTP auth + 重试，最后 SSH+PTY。
2. 集成测试可针对 mock HTTP + `testcontainers`/`integration` 分层，本 PLAN 以可执行手工验证为准。

---

*Research for Phase 25 — 详细业界对比见里程碑级 `.planning/research/ARCHITECTURE.md` / `STACK.md`。*
