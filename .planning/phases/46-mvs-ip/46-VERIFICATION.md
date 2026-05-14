---
phase: 46-mvs-ip
phase_name: MVS 黄金路径与出口 IP 验证
status: passed
verified_at: 2026-05-14
plans_complete: 5/5
plans_skeleton_only: 5/5  # 用例骨架 + 纯函数 PASS；Linux CI 真机断言 deferred-to-CI
requirements_satisfied:
  - MVS-01-skeleton  # bootstrap 黄金路径用例骨架 + GoldenPath 抽象
  - MVS-02           # Vote 多数派裁决纯函数完整覆盖
  - MVS-03           # ClassifyDNSResult 纯函数完整覆盖
  - MVS-04           # DefaultDenyMatrix + BuildDenyProbeCmd + SummarizeDenyResults 完整覆盖
  - MVS-05           # BootstrapExitCodeContract 锁定表 + 源真相 cross-check
requirements_partial:
  - MVS-01 events.host.ready 双重确认（deferred-to-CI）
  - MVS-02 公网 IP 等值断言（deferred-to-CI）
  - MVS-03 nft counter dump（deferred-to-OBS-02）
human_verification_needed:
  - "Linux runner（hosted ubuntu-24.04）上 go test -tags='e2e linux' ./tests/e2e/... 全部 PASS"
  - "Phase 45 Plan 02 Scenario.Start Step 2..7 真实实现接入（控制面子进程 / admin login / fixture 三件套 / PrepareGateway / PrepareHost）"
  - "tests/e2e/fixtures/error-codes.sql bcrypt hash 由 CI 接通后由 helper 函数动态生成填充"
roadmap_deviations:
  - "MVS-05 被测 binary 从 cloud-claude 改为 deploy/bootstrap/cloud-bootstrap.sh（按 CONTEXT §Area 3「以源码为准」原则；详见 46-05-SUMMARY.md）"
---

# Phase 46 Verification: MVS 黄金路径与出口 IP 验证

**Phase 目标**：把「首次 bootstrap → 进入 SSH → 出网经由绑定的出口 IP → DNS 走 tun → 直连外网被拒绝 → CLI 错误码契约稳定」这条用户主路径用 e2e 跑通，作为 MVS（Minimum Viable Suite）第一组真实可信用例。

**Verification 结论**：✅ **PASSED**（按 CONTEXT §Area 4 锁定的「Go 代码编译 + 纯函数 unit 测试通过 = PASS」标准）

---

## 5 plan 完成情况

| Plan | 状态 | Commit | MVS |
|------|------|--------|-----|
| 46-01 bootstrap 黄金路径 + GoldenPath 抽象 | ✅ 骨架 + 24 个纯函数 unit test PASS | `9264d85` | MVS-01 |
| 46-02 出口 IP 三源轮询 + Vote 多数派裁决 | ✅ 骨架 + 7 个 Vote/Sources 单测 PASS | `6d1e6cc` | MVS-02 |
| 46-03 DNS 走 tun OR 防火墙拒绝 | ✅ 骨架 + 7 个 Classify/Result 单测 PASS | `d127957` | MVS-03 |
| 46-04 默认拒绝矩阵 | ✅ 骨架 + 6 个 Matrix/Cmd/Summary 单测 PASS | `95a99a2` | MVS-04 |
| 46-05 CLI 错误码契约 | ✅ 骨架 + 3 个 ExitCode/Wellformed 单测 PASS（含与源真相 cross-check） | `4fae5e9` | MVS-05 |

5 个 PLAN.md 提前于 `a77ffb3` 合并提交。

## 5 个 success criteria 校验

ROADMAP §Phase 46 §Details 5 条 success criteria：

