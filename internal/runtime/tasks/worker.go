package tasks

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/zanel1u/cloud-cli-proxy/internal/agentapi"
	"github.com/zanel1u/cloud-cli-proxy/internal/network"
	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

const (
	defaultHostRoot       = "/var/lib/cloud-cli-proxy/hosts/"
	defaultWorkspaceMount = "/workspace"
	taskStatePending      = "pending"
	taskStateRunning      = "running"
	taskStateSucceeded    = "succeeded"
	taskStateFailed       = "failed"
	taskStateCanceled     = "canceled"
)

type WorkerRepo interface {
	UpdateTaskStatus(context.Context, string, string, string, string, string) (repository.Task, error)
	UpdateHostStatus(ctx context.Context, hostID string, status string) error
	GetEgressIPByHost(ctx context.Context, hostID string) (repository.EgressIP, error)
	GetHostWgKeys(ctx context.Context, hostID string) (string, string, error)
	RecordEvent(ctx context.Context, params repository.RecordEventParams) (repository.Event, error)
}

type Worker struct {
	repo     WorkerRepo
	provider network.Provider
}

func NewWorker(repo WorkerRepo, provider network.Provider) *Worker {
	return &Worker{repo: repo, provider: provider}
}

func (w *Worker) Execute(ctx context.Context, request agentapi.HostActionRequest) agentapi.TaskStatusUpdate {
	var err error
	switch request.Action {
	case agentapi.ActionCreateHost:
		err = w.createHost(ctx, request)
	case agentapi.ActionStartHost:
		err = w.startHost(ctx, request)
	case agentapi.ActionStopHost:
		err = w.stopHost(ctx, request)
	case agentapi.ActionRebuildHost:
		err = w.rebuildHost(ctx, request)
	case agentapi.ActionPrepareHost:
		err = w.validateAndPrepare(ctx, request.HostID)
	default:
		err = fmt.Errorf("unsupported host action: %s", request.Action)
	}

	if err != nil {
		errorCode := "host_action_failed"
		var sshErr *SSHNotReadyError
		var netErr *network.NetworkError
		if errors.As(err, &sshErr) {
			errorCode = "ssh_not_ready"
		} else if errors.As(err, &netErr) {
			switch netErr.Type {
			case network.ErrBindingMissing:
				errorCode = "egress_binding_missing"
			case network.ErrEgressIPMismatch:
				errorCode = "egress_ip_mismatch"
			case network.ErrDNSLeak:
				errorCode = "dns_leak"
			case network.ErrLeakNotBlocked:
				errorCode = "leak_not_blocked"
			case network.ErrEgressUnreachable:
				errorCode = "egress_unreachable"
			case network.ErrTunnelSetupFailed:
				errorCode = "tunnel_setup_failed"
			default:
				errorCode = "network_error"
			}
		}
		_ = w.repo.UpdateHostStatus(ctx, request.HostID, "failed")
		return agentapi.TaskStatusUpdate{
			TaskID:           request.TaskID,
			Status:           taskStateFailed,
			ErrorCode:        errorCode,
			ErrorMessage:     err.Error(),
			LastErrorSummary: summarizeError(err),
		}
	}

	hostStatus := actionToHostStatus(request.Action)
	_ = w.repo.UpdateHostStatus(ctx, request.HostID, hostStatus)

	return agentapi.TaskStatusUpdate{
		TaskID: request.TaskID,
		Status: taskStateSucceeded,
	}
}

func (w *Worker) UpdateTaskStatus(ctx context.Context, update agentapi.TaskStatusUpdate) error {
	_, err := w.repo.UpdateTaskStatus(
		ctx,
		update.TaskID,
		update.Status,
		update.ErrorCode,
		update.ErrorMessage,
		update.LastErrorSummary,
	)
	return err
}

func actionToHostStatus(action agentapi.HostAction) string {
	switch action {
	case agentapi.ActionCreateHost:
		return "running"
	case agentapi.ActionStartHost:
		return "running"
	case agentapi.ActionStopHost:
		return "stopped"
	case agentapi.ActionRebuildHost:
		return "running"
	default:
		return "stopped"
	}
}

