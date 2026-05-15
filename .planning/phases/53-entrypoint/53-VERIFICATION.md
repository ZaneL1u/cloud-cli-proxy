---
phase: 53-entrypoint
verified: 2026-05-16T02:55:00Z
status: gaps_found
score: 3/5 must-haves verified (2/5 deferred-to-CI; 1 PARTIAL with real gap)
overrides_applied: 0
gaps:
  - truth: "SC4 — 容器内 user shell `getpcaps $$` 输出空 cap 且 `ip link set tun0 down` 返回 EPERM"
    status: partial
    reason: "smoke.sh T-53-4 仅断言空 cap + sudo 拒绝；缺 SC4 显式要求的 `ip link set tun0 down` EPERM 断言。空 cap 本身在 Dockerfile L114-117 + workspace 不在 sudo group 已实现，但 SC 第二条断言无 oracle 覆盖。"
    artifacts:
      - path: "tests/phase53/smoke.sh"
        issue: "T-53-4 (L76-86) 缺一条 `docker exec -u workspace ip link set tun0 down 2>&1 | grep 'Operation not permitted'` 断言"
    missing:
      - "smoke.sh T-53-4 追加 `ip link set tun0 down` EPERM 断言（覆盖 SC4 第二条 oracle）"
  - truth: "IMG-03 — 镜像内置 nftables 与基础排障工具（`nft` / `iproute2` / `dig` / `getpcaps`）"
    status: partial
    reason: "Dockerfile L25-57 装了 `nftables` / `iproute2` / `libcap2-bin`（提供 `setcap`/`getcap`/`getpcaps`），但缺 `dnsutils` / `bind9-host`（提供 `dig`）。IMG-03 REQ 显式列 `dig`。NET-02 的 `dig @8.8.8.8 example.com` 验证用例无法在镜像内执行。nft drop 规则本身在 default-deny.nft L25-26 已实现，dig 仅是用户/运维角度的验证工具。"
    artifacts:
      - path: "deploy/docker/managed-user/Dockerfile"
        issue: "L25-57 apt install 列表无 `dnsutils`（或 `bind9-host`），grep 全文 0 hit"
    missing:
      - "Dockerfile apt install 段追加 `dnsutils`（或更精简的 `bind9-host`），使 `dig` 可用以满足 IMG-03 与 NET-02 的运行时 oracle 要求"
deferred:
  - truth: "SC1 — `docker run` 后容器内 `curl https://ip.me` 返回**绑定的出口 IP**（不是宿主真实 IP）"
    addressed_in: "Phase 55"
    evidence: "Phase 55 Success criterion 4: 'v3.6 MVS-01..10 + GoldenPath 用例迁移到单容器，所有出口 IP / DNS / default-deny / 错误码契约保持不变。' MVS-02 即 Phase 46 的出口 IP 三源轮询用例，迁移到 v4.0 单容器架构后会用真上游代理 outbound fixture 验证『出口 IP = 绑定的出口 IP』完整语义。本 phase fixture 用 `direct` outbound，出口 IP = 宿主 eth0 IP，仅能验证『路由经 tun0』而非『出口 IP = 绑定 IP』。"
