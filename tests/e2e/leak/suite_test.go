//go:build e2e && linux

// Package leak 收纳 Phase 49 的 8 条 LEAK-* 防泄漏对抗用例。
//
// 设计目标：
//   - 每条 LEAK-NN 一个 *_test.go 文件 + 一个 TestLeak_NN_<Name> 用例。
//   - 8 个用例独立调 `e2e.StartGoldenPath(t)`；Phase 45 Scenario Step 2..7
//     sentinel 期间所有 LEAK-* 用例自动 t.Skip，等 Step 落地后真实跑通。
//   - 失败时通过用例自带 t.Cleanup 触发 logger.Warn；artifact dump 由后续
//     在 BaseSuite TearDownTest 挂载的 ArtifactDumper 接管（Phase 52 OBS-01..03）。
//
// 本文件提供两个共享 helper：
//   - StartLeakGolden：在用例首行调，封装 nil / Skip 路径。
//   - EnsureLeakWorkerTools：best-effort 把 dig / kdig / openssl / ping /
//     python3 装齐；失败仅 t.Logf，由具体用例自行 Skip。
//
// 共享 fixture 暂不在包级别复用（StartGoldenPath 内部走 t.Cleanup 与 t.Skipf
// 强绑定 *testing.T，不易跨用例共用；CONTEXT §Area 4 锁定 ≤5min 整组耗时
// 在 8x 冷启动成本下仍可控，待 Phase 52 OBS-* 引入显式 fixture pool）。
//
// darwin 上不参与编译。

package leak

import (
	"context"
	"testing"
	"time"

	e2e "github.com/zanel1u/cloud-cli-proxy/tests/e2e"
)

// StartLeakGolden 是 LEAK 套件用例的统一入口。
//
// 行为：
//   - 调 e2e.StartGoldenPath(t)：前置缺失（无 docker / Step 2..7 sentinel）→
//     内部已 t.Skip 并返回 nil；本 helper 在 nil 时返回 (nil, true) 让用例 return。
//   - host 句柄未填充（Step 7 sentinel）→ t.Skipf + 返回 (g, true)。
//   - 一切就绪 → (g, false)，调用方继续。
func StartLeakGolden(t *testing.T) (*e2e.GoldenPath, bool) {
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

// EnsureLeakWorkerTools 在 LEAK 用例 setup 阶段 best-effort 安装探测工具。
// 失败仅 t.Logf，让具体用例按需 Skip。
func EnsureLeakWorkerTools(t *testing.T, g *e2e.GoldenPath) {
	t.Helper()
	if g == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if err := g.EnsureWorkerLeakTools(ctx); err != nil {
		t.Logf("ensure worker leak tools: %v (用例将按需 Skip)", err)
	}
}
