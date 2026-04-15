# Phase 27: 双 session 目录映射 - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-15
**Phase:** 27-session
**Areas discussed:** SFTP 服务端实现, Session 生命周期与时序, 挂载就绪检测, 异常退出清理
**Mode:** --auto (all decisions auto-selected)

---

## SFTP 服务端实现

| Option | Description | Selected |
|--------|-------------|----------|
| `github.com/pkg/sftp` | Go 生态最成熟的 SFTP 库，支持 NewServer() 接受 io.ReadWriter | ✓ |
| 自行实现 SFTP 协议 | 完全自定义但工作量大且易出错 | |

**User's choice:** [auto] `github.com/pkg/sftp`（推荐默认）
**Notes:** 无需自行实现 SFTP 协议，pkg/sftp 已广泛使用且稳定。

| Option | Description | Selected |
|--------|-------------|----------|
| 用户当前工作目录 (cwd) | 与 MAP-01 需求一致，映射用户正在工作的目录 | ✓ |
| 用户 home 目录 | 范围过大，不符合"当前目录"的需求定义 | |

**User's choice:** [auto] 用户当前工作目录（推荐默认）
**Notes:** MAP-01 明确要求"用户当前目录自动映射到容器 /workspace"。

---

## Session 生命周期与时序

| Option | Description | Selected |
|--------|-------------|----------|
| 先 sshfs 挂载，确认就绪后再启动 claude | 保证 /workspace 可用时 claude 才启动 | ✓ |
| 并行启动两个 session | 可能出现 claude 启动时 /workspace 尚未就绪的竞态 | |

**User's choice:** [auto] 先挂载后 claude（推荐默认）
**Notes:** claude 需要 /workspace 作为工作目录，必须先确保挂载成功。

| Option | Description | Selected |
|--------|-------------|----------|
| 拆分为 connect + mountWorkspace + runClaude | 关注点分离，每个阶段职责清晰 | ✓ |
| 在现有函数内添加第二个 session | 函数复杂度过高，难以测试和维护 | |

**User's choice:** [auto] 拆分为三阶段（推荐默认）
**Notes:** 与现有代码模式一致，便于单独测试每个阶段。

| Option | Description | Selected |
|--------|-------------|----------|
| `cd /workspace && claude ...` | 与 shellescape 模式一致 | ✓ |
| SSH env 变量设定工作目录 | 需要 SSH server 侧 AcceptEnv 配置 | |

**User's choice:** [auto] `cd /workspace && claude ...`（推荐默认）
**Notes:** 延续 Phase 26 的远程命令构建模式。

---

## 挂载就绪检测

| Option | Description | Selected |
|--------|-------------|----------|
| exec `mountpoint -q /workspace` 轮询 | 可靠的 POSIX 标准检测方式 | ✓ |
| 固定延时等待 | 不可靠，网络慢时可能不够 | |
| 监听 sshfs stderr | 输出格式不稳定，解析困难 | |

**User's choice:** [auto] `mountpoint -q /workspace` 轮询验证（推荐默认）
**Notes:** 200ms 间隔、10s 上限，标准 POSIX 工具可靠性最高。

---

## 异常退出清理

| Option | Description | Selected |
|--------|-------------|----------|
| 关闭 sshfs session channel（EOF 触发 sshfs 退出） | sshfs slave stdin 关闭时自动退出并卸载 | ✓ |
| 仅依赖 fusermount -u | 需要额外 session，且 sshfs 进程可能残留 | |

**User's choice:** [auto] 关闭 channel 触发自动清理 + fusermount 防御性补充（推荐默认）
**Notes:** 主路径依赖 sshfs slave 的 EOF 行为，fusermount 作为保底。

---

## Claude's Discretion

- sshfs 额外挂载参数（性能调优）
- pkg/sftp server 可选配置
- 挂载就绪轮询具体参数
- 挂载阶段用户提示文字

## Deferred Ideas

- Mutagen 备选 — v2.x ENH-01
- 大目录 ignore — v2.x ENH-04
- FUSE + AppArmor/seccomp — Phase 28
