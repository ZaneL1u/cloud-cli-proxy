package sshproxy

import (
	"fmt"
	"net"
	"testing"

	"golang.org/x/crypto/ssh"
)

// ---- channelOpenDirectMsg payload parsing ----

func TestDirectTCPIP_PayloadParse(t *testing.T) {
	msg := channelOpenDirectMsg{
		Raddr: "127.0.0.1",
		Rport: 8080,
		Laddr: "0.0.0.0",
		Lport: 0,
	}

	data := ssh.Marshal(&msg)

	var parsed channelOpenDirectMsg
	if err := ssh.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}

	if parsed.Raddr != "127.0.0.1" {
		t.Fatalf("Raddr: got %q, want %q", parsed.Raddr, "127.0.0.1")
	}
	if parsed.Rport != 8080 {
		t.Fatalf("Rport: got %d, want %d", parsed.Rport, 8080)
	}
	if parsed.Laddr != "0.0.0.0" {
		t.Fatalf("Laddr: got %q, want %q", parsed.Laddr, "0.0.0.0")
	}
	if parsed.Lport != 0 {
		t.Fatalf("Lport: got %d, want %d", parsed.Lport, 0)
	}
}

func TestDirectTCPIP_PayloadParse_TooShort(t *testing.T) {
	// Payload shorter than channelOpenDirectMsg wire size (4 string fields = at least 16 bytes header).
	data := []byte{0, 0, 0, 5, 'h', 'e', 'l', 'l', 'o'} // only 9 bytes, incomplete

	var parsed channelOpenDirectMsg
	err := ssh.Unmarshal(data, &parsed)
	if err == nil {
		t.Fatal("expected error for short payload, got nil")
	}
}

// ---- isForbiddenTarget ----

func TestIsForbiddenTarget_ManagementSubnet(t *testing.T) {
	tests := []struct {
		host string
		port int
		desc string
	}{
		{"10.99.1.1", 80, "management subnet IP"},
		{"10.99.0.1", 22, "management subnet gateway"},
		{"10.99.255.255", 443, "management subnet broadcast"},
		{"10.99.123.45", 1080, "management subnet arbitrary"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			if !isForbiddenTarget(tt.host, tt.port) {
				t.Errorf("expected forbidden for %s:%d, got allowed", tt.host, tt.port)
			}
		})
	}
}

func TestIsForbiddenTarget_AllowedIP(t *testing.T) {
	tests := []struct {
		host string
		port int
		desc string
	}{
		{"127.0.0.1", 8080, "localhost"},
		{"8.8.8.8", 53, "public DNS"},
		{"192.168.1.1", 443, "private but not forbidden"},
		{"172.16.0.1", 80, "docker default but not in blocklist"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			if isForbiddenTarget(tt.host, tt.port) {
				t.Errorf("expected allowed for %s:%d, got forbidden", tt.host, tt.port)
			}
		})
	}
}

func TestIsForbiddenTarget_MetadataEndpoint(t *testing.T) {
	if !isForbiddenTarget("metadata.google.internal", 80) {
		t.Error("expected forbidden for metadata.google.internal:80")
	}
}

func TestIsForbiddenTarget_DockerSocket(t *testing.T) {
	tests := []struct {
		host string
		port int
	}{
		{"169.254.169.254", 2375},
		{"169.254.169.254", 2376},
	}

	for _, tt := range tests {
		desc := net.JoinHostPort(tt.host, fmt.Sprintf("%d", tt.port))
		t.Run(desc, func(t *testing.T) {
			if !isForbiddenTarget(tt.host, tt.port) {
				t.Errorf("expected forbidden for %s:%d", tt.host, tt.port)
			}
		})
	}
}

func TestIsForbiddenTarget_NonIPHostname(t *testing.T) {
	// Hostname that isn't IP and isn't in forbiddenHosts — should not be forbidden
	// unless the port is forbidden.
	if isForbiddenTarget("example.com", 443) {
		t.Error("expected allowed for example.com:443")
	}
}

func TestIsForbiddenTarget_ForbiddenPortNonIP(t *testing.T) {
	// Port 2375 with a hostname (not an IP) should still be blocked by port check.
	if !isForbiddenTarget("some-host.local", 2375) {
		t.Error("expected forbidden for some-host.local:2375 (port match)")
	}
}

func TestIsForbiddenTarget_PublicIPAllowed(t *testing.T) {
	if isForbiddenTarget("8.8.8.8", 53) {
		t.Error("expected allowed for 8.8.8.8:53")
	}
}
