package agentapi

type HostAction string

const (
	ActionCreateHost   HostAction = "create_host"
	ActionStartHost    HostAction = "start_host"
	ActionStopHost     HostAction = "stop_host"
	ActionRebuildHost  HostAction = "rebuild_host"
	ActionPrepareHost  HostAction = "prepare_host"
	ActionVolumeRemove HostAction = "volume_remove" // Phase 33 D-13
)

type SSHKeyEntry struct {
	Purpose    string `json:"purpose"`
	Label      string `json:"label"`
	PublicKey  string `json:"public_key"`
	PrivateKey string `json:"private_key,omitempty"`
	KeyType    string `json:"key_type"`
}

// VolumeMount 描述 docker create --mount type=volume 的最小契约。
// Phase 29 仅支持 named volume；生命周期（create/rm）由 Phase 33 管理。
type VolumeMount struct {
	Name     string            `json:"name"`
	Target   string            `json:"target"`
	ReadOnly bool              `json:"read_only,omitempty"`
	Labels   map[string]string `json:"labels,omitempty"`
}

// BindMount 描述 docker create --mount type=bind 的宿主机路径映射。
type BindMount struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	ReadOnly bool   `json:"read_only,omitempty"`
}

type HostActionRequest struct {
	TaskID        string            `json:"task_id"`
	HostID        string            `json:"host_id"`
	Action        HostAction        `json:"action"`
	ImageName     string            `json:"image_name"`
	DefaultUser   string            `json:"default_user"`
	HomeMount     string            `json:"home_mount"`
	RebuildMode   string            `json:"rebuild_mode"`
	ContainerName string            `json:"container_name"`
	HomeDir       string            `json:"home_dir"`
	Labels        map[string]string `json:"labels"`
	Timezone      string            `json:"timezone"`
	Hostname      string            `json:"hostname"`
	MemoryLimitMB int               `json:"memory_limit_mb,omitempty"`
	CPULimit      float64           `json:"cpu_limit,omitempty"`
	Username      string            `json:"username,omitempty"`
	EntryPassword string            `json:"entry_password,omitempty"`
	SSHPublicKey  string            `json:"ssh_public_key,omitempty"`
	SSHPrivateKey string            `json:"ssh_private_key,omitempty"`
	SSHKeys       []SSHKeyEntry     `json:"ssh_keys,omitempty"`
	Volumes       []VolumeMount     `json:"volumes,omitempty"`
	// ClaudeAccountID 携带 Phase 30 D-09 规定的账号维度标识，供 Phase 33 worker
	// 组装 `claude-state-{claude_account_id}` volume 与容器 label 使用。
	// `omitempty` 是契约：空串表示"本次 action 无账号维度"，禁止写入空字符串来表达"已分配但未知"。
	ClaudeAccountID string `json:"claude_account_id,omitempty"`
	// BindMounts 携带宿主机目录 bind mount 配置，由 Runtime Service 从 repository.HostMounts 映射而来。
	BindMounts []BindMount `json:"bind_mounts,omitempty"`
}

type TaskStatusUpdate struct {
	TaskID           string `json:"task_id"`
	Status           string `json:"status"`
	ErrorCode        string `json:"error_code,omitempty"`
	ErrorMessage     string `json:"error_message,omitempty"`
	LastErrorSummary string `json:"last_error_summary,omitempty"`
	ProgressPercent  int    `json:"progress_percent,omitempty"`
	ProgressMessage  string `json:"progress_message,omitempty"`
}

type HostActionResponse struct {
	TaskID        string           `json:"task_id"`
	Action        HostAction       `json:"action"`
	ContainerName string           `json:"container_name"`
	Update        TaskStatusUpdate `json:"update"`
}

type ContainerStatusResponse struct {
	Name    string `json:"name"`
	Exists  bool   `json:"exists"`
	Running bool   `json:"running"`
}