func (w *Worker) createHost(ctx context.Context, request agentapi.HostActionRequest) error {
	homeDir := firstNonEmpty(request.HomeDir, hostHomeDir(request.HostID))
	containerName := firstNonEmpty(request.ContainerName, containerNameForHost(request.HostID))
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		return fmt.Errorf("prepare host home dir %s: %w", homeDir, err)
	}

	if exists, err := w.containerExists(ctx, containerName); err != nil {
		return err
	} else if exists {
		if err := w.runDocker(ctx, "rm", "-f", containerName); err != nil {
			return err
		}
	}

	hostname := request.Hostname
	if hostname == "" {
		hostname = containerName
	}

	args := []string{
		"create",
		"--name", containerName,
		"--network", "bridge",
		"--cap-add", "NET_ADMIN",
		"--label", "cloud-cli-proxy.managed=true",
		"--label", fmt.Sprintf("cloud-cli-proxy.host_id=%s", request.HostID),
		"--hostname", hostname,
		"--shm-size", "1g",
		"--sysctl", "net.ipv6.conf.all.disable_ipv6=1",
	}

	if request.MemoryLimitMB > 0 {
		args = append(args, "--memory", fmt.Sprintf("%dm", request.MemoryLimitMB))
	}
	if request.CPULimit > 0 {
		args = append(args, "--cpus", fmt.Sprintf("%.1f", request.CPULimit))
	}

	args = append(args,
		"-e", "TZ="+firstNonEmpty(request.Timezone, "America/Los_Angeles"),
		"-e", "LANG=en_US.UTF-8",
		"-e", "LANGUAGE=en_US:en",
		"-e", "LC_ALL=en_US.UTF-8",
		"-e", "CONTAINER_USER="+firstNonEmpty(request.Username, "workspace"),
		"-e", "CONTAINER_SSH_PASSWORD="+firstNonEmpty(request.EntryPassword, "workspace"),
		"-v", fmt.Sprintf("%s:%s", homeDir, firstNonEmpty(request.HomeMount, defaultWorkspaceMount)),
	)

	for key, value := range request.Labels {
		args = append(args, "--label", fmt.Sprintf("%s=%s", key, value))
	}

	args = append(args, request.ImageName)

	if err := w.runDocker(ctx, args...); err != nil {
		return err
	}

	if err := w.runDocker(ctx, "start", containerName); err != nil {
		return fmt.Errorf("start container after create: %w", err)
	}

	egressCfg, err := w.buildEgressConfig(ctx, request.HostID)
	if err != nil {
		return err
	}
	if egressCfg != nil {
		spec := network.HostNetworkSpec{
			HostID: request.HostID,
			Egress: egressCfg,
		}
		if err := w.provider.PrepareHost(ctx, spec); err != nil {
			w.recordNetworkError(ctx, request.HostID, err)
			return fmt.Errorf("prepare host network after create: %w", err)
		}
	}

	return nil
}

func (w *Worker) startHost(ctx context.Context, request agentapi.HostActionRequest) error {
	if err := w.validateAndPrepare(ctx, request.HostID); err != nil {
		return err
	}

	containerName := firstNonEmpty(request.ContainerName, containerNameForHost(request.HostID))
	if exists, err := w.containerExists(ctx, containerName); err != nil {
		return err
	} else if !exists {
		if err := w.createHost(ctx, request); err != nil {
			return err
		}
	}

	if err := w.runDocker(ctx, "start", containerName); err != nil {
		return err
	}

	egressCfg, err := w.buildEgressConfig(ctx, request.HostID)
	if err != nil {
		return err
	}

	if egressCfg != nil {
		spec := network.HostNetworkSpec{
			HostID: request.HostID,
			Egress: egressCfg,
		}
		if err := w.provider.PrepareHost(ctx, spec); err != nil {
			w.recordNetworkError(ctx, request.HostID, err)
			return fmt.Errorf("prepare host network: %w", err)
		}
	}

	w.repo.RecordEvent(ctx, repository.RecordEventParams{
		HostID:   &request.HostID,
		Level:    "info",
		Type:     "net.ready",
		Message:  "host started",
		Metadata: map[string]any{"host_id": request.HostID},
	})

	if err := w.waitForSSH(ctx, request, containerName); err != nil {
		return err
	}

	return nil
}

func (w *Worker) stopHost(ctx context.Context, request agentapi.HostActionRequest) error {
	containerName := firstNonEmpty(request.ContainerName, containerNameForHost(request.HostID))
	if exists, err := w.containerExists(ctx, containerName); err != nil {
		return err
	} else if !exists {
		return nil
	}

	if err := w.runDocker(ctx, "stop", containerName); err != nil {
		return err
	}

	if err := w.provider.CleanupHost(ctx, network.HostNetworkSpec{HostID: request.HostID}); err != nil {
		return fmt.Errorf("cleanup host network after stop: %w", err)
	}

	return nil
}

