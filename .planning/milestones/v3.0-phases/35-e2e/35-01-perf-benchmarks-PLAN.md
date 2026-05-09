---
phase: 35-e2e
plan: 01
type: execute
wave: 1
depends_on: []
autonomous: true
files_modified:
  - scripts/gen-bench-tree.sh
  - scripts/perf-benchmark.sh
  - scripts/cold-start-benchmark.sh
  - .planning/phases/35-e2e/benchmarks/.gitkeep
  - .planning/phases/35-e2e/benchmarks/README.md
requirements_addressed:
  - BASE-01
  - BASE-02
  - REQ-F1-B
  - REQ-F1-C
threat_model_severity: medium
must_haves:
  truths:
    - "执行 bash scripts/perf-benchmark.sh 后产出 JSON 与 markdown 双报告，JSON 的 results[].times[] 每档长度 = 10"
    - "对 mergerfs/sshfs-only 两档的 rg .` 与 ls -R P50 / P99 与本地档的比值被显式写入 markdown 表"
    - "执行 bash scripts/cold-start-benchmark.sh 后 JSON 中 attempts 数组长度 = 5 且含 stderr_progress_matches=true 字段"
    - "Wave 0 三个脚本均可 bash -n 通过；gen-bench-tree.sh 生成 10000 ± 50 个文件的 mono-repo 结构"
  artifacts:
    - path: "scripts/gen-bench-tree.sh"
      provides: "Synthetic 10k 文件树生成器（mono-repo 80/15/5 文件分布 + .git + node_modules）"
      min_lines: 80
    - path: "scripts/perf-benchmark.sh"
      provides: "hyperfine 驱动的三档基准（local / mergerfs / sshfs-only）+ P50/P99 + JSON/MD 双输出"
      min_lines: 120
    - path: "scripts/cold-start-benchmark.sh"
      provides: "BASE-02 首连 ≤ 8s 循环测量 + 三段式中文进度 stderr 断言"
      min_lines: 110
    - path: ".planning/phases/35-e2e/benchmarks/.gitkeep"
      provides: "产物目录存在性承诺（CONTEXT 第 115 行约定）"
    - path: ".planning/phases/35-e2e/benchmarks/README.md"
      provides: "产物目录自述（JSON schema / 文件命名 / 历史对比说明）"
      min_lines: 20
  key_links:
    - from: "scripts/perf-benchmark.sh"
      to: ".planning/phases/35-e2e/benchmarks/"
      via: "BENCH_DIR=\"${PROJECT_ROOT}/.planning/phases/35-e2e/benchmarks\" + mkdir -p"
      pattern: "BENCH_DIR=.*35-e2e/benchmarks"
    - from: "scripts/cold-start-benchmark.sh"
      to: "deploy/docker/managed-user/image.lock"
      via: "awk 解析 local_dev_image_name（与 verify-fuse-compat.sh 同风格）"
      pattern: "awk -F.*local_dev_image_name.*image.lock"
    - from: "scripts/perf-benchmark.sh"
      to: "scripts/gen-bench-tree.sh"
      via: "子调用生成 /tmp/bench-tree 后再跑 hyperfine"
      pattern: "gen-bench-tree.sh.*--count=10000"
---

<objective>
交付 BASE-01（10k 文件 `rg`/`ls -R` 1.5× 基线）与 BASE-02（首连 ≤ 8s + 三段式中文进度）两条性能基线的可自动化基准框架。
Purpose: 把 REQ-F1-B / REQ-F1-C 从"开发者主观感受"升级为"脚本可回归的数值比值"，让后续版本能够做性能回归检测。
Output: 三个 bash 脚本（`gen-bench-tree.sh` / `perf-benchmark.sh` / `cold-start-benchmark.sh`）+ 产物目录自述。所有脚本必须通过 `bash -n`，且在无 hyperfine / docker 环境下优雅 SKIP 而非 FAIL。
</objective>

<execution_context>
@.cursor/get-shit-done/workflows/execute-plan.md
@.cursor/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/PROJECT.md
@.planning/ROADMAP.md
@.planning/STATE.md
@.planning/phases/35-e2e/35-CONTEXT.md
@.planning/phases/35-e2e/35-RESEARCH.md
@.planning/phases/35-e2e/35-PATTERNS.md