human_verification:
  - test: "完整 `docker build -t managed-user:v4-dev -f deploy/docker/managed-user/Dockerfile .` 在 CI（ubuntu-latest runner）跑通"
    expected: "镜像构建 0 error，含 sing-box 1.13.3 + singbox uid=9000 + setcap + nftables 全部就位"
    why_human: "本地 build 被 D-53-PRE-1 阻塞（Claude Code 安装段 `claude.ai/install.sh` HTTP 403 + GitHub release `claude-code-linux-aarch64.tar.gz` HTTP 404）。CI 干净环境或修复远端资源后才能解锁。"
  - test: "`bash tests/phase53/smoke.sh` 在 ubuntu-latest CI runner 跑通 T-53-1..6"
    expected: "6 条断言全绿（T-53-1 tun0+uid=9000 / T-53-2 config rm / T-53-3 默认路由 dev=tun0 + curl 通 / T-53-4 workspace 空 cap + 无 sudo / T-53-5 ≤3s 容器死 / T-53-6 RestartCount≥1）"
    why_human: "本地 macOS docker desktop 上：(1) v4-dev 镜像不存在（D-53-PRE-1）；(2) `--device /dev/net/tun` + `--cap-add NET_ADMIN` 在 docker desktop VM 中时序与 linux 原生 daemon 有偏差，T-53-5 ≤3s 与 T-53-6 `--restart=on-failure` 行为不可靠。Phase 55 CI 在 ubuntu-latest 真机跑才是最终交付 oracle。"
  - test: "Plan 53-01 V5/V6 运行时复验：`docker run --rm managed-user:v4-dev sudo -n true` 必须非 0 退出 + `groups workspace` 输出不含 sudo / wheel / root"
    expected: "sudo 命令被拒（找不到 binary 或 user 不在 sudoers）；groups 仅返回 `workspace : workspace`"
    why_human: "53-01 SUMMARY V5/V6 仅做 Dockerfile 文本静态校验（grep 0 sudoers 写入 + useradd 行无 -G sudo）；运行时复验依赖完整镜像 build 解锁。"
  - test: "Plan 53-02 V3 运行时端到端：容器启动序列日志含 `tun0 ready` / `nft applied` / `config removed` 三条 marker"
    expected: "docker logs 时序：start_singbox_or_die → lock_resolv_conf → apply_nft_or_die → remove_singbox_config → monitor armed"
    why_human: "需要镜像 + 真 sing-box config fixture 同时就位才能跑；deferred-to-Phase-55-CI。"
  - test: "Plan 53-02 V4 运行时：`docker exec ... ps -o uid,user,comm -p $(pidof sing-box)` 输出 `9000 singbox sing-box`"
    expected: "uid=9000，user=singbox，comm=sing-box"
    why_human: "EP-02 / SC2 的运行时 oracle，依赖完整镜像 + sing-box 跑起来；deferred-to-Phase-55-CI。"
  - test: "Plan 53-02 V5 运行时：`docker exec ... cat /etc/sing-box/config.json` 输出 `No such file or directory`"
    expected: "shred -u 或 rm -f 之后 fs 不可见"
    why_human: "D-V4-2 / SC3 的运行时 oracle；deferred-to-Phase-55-CI。"
  - test: "Plan 53-02 V7 运行时：`kill -9 $(pidof sing-box)` → 容器在 ≤3s 内退出"
    expected: "monitor_singbox_fail_closed 子 shell `kill -0` polling 检测到 sing-box 死 → `kill -TERM 1` → tini 关停容器，端到端 ≤3s"
    why_human: "EP-04 / NET-03 / SC5 的运行时 oracle；deferred-to-Phase-55-CI。"
  - test: "Plan 53-03 V3 端到端：`bash tests/phase53/smoke.sh` 单独跑（同上『smoke.sh 在 CI 跑通』，列出仅为与 53-03 SUMMARY 对齐）"
    expected: "等同上面 smoke.sh 项"
    why_human: "Plan 53-03 V3 deferred-to-Phase-55-CI 的明确转写，避免遗漏。"
  - test: "`chattr +i /etc/resolv.conf` 在 overlay2 storage driver 上的真实行为（53-CONTEXT.md Open Question #4 兜底验证）"
    expected: "若失败仅 WARN 不 FATAL；nft drop 外部 53/853 兜底起效"
    why_human: "lock_resolv_conf (entrypoint L211-224) 已写容错 (`|| echo 'WARN'`)，但 overlay2 兼容性结论需在 ubuntu CI 真跑一次回填到 53-CONTEXT.md。"
---

# Phase 53: 镜像与 entrypoint 基线 Verification Report

**Phase Goal:** managed-user 镜像内置 sing-box + entrypoint 串接启动序列，本地手工 `docker run` 起来后 `curl ip.me` 走绑定的出口 IP；用户 SSH 进来非 root、无 NET_ADMIN、读不到 sing-box config。

**Verified:** 2026-05-16T02:55:00Z
**Status:** gaps_found
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

来自 ROADMAP Phase 53 § Success criteria 5 条：

