package repository

import (
	"encoding/json"
	"time"
)

type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusSucceeded TaskStatus = "succeeded"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCanceled  TaskStatus = "canceled"
)

type User struct {
	ID            string     `json:"id"`
	Username      string     `json:"username"`
	Status        string     `json:"status"`
	Role          string     `json:"role"`
	ShortID       string     `json:"short_id"`
	PasswordHash  string     `json:"password_hash,omitempty"`
	EntryPassword string     `json:"entry_password,omitempty"`
	SSHPublicKey  string     `json:"ssh_public_key,omitempty"`
	SSHPrivateKey string     `json:"ssh_private_key,omitempty"`
	SSHKeyType    string     `json:"ssh_key_type,omitempty"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type Host struct {
	ID               string    `json:"id"`
	UserID           string    `json:"user_id"`
	Status           string    `json:"status"`
	ShortID          string    `json:"short_id"`
	TemplateImageRef string    `json:"template_image_ref"`
	HomeVolumeName   string    `json:"home_volume_name"`
	SlotKey          string    `json:"slot_key"`
	Timezone         string    `json:"timezone"`
	Hostname         string    `json:"hostname"`
	MemoryLimitMB    int       `json:"memory_limit_mb"`
	CPULimit         float64   `json:"cpu_limit"`
	DiskLimitGB      int       `json:"disk_limit_gb"`
	HostMounts       HostMounts `json:"host_mounts"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type HostMount struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	ReadOnly bool   `json:"read_only,omitempty"`
}

type HostMounts []HostMount

// HostSSHAuth holds the data needed by the SSH proxy resolver.
type HostSSHAuth struct {
	HostID        string
	EntryPassword string
	HostStatus    string
	UserID        string
	UserStatus    string
	Username      string // SSH 登录用户名（外部标识）
	// ContainerUser 是容器内实际存在的 Unix 用户名（如 workspace）。
	// SSH proxy 连接容器时使用此用户，而非 Username。
	ContainerUser string
	// TemplateImageRef 对应 hosts.template_image_ref。
	// Phase 30 Plan 02 会基于它推导 Entry API 的 image_version / supports_* 能力字段（D-05/D-06/D-07）。
	// 数据层仅透传原值，不在此处做 tag 解析，避免数据层与协议层职责耦合。
	TemplateImageRef string
	// SSHPrivateKey 是控制面连接容器用的私钥（PEM 格式）。
	// 当非空时，SSH proxy 优先使用公钥认证连接容器。
	SSHPrivateKey string
}

type HostBinding struct {
	BindingID  string    `json:"binding_id"`
	HostID     string    `json:"host_id"`
	EgressIPID string    `json:"egress_ip_id"`
	CreatedAt  time.Time `json:"created_at"`
}

type Task struct {
	ID               string     `json:"id"`
	HostID           *string    `json:"host_id,omitempty"`
	Kind             string     `json:"kind"`
	Status           TaskStatus `json:"status"`
	RequestedBy      string     `json:"requested_by"`
	ErrorCode        string     `json:"error_code,omitempty"`
	ErrorMessage     string     `json:"error_message,omitempty"`
	LastErrorSummary string     `json:"last_error_summary,omitempty"`
	ProgressPercent  int        `json:"progress_percent"`
	ProgressMessage  string     `json:"progress_message"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type Event struct {
	ID        string         `json:"id"`
	TaskID    *string        `json:"task_id,omitempty"`
	HostID    *string        `json:"host_id,omitempty"`
	UserID    *string        `json:"user_id,omitempty"`
	Level     string         `json:"level"`
	Type      string         `json:"type"`
	Message   string         `json:"message"`
	Metadata  map[string]any `json:"metadata"`
	CreatedAt time.Time      `json:"created_at"`
}

type EgressIP struct {
	ID          string          `json:"id"`
	Label       string          `json:"label"`
	IPAddress   string          `json:"ip_address"`
	Provider    string          `json:"provider"`
	Status      string          `json:"status"`
	ProxyConfig json.RawMessage `json:"proxy_config,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type DashboardStats struct {
	ActiveUsers  int `json:"active_users"`
	RunningHosts int `json:"running_hosts"`
	AvailableIPs int `json:"available_ips"`
}

type AdminConfig struct {
	Username  string
	Password  string
	JWTSecret []byte
}

type BootstrapUserAuth struct {
	UserID       string
	Username     string
	PasswordHash string
	Status       string
	ShortID      string
}

type CreateEgressIPParams struct {
	Label       string
	IPAddress   string
	Provider    string
	ProxyConfig json.RawMessage
}

