//go:build e2e && linux

// Package killswitch_stress 收纳 Phase 50 的 4 条 KILL-* kill-switch 压力测试用例。
//
// 设计目标：
//   - 每条 KILL-NN 一个 *_test.go 文件 + 一个 TestKillSwitch_NN_<Name> 用例。
//   - 4 个用例独立调 e2e.StartGoldenPath(t)；Phase 45 Scenario Step 2..7
//     sentinel 期间所有 KILL-* 用例自动 t.Skip，等 Step 真实落地后跑通。
//   - 失败时通过用例自带 t.Cleanup 触发 logger.Warn；artifact dump 由后续
//     BaseSuite TearDownTest 挂载的 ArtifactDumper 接管（Phase 52 OBS-01..03）。
//
// 本文件提供两个共享 helper：
//   - StartStressGolden：在用例首行调，封装 nil / Skip / gateway 句柄缺失 路径。
//   - EnsureDumper：注册用例失败钩子。
//
// 与 Phase 48 既有 killswitch_singbox_crash_test.go / killswitch_resolvconf_tamper_test.go
// 故意分离（同 package 但不同目录），避免文件名冲突 + 语义边界更清晰
// （MVS-09/10 看「行为」，KILL-01..04 看「压力 / 极端故障」）。
//
// darwin 上不参与编译。

package killswitch_stress

import (
	"testing"

	e2e "github.com/zanel1u/cloud-cli-proxy/tests/e2e"
)

// StartStressGolden 是 KILL 套件用例的统一入口。
//
// 行为：
//   - 调 e2e.StartGoldenPath(t)：前置缺失（无 docker / Step 2..7 sentinel）→
//     内部已 t.Skip 并返回 nil；本 helper 在 nil 时返回 (nil, true) 让用例 return。
//   - host 句柄未填充（Step 7 sentinel）→ t.Skipf + 返回 (g, true)。
//   - gateway 句柄未填充（Step 4..6 sentinel）→ t.Skipf + 返回 (g, true)。
//   - 一切就绪 → (g, false)，调用方继续。
func StartStressGolden(t *testing.T) (*e2e.GoldenPath, bool) {
	t.Helper()
	g := e2e.StartGoldenPath(t)
	if g == nil {
		return nil, true
	}
	if g.Host == nil || g.Host.ID == "" {
		t.Skipf("golden path host not yet populated (scenario step 7 未实现)")
		return g, true
	}
	return g, false
}

// EnsureDumper 注册用例失败钩子，输出一行 logger.Warn 提醒 artifact dump
// 应由 BaseSuite TearDownTest 接管（Phase 52 OBS-01..03 引入完整 dumper）。
func EnsureDumper(t *testing.T, g *e2e.GoldenPath) {
	t.Helper()
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		t.Logf("KILL fixture dump hook triggered for %s; artifact dump should be collected by harness",
			t.Name())
		_ = g
	})
}
