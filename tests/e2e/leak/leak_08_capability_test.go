//go:build e2e && linux

// leak_08_capability_test.go 是 Phase 49 LEAK-08 的 e2e 主用例：
//
//   - `cat /proc/1/status` 在 worker 容器内取 4 行 Capability 掩码。
//   - 解析后 CapEff / CapBnd 都不能含 CAP_NET_RAW / CAP_NET_ADMIN / CAP_SYS_ADMIN。
//
// 当前 worker.go:217-218 显式 `--cap-add NET_ADMIN --cap-add SYS_ADMIN`，且未
// 显式 `--cap-drop NET_RAW`（docker 默认含 NET_RAW）。本用例**预期 fail**，
// 用 t.Errorf 不阻塞其它 LEAK 用例；VERIFICATION 标 backend GAP（Phase 51 QUAL-06）。

package leak

import (
	"context"
	"testing"
	"time"

	e2e "github.com/zanel1u/cloud-cli-proxy/tests/e2e"
)

func TestLeak_08_WorkerCapabilities_Locked(t *testing.T) {
	g, skip := StartLeakGolden(t)
	if skip {
		return
	}
	EnsureDumper(t, g)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	raw, err := g.GetProcCapabilities(ctx, 1)
	if err != nil {
		t.Fatalf("read /proc/1/status: %v", err)
	}

	caps, err := e2e.ParseProcCapabilities(raw)
	if err != nil {
		t.Fatalf("parse /proc/1/status capability lines: %v\nraw=%s", err, raw)
	}

	var failures []string
	for _, dangerous := range e2e.LeakDangerousCaps {
		if caps.Eff[dangerous] {
			failures = append(failures, "CapEff:"+dangerous)
		}
		if caps.Bnd[dangerous] {
			failures = append(failures, "CapBnd:"+dangerous)
		}
	}

	t.Logf("LEAK-08 caps Eff=%v Bnd=%v failures=%v", caps.Eff, caps.Bnd, failures)

	if len(failures) > 0 {
		t.Errorf(
			"LEAK-08 worker /proc/1/status 命中危险 capability：%v。"+
				"backend GAP：internal/runtime/tasks/worker.go:217-218 显式 --cap-add NET_ADMIN/SYS_ADMIN，"+
				"且未显式 --cap-drop NET_RAW（docker 默认 capability 集合含 NET_RAW）。"+
				"修复方案见 Phase 51 QUAL-06：去掉 --cap-add SYS_ADMIN，把 NET_ADMIN 改为运行时 setcap，"+
				"并显式 `--cap-drop NET_RAW`。",
			failures)
	}
}
