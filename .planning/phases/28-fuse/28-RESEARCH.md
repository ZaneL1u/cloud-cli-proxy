# Phase 28: 生产环境 FUSE 兼容性验证 - Research

**Researched:** 2026-04-15
**Domain:** Docker FUSE + Linux 安全模块兼容性（AppArmor / seccomp）
**Confidence:** HIGH

## Summary

在 Docker 容器内执行 sshfs FUSE 挂载需要突破两层安全屏障：seccomp 系统调用过滤和 AppArmor 强制访问控制。当前 `worker.go` 已有 `--cap-add SYS_ADMIN` + `--device /dev/fuse`，seccomp 层面已不构成阻碍（默认 seccomp profile 在容器拥有 `CAP_SYS_ADMIN` 时条件放行 `mount`/`umount`/`umount2` 系统调用），但 Docker 的 `docker-default` AppArmor profile 包含 `deny mount,` 规则，会无条件拦截所有挂载操作（包括 FUSE），这是当前阻断 sshfs 的**主要障碍**。

此外，Ubuntu 25.04+ 引入了宿主机级别的 `/etc/apparmor.d/fusermount3` AppArmor profile，即使容器设为 `apparmor=unconfined` 也可能因宿主机 fusermount3 profile 的路径限制而失败——但此问题仅影响容器内直接调用 `fusermount3` 的场景。在我们的架构中，sshfs 以 root（容器内）运行并拥有 `CAP_SYS_ADMIN`，可直接调用 `mount(2)` 系统调用而非 setuid 的 `fusermount3`，因此宿主机 fusermount3 profile 的影响有限但仍需验证。

sshfs slave 模式的 SFTP 数据走 SSH session channel 进程内 pipe，不经过容器网络栈，与 nftables 默认拒绝策略和 sing-box tun / WireGuard 隧道不存在数据路径冲突。验证只需在全隧道出网状态下确认此结论成立。

**Primary recommendation:** 在 `worker.go` 的容器创建参数中添加 `--security-opt apparmor=unconfined`，编写自动化验证脚本覆盖三项 Success Criteria，并更新部署文档说明 FUSE 前置要求。

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** 编写可复用的 shell 验证脚本（`scripts/verify-fuse-compat.sh`），自动检测宿主机安全模块状态、执行容器内 FUSE 挂载测试、验证读写操作、确认网络策略不阻断映射通道
- **D-02:** 验证脚本覆盖三项 Success Criteria：FUSE 挂载+读写、FUSE+nftables/sing-box 共存、端到端完整流程
- **D-03:** 优先检测默认 `docker-default` AppArmor profile 和 Docker 默认 seccomp profile 是否允许 FUSE 操作
- **D-04:** 如验证发现阻断，在 `worker.go` 添加 `--security-opt` 选项，优先最小权限方案
- **D-05:** 验证脚本加入 AppArmor 和 seccomp 状态检测步骤
- **D-06:** sshfs slave 模式 SFTP 数据走进程内 pipe，不经过容器网络栈
- **D-07:** 验证场景同时启用全隧道出网
- **D-08:** 产出物包括验证脚本、部署文档更新、必要代码修复
- **D-09:** 验证脚本输出结构化结果（PASS/FAIL + 诊断信息）

### Claude's Discretion
- 验证脚本的具体实现结构和诊断信息格式
- 是否需要编写自定义 AppArmor profile 文件或仅使用 `--security-opt` 开关
- 部署文档的更新范围和详细程度
- 验证脚本中是否包含性能基准测试

### Deferred Ideas (OUT OF SCOPE)
- Mutagen 备选目录映射路径 — v2.x ENH-01
- 大目录 ignore 策略 — v2.x ENH-04
- sshfs 性能调优（reconnect、cache 参数）
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| SRV-04 | 在 Linux 生产环境验证 FUSE + AppArmor/seccomp 兼容性 | AppArmor `deny mount,` 规则确认为主要阻断因素；seccomp 在有 CAP_SYS_ADMIN 时已放行；需 `--security-opt apparmor=unconfined` 修复 |
</phase_requirements>

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| FUSE 挂载系统调用 | 容器运行时（Docker + 内核） | 宿主机安全模块 | FUSE mount(2) 在容器 namespace 内执行，受 Docker seccomp/AppArmor + 宿主机 AppArmor 双层控制 |
| sshfs slave 数据传输 | SSH session（进程内 pipe） | — | 不经过容器网络栈，完全在 SSH channel 的 stdin/stdout 上运行 |
| 容器网络隔离 | 宿主机代理（nftables + WireGuard/sing-box） | 容器 namespace | 已由 Phase 2-8 实现，本阶段仅验证与 FUSE 无冲突 |
| 容器创建参数 | 控制面（worker.go） | — | `--security-opt` 参数在 createHost() 中追加 |