| #   | Truth | 状态 | 证据 |
| --- | ----- | ---- | ---- |
| 1   | `docker run` 后容器内 `curl https://ip.me` 返回**绑定的出口 IP**（不是宿主真实 IP） | ⚠️ DEFERRED | 代码 + fixture 仅验证『默认路由 dev=tun0 + curl 通』；fixture 用 `direct` outbound（出口 IP = 宿主 eth0 IP），完整『出口 IP = 绑定 IP』语义 deferred 到 Phase 55 真上游 outbound fixture（见 deferred 节）。运行时执行又额外 deferred-to-CI（D-53-PRE-1）。 |
| 2   | 容器内 `ps -o uid,user,comm -p $(pidof sing-box)` 显示 uid=9000 | ✓ VERIFIED (静态) / ⏸ DEFERRED-CI (运行时) | entrypoint L186 `runuser -u singbox -- /usr/local/bin/sing-box run -c "$SING_BOX_CONFIG" &`；Dockerfile L119-121 `useradd --uid 9000 --gid 9000 ... singbox`；setcap L188-190 文件 cap 让非 root 可拿 NET_ADMIN。运行时 `docker exec ps` deferred-to-Phase-55-CI（smoke T-53-1 覆盖）。 |
| 3   | 容器内 `cat /etc/sing-box/config.json` 失败 | ✓ VERIFIED (静态) / ⏸ DEFERRED-CI (运行时) | entrypoint L247-256 `remove_singbox_config` 调用 `shred -u $SING_BOX_CONFIG \|\| rm -f`，rm 失败仍残留则 kill sing-box + exit 1；启动序列 L345 已调用。运行时验证 deferred-to-Phase-55-CI（smoke T-53-2 覆盖）。 |
| 4   | 容器内 user shell `getpcaps $$` 输出空 cap 集合 + `ip link set tun0 down` 返回 EPERM | ✗ PARTIAL | 空 cap 部分：Dockerfile L114-117 删 sudo + workspace 不在 sudo group ✓ 实现；smoke T-53-4 (L76-86) 验空 cap + sudo 拒绝 ✓。**`ip link set tun0 down` EPERM 断言缺失** — smoke.sh 未覆盖 SC4 第二条 oracle（详见 gaps 节 #1）。运行时部分 deferred-to-Phase-55-CI。 |
| 5   | `docker exec kill -9 $(pidof sing-box)` 触发后容器 ≤3s 退出，docker `restart=on-failure` ≤5s 拉起新容器 | ✓ VERIFIED (静态) / ⏸ DEFERRED-CI (运行时) | entrypoint L258-274 `monitor_singbox_fail_closed` 子 shell `while kill -0 "$SING_BOX_PID"; do sleep 1; done` polling，sing-box 死 → `kill -TERM 1` + 2s 兜底 `kill -KILL 1`（已修复 plan 原文 `wait` bug，见 53-02-SUMMARY Deviation #1）；smoke T-53-5 (L88-106) ≤3s 断言 + T-53-6 (L108-123) RestartCount≥1 + restart 后 sing-box uid=9000。运行时 deferred-to-Phase-55-CI。 |

**Score:** 3/5 truths VERIFIED in static layer (T2/T3/T5 实现完整且 wired，仅运行时 oracle 待 CI 跑)；T1 PARTIAL — fixture direct outbound deferred-to-Phase-55；T4 PARTIAL — smoke 缺一条断言。

### Deferred Items

仅 T1 一项明确 deferred 到后续 milestone phase。其余 deferred 项均为运行时 oracle 待 CI 跑（归 human_verification 节，不算 deferred）。

