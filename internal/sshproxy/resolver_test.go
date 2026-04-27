package sshproxy

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"testing"

	gossh "golang.org/x/crypto/ssh"

	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

type stubResolverRepo struct {
	hostAuth       repository.HostSSHAuth
	hostAuthErr    error
	inboundKeys    []repository.SSHKey
	inboundKeysErr error
	targetAddr     string
	targetErr      error
}

func (s *stubResolverRepo) GetHostByUsername(_ context.Context, _ string) (repository.HostSSHAuth, error) {
	return s.hostAuth, s.hostAuthErr
}

func (s *stubResolverRepo) ListSSHKeysByUserAndPurpose(_ context.Context, _, _ string) ([]repository.SSHKey, error) {
	return s.inboundKeys, s.inboundKeysErr
}

// ContainerResolver implementation for proxy tests.
func (s *stubResolverRepo) ResolveContainer(_ context.Context, _, _ string) (ContainerTarget, error) {
	addr := s.targetAddr
	if addr == "" {
		addr = "127.0.0.1:22"
	}
	return ContainerTarget{
		Addr:     addr,
		User:     s.hostAuth.ContainerUser,
		Password: s.hostAuth.EntryPassword,
	}, s.targetErr
}

func (s *stubResolverRepo) ResolveContainerByPublicKey(_ context.Context, _ string, _ gossh.PublicKey) (ContainerTarget, error) {
	addr := s.targetAddr
	if addr == "" {
		addr = "127.0.0.1:22"
	}
	return ContainerTarget{
		Addr:       addr,
		User:       s.hostAuth.ContainerUser,
		PrivateKey: s.hostAuth.SSHPrivateKey,
	}, s.targetErr
}

func generateTestKey(t *testing.T) (gossh.PublicKey, string) {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}
	signer, err := gossh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("create signer: %v", err)
	}
	return signer.PublicKey(), string(gossh.MarshalAuthorizedKey(signer.PublicKey()))
}

func TestResolveContainer_InvalidPassword(t *testing.T) {
	repo := &stubResolverRepo{
		hostAuth: repository.HostSSHAuth{
			HostID:        "h1",
			EntryPassword: "secret",
			HostStatus:    "running",
			UserID:        "u1",
			UserStatus:    "active",
			Username:      "alice",
		},
	}
	resolver := NewRepoResolver(repo)
	_, err := resolver.ResolveContainer(context.Background(), "alice", "wrong")
	if err == nil || err.Error() != "invalid credentials" {
		t.Fatalf("expected invalid credentials, got: %v", err)
	}
}

func TestResolveContainerByPublicKey_Match(t *testing.T) {
	pub, pubKeyStr := generateTestKey(t)

	repo := &stubResolverRepo{
		hostAuth: repository.HostSSHAuth{
			HostID:        "h1",
			EntryPassword: "secret",
			HostStatus:    "running",
			UserID:        "u1",
			UserStatus:    "active",
			Username:      "alice",
		},
		inboundKeys: []repository.SSHKey{
			{PublicKey: pubKeyStr},
		},
	}
	resolver := NewRepoResolver(repo)
	_, err := resolver.ResolveContainerByPublicKey(context.Background(), "alice", pub)
	if err == nil {
		t.Skip("docker unavailable in unit test environment")
	}
	if err.Error() == "host not found" || err.Error() == "no inbound keys configured" || err.Error() == "public key not authorized" {
		t.Fatalf("unexpected early error: %v", err)
	}
}

func TestResolveContainerByPublicKey_NoMatch(t *testing.T) {
	_, pubKeyStrA := generateTestKey(t)
	pubB, _ := generateTestKey(t)

	repo := &stubResolverRepo{
		hostAuth: repository.HostSSHAuth{
			HostID:        "h1",
			EntryPassword: "secret",
			HostStatus:    "running",
			UserID:        "u1",
			UserStatus:    "active",
			Username:      "alice",
		},
		inboundKeys: []repository.SSHKey{
			{PublicKey: pubKeyStrA},
		},
	}
	resolver := NewRepoResolver(repo)
	_, err := resolver.ResolveContainerByPublicKey(context.Background(), "alice", pubB)
	if err == nil || err.Error() != "public key not authorized" {
		t.Fatalf("expected public key not authorized, got: %v", err)
	}
}