<!-- 分析对象引用（PATTERNS.md Skeleton 1-5 + Pattern A/C）-->
@scripts/verify-fuse-compat.sh
@scripts/ci-doctor-grep.sh
@scripts/verify-managed-image.sh
@deploy/docker/managed-user/image.lock
</context>

<interfaces>
<!-- Phase 35 脚本唯一依赖的外部契约 -->

hyperfine v1.18+ JSON schema（RESEARCH §Open Questions #2 已答）：
```json
{
  "results": [
    {
      "command": "local",
      "mean": 0.0234,
      "stddev": 0.0012,
      "median": 0.0231,
      "user": 0.0210,
      "system": 0.0020,
      "min": 0.0215,
      "max": 0.0267,
      "times": [0.0215, 0.0221, 0.0229, ...]   // length = --runs 值
    }
  ]
}
```

P50/P99 由 jq 从 `.results[].times[]` 排序后按索引抽取（见 PATTERNS.md Pattern A 末段）。

image.lock 字段（awk `-F': '` 解析）：
```
local_dev_image_name: ghcr.io/zanel1u/cloud-cli-proxy/managed-user:v3.0.0-dev
```

项目脚本 4 函数约定（PATTERNS.md Skeleton 2，**纯文本版本**，禁止 ANSI 色码）：
```bash
pass() { echo "[PASS]  $1"; PASS_COUNT=$((PASS_COUNT + 1)); }
fail() { echo "[FAIL]  $1"; FAIL_COUNT=$((FAIL_COUNT + 1)); }
warn() { echo "[WARN]  $1"; WARN_COUNT=$((WARN_COUNT + 1)); }
info() { echo "[INFO]  $1"; }
```
</interfaces>

<tasks>

<task type="execute" id="35-01-T1">
  <name>Task 1: gen-bench-tree.sh — synthetic 10k mono-repo 生成器</name>
  <files>scripts/gen-bench-tree.sh, .planning/phases/35-e2e/benchmarks/.gitkeep, .planning/phases/35-e2e/benchmarks/README.md</files>
  <read_first>
    - scripts/gen-bench-tree.sh（**新文件，先 ls 确认不存在，然后以空白起步**）
    - scripts/verify-fuse-compat.sh（1-30 行 — 复制 shebang + strict mode + 4 函数 skeleton；**`pass/fail/warn/info` 全文必须纯文本，禁止 ANSI 色码**，Pattern M）
    - scripts/ci-doctor-grep.sh（17-19 行 — `WORK="$(mktemp -d)"; trap 'rm -rf "$WORK"' EXIT` 模式）
    - .planning/phases/35-e2e/35-PATTERNS.md Pitfall 4（RESEARCH §Pitfalls）— `.git/` + `node_modules/` + 真实文件扩展名三要素
    - .planning/phases/35-e2e/35-CONTEXT.md §"性能基准测试方法"（80/15/5 比例，总 ~200MB）
  </read_first>
  <action>
创建 `scripts/gen-bench-tree.sh`（≥ 80 行），严格遵守 PATTERNS.md Skeleton 1-5：

1. **CLI flags**（必须支持 `--count=N`、`--output=DIR`、`--seed=N`、`--help`）：
   - `--count` 默认 10000；值域校验 1000..100000
   - `--output` 默认 `/tmp/bench-tree`；若 DIR 存在则 `rm -rf` 并重建
   - `--seed` 默认 42（`awk 'BEGIN { srand(SEED) }'` 伪随机，保证跨机器可复现）
2. **文件分布（CONTEXT 锁定）**：
   - 80% 小文件：扩展名在 `.go .ts .tsx .py .rs .md .json .yaml` 中循环，内容为 `head -c $(( RANDOM % 3800 + 200 )) /dev/urandom | base64`（< 4KB）
   - 15% 中等文件：扩展名 `.lock .sum .svg .html`，内容 `head -c $(( RANDOM % 900000 + 100000 )) /dev/urandom | base64`（100KB-1MB）
   - 5% 大文件：扩展名 `.bin .pack .wasm`，`dd if=/dev/urandom of=<path> bs=1M count=$(( RANDOM % 9 + 1 ))`（1-10MB）
3. **目录深度**（Claude's Discretion 内，但必须：）：
   - 顶层固定创建：`src/`、`pkg/`、`internal/`、`test/`、`docs/`、`.git/objects/pack/`、`node_modules/<随机 20 包>/dist/`
   - 深度 3-5 级，每级子目录数 3-8 个；**禁止** 超过 7 级（避免 mergerfs readdir 极端值）
