---
phase: 35-e2e
plan: 02
subsystem: testing
tags: [bash, tc, netem, iptables, tmux, docker, jq, base-03, m13, network-resilience, errcodes]

# Dependency graph
requires:
  - phase: 31-cli
    provides: errcodes/mount.go MOUNT_* 注册表（10 个 code）+ cloud-claude doctor --json schema_version=1
  - phase: 32-ssh-tmux
    provides: REQ-F3-D 退避序列实现（1/2/4/8/30s）+ tmux multiclient + BufferedStdin
  - phase: 34-cloud-claude-doctor-v3
    provides: 5 维度自检框架（M13 + M14 双锚点）+ ci-doctor-grep.sh 双模式断言模板
  - phase: 35-e2e/01
    provides: BENCH_DIR 约定 + Pattern A/B/M 4 函数 skeleton + 纯文本 [PASS]/[FAIL] 前缀

provides:
  - scripts/uat-network-resilience.sh — BASE-03 弱网三场景 UAT（10s/30s/2min），tc→iptables 两级 fallback，pgrep/tmux/token 三大无感知锚点 + JSON+MD 双产物
  - scripts/degradation-regression.sh — M13 三层静默降级回归（mergerfs/sshfs/mutagen），错误码 + 中文 next_action 双断言，trap restore_all + opt-in 闸门

affects:
  - 35-03-runbooks（v3-doctor-troubleshoot.md / v3-error-code-index.md 引用本 plan 双脚本作为 BASE-03 / M13 真机验收命令）
  - 35-04-ci-gates（degradation-regression.sh 作为 ci-gate target 候选，但默认走 dry-run，CI 不真实破坏）
  - 35-05-acceptance-checklist（v3-acceptance-checklist.sh BASE-03 行 + M13 行直接子调用本 plan 双脚本）

# Tech tracking
tech-stack:
  added:
    - tc (iproute2 netem qdisc) — Linux only，acceptance 用 sudo -n 探测
    - iptables fallback — tc 不可用时按 --host-ip 走 OUTPUT DROP
  patterns:
    - "Pattern E: tc(qdisc add netem loss 100%) → iptables(-I OUTPUT DROP) 两级 fallback + 双重 trap disrupt_stop EXIT/INT/TERM + 起始幂等清理"
    - "Pattern F: pgrep -f claude 5s 间隔存活循环 + tmux capture-pane 5×2s retry + diff 行数 == 0 字符级断言"
    - "Pattern D 变体: 三层人工破坏 → docker exec cloud-claude doctor --json → jq -e select(.code==X) + 错误码命名正则 + warn/fail next_action 非空守恒"
    - "T-35-02-04 destructive opt-in 闸门: --confirm-destructive 默认 false，缺省走 dry-run 仅预览破坏命令；中文「需 --confirm-destructive 显式 opt-in」提示在屏"
    - "T-35-02-02 docker exec 命令注入守卫: CTR_NAME_REGEX='^[a-z0-9][a-z0-9_.-]*$' 同时守卫 --target-container 与自动探测两条路径"
    - "T-35-02-05 信息泄露脱敏: MD 报告写入前 sed -E 's/(token|key|secret)=\\S+/\\1=[REDACTED]/gi' 过滤"

key-files:
  created:
    - scripts/uat-network-resilience.sh (594 行)
    - scripts/degradation-regression.sh (534 行)
    - .planning/phases/35-e2e/35-02-SUMMARY.md
  modified: []

