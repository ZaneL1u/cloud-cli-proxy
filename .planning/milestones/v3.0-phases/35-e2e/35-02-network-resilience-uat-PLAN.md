---
phase: 35-e2e
plan: 02
type: execute
wave: 1
depends_on: []
autonomous: true
files_modified:
  - scripts/uat-network-resilience.sh
  - scripts/degradation-regression.sh
requirements_addressed:
  - BASE-03
  - REQ-F3-B
  - REQ-F3-C
  - REQ-F3-D
  - REQ-F4-A
  - M13
threat_model_severity: high
must_haves:
  truths:
    - "执行 uat-network-resilience.sh --scenario=10s|30s|2min 后 JSON 报告含 pgrep_survived_full_duration=true 断言位"
    - "30s 拔网恢复后 tmux capture-pane diff 行数 = 0（字符级一致）"
    - "2min 拔网场景 stderr 匹配 REQ-F3-C 失败提示 + REQ-F3-D 退避 1/2/4/8/30s 序列至少出现一次"
    - "degradation-regression.sh 破坏三层（mergerfs kill / sshfs umount / mutagen-agent kill）每层都触发对应 MOUNT_* 错误码"
    - "两个脚本都实现 tc → iptables 两级 fallback，均带 trap EXIT 恢复网络"
  artifacts:
    - path: "scripts/uat-network-resilience.sh"
      provides: "BASE-03 弱网 UAT 三场景自动化（10s/30s/2min） + pgrep/tmux/stdin 三重量化断言"
      min_lines: 180
    - path: "scripts/degradation-regression.sh"
      provides: "M13 静默降级终验：三层人工破坏 + stderr 错误码 grep 断言"
      min_lines: 140
  key_links:
    - from: "scripts/uat-network-resilience.sh"
      to: "disrupt_start / disrupt_stop 函数对（tc qdisc netem loss 100% ↔ iptables DROP）"
      via: "trap disrupt_stop EXIT 保证脚本异常退出一定恢复网络"
      pattern: "trap disrupt_stop EXIT"
    - from: "scripts/degradation-regression.sh"
      to: "internal/cloudclaude/errcodes/mount.go 注册的 MOUNT_* Code"
      via: "docker exec + cloud-claude doctor --json 解析 .checks[].code"
      pattern: "jq .*select\\(.code == \"MOUNT_"
    - from: "scripts/uat-network-resilience.sh"
      to: "REQ-F3-D 退避序列 1/2/4/8/30s"
      via: "脚本注入 stdin + 每 10s 打印状态，日志含退避秒数"
      pattern: "backoff.*(1s|2s|4s|8s|30s)"
---

<objective>
交付 BASE-03（弱网 30s 无感知 / 2min 自动重连）与 M13（禁止静默降级）两条验收基线的可脚本化 UAT。
Purpose: 把 REQ-F3-B/C/D + REQ-F4-A 的 UX 行为从"人工观察"升级为"脚本可断言的量化指标"（CONTEXT.md §"无感知量化指标"）；把 M13 从"文档约定"升级为"三层破坏下 cloud-claude 必须吐错误码"的自动化回归。
Output: 两个 bash 脚本。UAT 脚本需支持 10s / 30s / 2min 三场景；M13 脚本需覆盖三层任意破坏 → 对应 MOUNT_* / SSH_* 错误码断言。两脚本必须做网络破坏的 `trap EXIT` 恢复，**脚本异常退出时一定回滚 tc/iptables 规则**，否则宿主机失联。
</objective>

<execution_context>
@.cursor/get-shit-done/workflows/execute-plan.md
@.cursor/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/PROJECT.md
@.planning/ROADMAP.md
@.planning/phases/35-e2e/35-CONTEXT.md
@.planning/phases/35-e2e/35-RESEARCH.md
@.planning/phases/35-e2e/35-PATTERNS.md

<!-- 分析对象引用 -->
@scripts/verify-fuse-compat.sh
@scripts/ci-doctor-grep.sh
@internal/cloudclaude/errcodes/mount.go
@internal/cloudclaude/errcodes/codes.go
@deploy/scripts/host-preflight.sh
</context>