4. **Pitfall 4 防御（RESEARCH §Pitfalls）**：
   - 在 `.git/objects/pack/` 下写 3 个 1-5MB 的 `pack-<hex>.pack` 文件（模拟大型 git pack）
   - 在 `node_modules/` 下创建 200 个空嵌套目录 `foo/node_modules/bar/node_modules/...`
   - 文件内容 5% 含二进制 `NUL` 字节（触发 `rg` binary skip）
5. **输出**：脚本末尾打印统计（`find ... | wc -l`、`du -sh`），且总文件数必须满足 `--count` ± 50（CI 断言）。
6. **退出码**：0 = 成功；1 = 参数错误；2 = 磁盘空间不足（< 500MB 时 `df` 检查 fail）。

同时：
- 创建 `.planning/phases/35-e2e/benchmarks/.gitkeep`（空文件）
- 创建 `.planning/phases/35-e2e/benchmarks/README.md`（≥ 20 行）：说明 JSON 文件命名 `bench-YYYYMMDD-HHMMSS.json` + `cold-start-YYYYMMDD-HHMMSS.json`、hyperfine schema 引用、历史对比 `jq '.results[] | .mean' bench-*.json | paste -s -d,` 范例。

**命令/路径具体值（不留占位）**：
- PROJECT_ROOT 解析：`PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"`
- 默认 output: `/tmp/bench-tree`
- `--help` 输出必须含字样 `"synthetic 10k mono-repo tree for Phase 35 BASE-01 benchmarks"`
  </action>
  <acceptance_criteria>
    - `bash -n scripts/gen-bench-tree.sh` 退出码 0
    - `bash scripts/gen-bench-tree.sh --help` 退出码 0 且 stdout 含字符串 `synthetic 10k mono-repo tree for Phase 35 BASE-01 benchmarks`
    - `bash scripts/gen-bench-tree.sh --count=1000 --output=/tmp/bench-tree-test` 执行成功，随后 `find /tmp/bench-tree-test -type f | wc -l` 返回介于 950..1050 的整数
    - `grep -c '^\(pass\|fail\|warn\|info\)()' scripts/gen-bench-tree.sh` ≥ 4（4 函数齐全）
    - `test -d /tmp/bench-tree-test/.git/objects/pack` + `test -d /tmp/bench-tree-test/node_modules` 均退出码 0（Pitfall 4 防御生效）
    - `! grep -qP '\x1b\[' scripts/gen-bench-tree.sh`（perl 正则真匹配 ANSI ESC `\x1b[`，纯文本无色码，Pattern M）
    - `grep -q 'PROJECT_ROOT="\$(cd "\$(dirname "\$0")/\.\." && pwd)"' scripts/gen-bench-tree.sh` 退出码 0（路径安全约束 Pattern N）
    - `test -f .planning/phases/35-e2e/benchmarks/.gitkeep` 退出码 0
    - `grep -c '^## ' .planning/phases/35-e2e/benchmarks/README.md` ≥ 3
    - 清理：`rm -rf /tmp/bench-tree-test`
  </acceptance_criteria>
  <done>gen-bench-tree.sh 可重复生成 ~10k 文件 mono-repo；benchmarks/ 目录及自述落地。</done>
</task>

<task type="execute" id="35-01-T2">
  <name>Task 2: perf-benchmark.sh — BASE-01 三档对比基准 + P50/P99</name>
  <files>scripts/perf-benchmark.sh</files>
  <read_first>
    - scripts/perf-benchmark.sh（**新文件**）
    - scripts/ci-doctor-grep.sh（整份 — Pattern A JSON 断言与 awk 段落匹配）
    - scripts/verify-fuse-compat.sh（1-30 行 skeleton + 74-85 行 docker container helper，Pattern C）
    - .planning/phases/35-e2e/35-PATTERNS.md Pattern A + Pattern B + Pattern L（产物目录）
    - .planning/phases/35-e2e/35-RESEARCH.md §"Code Examples / Benchmark Script Skeleton"
    - deploy/docker/managed-user/image.lock（字段 local_dev_image_name 的 awk 解析目标）
    - .planning/REQUIREMENTS.md L19 (REQ-F1-C 原文) + L77 (BASE-01 原文)
  </read_first>
  <action>
创建 `scripts/perf-benchmark.sh`（≥ 120 行），BASE-01 唯一入口：