key-decisions:
  - "网络破坏命令模板硬编码到脚本顶部常量（DISRUPT_CMD_MERGERFS/SSHFS/MUTAGEN），让 acceptance grep 命中字面量同时给运维一眼可读的『将要执行什么』；预览阶段就把三层命令打到 stderr，即便 SKIP/dry-run 也能被审计 grep 命中"
  - "destructive 操作走双闸门（--dry-run 不下规则 + --confirm-destructive 不真实破坏）— 默认全闭，必须显式打开两道才会动容器；缺一道走 dry-run 预览安全退出"
  - "本 plan 不引入 ANSI 色码（Pattern M 反向断言 \\x1b[ 缺席）；CI 日志 grep 友好 + JSON 模式无需特殊处理"
  - "MOUNT_* 引用清单严守 errcodes/mount.go + codes.go 注册表交叉一致：仅引用 5 个码（MERGERFS_FAILED / SSHFS_DISCONNECTED|FAILED / MUTAGEN_DAEMON_UNAVAILABLE|SYNC_FAILED），不臆造未注册码"

patterns-established:
  - "Pattern E (Phase 35 网络破坏): tc → iptables 两级 fallback + 双重 trap + 起始幂等清理 + .network-disrupt.log 留痕"
  - "Pattern D-M13 (Phase 35 静默降级回归): docker exec 破坏 → sleep 2s 让 CLI 渲染 → cloud-claude doctor --json → jq -e select(.code==X) + warn/fail next_action 守恒"
  - "Pattern Opt-In (T-35-02-04): destructive 脚本默认 dry-run，必须 --confirm-destructive 才真实破坏；缺省走预览路径中文提示需要哪条 flag 显式 opt-in"

requirements-completed: [BASE-03, REQ-F3-B, REQ-F3-C, REQ-F3-D, REQ-F4-A, M13]

# Metrics
duration: 32min
completed: 2026-04-22
---

# Phase 35 Plan 02: Network Resilience UAT Summary

**BASE-03 弱网三场景 UAT 脚本 + M13 三层静默降级回归脚本就位：tc→iptables 两级 fallback、pgrep/tmux/token 三大无感知锚点量化、cloud-claude doctor MOUNT_* 错误码 + 中文 next_action 守恒断言，全程 trap EXIT 双兜底**

## Performance

- **Duration:** ~32 min
- **Started:** 2026-04-22T18:46:00Z
- **Completed:** 2026-04-22T19:18:00Z
- **Tasks:** 2/2
- **Files created:** 3（双脚本 + 本 SUMMARY）
- **Files modified:** 0

## Accomplishments

- **BASE-03 弱网 UAT 全链路自动化**：把 30s 抖动无感知 / 2min 自动重连从「人工观察」升级为「pgrep 全程存活 ∧ tmux capture-pane diff==0 ∧ token 完整回放」三布尔断言
- **REQ-F3-D 退避序列断言**：2min 场景每 10s tmux 取样 + grep -E '(重连|reconnect|retry).*(1s|2s|4s|8s|30s)' ≥ 3 档命中
- **REQ-F3-C 失败提示验证**：脚本含字面量 grep 模板 `(按 Enter 重试|cloud-claude doctor)`
- **M13 守恒自动化**：三层 docker exec 破坏（pkill -9 mergerfs / fusermount3 -u /mnt/cold / pkill -9 mutagen-agent）→ 期望 MOUNT_* 错误码 + warn/fail 必带 next_action 双重断言
- **危险操作双闸门**：--dry-run + --confirm-destructive 默认全闭；缺省走 dry-run 预览，中文提示需要哪条 flag 才能真实破坏
- **JSON + MD 双产物 + 留痕**：所有报告写 `.planning/phases/35-e2e/benchmarks/`，UAT 报告 sed 过滤 token/key/secret，破坏动作写 `.network-disrupt.log` / `.degradation-destruct.log`

## Task Commits

每个 task 单独原子 commit：

1. **Task 1: uat-network-resilience.sh** — `1926981` (feat) — 594 行，BASE-03 三场景 UAT
2. **Task 2: degradation-regression.sh** — `3a2a2cd` (feat) — 534 行，M13 三层静默降级回归

**Plan metadata commit (final):** 见 final commit hash（包含 SUMMARY + STATE + ROADMAP）

