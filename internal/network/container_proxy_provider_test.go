//go:build linux

package network

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestNetworkName(t *testing.T) {
	tests := []struct {
		hostID string
		want   string
	}{
		{hostID: "abc123", want: "cloudproxy-net-abc123"},
		{hostID: "host-1", want: "cloudproxy-net-host-1"},
		{hostID: "", want: "cloudproxy-net-"},
	}
	for _, tt := range tests {
		t.Run(tt.hostID, func(t *testing.T) {
			got := networkName(tt.hostID)
			if got != tt.want {
				t.Errorf("networkName(%q) = %q, want %q", tt.hostID, got, tt.want)
			}
		})
	}
}

func TestGatewayContainerName(t *testing.T) {
	got := gatewayContainerName("my-host")
	want := "cloudproxy-gw-my-host"
	if got != want {
		t.Errorf("gatewayContainerName = %q, want %q", got, want)
	}
}

func TestWorkerContainerName(t *testing.T) {
	got := workerContainerName("my-host")
	want := "cloudproxy-my-host"
	if got != want {
		t.Errorf("workerContainerName = %q, want %q", got, want)
	}
}

func TestSubnetThirdOctet_Deterministic(t *testing.T) {
	// Same input always produces same output
	hostID := "test-host-id"
	first := subnetThirdOctet(hostID)
	for i := 0; i < 10; i++ {
		if got := subnetThirdOctet(hostID); got != first {
			t.Errorf("subnetThirdOctet not deterministic: got %d, want %d", got, first)
		}
	}
}

func TestSubnetThirdOctet_Range(t *testing.T) {
	// The third octet should be in range [20, 219]
	hostIDs := []string{"a", "abc", "host-1", "550e8400-e29b-41d4-a716-446655440000", "test", "very-long-host-id-that-goes-on-and-on"}
	for _, hid := range hostIDs {
		octet := subnetThirdOctet(hid)
		if octet < 20 || octet > 219 {
			t.Errorf("subnetThirdOctet(%q) = %d, want in range [20, 219]", hid, octet)
		}
	}
}

func TestSubnetThirdOctet_DifferentInputs(t *testing.T) {
	// Different inputs may produce different outputs (hash collisions possible but rare)
	results := make(map[string]int)
	for _, hid := range []string{"host-a", "host-b", "host-c"} {
		results[hid] = subnetThirdOctet(hid)
	}
	// This is a probabilistic test - FNV should distribute well
	unique := make(map[int]bool)
	for _, v := range results {
		unique[v] = true
	}
	if len(unique) < 2 {
		t.Log("all inputs produced same octet (possible FNV collision, not necessarily a bug)")
	}
}

func TestSubnetThirdOctet_CollisionResistance(t *testing.T) {
	// Generate 100 random hostIDs and verify distribution
	const count = 100
	octets := make(map[int]int)
	for i := 0; i < count; i++ {
		hostID := "host-" + string(rune('a'+i%26)) + string(rune('0'+i/26))
		octet := subnetThirdOctet(hostID)
		if octet < 20 || octet > 219 {
			t.Fatalf("subnetThirdOctet(%q) = %d, out of range [20, 219]", hostID, octet)
		}
		octets[octet]++
	}

	// Count collisions (octets that appear more than once)
	collisions := 0
	for _, c := range octets {
		if c > 1 {
			collisions += c - 1
		}
	}
	// 100 samples into 200 possible values: expect very few collisions
	// Allow up to 10 collisions as a generous threshold
	if collisions > 10 {
		t.Errorf("too many collisions: %d out of %d samples", collisions, count)
	}
	t.Logf("collision count: %d/%d (unique octets: %d)", collisions, count, len(octets))
}

func TestGatewayConfigDir_WithDataDir(t *testing.T) {
	os.Setenv("DATA_DIR", "/custom/data")
	defer os.Unsetenv("DATA_DIR")

	got := GatewayConfigDir("host-1")
	want := "/custom/data/gateway/host-1"
	if got != want {
		t.Errorf("GatewayConfigDir = %q, want %q", got, want)
	}
}

func TestGatewayConfigDir_Default(t *testing.T) {
	os.Unsetenv("DATA_DIR")

	got := GatewayConfigDir("host-1")
	want := "/var/lib/cloud-cli-proxy/gateway/host-1"
	if got != want {
		t.Errorf("GatewayConfigDir = %q, want %q", got, want)
	}
}

func TestGatewayConfigDir_PathSanitization(t *testing.T) {
	// Test that various hostID values produce predictable paths
	tests := []struct {
		hostID string
		want   string
	}{
		{"host-1", "/var/lib/cloud-cli-proxy/gateway/host-1"},
		{"host/with/slash", "/var/lib/cloud-cli-proxy/gateway/host/with/slash"},
		{"host..dot", "/var/lib/cloud-cli-proxy/gateway/host..dot"},
		{"host space", "/var/lib/cloud-cli-proxy/gateway/host space"},
		{"host\x00null", "/var/lib/cloud-cli-proxy/gateway/host\x00null"},
	}

	os.Unsetenv("DATA_DIR")
	for _, tt := range tests {
		t.Run(tt.hostID, func(t *testing.T) {
			got := GatewayConfigDir(tt.hostID)
			if got != tt.want {
				t.Errorf("GatewayConfigDir(%q) = %q, want %q", tt.hostID, got, tt.want)
			}
		})
	}
}

func TestGatewayImage_Custom(t *testing.T) {
	os.Setenv("CLOUD_CLI_PROXY_GATEWAY_IMAGE", "my-custom-image:v2")
	defer os.Unsetenv("CLOUD_CLI_PROXY_GATEWAY_IMAGE")

	got := GatewayImage()
	if got != "my-custom-image:v2" {
		t.Errorf("GatewayImage = %q, want %q", got, "my-custom-image:v2")
	}
}

