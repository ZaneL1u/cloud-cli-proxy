---
phase: 53-entrypoint
plan: "02"
subsystem: infra
tags: [entrypoint, sing-box, nftables, fail-closed, dns-lock, runuser, capabilities]

requires:
  - phase: 53-entrypoint
    plan: "01"
    provides: managed-user 镜像 v4.0 基线 Dockerfile（sing-box 1.13.3 binary + file cap cap_net_admin+eip + singbox uid=9000 + nftables/libcap2-bin 装好）
provides:
  - managed-user 容器 v4.0 fail-closed 启动序列：sing-box 子进程 + tun0 waitFor + nft default-deny + DNS lock + config shred + 死亡 PID 1 fail-closed
  - 容器内 nft ruleset 文件 `/etc/cloud-claude/default-deny.nft`（output policy drop + 仅放行 lo/tun0/已建立连接 + 显式 drop extdns/link-local；input policy drop + 放行 22/6080/lo）
  - 5 个新 entrypoint 函数：start_singbox_or_die / lock_resolv_conf / apply_nft_or_die / remove_singbox_config / monitor_singbox_fail_closed
  - 用户态彻底无 sudo / NOPASSWD 路径（v3.x sudoers 写入与 sed 全部删除 + 兜底 rm）
affects: [53-03, 54, 55, 56]

tech-stack:
  added:
    - nft table `inet cloud_proxy_v4`（output + input 双链 default drop ruleset）
    - bash 函数级 sing-box 生命周期管理（kill -0 polling + kill -TERM 1 fail-closed）
  patterns:
    - 启动序列 fail-fast：任一步失败 → entrypoint 非 0 退出 → tini 关停容器
    - sing-box 进程降权：`runuser -u singbox` 而不是 root，binary 文件 cap 提供 NET_ADMIN
    - 配置文件擦除：sing-box load 后 `shred -u` config，文件系统层不再可读（D-V4-2）
    - DNS 强制 stub：`/etc/resolv.conf` → 127.0.0.1 + nft drop 外部 53/853（双保险，T5 chattr 失败容忍）
    - 死亡监控：父 shell 后台启动 sing-box，子 shell `kill -0` polling + `kill -TERM 1` 触发 tini 关停（不依赖 `wait`，规避 plan 原文 bug）

key-files:
  created:
    - deploy/docker/managed-user/default-deny.nft
    - .planning/phases/53-entrypoint/53-02-SUMMARY.md
  modified:
    - deploy/docker/managed-user/Dockerfile
    - deploy/docker/managed-user/entrypoint.sh

key-decisions:
  - "采用 kill -0 polling 替代 plan 原文的 wait \"$SING_BOX_PID\" —— wait 在子 shell 看不到父 shell 的 background process，立即返回 127，monitor 失效。这是 Rule 1 bug 修复，monitor 行为与 plan 语义完全一致（sing-box 死 → kill PID 1）"
  - "nft 校验拆成 V2 必跑 + V3..V7 deferred-to-Plan-53-03 两阶段：V2 仅做语法（nft -c -f），V3..V7 需要真实 sing-box config 才能跑，留给 53-03 e2e"
  - "T5 chattr +i 容错策略：失败仅 WARN，依赖 nft drop 外部 53/853 兜底；overlay2 兼容性留 53-03 spike 确认"
  - "彻底删除 ALL_PROXY/HTTP_PROXY/NO_PROXY 环境变量注入：v4.0 走 tun 透明代理，应用不需要也不应该感知 proxy"

patterns-established:
  - "v4.0 entrypoint fail-closed 启动序列模板：start_xxx_or_die / apply_xxx_or_die 函数名约定 + 函数内即时 cleanup（kill background process）"
  - "shred -u 优先 + rm -f 兜底：sing-box config 必须从 fs 移除，但 read-only mount 时 shred 失败要降级"
  - "kill PID 1 → tini 关停范式：用 tini 的 signal-forwarding 触发优雅 shutdown，避开 kill -KILL 1 的粗暴退出（带 2s 兜底）"

