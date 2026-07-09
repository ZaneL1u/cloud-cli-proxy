package tasks

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/zanel1u/cloud-cli-proxy/internal/agentapi"
	"github.com/zanel1u/cloud-cli-proxy/internal/broadcast"
	"github.com/zanel1u/cloud-cli-proxy/internal/network"
	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

// pullImageTimeout 限制 docker pull 单次最长执行时间，防止 registry 卡死无限期 hold task
// （Phase 33 后置修复：原实现复用 outer task ctx，registry hang 会让 host 操作永远 pending）。
const pullImageTimeout = 5 * time.Minute

const (
	defaultHostRoot       = "/var/lib/cloud-cli-proxy/hosts/"
	defaultWorkspaceMount = "/workspace"
	defaultMemoryLimitMB  = 4096
	defaultCPULimit       = 2.0
	defaultPidsLimit      = 1024
	taskStatePending      = "pending"
	taskStateRunning      = "running"
	taskStateSucceeded    = "succeeded"
	taskStateFailed       = "failed"
	taskStateCanceled     = "canceled"
)

const containerDNSLockContent = `# v4.0: DNS forced through in-container sing-box DNS inbound
nameserver 127.0.0.1
options edns0 trust-ad
`

const (
	sshManagedBeginMarker = "# >>> cloud-cli-proxy managed keys (do not edit) >>>"
	sshManagedEndMarker   = "# <<< cloud-cli-proxy managed keys <<<"
)

type WorkerRepo interface {
	UpdateTaskStatus(context.Context, string, string, string, string, string) (repository.Task, error)
	UpdateHostStatus(ctx context.Context, hostID string, status string) error
	GetEgressIPByHost(ctx context.Context, hostID string) (repository.EgressIP, error)
	RecordEvent(ctx context.Context, params repository.RecordEventParams) (repository.Event, error)
	UpsertClaudeAccountPersistentVolumeName(ctx context.Context, accountID, volumeName string) error // Phase 33 D-06
	ReportTaskProgress(ctx context.Context, taskID string, percent int, message string) error
	// Phase 47 Plan 01：worker.handleReloadHostBypass 依赖以下三个 Bypass 方法。
	// Repository 已在 Phase 45-03 落地三套查询（见 internal/store/repository/queries_bypass.go）。
	GetBypassSnapshotByID(ctx context.Context, id string) (repository.BypassSnapshot, error)
	UpdateBypassSnapshotStatus(ctx context.Context, id string, status string) (repository.BypassSnapshot, error)
	GetLatestAppliedBypassSnapshot(ctx context.Context, hostID string) (repository.BypassSnapshot, error)
	UpdateEgressIPAddress(ctx context.Context, egressIPID string, newIP string) error
}

type Worker struct {
	repo     WorkerRepo
	provider network.Provider
}

func NewWorker(repo WorkerRepo, provider network.Provider) *Worker {
	return &Worker{repo: repo, provider: provider}
}

// TestPanicTrigger 是包级测试钩子，供单元测试注入 panic。
// 与 net/http 的 testHook 模式一致：默认恒返回 false，test 中临时替换。
// 导出为 TestPanicTrigger 以便跨包（如 internal/agent）测试使用。
var TestPanicTrigger = func(action agentapi.HostAction) bool { return false }

func (w *Worker) Execute(ctx context.Context, request agentapi.HostActionRequest) (update agentapi.TaskStatusUpdate) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("worker panic recovered",
				"task_id", request.TaskID,
				"host_id", request.HostID,
				"action", request.Action,
				"panic", r,
			)
			_ = w.repo.UpdateHostStatus(ctx, request.HostID, "failed")
			update = agentapi.TaskStatusUpdate{
				TaskID:           request.TaskID,
				Status:           taskStateFailed,
				ErrorCode:        "panic_recovered",
				ErrorMessage:     fmt.Sprintf("panic: %v", r),
				LastErrorSummary: summarizeError(fmt.Errorf("panic: %v", r)),
			}
		}
	}()

	if TestPanicTrigger(request.Action) {
		panic("test panic")
	}

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
		err = w.prepareExistingHost(ctx, request)
	case agentapi.ActionVolumeRemove:
		err = w.removeVolumes(ctx, request)
	case agentapi.ActionReloadHostBypass:
		// Phase 47 Plan 01：真实 reload 流程在 worker_bypass_reload.go。
		// 旧 Phase 46 占位字面量已移除（Plan 47-01 Task 3 success_criteria）。
		err = w.handleReloadHostBypass(ctx, request)
	default:
		err = fmt.Errorf("unsupported host action: %s", request.Action)
	}

	if err != nil {
		// 失败路径：reload_host_bypass 失败不杀容器（bypass 失败不影响主业务）。
		if request.Action != agentapi.ActionReloadHostBypass {
			containerName := firstNonEmpty(request.ContainerName, containerNameForHost(request.HostID))
			_ = w.runDocker(ctx, "stop", containerName)
		}

		errorCode := "host_action_failed"
		if strings.HasPrefix(err.Error(), "volume_in_use:") {
			errorCode = "volume_in_use"
		}
		// Phase 47 Plan 01：reload_host_bypass 错误码映射。
		// ErrBypassReloadInvalidInput → 调用方契约违规（空 snapshot id），不可重试。
		// ErrBypassReloadFailed → 健康检查耗尽且无可回滚 snapshot，状态终态。
		if errors.Is(err, ErrBypassReloadInvalidInput) {
			errorCode = "bypass_reload_invalid_input"
		} else if errors.Is(err, ErrBypassReloadFailed) {
			errorCode = "bypass_reload_failed"
		}
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
		// bypass reload 失败不翻 host 状态（非致命，容器仍在运行）。
		if request.Action != agentapi.ActionReloadHostBypass {
			_ = w.repo.UpdateHostStatus(ctx, request.HostID, "failed")
			broadcast.Broadcast("hosts", "update", request.HostID)
		}
		broadcast.Broadcast("events", "update", "")
		return agentapi.TaskStatusUpdate{
			TaskID:           request.TaskID,
			Status:           taskStateFailed,
			ErrorCode:        errorCode,
			ErrorMessage:     err.Error(),
			LastErrorSummary: summarizeError(err),
		}
	}

	hostStatus := actionToHostStatus(request.Action)
	if hostStatus != "" {
		_ = w.repo.UpdateHostStatus(ctx, request.HostID, hostStatus)
		broadcast.Broadcast("hosts", "update", request.HostID)
	}
	broadcast.Broadcast("events", "update", "")

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

