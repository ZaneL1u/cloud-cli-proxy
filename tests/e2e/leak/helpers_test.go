//go:build e2e && linux

// helpers_test.go 在 LEAK 包内复用 e2e 主包私有 helper 的兼容入口。
//
// e2e.GoldenPath.workerDockerName / gatewayDockerName 是包私有方法，LEAK
// 子包不能直接调；workerInspectName 通过 g.Host.ContainerName / Host.ID 公开
// 字段反推容器名（与 helpers_linux.go::workerDockerName 同语义）。

package leak

import (
	"context"
	"errors"
	"strings"

	e2e "github.com/zanel1u/cloud-cli-proxy/tests/e2e"
)

// workerInspectName 反推 worker 容器名。
//
// 优先级：
//   - g.Host.ContainerName 非空 → 直接用。
//   - g.Host.ID 非空 → "cloudproxy-" + ID（与 internal/network/container_proxy_provider.go
//     workerContainerName 命名约定一致）。
//   - 都为空 → 返回 err，调用方 t.Skip。
func workerInspectName(_ context.Context, g *e2e.GoldenPath) (string, error) {
	if g == nil || g.Host == nil {
		return "", errors.New("worker container name: host handle nil")
	}
	if name := strings.TrimSpace(g.Host.ContainerName); name != "" {
		return name, nil
	}
	if g.Host.ID != "" {
		return "cloudproxy-" + g.Host.ID, nil
	}
	return "", errors.New("worker container name: host.ID empty (scenario step 7 未实现)")
}