requirements-completed: [EP-01, EP-02, EP-03, EP-04, NET-01, NET-02, NET-03, NET-04]

duration: ~25 min
completed: 2026-05-16
---

# Phase 53 Plan 02: entrypoint 重写 — fail-closed 启动序列 + nft + DNS + config 保护 Summary

**entrypoint.sh 改造为 v4.0 fail-closed 启动序列：sing-box 通过 `runuser` 降权运行 → tun0 waitFor → nft default-deny → DNS 强制 stub → config shred → 死亡 PID 1 fail-closed → sshd foreground。新增 default-deny.nft ruleset 文件 + Dockerfile COPY 行。v3.x sudo / 旧 MODE=local sing-box 分支整体删除。**

## Performance

- **Duration:** ~25 min（含 V2 nft 容器内校验 + V1 shellcheck docker 镜像拉取）
- **Started:** 2026-05-16T02:26:00Z
- **Completed:** 2026-05-16T02:51:00Z
- **Tasks:** 5 (T1..T5)
- **Files modified:** 2（Dockerfile + entrypoint.sh）+ 1 created（default-deny.nft）+ 1 SUMMARY

## Accomplishments

- **T1 — `default-deny.nft` ruleset 文件**：`deploy/docker/managed-user/default-deny.nft` 新建（48 行）。`table inet cloud_proxy_v4` 双链：
  - **output**：policy drop + `oifname lo|tun0 accept` + 显式 drop `udp/53` 与 `tcp/{53,853}` (`ip daddr != 127.0.0.0/8`) + drop `169.254.0.0/16` + `ct state established,related accept` + counter 兜底 drop。
  - **input**：policy drop + `iifname lo accept` + `tcp dport 22 accept` (SSH) + `tcp dport 6080 accept` (KasmVNC) + `ct state established,related accept` + counter 兜底 drop。
  - 所有 accept/drop 都加 counter，便于 53-03 / 54 排障观察流量。

- **T2 — Dockerfile COPY**：在 `COPY entrypoint.sh` / `launch-chromium.sh` / `restart-vnc` 段之后追加：
  ```dockerfile
  COPY deploy/docker/managed-user/default-deny.nft /etc/cloud-claude/default-deny.nft
  RUN chmod 0644 /etc/cloud-claude/default-deny.nft
  ```
  仅 1 个新 COPY + 1 个 chmod，不改其他段。

- **T3 — entrypoint.sh 五个新函数 + 启动序列调用**：
  - 函数定义集中放在 `assert_tmux_version` 之后、SSH setup 之前（L155-280）：
    - `start_singbox_or_die`：config 文件存在性 + root:0600 权限校验 → `runuser -u singbox -- /usr/local/bin/sing-box run -c ...` 后台 → 30s 内 polling `ip link show tun0` + `kill -0 $SING_BOX_PID` 双 ready 判定 → 超时或 sing-box 死 → kill + exit 1。
    - `lock_resolv_conf`：`cat > /etc/resolv.conf` 写死 `nameserver 127.0.0.1` + `chattr +i` 兜底（失败仅 WARN）。
    - `apply_nft_or_die`：`nft -f $NFT_RULESET` + `nft list table inet cloud_proxy_v4` 二次 verify，失败任一步 kill sing-box + exit 1。
    - `remove_singbox_config`：`shred -u $SING_BOX_CONFIG` 优先，回退 `rm -f`；rm 失败仍残留则 kill sing-box + exit 1。
    - `monitor_singbox_fail_closed`：子 shell `while kill -0 $SING_BOX_PID; do sleep 1; done` polling，sing-box 死 → `kill -TERM 1` + 2s 兜底 `kill -KILL 1`。
  - 启动序列调用插入位置：`if [ -c /dev/fuse ]; then chmod 666 /dev/fuse; fi` 之后、`MODE="${MODE:-remote}"` 之前。所有 MODE（remote/local）都跑这套序列。