func (w *Worker) ReportTaskProgress(ctx context.Context, taskID string, percent int, message string) {
	if err := w.repo.ReportTaskProgress(ctx, taskID, percent, message); err != nil {
		slog.Warn("report task progress failed", "task_id", taskID, "error", err)
	}
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
	case agentapi.ActionPrepareHost:
		return ""
	case agentapi.ActionReloadHostBypass:
		return "" // 不改变 host 状态
	default:
		return "stopped"
	}
}

func dockerPidsLimitValue(limit *int) string {
	if limit == nil || *limit <= 0 {
		return fmt.Sprintf("%d", defaultPidsLimit)
	}
	return fmt.Sprintf("%d", *limit)
}

func dockerMemoryLimitValue(limitMB int) string {
	if limitMB <= 0 {
		limitMB = defaultMemoryLimitMB
	}
	return fmt.Sprintf("%dm", limitMB)
}

func (w *Worker) buildCreateArgs(request agentapi.HostActionRequest, containerName, hostname string, egressCfg *network.EgressConfig) ([]string, error) {
	homeDir := firstNonEmpty(request.HomeDir, hostHomeDir(request.HostID))

	args := []string{
		"create",
		"--name", containerName,
		"--network", "bridge",
		// unless-stopped：容器进程崩溃时 docker 自动重启，docker daemon 重启后也会恢复运行。
		"--restart", "unless-stopped",
		// 防止 fork 炸弹耗尽宿主机 pid；0 表示不限制。
		"--pids-limit", dockerPidsLimitValue(request.PidsLimit),
		"--log-opt", "max-size=10m",
		"--log-opt", "max-file=3",
		// Phase 51 QUAL-06 / 闭 Phase 49 GAP-1：worker capability 收紧。
		//   - 保留 --cap-add NET_ADMIN：sing-box 在容器内 netns 创建 tun0 设备
		//     需要 CAP_NET_ADMIN（Phase 54 单容器架构下由容器内 entrypoint 直接
		//     启动 sing-box，仍必须保留）。
		//   - 删除 --cap-add SYS_ADMIN：grep 业务代码不依赖；fuse mount 走
		//     `--device /dev/fuse + apparmor=unconfined` 路径，fusermount setuid
		//     root 即可，无需 SYS_ADMIN。
		//   - 显式 --cap-drop NET_RAW：docker 默认 capability 集合含 CAP_NET_RAW，
		//     必须显式 drop 才能去掉；移除后容器内 SOCK_RAW 创建立刻 PermissionDenied，
		//     闭 Phase 49 LEAK-06 攻击面。
		"--cap-add", "NET_ADMIN",
		"--cap-drop", "NET_RAW",
		// Phase 54：容器内 sing-box 需建 tun0，必须挂 /dev/net/tun 设备。
		// Phase 53 smoke.sh 已验证语法在 Linux + macOS Docker Desktop 上都被接受。
		"--device", "/dev/net/tun",
		"--device", "/dev/fuse",
		"--security-opt", "apparmor=unconfined",
		"--label", "cloud-cli-proxy.managed=true",
		"--label", fmt.Sprintf("cloud-cli-proxy.host_id=%s", request.HostID),
		"--hostname", hostname,
		"--shm-size", "1g",
		// Phase 47 Plan 02 I6 双保险：disable_ipv6 同时锁 all + default。
		// all 锁现有接口，default 锁未来创建的接口（如 Docker bridge reconnect）；
		// 仅设 all 会在某些 Docker / 内核组合下被 default 路径绕过。配合 worker
		// netns nft IPv6 表 input6 / output6 policy=drop，构成双层 fail-closed。
		"--sysctl", "net.ipv6.conf.all.disable_ipv6=1",
		"--sysctl", "net.ipv6.conf.default.disable_ipv6=1",
	}

	// macOS/Windows: expose SSH port via host port mapping because Docker Desktop
	// cannot route directly to container internal IPs from the host.
	if runtime.GOOS != "linux" {
		args = append(args, "-p", "0:22")
	}

	memoryLimit := dockerMemoryLimitValue(request.MemoryLimitMB)
	args = append(args, "--memory", memoryLimit, "--memory-swap", memoryLimit)

	cpuLimit := request.CPULimit
	if cpuLimit <= 0 {
		cpuLimit = defaultCPULimit
	}
	args = append(args, "--cpus", fmt.Sprintf("%.1f", cpuLimit))

	if request.EntryPassword == "" {
		return nil, fmt.Errorf("host %s entry_password is empty; refusing to build create args", request.HostID)
	}

	linuxUser := firstNonEmpty(request.DefaultUser, "workspace")
	args = append(args,
		"-e", "TZ="+firstNonEmpty(request.Timezone, "America/Los_Angeles"),
		"-e", "LANG=en_US.UTF-8",
		"-e", "LANGUAGE=en_US:en",
		"-e", "LC_ALL=en_US.UTF-8",
		"-e", "CONTAINER_USER="+linuxUser,
		"-e", "CONTAINER_SSH_PASSWORD="+request.EntryPassword,
		"-v", fmt.Sprintf("%s:%s", homeDir, firstNonEmpty(request.HomeMount, defaultWorkspaceMount)),
	)

	for _, vm := range request.Volumes {
		if vm.Name == "" || vm.Target == "" {
			return nil, fmt.Errorf("invalid volume mount: name=%q target=%q", vm.Name, vm.Target)
		}
		opts := fmt.Sprintf("type=volume,src=%s,dst=%s", vm.Name, vm.Target)
		if vm.ReadOnly {
			opts += ",readonly"
		}
		args = append(args, "--mount", opts)
	}

	for _, bm := range request.BindMounts {
		if bm.Source == "" || bm.Target == "" {
			return nil, fmt.Errorf("invalid bind mount: source=%q target=%q", bm.Source, bm.Target)
		}
		if !strings.HasPrefix(bm.Source, "/") || !strings.HasPrefix(bm.Target, "/") {
			return nil, fmt.Errorf("bind mount paths must be absolute: source=%q target=%q", bm.Source, bm.Target)
		}
		if err := os.MkdirAll(bm.Source, 0o755); err != nil {
			return nil, fmt.Errorf("create bind mount source dir %s: %w", bm.Source, err)
		}
		opts := fmt.Sprintf("type=bind,src=%s,dst=%s", bm.Source, bm.Target)
		if bm.ReadOnly {
			opts += ",readonly"
		}
		args = append(args, "--mount", opts)
	}

	for key, value := range request.Labels {
		args = append(args, "--label", fmt.Sprintf("%s=%s", key, value))
	}

	// 单容器架构下 host-agent 把 sing-box config.json 以 ro bind mount 注入到
	// user 容器内 /etc/sing-box/config.json。entrypoint start_singbox_or_die
	// 读取后即刻 shred -u 删除（D-V4-2 防 PoLP 泄漏）。
	//
	// 必须在 PrepareGateway 之后、docker create 之前注入（call-order 测试守护）。
	if egressCfg != nil && egressCfg.Proxy != nil {
		cfgPath := filepath.Join(network.SingBoxConfigDir(request.HostID), "config.json")
		args = append(args, "-v", cfgPath+":/etc/sing-box/config.json:ro")
	}

	args = append(args, request.ImageName)
	return args, nil
}

