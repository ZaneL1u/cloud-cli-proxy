# Phase 52: 可观测性与诊断 - Context

**Gathered:** 2026-05-14
**Status:** Ready for planning
**Mode:** Smart discuss (autonomous, auto_rest accept-all)

<domain>
## Phase Boundary

把 e2e 失败时的「事后排障」做成一键工程：脚本统一收集容器日志 / 网络状态 / Docker 元信息 / Postgres dump / 系统状态，CI 自动 `actions/upload-artifact`，开发者无需手工 ssh 到 runner。

**3 plan 对应 3 条 OBS 不变量**：

- OBS-01 `tests/e2e/harness/collect-artifacts.sh` 脚本（可在失败 trap 中调用）
- OBS-02 artifact 目录结构（logs / network / docker / postgres / system 五个子目录 + 各 README 说明）
- OBS-03 CI workflow 在 e2e 失败时自动 `actions/upload-artifact@v4` 归档

**关键设计**：
- 本 phase **是收尾 phase**，不引入新生产代码，只增强测试 / CI 基础设施
- 完整 artifact 采集覆盖前 6 个 phase 的失败场景（bootstrap / 治理 / kill-switch / 防泄漏 / 代码层加固）
- 本地开发者也能 `bash tests/e2e/harness/collect-artifacts.sh ./out` 复用同一套采集逻辑

**不在本 phase 范围**：

- 内核级观测（Tetragon TracingPolicy）属 v2 范围
- 长保留 artifact 持久化（30 天 GitHub Actions 默认）
- 体积裁剪 / 压缩 / 增量上传

**macOS 本地约束**：`tests/e2e/` `//go:build e2e && linux` 已就绪；`collect-artifacts.sh` 是 bash，macOS 也能跑（dump 的内容部分 Linux-only，darwin 上能跑出空 / 缺项）。

</domain>

<decisions>
## Implementation Decisions

### Area 1: collect-artifacts.sh 脚本设计 (OBS-01)

- **位置**：`tests/e2e/harness/collect-artifacts.sh`，bash + POSIX 兼容（hosted ubuntu-24.04 + darwin local 都能跑）
- **接口**：
  ```bash
  bash tests/e2e/harness/collect-artifacts.sh <output-dir> [scenario-id]
  ```
  - `output-dir`：必填，目标目录（如 `./artifacts/`）
  - `scenario-id`：选填，e2e 用例标识，用于在 output-dir 下建子目录
- **错误容错**：每个采集子命令 `|| echo "采集 X 失败" >&2`，永远不返回非 0；缺工具时跳过，写空文件占位
- **集成点**：
  - **失败 trap**：每个用例 `defer` 内 `if t.Failed() { harness.CollectArtifacts(t, outputDir) }`，Go 内 `os/exec` 调脚本
  - **手动**：本地开发 `bash tests/e2e/harness/collect-artifacts.sh ./out scenario_xyz`
  - **CI**：`actions/upload-artifact@v4` 上传整个 output-dir
- **执行时间**：单 scenario 采集 ≤30s（避免拖慢 e2e）

### Area 2: artifact 目录结构 (OBS-02)

- **5 个子目录**：
  - `logs/` — 容器日志（`docker logs <container>` 全部 container）
  - `network/` — 网络状态（`ip link / ip addr / ip route / nft list ruleset / netstat -tln`）
  - `docker/` — Docker 元信息（`docker ps -a / docker network ls / docker inspect <containers>`）
  - `postgres/` — Postgres dump（`pg_dump --schema-only` + key 表 SELECT，避免敏感数据全量）
  - `system/` — 系统状态（`uname -a / free -m / df -h / dmesg --time-format=iso` 最近 100 行）
- **每个子目录都有 README.md**：
  - 说明里面是什么、什么时候有用
  - 包含中文「排障指引」段
- **总大小限制**：单 scenario ≤ 100MB（Phase 45 CI 配额对齐）；超过则裁剪 logs（最近 N MB）
- **生成时间戳**：output-dir 根放 `metadata.txt`，含 timestamp / git SHA / runner ID / kernel version

### Area 3: CI workflow 集成 (OBS-03)

- **修改 `.github/workflows/e2e.yml`**（Phase 45 已建）：
  - e2e job 失败时（`if: failure()`）自动跑 `tests/e2e/harness/collect-artifacts.sh ./artifacts`
  - `actions/upload-artifact@v4` 上传 `./artifacts` 整个目录，retention-days: 30，max-size: 100MB
  - artifact name: `e2e-artifacts-${{ github.run_id }}-${{ github.run_attempt }}`
- **PR 评论**：失败时通过 `actions/github-script` 在 PR 上贴 artifact 下载链接（Phase 45 决策一致）
- **本地开发 README**：在 `tests/e2e/README.md` 中加「排障」节，引导用 `bash tests/e2e/harness/collect-artifacts.sh ./out`

### Area 4: Phase 45 既有 DumpHook 整合

- **Phase 45 `tests/e2e/harness/dump.go`** 已就绪占位 hook + `WaitForTimeout` 写 `system/wait-timeout.txt`
- **本 phase 整合**：
  - 把 `DumpHook` 内部调用切换为 `collect-artifacts.sh` 子进程，统一采集路径
  - 现有 Phase 48 / 50 在 cleanup 内的 tcpdump pcap 收集逻辑挪到 `collect-artifacts.sh`（避免重复写采集代码）