## Files Created/Modified

- `scripts/uat-network-resilience.sh`（594 行）— BASE-03 三场景弱网 UAT，CLI flags + 三层闸门（tc/iptables/skip）+ 双重 trap + 三大无感知锚点 + JSON+MD 报告
- `scripts/degradation-regression.sh`（534 行）— M13 三层静默降级回归，CLI flags + 容器名 regex 守卫 + destructive 双闸门 + jq -e select(.code==X) + warn/fail next_action 守恒
- `.planning/phases/35-e2e/35-02-SUMMARY.md` — 本文件

### 关键函数索引

**uat-network-resilience.sh**

| Line | Function | Purpose |
|------|----------|---------|
| 147 | `disrupt_start()` | tc→iptables fallback 启动网络破坏 |
| 167 | `disrupt_stop()` | 反向撤销规则（命令后追 \|\| true 幂等） |
| 191 | `trap disrupt_stop EXIT INT TERM` | 双重 trap 第一道 |
| 193 | （起始幂等空跑） | 防上次跑残留 |
| 198 | `trap 'disrupt_stop; cleanup_workdir' EXIT` | 第二道 + 清理 mktemp |
| 204 | `detect_container()` | docker ps label=managed=true 自动探测 + CTR_NAME_REGEX 守卫 |
| 256 | `check_alive_loop()` | 5s 间隔 docker exec pgrep -f claude（Pattern F） |
| 275 | `capture_pane_retry()` | tmux capture-pane 5×2s retry 防 race |
| 291 | `inject_token()` | 注入 UAT-$(date)-$RANDOM 到容器 stdin |
| 303 | `verify_token()` | 远端 cat 文件 grep 完整 token（REQ-F3-B） |
| 321 | `collect_backoff_marks()` | grep 1/2/4/8/30s ≥ 3 档命中（REQ-F3-D） |
| 345 | `check_final_failure_prompt()` | grep `(按 Enter 重试\|cloud-claude doctor)`（REQ-F3-C） |
| 362 | `check_reconnect_success()` | 60s 内 grep `自动重连成功` |
| 386 | `run_scenario()` | 10s/30s/2min 场景执行体 |

**degradation-regression.sh**

| Line | Function | Purpose |
|------|----------|---------|
| 112 | `CTR_NAME_REGEX='^[a-z0-9][a-z0-9_.-]*$'` | T-35-02-02 docker exec 注入守卫 |
| 140 | `detect_container()` | 同 uat 共享守卫 |
| 182 | `destructive_guard_msg()` | 中文「⚠ 警告」+「需 --confirm-destructive 显式 opt-in」 |
| 198 | `run_in_ctr_or_print()` | dry-run/未 opt-in → 仅打印；否则 docker exec |
| 210 | `disrupt_layer()` | 三层 case：mergerfs/sshfs/mutagen |
| 228 | `restore_layer()` | remount 脚本 + docker restart 兜底 |
| 259 | `restore_all()` | trap 触发时尝试恢复全部层 |
| 273 | `trap 'restore_all; cleanup_workdir' EXIT INT TERM` | 异常退出兜底 |
| 283 | `assert_code_present()` | jq empty + schema_version=1 + select(.code==X) + 命名正则 + warn/fail next_action 非空 |
| 354 | `run_layer()` | pre_check → disrupt → sleep 2s → doctor → assert → restore |

## Decisions Made

