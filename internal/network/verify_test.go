package network

import (
	"context"
	"testing"
)

func TestVerifyResult_AllPassed(t *testing.T) {
	tests := []struct {
		name     string
		result   VerifyResult
		expected bool
	}{
		{
			name:     "all checks pass",
			result:   VerifyResult{EgressIPMatch: true, DNSCorrect: true, LeakBlocked: true},
			expected: true,
		},
		{
			name:     "egress IP mismatch",
			result:   VerifyResult{EgressIPMatch: false, DNSCorrect: true, LeakBlocked: true},
			expected: false,
		},
		{
			name:     "DNS incorrect",
			result:   VerifyResult{EgressIPMatch: true, DNSCorrect: false, LeakBlocked: true},
			expected: false,
		},
		{
			name:     "leak not blocked",
			result:   VerifyResult{EgressIPMatch: true, DNSCorrect: true, LeakBlocked: false},
			expected: false,
		},
		{
			name:     "all checks fail",
			result:   VerifyResult{EgressIPMatch: false, DNSCorrect: false, LeakBlocked: false},
			expected: false,
		},
		{
			name:     "only egress passes",
			result:   VerifyResult{EgressIPMatch: true, DNSCorrect: false, LeakBlocked: false},
			expected: false,
		},
		{
			name:     "only DNS passes",
			result:   VerifyResult{EgressIPMatch: false, DNSCorrect: true, LeakBlocked: false},
			expected: false,
		},
		{
			name:     "only leak blocked passes",
			result:   VerifyResult{EgressIPMatch: false, DNSCorrect: false, LeakBlocked: true},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.result.AllPassed(); got != tt.expected {
				t.Errorf("AllPassed() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestFirstNetworkError_EgressUnreachable(t *testing.T) {
	cfg := EgressConfig{ExpectedIP: "1.2.3.4"}
	result := VerifyResult{EgressIPMatch: false, ActualEgressIP: "", DNSCorrect: false, LeakBlocked: false}

	err := firstNetworkError(cfg, result)
	if err.Type != ErrEgressUnreachable {
		t.Errorf("expected ErrEgressUnreachable, got %s", err.Type)
	}
}

func TestFirstNetworkError_EgressMismatch(t *testing.T) {
	cfg := EgressConfig{ExpectedIP: "1.2.3.4"}
	result := VerifyResult{EgressIPMatch: false, ActualEgressIP: "5.6.7.8", DNSCorrect: false, LeakBlocked: false}

	err := firstNetworkError(cfg, result)
	if err.Type != ErrEgressIPMismatch {
		t.Errorf("expected ErrEgressIPMismatch, got %s", err.Type)
	}
	if err.Metadata["expected"] != "1.2.3.4" || err.Metadata["actual"] != "5.6.7.8" {
		t.Errorf("unexpected metadata: %v", err.Metadata)
	}
}

func TestFirstNetworkError_DNSLeak(t *testing.T) {
	cfg := EgressConfig{
		ExpectedIP: "1.2.3.4",
		Proxy:      &ProxySpec{DNSServer: "10.0.0.1"},
	}
	result := VerifyResult{EgressIPMatch: true, DNSCorrect: false, ActualDNS: "8.8.8.8", LeakBlocked: true}

	err := firstNetworkError(cfg, result)
	if err.Type != ErrDNSLeak {
		t.Errorf("expected ErrDNSLeak, got %s", err.Type)
	}
	if err.Metadata["expected_dns"] != "10.0.0.1" || err.Metadata["actual_dns"] != "8.8.8.8" {
		t.Errorf("unexpected metadata: %v", err.Metadata)
	}
}

func TestFirstNetworkError_DNSLeak_NilProxy(t *testing.T) {
	// DNS error with nil Proxy should not panic
	cfg := EgressConfig{ExpectedIP: "1.2.3.4", Proxy: nil}
	result := VerifyResult{EgressIPMatch: true, DNSCorrect: false, ActualDNS: "8.8.8.8", LeakBlocked: true}

	err := firstNetworkError(cfg, result)
	if err.Type != ErrDNSLeak {
		t.Errorf("expected ErrDNSLeak, got %s", err.Type)
	}
	// expectedDNS should be empty since Proxy is nil
	expectedDNS, _ := err.Metadata["expected_dns"].(string)
	if expectedDNS != "" {
		t.Errorf("expected_dns should be empty when Proxy is nil, got %q", expectedDNS)
	}
}

func TestVerifyNetworkIntegrity_NoNsenter(t *testing.T) {
	// On macOS (and any system without nsenter), the nsenter commands will fail.
	// This tests the error paths without requiring real network or containers.
	// The test verifies that VerifyNetworkIntegrity handles command failures gracefully.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediate cancellation

	result, err := VerifyNetworkIntegrity(ctx, 1, EgressConfig{ExpectedIP: "1.2.3.4"})

	// All checks should fail because nsenter doesn't exist on this platform.
	// LeakBlocked = true because command failure means the direct outbound was "blocked".
	t.Logf("verify result: EgressIPMatch=%v DNSCorrect=%v LeakBlocked=%v",
		result.EgressIPMatch, result.DNSCorrect, result.LeakBlocked)
	t.Logf("verify error: %v", err)

	// With all checks failing, we expect a non-nil error.
	if err == nil {
		t.Error("expected error when nsenter is not available")
	}
	if result.LeakBlocked != true {
		t.Errorf("expected LeakBlocked=true (command failure = blocked), got %v", result.LeakBlocked)
	}
}

func TestVerifyNetworkIntegrity_BackgroundContext(t *testing.T) {
	// Using background context (not cancelled). nsenter will still fail on macOS
	// because the binary doesn't exist.
	if testing.Short() {
		// In short mode, we use a tight timeout to avoid the 15s egress check timeout.
		// The verify functions create their own timeout contexts (15s, 5s, 5s).
		// On macOS without nsenter, the commands fail instantly, so this is fast.
	}

	result, err := VerifyNetworkIntegrity(context.Background(), 99999, EgressConfig{
		ExpectedIP: "1.2.3.4",
		EgressIPID: "eip-test",
		TunnelType: TunnelTypeProxy,
		Proxy:      &ProxySpec{DNSServer: "10.0.0.1"},
	})

	// On macOS, nsenter doesn't exist, so all checks fail.
	t.Logf("verify result: EgressIPMatch=%v ActualEgressIP=%q DNSCorrect=%v ActualDNS=%q LeakBlocked=%v LeakTarget=%q",
		result.EgressIPMatch, result.ActualEgressIP, result.DNSCorrect, result.ActualDNS, result.LeakBlocked, result.LeakTarget)

	if err == nil {
		t.Error("expected error from VerifyNetworkIntegrity without nsenter")
	}
}

func TestFirstNetworkError_LeakNotBlocked(t *testing.T) {
	cfg := EgressConfig{ExpectedIP: "1.2.3.4"}
	result := VerifyResult{EgressIPMatch: true, DNSCorrect: true, LeakBlocked: false, LeakTarget: "1.1.1.1:80"}

	err := firstNetworkError(cfg, result)
	if err.Type != ErrLeakNotBlocked {
		t.Errorf("expected ErrLeakNotBlocked, got %s", err.Type)
	}
	if err.Metadata["target"] != "1.1.1.1:80" {
		t.Errorf("unexpected metadata: %v", err.Metadata)
	}
}