- **T4 — v3.x 残留删除**（grep 二次确认无残留）：
  1. ✅ `echo "${RUN_USER} ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/workspace` + `chmod 0440` — 整段删除，替换为 `rm -f /etc/sudoers.d/workspace` 兜底清理 + （如 RUN_USER ≠ workspace 时）额外 `rm -f /etc/sudoers.d/$RUN_USER`。
  2. ✅ 用户重命名块内 `if [ -f /etc/sudoers.d/workspace ]; then sed -i ...; fi` — 替换为注释说明。
  3. ✅ 旧 `MODE=local` sing-box 分支（原 L315-384，约 70 行）整段删除：
     - proxy fallback 逻辑（`SING_BOX_MODE` tun/proxy 二选一 + 自动 socks/http inbound 构造）
     - `ALL_PROXY` / `HTTP_PROXY` / `HTTPS_PROXY` / `NO_PROXY` 环境变量注入 + 写 `/etc/environment`
     - `command -v sing-box` + `sing-box run &` + `kill -0 $!` 探活
     替换为 8 行 v4.0 注释，说明出网由顶部 `start_singbox_or_die` 序列统一处理。

- **T5 — `chattr +i` overlay2 兜底**：
  - `lock_resolv_conf` 内 `chattr +i /etc/resolv.conf 2>/dev/null || echo "WARN: chattr +i 失败"` —— 不 FATAL，仅警告。
  - 双保险：nft default-deny output 链的 `extdns-udp-drop` / `extdns-tcp-drop` 规则即使用户 root 改回 `/etc/resolv.conf` 指向 `8.8.8.8`，外部 53/853 流量也会被 drop。

## Task Commits

按 Commit Plan 2 个 atomic commit：

| Commit | Hash | 范围 |
|---|---|---|
| 1. `feat(53-02): add default-deny.nft ruleset for in-container firewall` | `5c92c72` | T1（新增 default-deny.nft 48 行）+ T2（Dockerfile 1 个 COPY + 1 个 chmod） |
| 2. `feat(53-02): entrypoint v4.0 sing-box fail-closed startup sequence` | `f819824` | T3（5 函数 + 启动序列调用）+ T4（删除 v3.x sudoers + 旧 MODE=local 分支）+ T5（chattr +i 兜底） |

之后还会有 1 个 SUMMARY commit（本文件 + commit 自身）。

## Files Created/Modified

- `deploy/docker/managed-user/default-deny.nft` — 新建，48 行 nft ruleset。
- `deploy/docker/managed-user/Dockerfile` — 新增 2 行（COPY default-deny.nft + chmod 0644）。
- `deploy/docker/managed-user/entrypoint.sh` — 净 +142 -77。结构变动：
  - L155-280 新增 v4.0 函数定义段（5 函数 + 全局常量）。
  - L294 + L299-305 替换 v3.x sudoers 写入为 v4.0 删除路径 + 兜底 rm。
  - L340-346 新增 5 个启动序列调用。
  - L446-449 旧 MODE=local 分支整段删除，替换为 8 行 v4.0 注释。
- `.planning/phases/53-entrypoint/53-02-SUMMARY.md` — 本文件。

## Decisions Made

| ID | 内容 | 偏离 plan? |
|---|---|---|
| D-53-02-D1 | `monitor_singbox_fail_closed` 用 `kill -0` polling 替代 plan 原文 `wait "$SING_BOX_PID"` | ✅ 偏离（Rule 1 bug 修复，见下文 Deviations） |
| D-53-02-D2 | 启动序列调用顺序：`start → lock_resolv → apply_nft → remove_config → monitor`（与 plan 伪代码完全一致） | ❌ 不偏离 |
| D-53-02-D3 | 启动序列在 `MODE` 检测**之前**调用（所有 MODE 都跑） | ❌ 与 plan 一致 |
| D-53-02-D4 | 兜底 `rm -f /etc/sudoers.d/$RUN_USER` 加 `if RUN_USER ≠ workspace` 守卫（避免重复 rm 同一文件） | ⚠️ 微调，plan 原文是 `rm -f workspace /etc/sudoers.d/"${CONTAINER_USER:-workspace}"`，等价但更显式 |

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] `monitor_singbox_fail_closed` plan 原文 `wait` 不可用**

