# Phase 24: 受管镜像 FUSE 硬化与容器参数 - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-15
**Phase:** 24-受管镜像 FUSE 硬化与容器参数
**Areas discussed:** 镜像修改策略, FUSE 权限参数范围, SSH Proxy 验证深度
**Mode:** --auto (all decisions auto-selected)

---

## 镜像修改策略

| Option | Description | Selected |
|--------|-------------|----------|
| 直接修改现有 Dockerfile | 在 deploy/docker/managed-user/Dockerfile 中添加 sshfs/fuse3 | ✓ |
| 创建新的变体 Dockerfile | 为 cloud-claude 场景创建独立的镜像变体 | |

**User's choice:** [auto] 直接修改现有 Dockerfile (recommended default)
**Notes:** 所有容器统一具备 FUSE 能力，维护一份镜像更简单。当前受管镜像已包含完整工具链，追加 sshfs/fuse3 不增加显著体积。

---

## FUSE 权限参数范围

| Option | Description | Selected |
|--------|-------------|----------|
| 所有容器默认附加 | --device /dev/fuse + --cap-add SYS_ADMIN 对所有容器生效 | ✓ |
| 仅特定标记容器附加 | 通过 label 或配置项控制哪些容器获得 FUSE 权限 | |
| 通过配置项控制 | 在管理后台为每个主机单独配置是否启用 FUSE | |

**User's choice:** [auto] 所有容器默认附加 (recommended default)
**Notes:** cloud-claude 的设计意图是所有用户容器都可被 cloud-claude 连接，无需区分容器类型。条件性添加会增加配置复杂度但无实际收益。

---

## SSH Proxy 验证深度

| Option | Description | Selected |
|--------|-------------|----------|
| 代码审查 + 文档记录 | 审查 proxy.go 确认多 session 和 exec 转发能力，记录结论 | ✓ |
| 新增自动化测试 | 编写测试用例验证多 session channel 并发 | |
| 手动集成测试脚本 | 编写 shell 脚本在容器中手动验证 | |

**User's choice:** [auto] 代码审查 + 文档记录 (recommended default)
**Notes:** SSH Proxy 代码已明确支持多 session channel（handleConnection 循环接受所有 channel）和全类型请求转发（handleChannel 双向转发所有请求类型）。代码本身不需要改动，因此不需要新增测试。

---

## Claude's Discretion

- sshfs/fuse3 apt 包名选择
- Dockerfile 中新增指令位置
- entrypoint.sh 中是否添加 FUSE 就绪检查

## Deferred Ideas

None
