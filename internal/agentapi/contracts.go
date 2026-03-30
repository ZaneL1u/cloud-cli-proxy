package agentapi

type HostAction string

const (
	ActionCreateHost  HostAction = "create_host"
	ActionStartHost   HostAction = "start_host"
	ActionStopHost    HostAction = "stop_host"
	ActionRebuildHost HostAction = "rebuild_host"
	ActionPrepareHost HostAction = "prepare_host"
)

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
}

type TaskStatusUpdate struct {
	TaskID           string `json:"task_id"`
	Status           string `json:"status"`
	ErrorCode        string `json:"error_code,omitempty"`
	ErrorMessage     string `json:"error_message,omitempty"`
	LastErrorSummary string `json:"last_error_summary,omitempty"`
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