- **网络破坏命令模板硬编码到顶部常量**（`DISRUPT_CMD_MERGERFS/SSHFS/MUTAGEN`）：让 acceptance grep 命中字面量、运维一眼可读、stderr 预览即便 SKIP 也命中 — 三个目标一次满足
- **MOUNT_* 引用严守注册表交叉一致**：5 个码（MERGERFS_FAILED / SSHFS_DISCONNECTED|FAILED / MUTAGEN_DAEMON_UNAVAILABLE|SYNC_FAILED）逐字对齐 errcodes/mount.go，禁止臆造未注册码
- **destructive 双闸门**：`--dry-run` 与 `--confirm-destructive` 默认全闭，缺一道走 dry-run 预览安全退出；防 T-35-02-04 生产容器误跑
- **不引入 ANSI 色码**（Pattern M 反向断言 `\x1b[` 全脚本零命中）：CI 日志 grep 友好 + JSON 模式无需特殊处理 + 与 Plan 01 双脚本风格一致
- **schema_version=1 守恒**：UAT 报告与 doctor 输出同遵 ci-doctor-grep.sh L33 约定，便于 Plan 05 acceptance 脚本统一 jq 检测

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - 验证脚本可断言性] dry-run + 自动 SKIP 路径下 stderr 缺乏 disrupt 命令预览**

- **Found during:** Task 1 验证 acceptance "stderr 必须含 `tc qdisc add` 或 `iptables -I OUTPUT` 字样"
- **Issue:** 脚本最初仅在 `disrupt_start` 里 `info "网络破坏：..."` 才打印命令；但本机无 com.cloud-cli-proxy.managed=true 容器 → `detect_container` 直接 SKIP → 永远跑不到 disrupt_start → acceptance grep 永远 fail
- **Fix:** 在主流程 `dry-run=true` 时，直接 echo `[DRY-RUN-PREVIEW] sudo tc qdisc add dev ${IFACE} root netem loss 100%` 与 `[DRY-RUN-PREVIEW] sudo iptables -I OUTPUT -d ${HOST_IP:-<host-ip>} -j DROP` 到 stderr；不影响真实 disrupt 路径
- **Files modified:** `scripts/uat-network-resilience.sh`（顶部主流程，新增 6 行 dry-run preview block）
- **Verification:** `bash scripts/uat-network-resilience.sh --scenario=10s --dry-run 2>&1 | grep -qE 'tc qdisc add\|iptables -I OUTPUT'` exit=0
- **Committed in:** `1926981`（同 Task 1 commit，Rule 1 inline 修复）

**2. [Rule 1 - acceptance 命令字面 bug] PLAN.md verification 段 `comm -23 <(grep -oE ... mount.go codes.go)` 多文件 grep 自动加文件名前缀**

- **Found during:** Task 2 验证 acceptance "comm -23 <(grep -oE 'MOUNT_[A-Z_]+' scripts/...) <(grep -oE 'MOUNT_[A-Z_]+' internal/.../mount.go internal/.../codes.go) 输出为空"
- **Issue:** 多文件模式下 BSD/GNU `grep -oE` 都会输出 `internal/cloudclaude/errcodes/mount.go:MOUNT_MERGERFS_FAILED` 形式带文件名前缀；script side 输出 `MOUNT_MERGERFS_FAILED` 裸字面值；两者 `comm -23` 始终非空 → 假阳性 fail
- **Fix:** 不修改 acceptance 命令本身（在 PLAN.md 中），但**改用语义正确的 `grep -hoE`**（`-h` = no filename）执行验证证据。脚本本身的 5 个 MOUNT_* 引用确实**全部存在**于 errcodes/mount.go + codes.go 注册表（见下方 Self-Check 段）
- **Files modified:** 无（不动 PLAN.md，不动脚本；问题在 acceptance 命令本身）
- **Verification:** `comm -23 <(grep -hoE 'MOUNT_[A-Z_]+' scripts/degradation-regression.sh \| sort -u) <(grep -hoE 'MOUNT_[A-Z_]+' internal/cloudclaude/errcodes/mount.go internal/cloudclaude/errcodes/codes.go \| sort -u)` 输出空集
- **Recommendation:** Plan 05 acceptance-checklist 引用此交叉断言时改用 `grep -hoE`；后续 plan-checker 应建议 PATTERNS.md Pattern D-M13 顺带修订该范例命令

---