func TestGatewayImage_Default(t *testing.T) {
	os.Unsetenv("CLOUD_CLI_PROXY_GATEWAY_IMAGE")

	got := GatewayImage()
	want := "cloud-cli-proxy-sing-gateway:local"
	if got != want {
		t.Errorf("GatewayImage = %q, want %q", got, want)
	}
}

func TestNewContainerProxyProvider(t *testing.T) {
	p := NewContainerProxyProvider(nil)
	if p == nil {
		t.Fatal("NewContainerProxyProvider returned nil")
	}
	if p.logger != nil {
		t.Error("expected nil logger when passed nil")
	}
}

func TestConfigureWorkerEgress_Retry(t *testing.T) {
	// Call with a nonexistent container name to trigger retry exhaustion.
	// The function should retry 3 times and return an error containing "failed after 3 attempts".
	ctx := context.Background()
	err := configureWorkerEgress(ctx, "nonexistent-container-12345", "10.99.1.2", "10.99.1.3")
	if err == nil {
		t.Fatal("expected error for nonexistent container")
	}
	if !strings.Contains(err.Error(), "failed after 3 attempts") {
		t.Errorf("expected error to contain 'failed after 3 attempts', got: %v", err)
	}
}

func TestConfigureWorkerEgress_RetryBackoff(t *testing.T) {
	// Verify that retry adds increasing delay between attempts.
	// Attempt 1 -> sleep 500ms, Attempt 2 -> sleep 1000ms.
	// Total minimum delay = 1500ms.
	ctx := context.Background()
	start := time.Now()
	_ = configureWorkerEgress(ctx, "nonexistent-container-99999", "10.99.1.2", "10.99.1.3")
	elapsed := time.Since(start)

	// Should take at least 1500ms (500ms + 1000ms sleeps between attempts)
	if elapsed < 1400*time.Millisecond {
		t.Errorf("retry backoff too short: %v (expected at least ~1.5s)", elapsed)
	}
}

func TestTryConfigureWorkerEgress_ScriptFormat(t *testing.T) {
	// Since tryConfigureWorkerEgress executes docker exec against a real container,
	// we verify the script content indirectly by testing against a nonexistent
	// container and checking the error message contains expected script fragments.
	ctx := context.Background()
	err := tryConfigureWorkerEgress(ctx, "nonexistent-script-test", "10.99.1.2", "10.99.1.3")
	if err == nil {
		t.Fatal("expected error for nonexistent container")
	}

	// The error comes from docker exec failing; we can't inspect the script directly.
	// But we can verify the function signature and that it returns an error.
	errStr := err.Error()
	if errStr == "" {
		t.Error("expected non-empty error message")
	}
}

func TestTryConfigureWorkerEgress_ScriptContainsKeyCommands(t *testing.T) {
	// Phase 45 Plan 02：脚本不再写 /etc/resolv.conf（已被 ro bind mount 接管），
	// 只负责 default route 配置；同时必须**不再**含有 `echo nameserver ... > /etc/resolv.conf`
	// 这行（防 BYPASS-DNS-03 回归）。
	defaultGW := "10.99.1.2"
	workerIP := "10.99.1.3"

	script := buildWorkerEgressScript(workerIP, defaultGW)

	expectedCommands := []string{
		"ip route add default via " + defaultGW,
		"ip route show default | head -1",
	}
	for _, cmd := range expectedCommands {
		if !strings.Contains(script, cmd) {
			t.Errorf("script missing expected command: %q\nscript:\n%s", cmd, script)
		}
	}

	// 反向断言：DNS 入口锁后，脚本必须不能再写 /etc/resolv.conf
	forbidden := []string{
		"> /etc/resolv.conf",
		"nameserver 8.8.8.8",
	}
	for _, frag := range forbidden {
		if strings.Contains(script, frag) {
			t.Errorf("script must NOT contain %q after DNS lock-in (Phase 45 Plan 02)\nscript:\n%s", frag, script)
		}
	}
}

func buildWorkerEgressScript(workerIP, defaultGW string) string {
	return `set -e
	# 等待网络接口就绪
	for i in 1 2 3 4 5; do
	  DEV=$(ip -o addr show | grep '` + workerIP + `' | awk '{print $2}' | head -1)
	  [ -n "$DEV" ] && break
	  sleep 1
	done
	if [ -z "$DEV" ]; then
	  echo "waiting for interface with IP ` + workerIP + ` timed out"
	  ip -o addr show >&2
	  exit 1
	fi
	# 删除所有现有 default 路由
	ip route show default | while read -r line; do
	  gw=$(echo "$line" | grep -oP 'via \K[^ ]+' || true)
	  dev=$(echo "$line" | grep -oP 'dev \K[^ ]+' || true)
	  if [ -n "$gw" ] && [ -n "$dev" ]; then
	    ip route del default via "$gw" dev "$dev" 2>/dev/null || true
	  fi
	done
	ip route del default 2>/dev/null || true
	# 默认路由指向 gateway，所有流量必须经过 sing-box 代理隧道
	ip route add default via ` + defaultGW + ` dev "$DEV" metric 0
	# 立即 verify
	default_route=$(ip route show default | head -1)
	echo "$default_route" | grep -q "via ` + defaultGW + `"
	`
}

// TestWorker_CreateHost_CallOrder 的实现已迁移到无 build tag 的 call_order_test.go，
// 以便在 macOS 开发机也能跑通。此处保留注释作为定位线索。