1. **CLI flags**：`--ci-mode`（CI 里只跑本地档 + skipsshfs/mergerfs 容器档）、`--runs=N`（默认 10）、`--warmup=N`（默认 1）、`--output-dir=DIR`（默认 `$PROJECT_ROOT/.planning/phases/35-e2e/benchmarks`）、`--help`。
2. **工具前置断言**（PATTERNS Skeleton 4）：
   - `require_cmd hyperfine` / `require_cmd rg` / `require_cmd jq`（缺即 exit 1 + 安装提示）
   - `require_cmd docker` 仅在非 `--ci-mode` 时要求
3. **三档基准命令**（REQ-F1-C + CONTEXT 锁定）：
   - **本地档**：`hyperfine -n "local-rg" 'rg . /tmp/bench-tree' -n "local-ls" 'ls -R /tmp/bench-tree >/dev/null'`
   - **mergerfs 档（非 CI）**：docker run managed-user image（`--cap-add SYS_ADMIN --device /dev/fuse --security-opt apparmor=unconfined`，Pattern C），把 `/tmp/bench-tree` 绑定到容器内 `/mnt/cold`，让容器内 sshfs+mergerfs 合成 `/workspace`，再 `hyperfine -n "mergerfs-rg" 'docker exec $CTR rg . /workspace' -n "mergerfs-ls" 'docker exec $CTR ls -R /workspace >/dev/null'`
   - **sshfs-only 档**：同上但强制 `--mount-mode=sshfs-only`（通过 `docker exec cloud-claude --mount-mode=sshfs-only` 或容器内直接 `mount -t fuse.sshfs`；Claude's Discretion — 如容器不具备直接测，走 `/mnt/cold` 代替并在 README 注明）
4. **统计**（Pattern A 末段公式）：每档 `--warmup 1 --runs 10`；用 jq 从 `.results[].times[]` 排序后抽取：
   ```
   p50 = times_sorted[(length*0.5) | floor]
   p99 = times_sorted[(length*0.99) | floor]
   ratio_p50 = mergerfs_p50 / local_p50
   ratio_p99 = mergerfs_p99 / local_p99
   ```
5. **双输出**（PATTERNS.md Pattern A + Pattern L）：
   - JSON: `${BENCH_DIR}/bench-$(date +%Y%m%d-%H%M%S).json`（合并三档 hyperfine JSON + 显式 ratio 字段 + hostname + uname -a + kernel + cpu_count）
   - MD: `${BENCH_DIR}/bench-$(date +%Y%m%d-%H%M%S).md`（表格 `| 命令 | local P50 | mergerfs P50 | ratio P50 | local P99 | mergerfs P99 | ratio P99 | 裁决 |`；裁决列：ratio ≤ 1.5 → `PASS(1.5×)` / ≤ 2.0 → `WARN(≤2×)` / 其它 → `FAIL`）
6. **裁决闸门**：
   - P50 ratio > 1.5 → `fail "BASE-01 P50 超出 1.5× 基线"`
   - P99 ratio > 2.0 → `fail "BASE-01 P99 超出 2× 基线"`
   - 仅 `WARN` 档位退出码仍为 0（CI 不阻塞），`FAIL` 退出码 1
7. **环境感知 SKIP**（Pattern B）：
   - `is_ci() && ! has_privileged` → mergerfs 档 SKIP，仅本地档报告
   - `--ci-mode` → 显式只跑本地档 + 写明 "CI 基线模式，真机档在 APFS/Ubuntu 25.04 单独验证"