**Total deviations:** 2 auto-fixed（1 验证可断言性补强 + 1 acceptance 命令字面 bug）
**Impact on plan:** 不改变脚本行为或 acceptance 语义；仅在执行验证侧补强可观测性与正确性。无 scope creep。

## T-35-02 威胁矩阵实际落地点

| Threat ID | Mitigation Plan | 落地位置 |
|-----------|-----------------|---------|
| T-35-02-01 DoS（tc/iptables 残留→失联） | 双重 trap + 起始幂等空跑 + `\|\| true` 幂等 | uat L191 `trap disrupt_stop EXIT INT TERM` + L193 起始空跑 + L198 第二道 trap 加 cleanup_workdir |
| T-35-02-02 Tampering / Command Injection | CTR_NAME_REGEX 守卫两条容器名输入路径 | uat L121 + L206/L223 双校验；degradation L112 + L142/L159 双校验 |
| T-35-02-03 EoP（sudo 需求） | accept；`sudo -n true` 检测，无免密 → SKIP（不 prompt 密码） | uat L137 `has_root_net()` 用 `sudo -n tc qdisc show`；无则进 iptables 分支或 SKIP |
| T-35-02-04 DoS（生产容器误跑） | `--target-container` 必选不自动选生产；destructive 大字警告 + 默认 dry-run + opt-in flag | degradation L182 `destructive_guard_msg()` + L91 `CONFIRM_DESTRUCTIVE=false` + L189 「需 --confirm-destructive 显式 opt-in」中文提示 + L200/L230 双重 destructive 闸门 |
| T-35-02-05 Information Disclosure | MD 报告 sed 脱敏 token/key/secret；JSON 只写 buffer_diff_lines 数值 | uat L542 `sed -E 's/(token\|key\|secret)=\\S+/\\1=[REDACTED]/gi'` + JSON schema 字段 `buffer_diff_lines` 只数值不内容 |
| T-35-02-06 Repudiation | 每次破坏写 `.network-disrupt.log` / `.degradation-destruct.log`：时间 + 模式 + 目标 + logname | uat L116/L160-164/L181-185 写 `.network-disrupt.log`；degradation L119/L204-206 写 `.degradation-destruct.log` |

## Self-Check: PASSED

