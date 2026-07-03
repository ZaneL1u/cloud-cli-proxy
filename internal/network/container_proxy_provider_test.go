package network

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWorkerContainerName(t *testing.T) {
	got := workerContainerName("my-host")
	want := "cloudproxy-my-host"
	if got != want {
		t.Errorf("workerContainerName = %q, want %q", got, want)
	}
}

type countingVerifier struct {
	calls int
	err   error
}

func (v *countingVerifier) Verify(ctx context.Context, containerName string, egress EgressConfig) (VerifyResult, error) {
	v.calls++
	return VerifyResult{ActualEgressIP: "203.0.113.10"}, v.err
}

func TestContainerProxyProvider_PrepareHostSkipsCreateTimeEgressVerification(t *testing.T) {
	verifier := &countingVerifier{err: errors.New("egress probe should not run during create")}
	p := NewContainerProxyProvider(slog.Default(), verifier)

	err := p.PrepareHost(context.Background(), HostNetworkSpec{
		HostID: "h-no-create-probe",
		Egress: &EgressConfig{
			EgressIPID: "eip", ExpectedIP: "9.9.9.9", TunnelType: TunnelTypeProxy,
			Proxy: &ProxySpec{
				OutboundConfig: json.RawMessage(`{"type":"socks","server":"1.2.3.4","server_port":1080}`),
				DNSServer:      "1.1.1.1",
			},
		},
	})
	if err != nil {
		t.Fatalf("PrepareHost should allow host creation without egress IP probe, got: %v", err)
	}
	if verifier.calls != 0 {
		t.Fatalf("PrepareHost must not run create-time verifier, calls=%d", verifier.calls)
	}
}

// TestSingBoxConfigDir_DefaultBase 锁 Phase 54-01 D-54-9 路径契约：
// 不设 DATA_DIR 时落 /var/lib/cloud-cli-proxy/gateway/<host_id>。
// 路径名 "gateway" 是 v3.x 历史遗留（D-54-10），v4.0 保留以避免跨包改动。
func TestSingBoxConfigDir_DefaultBase(t *testing.T) {
	if err := os.Unsetenv("DATA_DIR"); err != nil {
		t.Fatalf("unset DATA_DIR: %v", err)
	}
	got := SingBoxConfigDir("abc")
	want := "/var/lib/cloud-cli-proxy/gateway/abc"
	if got != want {
		t.Errorf("SingBoxConfigDir(\"abc\") = %q, want %q", got, want)
	}
}

// TestSingBoxConfigDir_RespectsDATA_DIR 守护 SingBoxConfigDir 读 DATA_DIR
// 环境变量：DATA_DIR 设置后必须把 base 替换为该值。
func TestSingBoxConfigDir_RespectsDATA_DIR(t *testing.T) {
	t.Setenv("DATA_DIR", "/tmp/xyz")
	got := SingBoxConfigDir("abc")
	want := filepath.Join("/tmp/xyz", "gateway", "abc")
	if got != want {
		t.Errorf("SingBoxConfigDir(\"abc\") with DATA_DIR=/tmp/xyz = %q, want %q", got, want)
	}
}

// TestGatewayConfigDir_AliasMatchesSingBoxConfigDir 锁 D-54-9 alias 等价性：
// v4.0 兼容窗口内 GatewayConfigDir 必须与 SingBoxConfigDir 返回完全相同路径，
// 任意 hostID 都不能漂移；v4.1 删除 alias 时该测试同步移除。
func TestGatewayConfigDir_AliasMatchesSingBoxConfigDir(t *testing.T) {
	if err := os.Unsetenv("DATA_DIR"); err != nil {
		t.Fatalf("unset DATA_DIR: %v", err)
	}
	for _, hostID := range []string{"abc", "host-1", "host with space", "550e8400-e29b-41d4-a716-446655440000"} {
		if got, want := GatewayConfigDir(hostID), SingBoxConfigDir(hostID); got != want {
			t.Errorf("GatewayConfigDir(%q) = %q, want SingBoxConfigDir = %q", hostID, got, want)
		}
	}
}

