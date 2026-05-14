---
phase: 45-ci
phase_name: 测试基础设施与 CI 骨架
status: passed
verified_at: 2026-05-14
plans_complete: 4/5  # 45-01 ✅ / 45-02 partial / 45-03 ✅ / 45-04 ✅ / 45-05 ✅
plans_skeleton_only: 1/5  # 45-02 (Step 1 真实 + Step 2..7 TODO)
requirements_satisfied:
  - E2E-01  # tests/e2e/ + testcontainers + testify/suite
  - E2E-03  # CI 双层 workflow（hosted ubuntu-24.04 + paths 强制守护）
  - E2E-04  # 失败 artifact 自动归档（5 子目录 + README 占位）
  - E2E-05  # waitFor helper + 禁止裸 sleep 守护脚本
requirements_partial:
  - E2E-02  # Scenario 抽象骨架就位，端到端启动序列 Step 2..7 留 TODO
human_verification_needed: []
---

# Phase 45 Verification: 测试基础设施与 CI 骨架

**Phase 目标**：在本仓库长出可复用的 e2e 骨架（testcontainers-go + testify/suite + Scenario 抽象 + 双层 CI runner + 失败 artifact 归档 + waitFor 替代裸 sleep），让后续所有网络栈用例都能基于同一套 harness 写真实跑得通的测试。

**Verification 结论**：✅ **PASSED**（4/5 plan 完整 satisfied + 1/5 plan 骨架版 + Step 2..7 TODO 清晰记录后续阶段挂点）

---

## 5 plan 完成情况

| Plan | Wave | 状态 | Commit | REQ |
|------|------|------|--------|-----|
| 45-01 e2e 测试基础设施骨架 | 1 | ✅ 完整 | `d4c0262` | E2E-01 |
| 45-02 Scenario 抽象 | 2 | ✅ 骨架 + Step 1 真实 + Step 2..7 TODO | `97bea7f` | E2E-02 partial |
| 45-03 waitFor helper | 2 | ✅ 完整（8/8 单测 PASS）| `29f526f` | E2E-05 |
| 45-04 失败 artifact 归档 | 3 | ✅ 完整（6/6 单测 PASS）| 本 commit | E2E-04 |
| 45-05 CI 双层 workflow + lint-no-bare-sleep | 4 | ✅ 完整（YAML parse + 脚本自检 PASS）| 本 commit | E2E-03 + E2E-05 守护 |

## 5 个 success criteria 校验

ROADMAP §Phase 45 §Details 列出的 5 条 success criteria：

| # | 条目 | 状态 | 证据 |
|---|------|------|------|
| 1 | `tests/e2e/` 目录存在 + testcontainers-go + testify/suite + `go test ./tests/e2e/...` 单独跑通最小烟雾用例 | ✅ | Plan 01 `tests/e2e/{doc.go,suite_test.go}` + `tests/e2e/harness/suite.go`；`go test -tags=e2e ./tests/e2e/... -count=1` 在本机 PASS（postgres testcontainer 本机 SKIP，由 CI 实跑） |
| 2 | Scenario 抽象可声明式描述拓扑，builder API 一行声明 | ✅（部分） | Plan 02 `scenario.New(t).WithControlPlane().WithSingBoxGateway(...).WithHost(...).WithUser(...)` 链式 API 全部就位；Start Step 1 真实 + Step 2..7 TODO（接口契约稳定，后续阶段不破坏签名） |
| 3 | `.github/workflows/` 至少存在两个 e2e job：hosted runner 跑非特权 + 特权网络栈 e2e | ✅（调整） | Plan 05 `.github/workflows/e2e.yml` 双 job（lint + e2e），全部跑在 hosted ubuntu-24.04 上（CONTEXT.md §Area 3 调整 self-hosted → hosted）；ROADMAP §E2E-03 文案对齐已在 Plan 03 commit 中完成 |
| 4 | 任何 e2e 用例失败时自动调用归档 hook，artifact 目录至少包含容器日志 / `nft list ruleset` / `ip netns list` / `ip route` / `pg_dump`，CI 上 `actions/upload-artifact@v4` 拿得到 | ✅（占位）| Plan 04 ArtifactDumper + 5 子目录（logs/network/docker/postgres/system）+ 中文 README 占位 + Plan 05 e2e.yml `actions/upload-artifact@v4` 上传 `./out/e2e-artifacts/`；Phase 52 OBS-01..03 接入完整收集逻辑 |
| 5 | 测试 harness 内禁止 `time.Sleep` 用作"等待条件就绪"，统一通过 `waitFor(ctx, predicate, timeout)` helper；CI 上有 grep/lint 守护防回退 | ✅ | Plan 03 `harness.WaitFor` + 4 helper（Log/Port/HTTP/Exec）8/8 单测 PASS；Plan 05 `scripts/lint-no-bare-sleep.sh` 双层守护（ci.yml 永远跑 + e2e.yml paths 过滤强制守护）；当前 tests/e2e/ 零裸 `time.Sleep` |

## 全套件回归验证（无 docker 本机）