<interfaces>
<!-- errcodes/mount.go 注册的 Code（从 codes.go L120-133 实际值取材），degradation-regression 须断言其中一部分 -->

| 破坏动作 | 期望 .checks[].code |
|----------|---------------------|
| `pkill -9 mergerfs`（容器内） | `MOUNT_MERGERFS_FAILED` |
| `umount /mnt/cold`（容器内 sshfs） | `MOUNT_SSHFS_DISCONNECTED` 或 `MOUNT_SSHFS_FAILED` |
| `pkill -9 mutagen-agent`（容器内） | `MOUNT_MUTAGEN_DAEMON_UNAVAILABLE` 或 `MOUNT_MUTAGEN_SYNC_FAILED` |
| 破坏 mergerfs 参数（umount + mount 回少关键 option） | `MOUNT_MERGERFS_FAILED` |

errcodes 命名正则（codes.go L56）：`^[A-Z]+_[A-Z]+_[A-Z0-9]+(_[A-Z0-9]+)*$`

doctor JSON schema（Plan 01 Interfaces 已复用）：`.schema_version == 1` + `.checks[] | {domain, name, status, code, message, next_action}`。

REQ-F3-D 退避序列（REQUIREMENTS.md L34）：`1s → 2s → 4s → 8s → 30s 上限`。
</interfaces>

<tasks>

<task type="execute" id="35-02-T1">
  <name>Task 1: uat-network-resilience.sh — BASE-03 三场景 UAT + pgrep/tmux/stdin 量化</name>
  <files>scripts/uat-network-resilience.sh</files>
  <read_first>
    - scripts/uat-network-resilience.sh（**新文件**）
    - scripts/verify-fuse-compat.sh（1-30 行 skeleton + 74-85 行容器 helper）
    - scripts/ci-doctor-grep.sh（17-19 行 WORK + trap 模式）
    - deploy/scripts/host-preflight.sh（11-48 行 — is_ubuntu25 / has_tc 三层闸门样板，Pattern B）
    - .planning/phases/35-e2e/35-PATTERNS.md Pattern E（tc → iptables fallback）+ Pattern F（tmux capture-pane retry）
    - .planning/phases/35-e2e/35-CONTEXT.md §"弱网 UAT 执行方式" + "无感知量化指标"（全文必读，硬性锁定 3 条量化指标）
    - .planning/REQUIREMENTS.md L32-34（REQ-F3-B/C/D 原文） + L38（REQ-F4-A 原文） + L79（BASE-03 原文）
  </read_first>
  <action>
创建 `scripts/uat-network-resilience.sh`（≥ 180 行）。

1. **CLI flags**：
   - `--scenario=10s|30s|2min`（必选，互斥）
   - `--target-container=NAME`（默认从 `docker ps --filter label=com.cloud-cli-proxy.managed=true --format '{{.Names}}' | head -1` 自动探测；找不到则 `skip`）
   - `--iface=NAME`（默认 `eth0`；macOS 走 `iptables` fallback 或 `pfctl`，本脚本只实现 tc + iptables 两级）
   - `--host-ip=IP`（iptables fallback 时必须给 — 控制面 IP）
   - `--dry-run`（不真实下 tc/iptables 规则，只打印命令）
   - `--output-dir=DIR`（默认 `.planning/phases/35-e2e/benchmarks`）
   - `--help`
2. **环境闸门**（Pattern B 三层，PATTERNS.md L166-194）：
   - `is_linux` + `has_tc` + `has_root_net` → 使用 tc
   - 其次 `has_iptables` + `--host-ip` 非空 → 使用 iptables
   - 都没有 → `skip "BASE-03" "tc/iptables 均不可用或缺目标 IP"` 退出码 2
3. **网络破坏函数（Pattern E，PATTERNS.md L266-295）**：
   ```bash
   disrupt_start() { ... ; DISRUPT_MODE="tc"|"iptables" ; }
   disrupt_stop()  { 依 DISRUPT_MODE 反向撤销，命令后加 `|| true`（幂等） }
   trap disrupt_stop EXIT INT TERM
   ```
   **双重兜底**：脚本起始处 `disrupt_stop` 空跑一次清理任何遗留规则；末尾再显式调用。
