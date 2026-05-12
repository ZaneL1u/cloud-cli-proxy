package runtime

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/zanel1u/cloud-cli-proxy/internal/agentapi"
	"github.com/zanel1u/cloud-cli-proxy/internal/broadcast"
	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

func expandBindMountSource(src string) string {
	if !strings.HasPrefix(src, "~/") && src != "~" {
		return src
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return src
	}
	if src == "~" {
		return home
	}
	return filepath.Join(home, src[2:])
}

const (
	DefaultImageLockPath      = "deploy/docker/managed-user/image.lock"
	defaultRebuildMode        = "preserve-home"
	defaultManagedUserSlotKey = "primary"
	defaultDataDir            = "/var/lib/cloud-cli-proxy"
)

type RuntimeSpec struct {
	ImageName          string
	ImageVersion       string
	DefaultUser        string
	HomeMount          string
	RebuildModeDefault string
	SupportsMergerfs   bool
}

// QueueHostActionRepo 是 Service 在排队 host 操作时需要的仓储接口。
// 抽出独立类型避免 struct 字段 + 构造器形参两处重复声明（Phase 33 后置消化 Plan 01 carry-over）。
type QueueHostActionRepo interface {
	GetHost(context.Context, string) (repository.Host, error)
	GetUser(context.Context, string) (repository.User, error)
	CreateTask(context.Context, repository.CreateTaskParams) (repository.Task, error)
	ListSSHKeysByUser(context.Context, string) ([]repository.SSHKey, error)
	RecordEvent(context.Context, repository.RecordEventParams) (repository.Event, error)
	// ResolveClaudeAccountIDForEntry 按 Phase 30 D-05 的两阶段规则返回 claude_account_id：
	// 优先匹配 host 显式绑定，否则回退到 user 未绑定 host 的最早账号。
	// 用于 Phase 33 D-04..D-06：让 worker.createHost 在该字段非空时自动补 claude-state-<id> volume。
	ResolveClaudeAccountIDForEntry(ctx context.Context, userID, hostID string) (string, bool, error)
}

type Service struct {
	repo QueueHostActionRepo
	dispatcher interface {
		Dispatch(context.Context, agentapi.HostActionRequest) (agentapi.HostActionResponse, error)
	}
	imageLockPath string
	dataDir       string
}

func NewService(
	repo QueueHostActionRepo,
	dispatcher interface {
		Dispatch(context.Context, agentapi.HostActionRequest) (agentapi.HostActionResponse, error)
	},
	imageLockPath string,
) *Service {
	if imageLockPath == "" {
		imageLockPath = DefaultImageLockPath
	}

	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = defaultDataDir
	}

	return &Service{
		repo:          repo,
		dispatcher:    dispatcher,
		imageLockPath: imageLockPath,
		dataDir:       dataDir,
	}
}

