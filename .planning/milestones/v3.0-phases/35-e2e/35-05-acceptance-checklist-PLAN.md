---
phase: 35-e2e
plan: 05
type: execute
wave: 2
depends_on:
  - "35-01"
  - "35-02"
  - "35-03"
  - "35-04"
autonomous: false
files_modified:
  - scripts/v3-acceptance-checklist.sh
  - docs/runbooks/v3-acceptance-procedure.md
requirements_addressed:
  # 本 plan 主交付的 14 个 ID（v3.0 验收闸门 + 真机签字范围）
  - BASE-01
  - BASE-02
  - BASE-03
  - BASE-04
  - REQ-F1-B
  - REQ-F1-C
  - REQ-F3-B
  - REQ-F3-C
  - REQ-F3-D
  - REQ-F4-A
  - REQ-F5-D
  - M5
  - M13
  - C6
  # I-2 注释：脚本对其余 23 项 (REQ-F1-A / F1-D / F1-E / F2-A/B/C / F3-A / F4-B/C / F5-A/B/C / F6-A/B/C/D / F7-A/B/C/D / F8-A/B/C)
  # 做"交叉回归 + 报告枚举"，主交付 phase 见 .planning/REQUIREMENTS.md Traceability Matrix；
  # 本字段仅列主交付以避免与历史 phase 重复 ownership。
threat_model_severity: medium
must_haves:
  truths:
    - "scripts/v3-acceptance-checklist.sh --dry-run --track=all 枚举 30 条 functional REQ + 4 条 BASE ≥ 34 行项目"
    - "脚本支持 --track={base,req-f1,req-f3,req-f4,req-f5,req-f6,req-f7,req-f8,pitfalls,all} 分段运行"
    - "APFS 场景（M5）明确创建 Foo.txt + foo.txt 冲突文件并断言双向同步后两文件并存无覆盖"
    - "Ubuntu 25.04 真机场景（C6）显式调用 deploy/scripts/host-preflight.sh + scripts/verify-fuse-compat.sh"
    - "docs/runbooks/v3-acceptance-procedure.md 含三段执行流程（CI / macOS 真机 / Ubuntu 25.04 真机） + 签字模板"
    - "脚本末尾生成 docs/runbooks/v3-acceptance-report-$(date +%Y%m%d).md + JSON 版"
    - "人工签字 checkpoint：关键场景（APFS / 2min 拔网 / AppArmor 真机）需 user 在报告中标注机器信息后回复 approve"
  artifacts:
    - path: "scripts/v3-acceptance-checklist.sh"
      provides: "v3.0 验收主脚本，聚合 30 REQ + 4 BASE，三环境感知，PASS/FAIL/SKIP 三值汇总"
      min_lines: 250
    - path: "docs/runbooks/v3-acceptance-procedure.md"
      provides: "真机执行步骤 + 签字流程（三环境）+ 报告模板"
      min_lines: 140
  key_links:
    - from: "scripts/v3-acceptance-checklist.sh"
      to: "scripts/perf-benchmark.sh / cold-start-benchmark.sh / uat-network-resilience.sh / degradation-regression.sh / verify-fuse-compat.sh / ci-doctor-grep.sh / verify-managed-image.sh / deploy/scripts/host-preflight.sh"
      via: "每 track 的 check_item 函数调用底层脚本，聚合其退出码到 PASS/FAIL/SKIP"
      pattern: "bash scripts/(perf-benchmark|cold-start-benchmark|uat-network-resilience|degradation-regression|verify-fuse-compat|ci-doctor-grep|verify-managed-image)\\.sh"
    - from: "scripts/v3-acceptance-checklist.sh"
      to: ".planning/REQUIREMENTS.md"
      via: "REQ-ID 列表硬编码在脚本开头常量，与 REQUIREMENTS.md 对比必须匹配"
      pattern: "REQ-F1-A|REQ-F1-B|...|BASE-04"
    - from: "docs/runbooks/v3-acceptance-procedure.md"
      to: "scripts/v3-acceptance-checklist.sh"
      via: "手册 §2 步骤 1 命令：bash scripts/v3-acceptance-checklist.sh --track=all --output=..."
      pattern: "bash scripts/v3-acceptance-checklist\\.sh"
---

<objective>
交付 v3.0 验收闭环：一条 `bash scripts/v3-acceptance-checklist.sh` 命令在目标环境遍历 **30 条 functional REQ + 4 条 BASE** 自动产出 JSON+MD 报告，关键真机场景（APFS 冲突 / 2min 拔网 / Ubuntu 25.04 AppArmor 三路 FUSE）通过 checkpoint:human-verify 签字收尾。
Purpose: 对齐 Phase Success Criteria **#5 / #6 / #8**（APFS case-insensitive 双向同步无数据丢失 / Ubuntu 真机 AppArmor override 后三路并发 FUSE 全部成功 / 34 条 REQ 签字通过），把 Phase 29-34 的所有 user-visible 能力统一进一条可回归脚本。
Output: 一个聚合脚本（≥ 250 行）+ 一份真机执行手册（≥ 140 行）+ 一次人工 checkpoint 签字。**本 plan 不修改任何 Go 代码或产品行为**，只消费 Wave 0（Plan 01-04）的交付件。
</objective>

<execution_context>
@.cursor/get-shit-done/workflows/execute-plan.md
@.cursor/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/PROJECT.md
@.planning/REQUIREMENTS.md
@.planning/ROADMAP.md
@.planning/phases/35-e2e/35-CONTEXT.md
@.planning/phases/35-e2e/35-RESEARCH.md
@.planning/phases/35-e2e/35-PATTERNS.md