4. **存活断言循环（Pattern F，PATTERNS.md L303-316）**：
   - `check_alive_loop duration ctr` — 每 5s `docker exec $ctr pgrep -f claude`，任一次退出非 0 即 `fail "claude 进程在第 ${t}s 退出"` + 记 `pgrep_survived_full_duration=false`
5. **Buffer 完整性断言（Pattern F，L318-331）**：
   - 拔网前：`BUF_BEFORE=$(docker exec $ctr tmux capture-pane -t claude -p -e 2>/dev/null)`
   - 恢复网络并 sleep 2s 后用 retry 循环（最多 5 次 × 2s sleep）取 `BUF_AFTER`
   - `diff <(echo "$BUF_BEFORE") <(echo "$BUF_AFTER")` 行数必须为 0
6. **输入回放断言（REQ-F3-B 锁定，CONTEXT §"无感知"第 3 条）**：
   - 拔网前注入 `TOKEN="UAT-$(date +%s)-$RANDOM"`：`echo "cat > /tmp/uat-echo-$$.txt <<< $TOKEN" | docker exec -i $ctr tmux send-keys -t claude`
   - 拔网 + 恢复 + 60s 等待重连
   - 断言 `docker exec $ctr cat /tmp/uat-echo-*.txt | grep -qF "$TOKEN"`（远端最终收到完整字符串 → 证实本地 buffer 无丢无乱序）
7. **场景 10s**：
   - 判定：`pgrep_survived_full_duration=true` + `BUF diff==0` + `token_replayed=true` 三个布尔全为真 → PASS
   - 失败即退出码 1
8. **场景 30s**：
   - 执行 10s 场景的三条断言
   - 额外：恢复网络后 60s 内 `docker exec $ctr cloud-claude --json 2>&1 | grep -q '"reconnect":true'` 或 `tmux capture-pane | grep -qF "自动重连成功"`（REQ-F3-C 反向——成功路径无失败提示）
9. **场景 2min**：
   - 拔网 120s，在拔网过程中每 10s `tmux capture-pane` 取样并写 `$WORK/reconnect-samples.txt`
   - **REQ-F3-D 退避序列断言**：`grep -cE '(重连|reconnect|retry).*(1s|2s|4s|8s|30s)' $WORK/reconnect-samples.txt` ≥ 3（证据：至少三档退避 mark 出现过）
   - **REQ-F3-C 最终失败断言**：2min 结束后 `tmux capture-pane | grep -qE '(按 Enter 重试|cloud-claude doctor)'`（两者任意一条在屏）
   - **REQ-F4-A 进程存活断言**：`docker exec $ctr pgrep -f claude` 仍返回 0（tmux 内 claude 进程从未退出）
   - 恢复网络后 `tmux send-keys Enter` 重新触发连接，断言 10s 内 `tmux capture-pane` 不再含失败提示字样
10. **JSON 报告**（`$BENCH_DIR/uat-resilience-${SCENARIO}-$(date +%Y%m%d-%H%M%S).json`）：
    ```json
    { "schema_version": 1, "scenario": "30s", "container": "...", "disrupt_mode": "tc",
      "pgrep_survived_full_duration": true, "buffer_diff_lines": 0,
      "token_replayed": true, "reconnect_success": true,
      "backoff_marks_seen": ["1s","2s","4s"], "final_failure_prompt_seen": false,
      "outcome": "pass" }
    ```
11. **MD 报告**：人类可读表格 + 关键日志抽样（`$WORK/reconnect-samples.txt` 最后 30 行）。
12. **trap EXIT** 始终恢复网络（即使脚本被 Ctrl+C）。

