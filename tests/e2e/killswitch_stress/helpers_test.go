//go:build e2e && linux

// helpers_test.go 在 killswitch_stress 包内复用 e2e 主包公开字段反推容器名
// 的兼容入口。
//
// e2e.GoldenPath.workerDockerName / gatewayDockerName 是包私有方法；
// killswitch_stress 子包通过 g.Host.ContainerName / Host.ID 与
// g.Gateway.ContainerID / Gateway.HostID 公开字段反推容器名（与
// helpers_linux.go::{workerDockerName,gatewayDockerName} 同语义）。

package killswitch_stress

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"

	e2e "github.com/zanel1u/cloud-cli-proxy/tests/e2e"
	"github.com/zanel1u/cloud-cli-proxy/tests/e2e/harness"
)

// workerInspectName 反推 worker 容器名。
//
// 优先级：
//   - g.Host.ContainerName 非空 → 直接用。
//   - g.Host.ID 非空 → "cloudproxy-" + ID（与
//     internal/network/container_proxy_provider.go workerContainerName 一致）。
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

// gatewayInspectName 反推 gateway 容器名。
//
// 优先级：
//   - g.Gateway.ContainerID 非空 → 直接用。
//   - g.Gateway.HostID 非空 → "cloudproxy-gw-" + HostID（与
//     internal/network/container_proxy_provider.go gatewayContainerName 一致）。
//   - g.Host.ID 兜底 → "cloudproxy-gw-" + Host.ID。
//   - 都为空 → 返回 err，调用方 t.Skip。
func gatewayInspectName(_ context.Context, g *e2e.GoldenPath) (string, error) {
	if g == nil || g.Gateway == nil {
		return "", errors.New("gateway container name: gateway handle nil")
	}
	if id := strings.TrimSpace(g.Gateway.ContainerID); id != "" {
		return id, nil
	}
	if g.Gateway.HostID != "" {
		return "cloudproxy-gw-" + g.Gateway.HostID, nil
	}
	if g.Host != nil && g.Host.ID != "" {
		return "cloudproxy-gw-" + g.Host.ID, nil
	}
	return "", errors.New("gateway container name: ContainerID + HostID 均空 (scenario step 4..6 未实现)")
}

// dockerExecHandle 是 ContainerHandle 接口的最小实现，绕过 testcontainers，
// 直接通过 `docker exec <name>` 跑命令。KILL-03 用例需要 FetchEgressIPInContainer
// 拿三源回显，主包 workerContainerHandle 在 Scenario Step 7 sentinel 期间一律
// 返回 nil；本 wrapper 让 Pumba 注入后仍能消费 docker exec 路径（容器名已通过
// gatewayInspectName / workerInspectName 反推）。
type dockerExecHandle struct {
	name string
}

// Logs 仅为接口完整性提供占位实现；本用例不消费日志。
func (h *dockerExecHandle) Logs(ctx context.Context) (io.ReadCloser, error) {
	return nil, errors.New("dockerExecHandle: Logs not implemented")
}

// Exec 用 `docker exec <name> <argv...>` 跑命令，返回 (exitCode, stdoutReader, err)。
// docker exec 失败（容器不在 / daemon 不通）→ exitCode=-1, err 非 nil。
// 子进程退出码非 0 → exitCode + nil（与 testcontainers Container.Exec 同语义）。
func (h *dockerExecHandle) Exec(ctx context.Context, argv []string) (int, io.Reader, error) {
	full := append([]string{"exec", h.name}, argv...)
	cmd := exec.CommandContext(ctx, "docker", full...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	runErr := cmd.Run()
	if runErr == nil {
		return 0, &stdout, nil
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return exitErr.ExitCode(), &stdout, nil
	}
	return -1, &stdout, fmt.Errorf("docker exec %s: %w", h.name, runErr)
}

// newWorkerExecHandle 构造一个 ContainerHandle，用于 KILL-03 在 sing-box
// 劣化场景下消费 e2e.FetchEgressIPInContainer。containerName 通常由
// workerInspectName 反推得到。
func newWorkerExecHandle(containerName string) harness.ContainerHandle {
	return &dockerExecHandle{name: containerName}
}

// 防御性引用避免 unused：用例文件 build tag 与本文件一致，但万一未来
// 用例文件被 mv 出 killswitch_stress 包，dockerExecHandle 仍可被探测包级
// 单测引用；保留构造函数即可。
var _ = newWorkerExecHandle
