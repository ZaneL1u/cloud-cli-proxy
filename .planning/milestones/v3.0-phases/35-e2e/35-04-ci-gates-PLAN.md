---
phase: 35-e2e
plan: 04
type: execute
wave: 1
depends_on: []
autonomous: true
files_modified:
  - .github/workflows/ci.yml
requirements_addressed:
  - BASE-01
  - BASE-04
threat_model_severity: medium
must_haves:
  truths:
    - "`.github/workflows/ci.yml` 含两个新 job：`perf-benchmark` 与 `image-size-regression`"
    - "perf-benchmark job 调用 scripts/perf-benchmark.sh（Plan 01 交付）并 upload-artifact 到 `.planning/phases/35-e2e/benchmarks/`"
    - "image-size-regression job 调用 scripts/verify-managed-image.sh 并断言未压缩镜像 ≤ 734003200 字节（700 × 1024 × 1024）"
    - "两 job 触发条件与现有 `go-test` 一致（pull_request + push to main + workflow_call），并使用同样的 concurrency 组"
    - "CI yaml 通过 `yq '.jobs.perf-benchmark'` / `yq '.jobs.image-size-regression'` 读出非 null"
  artifacts:
    - path: ".github/workflows/ci.yml"
      provides: "新增 perf-benchmark 与 image-size-regression 两个 job（保留原有 go-test / web-build）"
      contains: "perf-benchmark|image-size-regression"
  key_links:
    - from: ".github/workflows/ci.yml::perf-benchmark"
      to: "scripts/perf-benchmark.sh --ci-mode"
      via: "run: bash scripts/perf-benchmark.sh --ci-mode --runs=10 --warmup=1"
      pattern: "scripts/perf-benchmark.sh --ci-mode"
    - from: ".github/workflows/ci.yml::image-size-regression"
      to: "scripts/verify-managed-image.sh"
      via: "run: bash scripts/verify-managed-image.sh"
      pattern: "scripts/verify-managed-image.sh"
    - from: ".github/workflows/ci.yml::image-size-regression"
      to: "700 × 1024 × 1024 字节 = 734003200"
      via: "max=$((700 * 1024 * 1024)) + size > max → exit 1"
      pattern: "700 \\* 1024 \\* 1024"
---

<objective>
把 BASE-01（本地档 perf-benchmark CI 回归）和 BASE-04（镜像 ≤ 700MB CI gate 二次回归）从"人工跑脚本"升级为"每个 PR 自动 gate"。
Purpose: Phase 29 已落地 BASE-04 的镜像大小断言，但当时 CI job 合并入 build-images，Phase 35 要求在主 CI（ci.yml）路径做二次回归 —— 任何 Dockerfile 改动都被阻断。Perf-benchmark 的 CI 化负责捕捉性能倒退，不跑 FUSE 特权档（本地 / mergerfs 档走真机 Plan 05），仅跑 gen-bench-tree + hyperfine 纯本地档。
Output: `.github/workflows/ci.yml` 在 `web-build` 之后新增两个 job，结构严格照搬现有 `go-test` 模式（runs-on: ubuntu-latest + actions/checkout@v4 + setup steps + 核心 run + optional artifact upload）。
</objective>

<execution_context>
@.cursor/get-shit-done/workflows/execute-plan.md
@.cursor/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/35-e2e/35-RESEARCH.md
@.planning/phases/35-e2e/35-PATTERNS.md
@.planning/phases/35-e2e/35-CONTEXT.md

<!-- 信源 -->
@.github/workflows/ci.yml
@.github/workflows/build-images.yml
@scripts/verify-managed-image.sh
</context>

<interfaces>
<!-- 现有 ci.yml 结构（ci.yml L17-32 go-test 与 L34-60 web-build）— 新 job 结构必须与此一致 -->

```yaml
jobs:
  <job-id>:
    name: <Human Readable Name>
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4
      - name: <Setup / Install>
        run: ...
      - name: <Core>
        run: ...
      - name: <Upload artifact>（可选）
        uses: actions/upload-artifact@v4
```

<!-- PATTERNS.md Pattern K 模板 L466-513 完整示范 -->