func (w *Worker) createHost(ctx context.Context, request agentapi.HostActionRequest) error {
	homeDir := firstNonEmpty(request.HomeDir, hostHomeDir(request.HostID))
	containerName := firstNonEmpty(request.ContainerName, containerNameForHost(request.HostID))
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		return fmt.Errorf("prepare host home dir %s: %w", homeDir, err)
	}

	w.pullImage(ctx, request.TaskID, request.HostID, request.ImageName)

	if exists, err := w.containerExists(ctx, containerName); err != nil {
		return err
	} else if exists {
		if err := w.runDocker(ctx, "rm", "-f", containerName); err != nil {
			return err
		}
	}

	// Phase 33 D-04/D-05/D-06：仅当 ClaudeAccountID 非空时自动补 claude-state volume + mount + upsert persistent_volume_name。
	// 空 ClaudeAccountID 走 D-07 fallback：跳过，不报错（v2.0 旧 host 重建路径）。
	if request.ClaudeAccountID != "" {
		volumeName, err := BuildClaudeStateVolumeName(request.ClaudeAccountID)
		if err != nil {
			return err
		}
		labels := map[string]string{
			claudeAccountLabelKey: request.ClaudeAccountID,
			claudeManagedLabelKey: claudeManagedLabelVal,
		}
		if err := ensureDockerVolume(ctx, volumeName, labels); err != nil {
			_, _ = w.repo.RecordEvent(ctx, repository.RecordEventParams{
				HostID:  &request.HostID,
				Level:   "error",
				Type:    "claude_account.volume_create_failed",
				Message: err.Error(),
				Metadata: map[string]any{
					"account_id":  request.ClaudeAccountID,
					"volume_name": volumeName,
				},
			})
			return fmt.Errorf("ensureDockerVolume: %w", err)
		}

		already := false
		for _, vm := range request.Volumes {
			if vm.Name == volumeName {
				already = true
				break
			}
		}
		if !already {
			request.Volumes = append(request.Volumes, agentapi.VolumeMount{
				Name:   volumeName,
				Target: claudeStateMountTarget,
				Labels: labels,
			})
		}

		if err := w.repo.UpsertClaudeAccountPersistentVolumeName(ctx, request.ClaudeAccountID, volumeName); err != nil {
			_, _ = w.repo.RecordEvent(ctx, repository.RecordEventParams{
				HostID:  &request.HostID,
				Level:   "warn",
				Type:    "claude_account.volume_name_persist_failed",
				Message: err.Error(),
				Metadata: map[string]any{
					"account_id":  request.ClaudeAccountID,
					"volume_name": volumeName,
				},
			})
		}
	}

	hostname := request.Hostname
	if hostname == "" {
		hostname = containerName
	}

	// Phase 54-01：在 docker create 之前先把 sing-box config 写到 host 端（54-02
	// 实现真正写盘逻辑，54-01 stub 返回 nil 让链路骨架就位），保证 worker 容器
	// 一旦 docker start，ro bind mount 接管的 /etc/sing-box/config.json 已存在。
	// 调用顺序硬约束（call_order_test.go 守护）：
	//   PrepareGateway → buildCreateArgs → docker create → docker start → PrepareHost
	egressCfg, err := w.buildEgressConfig(ctx, request.HostID)
	if err != nil {
		return err
	}
	// PrepareGateway 一旦写盘成功，任何后续失败（buildCreateArgs / docker create /
	// docker start / PrepareHost / waitForSSH）都必须把 host 端残留清干净，否则
	// 下次 createHost 之前 host 端遗留旧 config.json 可能让 docker create 用错
	// 配置启动 sing-box（资源泄漏 + 出口 IP 错绑 + 攻击面）。
	//
	// 用 gatewayPrepared 旗标 + defer 守护：成功路径末尾置 false 关闭 defer。
	// defer 内用 context.Background() 而非 ctx，避免 task ctx 已超时取消时
	// CleanupHost 也被中断。CleanupHost 内部已是 best-effort 幂等。
	var gatewayPrepared bool
	if egressCfg != nil {
		spec := network.HostNetworkSpec{
			HostID: request.HostID,
			Egress: egressCfg,
		}
		if err := w.provider.PrepareGateway(ctx, spec); err != nil {
			w.recordNetworkError(ctx, request.HostID, err)
			return fmt.Errorf("prepare gateway before create: %w", err)
		}
		gatewayPrepared = true
		defer func() {
			if !gatewayPrepared {
				return
			}
			cleanupCtx := context.Background()
			if cleanupErr := w.provider.CleanupHost(cleanupCtx, network.HostNetworkSpec{HostID: request.HostID}); cleanupErr != nil {
				w.recordNetworkError(cleanupCtx, request.HostID, fmt.Errorf("cleanup gateway after createHost failure: %w", cleanupErr))
			}
		}()
	}

	args, err := w.buildCreateArgs(request, containerName, hostname, egressCfg)
	if err != nil {
		return err
	}

	if err := w.runDocker(ctx, args...); err != nil {
		return err
	}

	w.connectComposeNetwork(ctx, containerName, false)

	if err := w.runDocker(ctx, "start", containerName); err != nil {
		return fmt.Errorf("start container after create: %w", err)
	}

	// 容器以 bridge 网络创建用于出站流量；compose 网络用于控制面访问容器。
	// start 前后都尝试接入 compose 网络，确保 entrypoint 能尽量看到最终网卡拓扑。
	if err := w.connectContainerNetworks(ctx, containerName); err != nil {
		return fmt.Errorf("connect container networks: %w", err)
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

	if err := w.waitForSSH(ctx, request, containerName); err != nil {
		return err
	}

	// 全部成功，关闭 defer 清理。
	gatewayPrepared = false
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

	// Phase 54-01：在 docker start 之前先把 sing-box config 写到 host 端，保证
	// worker 容器一旦运行，ro bind mount 引用的 /etc/sing-box/config.json 已就位。
	// PrepareGateway 是「mkdir + 写盘」幂等操作，重复调用安全。
	egressCfg, err := w.buildEgressConfig(ctx, request.HostID)
	if err != nil {
		return err
	}

	if egressCfg != nil {
		spec := network.HostNetworkSpec{
			HostID: request.HostID,
			Egress: egressCfg,
		}
		if err := w.provider.PrepareGateway(ctx, spec); err != nil {
			w.recordNetworkError(ctx, request.HostID, err)
			return fmt.Errorf("prepare gateway before start: %w", err)
		}
	}

	w.connectComposeNetwork(ctx, containerName, false)

	if err := w.runDocker(ctx, "start", containerName); err != nil {
		return err
	}

	if err := w.connectContainerNetworks(ctx, containerName); err != nil {
		return fmt.Errorf("connect container networks: %w", err)
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
			return fmt.Errorf("factory reset host home: %w", err)
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

func (w *Worker) prepareExistingHost(ctx context.Context, request agentapi.HostActionRequest) error {
	containerName := firstNonEmpty(request.ContainerName, containerNameForHost(request.HostID))
	if exists, err := w.containerExists(ctx, containerName); err != nil {
		return err
	} else if !exists {
		return fmt.Errorf("prepare host network: container %s does not exist", containerName)
	}

	running, err := w.containerRunning(ctx, containerName)
	if err != nil {
		return err
	}
	if !running {
		return fmt.Errorf("prepare host network: container %s is not running", containerName)
	}

	if err := w.connectContainerNetworks(ctx, containerName); err != nil {
		return fmt.Errorf("connect container networks before prepare: %w", err)
	}

	if err := w.reapplyLatestBypassSnapshot(ctx, request.HostID); err != nil {
		return fmt.Errorf("reapply latest bypass snapshot before prepare: %w", err)
	}

	egressCfg, err := w.buildEgressConfig(ctx, request.HostID)
	if err != nil {
		return err
	}
	if egressCfg == nil {
		return nil
	}

	spec := network.HostNetworkSpec{
		HostID: request.HostID,
		Egress: egressCfg,
	}
	if err := w.provider.PrepareHost(ctx, spec); err != nil {
		w.recordNetworkError(ctx, request.HostID, err)
		return fmt.Errorf("prepare existing host network: %w", err)
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
	// 出口 IP 自动纠正回调：验证阶段探测到真实出口 IP 与数据库不一致时自动更新。
	cfg.UpdateExpectedIP = func(ctx context.Context, newIP string) error {
		return w.repo.UpdateEgressIPAddress(ctx, cfg.EgressIPID, newIP)
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
	user := firstNonEmpty(request.DefaultUser, "workspace")
	if request.EntryPassword == "" {
		w.repo.RecordEvent(ctx, repository.RecordEventParams{
			HostID:  &request.HostID,
			Level:   "error",
			Type:    "runtime.entry_password_missing",
			Message: "host entry_password is empty; refusing to sync container credentials",
			Metadata: map[string]any{
				"host_id":   request.HostID,
				"container": containerName,
				"source":    "sync",
			},
		})
		return
	}
	pass := request.EntryPassword

	if out, err := execInContainer(ctx, containerName, "chpasswd", fmt.Sprintf("%s:%s\n", user, pass)); err != nil {
		w.repo.RecordEvent(ctx, repository.RecordEventParams{
			HostID:  &request.HostID,
			Level:   "warn",
			Type:    "runtime.password_sync_failed",
			Message: fmt.Sprintf("docker exec chpasswd failed: %s", strings.TrimSpace(string(out))),
		})
	}
}

func (w *Worker) injectSSHKeys(ctx context.Context, request agentapi.HostActionRequest, containerName string) {
	if len(request.SSHKeys) == 0 {
		if request.SSHPublicKey == "" && request.SSHPrivateKey == "" {
			return
		}
		w.injectSSHKeysLegacy(ctx, request, containerName)
		return
	}

	// 必须与容器内 CONTAINER_USER / entrypoint 一致；不能用平台用户名（request.Username）。
	user := firstNonEmpty(request.DefaultUser, "workspace")
	sshDir := "/workspace/.ssh"

	var managed []string
	if proxyPubKey := loadProxyPublicKey(); proxyPubKey != "" {
		managed = append(managed, proxyPubKey)
	}
	for _, key := range request.SSHKeys {
		if key.Purpose == "inbound" && key.PublicKey != "" {
			managed = append(managed, strings.TrimSpace(key.PublicKey))
		}
	}

	authorizedPath := sshDir + "/authorized_keys"
	existing, _ := containerReadFile(ctx, containerName, authorizedPath)
	merged := mergeAuthorizedKeys(existing, managed)

	switch {
	case merged == "":
		// managed 与 existing 均为空，彻底跳过，不创建文件
	case merged == existing:
		// 内容未变，保持幂等；仍尝试修正属主/权限（容错失败）
		w.fixFileOwnership(ctx, request, containerName, user, authorizedPath, "600")
	default:
		writeScript := fmt.Sprintf(
			"mkdir -p %s && cat > %s && chmod 600 %s && chown %s:%s %s",
			sshDir, authorizedPath, authorizedPath, user, user, authorizedPath,
		)
		if out, err := execInContainer(ctx, containerName, writeScript, merged); err != nil {
			w.repo.RecordEvent(ctx, repository.RecordEventParams{
				HostID:  &request.HostID,
				Level:   "warn",
				Type:    "runtime.ssh_authorized_keys_failed",
				Message: fmt.Sprintf("inject authorized_keys failed: %s", strings.TrimSpace(string(out))),
			})
		}
	}

	outboundIdx := 0
	for _, key := range request.SSHKeys {
		if key.Purpose != "outbound" {
			continue
		}

		var keyFile, pubFile string
		if outboundIdx == 0 {
			if key.KeyType == "rsa" || strings.Contains(key.PublicKey, "ssh-rsa") {
				keyFile = sshDir + "/id_rsa"
				pubFile = sshDir + "/id_rsa.pub"
			} else {
				keyFile = sshDir + "/id_ed25519"
				pubFile = sshDir + "/id_ed25519.pub"
			}
		} else {
			safeName := key.Label
			if safeName == "" {
				safeName = fmt.Sprintf("id_%d", outboundIdx)
			}
			keyFile = sshDir + "/" + safeName
			pubFile = sshDir + "/" + safeName + ".pub"
		}

		if key.PrivateKey != "" {
			if containerFileNonEmpty(ctx, containerName, keyFile) {
				w.repo.RecordEvent(ctx, repository.RecordEventParams{
					HostID:   &request.HostID,
					Level:    "info",
					Type:     "runtime.ssh_key_skipped_existing",
					Message:  "outbound private key file already present, skip overwrite",
					Metadata: map[string]any{"host_id": request.HostID, "file": keyFile},
				})
				w.fixFileOwnership(ctx, request, containerName, user, keyFile, "600")
			} else {
				script := fmt.Sprintf(
					"mkdir -p %s && cat > %s && chmod 600 %s && chown %s:%s %s",
					sshDir, keyFile, keyFile, user, user, keyFile,
				)
				if out, err := execInContainer(ctx, containerName, script, key.PrivateKey); err != nil {
					w.repo.RecordEvent(ctx, repository.RecordEventParams{
						HostID:  &request.HostID,
						Level:   "warn",
						Type:    "runtime.ssh_key_inject_failed",
						Message: fmt.Sprintf("inject outbound private key failed: %s", strings.TrimSpace(string(out))),
					})
				}
			}
		}

		if key.PublicKey != "" {
			if containerFileNonEmpty(ctx, containerName, pubFile) {
				w.repo.RecordEvent(ctx, repository.RecordEventParams{
					HostID:   &request.HostID,
					Level:    "info",
					Type:     "runtime.ssh_key_skipped_existing",
					Message:  "outbound public key file already present, skip overwrite",
					Metadata: map[string]any{"host_id": request.HostID, "file": pubFile},
				})
				w.fixFileOwnership(ctx, request, containerName, user, pubFile, "644")
			} else {
				script := fmt.Sprintf(
					"mkdir -p %s && cat > %s && chmod 644 %s && chown %s:%s %s",
					sshDir, pubFile, pubFile, user, user, pubFile,
				)
				if out, err := execInContainer(ctx, containerName, script, key.PublicKey); err != nil {
					w.repo.RecordEvent(ctx, repository.RecordEventParams{
						HostID:  &request.HostID,
						Level:   "warn",
						Type:    "runtime.ssh_key_inject_failed",
						Message: fmt.Sprintf("inject outbound public key failed: %s", strings.TrimSpace(string(out))),
					})
				}
			}
		}

		outboundIdx++
	}
}

func (w *Worker) injectSSHKeysLegacy(ctx context.Context, request agentapi.HostActionRequest, containerName string) {
	user := firstNonEmpty(request.DefaultUser, "workspace")
	sshDir := "/workspace/.ssh"

	if request.SSHPrivateKey != "" {
		keyFile := sshDir + "/id_ed25519"
		if strings.Contains(request.SSHPublicKey, "ssh-rsa") {
			keyFile = sshDir + "/id_rsa"
		}

		if containerFileNonEmpty(ctx, containerName, keyFile) {
			w.repo.RecordEvent(ctx, repository.RecordEventParams{
				HostID:   &request.HostID,
				Level:    "info",
				Type:     "runtime.ssh_key_skipped_existing",
				Message:  "legacy private key file already present, skip overwrite",
				Metadata: map[string]any{"host_id": request.HostID, "file": keyFile},
			})
			w.fixFileOwnership(ctx, request, containerName, user, keyFile, "600")
		} else {
			script := fmt.Sprintf(
				"mkdir -p %s && cat > %s && chmod 600 %s && chown %s:%s %s",
				sshDir, keyFile, keyFile, user, user, keyFile,
			)
			if out, err := execInContainer(ctx, containerName, script, request.SSHPrivateKey); err != nil {
				w.repo.RecordEvent(ctx, repository.RecordEventParams{
					HostID:  &request.HostID,
					Level:   "warn",
					Type:    "runtime.ssh_key_inject_failed",
					Message: fmt.Sprintf("inject private key failed: %s", strings.TrimSpace(string(out))),
				})
			}
		}
	}

	if request.SSHPublicKey != "" {
		pubKeyFile := sshDir + "/id_ed25519.pub"
		if strings.Contains(request.SSHPublicKey, "ssh-rsa") {
			pubKeyFile = sshDir + "/id_rsa.pub"
		}

		if containerFileNonEmpty(ctx, containerName, pubKeyFile) {
			w.repo.RecordEvent(ctx, repository.RecordEventParams{
				HostID:   &request.HostID,
				Level:    "info",
				Type:     "runtime.ssh_key_skipped_existing",
				Message:  "legacy public key file already present, skip overwrite",
				Metadata: map[string]any{"host_id": request.HostID, "file": pubKeyFile},
			})
			w.fixFileOwnership(ctx, request, containerName, user, pubKeyFile, "644")
		} else {
			script := fmt.Sprintf(
				"mkdir -p %s && cat > %s && chmod 644 %s && chown %s:%s %s",
				sshDir, pubKeyFile, pubKeyFile, user, user, pubKeyFile,
			)
			if out, err := execInContainer(ctx, containerName, script, request.SSHPublicKey); err != nil {
				w.repo.RecordEvent(ctx, repository.RecordEventParams{
					HostID:  &request.HostID,
					Level:   "warn",
					Type:    "runtime.ssh_key_inject_failed",
					Message: fmt.Sprintf("inject public key failed: %s", strings.TrimSpace(string(out))),
				})
			}
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
		ID:          eip.ID,
		IPAddress:   eip.IPAddress,
		TunnelType:  network.TunnelTypeProxy,
		ProxyConfig: eip.ProxyConfig,
	}, nil
}

func (rv *repoValidator) UpdateEgressIPAddress(ctx context.Context, egressIPID string, newIP string) error {
	return rv.repo.UpdateEgressIPAddress(ctx, egressIPID, newIP)
}

// execInContainer 在目标容器中以 `docker exec -i <container> bash -c <script>` 执行，
// 支持可选 stdin。暴露为 package-level 变量以便单元测试注入 fake。
var execInContainer = func(ctx context.Context, container, script, stdin string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "docker", "exec", "-i", container, "bash", "-c", script)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	return cmd.CombinedOutput()
}

// containerFileNonEmpty 判断容器内文件是否存在且非空。任何异常一律返回 false，
// 调用方据此走"需要写入"分支，保持原有自举路径可用。
// path 通过 stdin 传入，避免脚本字符串层面做 shell 拼接。
func containerFileNonEmpty(ctx context.Context, container, path string) bool {
	script := `P=$(cat) && [ -s "$P" ] && echo y || echo n`
	out, err := execInContainer(ctx, container, script, path)
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "y"
}

// containerReadFile 读取容器内文件；不存在或读失败都返回 ("", false)，
// 存在则返回完整原始内容与 true。path 通过 stdin 传入。
func containerReadFile(ctx context.Context, container, path string) (string, bool) {
	script := `P=$(cat) && [ -f "$P" ] && cat "$P" || exit 42`
	out, err := execInContainer(ctx, container, script, path)
	if err != nil {
		return "", false
	}
	return string(out), true
}

// mergeAuthorizedKeys 在已有 authorized_keys 内容与控制面权威条目之间做"marker 块合并"：
//   - existing 为空或不可读：managed 空 → 返回空串（调用方应跳过写入，避免创建空文件）；
//     managed 非空 → 只返回 marker 块包裹的 managed 内容。
//   - existing 非空且包含 marker 对：把 marker 对中间（含 marker）替换为新 marker 块；
//     managed 为空时整段删除，保留 marker 之外的其他行。
//   - existing 非空但没有 marker 对：managed 非空 → 末尾追加新 marker 块；managed 空 → 返回 existing 原样。
//
// 结果保证以 `\n` 结尾（除非整体为空字符串）；不做整体 TrimSpace，以免破坏用户自加行。
func mergeAuthorizedKeys(existing string, managed []string) string {
	if existing == "" {
		if len(managed) == 0 {
			return ""
		}
		return sshManagedBeginMarker + "\n" + strings.Join(managed, "\n") + "\n" + sshManagedEndMarker + "\n"
	}

	lines := strings.Split(existing, "\n")
	beginIdx, endIdx := -1, -1
	for i, line := range lines {
		if beginIdx == -1 && line == sshManagedBeginMarker {
			beginIdx = i
			continue
		}
		if beginIdx != -1 && line == sshManagedEndMarker {
			endIdx = i
			break
		}
	}

	var managedBlock []string
	if len(managed) > 0 {
		managedBlock = append(managedBlock, sshManagedBeginMarker)
		managedBlock = append(managedBlock, managed...)
		managedBlock = append(managedBlock, sshManagedEndMarker)
	}

	var result []string
	if beginIdx != -1 && endIdx != -1 {
		result = append(result, lines[:beginIdx]...)
		result = append(result, managedBlock...)
		result = append(result, lines[endIdx+1:]...)
	} else {
		if len(managed) == 0 {
			return existing
		}
		result = append(result, lines...)
		for len(result) > 0 && result[len(result)-1] == "" {
			result = result[:len(result)-1]
		}
		result = append(result, managedBlock...)
	}

	for len(result) > 0 && result[len(result)-1] == "" {
		result = result[:len(result)-1]
	}

	if len(result) == 0 {
		return ""
	}
	return strings.Join(result, "\n") + "\n"
}

// fixFileOwnership 只修属主/权限，不重写内容；chown/chmod 失败仅记录 warn 事件，不阻断后续流程。
func (w *Worker) fixFileOwnership(ctx context.Context, request agentapi.HostActionRequest, containerName, user, path, mode string) {
	script := fmt.Sprintf("chown %s:%s %s && chmod %s %s", user, user, path, mode, path)
	if out, err := execInContainer(ctx, containerName, script, ""); err != nil {
		w.repo.RecordEvent(ctx, repository.RecordEventParams{
			HostID:   &request.HostID,
			Level:    "warn",
			Type:     "runtime.ssh_key_chown_failed",
			Message:  fmt.Sprintf("chown %s failed: %s", path, strings.TrimSpace(string(out))),
			Metadata: map[string]any{"host_id": request.HostID, "file": path},
		})
	}
}

func loadProxyPublicKey() string {
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "/var/lib/cloud-cli-proxy"
	}
	pubKeyPath := dataDir + "/ssh_host_ed25519_key.pub"
	data, err := os.ReadFile(pubKeyPath)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func (w *Worker) pullImage(ctx context.Context, taskID, hostID, imageName string) {
	pullCtx, cancel := context.WithTimeout(ctx, pullImageTimeout)
	defer cancel()

	cmd := exec.CommandContext(pullCtx, "docker", "pull", "--progress=plain", imageName)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		slog.Warn("docker pull stderr pipe failed, falling back to sync pull",
			"image", imageName, "error", err)
		w.fallbackPullImage(pullCtx, imageName)
		return
	}

	if err := cmd.Start(); err != nil {
		slog.Warn("docker pull start failed, falling back to sync pull",
			"image", imageName, "error", err)
		return
	}

	tracker := newPullProgressTracker(taskID, hostID, w)
	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		line := scanner.Text()
		tracker.feed(line)
	}

	if err := cmd.Wait(); err != nil {
		timedOut := errors.Is(pullCtx.Err(), context.DeadlineExceeded)
		slog.Warn("docker pull failed, will use local image if available",
			"image", imageName,
			"error", err,
			"timed_out", timedOut,
			"timeout", pullImageTimeout)
		return
	}
	slog.Info("pulled latest image", "image", imageName)
}

func (w *Worker) fallbackPullImage(ctx context.Context, imageName string) {
	cmd := exec.CommandContext(ctx, "docker", "pull", imageName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		slog.Warn("docker pull failed, will use local image if available",
			"image", imageName, "error", err,
			"output", strings.TrimSpace(string(output)))
		return
	}
	slog.Info("pulled latest image", "image", imageName)
}

type pullProgressTracker struct {
	taskID     string
	hostID     string
	worker     *Worker
	layers     map[string]string // layerID -> status
	completed  int
	lastReport time.Time
}

func newPullProgressTracker(taskID, hostID string, worker *Worker) *pullProgressTracker {
	return &pullProgressTracker{
		taskID:     taskID,
		hostID:     hostID,
		worker:     worker,
		layers:     make(map[string]string),
		lastReport: time.Now().Add(-time.Second),
	}
}

func (t *pullProgressTracker) feed(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}

	// docker pull --progress=plain 每行格式: "layerID: status"
	parts := strings.SplitN(line, ": ", 2)
	if len(parts) != 2 {
		return
	}

	layerID := strings.TrimSpace(parts[0])
	status := strings.TrimSpace(parts[1])

	// 过滤掉非层ID的行（如 "latest: Pulling from ..."）
	// 层ID通常是12位以上的十六进制字符串
	if len(layerID) < 12 {
		return
	}
	isHex := true
	for _, c := range layerID {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			isHex = false
			break
		}
	}
	if !isHex {
		return
	}

	wasComplete := t.isComplete(t.layers[layerID])
	t.layers[layerID] = status
	isComplete := t.isComplete(status)

	if !wasComplete && isComplete {
		t.completed++
	}

	t.maybeReport()
}