## Standard Stack

本阶段无新增库依赖。工作集中在现有代码的配置调整和验证脚本编写。

### 核心工具

| 工具 | 用途 | 已有/新增 |
|------|------|-----------|
| Docker Engine 28.x | 容器运行时，seccomp/AppArmor 控制 | 已有 |
| sshfs + fuse3 | 容器内 FUSE 文件系统挂载 | 已有（镜像预装） |
| AppArmor utils (`aa-status`, `apparmor_parser`) | 宿主机安全模块状态检测 | 宿主机已有 |
| `mountpoint` | 验证 FUSE 挂载是否成功 | 容器内已有 |
| `nft` / `iptables` | 防火墙规则检查 | 宿主机已有 |

## Architecture Patterns

### 系统架构图 — FUSE 挂载数据流

```
用户终端 (macOS/Linux)
    │
    ▼
cloud-claude CLI ──SSH连接──▶ SSH Proxy (宿主机 :2222)
    │                            │
    │  Session 1: PTY (claude)   │  转发到容器 :22
    │  Session 2: sshfs slave    │
    │                            ▼
    │                    ┌─── Docker 容器 ───────────────────────┐
    │                    │                                       │
    │  [SFTP Server]◄────┤─── stdin/stdout pipe ───►[sshfs -o passive]
    │  (cloud-claude内)  │         ▲                     │      │
    │                    │         │                     mount(2)│
    │                    │    不经过网络栈              ▼      │
    │                    │                        /workspace     │
    │                    │                       (FUSE mount)    │
    │                    │                                       │
    │                    │  ┌─ 安全边界检查链 ──────────────┐    │
    │                    │  │ 1. seccomp: mount gated by     │    │
    │                    │  │    CAP_SYS_ADMIN → ✓ ALLOWED   │    │
    │                    │  │ 2. AppArmor docker-default:    │    │
    │                    │  │    deny mount → ✗ BLOCKED      │    │
    │                    │  │    (需 apparmor=unconfined)     │    │
    │                    │  │ 3. 宿主机 fusermount3 profile: │    │
    │                    │  │    Ubuntu 25.04+ 可能阻断      │    │
    │                    │  └────────────────────────────────┘    │
    │                    │                                       │
    │                    │  网络: nftables 默认拒绝 + 隧道出网   │
    │                    │  (与 FUSE 数据路径无交集)             │
    │                    └───────────────────────────────────────┘
```

### Pattern 1: AppArmor 解除方案 — `--security-opt apparmor=unconfined`

**What:** 禁用容器的 AppArmor profile，允许容器内进程执行 `mount(2)` 系统调用。

**When to use:** 容器需要 FUSE 挂载能力且安全边界由其他机制保障（本项目中由 nftables 默认拒绝 + 隧道出网 + namespace 隔离保障）。

**How it works in worker.go:**
```go
args := []string{
    "create",
    "--name", containerName,
    "--network", "bridge",
    "--cap-add", "NET_ADMIN",
    "--cap-add", "SYS_ADMIN",
    "--device", "/dev/fuse",
    "--security-opt", "apparmor=unconfined",  // 新增
    // ... 其余参数不变
}
```

**安全影响评估:**
- 本项目容器的网络隔离由 WireGuard/sing-box tun + nftables 默认拒绝 + namespace 路由保障（Phase 2-8），不依赖 AppArmor
- 容器用户为非特权用户 `workspace`（UID 1000），sudo 需要密码
- 容器已有 `CAP_SYS_ADMIN` + `CAP_NET_ADMIN`，AppArmor 的 `deny mount` 是仅剩的额外限制
- 禁用 AppArmor profile 的安全降级风险在现有隔离架构下可接受

