package local

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// DockerRunner abstracts docker CLI execution for testability.
type DockerRunner func(ctx context.Context, args ...string) ([]byte, error)

// DefaultDockerRunner executes the real docker CLI.
func DefaultDockerRunner(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("docker %s: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

// containerExists checks if a Docker container with the given name exists.
func containerExists(ctx context.Context, runner DockerRunner, name string) (bool, error) {
	_, err := runner(ctx, "inspect", "--format={{.Id}}", name)
	if err != nil {
		if strings.Contains(err.Error(), "No such container") || strings.Contains(err.Error(), "no such object") {
			return false, nil
		}
		return false, fmt.Errorf("inspect container %s: %w", name, err)
	}
	return true, nil
}

// inspectSSHPort extracts the host port mapped to container port 22.
func inspectSSHPort(ctx context.Context, runner DockerRunner, containerName string) (string, error) {
	out, err := runner(ctx, "inspect",
		"--format={{(index (index .NetworkSettings.Ports \"22/tcp\") 0).HostPort}}",
		containerName)
	if err != nil {
		return "", fmt.Errorf("inspect SSH port for %s: %w", containerName, err)
	}
	port := strings.TrimSpace(string(out))
	if port == "" || port == "<no value>" {
		return "", fmt.Errorf("no SSH port mapping found for container %s", containerName)
	}
	return port, nil
}

// inspectContainerStatus returns the Docker status string (e.g., "running", "exited").
func inspectContainerStatus(ctx context.Context, runner DockerRunner, name string) (string, error) {
	out, err := runner(ctx, "inspect", "--format={{.State.Status}}", name)
	if err != nil {
		if strings.Contains(err.Error(), "No such container") || strings.Contains(err.Error(), "no such object") {
			return "not_found", nil
		}
		return "", fmt.Errorf("inspect status for %s: %w", name, err)
	}
	return strings.TrimSpace(string(out)), nil
}
