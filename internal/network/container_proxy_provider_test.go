package network

import (
	"os"
	"testing"
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

func TestGatewayConfigDir_WithDataDir(t *testing.T) {
	os.Setenv("DATA_DIR", "/custom/data")
	defer os.Unsetenv("DATA_DIR")

	got := gatewayConfigDir("host-1")
	want := "/custom/data/gateway/host-1"
	if got != want {
		t.Errorf("gatewayConfigDir = %q, want %q", got, want)
	}
}

func TestGatewayConfigDir_Default(t *testing.T) {
	os.Unsetenv("DATA_DIR")

	got := gatewayConfigDir("host-1")
	want := "/var/lib/cloud-cli-proxy/gateway/host-1"
	if got != want {
		t.Errorf("gatewayConfigDir = %q, want %q", got, want)
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

