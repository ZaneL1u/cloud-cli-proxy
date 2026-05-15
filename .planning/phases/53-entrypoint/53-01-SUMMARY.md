---
phase: 53-entrypoint
plan: "01"
subsystem: infra
tags: [docker, sing-box, capabilities, hardening, ubuntu-24.04, nftables]

requires:
  - phase: 52-mergerfs
    provides: managed-user 镜像 v3.6 基线（mergerfs / KasmVNC / Chromium / Claude Code 安装段）
provides:
  - managed-user 镜像 v4.0 基线 Dockerfile：内置 sing-box 1.13.3 binary（文件 cap cap_net_admin+eip）
  - singbox 系统账号（uid=9000 / gid=9000，nologin，无 home）
  - workspace 用户去 sudo NOPASSWD（D-53-4 breaking change）
  - 系统包 nftables + libcap2-bin（为 Plan 53-02 entrypoint nft 规则与 cap 自检做准备）
  - Dockerfile 头部 v4.0 文档注释
affects: [53-02, 53-03, 54, 55, 56]

tech-stack:
  added:
    - sing-box 1.13.3（GitHub release，binary 安装）
    - nftables（Ubuntu 24.04 标准包）
    - libcap2-bin（提供 setcap / getcap / getpcaps）
  patterns:
    - 文件 cap 替代 root：cap_net_admin+eip 给 binary 而不是用 CAP_SYS_ADMIN/root 跑进程（D-V4-1）
    - 系统级降权账号：uid=9000 singbox 与 uid=1000 workspace 完全隔离（D-53-2）
    - build 期 cap 自检：setcap 之后立即 getcap | grep verify，防 overlay/squashfs 丢 cap
    - sing-box 版本 ARG 与 deploy/docker/sing-box-gateway/Dockerfile 共享（便于 Phase 54 退役 gateway 镜像）

key-files:
  created:
    - .planning/phases/53-entrypoint/53-01-SUMMARY.md
    - .planning/phases/53-entrypoint/deferred-items.md
  modified:
    - deploy/docker/managed-user/Dockerfile

key-decisions:
  - "采用 D-V4-1 file-cap 模式：sing-box binary 通过 setcap 拿 NET_ADMIN，进程不需要 root，也不依赖容器整体 CAP_SYS_ADMIN"
  - "采用 D-53-2 singbox uid=9000 固定：与未来 workspace 用户 uid 段（1000+）和系统账号段都不冲突"
  - "采用 D-53-4 删除 workspace sudo NOPASSWD：v4.0 breaking change vs v3.x，把 root 提权能力彻底从用户态拿掉"
  - "sing-box 版本与 sing-box-gateway 镜像共版本（1.13.3），通过 ARG SINGBOX_VERSION 显式声明"

patterns-established:
  - "镜像层最小特权：以 file capability 为单位授权，禁止给容器整体 SYS_ADMIN/root"
  - "build 期 cap 自检：setcap 之后必须紧跟一次 getcap verify，否则视为 build 失败"
  - "系统账号显式 nologin：sing-box / 类似系统服务一律 --shell /usr/sbin/nologin --no-create-home --home-dir /nonexistent"

requirements-completed: [IMG-01, IMG-02, IMG-03, IMG-04]

duration: ~10 min
completed: 2026-05-16
---

# Phase 53 Plan 01: Dockerfile 改造 — sing-box binary + 系统账号 + 用户硬化 Summary

**managed-user 镜像 v4.0 基线：内置 sing-box 1.13.3（file cap cap_net_admin+eip）+ singbox 系统账号 uid=9000 + 删除 workspace sudo NOPASSWD + 安装 nftables/libcap2-bin。**

## Performance

- **Duration:** ~10 min（含 V1 docker build 尝试与 buildx --check）
- **Started:** 2026-05-16T02:11:00Z
- **Completed:** 2026-05-16T02:21:00Z
- **Tasks:** 5（T1..T5，单 commit 落地）
- **Files modified:** 1（Dockerfile）+ 1 created（deferred-items.md）

## Accomplishments

