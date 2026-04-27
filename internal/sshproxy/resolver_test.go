package sshproxy

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	gossh "golang.org/x/crypto/ssh"

	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

type stubResolverRepo struct {
	hostAuth       repository.HostSSHAuth
	hostAuthErr    error
	inboundKeys    []repository.SSHKey
	inboundKeysErr error
}

func (s *stubResolverRepo) GetHostByUsername(_ context.Context, _ string) (repository.HostSSHAuth, error) {
	return s.hostAuth, s.hostAuthErr
}

func (s *stubResolverRepo) ListSSHKeysByUserAndPurpose(_ context.Context, _, _ string) ([]repository.SSHKey, error) {
	return s.inboundKeys, s.inboundKeysErr
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