<!-- Wave 0 交付（预期在 Wave 2 启动时已存在） -->
@scripts/perf-benchmark.sh
@scripts/cold-start-benchmark.sh
@scripts/uat-network-resilience.sh
@scripts/degradation-regression.sh
@docs/runbooks/v3-upgrade-guide.md
@docs/runbooks/v3-apparmor-deployment.md
@docs/runbooks/v3-doctor-troubleshoot.md
@docs/runbooks/v3-persistent-volume-lifecycle.md
@docs/runbooks/v3-error-code-index.md
@.github/workflows/ci.yml

<!-- 既有可复用脚本 -->
@scripts/verify-fuse-compat.sh
@scripts/ci-doctor-grep.sh
@scripts/verify-managed-image.sh
@deploy/scripts/host-preflight.sh
</context>

<interfaces>
<!-- REQ-ID 清单（硬编码常量；必须与 REQUIREMENTS.md L170-203 Traceability 表完全一致） -->

```bash
REQS_F1=(REQ-F1-A REQ-F1-B REQ-F1-C REQ-F1-D REQ-F1-E)
REQS_F2=(REQ-F2-A REQ-F2-B REQ-F2-C)
REQS_F3=(REQ-F3-A REQ-F3-B REQ-F3-C REQ-F3-D)
REQS_F4=(REQ-F4-A REQ-F4-B REQ-F4-C)
REQS_F5=(REQ-F5-A REQ-F5-B REQ-F5-C REQ-F5-D)
REQS_F6=(REQ-F6-A REQ-F6-B REQ-F6-C REQ-F6-D)
REQS_F7=(REQ-F7-A REQ-F7-B REQ-F7-C REQ-F7-D)
REQS_F8=(REQ-F8-A REQ-F8-B REQ-F8-C)
BASES=(BASE-01 BASE-02 BASE-03 BASE-04)
PITFALLS=(M5 M13 C6)
# 合计：5+3+4+3+4+4+4+3 = 30 functional REQ + 4 BASE + 3 pitfalls
```

<!-- PATTERNS.md Pattern B — 环境感知 skip 三层闸门（is_ci / is_macos / is_ubuntu25） -->

<!-- PATTERNS.md Pattern A — JSON + MD 双输出 + P50/P99 抽取 -->

<!-- 产物命名（CONTEXT L70） -->
docs/runbooks/v3-acceptance-report-YYYYMMDD.md        # 人类可读
.planning/phases/35-e2e/benchmarks/v3-acceptance-YYYYMMDD-HHMMSS.json  # 机器可读
</interfaces>

<tasks>

<task type="execute" id="35-05-T1">
  <name>Task 1: v3-acceptance-checklist.sh — 聚合 30 REQ + 4 BASE + 3 pitfalls 的主脚本</name>
  <files>scripts/v3-acceptance-checklist.sh</files>
  <read_first>
    - scripts/v3-acceptance-checklist.sh（**新文件**）
    - scripts/ci-doctor-grep.sh（整份 — Pattern A + Pattern D）
    - scripts/verify-fuse-compat.sh（整份 — 阶段结构 + 4 函数 + 汇总输出）
    - deploy/scripts/host-preflight.sh（L11-73 — Pattern B 三层闸门，is_ubuntu25 解析算法）
    - .planning/REQUIREMENTS.md（L170-203 Traceability 表 — REQ-ID 硬编码校准）
    - .planning/phases/35-e2e/35-PATTERNS.md Pattern B / Pattern L / Pattern M / Pattern N
    - .planning/phases/35-e2e/35-CONTEXT.md §"验收清单" + §"签字流程"
    - **Wave 0 已 ship 的脚本**：scripts/perf-benchmark.sh / cold-start-benchmark.sh / uat-network-resilience.sh / degradation-regression.sh（**假定 Plan 01/02 已合并；若尚未落盘则在 SKIP 时显式提示 "上游 Plan 尚未完成"**）
  </read_first>
  <action>
创建 `scripts/v3-acceptance-checklist.sh`（≥ 250 行）。

### 骨架（PATTERNS Skeleton 1-5）

```bash
#!/usr/bin/env bash
# scripts/v3-acceptance-checklist.sh — Phase 35 master checklist
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_ROOT"

PASS_COUNT=0; FAIL_COUNT=0; SKIP_COUNT=0; WARN_COUNT=0
pass() { echo "[PASS]  $1"; PASS_COUNT=$((PASS_COUNT+1)); }
fail() { echo "[FAIL]  $1"; FAIL_COUNT=$((FAIL_COUNT+1)); }
skip() { echo "[SKIP]  $1"; SKIP_COUNT=$((SKIP_COUNT+1)); }
warn() { echo "[WARN]  $1"; WARN_COUNT=$((WARN_COUNT+1)); }
info() { echo "[INFO]  $1"; }
```

### CLI flags

