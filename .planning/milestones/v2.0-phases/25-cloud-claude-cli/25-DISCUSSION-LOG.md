# Phase 25: cloud-claude CLI 骨架与连接 - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-15
**Phase:** 25-cloud-claude CLI 骨架与连接
**Mode:** `--auto`（全部灰色区域自动选用推荐项）
**Areas discussed:** CLI 结构与配置、控制面集成、SSH 会话语义、错误与退出码、自有网关

---

## CLI 结构与配置模型

| Option | Description | Selected |
|--------|-------------|----------|
| cobra + yaml 配置 | 与 STACK 研究一致，子命令清晰 | ✓ |
| 标准库 flag only | 依赖少但子命令弱 | |
| 单文件 JSON 配置 | 与 REQUIREMENTS 中 yaml 不一致 | |

**User's choice:** [auto] cobra + `~/.cloud-claude/config.yaml`
**Notes:** `[auto] Context exists — updating with auto-selected decisions.` — 新建上下文，无既有 CONTEXT 冲突。

---

## 控制面集成方式

| Option | Description | Selected |
|--------|-------------|----------|
| 复用 `POST /v1/entry/{shortId}/auth` | 与 bootstrap/entry.go 单一事实来源 | ✓ |
| 新设 cloud-claude 专用 REST | 重复契约、服务端改动 | |
| 仅 SSH、不调 HTTP | 无法在 SSH 前获知就绪与口令校验 | |

**User's choice:** [auto] 复用 Entry auth
**Notes:** `[auto] Selected all gray areas: CLI 结构、Entry 集成、SSH 语义、退出码、私有网关。`

---

## 主机未就绪时的行为

| Option | Description | Selected |
|--------|-------------|----------|
| 轮询同一 auth 端点直至超时 | 实现简单，与「请稍后再试」心智一致 | ✓ |
| 一次失败即退出 | 体验差 | |
| 调用独立任务 API | Phase 25 不扩大控制面依赖 | |

**User's choice:** [auto] 轮询 + 超时

---

## SSH 会话语义（Phase 25 范围内）

| Option | Description | Selected |
|--------|-------------|----------|
| 单 session + PTY + 远程 `claude` | 满足「进入 Claude Code 会话」 | ✓ |
| 多 session（预开 sftp） | 属 Phase 27 目录映射 | |
| 无 PTY | 不满足交互式 Claude Code | |

**User's choice:** [auto] 单 PTY session，远程启动 `claude`（argv 固定为空或最小）

---

## 错误与退出码

| Option | Description | Selected |
|--------|-------------|----------|
| 中文 stderr + 分类退出码 1–5 | 满足 CLI-04，便于脚本 | ✓ |
| 英文 only | 与 REQUIREMENTS 冲突 | |
| 恒为 1 | 不利于自动化 | |

**User's choice:** [auto] 中文 + 分类码

---

## 自有网关（CLI-05）

| Option | Description | Selected |
|--------|-------------|----------|
| 仅配置文件/环境变量，无内置默认 URL | 满足私有部署 | ✓ |
| 内置 SaaS 默认网关 | 与「可配置自有地址」并列时需更多分支 | |

**User's choice:** [auto] 无硬编码默认，必填来自 config

---

## Claude's Discretion

- 健康检查路径、轮询间隔/超时、远程 `claude` 的精确启动命令 — 见 CONTEXT「Claude's Discretion」。

## Deferred Ideas

- 见 CONTEXT `<deferred>`（参数透传、映射、TTY 全语义）。