| # | Item | Addressed In | Evidence |
|---|------|-------------|----------|
| 1 | T1 完整语义『curl 返回**绑定的出口 IP**』 | Phase 55 | Phase 55 SC4 显式：『v3.6 MVS-01..10 + GoldenPath 用例迁移到单容器，所有**出口 IP** / DNS / default-deny / 错误码契约保持不变』。MVS-02 出口 IP 三源轮询会用真上游代理 outbound 在 v4.0 单容器架构下重新跑。 |

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `deploy/docker/managed-user/Dockerfile` | sing-box install + setcap + singbox uid=9000 + 删 sudo + nftables/libcap2-bin | ✓ VERIFIED | L25-57 apt install nftables/libcap2-bin；L114-117 useradd workspace 无 -G sudo；L119-121 useradd singbox uid=9000 nologin；L172-182 sing-box 1.13.3 GitHub release install；L188-190 setcap cap_net_admin+eip + getcap verify。standalone build verify (sb-verify:53-01) PASS。⚠️ 缺 `dnsutils` (dig) — 见 gaps #2 |
| `deploy/docker/managed-user/entrypoint.sh` | 5 函数（start_singbox_or_die / lock_resolv_conf / apply_nft_or_die / remove_singbox_config / monitor_singbox_fail_closed） + 启动序列调用 | ✓ VERIFIED | L155-274 五函数完整定义；L342-346 启动序列固定顺序调用；shellcheck 0 error；bash -n PASS；grep NOPASSWD 0 hit；grep MODE=local 仅注释；ALL_PROXY/HTTP_PROXY 仅注释 |
| `deploy/docker/managed-user/default-deny.nft` | output policy drop + lo/tun0 accept + extdns/link-local drop + ct established accept；input policy drop + lo/22/6080 accept + ct accept | ✓ VERIFIED | 56 行 ruleset；output L13-36 + input L38-54 双链 + counter；nft -c -f 语法校验 PASS（53-02 V2 in ubuntu:24.04 + NET_ADMIN container） |
| `tests/phase53/smoke.sh` | T-53-1..6 6 条断言 | ⚠️ PARTIAL | 127 行；6 条断言齐全；shellcheck 0 issue；bash -n PASS。⚠️ T-53-4 缺 `ip link set tun0 down` EPERM 断言 — 见 gaps #1 |
| `tests/phase53/fixtures/test-singbox-config.json` | 最小可用 sing-box 1.13.x config | ✓ VERIFIED | 36 行；direct outbound（仅证 tun 接管，出口 IP 强约束 deferred-to-Phase-55）；`sing-box check` (ghcr.io/sagernet/sing-box:v1.13.3) PASS |
| `tests/phase53/README.md` | 使用说明 + plan 验收映射 | ✓ VERIFIED | 55 行；含 build + smoke 流程 + 与 53-01/02 验收映射表 + fixture direct outbound 语义说明 + 平台限制 |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | -- | --- | ------ | ------- |
| Dockerfile L172-182 (sing-box install) | `/usr/local/bin/sing-box` binary 进容器 fs | `install -m 0755 ... sing-box` | ✓ WIRED | install 命令 + 紧跟 `sing-box version` build-time verify |
| Dockerfile L188 (setcap) | sing-box binary 持 cap_net_admin+eip 文件 cap | `setcap` + 紧跟 `getcap \| grep cap_net_admin \|\| exit 1` build-time verify | ✓ WIRED | failure → build fail (`exit 1`) |
| entrypoint L186 (runuser) | sing-box 跑在 uid=9000 + binary file cap 提供 NET_ADMIN | `runuser -u singbox -- /usr/local/bin/sing-box run -c "$SING_BOX_CONFIG" &` | ✓ WIRED | runuser 来自 util-linux（Ubuntu 24.04 默认装）；singbox uid=9000 来自 Dockerfile L119-121；NET_ADMIN 来自 setcap 文件 cap |
| entrypoint L191-202 (waitFor tun0) | sing-box ready 判定 = tun0 接口存在 + sing-box pid 还活 | `while < deadline; do ip link show tun0 + kill -0 $SING_BOX_PID; sleep 0.5` | ✓ WIRED | 30s timeout，超时 fail-closed `kill $SING_BOX_PID + exit 1` |
| entrypoint L226-245 (apply_nft_or_die) | nft default-deny ruleset 应用到容器 netns | `nft -f $NFT_RULESET` + `nft list table inet cloud_proxy_v4` 二次 verify | ✓ WIRED | NFT_RULESET=`/etc/cloud-claude/default-deny.nft`，Dockerfile L220-221 COPY 进镜像 |
| entrypoint L211-224 (lock_resolv_conf) | DNS 强制走 sing-box stub | `cat > /etc/resolv.conf` + `chattr +i 2>/dev/null \|\| WARN` | ✓ WIRED | overlay2 chattr 失败容错（兜底走 nft drop 53/853） |
| entrypoint L247-256 (remove_singbox_config) | sing-box load 后 fs 删 config | `shred -u $SING_BOX_CONFIG 2>/dev/null \|\| rm -f`，rm 失败 → kill sing-box + exit 1 | ✓ WIRED | D-V4-2 凭据保护 |
| entrypoint L258-274 (monitor) | sing-box 死 → kill PID 1 → tini 关停容器 | `while kill -0 $SING_BOX_PID; do sleep 1; done; kill -TERM 1; sleep 2; kill -KILL 1` | ✓ WIRED | 子 shell 已修复原 plan `wait` bug（53-02 Deviation #1）；2s 兜底 KILL；tini 是 PID 1（Dockerfile L231 ENTRYPOINT）|
| Dockerfile L220-221 (COPY default-deny.nft) | nft ruleset 文件进容器 fs | `COPY ... default-deny.nft /etc/cloud-claude/default-deny.nft` + `chmod 0644` | ✓ WIRED | apply_nft_or_die L228-232 文件存在性 pre-check 兜底 |
| smoke.sh L37-45 (docker run) | T-53-1..6 oracle 链路 | `--device /dev/net/tun --cap-drop ALL --cap-add NET_ADMIN -v $TMP_CONFIG:/etc/sing-box/config.json --restart=on-failure:3` | ✓ WIRED | 与 ROADMAP SC1 命令行一致 |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| -------- | ------------- | ------ | ------------------ | ------ |
| `default-deny.nft` ruleset | `chain output policy drop` 后续放行/drop 规则 | 静态 ruleset 文件，由 entrypoint nft -f 加载 | 是 — nft 内核态规则集 | ✓ FLOWING |
| `entrypoint.sh` $SING_BOX_PID | sing-box 进程 pid | `runuser -u singbox -- ... &` 后立即 `SING_BOX_PID=$!` | 是 — 真实进程 pid | ✓ FLOWING |
| smoke.sh fixture config | sing-box 启动配置 | `cp $FIXTURE_DIR/test-singbox-config.json $TMP_CONFIG; chmod 600` | 是 — 真实 sing-box config（direct outbound） | ⚠️ STATIC（fixture 仅 direct，无真上游 outbound 凭据 — Phase 55 补） |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| entrypoint.sh 语法合法 | `bash -n deploy/docker/managed-user/entrypoint.sh` | exit 0 (BASH-SYNTAX-OK) | ✓ PASS |
| smoke.sh 语法合法 | `bash -n tests/phase53/smoke.sh` | exit 0 | ✓ PASS |
| smoke.sh shellcheck | `docker run --rm koalaman/shellcheck:stable .../smoke.sh` | 0 issue（53-03 V2 实测） | ✓ PASS |
| entrypoint shellcheck | `docker run --rm koalaman/shellcheck:stable .../entrypoint.sh` | 0 error 0 warning（53-02 V1 实测） | ✓ PASS |
| nft ruleset 语法 | `nft -c -f default-deny.nft`（ubuntu + NET_ADMIN container） | 0 退出（53-02 V2 实测） | ✓ PASS |
| fixture 合法性 | `sing-box check -c test-singbox-config.json`（ghcr.io/sagernet/sing-box:v1.13.3） | Configuration OK（53-03 V1 实测） | ✓ PASS |
| 镜像完整 build | `docker build -t managed-user:v4-dev ...` | exit 22 在 Claude Code 安装段（pre-existing D-53-PRE-1） | ✗ SKIP（deferred-to-CI） |
| `docker run` 启动 + smoke T-53-1..6 | `bash tests/phase53/smoke.sh` | 镜像不可用 → 无法跑 | ✗ SKIP（deferred-to-CI） |