- `--track={base|req-f1|req-f3|req-f4|req-f5|req-f6|req-f7|req-f8|pitfalls|all}`（默认 `all`；Pattern B 分段）
- `--confirm-destructive`（**默认 false**；当 `--track` 命中 `pitfalls` 或 `all` 且会触发 M13 三层破坏时必须透传给 `degradation-regression.sh`；缺省时 M13 项被记为 SKIP 并在报告中明确提示"需 --confirm-destructive 显式 opt-in 才执行破坏链"，T-35-05-03 落地）
- `--env={ci|macos|ubuntu25|auto}`（默认 `auto` — 用 Pattern B 的 is_ci/is_macos/is_ubuntu25 探测）
- `--target-container=NAME`（用于 Plan 02 UAT track）
- `--host-ip=IP`（iptables fallback 用）
- `--dry-run`（只枚举、不执行）
- `--output-dir=DIR`（默认 `$PROJECT_ROOT/.planning/phases/35-e2e/benchmarks`）
- `--report-md=PATH`（默认 `$PROJECT_ROOT/docs/runbooks/v3-acceptance-report-$(date +%Y%m%d).md`）
- `--help`

### 环境探测函数（PATTERNS Pattern B L160-194 照抄）

```bash
is_ci()       { [[ "${CI:-}" == "true" ]]; }
is_macos()    { [[ "$(uname -s)" == "Darwin" ]]; }
is_linux()    { [[ "$(uname -s)" == "Linux" ]]; }
is_ubuntu25() {
  [ -f /etc/os-release ] || return 1
  . /etc/os-release
  [ "${ID:-}" = "ubuntu" ] && {
    ubuntu_major="${VERSION_ID%%.*}"
    ver_rest="${VERSION_ID#*.}"
    ubuntu_minor="${ver_rest%%.*}"
    [ "${ubuntu_major}" -gt 25 ] || { [ "${ubuntu_major}" -eq 25 ] && [ "${ubuntu_minor:-0}" -ge 4 ]; }
  }
}
has_tc()       { command -v tc >/dev/null 2>&1; }
has_root_net() { [[ $EUID -eq 0 ]] || sudo -n tc qdisc show dev lo &>/dev/null; }
has_docker()   { command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; }
has_apfs()     { [[ "$(uname -s)" == "Darwin" ]] && diskutil info / 2>/dev/null | grep -q 'APFS'; }
```

### check_item 骨架（Pattern from CONTEXT L88 + 底层脚本调用）

```bash
check_item() {
  local id="$1" track="$2" desc="$3" runner_fn="$4"
  echo ""
  info "───── $id ($track): $desc ─────"
  if ! $runner_fn; then
    return   # runner_fn 自行调用 pass/fail/skip；非零返回视为 fail
  fi
}
```

### 每条 REQ 的 runner_fn（30 + 4 + 3 条）

**重要**：每条 REQ 调用对应底层脚本，将其退出码映射到 PASS/FAIL/SKIP：

| REQ-ID | runner_fn 策略 | 底层 |
|--------|----------------|------|
| BASE-01 | `bash scripts/perf-benchmark.sh --ci-mode --runs=10 --warmup=1`，退出码 0→pass, 1→fail, 2→skip | Plan 01 |
| BASE-02 | `bash scripts/cold-start-benchmark.sh --attempts=5 --threshold-seconds=8 --min-pass=4`，同样映射 | Plan 01 |
| BASE-03 | `bash scripts/uat-network-resilience.sh --scenario=30s`（+ 提示 2min 场景需 signoff）| Plan 02 |
| BASE-04 | `bash scripts/verify-managed-image.sh && size 检查 700MB` 同 Plan 04 job | Phase 29 / Plan 04 |
| REQ-F1-A | `docker exec $ctr mount | grep -q 'type fuse.mergerfs.*on /workspace'` + `docker exec $ctr readlink /workspace` 单一视图 | 无脚本，直接命令 |
| REQ-F1-B | reuse BASE-02 的 `stderr_progress_matches=true` | 复用 |
| REQ-F1-C | reuse BASE-01 的 P50 ratio ≤ 1.5 | 复用 |
| REQ-F1-D | `docker exec $ctr cloud-claude --mount-mode=auto ...` 触发 >50MB 候选目录场景；断言 stderr 含 `MOUNT_MUTAGEN_WHITELIST_REJECT` | 手工场景 |
| REQ-F1-E | `docker exec $ctr cloud-claude sync conflicts` 输出格式符合 `⚠ 有 N 个文件同步冲突` | 手工 + doctor |
| REQ-F2-A | `cloud-claude --mount-mode=full|mutagen-only|sshfs-only|auto` 四档分别启动后 banner 显示对应标签 | 手工枚举 |
| REQ-F2-B | reuse `degradation-regression.sh --layer=all`（Plan 02） | 复用 |
| REQ-F2-C | `NO_COLOR=1 cloud-claude ...` banner 无 ANSI；非 NO_COLOR 有 ANSI | 手工 |
| REQ-F3-A | `cloud-claude --server-alive-interval=10 ...` 启动立即报错 `SESSION_KEEPALIVE_TOO_AGGRESSIVE` | 命令 + grep |
| REQ-F3-B | reuse uat-network-resilience `--scenario=10s` 的 `token_replayed=true` | 复用 |
| REQ-F3-C | reuse uat-network-resilience `--scenario=2min` 的 `final_failure_prompt_seen=true` | 复用 |
| REQ-F3-D | reuse uat-network-resilience `--scenario=2min` 的 `backoff_marks_seen.length>=3` | 复用 |
| REQ-F4-A | reuse uat-network-resilience 30s 场景的 `pgrep_survived_full_duration=true` | 复用 |
| REQ-F4-B | `docker exec $ctr tmux ls` + `cloud-claude sessions ls` 列出 ≥ 1 session | 命令 |
| REQ-F4-C | 临时破坏容器 tmux 二进制权限 → cloud-claude banner 含 `容器内 tmux 不可用` | 手工 |
| REQ-F5-A | 两端同时 cloud-claude → 第二端无 "被踢" 错误 | 手工（CI SKIP） |
| REQ-F5-B | 第二端 banner 含 `另 1 个会话正在共享` | 手工 |
| REQ-F5-C | 测 `--new-session` + `--take-over` | 手工 |
| REQ-F5-D | 第二端 cloud-claude 日志含 `SESSION_SYNC_LOCKED` | 手工 |
| REQ-F6-A | `cloud-claude doctor --json | jq '.checks | group_by(.domain) | length'` == 5 | 直接命令 |
| REQ-F6-B | reuse `bash scripts/ci-doctor-grep.sh $cloud_claude_bin` | 复用 |
| REQ-F6-C | `cloud-claude doctor --fix --yes` 能修复 ≥ 5 种故障（列出 fix 成功的 check 数） | 命令 |
| REQ-F6-D | `cloud-claude doctor --verbose`、`--json`、`NO_COLOR=1` + 退出码 ∈ {0,1,2} | 命令枚举 |
| REQ-F7-A | `docker volume ls --filter label=com.cloud-cli-proxy.account_id=<id>` 返回唯一匹配 | 命令 |
| REQ-F7-B | 容器删除后 `docker volume ls` 仍可见 `claude-state-*`，重建容器后 `~/.claude/.credentials.json` 仍在 | 手工 |
| REQ-F7-C | 过期 OAuth 场景 → cloud-claude 连接前 stderr 含 `NET_OAUTH_EXPIRED` + 中文 | 手工 |
| REQ-F7-D | `curl -X DELETE .../v1/admin/claude-accounts/<id>` 后 `docker volume ls` 该 volume 不存在 | 手工 |
| REQ-F8-A/B | `cloud-claude explain --all \| jq length ≥ 42` + `errcodes` 注册表完整性（遍历代码 diff） | 命令 + diff |
| REQ-F8-C | `cloud-claude explain MOUNT_MUTAGEN_VERSION_SKEW` 返回非空且含"建议" | 命令 |
| M5 (APFS) | **仅 macos 真机**：创建 `/workspace/Foo.txt`（"A"）+ `/workspace/foo.txt`（"B"）；Mutagen 同步后断言两文件内容各自保留 | 手工（ci/linux SKIP） |
| M13 | reuse degradation-regression.sh，**主脚本调用时必须透传 `--confirm-destructive`**（Plan 02 T-35-02-04 opt-in 闸门），调用形如 `bash scripts/degradation-regression.sh --layer=all --target-container="$TARGET_CONTAINER" --confirm-destructive`；缺省 `--confirm-destructive` 时 degradation-regression.sh 走 dry-run 提示退出，主脚本将其判为 SKIP 而非 FAIL | 复用 Plan 02 |
| C6 | **仅 ubuntu25 真机**：`bash deploy/scripts/host-preflight.sh && bash scripts/verify-fuse-compat.sh`，退出码 0 + 三路 mount 全 PASS | 复用 |