func (w *Worker) rebuildHost(ctx context.Context, request agentapi.HostActionRequest) error {
	if err := w.validateAndPrepare(ctx, request.HostID); err != nil {
		return err
	}

	containerName := firstNonEmpty(request.ContainerName, containerNameForHost(request.HostID))
	if err := w.stopHost(ctx, request); err != nil {
		return err
	}
	if exists, err := w.containerExists(ctx, containerName); err != nil {
		return err
	} else if exists {
		if err := w.runDocker(ctx, "rm", "-f", containerName); err != nil {
			return err
		}
	}

	if request.RebuildMode == "wipe-/workspace" {
		if err := os.RemoveAll(firstNonEmpty(request.HomeDir, hostHomeDir(request.HostID))); err != nil {
			return fmt.Errorf("clear all data and rebuild host home: %w", err)
		}
	}

	if err := w.provider.CleanupHost(ctx, network.HostNetworkSpec{HostID: request.HostID}); err != nil {
		return fmt.Errorf("cleanup host network: %w", err)
	}

	if err := w.createHost(ctx, request); err != nil {
		return err
	}

	w.repo.RecordEvent(ctx, repository.RecordEventParams{
		HostID:   &request.HostID,
		Level:    "info",
		Type:     "net.ready",
		Message:  "host rebuilt",
		Metadata: map[string]any{"host_id": request.HostID},
	})

	if err := w.waitForSSH(ctx, request, containerName); err != nil {
		return err
	}

	return nil
}

func (w *Worker) validateAndPrepare(ctx context.Context, hostID string) error {
	validator := &repoValidator{repo: w.repo}
	_, err := network.ValidateEgressBinding(ctx, validator, hostID)
	if err != nil {
		w.recordNetworkError(ctx, hostID, err)
		return err
	}
	return nil
}

func (w *Worker) buildEgressConfig(ctx context.Context, hostID string) (*network.EgressConfig, error) {
	validator := &repoValidator{repo: w.repo}
	cfg, err := network.ValidateEgressBinding(ctx, validator, hostID)
	if err != nil {
		w.recordNetworkError(ctx, hostID, err)
		return nil, err
	}
	return &cfg, nil
}

func (w *Worker) waitForSSH(ctx context.Context, request agentapi.HostActionRequest, containerName string) error {
	w.repo.RecordEvent(ctx, repository.RecordEventParams{
		TaskID:   &request.TaskID,
		HostID:   &request.HostID,
		Level:    "info",
		Type:     "runtime.validating",
		Message:  "validating SSH readiness",
		Metadata: map[string]any{"host_id": request.HostID, "container": containerName},
	})

	if err := WaitForSSHReady(ctx, containerName, DefaultSSHReadyConfig); err != nil {
		w.repo.RecordEvent(ctx, repository.RecordEventParams{
			TaskID:   &request.TaskID,
			HostID:   &request.HostID,
			Level:    "error",
			Type:     "ssh.failed",
			Message:  err.Error(),
			Metadata: map[string]any{"host_id": request.HostID, "container": containerName},
		})
		return err
	}

	w.syncContainerCredentials(ctx, request, containerName)
	w.injectSSHKeys(ctx, request, containerName)

	w.repo.RecordEvent(ctx, repository.RecordEventParams{
		TaskID:   &request.TaskID,
		HostID:   &request.HostID,
		Level:    "info",
		Type:     "ssh.ready",
		Message:  "SSH port is ready",
		Metadata: map[string]any{"host_id": request.HostID, "container": containerName},
	})

	handoffMeta := BuildSSHHandoffMetadata(request.HostID, "root")
	w.repo.RecordEvent(ctx, repository.RecordEventParams{
		TaskID:   &request.TaskID,
		HostID:   &request.HostID,
		Level:    "info",
		Type:     "ssh.handoff.ready",
		Message:  "SSH handoff metadata ready",
		Metadata: handoffMeta,
	})

	return nil
}

// syncContainerCredentials forces the container's Linux password to match
// the request, regardless of what the entrypoint set via environment variables.
func (w *Worker) syncContainerCredentials(ctx context.Context, request agentapi.HostActionRequest, containerName string) {
	user := firstNonEmpty(request.Username, "workspace")
	pass := firstNonEmpty(request.EntryPassword, "workspace")

	cmd := exec.CommandContext(ctx, "docker", "exec", "-i", containerName, "chpasswd")
	cmd.Stdin = strings.NewReader(fmt.Sprintf("%s:%s\n", user, pass))
	if out, err := cmd.CombinedOutput(); err != nil {
		w.repo.RecordEvent(ctx, repository.RecordEventParams{
			HostID:  &request.HostID,
			Level:   "warn",
			Type:    "runtime.password_sync_failed",
			Message: fmt.Sprintf("docker exec chpasswd failed: %s", strings.TrimSpace(string(out))),
		})
	}
}