- **既有 e2e 用例零破坏**：DumpHook 公开 API 不变，只换内部实现

### Area 5: 验证策略

- **VERIFICATION 策略**：
  - darwin 上手动跑 `bash tests/e2e/harness/collect-artifacts.sh ./out`，验证 5 子目录都被创建 + README 存在 + metadata.txt 存在
  - 跑一个**故意失败**的 e2e 用例（`tests/e2e/harness/collect_artifacts_test.go` 内的 fixture，darwin 也能跑）触发 DumpHook，验证 `./out` 内容
  - Linux 真机签字（在 CI runner 上跑 e2e 失败 + upload-artifact 工作）列 `human_verification`
- **Plan 粒度**：严格 3 plan
- **不引入新 Go 依赖**

### Claude's Discretion

- `pg_dump` 是否包含数据（建议 `--schema-only` + 几个 key 表 SELECT；含敏感数据的表跳过）
- `tcpdump pcap` 是否纳入 `network/` 子目录（建议是；Phase 48 已建 netshoot sidecar 路径）
- 失败时 dmesg 行数（建议最近 100 行，足够覆盖一次 e2e 周期）
- collect 脚本是否带 `-x` debug 模式（建议默认关，环境变量 `COLLECT_DEBUG=1` 打开）
- CI artifact 命名 + retention（30 天 / 100MB 已锁）

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets

- **Phase 45 `tests/e2e/harness/dump.go` + `artifacts.go`**：DumpHook 已就绪，本 phase 切换内部实现到 `collect-artifacts.sh`
- **Phase 45 `tests/e2e/harness/scenario/` `Scenario` 5 子目录占位**：本 phase 把占位扩成完整采集
- **Phase 48 netshoot sidecar tcpdump 路径**：本 phase 把 pcap 持久化挪进 `collect-artifacts.sh`
- **Phase 45 `.github/workflows/e2e.yml`**：本 phase 加 `if: failure()` artifact 上传
- **Phase 47-50 各 GoldenPath 方法**：本 phase 不动；脚本采集靠 docker exec / nft / ip 命令独立

### Established Patterns

- **bash + POSIX 兼容**：项目已有多个 shell 脚本（`scripts/uat-bypass-*.sh`、`scripts/lint-no-bare-sleep.sh`），本 phase 沿用
- **中文沟通**：README / metadata / 排障指引中文
- **错误容错**：采集脚本不允许 fail；缺工具时 log + 占位

### Integration Points

- **新增文件**：
  - `tests/e2e/harness/collect-artifacts.sh`（核心采集脚本）
  - `tests/e2e/harness/collect-artifacts_test.go`（脚本 fixture-driven 单测：跑脚本 + 检查目录结构，无 build tag，darwin 也跑）
  - `tests/e2e/harness/artifacts/logs/README.md`
  - `tests/e2e/harness/artifacts/network/README.md`
  - `tests/e2e/harness/artifacts/docker/README.md`
  - `tests/e2e/harness/artifacts/postgres/README.md`
  - `tests/e2e/harness/artifacts/system/README.md`
- **修改文件**：
  - `tests/e2e/harness/dump.go`（DumpHook 内部切换到 `collect-artifacts.sh`）
  - `.github/workflows/e2e.yml`（加 `if: failure()` artifact 上传）
  - `tests/e2e/README.md`（如不存在则新建）—— 加排障节
- **不引入新 Go 依赖**

</code_context>

<specifics>
## Specific Ideas

- **`collect-artifacts.sh` 骨架**：
  ```bash
  #!/usr/bin/env bash
  set -uo pipefail   # 注意：不带 -e，允许部分采集失败
  OUT="${1:?需要 output-dir}"
  SCENARIO="${2:-default}"
  ROOT="$OUT/$SCENARIO"
  mkdir -p "$ROOT/logs" "$ROOT/network" "$ROOT/docker" "$ROOT/postgres" "$ROOT/system"
  # ... 调用各子采集函数 ...
  ```
- **`metadata.txt` 内容**：
  ```
  timestamp=2026-05-14T07:30:00Z
  git_sha=abcd1234
  runner=ubuntu-24.04
  kernel=Linux 6.x.x
  scenario=bootstrap_golden_path
  ```
- **CI workflow 改动伪 yaml**：
  ```yaml
  - name: Collect e2e artifacts on failure
    if: failure()
    run: bash tests/e2e/harness/collect-artifacts.sh ./artifacts ${{ github.job }}
  - name: Upload artifacts
    if: failure()
    uses: actions/upload-artifact@v4
    with:
      name: e2e-artifacts-${{ github.run_id }}-${{ github.run_attempt }}
      path: ./artifacts
      retention-days: 30
      if-no-files-found: warn
  ```

</specifics>

<deferred>
## Deferred Ideas

- **Tetragon / 内核 oracle**：v2 范围
- **artifact 长保留 / S3 持久化**：30 天 GitHub Actions 默认足够 MVP
- **artifact 体积自动裁剪 / 压缩 / 增量上传**：本 phase 锁 100MB 上限，自动裁剪属性能优化
- **失败 trap 抓 core dump / minidump**：属 OS 级排障，本 phase 不做
- **Slack / 飞书通知**：本 phase 锁 PR 评论 + artifact 下载链接，IM 通知属后续
- **Linux runner 真机签字**：deferred-to-CI

</deferred>
