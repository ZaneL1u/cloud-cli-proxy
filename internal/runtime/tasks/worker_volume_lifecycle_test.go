package tasks

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestBuildClaudeStateVolumeName_NonEmptyID_ReturnsPrefixedName(t *testing.T) {
	got, err := BuildClaudeStateVolumeName("acct-42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "claude-state-acct-42" {
		t.Fatalf("want claude-state-acct-42, got %q", got)
	}
}

func TestBuildClaudeStateVolumeName_EmptyID_ReturnsError(t *testing.T) {
	_, err := BuildClaudeStateVolumeName("")
	if err == nil {
		t.Fatal("expected error for empty id")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Fatalf("error must mention 'required', got %q", err.Error())
	}
}

func TestEnsureDockerVolume_NotExists_RunsCreate(t *testing.T) {
	calls := 0
	gotArgs := [][]string{}
	orig := dockerVolumeRunner
	dockerVolumeRunner = func(ctx context.Context, args ...string) ([]byte, error) {
		calls++
		gotArgs = append(gotArgs, args)
		if calls == 1 {
			return []byte("Error: No such volume"), fmt.Errorf("exit 1")
		}
		return []byte("claude-state-abc\n"), nil
	}
	t.Cleanup(func() { dockerVolumeRunner = orig })

	if err := realEnsureDockerVolume(context.Background(), "claude-state-abc",
		map[string]string{"com.cloud-cli-proxy.account_id": "abc", "com.cloud-cli-proxy.managed": "true"}); err != nil {
		t.Fatalf("create flow should succeed: %v", err)
	}
	if calls != 2 {
		t.Errorf("want 2 docker calls (inspect+create), got %d (args=%v)", calls, gotArgs)
	}
	if gotArgs[0][0] != "inspect" {
		t.Errorf("first call must be inspect, got %v", gotArgs[0])
	}
	if gotArgs[1][0] != "create" {
		t.Errorf("second call must be create, got %v", gotArgs[1])
	}
}

func TestEnsureDockerVolume_AlreadyExists_SkipsCreate(t *testing.T) {
	calls := 0
	orig := dockerVolumeRunner
	dockerVolumeRunner = func(ctx context.Context, args ...string) ([]byte, error) {
		calls++
		return []byte("[{\"Name\":\"claude-state-abc\"}]"), nil
	}
	t.Cleanup(func() { dockerVolumeRunner = orig })
	if err := realEnsureDockerVolume(context.Background(), "claude-state-abc", map[string]string{}); err != nil {
		t.Fatalf("inspect-success path must be nil: %v", err)
	}
	if calls != 1 {
		t.Errorf("want 1 docker call (inspect only), got %d", calls)
	}
}

func TestRemoveDockerVolume_NotFound_IsSuccess(t *testing.T) {
	orig := dockerVolumeRunner
	dockerVolumeRunner = func(ctx context.Context, args ...string) ([]byte, error) {
		return []byte("Error response from daemon: get claude-state-abc: no such volume"), fmt.Errorf("exit 1")
	}
	t.Cleanup(func() { dockerVolumeRunner = orig })
	if err := realRemoveDockerVolume(context.Background(), "claude-state-abc", false); err != nil {
		t.Errorf("not-found must be treated as success, got: %v", err)
	}
}

func TestRemoveDockerVolume_InUse_PropagatesVolumeInUseError(t *testing.T) {
	orig := dockerVolumeRunner
	dockerVolumeRunner = func(ctx context.Context, args ...string) ([]byte, error) {
		return []byte("Error response from daemon: remove claude-state-abc: volume is in use - [container_xyz]"), fmt.Errorf("exit 1")
	}
	t.Cleanup(func() { dockerVolumeRunner = orig })
	err := realRemoveDockerVolume(context.Background(), "claude-state-abc", false)
	if err == nil {
		t.Fatal("in-use must produce an error")
	}
	if !strings.HasPrefix(err.Error(), "volume_in_use:") {
		t.Errorf("error must start with volume_in_use:, got: %v", err)
	}
}

func TestRemoveDockerVolume_ForceTrue_PassesDashF(t *testing.T) {
	var captured []string
	orig := dockerVolumeRunner
	dockerVolumeRunner = func(ctx context.Context, args ...string) ([]byte, error) {
		captured = args
		return []byte("claude-state-abc"), nil
	}
	t.Cleanup(func() { dockerVolumeRunner = orig })
	if err := realRemoveDockerVolume(context.Background(), "claude-state-abc", true); err != nil {
		t.Fatalf("force=true success path: %v", err)
	}
	if len(captured) < 2 || captured[0] != "rm" || captured[1] != "-f" {
		t.Errorf("force=true must pass [rm -f ...], got %v", captured)
	}
}
