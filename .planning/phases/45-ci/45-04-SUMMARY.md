---
phase: 45-ci
plan: 04
subsystem: tests/e2e/harness
tags: [artifact-dump, dump-hook-impl, base-suite-integration]
provides:
  - artifact-dumper
  - 5-subdir-skeleton (logs/network/docker/postgres/system)
  - basesuite-teardown-test-failure-collect
  - dump-hook-static-assertion
requires:
  - tests/e2e/harness/dump.go (Plan 03 DumpHook 接口)
  - tests/e2e/harness/suite.go (Plan 01 BaseSuite + Plan 02 SetupSuite 修复)
affects:
  - tests/e2e/harness/artifacts.go
  - tests/e2e/harness/artifacts_test.go
  - tests/e2e/harness/dump.go (追加静态断言)
  - tests/e2e/harness/suite.go (新增 dumper 字段 + Set/Get + TearDownTest 改造)
tech-stack:
  added: []
  patterns:
    - "Phase 52 OBS-01..03 接入挂点：5 子目录 + 中文 README 占位"
    - "DumpHook 接口静态断言 var _ DumpHook = (*ArtifactDumper)(nil)"
    - "环境变量 > 编译默认 配置解析模式（CLOUD_CLI_PROXY_E2E_ARTIFACT_DIR > ./out/e2e-artifacts）"
key-files:
  created:
    - tests/e2e/harness/artifacts.go
    - tests/e2e/harness/artifacts_test.go
  modified:
    - tests/e2e/harness/dump.go
    - tests/e2e/harness/suite.go
decisions:
  - "ArtifactDumper.scenario 字段保留指针但不使用：Phase 52 OBS-01..03 接入时从 scenario 读 GatewayHandle.ContainerID / HostHandle.ContainerName 决定收集哪些容器；当前阶段 scenario 可为 nil"
  - "Plan 04 不强制改造 Plan 02 Scenario.Start 内的 WaitFor 调用点（建议留作 Plan 05 收尾时的优化）— 当前 Scenario.Start Step 1 用 testcontainers 自带 wait.ForLog；Step 2..7 实现时再统一切到 harness.WaitFor + WithDumpHook"
  - "fixture 目录结构锁定 5 子目录：logs / network / docker / postgres / system；Phase 52 不允许变更子目录命名（破坏 README 与 e2e.yml upload-artifact 路径契约）"
  - "TearDownTest 失败分支调 Collect 但不阻塞测试：dump 失败仅 logger.Warn，避免 'dump 自身失败 → 用例假阳成功'"
  - "TestArtifactDumper_EnvOverrideRespected 用 t.TempDir() 拼接子串作为 override 路径，不硬编码 /tmp/e2e-override-XYZ；守住 CONVENTIONS.md 禁绝对路径约定（即使是测试代码）"
metrics:
  duration: 约 30 分钟
  tasks_completed: 3/3
  files_modified: 4
  commits: 1（与 Plan 05 + Phase 45 VERIFICATION 合并提交）
  completed_at: 2026-05-14
requirements_satisfied:
  - E2E-04
---

# Phase 45 Plan 04: 失败 artifact 归档 hook Summary

## One-liner

实现 `tests/e2e/harness/artifacts.go` —— ArtifactDumper 把失败用例的排障证据按 `<baseDir>/<sanitizedName>/<timestamp>/` 归档到 `./out/e2e-artifacts/`（可被 `CLOUD_CLI_PROXY_E2E_ARTIFACT_DIR` 覆盖），自动建 5 子目录（logs/network/docker/postgres/system）+ 5 份中文 README 占位说明 Phase 52 OBS-01..03 接入后会写什么；同时实现 `OnWaitForTimeout` 把 waitFor 超时备忘录追加到 `system/wait-timeout.txt`。改造 `BaseSuite.TearDownTest` 在 `t.Failed()` 时自动调 `Collect`（dumper 通过 `SetArtifactDumper` 注入）。6 个新 unit test 全 PASS（无 docker 依赖，0.00s）。

## 实际产出

| 文件 | 性质 | 关键内容 |
|------|------|----------|
| `tests/e2e/harness/artifacts.go` | 新建 | `ArtifactDumper` + `Collect` + `OnWaitForTimeout` + `EnvArtifactBaseDir`/`DefaultArtifactBaseDir`/`ArtifactSubdirs` 常量 + `sanitizeName` / `defaultBaseDir` / `readmeContentFor` helper |
| `tests/e2e/harness/artifacts_test.go` | 新建 | 6 个 TestArtifactDumper_* / TestBaseSuite_* 单测 |
| `tests/e2e/harness/dump.go` | 修改 | 末尾追加 `var _ DumpHook = (*ArtifactDumper)(nil)` 静态接口断言 |
| `tests/e2e/harness/suite.go` | 修改 | BaseSuite 加 `dumper` 字段 + `SetArtifactDumper` / `ArtifactDumper` 方法 + `TearDownTest` 失败时自动 Collect |

## 验证结果