**硬编码具体值**：`--scenario` 仅接 `10s|30s|2min`；阈值常量 `RECONNECT_WINDOW_S=60`、`SAMPLE_INTERVAL_S=5`、`CAPTURE_RETRY=5`。
  </action>
  <acceptance_criteria>
    - `bash -n scripts/uat-network-resilience.sh` 退出码 0
    - `bash scripts/uat-network-resilience.sh --help` 退出码 0 且 stdout 含 `BASE-03` 与 `10s|30s|2min`
    - `bash scripts/uat-network-resilience.sh --scenario=10s --dry-run` 退出码 ≤ 2（SKIP 或 PASS 都接受，**但 stderr 必须含 `tc qdisc add` 或 `iptables -I OUTPUT` 字样**）
    - `grep -qE 'trap\s+disrupt_stop\s+EXIT' scripts/uat-network-resilience.sh` 退出码 0
    - `grep -qE 'tc qdisc add dev .* root netem loss 100%' scripts/uat-network-resilience.sh` 退出码 0
    - `grep -qE 'iptables -I OUTPUT .* -j DROP' scripts/uat-network-resilience.sh` 退出码 0
    - `grep -qE 'pgrep -f claude' scripts/uat-network-resilience.sh` 退出码 0
    - `grep -qE 'tmux capture-pane' scripts/uat-network-resilience.sh` 退出码 0
    - `grep -qF 'pgrep_survived_full_duration' scripts/uat-network-resilience.sh` 退出码 0（JSON 字段固化）
    - `grep -qF 'backoff_marks_seen' scripts/uat-network-resilience.sh` 退出码 0
    - `grep -qE '(按 Enter 重试|cloud-claude doctor)' scripts/uat-network-resilience.sh` 退出码 0（REQ-F3-C grep 模板存在）
    - `grep -qE '10s\|30s\|2min' scripts/uat-network-resilience.sh` 退出码 0（场景白名单）
  </acceptance_criteria>
  <done>BASE-03 三场景 UAT 可脚本化运行，支持 tc 与 iptables 两级破坏，异常退出一定恢复网络。</done>
</task>

<task type="execute" id="35-02-T2">
  <name>Task 2: degradation-regression.sh — M13 三层静默降级终验</name>
  <files>scripts/degradation-regression.sh</files>
  <read_first>
    - scripts/degradation-regression.sh（**新文件**）
    - scripts/ci-doctor-grep.sh（51-72 行 — awk section 匹配 + grep `错误码:\s*[A-Z]+_[A-Z]+_[A-Z0-9]+` 断言，Pattern D）
    - scripts/verify-fuse-compat.sh（74-85 行容器 helper）
    - internal/cloudclaude/errcodes/codes.go（120-178 行 — MOUNT_* / SYSTEM_* 错误码字面值）
    - internal/cloudclaude/errcodes/mount.go（init MustRegister 列表，确认 code 实际注册）
    - .planning/phases/35-e2e/35-PATTERNS.md Pattern D（L228-256 M13 专用变体）
    - .planning/phases/35-e2e/35-RESEARCH.md §Wave 0 Gaps L335-340
  </read_first>
  <action>
创建 `scripts/degradation-regression.sh`（≥ 140 行）。

1. **CLI flags**：`--target-container=NAME`（同 Task 1 自动探测）、`--layer=mergerfs|sshfs|mutagen|all`（默认 `all` 三层依次跑）、`--dry-run`、`--confirm-destructive`（**默认 false**；缺省时仅在 dry-run 模式下打印将执行的破坏命令并退出码 0 提示，强制要求显式 opt-in 才真实执行 pkill / fusermount，T-35-02-04 落地）、`--output-dir=DIR`、`--help`。
2. **流程骨架**（Pattern D，M13 专用变体，L245-254）：
   ```
   for LAYER in mergerfs sshfs mutagen; do
     pre_check "$LAYER"                  # 验证 mount 尚在
     disrupt_layer "$LAYER"              # 下一节
     sleep 2                             # 给 CLI stderr 捕获窗口（REQ-F2-B 明确"≤ 2s 内降级"）
     OUT=$(docker exec "$CTR" cloud-claude doctor --json 2>&1 || true)
     assert_code_present "$OUT" "$LAYER"
     restore_layer "$LAYER"              # 恢复，下一层独立测
   done
   ```
