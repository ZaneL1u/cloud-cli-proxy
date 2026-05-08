package local

import (
	"context"
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultImage       = "ghcr.io/zanel1u/cloud-cli-proxy/managed-user:latest"
	defaultUser        = "workspace"
	defaultMemoryMB    = 4096
	defaultCPULimit    = 2.0
	defaultPasswordLen = 16
	containerPrefix    = "cloud-claude-local-"
)

// LocalOptions configures local container behavior.
type LocalOptions struct {
	ProjectDir    string // Current project directory (for container name hash and workspace mount)
	Port          int    // 0 = auto-assign
	EgressConfig  string // --egress-config file path, empty = disabled
	ImageName     string // Default from image.lock or built-in default
	MemoryLimitMB int    // Default 4096
	CPULimit      float64 // Default 2.0
}

// SSHInfo holds the SSH connection details for a local container.
type SSHInfo struct {
	Host     string
	Port     string
	User     string
	Password string
}

// ContainerStatus holds the runtime status of a local container.
type ContainerStatus struct {
	Name        string
	Status      string // "running", "exited", "not_found"
	SSHPort     string
	Image       string
	CreatedAt   string
	PortMapping string // "127.0.0.1:{port}->22/tcp"
}

// LocalManager manages local Docker containers for cloud-claude.
type LocalManager struct {
	opts   LocalOptions
	runner DockerRunner
}

// NewLocalManager creates a new LocalManager with the given options.
func NewLocalManager(opts LocalOptions) *LocalManager {
	if opts.ImageName == "" {
		opts.ImageName = defaultImage
	}
	if opts.MemoryLimitMB == 0 {
		opts.MemoryLimitMB = defaultMemoryMB
	}
	if opts.CPULimit == 0 {
		opts.CPULimit = defaultCPULimit
	}
	return &LocalManager{
		opts:   opts,
		runner: DefaultDockerRunner,
	}
}

// NewLocalManagerWithRunner creates a LocalManager with a custom DockerRunner (for testing).
func NewLocalManagerWithRunner(opts LocalOptions, runner DockerRunner) *LocalManager {
	m := NewLocalManager(opts)
	m.runner = runner
	return m
}

// Up starts a local container and returns SSH connection info.
func (m *LocalManager) Up(ctx context.Context) (SSHInfo, error) {
	projectDir, err := filepath.Abs(m.opts.ProjectDir)
	if err != nil {
		return SSHInfo{}, fmt.Errorf("resolve project dir: %w", err)
	}

	password, err := GeneratePassword(defaultPasswordLen)
	if err != nil {
		return SSHInfo{}, fmt.Errorf("generate password: %w", err)
	}

	containerName := localContainerName(projectDir)

	// Remove existing container if present
	exists, err := containerExists(ctx, m.runner, containerName)
	if err != nil {
		return SSHInfo{}, fmt.Errorf("check existing container: %w", err)
	}
	if exists {
		_, _ = m.runner(ctx, "stop", containerName)
		if err := m.runDocker(ctx, "rm", "-f", containerName); err != nil {
			return SSHInfo{}, fmt.Errorf("remove existing container: %w", err)
		}
	}

	// Build create args
	args := m.buildCreateArgs(containerName, projectDir, password)

	// Inject egress config if provided
	if m.opts.EgressConfig != "" {
		_, egressMode, err := ValidateEgressConfig(m.opts.EgressConfig)
		if err != nil {
			return SSHInfo{}, fmt.Errorf("validate egress config: %w", err)
		}
		// Add egress-specific capabilities for tun mode
		if egressMode == EgressModeTun {
			args = append(args, "--cap-add", "NET_ADMIN", "--device", "/dev/net/tun")
		}
		// Mount outbound config into container
		args = append(args, "-v", egressMountArg(m.opts.EgressConfig))
		args = append(args, "-e", "SING_BOX_MODE="+string(egressMode))
	}

	// Create container
	if err := m.runDocker(ctx, args...); err != nil {
		return SSHInfo{}, fmt.Errorf("create container: %w", err)
	}

	// Start container
	if err := m.runDocker(ctx, "start", containerName); err != nil {
		return SSHInfo{}, fmt.Errorf("start container: %w", err)
	}

	// Extract SSH port
	sshPort, err := inspectSSHPort(ctx, m.runner, containerName)
	if err != nil {
		return SSHInfo{}, fmt.Errorf("get SSH port: %w", err)
	}

	return SSHInfo{
		Host:     "127.0.0.1",
		Port:     sshPort,
		User:     defaultUser,
		Password: password,
	}, nil
}