<!-- Plan 01 交付的脚本 -->
- scripts/gen-bench-tree.sh（--count=10000 --output=/tmp/bench-tree）
- scripts/perf-benchmark.sh（--ci-mode，CI 仅跑本地档；mergerfs/sshfs-only 档在 CI SKIP）

<!-- 现有脚本（verify-managed-image.sh L4） -->
IMAGE_NAME 从 `deploy/docker/managed-user/image.lock` 的 `local_dev_image_name:` 字段解析

<!-- 镜像大小硬阈值 -->
未压缩：700 × 1024 × 1024 = 734003200 bytes
docker image inspect --format='{{.Size}}' <image>
</interfaces>

<tasks>

<task type="execute" id="35-04-T1">
  <name>Task 1: ci.yml 追加 perf-benchmark 与 image-size-regression 两个 job</name>
  <files>.github/workflows/ci.yml</files>
  <read_first>
    - .github/workflows/ci.yml（**整份**现有文件 — L1-60，保留原 go-test / web-build，新 job 接在末尾）
    - .github/workflows/build-images.yml（整份 — strategy.matrix / docker build 流程参考，Pattern K 信源 L25）
    - scripts/verify-managed-image.sh（整份 — image.lock awk 解析）
    - .planning/phases/35-e2e/35-PATTERNS.md Pattern K L466-513（两 job 模板完整示范 — executor 照抄结构，仅调整细节）
    - 确认 Plan 01 的 scripts/gen-bench-tree.sh 与 scripts/perf-benchmark.sh 最终行为（跨 Plan 依赖，本 Plan **不重复** ships 脚本）
  </read_first>
  <action>
**不重写 ci.yml**，仅在末尾追加两个 job（保留现有 `go-test` / `web-build` 完全不动）。

### 新 Job #1：`perf-benchmark`（BASE-01 CI gate，本地档）

```yaml
  perf-benchmark:
    name: Performance Benchmark (synthetic 10k, local baseline)
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Install hyperfine + ripgrep + jq
        run: |
          sudo apt-get update
          sudo apt-get install -y --no-install-recommends hyperfine ripgrep jq

      - name: Generate synthetic tree
        run: bash scripts/gen-bench-tree.sh --count=10000 --output=/tmp/bench-tree --seed=42

      - name: Run benchmark (CI baseline only)
        run: bash scripts/perf-benchmark.sh --ci-mode --runs=10 --warmup=1

      - name: Upload bench artifact
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: perf-bench-${{ github.sha }}
          path: .planning/phases/35-e2e/benchmarks/
          if-no-files-found: warn
          retention-days: 30
```

**约束硬性落实**：
- `runs-on: ubuntu-latest` 与 `go-test` 完全一致
- `--no-install-recommends` 保留（减小依赖体积）
- `--seed=42` 保证跨 run 基准可复现
- `--runs=10 --warmup=1` 与 perf-benchmark.sh 默认值一致（CONTEXT 锁定）
- `if: always()` 使 benchmark 失败时也能上传产物供调试
- artifact 保留 30 天（PR 验证周期足够）

### 新 Job #2：`image-size-regression`（BASE-04 二次回归）

```yaml
  image-size-regression:
    name: Image Size Regression (BASE-04 ≤ 700MB)
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Read expected image name
        id: read-image
        run: |
          IMAGE_NAME=$(awk -F': ' '$1 == "local_dev_image_name" { print $2 }' deploy/docker/managed-user/image.lock)
          if [ -z "$IMAGE_NAME" ]; then
            echo "failed to read local_dev_image_name from image.lock" >&2
            exit 1
          fi
          echo "IMAGE_NAME=$IMAGE_NAME" >> "$GITHUB_OUTPUT"

      - name: Build managed-user image
        run: |
          docker build \
            -t "${{ steps.read-image.outputs.IMAGE_NAME }}" \
            -f deploy/docker/managed-user/Dockerfile \
            .

      - name: Run verify-managed-image.sh
        run: bash scripts/verify-managed-image.sh

      - name: Assert uncompressed size ≤ 700MB
        run: |
          size=$(docker image inspect "${{ steps.read-image.outputs.IMAGE_NAME }}" --format='{{.Size}}')
          max=$((700 * 1024 * 1024))    # 700 MiB = 734003200 bytes
          echo "image_size_bytes=$size (max=$max)"
          if [ "$size" -gt "$max" ]; then
            echo "FAIL: image size $size > $max (700MB)" >&2
            exit 1
          fi
          echo "PASS: image size within BASE-04 budget"
```