| 验证 | 命令 | 结果 |
|------|------|------|
| 6 个新 unit test 全 PASS | `go test -tags=e2e ./tests/e2e/harness/ -count=1 -run 'TestArtifact\|TestBaseSuite' -v` | 6/6 PASS in <1s ✓ |
| Plan 03 既有 8 个 waitfor 测试无回归 | `go test -tags=e2e ./tests/e2e/harness/ -count=1 -run TestWaitFor -v` | 8/8 PASS ✓ |
| 编译通过 | `go build -tags=e2e ./tests/e2e/...` | exit 0 ✓ |
| 默认路径不受影响 | `go build ./...` | exit 0 ✓ |
| `go vet -tags=e2e` | `go vet -tags=e2e ./tests/e2e/...` | exit 0 ✓ |
| 零 `time.Sleep` | `grep -rnE '^\s*time\.Sleep\(' tests/e2e/` | 无命中 ✓ |
| 无绝对路径 | `grep -E '/Users/\|/home/' tests/e2e/harness/artifacts.go` | 无命中 ✓ |
| 静态接口断言 | `grep 'var _ DumpHook = (\*ArtifactDumper)(nil)' tests/e2e/harness/dump.go` | 命中 ✓ |

## 给后续 plan / phase 的接口契约

### Phase 52 OBS-01..03 接入
- 扩展 `ArtifactDumper.Collect` 在 mkdir 后追加各子目录的真实收集逻辑：
  - `logs/`：从 `s.scenario.Host(name).ContainerName` 读容器日志
  - `network/`：`nft list ruleset` / `ip netns list` / `ip route`
  - `docker/`：`docker ps` / `docker inspect`
  - `postgres/`：`pg_dump` 关键表
  - `system/`：`dmesg` / `proc/meminfo` / `kernel-version`
- DumpHook 接口签名稳定，不需要再变（`OnWaitForTimeout(ctx, name, lastErr) error`）
- README 占位会被覆盖（Phase 52 实现时按需保留或换内容）

### Plan 02 后续阶段（Step 2..7 实现时）
- 在 Scenario.Start 内部所有 WaitFor 调用点用 `WithDumpHook(harness.NewArtifactDumper(s, ""))` 注入真实 dumper
- 这样 e2e 用例在 SetupSuite 一次 `s.SetArtifactDumper(harness.NewArtifactDumper(scenario, ""))` 即可让 Scenario.Start + 用例 WaitFor 都共享同一 dumper

### Plan 05 e2e.yml
- artifact 上传路径锁定 `./out/e2e-artifacts/`（与 `DefaultArtifactBaseDir` 字面量一致）
- 环境变量 `CLOUD_CLI_PROXY_E2E_ARTIFACT_DIR` 可在 e2e job 的 env 段覆盖（保留 self-hosted runner 切换持久卷的扩展点）

## 决策回顾

1. **Plan 04 不主动改 Scenario.Start 内的 WaitFor**：
   - PLAN 04 Task 2 明确「仅暴露 BaseSuite.SetArtifactDumper 让用例侧能注入」
   - Scenario.Start 内部批量改造（把 testcontainers wait.ForLog 全换成 harness.WaitFor + WithDumpHook）作为后续可选优化
   - 当前阶段保持 Plan 02 / Plan 03 / Plan 04 三方解耦
2. **5 子目录命名锁死**：
   - logs / network / docker / postgres / system 与 REQUIREMENTS.md §OBS-02 1:1 对齐
   - Phase 52 接入时不允许改子目录名（会破坏 README 与 e2e.yml upload-artifact path）
3. **`scenario` 字段保留但当前不用**：
   - 留作 Phase 52 OBS-01..03 扩展挂点（从中读 GatewayHandle.ContainerID 决定收集哪些容器）
   - 当前 unit test 用 `nil` 传入，证明无 scenario 也能用（dumper 是无状态的）

## 风险与遗留

- **失败路径 Collect 的 e2e 集成验证**：本 plan 没写 `TestBaseSuite_TearDownTestCollectsOnFailure`（PLAN 04 Task 3 第 g 条建议跳过），原因是让 `t.Failed()` 返回 true 会污染外层测试自身。失败路径的真实端到端验证留给 Plan 05 e2e CI 跑 ScenarioSmokeSuite 顺带覆盖（当 Step 2..7 实现且某个用例失败时）
- **OnWaitForTimeout 错误未传播到 caller**：当前 hook 失败时 `OnWaitForTimeout` 返回 error，WaitFor 把它合并到 timeout 错误文案；但 BaseSuite.TearDownTest 调 `Collect` 失败时只 logger.Warn 不返回 → 用例不会因 dump 失败而 fail。这是 deliberate trade-off（避免雪崩），文档化即可
- **artifact 路径在 macOS / Linux 都用 forward slash**：`filepath.Join` 在 Windows 会用 `\`，但 v3.6 e2e 套件不支持 Windows，问题不存在

## 完成度

- ✅ ArtifactDumper + Collect + OnWaitForTimeout 全部就位
- ✅ 5 子目录契约 + 中文 README 占位 + sanitizeName / defaultBaseDir
- ✅ DumpHook 静态断言 + BaseSuite 集成
- ✅ 6 个新 unit test 全 PASS
- ✅ Plan 03 既有 8 个 waitfor 测试无回归
- ✅ E2E-04 需求 satisfied