**APFS 场景（M5）runner 细节**：
```bash
runner_m5_apfs() {
  is_macos && has_apfs || { skip "M5 APFS: 非 macOS APFS 环境"; return; }
  has_docker || { skip "M5 APFS: 缺 docker"; return; }
  # 前置 cloud-claude 已连接的容器（用户提供 --target-container）
  local ctr="${TARGET_CONTAINER:?}"
  local tmp_local; tmp_local=$(mktemp -d)
  printf 'A-upper' > "$tmp_local/Foo.txt"
  printf 'B-lower' > "$tmp_local/foo.txt" 2>/dev/null || {
    # APFS 默认 case-insensitive 时，第 2 条会覆盖；切到 --mode=two-way-resolved 后应保留
    warn "M5 APFS: 本地 APFS case-insensitive 覆盖行为触发，观测 Mutagen 冲突策略"
  }
  # 推同步（假设 cloud-claude 默认同步 /workspace）
  sleep 10    # 给 Mutagen 同步窗口
  local remote_foo_upper remote_foo_lower
  remote_foo_upper=$(docker exec "$ctr" cat /workspace/Foo.txt 2>/dev/null || echo "")
  remote_foo_lower=$(docker exec "$ctr" cat /workspace/foo.txt 2>/dev/null || echo "")
  if [[ -n "$remote_foo_upper" && -n "$remote_foo_lower" && "$remote_foo_upper" != "$remote_foo_lower" ]]; then
    pass "M5 APFS: Foo.txt / foo.txt 双向同步保留，无数据丢失"
  else
    fail "M5 APFS: Foo.txt='$remote_foo_upper' / foo.txt='$remote_foo_lower' 至少一侧丢失"
  fi
  rm -rf "$tmp_local"
}
```

### 汇总 + 报告生成（PATTERNS Pattern A + L）

```bash
summarize() {
  local total=$((PASS_COUNT + FAIL_COUNT + SKIP_COUNT + WARN_COUNT))
  echo ""
  echo "========================================"
  echo "验收汇总: $PASS_COUNT PASS / $FAIL_COUNT FAIL / $SKIP_COUNT SKIP / $WARN_COUNT WARN (total=$total)"
  echo "========================================"
  # 写 JSON / MD 到 --output-dir + --report-md
  ...
  [ "$FAIL_COUNT" -eq 0 ]
}
```

### 退出码

- `0` = 无 FAIL（至少一条 SKIP 允许通过 — 真机环境不全时）
- `1` = 至少一条 FAIL
- `2` = 环境完全不适配（0 pass + 全 skip）