type UpdateEgressIPParams struct {
	Label       string
	IPAddress   string
	Provider    string
	Status      string
	ProxyConfig json.RawMessage
}

type HostDetail struct {
	Host     Host          `json:"host"`
	User     User          `json:"user"`
	Bindings []BindingWithIP `json:"bindings"`
}

type BindingWithIP struct {
	BindingID string    `json:"binding_id"`
	EgressIP  EgressIP  `json:"egress_ip"`
	CreatedAt time.Time `json:"created_at"`
}

type HostWithUsername struct {
	Host
	Username       string  `json:"username"`
	EgressIPLabel  *string `json:"egress_ip_label,omitempty"`
	EgressIPAddr   *string `json:"egress_ip_address,omitempty"`
	DockerStatus   string  `json:"docker_status,omitempty"`
}

// HostWithClaudeAccount D-23：纯 DB JOIN，避免在 detail handler 引入 docker exec。
// 配合 GetHostWithClaudeAccount LEFT JOIN 使用；空 PersistentVolumeName = 该 host 关联 account 未分配 volume 或无 account。
type HostWithClaudeAccount struct {
	Host
	PersistentVolumeName string `json:"persistent_volume_name,omitempty"`
}

type UserHostSummary struct {
	ID        string    `json:"id"`
	Hostname  string    `json:"hostname"`
	Status    string    `json:"status"`
	EgressIP  string    `json:"egress_ip"`
	CreatedAt time.Time `json:"created_at"`
}

type UserHostDetail struct {
	ID             string              `json:"id"`
	Hostname       string              `json:"hostname"`
	Status         string              `json:"status"`
	Timezone       string              `json:"timezone"`
	CreatedAt      time.Time           `json:"created_at"`
	UpdatedAt      time.Time           `json:"updated_at"`
	EgressBindings []UserEgressBinding `json:"egress_bindings"`
	ConnectionInfo *ConnectionInfo     `json:"connection_info,omitempty"`
}

type ConnectionInfo struct {
	CurlCommand string `json:"curl_command"`
	SSHCommand  string `json:"ssh_command"`
	SSHPort     int    `json:"ssh_port"`
	VNCURL      string `json:"vnc_url,omitempty"`
}

type UserEgressBinding struct {
	IPAddress string `json:"ip_address"`
}

type CreateUserParams struct {
	Username      string
	PasswordHash  string
	ShortID       string
	EntryPassword string
}

type CreateUserWithRoleParams struct {
	Username     string
	PasswordHash string
	ShortID      string
	Role         string
}

type ClaudeAccount struct {
	ID     string  `json:"id"`
	UserID string  `json:"user_id"`
	HostID *string `json:"host_id,omitempty"`
	Email  string  `json:"email"`
	// PersistentVolumeName 对应 claude_accounts.persistent_volume_name。
	// 语义（Phase 30 D-02）：nil = 控制面尚未为该账号分配持久化 volume；
	// 非 nil 值 = 已分配的 Docker named volume 名称，规范化为 claude-state-{claude_account_id}（D-01）。
	PersistentVolumeName *string   `json:"persistent_volume_name,omitempty"`
	DisplayName          string    `json:"display_name"`
	Status               string    `json:"status"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type SSHKey struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	Purpose     string    `json:"purpose"`
	Label       string    `json:"label"`
	PublicKey   string    `json:"public_key"`
	PrivateKey  string    `json:"private_key,omitempty"`
	KeyType     string    `json:"key_type"`
	Fingerprint string    `json:"fingerprint"`
	CreatedAt   time.Time `json:"created_at"`
}

type CreateTaskParams struct {
	HostID           *string
	Kind             string
	Status           TaskStatus
	RequestedBy      string
	ErrorCode        string
	ErrorMessage     string
	LastErrorSummary string
}

type UpsertHostParams struct {
	UserID           string
	Status           string
	ShortID          string
	TemplateImageRef string
	HomeVolumeName   string
	SlotKey          string
	Timezone         string
	Hostname         string
	MemoryLimitMB    int
	CPULimit         float64
	DiskLimitGB      int
	HostMounts       HostMounts
}

type RecordEventParams struct {
	TaskID   *string
	HostID   *string
	UserID   *string
	Level    string
	Type     string
	Message  string
	Metadata map[string]any
}

type ListEventsParams struct {
	EventType string
	UserID    string
	HostID    string
	Since     time.Time
	Until     time.Time
	Limit     int
	Offset    int
}

type ListEventsResult struct {
	Events []Event `json:"events"`
	Total  int     `json:"total"`
}

// ---------------------------------------------------------------------------
// Phase 45 Plan 03 — Bypass / Whitelist data model
// ---------------------------------------------------------------------------
//
// 五类核心实体 + 对应 *Params：BypassPreset / BypassRule / BypassBinding /
// BypassSnapshot / BypassAuditLog。所有 UUID 字段在 Go 层用 string，与
// queries.go 现有 `id::text` 模式对齐；JSONB 字段用 json.RawMessage 或专门
// 的内嵌结构。可空 UUID 列（host_id / preset_id / rule_id / created_by /
// actor_id / target_id）用 *string；可空 JSONB 用 json.RawMessage。

// BypassPresetRule 是 host_bypass_presets.rules JSONB 数组元素。
type BypassPresetRule struct {
	RuleType string `json:"rule_type"`
	Value    string `json:"value"`
	Note     string `json:"note,omitempty"`
}

type BypassPreset struct {
	ID          string             `json:"id"`
	Slug        string             `json:"slug"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	IsSystem    bool               `json:"is_system"`
	IsForceOn   bool               `json:"is_force_on"`
	IsActive    bool               `json:"is_active"`
	Rules       []BypassPresetRule `json:"rules"`
	CreatedAt   time.Time          `json:"created_at"`
	UpdatedAt   time.Time          `json:"updated_at"`
}