func TestGatewayConfigDir_PathSanitization(t *testing.T) {
	if err := os.Unsetenv("DATA_DIR"); err != nil {
		t.Fatalf("unset DATA_DIR: %v", err)
	}
	tests := []struct {
		hostID string
		want   string
	}{
		{"host-1", "/var/lib/cloud-cli-proxy/gateway/host-1"},
		{"host/with/slash", "/var/lib/cloud-cli-proxy/gateway/host/with/slash"},
		{"host..dot", "/var/lib/cloud-cli-proxy/gateway/host..dot"},
		{"host space", "/var/lib/cloud-cli-proxy/gateway/host space"},
	}
	for _, tt := range tests {
		t.Run(tt.hostID, func(t *testing.T) {
			got := GatewayConfigDir(tt.hostID)
			if got != tt.want {
				t.Errorf("GatewayConfigDir(%q) = %q, want %q", tt.hostID, got, tt.want)
			}
		})
	}
}

// Test_CleanupHost_RemovesConfigDir 守护 CleanupHost 单一职责：
// 给定 SingBoxConfigDir 存在 + 含 config.json，CleanupHost 调用后整个目录
// 必须被 os.RemoveAll 干净移除（IsNotExist）。这是 host stop / failure 路径
// 上唯一的 host 端清理动作，覆盖 v4.0 单容器架构下不再有 gateway 容器 /
// bridge 网络要清理的现实。
func Test_CleanupHost_RemovesConfigDir(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())

	hostID := "cleanup-host-1"
	dir := SingBoxConfigDir(hostID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir SingBoxConfigDir: %v", err)
	}
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{"stub":true}`), 0o640); err != nil {
		t.Fatalf("write stub config.json: %v", err)
	}

	p := NewContainerProxyProvider(slog.Default(), &NopVerifier{})
	if err := p.CleanupHost(context.Background(), HostNetworkSpec{HostID: hostID}); err != nil {
		t.Fatalf("CleanupHost returned err: %v", err)
	}

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("expected SingBoxConfigDir to be removed, got Stat err=%v", err)
	}
}

func TestNewContainerProxyProvider(t *testing.T) {
	p := NewContainerProxyProvider(nil, nil)
	if p == nil {
		t.Fatal("NewContainerProxyProvider returned nil")
	}
	if p.logger != nil {
		t.Error("expected nil logger when passed nil")
	}
	if p.verifier != nil {
		t.Error("expected nil verifier when passed nil")
	}
}

// TestContainerProxyProvider_PrepareGateway_DarwinChownNotFatal 锁 D-54-2 darwin
// 容错：darwin / 非 root 跑 chown root:singbox 必失败，PrepareGateway 应降级为
// Warn 不阻断（return nil），并保留已写入磁盘的 config.json 供 worker.buildCreateArgs
// bind mount 引用。
//
// Linux root 环境下 chown 必成功，本测试 SKIP。
func TestContainerProxyProvider_PrepareGateway_DarwinChownNotFatal(t *testing.T) {
	if runtime.GOOS == "linux" && os.Geteuid() == 0 {
		t.Skip("test simulates darwin chown EPERM; skipping on linux root")
	}
	tmp := t.TempDir()
	t.Setenv("DATA_DIR", tmp)
	p := NewContainerProxyProvider(slog.Default(), &NopVerifier{})
	err := p.PrepareGateway(context.Background(), HostNetworkSpec{
		HostID: "h-darwin",
		Egress: &EgressConfig{
			EgressIPID: "eip", ExpectedIP: "9.9.9.9", TunnelType: TunnelTypeProxy,
			Proxy: &ProxySpec{
				OutboundConfig: json.RawMessage(`{"type":"socks","server":"1.2.3.4","server_port":1080}`),
				DNSServer:      "1.1.1.1",
			},
		},
	})
	if err != nil {
		t.Errorf("PrepareGateway should not fail on chown EPERM (darwin/non-root); got: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(tmp, "gateway", "h-darwin", "config.json")); statErr != nil {
		t.Errorf("config.json should exist after PrepareGateway: %v", statErr)
	}
}