8. **Pitfall 1 防御（RESEARCH §Pitfalls）**：warmup = 1 且 runs = 10 必守，在 MD 报告头部写入硬件信息（`uname -a` / `nproc` / `lscpu | grep MHz` 三行）。
9. **整个脚本必须能被 `scripts/v3-acceptance-checklist.sh`（Plan 05）子调用**：退出码 0 / 1 / 2（2 = SKIP 场景占位）。
  </action>
  <acceptance_criteria>
    - `bash -n scripts/perf-benchmark.sh` 退出码 0
    - `bash scripts/perf-benchmark.sh --help` 退出码 0 且输出含 `BASE-01`
    - `grep -qE 'hyperfine.*--warmup 1.*--runs 10' scripts/perf-benchmark.sh` 退出码 0（统计方法硬编码）
    - `grep -qE 'BENCH_DIR=.*\.planning/phases/35-e2e/benchmarks' scripts/perf-benchmark.sh` 退出码 0
    - `grep -qE 'times\s*\|\s*sort' scripts/perf-benchmark.sh` 退出码 0（P50/P99 公式存在）
    - `grep -qE 'ratio.*1\.5' scripts/perf-benchmark.sh` 退出码 0（1.5× 基线硬编码）
    - `grep -qE 'awk.*local_dev_image_name' scripts/perf-benchmark.sh` 退出码 0（复用 image.lock 解析）
    - `grep -qE '\\-\\-ci-mode' scripts/perf-benchmark.sh` 退出码 0（CI 开关存在）
    - 在 CI 模式试跑：`bash scripts/perf-benchmark.sh --ci-mode --runs=2 --warmup=1`（要求 Task 1 已完成且 `/tmp/bench-tree` 已存在）退出码 ≤ 1；`ls .planning/phases/35-e2e/benchmarks/bench-*.json` 返回至少 1 个文件；`jq -e '.results | length >= 1' $(ls -t .planning/phases/35-e2e/benchmarks/bench-*.json | head -1)` 退出码 0
  </acceptance_criteria>
  <done>BASE-01 可通过单条命令产出三档 JSON+MD 报告 + 裁决列；CI 与真机两种路径都走通。</done>
</task>

<task type="execute" id="35-01-T3">
  <name>Task 3: cold-start-benchmark.sh — BASE-02 首连 ≤ 8s + 三段式进度断言</name>
  <files>scripts/cold-start-benchmark.sh</files>
  <read_first>
    - scripts/cold-start-benchmark.sh（**新文件**）
    - scripts/ci-doctor-grep.sh（整份 — awk section 匹配 stderr + grep 三段式关键字）
    - scripts/verify-fuse-compat.sh（74-85 行 Docker helper 模式）
    - .planning/phases/35-e2e/35-RESEARCH.md §"Open Questions #3"（prompt 可输入的 grep pattern）
    - .planning/REQUIREMENTS.md L18 (REQ-F1-B 原文 — "初始化文件映射 (1/3) 热同步源码中…" 三段式模板) + L78 (BASE-02)
    - internal/cloudclaude/mount_strategy.go（**可选，grep 三段式文案** — 验证 CLI 实际输出字符串；如找不到直接在脚本中列出期望 regex 由 verifier 二次校准）
  </read_first>
  <action>
创建 `scripts/cold-start-benchmark.sh`（≥ 110 行），BASE-02 专用：

1. **CLI flags**：`--attempts=N`（默认 5，CONTEXT 锁定）、`--threshold-seconds=N`（默认 8，CONTEXT 锁定）、`--min-pass=N`（默认 4 — "5 次中 ≥ 4 次 ≤ 8s"）、`--output-dir=DIR`（默认 `$PROJECT_ROOT/.planning/phases/35-e2e/benchmarks`）、`--ci-mode`、`--help`。
2. **流程**（每次 attempt 独立）：
   1. `docker run -d managed-user ... sleep 600` 起容器（Pattern C 容器 helper，`trap docker rm -f` 保证 EXIT 清理）
   2. 记录 `t0=$(date +%s%3N)`（毫秒精度）
   3. `docker exec $CTR cloud-claude --mount-mode=auto --json 2> "$WORK/attempt-N.stderr" & CLI_PID=$!`
   4. **就绪探测循环**（RESEARCH §Open Question #3）：
      - 每 200ms 一次 `docker exec $CTR tmux capture-pane -p 2>/dev/null | grep -qE '(^>|claude> |➜ )'`
      - 命中后记录 `t1=$(date +%s%3N)`，`duration_ms=$((t1 - t0))`
      - 硬超时 15000ms — 超时记录为 `duration_ms=-1`（该 attempt 记为 FAIL）
   5. `kill $CLI_PID; docker rm -f $CTR`
3. **三段式中文进度断言**（REQ-F1-B 锁定）：
   - `grep -qF "初始化文件映射 (1/3) 热同步源码中" "$WORK/attempt-N.stderr"` → 记 `stage_1=true`
   - `grep -qF "(2/3) 启动冷兜底" "$WORK/attempt-N.stderr"` → 记 `stage_2=true`
   - `grep -qF "(3/3) 合并视图" "$WORK/attempt-N.stderr"` → 记 `stage_3=true`
   - `stderr_progress_matches = (stage_1 AND stage_2 AND stage_3)`