[VERIFIED: Docker docs + moby/moby#33060 + moby/moby#16233]

### Pattern 2: 自定义 AppArmor Profile（备选方案）

**What:** 编写允许 FUSE 挂载但保留其他限制的自定义 AppArmor profile。

**When to use:** 仅当安全审计要求不能使用 `apparmor=unconfined` 时考虑。

```
profile cloud-cli-proxy flags=(attach_disconnected,mediate_deleted) {
  #include <abstractions/base>
  network,
  capability,
  file,
  umount,
  mount fstype=fuse,
  mount fstype=fuse.*,
  deny @{PROC}/* w,
  deny @{PROC}/sysrq-trigger rwklx,
  deny @{PROC}/kcore rwklx,
  deny /sys/firmware/** rwklx,
  deny /sys/kernel/security/** rwklx,
}
```

**Tradeoff:** 增加运维复杂度（需在每台宿主机安装和加载 profile），收益有限（项目安全边界不依赖 AppArmor）。

[CITED: moby/moby PR#41880]

### Pattern 3: 验证脚本结构化输出

**What:** 验证脚本遵循 PASS/FAIL 结构化输出，与现有 `verify.go` 三项网络检测模式对齐。

```bash
[INFO]  宿主机安全模块检测
[PASS]  AppArmor: 已加载, 状态: enforcing
[PASS]  fusermount3 profile: 未加载 (Ubuntu 24.04)
[INFO]  容器 FUSE 挂载测试
[PASS]  sshfs FUSE 挂载成功
[PASS]  FUSE 读写验证通过
[INFO]  网络策略共存测试
[PASS]  FUSE + nftables 默认拒绝: 共存正常
[PASS]  FUSE + 全隧道出网: 共存正常
[INFO]  端到端流程验证
[PASS]  cloud-claude → SSH Proxy → 目录映射 → Claude Code: 端到端通过
========================================
验证结果: 7/7 PASS, 0 FAIL
```

### Anti-Patterns to Avoid

- **使用 `--privileged` 替代精确的安全选项:** `--privileged` 禁用所有安全限制（seccomp、AppArmor、capabilities 上限），远超 FUSE 所需。应使用 `--cap-add SYS_ADMIN --device /dev/fuse --security-opt apparmor=unconfined` 的精确组合。
- **在部署文档中遗漏 FUSE 内核模块检查:** 如果宿主机没有加载 `fuse` 内核模块，`/dev/fuse` 设备不存在，容器内 FUSE 操作会静默失败。
- **忽略 Ubuntu 版本差异:** Ubuntu 24.04 和 25.04+ 的 AppArmor 行为有显著差异（fusermount3 profile），验证脚本必须检测并区分处理。

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| AppArmor 状态检测 | 自己解析 `/sys/kernel/security/apparmor` | `aa-status` + `aa-enabled` | 标准工具覆盖所有发行版差异 |
| FUSE 挂载验证 | 自写挂载检测逻辑 | `mountpoint -q /path` | 已被 mount.go 的 waitForMount 采用的标准方式 |
| seccomp profile 检查 | 解析 OCI runtime spec | `docker info` 输出中的 Security Options | 足够判断 seccomp 是否启用 |

## Common Pitfalls

### Pitfall 1: AppArmor `deny mount` 与 capabilities 的优先级

**What goes wrong:** 开发者以为 `--cap-add SYS_ADMIN` 足以允许 `mount(2)` 系统调用，但 AppArmor 的 `deny` 规则优先级高于 Linux capabilities。容器拥有 `CAP_SYS_ADMIN` 只是通过了 seccomp 检查，AppArmor 仍然会拦截。

**Why it happens:** Linux 安全模块（LSM）的检查链是：seccomp → DAC → capabilities → AppArmor/SELinux。即使前面的检查全部通过，AppArmor 的显式 `deny` 仍会拒绝操作。Docker 的 `docker-default` profile 包含 `deny mount,`。

**How to avoid:** 添加 `--security-opt apparmor=unconfined` 或使用允许 FUSE mount 的自定义 AppArmor profile。

**Warning signs:** `dmesg` 或 `/var/log/syslog` 中出现 `apparmor="DENIED" operation="mount" profile="docker-default" fstype="fuse.sshfs"`。

[VERIFIED: moby/moby#16233, moby/moby#33060, Docker docs — AppArmor security profiles]

### Pitfall 2: Ubuntu 25.04+ 宿主机 fusermount3 AppArmor Profile

**What goes wrong:** 即使容器设为 `apparmor=unconfined` 或 `--privileged`，fusermount3 在 Ubuntu 25.04+ 上仍然失败，报 `fusermount3: mount failed: Permission denied`。

**Why it happens:** Ubuntu 25.04 引入了宿主机级别的 `/etc/apparmor.d/fusermount3` AppArmor profile。这个 profile 不是 Docker 容器 profile，而是对 `fusermount3` 二进制文件本身的限制，会影响所有 namespace 中的 fusermount3 调用。

**How to avoid:**
1. 验证脚本应检测 fusermount3 profile 是否存在并处于 enforcing 状态
2. 如果存在，在 `/etc/apparmor.d/local/fusermount3` 添加 `capability dac_override,` 然后 `apparmor_parser -r /etc/apparmor.d/fusermount3`
3. 或使用 `aa-disable /usr/bin/fusermount3`
4. 在本项目中，sshfs 以 root 运行且有 `CAP_SYS_ADMIN`，会直接调用 `mount(2)` 而非 fusermount3，影响可能有限

**Warning signs:** `audit` 日志中 `profile="fusermount3" ... apparmor="DENIED"`。

[VERIFIED: moby/moby#50013, Launchpad Bug#2111105, containerd/stargz-snapshotter#2144]

### Pitfall 3: FUSE 内核模块未加载

**What goes wrong:** `/dev/fuse` 设备不存在，Docker 的 `--device /dev/fuse` 选项静默失败或容器启动报错。

**Why it happens:** 某些精简版 Linux 内核或云服务商的定制内核未编译 `fuse` 模块，或模块未自动加载。

**How to avoid:** 验证脚本首先检查 `modprobe fuse` 和 `/dev/fuse` 存在性。部署文档中列出 FUSE 内核模块为前置条件。

**Warning signs:** `ls -la /dev/fuse` 返回 "No such file or directory"；`lsmod | grep fuse` 无输出。

[ASSUMED]

### Pitfall 4: sshfs passive 模式挂载点权限

**What goes wrong:** 容器内 `/workspace` 在 sshfs 挂载前已有内容或权限不正确，导致 FUSE 挂载失败或用户无法访问。

**Why it happens:** entrypoint.sh 已设置 `/workspace` 归属为容器用户，但 sshfs 挂载会覆盖目录的可见内容。如果挂载以 root 执行但 `user_allow_other` 未配置，非 root 用户将无法访问挂载点。

**How to avoid:** Dockerfile 已配置 `user_allow_other`（`sed -i 's/^#user_allow_other/user_allow_other/' /etc/fuse.conf`）。验证脚本应以容器内普通用户身份验证挂载点的读写权限。

[VERIFIED: 代码库中 deploy/docker/managed-user/Dockerfile 第 43-44 行]

## Code Examples

### 在 worker.go 中添加 AppArmor 解除参数

```go
// Source: worker.go createHost() 修改点
args := []string{
    "create",
    "--name", containerName,
    "--network", "bridge",
    "--cap-add", "NET_ADMIN",
    "--cap-add", "SYS_ADMIN",
    "--device", "/dev/fuse",
    "--security-opt", "apparmor=unconfined",
    "--label", "cloud-cli-proxy.managed=true",
    // ...
}
```

### 验证脚本：AppArmor 状态检测

```bash
detect_apparmor_status() {
    if ! command -v aa-enabled >/dev/null 2>&1; then
        echo "[INFO]  AppArmor: 未安装"
        return 0
    fi

    if aa-enabled --quiet 2>/dev/null; then
        echo "[INFO]  AppArmor: 已启用 (enforcing)"

        if aa-status 2>/dev/null | grep -q "fusermount3"; then
            echo "[WARN]  fusermount3 AppArmor profile 已加载 (Ubuntu 25.04+)"
            echo "[WARN]  可能需要调整: /etc/apparmor.d/local/fusermount3"
        else
            echo "[INFO]  fusermount3 AppArmor profile: 未加载"
        fi
    else
        echo "[INFO]  AppArmor: 未启用"
    fi
}
```

### 验证脚本：容器内 FUSE 挂载测试

```bash
test_fuse_mount() {
    local container="$1"
    local test_dir="/tmp/fuse-test-$$"

    mkdir -p "$test_dir"
    echo "fuse-test-content" > "$test_dir/test.txt"

    # 在容器内启动 sshfs passive 挂载（模拟 cloud-claude 的挂载流程）
    docker exec "$container" bash -c '
        mkdir -p /tmp/fuse-verify
        sshfs : /tmp/fuse-verify -o passive -f &
        SSHFS_PID=$!
        sleep 2
        if mountpoint -q /tmp/fuse-verify; then
            echo "MOUNT_OK"
            echo "test-write" > /tmp/fuse-verify/write-test.txt
            if [ -f /tmp/fuse-verify/write-test.txt ]; then
                echo "WRITE_OK"
            fi
        fi
        kill $SSHFS_PID 2>/dev/null
        fusermount -u /tmp/fuse-verify 2>/dev/null || true
    '

    rm -rf "$test_dir"
}
```

## State of the Art

| 旧方案 | 当前方案 | 变更时间 | 影响 |
|--------|---------|---------|------|
| Docker 容器 FUSE 需要 `--privileged` | `--cap-add SYS_ADMIN --device /dev/fuse --security-opt apparmor=unconfined` | Docker 1.13+ | 精确授权替代全开，安全性更好 |
| Ubuntu 24.04: 无 fusermount3 AppArmor profile | Ubuntu 25.04+: 新增 `/etc/apparmor.d/fusermount3` | 2025-04 | 宿主机 OS 升级后 FUSE 可能失效，需部署文档提示 |
| moby/moby PR#41880: 尝试原生支持 FUSE | PR 仍为 open 状态（2021-01 至今） | — | Docker 短期内不会原生支持 FUSE，需持续使用 security-opt 方案 |

**Deprecated/outdated:**
- Docker 早期的 `--cap-add SYS_ADMIN --device /dev/fuse` 双参数方案在 AppArmor 启用的系统上不充分，必须配合 `--security-opt apparmor=unconfined`

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | FUSE 内核模块在目标宿主机已加载或可加载 | Common Pitfalls | 如果宿主机内核未编译 FUSE 支持，整个映射方案不可用，需退回到 bind mount |
| A2 | sshfs 以 root 运行时直接调用 mount(2) 而非 fusermount3 | Summary | 如果 sshfs 始终调用 fusermount3，Ubuntu 25.04+ 宿主机 fusermount3 profile 会成为额外阻碍 |
| A3 | Docker Engine 28.x 的默认 AppArmor 模板与已知 docker-default 内容一致 | Common Pitfalls | 如果新版 Docker 修改了模板（如放开 FUSE mount），`--security-opt` 可能不再必需 |

## Open Questions

1. **sshfs 在 root 权限下是否绕过 fusermount3？**
   - What we know: fusermount3 是 setuid helper，用于非特权用户执行 FUSE mount。root 用户通常直接调用 mount(2)
   - What's unclear: sshfs 的实现是否在 root 时仍调用 fusermount3（某些版本可能硬编码使用 fusermount）
   - Recommendation: 验证脚本应同时检测两种路径，在 `dmesg` 中确认实际触发的是 `mount(2)` 还是 `fusermount3`

2. **目标宿主机 Ubuntu 版本是 24.04 LTS 还是更新版本？**
   - What we know: Dockerfile 基于 `ubuntu:24.04`，但宿主机 OS 可能不同
   - What's unclear: 生产宿主机是否运行 Ubuntu 25.04+
   - Recommendation: 验证脚本应检测宿主机 OS 版本并输出到诊断信息，24.04 和 25.04+ 走不同处理分支

## Environment Availability

> 本阶段依赖的外部工具均为宿主机和容器内已有组件，不需要额外安装。

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Docker Engine | 容器创建和管理 | ✓ | 28.x | — |
| FUSE 内核模块 | /dev/fuse 设备 | 需验证 | — | 无（阻断性） |
| sshfs | 容器内 FUSE 挂载 | ✓ | 容器镜像预装 | — |
| fuse3 | fusermount3 | ✓ | 容器镜像预装 | — |
| AppArmor utils | 宿主机安全模块检测 | 需验证 | — | 跳过 AppArmor 检测步骤 |
| nft | 防火墙规则检查 | ✓ | 宿主机已有 | — |

**Missing dependencies with no fallback:**
- FUSE 内核模块（如果未加载，验证脚本应报告并建议 `modprobe fuse`）

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | — |
| V3 Session Management | no | — |
| V4 Access Control | yes | 容器安全隔离由 nftables + namespace + 隧道保障，AppArmor 禁用不降低实际隔离等级 |
| V5 Input Validation | no | — |
| V6 Cryptography | no | — |

### Known Threat Patterns

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| 容器 AppArmor 禁用后的 mount 滥用 | Elevation of Privilege | 容器内非 root 用户无 CAP_SYS_ADMIN；网络隔离由 nftables 默认拒绝保障；mount namespace 隔离 |
| FUSE 挂载点绕过文件系统权限 | Tampering | sshfs 挂载由控制面发起（cloud-claude），不暴露给容器内用户 |

## Sources

### Primary (HIGH confidence)
- [Docker Docs — Seccomp security profiles](https://docs.docker.com/engine/security/seccomp) — 确认 mount/umount/umount2 在 CAP_SYS_ADMIN 条件下被默认 seccomp profile 放行
- [moby/profiles seccomp/default.json](https://github.com/moby/profiles/blob/main/seccomp/default.json) — 源码确认 mount 在 `includes.caps: ["CAP_SYS_ADMIN"]` 条件下为 `SCMP_ACT_ALLOW`
- [Docker Docs — AppArmor security profiles](https://docs.docker.com/engine/security/apparmor) — 确认 docker-default profile 包含 `deny mount,`
- [moby/moby#33060](https://github.com/moby/moby/issues/33060) — docker-default AppArmor profile 完整模板，确认 `deny mount,` 存在
- [moby/moby#16233](https://github.com/moby/moby/issues/16233) — sshfs FUSE 在 docker-default AppArmor 下被拦截的确认案例，`--security-opt apparmor:unconfined` 为解决方案

### Secondary (MEDIUM confidence)
- [moby/moby#50013](https://github.com/moby/moby/issues/50013) — Ubuntu 25.04 + Docker 28.1.1 FUSE 挂载失败，AppArmor fusermount3 profile 为根因
- [Launchpad Bug#2111105](https://bugs.launchpad.net/ubuntu/+source/fuse3/+bug/2111105) — Ubuntu 25.04 fuse3 + AppArmor 兼容性 bug 报告
- [Launchpad Bug#2111807](https://bugs.launchpad.net/ubuntu/+source/apparmor/+bug/2111807) — Ubuntu 25.04 sshfs + fusermount3 AppArmor 修复（apparmor 4.1.0~beta5-0ubuntu14.1）
- [containerd/stargz-snapshotter#2144](https://github.com/containerd/stargz-snapshotter/issues/2144) — Ubuntu 25.04+ fusermount3 AppArmor profile 影响确认
- [moby/moby PR#41880](https://github.com/moby/moby/pull/41880) — Docker 原生 FUSE 支持 PR（仍为 open 状态）

### Tertiary (LOW confidence)
- 无

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — 无新增依赖，所有工具已在代码库中使用
- Architecture: HIGH — FUSE + AppArmor/seccomp 交互机制已通过官方文档和 GitHub issues 交叉验证
- Pitfalls: HIGH — 基于多个真实 bug 报告和官方源码确认
- 网络共存: MEDIUM — sshfs slave pipe 不经过网络栈的结论基于架构分析，需生产环境实证

**Research date:** 2026-04-15
**Valid until:** 2026-05-15（稳定领域，Docker/AppArmor 变化缓慢）