// Down stops and removes the local container. Idempotent.
func (m *LocalManager) Down(ctx context.Context) error {
	containerName := localContainerName(m.opts.ProjectDir)

	exists, err := containerExists(ctx, m.runner, containerName)
	if err != nil {
		return fmt.Errorf("check container: %w", err)
	}
	if !exists {
		return nil
	}

	_, _ = m.runner(ctx, "stop", containerName)
	if err := m.runDocker(ctx, "rm", "-f", containerName); err != nil {
		return fmt.Errorf("remove container: %w", err)
	}

	return nil
}

// Status returns the runtime status of the local container.
func (m *LocalManager) Status(ctx context.Context) (*ContainerStatus, error) {
	containerName := localContainerName(m.opts.ProjectDir)

	status, err := inspectContainerStatus(ctx, m.runner, containerName)
	if err != nil {
		return nil, err
	}
	if status == "not_found" {
		return &ContainerStatus{Name: containerName, Status: "not_found"}, nil
	}

	sshPort, _ := inspectSSHPort(ctx, m.runner, containerName)

	imageOut, _ := m.runner(ctx, "inspect", "--format={{.Config.Image}}", containerName)
	createdAtOut, _ := m.runner(ctx, "inspect", "--format={{.Created}}", containerName)

	portMapping := ""
	if sshPort != "" {
		portMapping = fmt.Sprintf("127.0.0.1:%s->22/tcp", sshPort)
	}

	return &ContainerStatus{
		Name:        containerName,
		Status:      status,
		SSHPort:     sshPort,
		Image:       strings.TrimSpace(string(imageOut)),
		CreatedAt:   strings.TrimSpace(string(createdAtOut)),
		PortMapping: portMapping,
	}, nil
}

// buildCreateArgs builds the docker create command arguments.
func (m *LocalManager) buildCreateArgs(containerName, projectDir, password string) []string {
	hostname := containerName
	args := []string{
		"create",
		"--name", containerName,
		"--hostname", hostname,
		"--shm-size", "1g",
		"-e", "MODE=local",
		"-e", "TZ=" + envOrDefault("TZ", "America/Los_Angeles"),
		"-e", "LANG=en_US.UTF-8",
		"-e", "LANGUAGE=en_US:en",
		"-e", "LC_ALL=en_US.UTF-8",
		"-e", "CONTAINER_USER=" + defaultUser,
		"-e", "CONTAINER_SSH_PASSWORD=" + password,
		"-v", projectDir + ":/workspace",
	}

	// Inject user's SSH public key for key-based auth (required by VS Code Remote-SSH)
	if key, err := findSSHPublicKey(); err == nil && key != "" {
		args = append(args, "-e", "CONTAINER_SSH_AUTHORIZED_KEY="+key)
	}

	// Expose SSH port via Docker -p
	if m.opts.Port > 0 {
		args = append(args, "-p", fmt.Sprintf("%d:22", m.opts.Port))
	} else {
		args = append(args, "-p", "0:22")
	}

	if m.opts.MemoryLimitMB > 0 {
		args = append(args, "--memory", fmt.Sprintf("%dm", m.opts.MemoryLimitMB))
	}
	if m.opts.CPULimit > 0 {
		args = append(args, "--cpus", fmt.Sprintf("%.1f", m.opts.CPULimit))
	}

	args = append(args, m.opts.ImageName)
	return args
}

func (m *LocalManager) runDocker(ctx context.Context, args ...string) error {
	_, err := m.runner(ctx, args...)
	return err
}

// localContainerName generates a deterministic container name from the project directory.
func localContainerName(projectDir string) string {
	abs, _ := filepath.Abs(projectDir)
	hash := md5.Sum([]byte(abs))
	return fmt.Sprintf("%s%x", containerPrefix, hash[:4])
}

// findSSHPublicKey returns the contents of the first available SSH public key.
func findSSHPublicKey() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	for _, name := range []string{"id_ed25519.pub", "id_rsa.pub", "id_ecdsa.pub"} {
		data, err := os.ReadFile(filepath.Join(home, ".ssh", name))
		if err == nil {
			return strings.TrimSpace(string(data)), nil
		}
	}
	return "", nil
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
