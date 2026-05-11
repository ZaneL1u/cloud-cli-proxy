package http

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// TestResolveProbeNetworking_HostnameEmpty 验证 hostname 为空时落到 host-binary 分支。
func TestResolveProbeNetworking_HostnameEmpty(t *testing.T) {
	prevHostname := hostnameProvider
	prevInspect := dockerInspectRunner
	t.Cleanup(func() {
		hostnameProvider = prevHostname
		dockerInspectRunner = prevInspect
	})

	hostnameProvider = func() (string, error) { return "", nil }
	dockerInspectRunner = func(ctx context.Context, name string) ([]byte, error) {
		t.Fatalf("dockerInspectRunner 不应在 hostname 为空时被调用")
		return nil, nil
	}

	args := resolveProbeNetworking(context.Background(), 12345)
	want := []string{"--network", "bridge", "-p", "127.0.0.1:12345:12345"}
	if !equalStrSlice(args, want) {
		t.Fatalf("args mismatch:\n  got=%v\n want=%v", args, want)
	}
}

// TestResolveProbeNetworking_InspectFails 验证 docker inspect 失败时 fallback 到 host-binary。
func TestResolveProbeNetworking_InspectFails(t *testing.T) {
	prevHostname := hostnameProvider
	prevInspect := dockerInspectRunner
	t.Cleanup(func() {
		hostnameProvider = prevHostname
		dockerInspectRunner = prevInspect
	})

	hostnameProvider = func() (string, error) { return "Vision-2.local", nil }
	dockerInspectRunner = func(ctx context.Context, name string) ([]byte, error) {
		return nil, errors.New("Error: No such object: Vision-2.local")
	}

	args := resolveProbeNetworking(context.Background(), 23456)
	want := []string{"--network", "bridge", "-p", "127.0.0.1:23456:23456"}
	if !equalStrSlice(args, want) {
		t.Fatalf("args mismatch:\n  got=%v\n want=%v", args, want)
	}
}

// TestResolveProbeNetworking_InspectEmptyOutput 验证 docker inspect 返回空 stdout 时也走 host-binary。
func TestResolveProbeNetworking_InspectEmptyOutput(t *testing.T) {
	prevHostname := hostnameProvider
	prevInspect := dockerInspectRunner
	t.Cleanup(func() {
		hostnameProvider = prevHostname
		dockerInspectRunner = prevInspect
	})

	hostnameProvider = func() (string, error) { return "Vision-2.local", nil }
	dockerInspectRunner = func(ctx context.Context, name string) ([]byte, error) {
		return []byte("   \n\t\n"), nil
	}

	args := resolveProbeNetworking(context.Background(), 34567)
	want := []string{"--network", "bridge", "-p", "127.0.0.1:34567:34567"}
	if !equalStrSlice(args, want) {
		t.Fatalf("args mismatch:\n  got=%v\n want=%v", args, want)
	}
}

// TestResolveProbeNetworking_InspectSucceeds 验证 hostname 是合法容器时走 in-container 分支。
func TestResolveProbeNetworking_InspectSucceeds(t *testing.T) {
	prevHostname := hostnameProvider
	prevInspect := dockerInspectRunner
	t.Cleanup(func() {
		hostnameProvider = prevHostname
		dockerInspectRunner = prevInspect
	})

	const fakeID = "fake-id"
	hostnameProvider = func() (string, error) { return fakeID, nil }
	dockerInspectRunner = func(ctx context.Context, name string) ([]byte, error) {
		if name != fakeID {
			return nil, fmt.Errorf("expected name=%q, got %q", fakeID, name)
		}
		return []byte("abc123def456\n"), nil
	}

	args := resolveProbeNetworking(context.Background(), 45678)
	want := []string{"--network", "container:" + fakeID}
	if !equalStrSlice(args, want) {
		t.Fatalf("args mismatch:\n  got=%v\n want=%v", args, want)
	}

	// 同时确认输出里没有 -p 端口暴露（host-binary 专属标记）
	for _, a := range args {
		if strings.HasPrefix(a, "127.0.0.1:") {
			t.Fatalf("in-container 分支不应包含端口暴露: %v", args)
		}
	}
}

func equalStrSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