// QueueHostAction 入队一个 host action。
//
//   - requestedBy 是触发者标识（admin UI / system / user-id），写入 audit + task.requested_by。
//   - bypassSnapshotID 仅当 action == ActionReloadHostBypass 时被透传到 request.BypassSnapshotID；
//     其它 action 调用方传 "" 即可，本函数会按 action 类型 gating，避免误用语义。
//
// Phase 47 Plan 01：把 Phase 46 旧实现「借用 requestedBy 形参承载 snapshot ID」的 hack
// 改为显式字段透传，让 admin_bypass_snapshots / runtime_service 的契约自洽。
func (s *Service) QueueHostAction(ctx context.Context, hostID string, action agentapi.HostAction, requestedBy string, bypassSnapshotID string) (repository.Task, error) {
	host, err := s.repo.GetHost(ctx, hostID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return repository.Task{}, fmt.Errorf("host %s not found: %w", hostID, err)
		}
		return repository.Task{}, fmt.Errorf("load host: %w", err)
	}

	spec, err := LoadRuntimeSpec(s.imageLockPath)
	if err != nil {
		return repository.Task{}, fmt.Errorf("load image.lock runtime spec: %w", err)
	}

	owner, err := s.repo.GetUser(ctx, host.UserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return repository.Task{}, fmt.Errorf("host owner user %s not found: %w", host.UserID, err)
		}
		return repository.Task{}, fmt.Errorf("load host owner user: %w", err)
	}

	// Phase 33 D-04/D-07：解析 claude_account_id 注入 request，供 worker.createHost 自动补
	// claude-state-<id> named volume（D-04）+ upsert persistent_volume_name（D-06）。
	// 解析失败或未命中时走 D-07 fallback：claudeAccountID 留空，createHost 跳过自动补 volume，
	// 不阻塞容器启动（v2.0 旧 host 重建路径）。
	claudeAccountID, _, claudeErr := s.repo.ResolveClaudeAccountIDForEntry(ctx, host.UserID, host.ID)
	if claudeErr != nil {
		slog.Warn("resolve claude_account_id failed, will skip volume auto-attach (D-07 fallback)",
			"host_id", host.ID, "user_id", host.UserID, "error", claudeErr)
	}

	task, err := s.repo.CreateTask(ctx, repository.CreateTaskParams{
		HostID:      &host.ID,
		Kind:        string(action),
		Status:      repository.TaskStatusPending,
		RequestedBy: requestedBy,
	})
	if err != nil {
		return repository.Task{}, fmt.Errorf("create lifecycle task: %w", err)
	}

	var keyEntries []agentapi.SSHKeyEntry
	sshKeys, sshKeysErr := s.repo.ListSSHKeysByUser(ctx, host.UserID)
	if sshKeysErr != nil {
		slog.Warn("failed to query ssh keys, container will have no ssh keys",
			"host_id", hostID, "user_id", host.UserID, "error", sshKeysErr)
	} else {
		for _, k := range sshKeys {
			keyEntries = append(keyEntries, agentapi.SSHKeyEntry{
				Purpose:    k.Purpose,
				Label:      k.Label,
				PublicKey:  k.PublicKey,
				PrivateKey: k.PrivateKey,
				KeyType:    k.KeyType,
			})
		}
	}

	request := agentapi.HostActionRequest{
		TaskID:        task.ID,
		HostID:        host.ID,
		Action:        action,
		ImageName:     spec.ImageName,
		DefaultUser:   spec.DefaultUser,
		HomeMount:     spec.HomeMount,
		RebuildMode:   spec.RebuildModeDefault,
		ContainerName: containerNameForHost(host.ID),
		HomeDir:       fmt.Sprintf("%s/hosts/%s/home", s.dataDir, host.ID),
		Labels: map[string]string{
			"cloud-cli-proxy.host_id":  host.ID,
			"cloud-cli-proxy.slot_key": firstNonEmpty(host.SlotKey, defaultManagedUserSlotKey),
		},
		Timezone:      host.Timezone,
		Hostname:      host.Hostname,
		MemoryLimitMB: defaultIntIfZero(host.MemoryLimitMB, 4096),
		CPULimit:      defaultFloatIfZero(host.CPULimit, 2.0),
		Username:        owner.Username,
		EntryPassword:   owner.EntryPassword,
		SSHPublicKey:    "",
		SSHPrivateKey:   "",
		SSHKeys:         keyEntries,
		ClaudeAccountID: claudeAccountID,
	}

	// 宿主机 bind mount 映射：repository.HostMounts -> agentapi.BindMounts
	if len(host.HostMounts) > 0 {
		request.BindMounts = make([]agentapi.BindMount, 0, len(host.HostMounts))
		for _, hm := range host.HostMounts {
			request.BindMounts = append(request.BindMounts, agentapi.BindMount{
				Source:   expandBindMountSource(hm.Source),
				Target:   hm.Target,
				ReadOnly: hm.ReadOnly,
			})
		}
	}

	// Phase 47 Plan 01：仅当 action 是 reload_host_bypass 时把 snapshot ID 透传给 worker。
	// 其它 action 即使调用方误传非空 bypassSnapshotID 也不会污染 request（避免语义混淆）。
	if action == agentapi.ActionReloadHostBypass {
		request.BypassSnapshotID = bypassSnapshotID
	}

	if request.EntryPassword == "" {
		hid := hostID
		if s.repo != nil {
			_, _ = s.repo.RecordEvent(ctx, repository.RecordEventParams{
				HostID:  &hid,
				Level:   "error",
				Type:    "runtime.entry_password_missing",
				Message: "host entry_password is empty; refusing to queue action",
				Metadata: map[string]any{
					"host_id": hostID,
					"action":  string(action),
					"source":  "queue",
				},
			})
		}
		slog.Error("refusing to queue host action: entry_password empty",
			"host_id", hostID, "action", action)
		return repository.Task{}, fmt.Errorf("host %s entry_password is empty; refusing to queue %s", hostID, action)
	}
	slog.Info("queuing host action",
		"host_id", hostID, "action", action,
		"username", request.Username,
		"has_entry_password", request.EntryPassword != "")

	go func() {
		resp, derr := s.dispatcher.Dispatch(context.Background(), request)
		if derr != nil {
			slog.Error("host action dispatch failed",
				"task_id", task.ID,
				"host_id", hostID,
				"action", action,
				"error", derr)
			if s.repo != nil {
				_, _ = s.repo.RecordEvent(context.Background(), repository.RecordEventParams{
					HostID:  &hostID,
					Level:   "error",
					Type:    "runtime.dispatch_failed",
					Message: derr.Error(),
					Metadata: map[string]any{
						"task_id": task.ID,
						"action":  string(action),
					},
				})
			}
			broadcast.Broadcast("tasks", "update", task.ID)
			return
		}
		if resp.Update.Status == string(repository.TaskStatusFailed) {
			slog.Error("host action execution failed",
				"task_id", task.ID,
				"host_id", hostID,
				"error_code", resp.Update.ErrorCode,
				"error_message", resp.Update.ErrorMessage)
		}
		broadcast.Broadcast("tasks", "update", task.ID)
	}()

	return task, nil
}

func LoadRuntimeSpec(path string) (RuntimeSpec, error) {
	file, err := os.Open(path)
	if err != nil {
		return RuntimeSpec{}, fmt.Errorf("open runtime spec: %w", err)
	}
	defer file.Close()

	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		values[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}

	if err := scanner.Err(); err != nil {
		return RuntimeSpec{}, fmt.Errorf("scan runtime spec: %w", err)
	}

	spec := RuntimeSpec{
		ImageName:          values["image_name"],
		ImageVersion:       values["image_version"],
		DefaultUser:        values["default_user"],
		HomeMount:          values["home_mount"],
		RebuildModeDefault: firstNonEmpty(values["rebuild_mode_default"], defaultRebuildMode),
		SupportsMergerfs:   values["supports_mergerfs"] == "true",
	}

	if spec.ImageName == "" {
		return RuntimeSpec{}, fmt.Errorf("image.lock missing image_name")
	}
	if spec.HomeMount == "" {
		return RuntimeSpec{}, fmt.Errorf("image.lock missing home_mount")
	}
	if spec.DefaultUser == "" {
		return RuntimeSpec{}, fmt.Errorf("image.lock missing default_user")
	}

	return spec, nil
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

func defaultIntIfZero(value, fallback int) int {
	if value == 0 {
		return fallback
	}
	return value
}

func defaultFloatIfZero(value, fallback float64) float64 {
	if value == 0 {
		return fallback
	}
	return value
}
