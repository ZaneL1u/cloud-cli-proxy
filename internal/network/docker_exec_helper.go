package network

import (
	"context"
	"os/exec"
)

// 测试注入点：替换后可在单测中模拟 docker exec 行为。
var dockerExecContext = defaultDockerExecContext

// defaultDockerExecContext 构造 docker exec -i 命令（-i 使容器接受 stdin）。
// 可被测试替换为返回预设输出的 fake。
func defaultDockerExecContext(ctx context.Context, container string, args ...string) *exec.Cmd {
	fullArgs := append([]string{"exec", "-i", container}, args...)
	return exec.CommandContext(ctx, "docker", fullArgs...)
}
