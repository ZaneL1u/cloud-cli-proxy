package network

import (
	"os"
	"strings"
	"testing"
)

// TestWorker_CreateHost_CallOrder 守护 Phase 45 Plan 02 引入的调用顺序硬约束：
//
//	PrepareGateway → buildCreateArgs → docker create → docker start → PrepareHost
//
// 通过读 worker.go::createHost 的源码文本，找到关键标识符首次出现的字节位置，
// 按位置严格升序断言。任何顺序回归（例如有人把 PrepareGateway 挪到 docker
// create 之后）都会立即在 CI 失败。
//
// 用文本断言而非反射 / spy 是最低保底方案：worker.Worker 依赖 docker / 系统
// repo，构造代价过高；静态文本断言对所有平台 / build tag 都跑得通，故本文件
// **不带** build tag，确保在 macOS 开发机也能本地跑通。
func TestWorker_CreateHost_CallOrder(t *testing.T) {
	// 测试工作目录是 internal/network/，worker.go 在仓库根 internal/runtime/tasks/。
	src, err := os.ReadFile("../runtime/tasks/worker.go")
	if err != nil {
		t.Fatalf("read worker.go: %v", err)
	}
	content := string(src)

	// 定位 createHost 函数体起始与结束（match `func (w *Worker) createHost(` ...
	// 下一个顶层 `func ` 之前）。这样关键标识符的搜索只在函数体内进行，避免误
	// 抓到 startHost / rebuildHost 中的 PrepareGateway / PrepareHost 调用。
	startSig := "func (w *Worker) createHost("
	startIdx := strings.Index(content, startSig)
	if startIdx < 0 {
		t.Fatalf("createHost 函数签名未找到")
	}
	// 函数体结束 ≈ 下一个顶层 "\nfunc " 出现的位置
	rest := content[startIdx+len(startSig):]
	endRel := strings.Index(rest, "\nfunc ")
	if endRel < 0 {
		t.Fatalf("createHost 函数体结束未找到")
	}
	body := rest[:endRel]

	markers := []struct {
		name   string
		needle string
	}{
		{"PrepareGateway", "w.provider.PrepareGateway(ctx, spec)"},
		{"buildCreateArgs", "w.buildCreateArgs(request, containerName, hostname, egressCfg)"},
		{"docker_create", "w.runDocker(ctx, args...)"},
		{"docker_start", `w.runDocker(ctx, "start", containerName)`},
		{"PrepareHost", "w.provider.PrepareHost(ctx, spec)"},
	}

	positions := make([]int, len(markers))
	for i, m := range markers {
		idx := strings.Index(body, m.needle)
		if idx < 0 {
			t.Fatalf("createHost 函数体内未找到 %s 调用，期望 needle=%q", m.name, m.needle)
		}
		positions[i] = idx
	}

	for i := 1; i < len(positions); i++ {
		if !(positions[i-1] < positions[i]) {
			t.Errorf("调用顺序违反：%s (pos=%d) 必须先于 %s (pos=%d)\nworker.go::createHost 函数体内的标识符顺序应为 %s",
				markers[i-1].name, positions[i-1], markers[i].name, positions[i],
				"PrepareGateway < buildCreateArgs < docker_create < docker_start < PrepareHost")
		}
	}
	t.Logf("OK call order positions (createHost): %v", positions)
}
