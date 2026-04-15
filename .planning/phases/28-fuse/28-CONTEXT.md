# Phase 28: 生产环境 FUSE 兼容性验证 - Context

**Gathered:** 2026-04-15
**Status:** Ready for planning

<domain>
## Phase Boundary

在目标 Linux 生产宿主机上验证 FUSE + AppArmor/seccomp 安全模块的完整兼容性，确保 cloud-claude 全链路（CLI → SSH Proxy → sshfs 目录映射 → Claude Code 运行）在生产环境端到端可用。

本阶段不涉及：新功能开发、sshfs 映射逻辑修改（Phase 27 已完成）、cloud-claude CLI 改动（Phase 25-26 已完成）。本阶段可能包含：根据验证结果对 `worker.go` 容器创建参数做 `--security-opt` 调整、编写验证脚本、更新部署文档。

</domain>

<decisions>
## Implementation Decisions

### 验证环境与方法
- **D-01:** 编写一个可复用的 shell 验证脚本（`scripts/verify-fuse-compat.sh`），自动检测宿主机 AppArmor/seccomp 状态、执行容器内 FUSE 挂载测试、验证读写操作、确认网络策略不阻断映射通道。脚本应支持在不同 Linux 发行版上重复执行。
- **D-02:** 验证脚本应覆盖三项 Success Criteria 对应的测试点：1) FUSE 挂载成功 + 读写正常，2) FUSE 与 nftables/sing-box 共存，3) 端到端完整流程验证。

### AppArmor/seccomp 处理策略
- **D-03:** 优先检测默认 `docker-default` AppArmor profile 和 Docker 默认 seccomp profile 是否允许 FUSE 操作。当前 `worker.go` 已有 `--cap-add SYS_ADMIN` + `--device /dev/fuse`，大多数情况下 FUSE 在默认 profile 下可正常工作。
- **D-04:** 如果验证发现默认 profile 阻断 FUSE 操作（如 `mount` 系统调用被 seccomp 拦截或 AppArmor deny），在 `worker.go` 的容器创建参数中添加必要的 `--security-opt` 选项。优先使用最小权限方案（自定义 seccomp profile 白名单 `mount`/`umount2` 或自定义 AppArmor profile），退而求其次才使用 `apparmor=unconfined`。
- **D-05:** 验证脚本中加入 AppArmor 和 seccomp 状态检测步骤，在测试前报告宿主机安全模块状态，帮助排障。

### FUSE 与网络策略共存
- **D-06:** sshfs slave 模式的 SFTP 数据走 SSH session channel（进程内 pipe），不经过容器网络栈，因此与 nftables 默认拒绝策略和 sing-box tun 隧道不存在冲突。验证重点是在生产环境确认这一点成立，而非编写额外的防火墙规则。
- **D-07:** 验证场景应同时启用全隧道出网（WireGuard 或 sing-box tun + nftables 默认拒绝），在此状态下测试 sshfs 挂载和文件读写。

### 验证产出物
- **D-08:** 产出物包括：1) 验证脚本（`scripts/verify-fuse-compat.sh`），2) 部署文档更新（说明 FUSE 在不同 Linux 发行版上的前置要求和已知限制），3) 根据验证结果对 `worker.go` 做的必要代码修复（如 `--security-opt` 参数添加）。
- **D-09:** 验证脚本应输出结构化结果（PASS/FAIL + 诊断信息），支持 CI 集成和运维人员手工复查。

### Claude's Discretion
- 验证脚本的具体实现结构和诊断信息格式
- 是否需要编写自定义 AppArmor profile 文件或仅使用 `--security-opt` 开关
- 部署文档的更新范围和详细程度
- 验证脚本中是否包含性能基准测试（sshfs 读写延迟和吞吐量）

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### 需求定义
- `.planning/REQUIREMENTS.md` — SRV-04 定义了在 Linux 生产环境验证 FUSE + AppArmor/seccomp 兼容性的要求

### 路线图与成功标准
- `.planning/ROADMAP.md` — Phase 28 Goal 和三项 Success Criteria

### 前序阶段产出
- `.planning/phases/24-fuse/24-CONTEXT.md` — D-01~D-06：sshfs/fuse3 预装、FUSE 设备权限、SSH Proxy 多 session 确认
- `.planning/phases/27-session/27-CONTEXT.md` — D-01~D-09：sshfs slave + SFTP server 完整映射方案和清理逻辑

### 容器创建代码
- `internal/runtime/tasks/worker.go` — createHost() 函数，当前已有 `--cap-add SYS_ADMIN` + `--device /dev/fuse`，无 `--security-opt`
- `deploy/docker/managed-user/Dockerfile` — 受管镜像已预装 sshfs + fuse3，已配置 user_allow_other
- `deploy/docker/managed-user/entrypoint.sh` — 已有 `chmod 666 /dev/fuse` 设备权限设置

### 目录映射实现
- `internal/cloudclaude/mount.go` — mountWorkspace / waitForMount / fusermountCleanup 完整实现

### 网络隔离
- `internal/network/` — WireGuard 和 sing-box 两种 provider 的隧道实现

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/runtime/tasks/worker.go`: createHost() 已有清晰的 args 切片构建模式，新增 `--security-opt` 参数只需在 `--cap-add SYS_ADMIN` 同级追加
- `deploy/docker/managed-user/entrypoint.sh`: 已有 FUSE 设备权限设置（`chmod 666 /dev/fuse`），可作为验证基线
- `internal/cloudclaude/mount.go`: mountWorkspace 和 waitForMount 可直接用于端到端验证流程中的挂载测试

### Established Patterns
- 容器参数通过 `args = append(args, ...)` 逐步构建，新增 `--security-opt` 参数自然融入此模式
- 验证脚本可参考现有 `scripts/` 目录下的 shell 脚本风格
- 网络测试参考 v1.1 的代理测试 API 三项检测模式（连通性 + 出口 IP 匹配 + DNS 泄漏）

### Integration Points
- 如果需要添加 `--security-opt`，修改点在 `worker.go` createHost() 的 args 构建区域
- 验证脚本需要访问 Docker Engine 和容器内 sshfs 命令
- 部署文档更新对应 `docs/` 或项目根目录的部署指南

</code_context>

<specifics>
## Specific Ideas

无额外产品偏好 — `/gsd-discuss-phase 28 --auto` 采用推荐默认（自动化验证脚本 + 检测优先的安全策略 + 必要时最小权限修复）。

</specifics>

<deferred>
## Deferred Ideas

- Mutagen 备选目录映射路径 — v2.x ENH-01
- 大目录 ignore 策略 — v2.x ENH-04
- sshfs 性能调优（reconnect、cache 参数）— 当性能成为主诉时再评估

</deferred>

---

*Phase: 28-fuse*
*Context gathered: 2026-04-15*