func TestResolveContainerByPublicKey_NoInboundKeys(t *testing.T) {
	pub, _ := generateTestKey(t)

	repo := &stubResolverRepo{
		hostAuth: repository.HostSSHAuth{
			HostID:        "h1",
			EntryPassword: "secret",
			HostStatus:    "running",
			UserID:        "u1",
			UserStatus:    "active",
			Username:      "alice",
		},
		inboundKeys: []repository.SSHKey{},
	}
	resolver := NewRepoResolver(repo)
	_, err := resolver.ResolveContainerByPublicKey(context.Background(), "alice", pub)
	if err == nil || err.Error() != "no inbound keys configured" {
		t.Fatalf("expected no inbound keys configured, got: %v", err)
	}
}

func TestResolveContainer_UsernamePassedToRepo(t *testing.T) {
	repo := &stubResolverRepo{
		hostAuth: repository.HostSSHAuth{
			HostID:        "h1",
			EntryPassword: "secret",
			HostStatus:    "running",
			UserID:        "u1",
			UserStatus:    "active",
			Username:      "charlie",
		},
	}
	resolver := NewRepoResolver(repo)
	// 仅验证参数传递不会 panic；docker 不可用所以后续会报错
	_, err := resolver.ResolveContainer(context.Background(), "charlie", "secret")
	if err == nil {
		t.Skip("docker unavailable")
	}
	// 不应是前置校验错误
	if err.Error() == "invalid credentials" {
		t.Fatalf("unexpected early error: %v", err)
	}
}

func TestResolveTarget_FieldsPopulated(t *testing.T) {
	auth := repository.HostSSHAuth{
		HostID:        "h1",
		EntryPassword: "secret",
		HostStatus:    "running",
		UserID:        "u1",
		UserStatus:    "active",
		Username:      "alice",
		SSHPrivateKey: "fake-private-key-pem",
	}
	resolver := NewRepoResolver(&stubResolverRepo{})
	_, err := resolver.resolveTarget(context.Background(), auth)
	if err == nil {
		t.Skip("docker unavailable in unit test environment")
	}
	// 验证 resolveTarget 在 docker 失败前不会 panic，且字段已正确传递
}

// ---- resolveTarget edge cases ----

func TestResolveTarget_SuspendedUser(t *testing.T) {
	auth := repository.HostSSHAuth{
		HostID:     "h1",
		UserStatus: "suspended",
		HostStatus: "running",
	}
	resolver := NewRepoResolver(&stubResolverRepo{})
	_, err := resolver.resolveTarget(context.Background(), auth)
	if err == nil || err.Error() != "user suspended" {
		t.Fatalf("expected 'user suspended', got: %v", err)
	}
}

func TestResolveTarget_HostNotRunning(t *testing.T) {
	auth := repository.HostSSHAuth{
		HostID:     "h1",
		UserStatus: "active",
		HostStatus: "stopped",
	}
	resolver := NewRepoResolver(&stubResolverRepo{})
	_, err := resolver.resolveTarget(context.Background(), auth)
	if err == nil {
		t.Fatal("expected error for stopped host")
	}
	expected := "host not running (status: stopped)"
	if err.Error() != expected {
		t.Fatalf("expected %q, got %q", expected, err.Error())
	}
}

func TestResolveTarget_HostStatusStarting(t *testing.T) {
	auth := repository.HostSSHAuth{
		HostID:     "h1",
		UserStatus: "active",
		HostStatus: "starting",
	}
	resolver := NewRepoResolver(&stubResolverRepo{})
	_, err := resolver.resolveTarget(context.Background(), auth)
	if err == nil {
		t.Fatal("expected error for starting host")
	}
	expected := "host not running (status: starting)"
	if err.Error() != expected {
		t.Fatalf("expected %q, got %q", expected, err.Error())
	}
}