### Probe Execution

本 phase 不含 `scripts/*/tests/probe-*.sh` 类型 probe；smoke.sh 在 Plan 53-03 SUMMARY 已声明 deferred-to-Phase-55-CI（依赖 D-53-PRE-1 解锁）。

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| ----------- | ----------- | ----------- | ------ | -------- |
| IMG-01 | 53-01 | `/usr/local/bin/sing-box` 内置且可执行 | ✓ SATISFIED | Dockerfile L172-182 GitHub release install + L188 setcap；standalone build verify `sing-box version 1.13.3` PASS |
| IMG-02 | 53-01 | singbox 系统账号 uid=9000 nologin | ✓ SATISFIED | Dockerfile L119-121 `groupadd --gid 9000 singbox` + `useradd --uid 9000 --gid 9000 --home-dir /nonexistent --no-create-home --shell /usr/sbin/nologin singbox` |
| IMG-03 | 53-01 | nft / iproute2 / dig / getpcaps 全部就位 | ✗ BLOCKED | Dockerfile L25-57 装了 nftables / iproute2 / libcap2-bin（提供 setcap/getcap/getpcaps）；**缺 dnsutils（提供 dig）**。grep 全文 0 hit。NET-02 `dig @8.8.8.8` 无可执行 binary。 |
| IMG-04 | 53-01 | 用户 shell 非 root、无 sudo / wheel、无 cap_net_admin | ✓ SATISFIED | Dockerfile L114-117 useradd workspace 无 -G sudo / wheel；L113 注释明确删除 sudoers.d/workspace 写入；workspace uid=1000 binary 上无 file cap → getpcaps 默认空（运行时 oracle deferred-to-CI） |
| EP-01 | 53-02 | entrypoint 串行 fail-fast | ✓ SATISFIED | entrypoint `set -euo pipefail` (L2) + L342-346 五函数固定顺序调用 + 每函数失败 exit 1 |
| EP-02 | 53-02 | sing-box uid=9000 | ✓ SATISFIED | entrypoint L186 `runuser -u singbox -- /usr/local/bin/sing-box run` + Dockerfile setcap 提供 NET_ADMIN（运行时 oracle deferred-to-CI） |
| EP-03 | 53-02 | tun0 waitFor 替代裸 sleep | ✓ SATISFIED | entrypoint L191-202 `SECONDS deadline` + `ip link show tun0` + `kill -0 $SING_BOX_PID` 双 ready 判定 + 30s timeout |
| EP-04 | 53-02 | sing-box 死 → 容器死 | ✓ SATISFIED | entrypoint L258-274 monitor 子 shell `kill -0` polling + `kill -TERM 1` + 2s `kill -KILL 1` 兜底（运行时 ≤3s oracle deferred-to-CI） |
| NET-01 | 53-02 | nft default-deny applied | ✓ SATISFIED | default-deny.nft 56 行 ruleset + entrypoint L226-245 apply_nft_or_die `nft -f` + 二次 verify |
| NET-02 | 53-02 | DNS 强制 stub + 外部 53/853 drop（DoH 443 不在 nft 拦截，由 sing-box DNS over tun 接管 — D-V4-3） | ⚠️ PARTIAL | resolv.conf lock + nft drop 53/853 实现 ✓（default-deny.nft L25-26）；DoH 443 决策与 53-CONTEXT D-V4-3 一致（不拦截）。⚠️ 验证 oracle `dig @8.8.8.8 example.com` 因 IMG-03 缺 dig 工具不可执行 — 依赖 IMG-03 修复或用 nft counter 替代 oracle |
| NET-03 | 53-02 | sing-box 死 ≤3s + restart ≤5s | ✓ SATISFIED | monitor (L258-274) + smoke T-53-5 (L88-106) ≤3s 断言 + T-53-6 (L108-123) RestartCount≥1 / Running=true / 新 sing-box uid=9000（运行时 oracle deferred-to-CI） |
| NET-04 | 53-02 | user 进程空 cap + ip link set 拒绝 | ⚠️ PARTIAL | smoke T-53-4 (L76-86) 验空 cap + sudo 拒绝 ✓；**ip link set tun0 down EPERM 断言缺失** — smoke 未覆盖 SC4 第二条 oracle（gap #1） |