func (t *pullProgressTracker) isComplete(status string) bool {
	return strings.Contains(status, "complete") ||
		strings.Contains(status, "Already exists") ||
		strings.Contains(status, "Pull complete")
}

func (t *pullProgressTracker) maybeReport() {
	total := len(t.layers)
	if total == 0 {
		return
	}

	now := time.Now()
	// 限制上报频率，避免频繁写库
	if now.Sub(t.lastReport) < 500*time.Millisecond {
		return
	}
	t.lastReport = now

	percent := t.completed * 100 / total
	message := fmt.Sprintf("拉取镜像中 (%d/%d 层)", t.completed, total)
	if percent >= 100 {
		message = "镜像拉取完成"
		percent = 100
	}

	t.worker.ReportTaskProgress(context.Background(), t.taskID, percent, message)

	layersCopy := make(map[string]string, len(t.layers))
	for k, v := range t.layers {
		layersCopy[k] = v
	}
	broadcast.BroadcastJSON("tasks", map[string]any{
		"topic":  "tasks",
		"action": "progress",
		"id":     t.taskID,
		"payload": map[string]any{
			"percent": percent,
			"message": message,
			"host_id": t.hostID,
			"layers":  layersCopy,
		},
	})
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

func (w *Worker) containerRunning(ctx context.Context, name string) (bool, error) {
	cmd := exec.CommandContext(ctx, "docker", "container", "inspect", "-f", "{{.State.Running}}", name)
	out, err := cmd.Output()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok && exitError.ExitCode() != 0 {
			return false, nil
		}
		return false, fmt.Errorf("inspect docker container running state %s: %w", name, err)
	}
	return strings.TrimSpace(string(out)) == "true", nil
}

