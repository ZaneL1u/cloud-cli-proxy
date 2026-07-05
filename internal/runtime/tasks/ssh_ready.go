package tasks

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

type SSHNotReadyError struct {
	Container string
	Timeout   time.Duration
	LastErr   error
}

func (e *SSHNotReadyError) Error() string {
	return fmt.Sprintf("ssh not ready on container %s after %s: %v", e.Container, e.Timeout, e.LastErr)
}

func (e *SSHNotReadyError) Unwrap() error {
	return e.LastErr
}

type SSHReadyConfig struct {
	PollInterval time.Duration
	Timeout      time.Duration
	Check        func(ctx context.Context, containerName string) error
}

var DefaultSSHReadyConfig = SSHReadyConfig{
	PollInterval: 1 * time.Second,
	Timeout:      120 * time.Second,
	Check:        DockerExecSSHCheck,
}

func DockerExecSSHCheck(ctx context.Context, containerName string) error {
	cmd := exec.CommandContext(ctx, "docker", "exec", containerName, "bash", "-c", "</dev/tcp/127.0.0.1/22")
	return cmd.Run()
}

// WaitForSSHReady polls the SSH port inside a container until it becomes
// reachable or the timeout elapses. Returns SSHNotReadyError on timeout.
func WaitForSSHReady(ctx context.Context, containerName string, cfg SSHReadyConfig) error {
	if cfg.Check == nil {
		cfg.Check = DockerExecSSHCheck
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 1 * time.Second
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 120 * time.Second
	}

	var lastErr error
	if err := cfg.Check(ctx, containerName); err == nil {
		return nil
	} else {
		lastErr = err
	}

	deadline := time.NewTimer(cfg.Timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return &SSHNotReadyError{
				Container: containerName,
				Timeout:   cfg.Timeout,
				LastErr:   lastErr,
			}
		case <-ticker.C:
			if err := cfg.Check(ctx, containerName); err != nil {
				lastErr = err
				continue
			}
			return nil
		}
	}
}