**12/12 REQ 跨 plan 全部声明：**
- 53-01: IMG-01..04 ✓
- 53-02: EP-01..04, NET-01..04 ✓
- 53-03: 无 REQ 直接声明（test infra plan，frontmatter requirements 为空 — 与 PLAN frontmatter 一致）

无 ORPHANED requirements（REQUIREMENTS.md 53 phase 12 REQ 全部出现在 53-01 + 53-02 PLAN frontmatter 中）。

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| `entrypoint.sh` | L300 | 注释 `# v4.0 (D-53-4): 删除 v3.x 的 sudoers NOPASSWD 写入` 解释意图 | ℹ️ Info | 解释非显然意图，符合 conventions（注释可保留）|
| `entrypoint.sh` | L221 | `chattr +i ... 2>/dev/null` 失败仅 WARN | ⚠️ Warning（已知容忍）| overlay2 兼容性已知，nft 兜底；53-CONTEXT Open Question #4 标记后续验证 |
| `Dockerfile` | L25-57 | apt install 无 `dnsutils` | ⚠️ Warning（gap #2）| IMG-03 REQ 显式列 dig；NET-02 dig oracle 不可执行 |
| `tests/phase53/smoke.sh` | L76-86 (T-53-4) | 缺 `ip link set tun0 down` EPERM 断言 | ⚠️ Warning（gap #1）| SC4 第二条 oracle 无覆盖 |
| `tests/phase53/fixtures/test-singbox-config.json` | L23-28 | direct outbound 而非真上游代理 | ℹ️ Info（设计决策）| 53-CONTEXT 已说明 fixture 仅证 tun 接管，真出口 IP 强约束 Phase 55 真上游 outbound fixture |