- **Found during:** T3 实现 monitor 函数时
- **Issue:** PLAN.md L211-220 给的实现：
  ```bash
  (
    wait "$SING_BOX_PID" 2>/dev/null
    rc=$?
    echo "FATAL: sing-box 退出 (exit=$rc)..."
    kill -TERM 1 2>/dev/null
    sleep 2
    kill -KILL 1 2>/dev/null
  ) &
  ```
  在 bash 中，`wait` 只能等当前 shell 的子进程。`$SING_BOX_PID` 是父 shell 启动的 background process，子 shell `(...)` 看不到它作为自己的子进程，`wait` 立即返回 127（"no such job"），整个 monitor 子 shell 在 sing-box 真正死之前就跑完了 echo+kill 序列 → 把容器误杀。
- **Fix:**
  ```bash
  (
    while kill -0 "$SING_BOX_PID" 2>/dev/null; do
      sleep 1
    done
    echo "[entrypoint] FATAL: sing-box 退出..."
    kill -TERM 1 2>/dev/null || true
    sleep 2
    kill -KILL 1 2>/dev/null || true
  ) &
  ```
  `kill -0` 不要求是子进程关系，只检查 pid 是否存在 + 当前用户是否有权 signal。监控行为与 plan 语义完全一致（sing-box 死 → ≤2s 容器死）。

  另外把 `kill -TERM 1` / `kill -KILL 1` 加 `|| true`（避免 set -e 在 signal 失败时误退出 monitor 子 shell）。
- **Files modified:** `deploy/docker/managed-user/entrypoint.sh` 函数 `monitor_singbox_fail_closed`
- **Commit:** `f819824`

**2. [Rule 2 - 关键防御] `apply_nft_or_die` 增加 ruleset 文件存在性预检**

- **Found during:** T3 实现 apply_nft 函数时
- **Issue:** PLAN.md 给的实现直接 `nft -f $NFT_RULESET`，但如果 Dockerfile COPY 漏了（运行时镜像构建 cache 异常），nft 报错信息会比较模糊。
- **Fix:** 在 `nft -f` 之前加 `[ ! -f "$NFT_RULESET" ]` 检查并 FATAL，错误信息明确指向 ruleset 路径。
- **Files modified:** 同上 函数 `apply_nft_or_die`
- **Commit:** `f819824`
- **Rationale:** Rule 2 — entrypoint fail-fast 路径上必须有明确诊断信息，符合 EP-01 "任一步失败 entrypoint 非 0 退出" 的可观测性要求。

**3. [Rule 1 - Bug] `start_singbox_or_die` 局部变量 perm/owner 缺 `local` 声明**

- **Found during:** T3 实现时 plan 原文这两行没标 `local`，会污染全局命名空间。
- **Fix:** `local perm owner` + 单独赋值。
- **Files modified:** 同上 函数 `start_singbox_or_die`
- **Commit:** `f819824`

### 计划外但符合 plan 语义的微调

- **`kill -TERM 1` / `kill "$SING_BOX_PID"` 都加 `|| true`**：plan 原文部分加部分没加，统一加上。`set -euo pipefail` 下 kill 失败会让 entrypoint 在错误路径中再误退出，加 `|| true` 保证 cleanup 一定完成。
- **`SING_BOX_PID` 全局初始化为空字符串 + cleanup 前 `[ -n "$SING_BOX_PID" ]` 守卫**：plan 原文假定 SING_BOX_PID 一定被赋值，但 `start_singbox_or_die` 在赋值前可能因 config 文件缺失提前 exit 1。加守卫避免 cleanup 在未赋值时 `kill ""` 报错。

## Verification Results