### 报告模板（末端写入）

```markdown
# v3.0 验收报告 — $(hostname) $(date -u +%FT%TZ)

> 执行环境：$ENV_DETECTED / 内核：$(uname -a) / docker: $(docker --version)

## 汇总
| | PASS | FAIL | SKIP | WARN | 总计 |
|-|------|------|------|------|------|
| | $PASS_COUNT | $FAIL_COUNT | $SKIP_COUNT | $WARN_COUNT | $total |

## 明细（按 track）
（表格：REQ-ID / 状态 / 证据文件路径 / 裁决）

## 关键场景签字（人工）
- [ ] M5 APFS case-insensitive：签字人 / 日期
- [ ] 2min 拔网自动重连：签字人 / 日期
- [ ] Ubuntu 25.04 AppArmor 三路 FUSE：签字人 / 日期
```
  </action>
  <acceptance_criteria>
    - `bash -n scripts/v3-acceptance-checklist.sh` 退出码 0
    - `bash scripts/v3-acceptance-checklist.sh --help` 退出码 0 且含 `--track=` 字样
    - `bash scripts/v3-acceptance-checklist.sh --track=all --dry-run` 退出码 ≤ 2 且 stdout 至少出现 34 个枚举行（`bash scripts/v3-acceptance-checklist.sh --track=all --dry-run | grep -cE '───── (REQ-F[1-8]-[A-E]|BASE-0[1-4]|M5|M13|C6) '` ≥ 34；I-1 收紧 regex，避免 `M|C` 单字母误命中 MOUNT_* / Cleanup 等字样）
    - `wc -l < scripts/v3-acceptance-checklist.sh` ≥ 250
    - REQ-ID 清单完整（脚本内硬编码）：`for id in REQ-F1-A REQ-F1-B REQ-F1-C REQ-F1-D REQ-F1-E REQ-F2-A REQ-F2-B REQ-F2-C REQ-F3-A REQ-F3-B REQ-F3-C REQ-F3-D REQ-F4-A REQ-F4-B REQ-F4-C REQ-F5-A REQ-F5-B REQ-F5-C REQ-F5-D REQ-F6-A REQ-F6-B REQ-F6-C REQ-F6-D REQ-F7-A REQ-F7-B REQ-F7-C REQ-F7-D REQ-F8-A REQ-F8-B REQ-F8-C BASE-01 BASE-02 BASE-03 BASE-04 M5 M13 C6; do grep -qF "$id" scripts/v3-acceptance-checklist.sh || { echo "missing $id"; exit 1; }; done; echo ok`（最终 echo ok）
    - track 白名单：`grep -qE '(base\|req-f1\|req-f3\|req-f4\|req-f5\|req-f6\|req-f7\|req-f8\|pitfalls\|all)' scripts/v3-acceptance-checklist.sh` 退出码 0
    - `grep -qE 'is_macos|is_ubuntu25|is_ci' scripts/v3-acceptance-checklist.sh` 退出码 0（三层闸门）
    - `grep -qF 'scripts/perf-benchmark.sh' scripts/v3-acceptance-checklist.sh` 退出码 0
    - `grep -qF 'scripts/cold-start-benchmark.sh' scripts/v3-acceptance-checklist.sh` 退出码 0
    - `grep -qF 'scripts/uat-network-resilience.sh' scripts/v3-acceptance-checklist.sh` 退出码 0
    - `grep -qF 'scripts/degradation-regression.sh' scripts/v3-acceptance-checklist.sh` 退出码 0
    - `grep -qF 'scripts/verify-managed-image.sh' scripts/v3-acceptance-checklist.sh` 退出码 0
    - `grep -qF 'scripts/verify-fuse-compat.sh' scripts/v3-acceptance-checklist.sh` 退出码 0
    - `grep -qF 'deploy/scripts/host-preflight.sh' scripts/v3-acceptance-checklist.sh` 退出码 0
    - `grep -qF 'Foo.txt' scripts/v3-acceptance-checklist.sh` 退出码 0（M5 APFS 实证）
    - `grep -qF 'foo.txt' scripts/v3-acceptance-checklist.sh` 退出码 0
    - `grep -qF 'v3-acceptance-report-' scripts/v3-acceptance-checklist.sh` 退出码 0（报告命名含日期戳）
    - `grep -qF 'schema_version' scripts/v3-acceptance-checklist.sh` 退出码 0（JSON schema）
    - `grep -qE 'confirm-destructive' scripts/v3-acceptance-checklist.sh` 退出码 0（W-2 联动：M13 破坏链必须 opt-in 透传，T-35-05-03 落地）
    - `grep -qE 'degradation-regression\.sh.*--confirm-destructive' scripts/v3-acceptance-checklist.sh` 退出码 0（透传调用形态固化）
  </acceptance_criteria>
  <done>主脚本能一条命令枚举 34 项 REQ+BASE + 3 pitfall，支持环境感知 SKIP，退出码对齐 0/1/2，人工场景在 MD 报告生成待签字栏。</done>
</task>

<task type="execute" id="35-05-T2">
  <name>Task 2: v3-acceptance-procedure.md — 真机执行手册 + 签字流程</name>
  <files>docs/runbooks/v3-acceptance-procedure.md</files>
  <read_first>
    - docs/runbooks/v3-acceptance-procedure.md（**新文件**）
    - docs/runbooks/v3-claude-state-volumes.md（Pattern G 样板）
    - .planning/phases/35-e2e/35-CONTEXT.md §"真机环境矩阵" + §"签字流程"（全文）
    - .planning/phases/35-e2e/35-PATTERNS.md Pattern G（手册头部 + 章节骨架）
    - scripts/v3-acceptance-checklist.sh（**Task 1 输出** — 本 Task 的权威引用对象）
  </read_first>
  <action>