4. **聚合输出**：
   - JSON `${BENCH_DIR}/cold-start-$(date +%Y%m%d-%H%M%S).json`：
     ```json
     { "schema_version": 1, "attempts": [{"idx":1,"duration_ms":7812,"stderr_progress_matches":true,"outcome":"pass"}, ...],
       "summary": {"total":5,"pass":4,"fail":1,"threshold_ms":8000,"progress_matches_all":true} }
     ```
   - MD `${BENCH_DIR}/cold-start-$(date +%Y%m%d-%H%M%S).md`：表格 `| # | duration(ms) | progress | outcome |`
5. **裁决闸门**：
   - `summary.pass < --min-pass` → `fail "BASE-02 首连 ≤ 8s 达标次数 $pass/$total 低于要求 $min"` + exit 1
   - `summary.progress_matches_all == false` → `fail "REQ-F1-B 三段式进度缺失"` + exit 1
   - 两者通过 → `pass "BASE-02 ok"` + exit 0
6. **CI 感知 SKIP**（Pattern B）：`is_ci && ! has_docker_privileged` → `skip "BASE-02" "CI 无 FUSE 特权，本档仅真机执行"` + exit 2。
7. **Pitfall 3 防御（tmux race）**：capture-pane 前 `sleep 2`；就绪探测最多 3 次重试。

**硬编码具体值**：`--attempts=5` / `--threshold-seconds=8` / `--min-pass=4` / 三段式关键字精确字符串（不用 regex 偷懒）。
  </action>
  <acceptance_criteria>
    - `bash -n scripts/cold-start-benchmark.sh` 退出码 0
    - `bash scripts/cold-start-benchmark.sh --help` 退出码 0 且 stdout 含 `BASE-02` 与 `≤ 8s`
    - `grep -qF '初始化文件映射 (1/3) 热同步源码中' scripts/cold-start-benchmark.sh` 退出码 0
    - `grep -qF '(2/3) 启动冷兜底' scripts/cold-start-benchmark.sh` 退出码 0
    - `grep -qF '(3/3) 合并视图' scripts/cold-start-benchmark.sh` 退出码 0
    - `grep -qE 'threshold_(ms|seconds).*8' scripts/cold-start-benchmark.sh` 退出码 0
    - `grep -qE '--attempts=5' scripts/cold-start-benchmark.sh` 退出码 0
    - `grep -qE '--min-pass=4' scripts/cold-start-benchmark.sh` 退出码 0
    - `grep -qE 'stderr_progress_matches' scripts/cold-start-benchmark.sh` 退出码 0（JSON 字段名固化）
    - `grep -qE 'schema_version.*1' scripts/cold-start-benchmark.sh` 退出码 0
    - `grep -qE 'is_ci.*skip' scripts/cold-start-benchmark.sh` 退出码 0（CI skip 逻辑存在）
  </acceptance_criteria>
  <done>BASE-02 首连基准脚本可产出 JSON/MD 报告并断言三段式进度；无 docker privileged 时 CI 优雅 SKIP。</done>
</task>

</tasks>

<verification>
脚本层自动化验证：
```bash
bash -n scripts/gen-bench-tree.sh && echo "gen-bench-tree.sh ok"
bash -n scripts/perf-benchmark.sh && echo "perf-benchmark.sh ok"
bash -n scripts/cold-start-benchmark.sh && echo "cold-start-benchmark.sh ok"
bash scripts/gen-bench-tree.sh --help >/dev/null && echo "help ok"
bash scripts/perf-benchmark.sh --help   >/dev/null && echo "help ok"
bash scripts/cold-start-benchmark.sh --help >/dev/null && echo "help ok"

# 小规模烟测（CI-friendly）
bash scripts/gen-bench-tree.sh --count=1000 --output=/tmp/bench-tree-smoke
find /tmp/bench-tree-smoke -type f | wc -l   # 950..1050

# 在能跑 FUSE 特权的 Linux 环境可加跑：
# bash scripts/perf-benchmark.sh --ci-mode --runs=2 --warmup=1
# ls .planning/phases/35-e2e/benchmarks/bench-*.json

rm -rf /tmp/bench-tree-smoke
```

关键字面量断言（防止被 executor 简化成"基本工作"）：
```bash
grep -qF '初始化文件映射 (1/3) 热同步源码中' scripts/cold-start-benchmark.sh
grep -qF 'BASE-01' scripts/perf-benchmark.sh
grep -qE 'ratio.*1\.5' scripts/perf-benchmark.sh
grep -qE 'threshold_(ms|seconds).*8' scripts/cold-start-benchmark.sh
```

