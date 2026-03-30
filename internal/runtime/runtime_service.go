package runtime

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/zanel1u/cloud-cli-proxy/internal/agentapi"
	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

const (
	DefaultImageLockPath      = "deploy/docker/managed-user/image.lock"
	defaultRebuildMode        = "preserve-home"
	defaultManagedUserSlotKey = "primary"
	defaultDataDir            = "/var/lib/cloud-cli-proxy"
)

type RuntimeSpec struct {
	ImageName          string
	DefaultUser        string
	HomeMount          string
	RebuildModeDefault string
}

type Service struct {
	repo interface {
		GetHost(context.Context, string) (repository.Host, error)
		GetUser(context.Context, string) (repository.User, error)
		CreateTask(context.Context, repository.CreateTaskParams) (repository.Task, error)
	}
	dispatcher interface {
		Dispatch(context.Context, agentapi.HostActionRequest) (agentapi.HostActionResponse, error)
	}
	imageLockPath string
	dataDir       string
}

func NewService(
	repo interface {
		GetHost(context.Context, string) (repository.Host, error)
		GetUser(context.Context, string) (repository.User, error)
		CreateTask(context.Context, repository.CreateTaskParams) (repository.Task, error)
	},
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

func (s *Service) QueueHostAction(ctx context.Context, hostID string, action agentapi.HostAction, requestedBy string) (repository.Task, error) {
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

	task, err := s.repo.CreateTask(ctx, repository.CreateTaskParams{
		HostID:      &host.ID,
		Kind:        string(action),
		Status:      repository.TaskStatusPending,
		RequestedBy: requestedBy,
	})
	if err != nil {
		return repository.Task{}, fmt.Errorf("create lifecycle task: %w", err)
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
		Username:      owner.Username,
		EntryPassword: host.EntryPassword,
		SSHPublicKey:  owner.SSHPublicKey,
		SSHPrivateKey: owner.SSHPrivateKey,
	}

	if request.EntryPassword == "" {
		slog.Warn("host entry_password is empty, container will use default password",
			"host_id", hostID, "action", action, "username", owner.Username)
	}
	slog.Info("queuing host action",
		"host_id", hostID, "action", action,
		"username", request.Username,
		"has_entry_password", request.EntryPassword != "")

	go func() {
		_, _ = s.dispatcher.Dispatch(context.Background(), request)
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
		DefaultUser:        values["default_user"],
		HomeMount:          values["home_mount"],
		RebuildModeDefault: firstNonEmpty(values["rebuild_mode_default"], defaultRebuildMode),
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