创建 `docs/runbooks/v3-acceptance-procedure.md`（≥ 140 行）。

章节结构（Pattern G 头部 + 7 章）：

1. **背景** — v3.0 验收 phase 目标 + 三环境矩阵（CI / macOS APFS / Ubuntu 25.04 AppArmor）
2. **前置条件** —
   - CI：`.github/workflows/ci.yml` 已含 perf-benchmark + image-size-regression（Plan 04）
   - macOS：Apple Silicon、APFS case-insensitive、Docker Desktop、已 `cloud-claude login`
   - Ubuntu 25.04：裸机或云主机、内核 ≥ 6.12、AppArmor override 已部署（引用 `v3-apparmor-deployment.md`）
3. **执行流程**
   - **3.1 CI 环境（PR 触发自动）**：列出 GH Actions 两个 job 的期望 ✅；手工触发 `gh workflow run ci.yml`
   - **3.2 macOS APFS 真机**：
     ```bash
     # 3.2 步骤 1：启动 cloud-claude 到目标容器
     cloud-claude --mount-mode=auto
     # 3.2 步骤 2：在另一个终端跑验收
     bash scripts/v3-acceptance-checklist.sh \
       --track=all \
       --env=macos \
       --target-container=<ctr> \
       --report-md=docs/runbooks/v3-acceptance-report-$(date +%Y%m%d).md
     ```
     预期：`pitfalls` track 含 M5 APFS 场景 PASS；其它 `REQ-F5-*` / `REQ-F7-*` 需手工观察签字
   - **3.3 Ubuntu 25.04 真机**：
     ```bash
     # 3.3 步骤 1：AppArmor override 部署（参照 v3-apparmor-deployment.md）
     sudo bash deploy/scripts/host-preflight.sh
     # 3.3 步骤 2：验收
     bash scripts/v3-acceptance-checklist.sh --track=all --env=ubuntu25 --target-container=<ctr>
     # 3.3 步骤 3：三路 FUSE 专项
     bash scripts/verify-fuse-compat.sh
     ```
4. **签字栏模板**（CONTEXT L66-70 锁定）—

   ```markdown
   ### 签字（人工关键场景）

   | 场景 | 机器信息 | 执行时间 | 签字人 | 证据 |
   |------|---------|---------|--------|------|
   | M5 APFS case-insensitive 双向同步 | hostname / OS 版本 / CPU | YYYY-MM-DD HH:MM | @user | [v3-acceptance-report-YYYYMMDD.md#M5](...) |
   | 2min 拔网自动重连（BASE-03） | hostname / iface / 断网方式 | YYYY-MM-DD HH:MM | @user | [v3-acceptance-report-YYYYMMDD.md#BASE-03](...) |
   | Ubuntu 25.04 AppArmor + 三路 FUSE（C6） | hostname / kernel / ubuntu_version | YYYY-MM-DD HH:MM | @user | [v3-acceptance-report-YYYYMMDD.md#C6](...) |
   ```

   **签字流程（CONTEXT L65-69 锁定）**：
   1. 脚本在目标环境执行生成报告
   2. 报告附于 Phase 35 PR 中
   3. PR 合并视为"签字通过"
   4. 真机环境需在报告中显式标注机器信息（OS 版本、硬件型号、执行时间）

5. **报告归档**
   - `.planning/phases/35-e2e/benchmarks/*.json` → 机器可读历史
   - `docs/runbooks/v3-acceptance-report-YYYYMMDD.md` → 人类可读 + 签字
   - PR description 贴 summary 表（PASS/FAIL/SKIP 数字）

6. **回归触发（Rollback trigger）** —
   - 任一 BASE 持续失败 2 个版本 → 停止 release、回流对应 phase
   - 任一 pitfall 场景失败 → 触发 `/gsd-plan-phase XX --gaps` 补丁 phase
7. **快速诊断命令** — 3-5 条（`bash scripts/v3-acceptance-checklist.sh --track=base --dry-run` / `cat docs/runbooks/v3-acceptance-report-*.md | head -30` 等）
8. **参考** — 本 plan 全部交付件路径 + REQUIREMENTS.md