| ID | 项 | 期望 | 实际 | 状态 |
|---|---|---|---|---|
| V1 | `shellcheck entrypoint.sh` | 0 error | 0 error + 0 warning（`koalaman/shellcheck:stable` docker 镜像，默认 severity） | ✅ |
| V1' | `bash -n entrypoint.sh` | 0 退出码 | `BASH-SYNTAX-OK` | ✅ |
| V2 | `nft -c -f default-deny.nft` | 0 退出码 | `NFT-SYNTAX-OK`（ubuntu:24.04 容器 + `--cap-add=NET_ADMIN` + `--network=host`） | ✅ |
| V3 | 镜像构建 + 容器跑通 + 日志含 `tun0 ready / nft applied / config removed` | sing-box config fixture 准备就绪后跑通 | **Deferred to Plan 53-03** | ⏸ |
| V4 | `docker exec ps -o uid,user,comm -p $(pidof sing-box)` = `9000 singbox sing-box` | EP-02 验证 | **Deferred to Plan 53-03** | ⏸ |
| V5 | `docker exec cat /etc/sing-box/config.json` = `No such file or directory` | D-V4-2 验证 | **Deferred to Plan 53-03** | ⏸ |
| V6 | `docker exec -u workspace getpcaps $$` = 空 effective cap | SEC-03 / NET-04 验证 | **Deferred to Plan 53-03** | ⏸ |
| V7 | sing-box 死 → 容器死 ≤ 3s | EP-04 / D-V4-4 验证 | **Deferred to Plan 53-03** | ⏸ |

**说明：**

1. **V1** 通过 `docker run --rm -v $(pwd):/work koalaman/shellcheck:stable /work/.../entrypoint.sh` 跑，0 error 0 warning。
2. **V2** macOS 本机无 nft，用 `docker run --rm --cap-add=NET_ADMIN --network=host -v ... ubuntu:24.04 sh -c 'apt-get install -y nftables && nft -c -f /tmp/default-deny.nft'` 校验，`NFT-SYNTAX-OK`。注：`nft -c` 即便是 syntax-only 也会做 cache initialization，必须给容器 NET_ADMIN cap 才不会 `Operation not permitted`。
3. **V3..V7 全部 Deferred-to-Plan-53-03**：原因——
   - 需要真实 sing-box config fixture（Plan 53-02 scope 不包括 config 注入）。
   - 需要 docker 完整镜像构建成功（Plan 53-01 V1 deferred 因 Claude Code 安装段 pre-existing 远端 404/403，尚未解锁）。
   - 这些 V 项本质都是运行时 UAT，Plan 53-03 标题 "容器启动验证 + 出口 IP 强约束 e2e" 显式覆盖。
4. **静态语义校验补充**（运行时之外的同等强度证据）：
   - `grep -n sudoers entrypoint.sh` — 仅出现在注释 + 清理 `rm` 行，无任何 `echo ... > /etc/sudoers.d/`。
   - `grep -n NOPASSWD entrypoint.sh` — 0 hit（v3.x 残留全部清除）。
   - `grep -n ALL_PROXY\|HTTP_PROXY entrypoint.sh` — 仅出现在注释，无 export / 写 /etc/environment。
   - `grep -n MODE=local entrypoint.sh` — 仅出现在注释，无旧分支逻辑。

## Deferred Issues

- **V3..V7 端到端验证 Deferred-to-Plan-53-03**：见上表。Plan 53-03 需要：
  1. 准备 sing-box config fixture（基于 v3.6 gateway 配置裁剪，挂载或 `--mount type=tmpfs` 注入）。
  2. 解锁 Plan 53-01 V1 镜像完整构建（要么修复 Claude Code 安装段 pre-existing 远端 404/403，要么走 CI 干净环境）。
  3. 跑 V3 (启动日志) / V4 (sing-box uid=9000) / V5 (config 已 rm) / V6 (workspace 空 cap) / V7 (sing-box 死容器死 ≤3s)。
- **chattr +i overlay2 兼容性 spike**：T5 已加运行时容错（WARN + nft 兜底），但 53-CONTEXT.md Open Question #4 仍未回答 "overlay2 上 chattr 是否 always 失败"。Plan 53-03 顺手验证一次，结果回填 CONTEXT。
- **`/etc/cloud-claude/sing-box-outbound.json` 文件路径残留**：旧 MODE=local 分支引用了这个文件，但没有任何上游 Plan 显式说明它在哪里被写入。删除分支后 v4.0 不再依赖此文件，但 53-CONTEXT.md / Phase 54 spec 应明确 sing-box config 注入路径（应是 `/etc/sing-box/config.json` 由控制面注入）。归类为 Phase 54 的 spec 收口项，不在 53-02 修复责任范围。