3. **三层破坏方法（硬编码，具体命令）**：
   - **mergerfs**：`docker exec $CTR sh -c 'pkill -9 mergerfs; umount /workspace 2>/dev/null || true'` → 期望 code 含 `MOUNT_MERGERFS_FAILED`
   - **sshfs**：`docker exec $CTR sh -c 'fusermount3 -u /mnt/cold 2>/dev/null || umount -l /mnt/cold 2>/dev/null || true'` → 期望 code 含 `MOUNT_SSHFS_DISCONNECTED` **或** `MOUNT_SSHFS_FAILED`（任一即通过）
   - **mutagen**：`docker exec $CTR sh -c 'pkill -9 mutagen-agent'` → 期望 code 含 `MOUNT_MUTAGEN_DAEMON_UNAVAILABLE` **或** `MOUNT_MUTAGEN_SYNC_FAILED`（任一即通过）
4. **断言（Pattern D）**：
   - JSON schema 合法：`jq empty "$OUT"`（失败=fail 且 dump raw output 到 stderr，同 ci-doctor-grep.sh L29-30 风格）
   - 错误码存在：`jq -e --arg c "$EXPECTED_CODE" '.checks[] | select(.code == $c)' <<< "$OUT"` 退出码 0
   - 错误码非空字符串匹配 `^[A-Z]+_[A-Z]+_[A-Z0-9]+(_[A-Z0-9]+)*$`（codes.go L56 正则）
   - **M13 核心断言**：`status ∈ {warn,fail}` 的 check **必须** 有非空 `next_action`（同 ci-doctor-grep.sh L37-44 守恒）—— "禁止静默降级" 等价于"stderr 必有错误码 + 中文下一步"
5. **恢复函数**（每层独立）：
   - mergerfs：`docker exec $CTR /etc/cloud-claude/remount-mergerfs.sh`（若不存在则 `docker restart $CTR` 并 wait healthcheck）
   - sshfs：同上（restart 兜底）
   - mutagen：`docker exec $CTR sh -c '/etc/cloud-claude/mutagen-agent &'` 或 restart
   - `trap restore_all EXIT INT TERM`（异常退出时一定恢复）
6. **产物**：
   - JSON `$BENCH_DIR/degradation-regression-$(date +%Y%m%d-%H%M%S).json`：
     ```json
     { "schema_version": 1, "layers": [{"layer":"mergerfs","expected_code":"MOUNT_MERGERFS_FAILED","observed_codes":["MOUNT_MERGERFS_FAILED"],"next_action_present":true,"outcome":"pass"},...],
       "summary": {"total":3,"pass":3,"fail":0,"skip":0} }
     ```
   - MD 表格 + 每层破坏前/后的 doctor 摘要片段
7. **退出码**：任一层 assert fail → exit 1；全部 skip（容器不在）→ exit 2；全部 pass → exit 0。