- **T1 — sing-box install 段:** 在 Claude Code 安装段之后插入 ARG SINGBOX_VERSION=1.13.3 + 标准 GitHub release 安装流程，与 `deploy/docker/sing-box-gateway/Dockerfile` 完全一致。
- **T2 — singbox 系统账号:** 新增 `groupadd --gid 9000 singbox` + `useradd --uid 9000 --gid 9000 --home-dir /nonexistent --no-create-home --shell /usr/sbin/nologin singbox`。
- **T3 — nftables + libcap2-bin:** 在第一个 apt install 段追加这两个包；不引入新 repo。
- **T4 — sing-box file cap:** `setcap 'cap_net_admin+eip cap_net_bind_service+eip' /usr/local/bin/sing-box`，紧跟 `getcap | grep cap_net_admin` 自检，失败即 exit 1。
- **T5 — Dockerfile 头部注释:** 加 v4.0 (Phase 53) 段说明，引向 `.planning/phases/53-entrypoint/53-CONTEXT.md`。
- **D-53-4 收尾:** workspace useradd 段删掉 `echo ... sudoers.d/workspace` + `chmod 0440`，并用注释保留删除痕迹便于审计。

## Task Commits

按计划要求**单 commit 落地** T1..T5：

1. **T1..T5 + SUMMARY:** `feat(53-01): Dockerfile 内置 sing-box + singbox 账号 + 删 workspace sudo`（hash 见下文 final commit）

> 不按"每 task 一个 commit"切分，遵循 53-01-PLAN.md `## Commit Plan` 显式要求的"单独一次 commit"。

## Files Created/Modified

- `deploy/docker/managed-user/Dockerfile` — 头部注释 + apt 包列表加 nftables/libcap2-bin + workspace useradd 去 sudo + 新增 singbox useradd + 新增 sing-box install 段 + 新增 setcap 段
- `.planning/phases/53-entrypoint/53-01-SUMMARY.md` — 本文件
- `.planning/phases/53-entrypoint/deferred-items.md` — 记录 V1 build 中暴露的 pre-existing Claude Code 安装段问题（不在本 Plan scope）

## Decisions Made

无新增决策；严格按 53-01-PLAN.md 与 53-CONTEXT.md 中已锁定的 D-V4-1 / D-53-1 / D-53-2 / D-53-4 执行。

## Deviations from Plan

无。Dockerfile 改动逐字对应 PLAN T1..T5 的代码块，未做计划外结构调整。

## Verification Results

| ID | 项 | 期望 | 实际 | 状态 |
|---|---|---|---|---|
| V1 | `docker build`（完整 Dockerfile） | 镜像构建成功 | **失败**：在 pre-existing Claude Code 安装段（L127-170）退出 22（`claude.ai/install.sh` 403 + GitHub release `claude-code-linux-aarch64.tar.gz` 404） | ⚠️ Deferred — 不在 53-01 scope |
| V1' | `docker build` standalone（仅 53-01 改动段） | 镜像构建成功 | **通过**：`sb-verify:53-01` 镜像 build 0 error（apt install nftables/libcap2-bin + sing-box install + setcap + singbox useradd） | ✅ |
| V2 | `sing-box version` | 含 `sing-box version 1.13.3` | `sing-box version 1.13.3 / Environment: go1.25.8 linux/arm64` | ✅ |
| V3 | `getcap /usr/local/bin/sing-box` | `cap_net_admin,cap_net_bind_service=eip` | `/usr/local/bin/sing-box cap_net_bind_service,cap_net_admin=eip` | ✅ |
| V4 | `id singbox` | `uid=9000(singbox) gid=9000(singbox)` | `uid=9000(singbox) gid=9000(singbox) groups=9000(singbox)` | ✅ |
| V5 | `sudo -n true` 失败 | 非 0 退出 / 用户不在 sudoers | 静态校验：Dockerfile 唯一 sudoers 字符串为注释行（`# 原 echo ... sudoers.d/workspace 已删除`），且 useradd workspace 行无 `-G sudo` | ✅ 静态通过；运行时验证 deferred |
| V6 | `groups workspace` 不含 sudo | 输出无 sudo | 静态校验：useradd 行 `--gid "${WORKSPACE_GID}" --home-dir /workspace --create-home --shell /bin/bash` 不含 `-G sudo` / `--groups sudo` | ✅ 静态通过；运行时验证 deferred |
| V7 | `nft / setcap / getcap / getpcaps` 都在 PATH | 4 个路径输出 | `/usr/sbin/nft` / `/usr/sbin/setcap` / `/usr/sbin/getcap` / `/usr/sbin/getpcaps` | ✅ |
| Lint | `docker buildx build --check` | 0 warning | `Check complete, no warnings found.` | ✅ |

