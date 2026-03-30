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
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type Host struct {
	ID               string    `json:"id"`
	UserID           string    `json:"user_id"`
	Status           string    `json:"status"`
	ShortID          string    `json:"short_id"`
	EntryPassword    string    `json:"entry_password,omitempty"`
	TemplateImageRef string    `json:"template_image_ref"`
	HomeVolumeName   string    `json:"home_volume_name"`
	SlotKey          string    `json:"slot_key"`
	Timezone         string    `json:"timezone"`
	Hostname         string    `json:"hostname"`
	MemoryLimitMB    int       `json:"memory_limit_mb"`
	CPULimit         float64   `json:"cpu_limit"`
	DiskLimitGB      int       `json:"disk_limit_gb"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// HostSSHAuth holds the data needed by the SSH proxy resolver.
type HostSSHAuth struct {
	HostID        string
	HostShortID   string
	EntryPassword string
	HostStatus    string
	UserID        string
	UserStatus    string
	Username      string // 关联用户的 username，用于连接容器内 sshd
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
	ID             string    `json:"id"`
	Label          string    `json:"label"`
	IPAddress      string    `json:"ip_address"`
	Provider       string    `json:"provider"`
	Status         string    `json:"status"`
	TunnelType     string    `json:"tunnel_type"`
	WgEndpoint     *string   `json:"wg_endpoint,omitempty"`
	WgPublicKey    *string   `json:"wg_public_key,omitempty"`
	WgPresharedKey *string   `json:"wg_preshared_key,omitempty"`
	WgAllowedIPs   string    `json:"wg_allowed_ips"`
	WgDNSServer    *string   `json:"wg_dns_server,omitempty"`
	WgPeerAddress  *string          `json:"wg_peer_address,omitempty"`
	ProxyConfig    json.RawMessage  `json:"proxy_config,omitempty"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
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
	Label          string
	IPAddress      string
	Provider       string
	WgEndpoint     *string
	WgPublicKey    *string
	WgPresharedKey *string
	WgAllowedIPs   string
	WgDNSServer    *string
	WgPeerAddress  *string
	TunnelType     string
	ProxyConfig    json.RawMessage
}

type UpdateEgressIPParams struct {
	Label          string
	IPAddress      string
	Provider       string
	Status         string
	WgEndpoint     *string
	WgPublicKey    *string
	WgPresharedKey *string
	WgAllowedIPs   string
	WgDNSServer    *string
	WgPeerAddress  *string
	TunnelType     string
	ProxyConfig    json.RawMessage
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
	IPAddress  string `json:"ip_address"`
	TunnelType string `json:"tunnel_type"`
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
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	HostID      *string   `json:"host_id,omitempty"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
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
	EntryPassword    string
	TemplateImageRef string
	HomeVolumeName   string
	SlotKey          string
	Timezone         string
	Hostname         string
	MemoryLimitMB    int
	CPULimit         float64
	DiskLimitGB      int
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