ANSI 色码反向断言（Pattern M，统一约束 W-3 修复）：
```bash
# 用 perl 正则匹配真实 ANSI ESC 字节 \x1b[，避免 BRE/ERE 把 \033 当字面 4 字符
for f in scripts/gen-bench-tree.sh scripts/perf-benchmark.sh scripts/cold-start-benchmark.sh; do
  ! grep -qP '\x1b\[' "$f" || { echo "ANSI escape detected in $f"; exit 1; }
done
echo "no-ANSI ok (Plan 01 三脚本均为纯文本)"
```
</verification>

<success_criteria>
- Phase SC #1（BASE-01）：perf-benchmark.sh 输出 JSON 包含本地/mergerfs/sshfs-only 三档 P50+P99 ratio，裁决列含 1.5× / 2× 两档闸门
- Phase SC #2（BASE-02）：cold-start-benchmark.sh 输出 JSON 包含 5 次 attempts + pass/fail 汇总 + 三段式 stderr 匹配位
- REQ-F1-B / REQ-F1-C / BASE-01 / BASE-02 在本 plan 的 `requirements_addressed` 全部出现且有对应脚本
- 所有脚本通过 `bash -n`、`--help` 可用、无 ANSI 色码、路径经 `PROJECT_ROOT` 解析
</success_criteria>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| 脚本 → 本地文件系统 | gen-bench-tree.sh 生成 ~200MB 临时数据到 `/tmp/bench-tree`；perf-benchmark.sh 写 JSON/MD 报告到仓库内 `.planning/phases/35-e2e/benchmarks/` |
| 脚本 → Docker daemon | perf/cold-start 脚本 `docker run --cap-add SYS_ADMIN --device /dev/fuse --security-opt apparmor=unconfined`（managed-user 镜像约定） |
| 脚本 → hyperfine 子进程 | 以当前用户权限跑 `rg` / `ls -R`；不触网 |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-35-01-01 | Tampering | gen-bench-tree.sh 的 `rm -rf "$OUTPUT"` | mitigate | `--output` 路径硬校验：禁止空串、`/`、`$HOME`、`$PROJECT_ROOT`；`case "$OUTPUT" in /|""|"$HOME") echo "非法 output 路径"; exit 1 ;; esac` 显式落地 |
| T-35-01-02 | Denial of Service | 生成 10k 文件 × 5% 大文件 × 10MB ≈ 500MB 磁盘 | mitigate | 脚本开头 `df -k "$(dirname "$OUTPUT")"` 检查可用空间 ≥ 1GB，不足直接 exit 2 |
| T-35-01-03 | Information Disclosure | hyperfine JSON 含 hostname / uname -a | accept | benchmark 报告默认写入仓库，仓库已私有；如未来开源需在 JSON 输出处脱敏（v3.1 backlog） |
| T-35-01-04 | Elevation of Privilege | docker run 带 SYS_ADMIN + apparmor=unconfined | accept | 与现有 `scripts/verify-fuse-compat.sh` 一致，用完 `trap docker rm -f` 立即销毁；不引入比现有更高权限 |
| T-35-01-05 | Repudiation | 基准报告文件名含时间戳但不含作者 | mitigate | README.md 约定：真机运行的报告在 markdown 头部追加 `> 执行机器: <hostname> / <OS> / <运行人>`，人工签字处 |
</threat_model>

<rollback>
- 所有 3 个脚本为新文件，回滚 = `git rm scripts/gen-bench-tree.sh scripts/perf-benchmark.sh scripts/cold-start-benchmark.sh`
- `.planning/phases/35-e2e/benchmarks/` 目录为新建，回滚 = `rm -rf` 目录
- 无既有文件修改，无需反向迁移
- 若 `/tmp/bench-tree` 已生成，脚本 trap EXIT 在异常退出时清理；手动兜底 `rm -rf /tmp/bench-tree /tmp/bench-tree-*`
</rollback>

<output>
After completion, create `.planning/phases/35-e2e/35-01-SUMMARY.md` documenting:
- 三个脚本最终行数与关键函数列表
- 烟测命令与实际输出
- 任何 Claude's Discretion 决策的落地（如 mergerfs 档的具体实现方式）
- Deferred 到 Plan 05 真机执行的项（APFS / Ubuntu 25.04 实测结果）
</output>