**约束硬性落实**：
- 阈值 `700 * 1024 * 1024` 字面量（**不要算成 734003200**，让 review 者能直接认出 700MB 语义）
- 从 `image.lock` 读镜像名（与 verify-managed-image.sh 同风格，避免重复常量）
- Phase 29 已落地过 `BASE-04` 但入口在 build-images.yml；本 job 在 ci.yml 做**二次**回归（不是替代 build-images.yml 中的那一份）
- 构建成功才跑 verify-managed-image.sh（防止脚本对不存在镜像 inspect 报错误）

### 触发条件 & 并发组

两 job 均复用 ci.yml 顶部现有配置（L3-14）：
- `on: pull_request + push to main + workflow_call`
- `concurrency: ci-${{ github.workflow }}-${{ github.ref }}` cancel-in-progress
**禁止**在 job 级另外加 `if:`（保持与 go-test / web-build 一致的 PR 触发）

### 最终 yaml 结构（参考）

```
name: CI
on: ...
permissions: ...
concurrency: ...

jobs:
  go-test:       # 保留不动
  web-build:     # 保留不动
  perf-benchmark:         # 新增
  image-size-regression:  # 新增
```

**不要** 改动 go-test / web-build；**不要** 改动顶部 name/on/permissions/concurrency；**不要** 引入 strategy.matrix（两 job 都是单机跑）。
  </action>
  <acceptance_criteria>
    - `test -f .github/workflows/ci.yml` 退出码 0
    - CI yaml 语法合法：`yq eval '.jobs | keys' .github/workflows/ci.yml`（或 `python3 -c 'import yaml,sys; yaml.safe_load(open(".github/workflows/ci.yml"))'`）退出码 0
    - `yq eval '.jobs.perf-benchmark.runs-on' .github/workflows/ci.yml` 输出 `ubuntu-latest`（或等价 `yq '.jobs.perf-benchmark' ...` 不为 null；使用任一可用 yq 实现）
    - `yq eval '.jobs.image-size-regression.runs-on' .github/workflows/ci.yml` 输出 `ubuntu-latest`
    - `yq eval '.jobs.go-test.runs-on' .github/workflows/ci.yml` 输出 `ubuntu-latest`（原 job 保留断言）
    - `yq eval '.jobs.web-build' .github/workflows/ci.yml` 非 null（原 job 保留断言）
    - `grep -qF 'scripts/perf-benchmark.sh --ci-mode' .github/workflows/ci.yml` 退出码 0
    - `grep -qF 'scripts/gen-bench-tree.sh' .github/workflows/ci.yml` 退出码 0
    - `grep -qF 'scripts/verify-managed-image.sh' .github/workflows/ci.yml` 退出码 0
    - `grep -qF '700 * 1024 * 1024' .github/workflows/ci.yml` 退出码 0（BASE-04 阈值显式字面量）
    - `grep -qF 'hyperfine' .github/workflows/ci.yml` 退出码 0
    - `grep -qF 'ripgrep' .github/workflows/ci.yml` 退出码 0
    - `grep -qF 'actions/upload-artifact@v4' .github/workflows/ci.yml` 退出码 0
    - `grep -qF 'actions/checkout@v4' .github/workflows/ci.yml` 退出码 0（与现有 job 一致）
    - `grep -qE 'awk -F.*local_dev_image_name' .github/workflows/ci.yml` 退出码 0（与 verify-managed-image.sh 同 awk 解析风格）
    - 如果工具链含 act（`command -v act`）：`act -l` 能列出 4 个 job；否则跳过此项
    - 无原 job 被破坏：`diff <(grep -E '^  (go-test|web-build):' .github/workflows/ci.yml | sort) <(echo -e "  go-test:\n  web-build:")` 输出为空
  </acceptance_criteria>
  <done>ci.yml 追加两 job，yaml 合法，所有关键字面量（scripts 路径、阈值、setup）落地；原 job 零破坏。</done>