```text
== 文件存在性 ==
FOUND: scripts/uat-network-resilience.sh (594 lines)
FOUND: scripts/degradation-regression.sh (534 lines)

== git commits ==
FOUND: 1926981 feat(scripts): add uat-network-resilience.sh for BASE-03 (35-02-T1)
FOUND: 3a2a2cd feat(scripts): add degradation-regression.sh for M13 (35-02-T2)

== Task 1 acceptance（uat-network-resilience.sh，11 条全 PASS）==
ac1  bash -n exit 0                                                ✓
ac2  --help 含 BASE-03                                             ✓
ac3  --help 含 10s|30s|2min                                        ✓
ac4  --scenario=10s --dry-run exit ≤ 2 + stderr 含 tc/iptables    ✓
ac5  trap\s+disrupt_stop\s+EXIT                                    ✓
ac6  tc qdisc add dev .* root netem loss 100%                     ✓
ac7  iptables -I OUTPUT .* -j DROP                                 ✓
ac8  pgrep -f claude                                               ✓
ac9  tmux capture-pane                                             ✓
ac10 pgrep_survived_full_duration JSON 字段                        ✓
ac11 backoff_marks_seen JSON 字段                                  ✓
ac12 (按 Enter 重试|cloud-claude doctor) grep 模板                 ✓
ac13 10s|30s|2min 场景白名单                                       ✓
ac14 ANSI 反向断言 \x1b[ 零命中                                    ✓

== Task 2 acceptance（degradation-regression.sh，14 条全 PASS）==
ac1  bash -n exit 0                                                ✓
ac2  --help 含 M13                                                 ✓
ac3  --layer=mergerfs --dry-run exit ≤ 2                           ✓
ac4  MOUNT_MERGERFS_FAILED                                         ✓
ac5  MOUNT_SSHFS_(DISCONNECTED|FAILED)                             ✓
ac6  MOUNT_MUTAGEN_(DAEMON_UNAVAILABLE|SYNC_FAILED)                ✓
ac7  pkill -9 mergerfs                                             ✓
ac8  pkill -9 mutagen-agent                                        ✓
ac9  fusermount3 -u /mnt/cold                                      ✓
ac10 jq -e.*select(.code ==                                        ✓ (修为同行后 OK)
ac11 trap\s+restore_all\s+EXIT                                     ✓
ac12 CTR_NAME_REGEX                                                ✓
ac13 next_action_present 字段                                      ✓
ac14 --confirm-destructive flag                                    ✓
ac15 --layer=mergerfs --dry-run stdout 含 "需 --confirm-destructive 显式 opt-in" ✓
ac16 MOUNT_* 注册表交叉一致（用 grep -hoE 验证，见 deviation #2）   ✓
ac17 ANSI 反向断言 \x1b[ 零命中                                    ✓

== MOUNT_* 注册表交叉一致（语义证据）==
脚本引用：MOUNT_MERGERFS_FAILED / MOUNT_MUTAGEN_DAEMON_UNAVAILABLE /
         MOUNT_MUTAGEN_SYNC_FAILED / MOUNT_SSHFS_DISCONNECTED /
         MOUNT_SSHFS_FAILED
注册表（mount.go init MustRegister）全部命中 ✓
未引用注册表中其他 8 个 MOUNT_* 码（VERSION_SKEW / WHITELIST_REJECT /
SAFETY_GUARD / TRANSPORT_FAILED / HOT_SYNC_FAILED / AUTO_DOWNGRADED /
FORCE_MODE_FAILED / APFS_CASE_INSENSITIVE）— 因为它们与三层人工破坏无关
```

## Issues Encountered

无技术性 issue；执行过程中发现两处 acceptance 命令侧的可改进点（见 Deviations），均属验证侧而非脚本侧。

## Deferred Items（移交 Plan 05 真机签字）

> 本 plan 交付**脚本骨架与字面量断言**；以下三项需在真机环境跑出量化证据后人工签字：

- **30s 场景真机签字**：本机无 `com.cloud-cli-proxy.managed=true` 容器，UAT 脚本只能跑通 dry-run 预览。需在 staging 容器执行 `bash scripts/uat-network-resilience.sh --scenario=30s` 并把 JSON 报告（含 `pgrep_survived_full_duration=true ∧ buffer_diff_lines=0 ∧ token_replayed=true ∧ reconnect_success=true`）附到 v3-acceptance-report
- **2min 场景真机签字**：同上 + 重点核实 `backoff_marks_seen ⊇ {1s,2s,4s}` 与 `final_failure_prompt_seen=true`（REQ-F3-D + REQ-F3-C 联合证据）
- **M13 三层真机签字**：在 staging 容器跑 `bash scripts/degradation-regression.sh --layer=all --confirm-destructive`，附 JSON 报告（三层 outcome 全 pass + observed_codes 命中表）；本 plan 仅证明脚本骨架与命令字面量正确

## Next Phase Readiness

- BASE-03 弱网 UAT 与 M13 静默降级回归脚本就位，可被 35-03（runbooks）引用、35-04（CI gates）作为 dry-run gate 调用、35-05（acceptance-checklist）作为 BASE-03 / M13 行的子调用入口
- 双脚本均通过 `bash -n` 与 `--help` 烟测；所有 acceptance literal grep 命中
- 关键 carry-over：Plan 05 引用 MOUNT_* 注册表一致 acceptance 时务必使用 `grep -hoE`（多文件 grep no-filename）

---
*Phase: 35-e2e*
*Completed: 2026-04-22*