## Issues Encountered

- **Plan 原文 `wait "$SING_BOX_PID"` bug** — 见 Deviation #1。如果原样实现，monitor 子 shell 会立即误触发容器死。这是个 silent failure，仅在端到端测试时才会暴露。Rule 1 修复落地 commit `f819824`。

## Threat Flags

无新增 threat surface。本 Plan 的安全性变化全部是**收紧**攻击面：

- **彻底无 sudo 路径**：v3.x sudoers NOPASSWD 写入与 sed 改名逻辑全部删除（T4），用户态再无 `sudo` 提权可能。
- **DNS 强制 stub + nft drop**：用户即使 root 改 `/etc/resolv.conf` 或本地起 DNS proxy，外部 53/853 也被 nft drop（T1 + T5 双保险）。
- **sing-box config 不持久化**：`shred -u` 之后镜像/容器 fs 都不再有 config（T3 `remove_singbox_config`），用户进容器拿 root 也读不到出口凭证。
- **死亡 fail-closed**：sing-box 进程被 kill / OOM / 自己 panic，monitor 在 ≤2s 内把容器整死，杜绝"sing-box 死了但容器还在跑、用户走默认路由直连"的窗口。

## User Setup Required

无。本 Plan 仅修改镜像内 entrypoint.sh / Dockerfile / nft ruleset 文件，不需要 host 端配置。运行时 V3..V7 留给 Plan 53-03。

## Next Phase Readiness

- ✅ Plan 53-03（容器启动验证 + 出口 IP 强约束 e2e）有了 entrypoint 完整 v4.0 启动序列作为输入，可以编写 sing-box config fixture 并跑 V3..V7。
- ✅ Phase 54（控制面集成）的 sing-box config 注入接口（`/etc/sing-box/config.json` root:0600）已经在 entrypoint 中显式校验，控制面只需保证写入时权限正确。
- ⚠️ 上线前依旧需要在 CI 或干净环境跑一次完整 docker build（依赖 Plan 53-01 V1 deferred 项解锁）+ 53-03 V3..V7 全套 UAT。

## TDD Gate Compliance

Plan 53-02 frontmatter 无 `type: tdd`，单 task 也没标 `tdd="true"`，不适用 RED/GREEN/REFACTOR 强制门。所有 V1/V2 都是静态语义校验，V3..V7 运行时 UAT 留给 Plan 53-03。

## Self-Check: PASSED

- `[ -f deploy/docker/managed-user/default-deny.nft ]` ✅
- `[ -f deploy/docker/managed-user/entrypoint.sh ]` ✅（净改动 +142 -77）
- `[ -f deploy/docker/managed-user/Dockerfile ]` ✅（COPY default-deny.nft 已加）
- `[ -f .planning/phases/53-entrypoint/53-02-SUMMARY.md ]` ✅
- `git log --oneline | grep -E "5c92c72|f819824"` 输出 2 行 ✅（Commit 1 + Commit 2 都存在）
- `shellcheck entrypoint.sh` 0 error 0 warning ✅
- `bash -n entrypoint.sh` 0 退出 ✅
- `nft -c -f default-deny.nft`（docker + NET_ADMIN）0 退出 ✅
- `grep -n NOPASSWD entrypoint.sh` 0 hit（v3.x sudoers 残留清除）✅
- `grep -n 'MODE=local' entrypoint.sh` 仅注释（旧 sing-box 分支删除）✅
- 5 个函数定义齐全：start_singbox_or_die / lock_resolv_conf / apply_nft_or_die / remove_singbox_config / monitor_singbox_fail_closed ✅
- 启动序列调用顺序与 plan 一致 ✅

---
*Phase: 53-entrypoint*
*Completed: 2026-05-16*
