package sshproxy

import (
	"context"
	"crypto/subtle"
	"fmt"
	"os/exec"
	"strings"

	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

type resolverRepo interface {
	GetHostByShortID(ctx context.Context, shortID string) (repository.HostSSHAuth, error)
}

// ContainerTarget is the SSH endpoint and credentials for dialing the user container.
type ContainerTarget struct {
	Addr     string
	User     string
	Password string
}

// RepoResolver implements ContainerResolver using the database repository.
// It validates the host's entry_password, checks that the owning user is
// active and the host is running, then resolves the container's SSH address.
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