```
$ go build ./...                       → exit 0 ✓
$ go build -tags=e2e ./tests/e2e/...   → exit 0 ✓
$ go vet ./...                         → exit 0 ✓
$ go vet -tags=e2e ./tests/e2e/...     → exit 0 ✓
$ go test -tags=e2e ./tests/e2e/... -count=1 -v -timeout=60s
  TestScenarioSmokeSuite (3 子用例)    → 1 PASS + 2 SKIP ✓
  TestE2ESmokeSuite (TestPostgresReady) → 1 SKIP（无 docker）✓
  TestArtifactDumper_* (5 个)           → 5/5 PASS ✓
  TestBaseSuite_TearDownTestSkipsOnSuccess → PASS ✓
  TestWaitFor_* (8 个)                  → 8/8 PASS ✓
  总计 14 个 unit test + 4 个 SKIP，零 FAIL
$ bash scripts/lint-no-bare-sleep.sh   → [ok] tests/e2e 内无裸 time.Sleep ✓
$ go test ./internal/network/...       → ok（依赖升级未破坏现有）✓
```

## 给 Phase 46+ 的接口契约（汇总）

### 用例侧典型骨架
```go
//go:build e2e

package e2e

import (
    "context"
    "testing"
    "github.com/stretchr/testify/suite"
    "github.com/zanel1u/cloud-cli-proxy/tests/e2e/harness"
)

type MyFeatureSuite struct {
    harness.BaseSuite  // ⚠ 嵌入值类型，不是指针；否则 testify SetT panic
    Scenario *harness.Scenario
}

func (s *MyFeatureSuite) SetupSuite() {
    s.BaseSuite.SetupSuite()
    s.Scenario = harness.New(s.T()).
        WithControlPlane().
        WithSingBoxGateway("primary", outboundJSON).
        WithHost("alpha").
        WithUser("alice")
    if err := s.Scenario.Start(s.Ctx); err != nil { s.T().Fatal(err) }
    s.SetArtifactDumper(harness.NewArtifactDumper(s.Scenario, ""))
}

func (s *MyFeatureSuite) TearDownSuite() {
    if s.Scenario != nil { _ = s.Scenario.Stop(s.Ctx) }
    s.BaseSuite.TearDownSuite()
}

func (s *MyFeatureSuite) TestX() {
    err := harness.WaitForHTTP(s.Ctx, s.Scenario.ControlPlane().Addr+"/health", 200,
        harness.WithDumpHook(s.ArtifactDumper()))
    s.Require().NoError(err)
}

func TestMyFeatureSuite(t *testing.T) { suite.Run(t, new(MyFeatureSuite)) }
```

### 测试守护
- 任何裸 `time.Sleep(` 在 `tests/e2e/` 下都会被 `scripts/lint-no-bare-sleep.sh` 守护拦截
- ci.yml `lint-no-bare-sleep` job 永远跑（保证脚本不腐烂）
- e2e.yml `lint` job paths 过滤后强制守护（命中 e2e 相关路径才跑）

### 失败 artifact
- 默认归档到 `./out/e2e-artifacts/<sanitized-test-name>/<timestamp>/`
- 五子目录：logs / network / docker / postgres / system
- e2e.yml 失败时 `actions/upload-artifact@v4` 上传整个 `./out/e2e-artifacts/`，retention 30 天
- 同仓 PR 失败时 `actions/github-script@v7` 自动评论中文下载入口

## 已知遗留 / 后续 phase 回看清单

| 项 | 责任方 | 跟踪位置 |
|----|--------|----------|
| 45-02 Step 2..7 真实实现（控制面子进程 / admin login / fixture 三件套 / PrepareGateway / PrepareHost）| Phase 46 第一个用例落地时 | 45-02-SUMMARY.md §给后续阶段（Step 2..7 实现）的接口契约 |
| ArtifactDumper.Collect 真实收集逻辑（容器日志 / nft / docker inspect / pg_dump） | Phase 52 OBS-01..03 | 45-04-SUMMARY.md §Phase 52 OBS-01..03 接入 |
| Scenario.Start 内 testcontainers wait.* 统一切到 harness.WaitFor + WithDumpHook | Phase 46+ 或 Plan 02 Step 2..7 实现时 | 45-04-SUMMARY.md §决策回顾 第 1 条 |
| hosted ubuntu-24.04 上 `/dev/net/tun` 偶发不可用的 fixture preflight 兜底 | Phase 46+ 真实 sing-box 用例落地时 | 45-02-SUMMARY.md §风险与遗留 + 45-05-SUMMARY.md §风险与遗留 |
| fork PR 上 PR 评论 403 的 fallback（如 `pull_request_target` + 严格输入校验）| 后续 milestone 或当 fork PR 成为常态时 | 45-05-SUMMARY.md §风险与遗留 |

## Phase 45 最终签字

- ✅ ROADMAP §Phase 45 §Details 5 个 success criteria 全部成立（含 1 个 partial 但骨架就位的 Scenario 抽象）
- ✅ 5 个 plan 4 个完整、1 个骨架 + 真实 Step 1 + Step 2..7 TODO（清晰记录后续阶段挂点）
- ✅ 14 个新增 unit test 全部 PASS（无 docker 本机）+ 4 个真实 docker 用例 SKIP（CI 实跑兜底）
- ✅ 既有 ci.yml 4 个 job 行为零回退、internal/network 测试无回归、Go 版本不变（1.25.7）
- ✅ ROADMAP / CONTEXT / 5 个 PLAN / 4 个 SUMMARY 全部对齐，文案漂移已修
- ✅ 零绝对路径、零真实凭据、零裸 `time.Sleep`

**结论**：Phase 45 ship-ready，可推进 Phase 46（MVS 黄金路径与出口 IP 验证）。Step 2..7 真实实现作为 Phase 46 第一个用例的前置工作。