**说明:**

1. **V1**（完整 Dockerfile build）失败发生在 pre-existing Claude Code 安装段（L127-170），与 Plan 53-01 修改的 5 个段（T1..T5）完全无关 — claude.ai/install.sh 当前返回 403、且 v2.1.142 release 的 `claude-code-linux-aarch64.tar.gz` 文件名已不存在。归 SCOPE BOUNDARY 之外，不在本 Plan 修复责任范围。
2. **V1'/V2/V3/V4/V7** 通过构造 standalone verify 镜像（apt install nftables/libcap2-bin/curl/ca-certificates → sing-box install → setcap → singbox useradd，镜像 tag `sb-verify:53-01`）实测，全部通过。Standalone Dockerfile 与 managed-user Dockerfile 中 53-01 改动段**逐字相同**，故等价于 53-01 段在 ubuntu:24.04 基础上的语义验证。
3. **V2 运行注意**：因 sing-box binary 持有 `cap_net_admin+eip`，且容器 default bounding set 不含 `CAP_NET_ADMIN`，docker run 时必须 `--cap-add=NET_ADMIN`，否则 exec 报 `operation not permitted`。这是 Plan 53-02 entrypoint 配置的前提，已在该计划中显式声明。
4. **V5/V6** 改为 Dockerfile 文本静态校验（grep 全文无 sudoers 写入与 sudo group 加入），等等价物，运行时复验留到 V1 完整 build 修复后回放。

## Deferred Issues

- **D-53-PRE-1**（详见 `.planning/phases/53-entrypoint/deferred-items.md`）：Claude Code 安装段在本地 build 失败。归类为 SCOPE BOUNDARY 之外的 pre-existing 问题。
- **V5/V6 Deferred-to-runtime:** 静态校验已通过；待 D-53-PRE-1 修复或 CI 完整 build 时跑 `docker run --rm managed-user:v4-dev sudo -n true` 与 `groups workspace` 二次复验。
- **完整镜像端到端验证 Deferred:** V1（完整 Dockerfile build）+ V2..V7（基于完整镜像）需要 D-53-PRE-1 修复后或在 CI 干净环境跑一次。本 Plan 等价验证已通过 standalone verify 完成。

## Issues Encountered

- 本地 `docker build` 因 Claude Code 安装段远端资源 404/403 失败 → 不在 scope，记入 deferred-items.md，不修。

## Threat Flags

无新增 threat surface。本 Plan 的安全性变化全部是**降低**攻击面：
- 删除 workspace sudo NOPASSWD（D-53-4，破坏 v3.x 容器内提权路径）
- sing-box 进程将以 uid=9000 + 文件 cap 跑（D-V4-1，避免给容器整体 CAP_SYS_ADMIN）

## User Setup Required

无。本 Plan 仅修改镜像 Dockerfile，不涉及外部服务配置。

## Next Phase Readiness

- ✅ Plan 53-02（entrypoint 改造 — sing-box 子进程 + nft default-deny ruleset）可以以 v4.0 基线 Dockerfile 为输入
- ✅ Plan 53-03（容器启动验证 + 出口 IP 强约束 e2e）依赖 53-01 + 53-02 都完成
- ⚠️ 上线前需要在 CI 或干净环境 build 一次完整镜像并跑 V1..V7（依赖 D-53-PRE-1 修复或绕开）

## Self-Check: PASSED

- `[ -f deploy/docker/managed-user/Dockerfile ]` ✅
- `[ -f .planning/phases/53-entrypoint/53-01-SUMMARY.md ]` ✅
- `[ -f .planning/phases/53-entrypoint/deferred-items.md ]` ✅
- 所有 T1..T5 改动在 commit `9833227` 中可见且与 PLAN 代码块一致 ✅
- `docker buildx build --check` 0 warning ✅
- Standalone verify 镜像 `sb-verify:53-01` build PASS + V2/V3/V4/V7 实测 PASS ✅
- V5/V6 静态校验：Dockerfile 全文无 sudoers 写入、useradd workspace 不带 sudo group ✅

---
*Phase: 53-entrypoint*
*Completed: 2026-05-16*
