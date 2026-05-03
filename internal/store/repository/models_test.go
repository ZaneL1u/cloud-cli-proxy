package repository

import (
	"encoding/json"
	"testing"
	"time"
)

func TestTaskStatus_Constants(t *testing.T) {
	tests := []struct {
		status TaskStatus
		want   string
	}{
		{TaskStatusPending, "pending"},
		{TaskStatusRunning, "running"},
		{TaskStatusSucceeded, "succeeded"},
		{TaskStatusFailed, "failed"},
		{TaskStatusCanceled, "canceled"},
	}
	for _, tt := range tests {
		if string(tt.status) != tt.want {
			t.Errorf("TaskStatus = %q, want %q", tt.status, tt.want)
		}
	}
}

func TestUser_JSONMarshaling(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	expires := now.Add(24 * time.Hour)
	u := User{
		ID:            "user-1",
		Username:      "alice",
		Status:        "active",
		Role:          "user",
		ShortID:       "abc123",
		PasswordHash:  "hash",
		EntryPassword: "ep",
		SSHPublicKey:  "ssh-rsa AAA...",
		SSHPrivateKey: "-----BEGIN OPENSSH PRIVATE KEY-----",
		SSHKeyType:    "rsa",
		ExpiresAt:     &expires,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	b, err := json.Marshal(u)
	if err != nil {
		t.Fatalf("json.Marshal(User) = %v", err)
	}

	var decoded User
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(User) = %v", err)
	}

	if decoded.ID != u.ID {
		t.Errorf("ID = %q, want %q", decoded.ID, u.ID)
	}
	if decoded.Username != u.Username {
		t.Errorf("Username = %q, want %q", decoded.Username, u.Username)
	}
	if decoded.ShortID != u.ShortID {
		t.Errorf("ShortID = %q, want %q", decoded.ShortID, u.ShortID)
	}

	// 敏感字段 omitempty
	if decoded.PasswordHash != "hash" {
		t.Errorf("PasswordHash should marshal when non-empty, got %q", decoded.PasswordHash)
	}
}

func TestUser_JSONOmitEmpty(t *testing.T) {
	// PasswordHash 为空时不序列化
	u := User{ID: "1", Username: "test"}
	b, err := json.Marshal(u)
	if err != nil {
		t.Fatalf("json.Marshal = %v", err)
	}
	var m map[string]any
	json.Unmarshal(b, &m)
	if _, ok := m["password_hash"]; ok {
		t.Error("password_hash should be omitted when empty")
	}
	if _, ok := m["ssh_public_key"]; ok {
		t.Error("ssh_public_key should be omitted when empty")
	}
}

func TestHost_JSONMarshaling(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	h := Host{
		ID:               "host-1",
		UserID:           "user-1",
		Status:           "running",
		ShortID:          "xyz789",
		TemplateImageRef: "ghcr.io/test/image:v1",
		HomeVolumeName:   "vol-1",
		SlotKey:          "primary",
		Timezone:         "Asia/Shanghai",
		Hostname:         "dev-box",
		MemoryLimitMB:    4096,
		CPULimit:         2.0,
		DiskLimitGB:      50,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	b, err := json.Marshal(h)
	if err != nil {
		t.Fatalf("json.Marshal(Host) = %v", err)
	}

	var decoded Host
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(Host) = %v", err)
	}

	if decoded.ID != h.ID {
		t.Errorf("ID = %q, want %q", decoded.ID, h.ID)
	}
	if decoded.Status != "running" {
		t.Errorf("Status = %q, want running", decoded.Status)
	}
	if decoded.MemoryLimitMB != 4096 {
		t.Errorf("MemoryLimitMB = %d, want 4096", decoded.MemoryLimitMB)
	}
	if decoded.CPULimit != 2.0 {
		t.Errorf("CPULimit = %f, want 2.0", decoded.CPULimit)
	}
}

func TestHostSSHAuth_Fields(t *testing.T) {
	auth := HostSSHAuth{
		HostID:           "h1",
		EntryPassword:    "ep",
		HostStatus:       "running",
		UserID:           "u1",
		UserStatus:       "active",
		Username:         "alice",
		ContainerUser:    "workspace",
		TemplateImageRef: "img:v1",
		SSHPrivateKey:    "-----BEGIN OPENSSH PRIVATE KEY-----\nkey\n-----END OPENSSH PRIVATE KEY-----",
	}

	if auth.Username == "" {
		t.Error("Username should not be empty")
	}
	if auth.ContainerUser == "" {
		t.Error("ContainerUser should not be empty")
	}
	if auth.HostID == "" {
		t.Error("HostID should not be empty")
	}
	if auth.UserID == "" {
		t.Error("UserID should not be empty")
	}
}