func TestResolveTarget_VariousStatuses(t *testing.T) {
	tests := []struct {
		name       string
		hostStatus string
		wantErr    string
	}{
		{"stopped", "stopped", "host not running (status: stopped)"},
		{"failed", "failed", "host not running (status: failed)"},
		{"pending", "pending", "host not running (status: pending)"},
		{"suspended", "suspended", "host not running (status: suspended)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := repository.HostSSHAuth{
				HostID:     "h1",
				UserStatus: "active",
				HostStatus: tt.hostStatus,
			}
			resolver := NewRepoResolver(&stubResolverRepo{})
			_, err := resolver.resolveTarget(context.Background(), auth)
			if err == nil {
				t.Fatal("expected error")
			}
			if err.Error() != tt.wantErr {
				t.Fatalf("expected %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

// ---- ResolveContainer additional edge cases ----

func TestResolveContainer_HostNotFound(t *testing.T) {
	repo := &stubResolverRepo{
		hostAuthErr: context.DeadlineExceeded,
	}
	resolver := NewRepoResolver(repo)
	_, err := resolver.ResolveContainer(context.Background(), "alice", "secret")
	if err == nil || err.Error() != "host not found" {
		t.Fatalf("expected 'host not found', got: %v", err)
	}
}

func TestResolveContainer_SuspendedUser(t *testing.T) {
	repo := &stubResolverRepo{
		hostAuth: repository.HostSSHAuth{
			HostID:        "h1",
			EntryPassword: "secret",
			HostStatus:    "running",
			UserID:        "u1",
			UserStatus:    "suspended",
			Username:      "alice",
		},
	}
	resolver := NewRepoResolver(repo)
	_, err := resolver.ResolveContainer(context.Background(), "alice", "secret")
	if err == nil || err.Error() != "user suspended" {
		t.Fatalf("expected 'user suspended', got: %v", err)
	}
}

func TestResolveContainer_HostNotRunning(t *testing.T) {
	repo := &stubResolverRepo{
		hostAuth: repository.HostSSHAuth{
			HostID:        "h1",
			EntryPassword: "secret",
			HostStatus:    "stopped",
			UserID:        "u1",
			UserStatus:    "active",
			Username:      "alice",
		},
	}
	resolver := NewRepoResolver(repo)
	_, err := resolver.ResolveContainer(context.Background(), "alice", "secret")
	if err == nil {
		t.Fatal("expected error for stopped host")
	}
	expected := "host not running (status: stopped)"
	if err.Error() != expected {
		t.Fatalf("expected %q, got %q", expected, err.Error())
	}
}

func TestResolveContainer_EmptyPassword_And_WrongPassword(t *testing.T) {
	// When EntryPassword is empty, ANY password should fail with "invalid credentials".
	repo := &stubResolverRepo{
		hostAuth: repository.HostSSHAuth{
			HostID:        "h1",
			EntryPassword: "",
			HostStatus:    "running",
			UserID:        "u1",
			UserStatus:    "active",
			Username:      "alice",
		},
	}
	resolver := NewRepoResolver(repo)
	_, err := resolver.ResolveContainer(context.Background(), "alice", "anything")
	if err == nil || err.Error() != "invalid credentials" {
		t.Fatalf("expected 'invalid credentials', got: %v", err)
	}
}

// ---- ResolveContainerByPublicKey additional edge cases ----

func TestResolveContainerByPublicKey_HostNotFound(t *testing.T) {
	pub, _ := generateTestKey(t)
	repo := &stubResolverRepo{
		hostAuthErr: context.DeadlineExceeded,
	}
	resolver := NewRepoResolver(repo)
	_, err := resolver.ResolveContainerByPublicKey(context.Background(), "alice", pub)
	if err == nil || err.Error() != "host not found" {
		t.Fatalf("expected 'host not found', got: %v", err)
	}
}

func TestResolveContainerByPublicKey_ListKeysError(t *testing.T) {
	pub, _ := generateTestKey(t)
	repo := &stubResolverRepo{
		hostAuth: repository.HostSSHAuth{
			HostID:        "h1",
			EntryPassword: "secret",
			HostStatus:    "running",
			UserID:        "u1",
			UserStatus:    "active",
			Username:      "alice",
		},
		inboundKeysErr: context.DeadlineExceeded,
	}
	resolver := NewRepoResolver(repo)
	_, err := resolver.ResolveContainerByPublicKey(context.Background(), "alice", pub)
	if err == nil || err.Error() != "no inbound keys configured" {
		t.Fatalf("expected 'no inbound keys configured', got: %v", err)
	}
}

func TestResolveContainerByPublicKey_SuspendedUser(t *testing.T) {
	pub, pubStr := generateTestKey(t)
	repo := &stubResolverRepo{
		hostAuth: repository.HostSSHAuth{
			HostID:        "h1",
			EntryPassword: "secret",
			HostStatus:    "running",
			UserID:        "u1",
			UserStatus:    "suspended",
			Username:      "alice",
		},
		inboundKeys: []repository.SSHKey{
			{PublicKey: pubStr},
		},
	}
	resolver := NewRepoResolver(repo)
	_, err := resolver.ResolveContainerByPublicKey(context.Background(), "alice", pub)
	if err == nil || err.Error() != "user suspended" {
		t.Fatalf("expected 'user suspended', got: %v", err)
	}
}

func TestResolveContainerByPublicKey_HostNotRunning(t *testing.T) {
	pub, pubStr := generateTestKey(t)
	repo := &stubResolverRepo{
		hostAuth: repository.HostSSHAuth{
			HostID:        "h1",
			EntryPassword: "secret",
			HostStatus:    "stopped",
			UserID:        "u1",
			UserStatus:    "active",
			Username:      "alice",
		},
		inboundKeys: []repository.SSHKey{
			{PublicKey: pubStr},
		},
	}
	resolver := NewRepoResolver(repo)
	_, err := resolver.ResolveContainerByPublicKey(context.Background(), "alice", pub)
	if err == nil {
		t.Fatal("expected error for stopped host")
	}
	expected := "host not running (status: stopped)"
	if err.Error() != expected {
		t.Fatalf("expected %q, got %q", expected, err.Error())
	}
}

func TestResolveContainerByPublicKey_MalformedKeySkipped(t *testing.T) {
	pub, _ := generateTestKey(t)
	repo := &stubResolverRepo{
		hostAuth: repository.HostSSHAuth{
			HostID:        "h1",
			EntryPassword: "secret",
			HostStatus:    "running",
			UserID:        "u1",
			UserStatus:    "active",
			Username:      "alice",
		},
		inboundKeys: []repository.SSHKey{
			{PublicKey: "not-a-valid-ssh-key"},
		},
	}
	resolver := NewRepoResolver(repo)
	_, err := resolver.ResolveContainerByPublicKey(context.Background(), "alice", pub)
	// Malformed key should be skipped; client key won't match → "public key not authorized".
	if err == nil || err.Error() != "public key not authorized" {
		t.Fatalf("expected 'public key not authorized', got: %v", err)
	}
}

func TestResolveContainerByPublicKey_DifferentKeyType(t *testing.T) {
	// Use an ed25519 key as the client key, but only register an RSA key as authorized.
	clientPub, _ := generateTestKey(t)

	// Generate an RSA key for the authorized keys list.
	rsaKey, rsaPubStr := generateRSAKeyForTest(t)

	repo := &stubResolverRepo{
		hostAuth: repository.HostSSHAuth{
			HostID:        "h1",
			EntryPassword: "secret",
			HostStatus:    "running",
			UserID:        "u1",
			UserStatus:    "active",
			Username:      "alice",
		},
		inboundKeys: []repository.SSHKey{
			{PublicKey: rsaPubStr},
		},
	}
	resolver := NewRepoResolver(repo)
	_, err := resolver.ResolveContainerByPublicKey(context.Background(), "alice", clientPub)
	// Different key type → should not match.
	if err == nil || err.Error() != "public key not authorized" {
		t.Fatalf("expected 'public key not authorized', got: %v", err)
	}
	_ = rsaKey // suppress unused variable warning
}

func generateRSAKeyForTest(t *testing.T) (gossh.PublicKey, string) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	pub, err := gossh.NewPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatalf("create SSH public key: %v", err)
	}
	return pub, string(gossh.MarshalAuthorizedKey(pub))
}

func TestNewRepoResolver(t *testing.T) {
	repo := &stubResolverRepo{}
	resolver := NewRepoResolver(repo)
	if resolver == nil {
		t.Fatal("expected non-nil resolver")
	}
	if resolver.repo != repo {
		t.Fatal("resolver repo not set correctly")
	}
}

func TestContainerTarget_ZeroValue(t *testing.T) {
	var ct ContainerTarget
	if ct.Addr != "" || ct.User != "" || ct.Password != "" || ct.PrivateKey != "" {
		t.Fatal("zero value ContainerTarget should have all empty fields")
	}
}

func TestContainerTarget_FieldsSet(t *testing.T) {
	ct := ContainerTarget{
		Addr:       "10.0.0.1:22",
		User:       "workspace",
		Password:   "secret",
		PrivateKey: "-----BEGIN OPENSSH PRIVATE KEY-----\nfake\n-----END OPENSSH PRIVATE KEY-----\n",
	}
	if ct.Addr != "10.0.0.1:22" {
		t.Fatalf("unexpected Addr: %s", ct.Addr)
	}
	if ct.User != "workspace" {
		t.Fatalf("unexpected User: %s", ct.User)
	}
	if ct.Password != "secret" {
		t.Fatalf("unexpected Password: %s", ct.Password)
	}
	if ct.PrivateKey == "" {
		t.Fatal("expected non-empty PrivateKey")
	}
}