type CreateBypassPresetParams struct {
	Slug        string
	Name        string
	Description string
	IsForceOn   bool
	IsActive    bool
	Rules       []BypassPresetRule
}

// UpdateBypassPresetParams：所有字段为指针 = nil 表示不更新（COALESCE 兜底）。
type UpdateBypassPresetParams struct {
	Name        *string
	Description *string
	IsForceOn   *bool
	IsActive    *bool
	Rules       *[]BypassPresetRule
}

type BypassRule struct {
	ID        string    `json:"id"`
	Scope     string    `json:"scope"` // global / host
	HostID    *string   `json:"host_id,omitempty"`
	RuleType  string    `json:"rule_type"`
	Value     string    `json:"value"`
	Note      string    `json:"note,omitempty"`
	IsRisky   bool      `json:"is_risky"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreateBypassRuleParams struct {
	Scope    string
	HostID   *string
	RuleType string
	Value    string
	Note     string
	IsRisky  bool
}

type UpdateBypassRuleParams struct {
	Value   *string
	Note    *string
	IsRisky *bool
}

type BypassBinding struct {
	ID        string    `json:"id"`
	HostID    string    `json:"host_id"`
	PresetID  *string   `json:"preset_id,omitempty"`
	RuleID    *string   `json:"rule_id,omitempty"`
	Enabled   bool      `json:"enabled"`
	Source    string    `json:"source"` // admin / system
	CreatedAt time.Time `json:"created_at"`
}

type CreateBypassBindingParams struct {
	HostID   string
	PresetID *string
	RuleID   *string
	Enabled  bool
	Source   string
}

type BypassSnapshot struct {
	ID                   string          `json:"id"`
	HostID               string          `json:"host_id"`
	Version              int64           `json:"version"`
	ConfigHash           string          `json:"config_hash"`
	WhitelistCIDRsJSON   json.RawMessage `json:"whitelist_cidrs_json"`
	WhitelistDomainsJSON json.RawMessage `json:"whitelist_domains_json"`
	AppliedStatus        string          `json:"applied_status"` // pending / applied / failed / rolled_back
	Source               string          `json:"source"`         // apply / rollback (Phase 46 Plan 02 migration 0020)
	CreatedBy            *string         `json:"created_by,omitempty"`
	CreatedAt            time.Time       `json:"created_at"`
}

type CreateBypassSnapshotParams struct {
	HostID               string
	Version              int64
	ConfigHash           string
	WhitelistCIDRsJSON   json.RawMessage
	WhitelistDomainsJSON json.RawMessage
	Source               string // "apply" / "rollback"，空字符串走 SQL DEFAULT 'apply'
	CreatedBy            *string
}

type BypassAuditLog struct {
	ID         string          `json:"id"`
	ActorID    *string         `json:"actor_id,omitempty"`
	ActorIP    string          `json:"actor_ip,omitempty"`
	Action     string          `json:"action"`
	TargetKind string          `json:"target_kind"`
	TargetID   *string         `json:"target_id,omitempty"`
	Before     json.RawMessage `json:"before,omitempty"`
	After      json.RawMessage `json:"after,omitempty"`
	Note       string          `json:"note,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
}

type InsertBypassAuditLogParams struct {
	ActorID    *string
	ActorIP    string
	Action     string
	TargetKind string
	TargetID   *string
	Before     json.RawMessage
	After      json.RawMessage
	Note       string
}
