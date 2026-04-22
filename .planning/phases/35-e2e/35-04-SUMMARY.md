---
phase: 35-e2e
plan: 04
subsystem: infra
tags: [github-actions, ci, hyperfine, docker, image-size-gate, perf-regression]

# Dependency graph
requires:
  - phase: 35-e2e
    provides: scripts/perf-benchmark.sh + scripts/gen-bench-tree.sh (Plan 01)
  - phase: 29
    provides: scripts/verify-managed-image.sh + image.lock 镜像名解析约定
provides:
  - .github/workflows/ci.yml::perf-benchmark job (BASE-01 CI gate, 本地档 only)
  - .github/workflows/ci.yml::image-size-regression job (BASE-04 二次回归 ≤ 700MB)
  - perf-bench-${sha} artifact 上传机制 (retention 30d)
affects: [phase 35-e2e/05-acceptance-checklist, future PRs touching Dockerfile/scripts]

# Tech tracking
tech-stack:
  added: [hyperfine (apt), ripgrep (apt), jq (apt), actions/upload-artifact@v4]
  patterns:
    - "PATTERNS Pattern K：CI 新 job 模板（runs-on: ubuntu-latest + checkout@v4 + setup + 核心 run + 可选 artifact upload）"
    - "阈值字面量保留语义：700 * 1024 * 1024（不预先算成 734003200，让 review 者一眼认出 700MB）"
    - "镜像名从 image.lock 解析，与 verify-managed-image.sh 共用 awk -F': ' 风格，避免常量重复"

key-files:
  created: []
  modified:
    - .github/workflows/ci.yml

key-decisions:
  - "perf-benchmark job 仅跑 --ci-mode 本地档（mergerfs/sshfs 档延后到 Plan 05 真机），CI runner 无 FUSE 特权"
  - "image-size-regression 在 ci.yml 做二次回归（与 build-images.yml 内同断言并行），Dockerfile 改动被双 gate 阻断"
  - "复用顶部 on/concurrency 配置，禁止 job 级 if:，PR 触发条件与 go-test/web-build 完全一致"
  - "if: always() 使 benchmark 失败时也上传 artifact，便于回归调试"
  - "--seed=42 与 perf-benchmark.sh 默认 --runs=10 --warmup=1 落地，跨 run 基准可复现"

patterns-established:
  - "Pattern K (CI 新 job 模板)：4 个 step（Checkout → Install → Generate/Build → Core → Upload artifact）"
  - "镜像 size gate 二次回归模式：image.lock 解析 → docker build → 工具脚本 → docker image inspect 字面量阈值断言"

requirements-completed: [BASE-01, BASE-04]

# Metrics
duration: 2 min
completed: 2026-04-22
---

# Phase 35 Plan 04: CI Gates Summary

**`.github/workflows/ci.yml` 追加 `perf-benchmark` 与 `image-size-regression` 两个 job，把 BASE-01 性能基线与 BASE-04 镜像 ≤ 700MB 从「人工跑脚本」升级为「每个 PR 自动 gate」。**

## Performance

- **Duration:** ~2 min
- **Started:** 2026-04-22T11:12:51Z
- **Completed:** 2026-04-22T11:13:56Z
- **Tasks:** 1
- **Files modified:** 1

## Accomplishments

- ci.yml 由 60 行扩展为 125 行（+65 行），新增 2 个 job 共 8 个 step
- perf-benchmark job：`hyperfine + ripgrep + jq` 安装 → `gen-bench-tree.sh --count=10000 --seed=42` → `perf-benchmark.sh --ci-mode --runs=10 --warmup=1` → upload `.planning/phases/35-e2e/benchmarks/` artifact（retention 30d）
- image-size-regression job：从 `image.lock` 解析 `local_dev_image_name` → `docker build` managed-user → 调 `scripts/verify-managed-image.sh` → 断言未压缩 size ≤ `700 * 1024 * 1024` 字节
- 原 `go-test` / `web-build` 两 job 零改动，顶部 name/on/permissions/concurrency 配置完全保留

## Task Commits

1. **Task 1: ci.yml 追加 perf-benchmark 与 image-size-regression 两个 job** — `998c32d` (ci)

**Plan metadata:** 待最终 docs commit 提供（包含本 SUMMARY.md + STATE.md + ROADMAP.md）

## Files Created/Modified

- `.github/workflows/ci.yml` — 新增 perf-benchmark job (L62-87, 26 行) 与 image-size-regression job (L89-125, 37 行)；保留 go-test (L18-32) / web-build (L34-60)。

### 关键字面量引用位置

