package network

import (
	"os"
	"strings"
	"testing"
)

// extractFuncBody 找到 worker.go 内顶层 `func <signature>` 的函数体片段。
// 函数体结束 = 紧接出现的下一个**顶层** `func ` 行（第 0 列起始）。
// Phase 45 WR-02：旧实现用 `\nfunc ` 简单子串匹配，遇到 long-signature 闭包
// 换行格式时会被误截；这里改用「行首位置 + 长度累加」精确定位顶层函数边界。
func extractFuncBody(t *testing.T, content, startSig string) string {
	t.Helper()
	startIdx := strings.Index(content, startSig)
	if startIdx < 0 {
		t.Fatalf("函数签名未找到: %q", startSig)
	}
	rest := content[startIdx+len(startSig):]
	lines := strings.Split(rest, "\n")
	acc := 0
	endRel := -1
	for i, ln := range lines {
		// 跳过当前函数自身首行；只关心后续顶层 func 起点
		if i > 0 && strings.HasPrefix(ln, "func ") {
			endRel = acc
			break
		}
		acc += len(ln) + 1 // +1 是换行符
	}
	if endRel < 0 {
		t.Fatalf("函数体结束未找到（签名 %q 之后未匹配到下一个顶层 func）", startSig)
	}
	return rest[:endRel]
}

// assertCallOrder 在函数体内按 markers 顺序断言每个 needle 的首次出现位置严格升序。
func assertCallOrder(t *testing.T, body, funcName string, markers []struct {
	name   string
	needle string
}) {
	t.Helper()
	positions := make([]int, len(markers))
	for i, m := range markers {
		idx := strings.Index(body, m.needle)
		if idx < 0 {
			t.Fatalf("%s 函数体内未找到 %s 调用，期望 needle=%q", funcName, m.name, m.needle)
		}
		positions[i] = idx
	}
	for i := 1; i < len(positions); i++ {
		if !(positions[i-1] < positions[i]) {
			t.Errorf("%s 调用顺序违反：%s (pos=%d) 必须先于 %s (pos=%d)",
				funcName, markers[i-1].name, positions[i-1], markers[i].name, positions[i])
		}
	}
	t.Logf("OK call order positions (%s): %v", funcName, positions)
}

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
	src, err := os.ReadFile("../runtime/tasks/worker.go")
	if err != nil {
		t.Fatalf("read worker.go: %v", err)
	}
	body := extractFuncBody(t, string(src), "func (w *Worker) createHost(")

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
	assertCallOrder(t, body, "createHost", markers)
}

// TestWorker_StartHost_CallOrder 守护 startHost 内的相同硬约束：
//
//	PrepareGateway → docker start → PrepareHost
//
// startHost 与 createHost 走两条路径但都依赖 sing-box gateway 在容器 start
// 之前完成 tun0 监听，否则 worker 容器内 ro-bind 的 /etc/resolv.conf 指向
// 一个还没监听的 172.19.0.1，触发 BYPASS-DNS-03。
func TestWorker_StartHost_CallOrder(t *testing.T) {
	src, err := os.ReadFile("../runtime/tasks/worker.go")
	if err != nil {
		t.Fatalf("read worker.go: %v", err)
	}
	body := extractFuncBody(t, string(src), "func (w *Worker) startHost(")

	markers := []struct {
		name   string
		needle string
	}{
		{"PrepareGateway", "w.provider.PrepareGateway(ctx, spec)"},
		{"docker_start", `w.runDocker(ctx, "start", containerName)`},
		{"PrepareHost", "w.provider.PrepareHost(ctx, spec)"},
	}
	assertCallOrder(t, body, "startHost", markers)
}

// TestWorker_RebuildHost_CallOrder 守护 rebuildHost 内的硬约束：
//
//	stopHost → CleanupHost → createHost
//
// rebuildHost 走「先 stop 后 create」流程；任何把 createHost 移到 CleanupHost
// 之前的回归都会让旧 gateway 与新 worker 同时存在，造成 IP 冲突 + 容器名冲突。
func TestWorker_RebuildHost_CallOrder(t *testing.T) {
	src, err := os.ReadFile("../runtime/tasks/worker.go")
	if err != nil {
		t.Fatalf("read worker.go: %v", err)
	}
	body := extractFuncBody(t, string(src), "func (w *Worker) rebuildHost(")

	markers := []struct {
		name   string
		needle string
	}{
		{"stopHost", "w.stopHost(ctx, request)"},
		{"CleanupHost", "w.provider.CleanupHost(ctx, network.HostNetworkSpec{HostID: request.HostID})"},
		{"createHost", "w.createHost(ctx, request)"},
	}
	assertCallOrder(t, body, "rebuildHost", markers)
}