func (w *Worker) runDocker(ctx context.Context, args ...string) error {
	output, err := dockerRunner(ctx, args...)
	if err != nil {
		return fmt.Errorf("docker %s: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}

	return nil
}

var dockerRunner = func(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	return cmd.CombinedOutput()
}

// connectComposeNetwork 将容器接入 compose 网络（控制面 VNC 访问需要）。
// 容器已经在 create 时通过 --network bridge 接入了 bridge 网络。
func (w *Worker) connectComposeNetwork(ctx context.Context, containerName string, warnOnError bool) {
	composeNetwork := os.Getenv("COMPOSE_NETWORK")
	if composeNetwork == "" {
		composeNetwork = "cloud-cli-proxy_default"
	}

	// compose 网络允许控制面通过容器 IP 直连 VNC/SSH 等端口。
	// docker network connect 对已连接容器幂等，不需额外判重。
	if err := w.runDocker(ctx, "network", "connect", composeNetwork, containerName); err != nil {
		if !warnOnError {
			return
		}
		// compose 网络不存在时降级为警告（非 compose 部署场景）
		slog.Warn("connect container to compose network failed, continuing",
			"container", containerName,
			"network", composeNetwork,
			"error", err,
		)
	}
}

// connectContainerNetworks 做 start 后的网络收尾：
//   - 再次尝试接入 compose 网络，兼容 Docker 不允许 stopped container connect 的环境；
//   - 触发新镜像内的 sing-box egress interface 同步脚本；
//   - 重新锁定 resolv.conf，覆盖 Docker network connect 注入的 127.0.0.11。
func (w *Worker) connectContainerNetworks(ctx context.Context, containerName string) error {
	w.connectComposeNetwork(ctx, containerName, true)
	if err := syncContainerSingBoxInterface(ctx, containerName); err != nil {
		slog.Warn("sync container sing-box egress interface failed, continuing",
			"container", containerName,
			"error", err,
		)
	}
	if err := lockContainerDNS(ctx, containerName); err != nil {
		return fmt.Errorf("re-lock container DNS after network connect: %w", err)
	}
	return nil
}

func syncContainerSingBoxInterface(ctx context.Context, containerName string) error {
	script := strings.Join([]string{
		"set -e",
		"if command -v cloud-cli-proxy-sync-singbox-interface >/dev/null 2>&1; then",
		"  cloud-cli-proxy-sync-singbox-interface --source /etc/sing-box/config.json --config /run/sing-box/config.json --if-present",
		"else",
		"  echo 'cloud-cli-proxy-sync-singbox-interface not found; skip'",
		"fi",
	}, "\n")
	out, err := execInContainer(ctx, containerName, script, "")
	if err != nil {
		return fmt.Errorf("sync sing-box egress interface: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func lockContainerDNS(ctx context.Context, containerName string) error {
	script := strings.Join([]string{
		"set -e",
		"chattr -i /etc/resolv.conf 2>/dev/null || true",
		"cat > /etc/resolv.conf",
		"chmod 0644 /etc/resolv.conf",
		"chattr +i /etc/resolv.conf 2>/dev/null || true",
	}, "\n")
	out, err := execInContainer(ctx, containerName, script, containerDNSLockContent)
	if err != nil {
		return fmt.Errorf("write /etc/resolv.conf: %w (%s)", err, strings.TrimSpace(string(out)))
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

const (
	claudeStateVolumePrefix = "claude-state-"
	claudeStateMountTarget  = "/var/lib/claude-persist"
	claudeAccountLabelKey   = "com.cloud-cli-proxy.account_id"
	claudeManagedLabelKey   = "com.cloud-cli-proxy.managed"
	claudeManagedLabelVal   = "true"
)

// BuildClaudeStateVolumeName 返回 D-01 规范的 volume 名 `claude-state-{id}`（保留 UUID 原格式含连字符）。空 id 返回错误。
func BuildClaudeStateVolumeName(claudeAccountID string) (string, error) {
	if claudeAccountID == "" {
		return "", fmt.Errorf("BuildClaudeStateVolumeName: claude_account_id is required")
	}
	return claudeStateVolumePrefix + claudeAccountID, nil
}

// dockerVolumeRunner 抽象 docker volume 子命令的实际执行；包级 var 便于单元测试注入 mock。
// 与 var execInContainer = ... 模式一致。
var dockerVolumeRunner = func(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "docker", append([]string{"volume"}, args...)...)
	return cmd.CombinedOutput()
}

// ensureDockerVolume 幂等创建 named volume（D-04）：
//   - inspect 成功：视为已存在，返回 nil
//   - inspect 失败：执行 create --label k=v --label k=v <name>
//
// 暴露为包级 var 以便测试注入 mock。
var ensureDockerVolume = realEnsureDockerVolume

func realEnsureDockerVolume(ctx context.Context, name string, labels map[string]string) error {
	if name == "" {
		return fmt.Errorf("ensureDockerVolume: empty name")
	}
	if _, err := dockerVolumeRunner(ctx, "inspect", name); err == nil {
		return nil
	}
	args := []string{"create"}
	for k, v := range labels {
		args = append(args, "--label", fmt.Sprintf("%s=%s", k, v))
	}
	args = append(args, name)
	out, err := dockerVolumeRunner(ctx, args...)
	if err != nil {
		return fmt.Errorf("docker volume create %s: %w (%s)", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// removeDockerVolume 幂等删除（D-15）：
//   - "no such volume" 视为成功（幂等）
//   - "volume is in use" 包装为 volume_in_use: 前缀错误（供 Execute 错误码映射）
//   - force=true 追加 -f 标志
var removeDockerVolume = realRemoveDockerVolume

func realRemoveDockerVolume(ctx context.Context, name string, force bool) error {
	if name == "" {
		return fmt.Errorf("removeDockerVolume: empty name")
	}
	args := []string{"rm"}
	if force {
		args = append(args, "-f")
	}
	args = append(args, name)
	out, err := dockerVolumeRunner(ctx, args...)
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(string(out))
	low := strings.ToLower(msg)
	if strings.Contains(low, "no such volume") {
		return nil
	}
	if strings.Contains(low, "volume is in use") {
		return fmt.Errorf("volume_in_use: %s", msg)
	}
	return fmt.Errorf("docker volume rm %s: %w (%s)", name, err, msg)
}

// removeVolumes 处理 ActionVolumeRemove：遍历 request.Volumes 调 removeDockerVolume，
// 任一失败立即写 audit event 并 return（D-14 + D-21 metadata 不写凭据）。
func (w *Worker) removeVolumes(ctx context.Context, request agentapi.HostActionRequest) error {
	force := request.Labels["force"] == "true"
	for _, vm := range request.Volumes {
		if err := removeDockerVolume(ctx, vm.Name, force); err != nil {
			_, _ = w.repo.RecordEvent(ctx, repository.RecordEventParams{
				Level:   "error",
				Type:    "claude_account.volume_rm_failed",
				Message: err.Error(),
				Metadata: map[string]any{
					"volume_name": vm.Name,
					"force":       force,
				},
			})
			return err
		}
	}
	return nil
}