无 TBD / FIXME / XXX 调试遗留；无 console.log 类 stub；无 placeholder 字面量；无空 return null/[]/{} 代替实现。

### Human Verification Required

详见 frontmatter `human_verification` 节。9 项均为运行时 oracle，全部 deferred-to-CI（ubuntu-latest runner 跑完整 docker build + smoke.sh）：

1. 完整 `docker build -t managed-user:v4-dev` 在 CI 跑通（前置：D-53-PRE-1 修复或 CI 干净环境跳过远端 404/403）
2. `bash tests/phase53/smoke.sh` 在 CI 跑 T-53-1..6 全绿
3. 53-01 V5/V6 运行时复验（sudo + groups workspace）
4. 53-02 V3 运行时启动序列日志 marker 验证
5. 53-02 V4 sing-box uid=9000 运行时 ps 验证
6. 53-02 V5 cat config 失败运行时验证
7. 53-02 V7 kill 死 ≤3s 运行时时序验证
8. 53-03 V3 smoke 端到端（与 #2 等价，列出与 53-03 SUMMARY 对齐）
9. chattr +i overlay2 兼容性 spike 回填 53-CONTEXT Open Question #4

### Gaps Summary

**Phase 53 实现层完整、wiring 正确、anti-pattern 干净**。所有 12 REQ 声明并基本满足；5 条 SC 在静态/代码层 3/5 满分，其余 2 条因 fixture 设计决策（T1 真出口 IP）与 smoke 测试覆盖（T4 ip link 断言）部分缺失。

**真实 gap（2 项）：**

1. **smoke.sh T-53-4 缺 `ip link set tun0 down` EPERM 断言** —— SC4 / NET-04 的第二条 oracle（"`ip link set tun0 down` 必须返回 `Operation not permitted`"）在 smoke.sh 中无对应断言。空 cap 实现完整（Dockerfile 删 sudo + 不在 sudo group + 用户 binary 无 file cap），EPERM 是内核行为而非额外实现，但 SC 显式要求测试 oracle 覆盖。修复成本：smoke.sh L86 后追加 1 条 docker exec 断言。

2. **Dockerfile 缺 `dnsutils` (dig 工具)** —— IMG-03 REQ 显式列 `dig` 为镜像必装工具，NET-02 SC `dig @8.8.8.8 example.com` 验证用例无可执行 binary。当前实现：nft drop 53/853 规则 ✓（default-deny.nft L25-26），nft counter 可作替代 oracle，但与 REQ 文本不一致。修复成本：Dockerfile L25-57 apt install 段追加 `dnsutils`（或更精简的 `bind9-host`）。

**Deferred items（明确归属，非 gap）：**

- **deferred-to-Phase-55-CI（运行时端到端，9 项）：** 所有 docker run / docker exec 类 oracle（含 T-53-1..6 smoke 跑通、53-01 V5/V6 复验、53-02 V3..V7 端到端、53-03 V3、chattr overlay2 spike）。前置：D-53-PRE-1（Claude Code 安装段 `claude.ai/install.sh` HTTP 403 + GitHub release `claude-code-linux-aarch64.tar.gz` HTTP 404）修复或 CI 干净环境。

- **deferred-to-Phase-55（fixture 真上游 outbound，1 项）：** SC1 完整语义 "curl 返回**绑定的出口 IP**"。本 phase fixture 用 `direct` outbound（出口 IP = 宿主 eth0 IP），仅能验证『路由经 tun0』而非『出口 IP = 绑定 IP』。Phase 55 SC4 显式覆盖 v3.6 MVS-02 出口 IP 三源轮询用例迁移，届时用真上游代理 outbound fixture 验证完整契约。

**修复建议（非 phase blocker，可在 Phase 55 CI 解锁前一起 close）：**

```dockerfile
# Dockerfile L25-57 apt install 段追加：
        dnsutils \   # 或 bind9-host（更精简）
```

```bash
# tests/phase53/smoke.sh L86 后追加 T-53-4b：
log "[T-53-4b] workspace 不能 ip link set tun0 down"
if docker exec -u workspace "$CONTAINER_NAME" ip link set tun0 down 2>&1 | grep -q "Operation not permitted"; then
  : # OK
else
  fail "T-53-4b workspace 居然能 ip link set tun0 down"
fi
```

---

_Verified: 2026-05-16T02:55:00Z_
_Verifier: Claude (gsd-verifier)_
