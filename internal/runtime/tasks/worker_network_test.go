package tasks

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestConnectContainerNetworks_RelocksDNSAfterNetworkConnect(t *testing.T) {
	t.Setenv("COMPOSE_NETWORK", "test-compose-net")

	var dockerCalls [][]string
	prevDockerRunner := dockerRunner
	dockerRunner = func(_ context.Context, args ...string) ([]byte, error) {
		dockerCalls = append(dockerCalls, append([]string{}, args...))
		return nil, nil
	}
	t.Cleanup(func() { dockerRunner = prevDockerRunner })

	fc := newFakeContainer()
	prevExec := execInContainer
	execInContainer = fc.runner
	t.Cleanup(func() { execInContainer = prevExec })

	w := &Worker{}
	if err := w.connectContainerNetworks(context.Background(), "cloudproxy-h1"); err != nil {
		t.Fatalf("connectContainerNetworks: %v", err)
	}

	wantDockerCall := []string{"network", "connect", "test-compose-net", "cloudproxy-h1"}
	if len(dockerCalls) != 1 || !reflect.DeepEqual(dockerCalls[0], wantDockerCall) {
		t.Fatalf("docker calls = %#v, want %#v", dockerCalls, [][]string{wantDockerCall})
	}

	got := fc.files["/etc/resolv.conf"]
	if !strings.Contains(got, "nameserver 127.0.0.1") {
		t.Fatalf("resolv.conf not re-locked to sing-box DNS: %q", got)
	}
	if strings.Contains(got, "127.0.0.11") {
		t.Fatalf("resolv.conf still points at Docker embedded DNS: %q", got)
	}
}

func TestConnectContainerNetworks_SyncsSingBoxInterfaceAfterNetworkConnect(t *testing.T) {
	t.Setenv("COMPOSE_NETWORK", "test-compose-net")

	prevDockerRunner := dockerRunner
	dockerRunner = func(_ context.Context, args ...string) ([]byte, error) {
		return nil, nil
	}
	t.Cleanup(func() { dockerRunner = prevDockerRunner })

	fc := newFakeContainer()
	prevExec := execInContainer
	execInContainer = fc.runner
	t.Cleanup(func() { execInContainer = prevExec })

	w := &Worker{}
	if err := w.connectContainerNetworks(context.Background(), "cloudproxy-h1"); err != nil {
		t.Fatalf("connectContainerNetworks: %v", err)
	}

	found := false
	for _, script := range fc.log {
		if strings.Contains(script, "cloud-cli-proxy-sync-singbox-interface") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected sing-box egress interface sync after network connect, scripts=%#v", fc.log)
	}
}

func TestConnectContainerNetworks_ReturnsDNSRelockError(t *testing.T) {
	prevDockerRunner := dockerRunner
	dockerRunner = func(_ context.Context, args ...string) ([]byte, error) {
		return nil, nil
	}
	t.Cleanup(func() { dockerRunner = prevDockerRunner })

	prevExec := execInContainer
	execInContainer = func(_ context.Context, _, _, _ string) ([]byte, error) {
		return []byte("read-only file system"), errors.New("exit 1")
	}
	t.Cleanup(func() { execInContainer = prevExec })

	w := &Worker{}
	err := w.connectContainerNetworks(context.Background(), "cloudproxy-h1")
	if err == nil {
		t.Fatal("expected DNS re-lock error, got nil")
	}
	if !strings.Contains(err.Error(), "re-lock container DNS after network connect") {
		t.Fatalf("error should mention DNS re-lock context, got: %v", err)
	}
}