| 字面量 | 行号 | 作用 |
|--------|------|------|
| `scripts/gen-bench-tree.sh --count=10000 --output=/tmp/bench-tree --seed=42` | L75 | Plan 01 同步提供，--seed=42 保证跨 run 复现 |
| `scripts/perf-benchmark.sh --ci-mode --runs=10 --warmup=1` | L78 | --ci-mode 跳过 mergerfs/sshfs 真机档，--runs/--warmup 与脚本默认值显式对齐 |
| `actions/upload-artifact@v4` + `retention-days: 30` | L82-87 | 失败时也上传（`if: always()`），便于回归调试 |
| `awk -F': ' '$1 == "local_dev_image_name" { print $2 }' deploy/docker/managed-user/image.lock` | L98 | 与 `scripts/verify-managed-image.sh:4` 同风格，避免常量重复 |
| `docker build -t ... -f deploy/docker/managed-user/Dockerfile .` | L107-111 | 仅基于本仓库 Dockerfile 构建本地 tag，不从外部 pull（T-35-04-04 mitigation） |
| `bash scripts/verify-managed-image.sh` | L114 | Phase 29 已存脚本：sshd_config / claude / chromium / xterm / pcmanfm 全套二进制存在性 |
| `max=$((700 * 1024 * 1024))    # 700 MiB = 734003200 bytes` | L119 | BASE-04 阈值显式字面量保留 700MB 语义 |

## Decisions Made

- 复用 ci.yml 顶部 `on: pull_request + push to main + workflow_call` 与 `concurrency: cancel-in-progress`，**不**给新 job 单独加 `if:`，确保触发条件与既有 go-test / web-build 完全一致
- perf-benchmark 仅跑本地档：mergerfs/sshfs-only 档需要 FUSE 特权，CI runner 不允许，已留 Plan 05 真机矩阵
- image-size-regression 是**二次**回归：Phase 29 在 build-images.yml 内已落 BASE-04 断言，本 plan 在主 CI 路径再做一次，确保任何 Dockerfile 改动被双 gate 阻断（不是替代）
- 阈值用字面量 `700 * 1024 * 1024`（不预算成 734003200），保留人类可读语义并便于 review

## Deviations from Plan

None — plan executed exactly as written. 唯一一个 type="execute" task 严格按 PLAN action 段提供的 yaml 模板字面量追加，所有 acceptance criteria 通过。

## Issues Encountered

- 本地 `python3 -c 'import yaml'` 因 PEP 668 禁止 pip3 install 而无法 import pyyaml；改用 `ruby -ryaml` 做 yaml 合法性验证（PLAN success criteria 允许等价工具），SUMMARY 自检 PASS。`yq` / `actionlint` / `act` 本地未安装，跳过这些可选检查（CI 真跑时由 GitHub Actions runner 自身做 yaml parse 兜底）。

### Acceptance Criteria 自检结果

- ✅ `test -f .github/workflows/ci.yml`
- ✅ `ruby -ryaml -e "YAML.safe_load(...)"` 解析通过（python yaml 不可用 → 等价替代）
- ✅ Ruby YAML parse 列出 4 jobs：`["go-test", "web-build", "perf-benchmark", "image-size-regression"]`
- ✅ 4 个 job 的 `runs-on` 均为 `ubuntu-latest`
- ✅ 11 条字面量 grep 全部命中：scripts/perf-benchmark.sh --ci-mode、scripts/gen-bench-tree.sh、scripts/verify-managed-image.sh、`700 * 1024 * 1024`、hyperfine、ripgrep、actions/upload-artifact@v4、actions/checkout@v4、`awk -F.*local_dev_image_name`
- ✅ 原 go-test / web-build 行首匹配仍存在
- ⊘ act / actionlint / yq 本地缺失，跳过（PLAN 允许 if available）

## User Setup Required

None — 无新 secrets、无 dashboard 改动、无 env 变量；新 job 在 PR 自动触发。首次成功 run 的 URL 待 push 后由 CI 提供。

## Next Phase Readiness

- BASE-01 / BASE-04 CI gate 就位，PR 自动跑回归
- Plan 05 (acceptance-checklist) 可直接引用本 SUMMARY 把「CI 自动 gate」勾掉，仅剩真机矩阵 (mergerfs/sshfs/AppArmor/macOS APFS) 进入人工 UAT 项

## Threat Surface Scan

无新 threat flag — 所有变更走既有 trust boundary（GitHub Actions runner ↔ docker daemon），与 build-images.yml 同 surface；artifact 上传范围限制在 `.planning/phases/35-e2e/benchmarks/`，仓库已私有。

---
*Phase: 35-e2e*
*Plan: 04 ci-gates*
*Completed: 2026-04-22*

## Self-Check: PASSED

- ✅ FOUND: `.github/workflows/ci.yml` (modified, 125 lines)
- ✅ FOUND: commit `998c32d` (ci(35-04): add perf-benchmark + image-size-regression jobs)
- ✅ FOUND: `.planning/phases/35-e2e/35-04-SUMMARY.md` (this file)