**安全约束（T-35-02-02 硬性）**：脚本开头硬编码 `CTR_NAME_REGEX='^[a-z0-9][a-z0-9_.-]*$'`，拒绝任何不符合 docker 容器名规范的 `--target-container` 参数，防止命令注入。
  </action>
  <acceptance_criteria>
    - `bash -n scripts/degradation-regression.sh` 退出码 0
    - `bash scripts/degradation-regression.sh --help` 退出码 0 且 stdout 含 `M13` 字样
    - `bash scripts/degradation-regression.sh --layer=mergerfs --dry-run` 退出码 ≤ 2
    - `grep -qE 'MOUNT_MERGERFS_FAILED' scripts/degradation-regression.sh` 退出码 0
    - `grep -qE 'MOUNT_SSHFS_(DISCONNECTED|FAILED)' scripts/degradation-regression.sh` 退出码 0
    - `grep -qE 'MOUNT_MUTAGEN_(DAEMON_UNAVAILABLE|SYNC_FAILED)' scripts/degradation-regression.sh` 退出码 0
    - `grep -qE 'pkill -9 mergerfs' scripts/degradation-regression.sh` 退出码 0
    - `grep -qE 'pkill -9 mutagen-agent' scripts/degradation-regression.sh` 退出码 0
    - `grep -qE 'fusermount3 -u /mnt/cold' scripts/degradation-regression.sh` 退出码 0
    - `grep -qE 'jq -e.*select\(.code ==' scripts/degradation-regression.sh` 退出码 0（Pattern D 断言存在）
    - `grep -qE 'trap\s+restore_all\s+EXIT' scripts/degradation-regression.sh` 退出码 0
    - `grep -qE 'CTR_NAME_REGEX' scripts/degradation-regression.sh` 退出码 0（容器名校验，T-35-02-02）
    - `grep -qE 'next_action_present|next_action.*!=\s*""' scripts/degradation-regression.sh` 退出码 0（M13 中文下一步断言）
    - `grep -qE '\-\-confirm-destructive' scripts/degradation-regression.sh` 退出码 0（T-35-02-04 opt-in 闸门存在）
    - `bash scripts/degradation-regression.sh --layer=mergerfs --dry-run` stdout 含 "需 --confirm-destructive 显式 opt-in" 字样（缺省安全提示生效）
    - `comm -23 <(grep -oE 'MOUNT_[A-Z_]+' scripts/degradation-regression.sh | sort -u) <(grep -oE 'MOUNT_[A-Z_]+' internal/cloudclaude/errcodes/mount.go internal/cloudclaude/errcodes/codes.go | sort -u)` 输出为空（脚本提及的每个 MOUNT_* 码都在注册表中存在）
  </acceptance_criteria>
  <done>M13 回归脚本可自动破坏三层、断言错误码 + 中文 next_action，并保证退出时恢复容器挂载。</done>
</task>

</tasks>

<verification>
```bash
bash -n scripts/uat-network-resilience.sh && echo "uat ok"
bash -n scripts/degradation-regression.sh  && echo "degradation ok"
bash scripts/uat-network-resilience.sh --help >/dev/null
bash scripts/degradation-regression.sh --help  >/dev/null

# 关键字面量断言（防"简化版"）
grep -qE 'tc qdisc add dev .* root netem loss 100%' scripts/uat-network-resilience.sh
grep -qE 'iptables -I OUTPUT .* -j DROP' scripts/uat-network-resilience.sh
grep -qE 'trap\s+disrupt_stop\s+EXIT' scripts/uat-network-resilience.sh
grep -qE 'trap\s+restore_all\s+EXIT' scripts/degradation-regression.sh

# 与 errcodes 注册表交叉一致
comm -23 <(grep -oE 'MOUNT_[A-Z_]+' scripts/degradation-regression.sh | sort -u) \
         <(grep -oE 'MOUNT_[A-Z_]+' internal/cloudclaude/errcodes/mount.go internal/cloudclaude/errcodes/codes.go | sort -u) \
  | tee /tmp/degradation-unknown-codes.txt
test ! -s /tmp/degradation-unknown-codes.txt && echo "所有 MOUNT_* 均在注册表"

# ANSI 色码反向断言（Pattern M，统一约束 W-3 修复）
for f in scripts/uat-network-resilience.sh scripts/degradation-regression.sh; do
  ! grep -qP '\x1b\[' "$f" || { echo "ANSI escape detected in $f"; exit 1; }
done
echo "no-ANSI ok (Plan 02 双脚本均为纯文本)"

# T-35-02-04 opt-in 闸门：默认 dry-run 即退出
bash scripts/degradation-regression.sh --layer=mergerfs --dry-run 2>&1 \
  | grep -qF 'confirm-destructive' \
  && echo "confirm-destructive opt-in 提示存在"
```
</verification>