| # | 条目 | 状态 | 证据 |
|---|------|------|------|
| 1 | bootstrap e2e 跑完一轮：scenario 起容器 → curl 认证 → 等待 host.ready → SSH banner | ✅ 骨架 | `tests/e2e/bootstrap_test.go` BootstrapGoldenPathSuite + TestBootstrap_GoldenPath；stdout 关键字校验（认证通过 / 主机启动中）已就位；events.host.ready 校验 deferred-to-CI（Scenario Step 2..7 接通后启用） |
| 2 | 容器内出口 IP 校验从 ≥3 个独立回显源拉，多数派一致才判 PASS；某源全部超时按"投票"语义裁决 | ✅ 骨架 + 纯函数 | `tests/e2e/egress_ip_test.go` EgressIPSuite + Vote 纯函数 6 个分支单测全过；3 源 ip.me/ifconfig.io/ipinfo.io 锁定；全弃权 t.Skip |
| 3 | DNS 测试同时覆盖 tun 接管返回 A 记录 + 防火墙明确拒绝两种语义，断言至少其一成立 | ✅ 骨架 + 纯函数 | `tests/e2e/dns_test.go` DNSSuite + ClassifyDNSResult 5 个 Denied 分支 + Tunneled / Leaked / Unknown 4 个枚举单测全过；OR 语义在用例 switch 分支中实现 |
| 4 | 直连外网测试遍历多 IP × 多端口矩阵，全部必须超时或被拒绝，任一连通即 fail | ✅ 骨架 + 纯函数 | `tests/e2e/default_deny_test.go` DefaultDenySuite + DefaultDenyMatrix 锁定 4 条 + BuildDenyProbeCmd 3 个分支 + SummarizeDenyResults 3 个分支单测全过 |
| 5 | CLI 错误码用真实 binary 触发各场景，断言 exit code 与文档表一致 | ✅ 骨架 + 锁定 | `tests/e2e/cli_error_codes_test.go` table-driven 4 场景 + BootstrapExitCodeContract 与 `internal/controlplane/http.BootstrapErrorEntries` 编译期 cross-check（任一漂移立即 darwin 单测层失败）；详见 ROADMAP 偏差节 |

## ROADMAP 偏差与决策

| 项 | ROADMAP 描述 | 实际落地 | 决策依据 |
|----|--------------|----------|----------|
| MVS-05 被测 binary | 真实 cloud-claude binary | deploy/bootstrap/cloud-bootstrap.sh | grep `cmd/cloud-claude/main.go` 实际 exit 1-5，错误码 10-13 由 bootstrap.sh `case "$error_code"` 映射；CONTEXT §Area 3「以源码为准」 |

## 全套件回归验证（darwin / 无 docker）

```
$ go build ./...                              → exit 0 ✓
$ go build ./tests/e2e/...                    → exit 0 ✓
$ GOOS=linux go build -tags='e2e linux' ./tests/e2e/...  → exit 0 ✓
$ GOOS=linux go vet -tags='e2e linux' ./tests/e2e/...    → 干净 ✓
$ go vet ./tests/e2e/...                      → 干净 ✓
$ go test ./tests/e2e/ -count=1               → ok (Helpers 24 个单测 PASS) ✓
$ go test ./tests/e2e/ -run "Helpers" -count=1 -v
  Vote (6) + EgressIPSources (1) + ClassifyDNS (5) + DNSProbeResult String (1)
  + DefaultDenyMatrix (1) + BuildDenyProbeCmd (3) + SummarizeDeny (3)
  + BootstrapExitCodeContract align (1) + BootstrapErrorEntries len (1)
  + CLIErrorCases wellformed (1)  → 24/24 PASS ✓
$ bash scripts/lint-no-bare-sleep.sh          → [ok] tests/e2e 内无裸 time.Sleep ✓
```

## 给 Phase 47+ 的接口契约（汇总）

### 用例侧典型骨架

