# Phase 24: 受管镜像 FUSE 硬化与容器参数 - Context

**Gathered:** 2026-04-15
**Status:** Ready for planning

<domain>
## Phase Boundary

交付容器侧 FUSE/sshfs 的前置条件和运行参数，使受管镜像包含 sshfs + fuse3 工具、容器创建时附加 FUSE 设备权限，并通过代码审查验证 SSH Proxy 现有多 session channel 和 exec 转发能力可直接支持 cloud-claude 连接模式。

本阶段不涉及 cloud-claude CLI 开发（Phase 25）、参数透传（Phase 26）或目录映射实现（Phase 27），但为这些后续阶段建立服务端基础。

</domain>

<decisions>
## Implementation Decisions

### 镜像修改策略
- **D-01:** 直接在现有 `deploy/docker/managed-user/Dockerfile` 中添加 sshfs + fuse3 包，不创建新的镜像变体。所有用户容器统一具备 FUSE 能力，减少镜像维护负担。
- **D-02:** 在镜像中配置 `/etc/fuse.conf` 启用 `user_allow_other`，允许非 root 用户使用 `-o allow_other` 选项挂载 sshfs。

### FUSE 权限参数范围
- **D-03:** Worker 创建容器时对所有容器统一附加 `--device /dev/fuse` 和 `--cap-add SYS_ADMIN`，不做条件区分。cloud-claude 设计意图是所有用户容器都可被连接。
- **D-04:** 新增参数在 `internal/runtime/tasks/worker.go` 的 `createHost()` 函数中添加，与现有 `--cap-add NET_ADMIN` 同级。

### SSH Proxy 验证
- **D-05:** SSH Proxy 保持零改造。通过代码审查确认现有能力并在本 CONTEXT 中记录结论：
  - `handleConnection()` 循环接受所有 session channel（`for newChan := range chans`），天然支持多 session
  - `handleChannel()` 双向转发所有请求类型（pty-req, shell, exec, env, window-change, exit-status, exit-signal）
  - 单个 SSH 连接可承载多个并发 session channel，满足 cloud-claude 需要同时建立交互 session 和 sshfs slave session 的需求
- **D-06:** 验证结论以文档形式记录在计划中，不新增自动化测试（SSH Proxy 代码本身不改动）。

### Claude's Discretion
- sshfs 和 fuse3 的具体 apt 包名选择（`sshfs` + `fuse3` 或更细粒度的包选择）
- Dockerfile 中新增 RUN 指令的位置（合并到现有 apt-get install 还是独立 RUN 层）
- 是否需要在 entrypoint.sh 中添加 FUSE 相关检查逻辑

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### 需求定义
- `.planning/REQUIREMENTS.md` — SRV-01, SRV-02, SRV-03 定义了镜像预装、容器参数和 SSH Proxy 零改造的具体要求

### 现有镜像
- `deploy/docker/managed-user/Dockerfile` — 当前受管镜像的完整定义，需要在此基础上添加 sshfs/fuse3
- `deploy/docker/managed-user/entrypoint.sh` — 容器 entrypoint 脚本，可能需要添加 FUSE 就绪检查

### Worker 容器创建
- `internal/runtime/tasks/worker.go` — createHost() 函数构建 docker create 参数，需要在此添加 FUSE 设备和 SYS_ADMIN

### SSH Proxy（零改造验证）
- `internal/sshproxy/proxy.go` — handleConnection() 和 handleChannel() 已实现多 session channel 和全类型请求转发

### 项目约束
- `.planning/PROJECT.md` — Key Decisions 表和 v2.0 里程碑目标

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `deploy/docker/managed-user/Dockerfile`: Ubuntu 24.04 基础镜像，已有完整的 apt-get install 块、用户创建、SSH 配置等，sshfs/fuse3 可直接追加到现有包列表
- `internal/runtime/tasks/worker.go`: createHost() 已有清晰的 args 切片构建模式，添加 `--device` 和 `--cap-add` 参数只需在现有位置追加

### Established Patterns
- 容器参数通过 `args = append(args, ...)` 逐步构建，新增参数自然融入此模式
- 容器内工作用户为 workspace (UID 1000)，sshfs 挂载需确保此用户有 FUSE 访问权限
- 受管镜像使用单一 Dockerfile + entrypoint.sh 模式
- 容器已有 `--cap-add NET_ADMIN` 用于网络隔离，`--cap-add SYS_ADMIN` 是同级别的权限添加

### Integration Points
- Phase 25 的 cloud-claude CLI 将通过 SSH Proxy 连接到容器，依赖本阶段确保的多 session 能力
- Phase 27 的 sshfs slave 映射依赖本阶段在镜像中预装的 sshfs 和 FUSE 设备权限
- Phase 28 将验证 FUSE + AppArmor/seccomp 在生产环境的兼容性

</code_context>

<specifics>
## Specific Ideas

No specific requirements — open to standard approaches

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 24-fuse*
*Context gathered: 2026-04-15*