<success_criteria>
- Phase SC #3（BASE-03）：uat-network-resilience.sh 能对 10s/30s/2min 三场景自动断言 pgrep 存活 + tmux buffer 一致 + 输入回放完整 + 退避序列 + 最终失败提示五要素
- Phase SC #7（M13 静默降级回归）：degradation-regression.sh 三层破坏全部触发对应 MOUNT_* 错误码 + 中文 next_action
- REQ-F3-B / F3-C / F3-D / F4-A / M13 / BASE-03 全部出现在本 plan `requirements_addressed` 且对应 `acceptance_criteria` 有 grep 证据
- 两脚本均有 `trap EXIT` 恢复动作 —— CI 或开发机跑中断后不留 tc/iptables 残留规则
</success_criteria>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| 脚本 → 宿主机内核网络 | `sudo tc qdisc add dev eth0 root netem loss 100%`（影响整机网络），或 `sudo iptables -I OUTPUT -d <ip> -j DROP` |
| 脚本 → docker daemon | `docker exec -i <ctr> sh -c "pkill -9 mergerfs | pkill -9 mutagen-agent | fusermount3 -u /mnt/cold"`（破坏容器内核心进程） |
| 脚本 → tmux send-keys | `docker exec -i <ctr> tmux send-keys -t claude "<token>"`（注入字符串到活动会话） |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-35-02-01 | Denial of Service | tc/iptables 规则脚本异常退出未清理 → 宿主机失联 | mitigate | 双重 `trap disrupt_stop EXIT INT TERM` + 脚本开头幂等清理一次 + 每次 disrupt 后 sleep 阶段独立 trap 记录；所有规则命令后追加 `|| true` 保证 disrupt_stop 幂等；rollback 段显式列出手工命令 |
| T-35-02-02 | Tampering / Command Injection | `--target-container` 用户可注入 `$(...)` 到 docker exec | mitigate | 硬编码正则 `CTR_NAME_REGEX='^[a-z0-9][a-z0-9_.-]*$'`；不通过 → exit 1；所有 `docker exec` 参数用 `printf '%q'` 引号兜底 |
| T-35-02-03 | Elevation of Privilege | 需要 sudo 下 tc/iptables | accept | 脚本开头 `sudo -n true` 检测；无免密 sudo → skip + 清晰引导；**禁止** 在脚本中自动 prompt 密码（CI 场景会卡死） |
| T-35-02-04 | Denial of Service | degradation-regression 在生产容器误跑 → 中断用户 claude 进程 | mitigate | `--target-container` 必选，**不自动选择** 生产容器；脚本开头打印大字警告 `\n⚠ 本脚本将 pkill 容器内 mergerfs/sshfs/mutagen-agent，请仅在 staging 或 fixture 容器执行`；`--dry-run` 默认建议（首次执行要 `--confirm-destructive` 显式 opt-in） |
| T-35-02-05 | Information Disclosure | tmux capture-pane 可能含 OAuth token 片段 | mitigate | UAT JSON 写入 `buffer_diff_lines` 数值而非内容；MD 报告只抽 `$WORK/reconnect-samples.txt` 最后 30 行并在脚本中 `sed -E 's/(token|key|secret)=\S+/\1=[REDACTED]/gi'` 过滤 |
| T-35-02-06 | Repudiation | 误跑后无记录证据 | mitigate | 每次网络破坏写 `$BENCH_DIR/.network-disrupt.log`：时间、模式、目标、谁触发（`logname`） |
</threat_model>

<rollback>
- 脚本新增，回滚 = `git rm scripts/uat-network-resilience.sh scripts/degradation-regression.sh`
- **如果脚本异常退出未清网络规则，手工恢复**：
  ```bash
  sudo tc qdisc del dev eth0 root netem 2>/dev/null || true
  sudo iptables -D OUTPUT -d <host_ip> -j DROP 2>/dev/null || true
  ```
- 如果 degradation-regression 破坏后未 restore：
  ```bash
  docker restart <ctr>    # 最暴力兜底，恢复 entrypoint 重建三层挂载
  ```
- 无仓库持久化数据污染；benchmarks/ 下 JSON/MD 可 `rm` 清理
</rollback>

<output>
After completion, create `.planning/phases/35-e2e/35-02-SUMMARY.md` documenting:
- 两个脚本最终行数 + 关键函数（disrupt_start/disrupt_stop/check_alive_loop/assert_code_present/restore_all）
- 烟测命令与 `--help` 输出
- 与 errcodes 注册表交叉断言（列出脚本引用的所有 MOUNT_* 码 + 确认均存在）
- T-35-02 威胁矩阵每条的实际落地位置（脚本行号）
- Deferred 到 Plan 05 真机签字的项（30s / 2min 人工观察确认）
</output>