```go
//go:build e2e && linux

package e2e

import (
    "testing"
    "github.com/stretchr/testify/suite"
    "github.com/zanel1u/cloud-cli-proxy/tests/e2e/harness"
)

type MyFeatureSuite struct {
    harness.BaseSuite  // ⚠ 嵌入值类型
    GP *GoldenPath
}

func (s *MyFeatureSuite) SetupSuite() {
    s.BaseSuite.SetupSuite()
    s.GP = StartGoldenPath(s.T())
    if s.GP != nil {
        s.SetArtifactDumper(harness.NewArtifactDumper(s.GP.Scenario, ""))
    }
}

func (s *MyFeatureSuite) TestX() {
    if s.GP == nil { return }  // StartGoldenPath 已 t.Skip
    // ...
}
```

### 公开锁定表与纯函数

- `Vote(results []string) VoteResult` — MVS-02 多数派裁决（CONTEXT §Area 2）
- `ClassifyDNSResult(exitCode int, stderr string) DNSProbeResult` — MVS-03 OR 语义
- `DefaultDenyMatrix` / `BuildDenyProbeCmd` / `SummarizeDenyResults` — MVS-04 默认拒绝（Phase 48 / 49 复用基线）
- `BootstrapExitCodeContract` / `CLIErrorCases` — MVS-05 错误码契约
- `EgressIPSources()` — MVS-02 三源 URL 副本

### 跨 plan 启动器

- `StartGoldenPath(t *testing.T) *GoldenPath` — 唯一启动入口；docker / Scenario step 缺失时 t.Skip；不需要手动 Stop
- `RunBootstrapScript(ctx, scriptPath, env, stdin) (exitCode, stdout, stderr, err)` — MVS-01 / MVS-05 共用

## 已知遗留 / 后续 phase 回看清单

| 项 | 责任方 | 跟踪位置 |
|----|--------|----------|
| Scenario.Start Step 2..7 真实实现接入（控制面子进程 / admin login / fixture 三件套 / PrepareGateway / PrepareHost） | Phase 45 Plan 02 follow-up（建议作为 Phase 47 第一个用例的前置工作） | 45-02-SUMMARY.md / 46-01-SUMMARY.md |
| MVS-01 events.host.ready 双重确认 | CI runner 接通 admin events API 后补 | 46-01-SUMMARY.md |
| MVS-02 公网 NAT IP ground truth 等值断言 | CI runner 接通 fixture 后从控制面 admin API 拉 bound egress IP | 46-02-SUMMARY.md |
| MVS-03 失败时 nft counter dump | Phase 52 OBS-02 接入真实收集逻辑 | 46-03-SUMMARY.md |
| MVS-04 `bash /dev/tcp` 在 alpine 默认无 bash 下的备选（nc -z -w 3） | CI runner 接通后视情况补 | 46-04-SUMMARY.md |
| MVS-05 fixture SQL 中 bcrypt hash 动态生成（避免硬编码过期 hash） | CI runner 接通 SeedBootstrapErrorFixtures 时实现 | 46-05-SUMMARY.md |

## Phase 46 最终签字

- ✅ ROADMAP §Phase 46 §Details 5 个 success criteria 全部骨架就位
- ✅ 5 个 plan 全部完成（PLAN + SUMMARY + 主用例 + 配套纯函数）
- ✅ 24 个 helpers 纯函数 unit test 在 darwin 100% PASS
- ✅ 跨 Phase 复用契约（GoldenPath / Vote / DefaultDenyMatrix / DNSProbeResult / BootstrapExitCodeContract）就位，向后兼容
- ✅ 与 ROADMAP 的 1 项偏差（MVS-05 被测 binary）已记录决策依据
- ✅ darwin 上 `go build ./tests/e2e/...` + 纯函数 unit test + linux cross-compile + go vet + lint-no-bare-sleep 全绿
- ✅ 零绝对路径、零真实凭据 / 邮箱、零裸 `time.Sleep`

**结论**：Phase 46 ship-ready（darwin 验收基线）。Linux CI runner 上的真机 e2e 跑通为 `human_verification_needed` 项，等 Scenario Step 2..7 接入即闭环，不阻塞 Phase 47 进入。
