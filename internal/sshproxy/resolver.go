package sshproxy

import (
	"context"
	"crypto/subtle"
	"fmt"
	"os/exec"
	"strings"

	gossh "golang.org/x/crypto/ssh"

	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

type resolverRepo interface {
	GetHostByShortID(ctx context.Context, shortID string) (repository.HostSSHAuth, error)
	ListSSHKeysByUserAndPurpose(ctx context.Context, userID, purpose string) ([]repository.SSHKey, error)
}

type ContainerTarget struct {
	Addr     string
	User     string
	Password string
}

type RepoResolver struct {
	repo resolverRepo
}

func NewRepoResolver(repo resolverRepo) *RepoResolver {
	return &RepoResolver{repo: repo}
}

func (r *RepoResolver) ResolveContainer(ctx context.Context, hostShortID, password string) (ContainerTarget, error) {
	auth, err := r.repo.GetHostByShortID(ctx, hostShortID)
	if err != nil {
		return ContainerTarget{}, fmt.Errorf("host not found")
	}

	if auth.EntryPassword == "" || subtle.ConstantTimeCompare([]byte(auth.EntryPassword), []byte(password)) != 1 {
		return ContainerTarget{}, fmt.Errorf("invalid credentials")
	}

	return r.resolveTarget(ctx, auth)
}

func (r *RepoResolver) ResolveContainerByPublicKey(ctx context.Context, hostShortID string, clientKey gossh.PublicKey) (ContainerTarget, error) {
	auth, err := r.repo.GetHostByShortID(ctx, hostShortID)
	if err != nil {
		return ContainerTarget{}, fmt.Errorf("host not found")
	}

	inboundKeys, err := r.repo.ListSSHKeysByUserAndPurpose(ctx, auth.UserID, "inbound")
	if err != nil || len(inboundKeys) == 0 {
		return ContainerTarget{}, fmt.Errorf("no inbound keys configured")
	}

	clientKeyData := clientKey.Marshal()
	matched := false
	for _, k := range inboundKeys {
		parsed, _, _, _, err := gossh.ParseAuthorizedKey([]byte(k.PublicKey))
		if err != nil {
			continue
		}
		if clientKey.Type() == parsed.Type() && subtle.ConstantTimeCompare(clientKeyData, parsed.Marshal()) == 1 {
			matched = true
			break
		}
	}
	if !matched {
		return ContainerTarget{}, fmt.Errorf("public key not authorized")
	}

	return r.resolveTarget(ctx, auth)
}

func (r *RepoResolver) resolveTarget(ctx context.Context, auth repository.HostSSHAuth) (ContainerTarget, error) {
	if auth.UserStatus != "active" {
		return ContainerTarget{}, fmt.Errorf("user suspended")
	}
	if auth.HostStatus != "running" {
		return ContainerTarget{}, fmt.Errorf("host not running (status: %s)", auth.HostStatus)
	}

	containerName := fmt.Sprintf("cloudproxy-%s", auth.HostID)
	containerIP, err := getContainerIP(ctx, containerName)
	if err != nil {
		return ContainerTarget{}, fmt.Errorf("cannot resolve container address: %w", err)
	}

	user := auth.Username
	if user == "" {
		user = "workspace"
	}

	return ContainerTarget{
		Addr:     fmt.Sprintf("%s:22", containerIP),
		User:     user,
		Password: auth.EntryPassword,
	}, nil
}

func getContainerIP(ctx context.Context, containerName string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", "inspect", "-f",
		"{{range .NetworkSettings.Networks}}{{.IPAddress}} {{end}}", containerName)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("docker inspect: %w", err)
	}
	ips := strings.Fields(strings.TrimSpace(string(out)))
	if len(ips) == 0 {
		return "", fmt.Errorf("no IP found for container %s", containerName)
	}
	return ips[len(ips)-1], nil
}