### 头部 Pattern G（必须）
```markdown
# v3.0 验收流程手册（v3.0+）

> 适用版本：v3.0 起；对应阶段 Phase 35（e2e）
> 关联需求：BASE-01..04 / 30 条 functional REQ / M5 / M13 / C6
```
  </action>
  <acceptance_criteria>
    - `test -f docs/runbooks/v3-acceptance-procedure.md` 退出码 0
    - `wc -l < docs/runbooks/v3-acceptance-procedure.md` ≥ 140
    - `grep -c '^## ' docs/runbooks/v3-acceptance-procedure.md` ≥ 7
    - `grep -qF '适用版本：v3.0' docs/runbooks/v3-acceptance-procedure.md` 退出码 0
    - `grep -qF 'bash scripts/v3-acceptance-checklist.sh' docs/runbooks/v3-acceptance-procedure.md` 退出码 0
    - `grep -qF 'M5 APFS' docs/runbooks/v3-acceptance-procedure.md` 退出码 0
    - `grep -qF 'Ubuntu 25.04' docs/runbooks/v3-acceptance-procedure.md` 退出码 0
    - `grep -qF 'AppArmor' docs/runbooks/v3-acceptance-procedure.md` 退出码 0
    - `grep -qF 'bash deploy/scripts/host-preflight.sh' docs/runbooks/v3-acceptance-procedure.md` 退出码 0
    - `grep -qF 'bash scripts/verify-fuse-compat.sh' docs/runbooks/v3-acceptance-procedure.md` 退出码 0
    - `grep -qF '签字' docs/runbooks/v3-acceptance-procedure.md` 退出码 0
    - `grep -qF 'v3-acceptance-report-' docs/runbooks/v3-acceptance-procedure.md` 退出码 0
    - `grep -qF '快速诊断命令' docs/runbooks/v3-acceptance-procedure.md` 退出码 0
    - `grep -cE '### 签字|签字栏|签字流程' docs/runbooks/v3-acceptance-procedure.md` ≥ 1（签字小节存在）
  </acceptance_criteria>
  <done>真机执行手册完整，含三环境执行步骤 + 签字栏模板 + 回归触发条件。</done>
</task>

<task type="checkpoint:human-verify" id="35-05-T3" gate="blocking">
  <name>Task 3: 真机验收签字 checkpoint（最终人工确认）</name>
  <what-built>
    Wave 0 的 6 个脚本（Plan 01 × 3 + Plan 02 × 2 + Plan 04 ci.yml）+ 5 份运维手册（Plan 03）+ Wave 1 的 master checklist（Task 1）+ 真机手册（Task 2）全部落盘，且各自 acceptance_criteria 均通过。
  </what-built>
  <how-to-verify>
**执行步骤（按顺序）**：

1. 干跑主脚本，确认枚举完整（CI / 本机均可）：
   ```bash
   bash scripts/v3-acceptance-checklist.sh --track=all --dry-run \
     | grep -cE '───── (REQ-|BASE-|M|C)'    # 期望 ≥ 34
   ```

2. CI 回归（任何最近 PR）：
   - GitHub Actions 的 `perf-benchmark` + `image-size-regression` 两 job 绿
   - artifact `perf-bench-<sha>` 含 `.planning/phases/35-e2e/benchmarks/bench-*.json`

3. 真机执行（至少一台 macOS 或 Ubuntu 25.04）：
   - 按 `docs/runbooks/v3-acceptance-procedure.md` §3.2 或 §3.3 操作
   - 生成 `docs/runbooks/v3-acceptance-report-YYYYMMDD.md`
   - 报告中 FAIL 计数必须为 0；SKIP 仅限环境不可达的项

4. 关键场景签字（必须三项全签）：
   - [ ] M5 APFS 冲突：`Foo.txt` / `foo.txt` 内容各自保留
   - [ ] BASE-03 / REQ-F3-C 2min 拔网：自动重连后 tmux session 可用
   - [ ] C6 Ubuntu 25.04：AppArmor override 后 `bash scripts/verify-fuse-compat.sh` 三路全 PASS

5. 全部完成后在聊天里回复 `approved` + 贴签字报告路径；若任何一条 FAIL 则描述现象并给出 `/gsd-plan-phase 35 --gaps` 候选。

**人工判定口径（口径统一，不主观）**：
- "无感知" = `pgrep_survived_full_duration=true` AND `buffer_diff_lines=0` AND `token_replayed=true`（三项全 true，来自 Plan 02 的 JSON 字段）
- "三段式进度" = `stderr_progress_matches=true`（Plan 01 `cold-start-benchmark.sh` JSON 字段）
- "双向无数据丢失" = `cat /workspace/Foo.txt` 与 `cat /workspace/foo.txt` 均非空且内容不同

**环境信息收集（签字必需）**：
```bash
echo "hostname: $(hostname)"
echo "uname:    $(uname -a)"
echo "docker:   $(docker --version 2>/dev/null)"
echo "cloud-claude: $(cloud-claude --version 2>/dev/null)"
# macOS 补充：diskutil info / | grep -E 'Type|Encrypted'
# Ubuntu 补充：. /etc/os-release && echo "ubuntu: $VERSION_ID"
```
  </how-to-verify>
  <resume-signal>
回复 `approved` 代表已签字；或回复具体 FAIL 的 REQ-ID 列表 + 现象描述（触发 gap-closure 流程）。
  </resume-signal>
</task>

</tasks>

