package app

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain Phase 51 QUAL-08：接入 goleak.VerifyTestMain 拦截控制面 app 包的
// goroutine 泄漏。
//
// IgnoreList 来源（首跑实测）：
//   - internal/broadcast.(*Hub).cleanupLoop：跨包包级 init 启动的 SSE 清理
//     goroutine（不退出，设计内）。
//
// 后续若 SQLite 连接或 sing-box 等被触达再追加。
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		goleak.IgnoreTopFunction("github.com/zanel1u/cloud-cli-proxy/internal/broadcast.(*Hub).cleanupLoop"),
	)
}