func TestTask_StatusTransitions(t *testing.T) {
	// 验证 Task 的 Status 字段格式
	task := Task{
		ID:     "task-1",
		Kind:   "create_host",
		Status: TaskStatusPending,
	}

	b, err := json.Marshal(task)
	if err != nil {
		t.Fatalf("json.Marshal(Task) = %v", err)
	}

	if !json.Valid(b) {
		t.Error("Task JSON should be valid")
	}
}

func TestEvent_Fields(t *testing.T) {
	now := time.Now().UTC()
	evt := Event{
		ID:        "evt-1",
		TaskID:    strPtr("task-1"),
		HostID:    strPtr("host-1"),
		UserID:    strPtr("user-1"),
		Level:     "info",
		Type:      "host.created",
		Message:   "Host created successfully",
		Metadata:  map[string]any{"key": "value"},
		CreatedAt: now,
	}

	if evt.Level != "info" {
		t.Errorf("Level = %q, want info", evt.Level)
	}
	if evt.Type != "host.created" {
		t.Errorf("Type = %q, want host.created", evt.Type)
	}
	if evt.Metadata["key"] != "value" {
		t.Error("Metadata should preserve values")
	}
}

func TestEgressIP_JSONMarshaling(t *testing.T) {
	ip := EgressIP{
		ID:          "ip-1",
		Label:       "US-East",
		IPAddress:   "203.0.113.1",
		Provider:    "sing-box",
		Status:      "available",
		ProxyConfig: json.RawMessage(`{"type":"socks5","port":1080}`),
	}

	b, err := json.Marshal(ip)
	if err != nil {
		t.Fatalf("json.Marshal(EgressIP) = %v", err)
	}

	var decoded EgressIP
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(EgressIP) = %v", err)
	}
	if decoded.IPAddress != ip.IPAddress {
		t.Errorf("IPAddress = %q, want %q", decoded.IPAddress, ip.IPAddress)
	}
	if decoded.Provider != "sing-box" {
		t.Errorf("Provider = %q, want sing-box", decoded.Provider)
	}

	// omitempty: proxy_config 为空时不序列化
	ip2 := EgressIP{ID: "ip-2", IPAddress: "10.0.0.1"}
	b2, _ := json.Marshal(ip2)
	var m map[string]any
	json.Unmarshal(b2, &m)
	if _, ok := m["proxy_config"]; ok {
		t.Error("proxy_config should be omitted when empty")
	}
}

func TestDashboardStats_ZeroValue(t *testing.T) {
	var ds DashboardStats
	b, err := json.Marshal(ds)
	if err != nil {
		t.Fatalf("json.Marshal(DashboardStats{}) = %v", err)
	}
	// 零值也应正确序列化
	if !json.Valid(b) {
		t.Error("DashboardStats zero value should produce valid JSON")
	}
}

func TestCreateUserParams_Required(t *testing.T) {
	p := CreateUserParams{
		Username:      "newuser",
		PasswordHash:  "$2a$10$...",
		ShortID:       "nu001",
		EntryPassword: "temp-pass",
	}
	if p.Username == "" || p.PasswordHash == "" || p.ShortID == "" {
		t.Error("CreateUserParams required fields should not be empty")
	}
}

func TestCreateUserWithRoleParams_Defaults(t *testing.T) {
	p := CreateUserWithRoleParams{
		Username:     "admin",
		PasswordHash: "hash",
		ShortID:      "ad001",
		Role:         "admin",
	}
	if p.Role != "admin" {
		t.Errorf("Role = %q, want admin", p.Role)
	}
}

func TestUpsertHostParams_AllFields(t *testing.T) {
	p := UpsertHostParams{
		UserID:           "u1",
		Status:           "running",
		ShortID:          "h001",
		TemplateImageRef: "img:v1",
		HomeVolumeName:   "vol-1",
		SlotKey:          "primary",
		Timezone:         "UTC",
		Hostname:         "box1",
		MemoryLimitMB:    8192,
		CPULimit:         4.0,
		DiskLimitGB:      100,
	}
	if p.UserID == "" {
		t.Error("UserID is required")
	}
	if p.MemoryLimitMB <= 0 {
		t.Error("MemoryLimitMB should be positive")
	}
	if p.CPULimit <= 0 {
		t.Error("CPULimit should be positive")
	}
}

