package http

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type VNCServiceStatus struct {
	Status             string    `json:"status"`
	Running            bool      `json:"running"`
	CanStart           bool      `json:"can_start"`
	CanRestart         bool      `json:"can_restart"`
	AutoRestartLimited bool      `json:"auto_restart_limited"`
	Display            string    `json:"display,omitempty"`
	WebsocketPort      int       `json:"websocket_port,omitempty"`
	LastError          string    `json:"last_error,omitempty"`
	CheckedAt          time.Time `json:"checked_at"`
}

var runContainerVNCCommand = func(ctx context.Context, containerName string, args ...string) ([]byte, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("missing vnc command")
	}
	command := args[0]
	if strings.Contains(command, "/") {
		return nil, fmt.Errorf("invalid vnc command %q", command)
	}
	dockerArgs := []string{"exec", "-i", containerName, "/usr/local/bin/" + command}
	dockerArgs = append(dockerArgs, args[1:]...)
	cmd := exec.CommandContext(ctx, "docker", dockerArgs...)
	return cmd.CombinedOutput()
}

func inspectContainerVNC(ctx context.Context, containerName string, hostRunning bool) (VNCServiceStatus, error) {
	checkedAt := time.Now().UTC()
	if !hostRunning {
		return VNCServiceStatus{
			Status:    "host_stopped",
			CheckedAt: checkedAt,
		}, nil
	}

	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	output, err := runContainerVNCCommand(ctx, containerName, "vnc-status", "--json")
	if err != nil {
		return VNCServiceStatus{
			Status:    "unavailable",
			CanStart:  true,
			LastError: strings.TrimSpace(string(output)),
			CheckedAt: checkedAt,
		}, nil
	}

	var status VNCServiceStatus
	if err := json.Unmarshal(output, &status); err != nil {
		return VNCServiceStatus{}, fmt.Errorf("parse vnc status: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	status.CheckedAt = checkedAt
	if status.Display == "" {
		status.Display = ":99"
	}
	if status.WebsocketPort == 0 {
		status.WebsocketPort = 6080
	}
	status.CanStart = !status.Running
	status.CanRestart = status.Running
	return status, nil
}

func controlContainerVNC(ctx context.Context, containerName, action string) error {
	switch action {
	case "start", "restart":
	default:
		return fmt.Errorf("unsupported vnc action %q", action)
	}

	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	output, err := runContainerVNCCommand(ctx, containerName, "restart-vnc", action)
	if err != nil {
		return fmt.Errorf("docker exec restart-vnc %s: %w (%s)", action, err, strings.TrimSpace(string(output)))
	}
	return nil
}