</task>

</tasks>

<verification>
```bash
# 1) yaml 合法
python3 -c 'import yaml; yaml.safe_load(open(".github/workflows/ci.yml"))'

# 2) 4 个 job
if command -v yq >/dev/null; then
  yq eval '.jobs | keys' .github/workflows/ci.yml
  # 预期：[go-test, web-build, perf-benchmark, image-size-regression]
fi

# 3) 关键字面量
grep -F 'scripts/perf-benchmark.sh --ci-mode' .github/workflows/ci.yml
grep -F '700 * 1024 * 1024' .github/workflows/ci.yml
grep -F 'scripts/verify-managed-image.sh' .github/workflows/ci.yml
grep -F 'scripts/gen-bench-tree.sh' .github/workflows/ci.yml

# 4) 原 job 保留
grep -F 'go-test:' .github/workflows/ci.yml
grep -F 'web-build:' .github/workflows/ci.yml

# 5) 可选：用 actionlint 做静态检查
if command -v actionlint >/dev/null; then
  actionlint .github/workflows/ci.yml
fi
```
</verification>

<success_criteria>
- Phase SC #1 CI 回归：每个 PR 自动跑 BASE-01 本地档 benchmark + 上传产物
- Phase SC #4：每个 PR 自动验证镜像未压缩 ≤ 700MB，镜像预算被显式 gate
- `.github/workflows/ci.yml` 合法 yaml，含 4 个 job
- 本 plan 不修改任何 shell 脚本（依赖 Plan 01 与 Phase 29）
- 不破坏既有 go-test / web-build（diff 验证）
</success_criteria>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| GitHub Actions runner → docker daemon | 公用 ubuntu-latest runner，`docker build` 仅构建镜像，不推送到任何 registry |
| Runner → artifact 存储 | `actions/upload-artifact` 上传 perf 报告；PR diff 者可下载 |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-35-04-01 | Information Disclosure | perf artifact 含 hostname / hardware info | accept | 仓库现为私有；artifact 保留 30 天自动过期；如开源需在 Plan 01 脚本内脱敏 |
| T-35-04-02 | Denial of Service | perf-benchmark 在 PR 洪泛时占用 runner 配额 | mitigate | 复用顶部 concurrency group `cancel-in-progress: true`（ci.yml L13-15），同一 PR 新 push 自动取消旧 run |
| T-35-04-03 | Tampering | PR 作者修改 perf-benchmark.sh 绕开 gate | mitigate | perf-benchmark.sh 由 Plan 01 落盘并走 code review；本 job 只调用脚本不内联逻辑，降低篡改 surface |
| T-35-04-04 | Tampering | PR 修改 image.lock 抬高 `local_dev_image_name` 到不受控仓库 | mitigate | image-size-regression job 仅基于本仓库 Dockerfile `docker build -f deploy/docker/managed-user/Dockerfile .` 本地 tag；不从外部 pull；tag 名来自 image.lock 但镜像来源仅仓库代码 |
| T-35-04-05 | Elevation of Privilege | docker build 被投毒 → 运行 `curl | sh` 安装 payload | accept | 既有 Dockerfile 审核范围，本 job 行为与 build-images.yml 完全一致；不引入新的 privilege boundary |
</threat_model>

<rollback>
- ci.yml 仅追加两个 job block，回滚 = `git checkout -- .github/workflows/ci.yml`
- 无 secrets 改动；无依赖新增
- 如果 perf-benchmark job 长时间跑挂（> 10min），可临时 `if: false` 在 job 节点禁用而非删除
</rollback>

<output>
After completion, create `.planning/phases/35-e2e/35-04-SUMMARY.md` documenting:
- ci.yml 最终 job 列表（4 个）与每个 job 行数
- 两个新 job 关键字面量引用位置（scripts 路径 + 阈值）
- 与 Plan 01 脚本的依赖约定（版本兼容性）
- 若 CI 真跑过（PR 上），附第一次成功 run 的 URL
</output>