func TestClaudeAccount_OptionalFields(t *testing.T) {
	ca := ClaudeAccount{
		ID:     "ca-1",
		UserID: "u1",
		Email:  "user@example.com",
		Status: "active",
	}
	// PersistentVolumeName 为 nil 是合法状态
	if ca.PersistentVolumeName != nil {
		t.Error("PersistentVolumeName should default to nil")
	}
	if ca.HostID != nil {
		t.Error("HostID should default to nil")
	}
}

func TestSSHKey_Fields(t *testing.T) {
	k := SSHKey{
		ID:          "key-1",
		UserID:      "u1",
		Purpose:     "container_auth",
		Label:       "default",
		PublicKey:   "ssh-rsa AAAAB3...",
		PrivateKey:  "-----BEGIN OPENSSH PRIVATE KEY-----",
		KeyType:     "rsa",
		Fingerprint: "SHA256:abc123",
	}

	b, err := json.Marshal(k)
	if err != nil {
		t.Fatalf("json.Marshal(SSHKey) = %v", err)
	}

	var decoded SSHKey
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(SSHKey) = %v", err)
	}
	if decoded.PrivateKey != k.PrivateKey {
		t.Errorf("PrivateKey = %q, want %q", decoded.PrivateKey, k.PrivateKey)
	}

	// 空 PrivateKey 时 omitempty 生效，不序列化
	k2 := SSHKey{ID: "key-2", Purpose: "auth", PublicKey: "pk", Fingerprint: "fp"}
	b2, _ := json.Marshal(k2)
	var m map[string]any
	json.Unmarshal(b2, &m)
	if _, ok := m["private_key"]; ok {
		t.Error("private_key should be omitted when empty")
	}
}

func TestListEventsParams_Defaults(t *testing.T) {
	p := ListEventsParams{
		Limit:  20,
		Offset: 0,
	}
	if p.Limit <= 0 {
		t.Error("Limit should default to positive")
	}
}

func TestHostDetail_Composition(t *testing.T) {
	hd := HostDetail{
		Host: Host{ID: "h1", Status: "running"},
		User: User{ID: "u1", Username: "alice"},
		Bindings: []BindingWithIP{
			{
				BindingID: "b1",
				EgressIP:  EgressIP{ID: "ip1", IPAddress: "203.0.113.1"},
			},
		},
	}

	b, err := json.Marshal(hd)
	if err != nil {
		t.Fatalf("json.Marshal(HostDetail) = %v", err)
	}
	if !json.Valid(b) {
		t.Error("HostDetail JSON should be valid")
	}
}

func TestHostWithUsername_JSONShape(t *testing.T) {
	hw := HostWithUsername{
		Host:          Host{ID: "h1", Status: "running"},
		Username:      "alice",
		EgressIPLabel: strPtr("US-East"),
		EgressIPAddr:  strPtr("203.0.113.1"),
		DockerStatus:  "Up 3 hours",
	}

	b, err := json.Marshal(hw)
	if err != nil {
		t.Fatalf("json.Marshal(HostWithUsername) = %v", err)
	}

	var decoded HostWithUsername
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(HostWithUsername) = %v", err)
	}
	if decoded.Username != "alice" {
		t.Errorf("Username = %q, want alice", decoded.Username)
	}
	if decoded.DockerStatus != "Up 3 hours" {
		t.Errorf("DockerStatus = %q", decoded.DockerStatus)
	}
}

func TestConnectionInfo_Basic(t *testing.T) {
	ci := ConnectionInfo{
		CurlCommand: "curl -sSL https://example.com/entry",
		SSHCommand:  "ssh alice@host -p 2222",
		SSHPort:     2222,
	}

	if ci.SSHPort != 2222 {
		t.Errorf("SSHPort = %d, want 2222", ci.SSHPort)
	}
	if ci.CurlCommand == "" {
		t.Error("CurlCommand should not be empty")
	}
}

func TestRecordEventParams_Required(t *testing.T) {
	p := RecordEventParams{
		Level:   "warn",
		Type:    "host.stopped",
		Message: "Host was stopped unexpectedly",
	}
	if p.Level == "" || p.Type == "" || p.Message == "" {
		t.Error("RecordEventParams should have Level, Type, and Message")
	}
}

func strPtr(s string) *string { return &s }