<verification>
```bash
# Task 1 / Task 2 自动断言
bash -n scripts/v3-acceptance-checklist.sh && echo "主脚本 syntax ok"
bash scripts/v3-acceptance-checklist.sh --help >/dev/null && echo "--help ok"
bash scripts/v3-acceptance-checklist.sh --track=all --dry-run > /tmp/checklist-dryrun.txt
# I-1: regex 收紧到精确 ID 形态，避免 `M|C` 单字母误命中 MOUNT_* / Cleanup
grep -cE '───── (REQ-F[1-8]-[A-E]|BASE-0[1-4]|M5|M13|C6) ' /tmp/checklist-dryrun.txt   # ≥ 34

# 全部 REQ-ID 在脚本中硬编码
for id in REQ-F1-A REQ-F1-B REQ-F1-C REQ-F1-D REQ-F1-E REQ-F2-A REQ-F2-B REQ-F2-C \
          REQ-F3-A REQ-F3-B REQ-F3-C REQ-F3-D REQ-F4-A REQ-F4-B REQ-F4-C \
          REQ-F5-A REQ-F5-B REQ-F5-C REQ-F5-D REQ-F6-A REQ-F6-B REQ-F6-C REQ-F6-D \
          REQ-F7-A REQ-F7-B REQ-F7-C REQ-F7-D REQ-F8-A REQ-F8-B REQ-F8-C \
          BASE-01 BASE-02 BASE-03 BASE-04 M5 M13 C6; do
  grep -qF "$id" scripts/v3-acceptance-checklist.sh || { echo "missing $id"; exit 1; }
done
echo "all 34 REQ/BASE + 3 pitfall 枚举就位"

# 手册
test -f docs/runbooks/v3-acceptance-procedure.md
grep -qF 'bash scripts/v3-acceptance-checklist.sh' docs/runbooks/v3-acceptance-procedure.md
grep -qF '签字' docs/runbooks/v3-acceptance-procedure.md

# ANSI 色码反向断言（Pattern M，统一约束 W-3 修复）
for f in scripts/v3-acceptance-checklist.sh; do
  ! grep -qP '\x1b\[' "$f" || { echo "ANSI escape detected in $f"; exit 1; }
done
echo "no-ANSI ok (主脚本为纯文本)"

# W-2 联动：M13 破坏链 opt-in 透传断言
grep -qE 'degradation-regression\.sh.*--confirm-destructive' scripts/v3-acceptance-checklist.sh \
  && echo "M13 透传 --confirm-destructive ok"
```
</verification>

<success_criteria>
- Phase SC #5 APFS：scripts/v3-acceptance-checklist.sh 对 M5 场景创建 `Foo.txt`+`foo.txt` 并断言两文件保留
- Phase SC #6 Ubuntu 25.04 AppArmor：脚本 + 手册都引导 `host-preflight.sh + verify-fuse-compat.sh` 三路 FUSE 回归
- Phase SC #8：30 条 functional REQ + 4 条 BASE 在脚本中硬编码 + 报告签字流程就位
- Task 3 checkpoint 签字收尾（user 回 `approved`）
- 本 plan 无代码 / 镜像 / 控制面改动，只消费 Wave 0 产物
</success_criteria>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| 主脚本 → 底层脚本 | `bash scripts/perf-benchmark.sh` 等依次调用；受 trust 的命令来自仓库本身 |
| 主脚本 → docker / tmux / sudo 命令 | 继承 Plan 02 相同 privilege 面；不新增 |
| 手册 → 读者 | copy-paste 命令必须含清晰 warning；`sudo apparmor_parser -r` 等操作风险明确标注 |
| 报告文件 → 仓库 | `docs/runbooks/v3-acceptance-report-*.md` 会被 PR review + 合入历史；含 hostname / 机器信息 |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-35-05-01 | Information Disclosure | 验收报告含 hostname / kernel / docker version（可能暴露内部基础设施） | mitigate | 手册 §"签字"明确：开源仓库场景下需在报告中脱敏 hostname（如 replace 为 `macbook-pro-<user>`）；私有仓库可保留原值；脚本输出 JSON 默认含完整 uname，README 说明"如需开源请手工脱敏" |
| T-35-05-02 | Tampering | --target-container 可注入 shell | mitigate | 与 Plan 02 一致：硬编码正则 `^[a-z0-9][a-z0-9_.-]*$`；所有 docker exec 参数加 `printf %q` 引号 |
| T-35-05-03 | Denial of Service | 运维在生产容器误跑 M13 破坏链 | mitigate | 主脚本 `--track=pitfalls` 默认要求 `--confirm-destructive` opt-in；手册 §"前置条件"明确"仅在 fixture 或 staging 容器执行"；默认使用 Plan 02 的 dry-run 模式展示将要执行的命令 |
| T-35-05-04 | Repudiation | 签字无审计 | mitigate | 报告文件归档 git（PR commit 即签名）；报告表头含 hostname + uname + 时间；checkpoint 要求 user 在回复中附报告路径 |
| T-35-05-05 | Elevation of Privilege | 本 plan 不引入新 privilege | accept | 复用 Wave 0 的 privilege 边界；本 plan 不触碰 sudo 行为 |
| T-35-05-06 | Tampering | 主脚本直接 exec 底层脚本 → 若底层被篡改，主脚本照跑 | mitigate | git commit 作为完整性锚；CI 对 scripts/ 下任一改动都跑 go-test + 后续 perf-benchmark；人工签字前 review report 所指 JSON 产物 |
</threat_model>

<rollback>
- 主脚本 + 手册为新建，回滚 = `git rm scripts/v3-acceptance-checklist.sh docs/runbooks/v3-acceptance-procedure.md`
- 验收报告（已生成）保留在 `.planning/phases/35-e2e/benchmarks/` 与 `docs/runbooks/`；回滚 plan 本身 ≠ 删除历史报告
- 无产品代码改动，无数据库 / 镜像 / 容器影响
- 如 checkpoint 未通过 → 走 `/gsd-plan-phase 35 --gaps` 针对具体 FAIL 项补丁
</rollback>

<output>
After completion, create `.planning/phases/35-e2e/35-05-SUMMARY.md` documenting:
- 主脚本行数、CLI flags、runner_fn 数量
- 真机手册行数、章节 TOC
- 真机执行报告路径（`docs/runbooks/v3-acceptance-report-YYYYMMDD.md`）链接与汇总数字
- 任何 FAIL 项对应的 gap-closure 跟进 phase 编号
- 签字人 / 机器信息 / 执行时间（三项齐备）
</output>