func (w *Worker) injectSSHKeys(ctx context.Context, request agentapi.HostActionRequest, containerName string) {
	if request.SSHPublicKey == "" && request.SSHPrivateKey == "" {
		return
	}

	user := firstNonEmpty(request.Username, "workspace")
	sshDir := "/workspace/.ssh"

	if request.SSHPrivateKey != "" {
		keyFile := sshDir + "/id_ed25519"
		if strings.Contains(request.SSHPublicKey, "ssh-rsa") {
			keyFile = sshDir + "/id_rsa"
		}

		script := fmt.Sprintf(
			"mkdir -p %s && cat > %s && chmod 600 %s && chown %s:%s %s",
			sshDir, keyFile, keyFile, user, user, keyFile,
		)
		cmd := exec.CommandContext(ctx, "docker", "exec", "-i", containerName, "bash", "-c", script)
		cmd.Stdin = strings.NewReader(request.SSHPrivateKey)
		if out, err := cmd.CombinedOutput(); err != nil {
			w.repo.RecordEvent(ctx, repository.RecordEventParams{
				HostID:  &request.HostID,
				Level:   "warn",
				Type:    "runtime.ssh_key_inject_failed",
				Message: fmt.Sprintf("inject private key failed: %s", strings.TrimSpace(string(out))),
			})
		}
	}

	if request.SSHPublicKey != "" {
		pubKeyFile := sshDir + "/id_ed25519.pub"
		if strings.Contains(request.SSHPublicKey, "ssh-rsa") {
			pubKeyFile = sshDir + "/id_rsa.pub"
		}

		script := fmt.Sprintf(
			"mkdir -p %s && cat > %s && chmod 644 %s && chown %s:%s %s",
			sshDir, pubKeyFile, pubKeyFile, user, user, pubKeyFile,
		)
		cmd := exec.CommandContext(ctx, "docker", "exec", "-i", containerName, "bash", "-c", script)
		cmd.Stdin = strings.NewReader(request.SSHPublicKey)
		if out, err := cmd.CombinedOutput(); err != nil {
			w.repo.RecordEvent(ctx, repository.RecordEventParams{
				HostID:  &request.HostID,
				Level:   "warn",
				Type:    "runtime.ssh_key_inject_failed",
				Message: fmt.Sprintf("inject public key failed: %s", strings.TrimSpace(string(out))),
			})
		}
	}
}

func (w *Worker) recordNetworkError(ctx context.Context, hostID string, err error) {
	var netErr *network.NetworkError
	if errors.As(err, &netErr) {
		_, _ = w.repo.RecordEvent(ctx, repository.RecordEventParams{
			HostID:   &hostID,
			Level:    "error",
			Type:     netErr.EventType(),
			Message:  netErr.Error(),
			Metadata: netErr.EventMetadata(),
		})
	}
}

type repoValidator struct {
	repo WorkerRepo
}

func (rv *repoValidator) GetEgressIPByHost(ctx context.Context, hostID string) (network.EgressIPRecord, error) {
	eip, err := rv.repo.GetEgressIPByHost(ctx, hostID)
	if err != nil {
		return network.EgressIPRecord{}, err
	}
	return network.EgressIPRecord{
		ID:             eip.ID,
		IPAddress:      eip.IPAddress,
		TunnelType:     eip.TunnelType,
		ProxyConfig:    eip.ProxyConfig,
		WgEndpoint:     eip.WgEndpoint,
		WgPublicKey:    eip.WgPublicKey,
		WgPresharedKey: eip.WgPresharedKey,
		WgAllowedIPs:   eip.WgAllowedIPs,
		WgDNSServer:    eip.WgDNSServer,
		WgPeerAddress:  eip.WgPeerAddress,
	}, nil
}

func (rv *repoValidator) GetHostWgKeys(ctx context.Context, hostID string) (string, string, error) {
	return rv.repo.GetHostWgKeys(ctx, hostID)
}

func (w *Worker) containerExists(ctx context.Context, name string) (bool, error) {
	cmd := exec.CommandContext(ctx, "docker", "container", "inspect", name)
	if err := cmd.Run(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok && exitError.ExitCode() != 0 {
			return false, nil
		}
		return false, fmt.Errorf("inspect docker container %s: %w", name, err)
	}

	return true, nil
}

func (w *Worker) runDocker(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "docker", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker %s: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}

	return nil
}

func hostHomeDir(hostID string) string {
	return fmt.Sprintf("%s%s/home", defaultHostRoot, hostID)
}

func containerNameForHost(hostID string) string {
	return fmt.Sprintf("cloudproxy-%s", hostID)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}

	return ""
}

func summarizeError(err error) string {
	message := err.Error()
	if len(message) > 160 {
		return message[:160]
	}

	return message
}
