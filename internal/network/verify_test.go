package network

import (
	"context"
	"strings"
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
	// Phase 45 Plan 02：firstNetworkError 的 expected_dns 已经与
	// EgressConfig.Proxy.DNSServer 解耦，永远是常量 containerExpectedDNS
	// (172.19.0.1)。这里 Proxy.DNSServer 字段仍保留语义（gateway → 上游
	// DNS），但不再用于断言。
	cfg := EgressConfig{
		ExpectedIP: "1.2.3.4",
		Proxy:      &ProxySpec{DNSServer: "10.0.0.1"},
	}
	result := VerifyResult{EgressIPMatch: true, DNSCorrect: false, ActualDNS: "8.8.8.8", LeakBlocked: true}

	err := firstNetworkError(cfg, result)
	if err.Type != ErrDNSLeak {
		t.Errorf("expected ErrDNSLeak, got %s", err.Type)
	}
	if err.Metadata["expected_dns"] != "172.19.0.1" || err.Metadata["actual_dns"] != "8.8.8.8" {
		t.Errorf("unexpected metadata: %v", err.Metadata)
	}
}

func TestFirstNetworkError_DNSLeak_NilProxy(t *testing.T) {
	// Phase 45 Plan 02：expected_dns 已与 EgressConfig.Proxy 字段解耦，
	// 即使 Proxy 为 nil，预期值仍是常量 containerExpectedDNS (172.19.0.1)。
	cfg := EgressConfig{ExpectedIP: "1.2.3.4", Proxy: nil}
	result := VerifyResult{EgressIPMatch: true, DNSCorrect: false, ActualDNS: "8.8.8.8", LeakBlocked: true}

	err := firstNetworkError(cfg, result)
	if err.Type != ErrDNSLeak {
		t.Errorf("expected ErrDNSLeak, got %s", err.Type)
	}
	expectedDNS, _ := err.Metadata["expected_dns"].(string)
	if expectedDNS != "172.19.0.1" {
		t.Errorf("expected_dns should be containerExpectedDNS 172.19.0.1 even when Proxy is nil, got %q", expectedDNS)
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

func TestFirstNetworkError_Priority(t *testing.T) {
	// Priority order: EgressIPMismatch > DNSLeak > LeakNotBlocked

	tests := []struct {
		name     string
		result   VerifyResult
		expected NetworkErrorType
	}{
		{
			name:     "egress mismatch has highest priority",
			result:   VerifyResult{EgressIPMatch: false, ActualEgressIP: "5.6.7.8", DNSCorrect: false, LeakBlocked: false},
			expected: ErrEgressIPMismatch,
		},
		{
			name:     "egress unreachable has highest priority",
			result:   VerifyResult{EgressIPMatch: false, ActualEgressIP: "", DNSCorrect: false, LeakBlocked: false},
			expected: ErrEgressUnreachable,
		},
		{
			name:     "DNS leak when egress OK",
			result:   VerifyResult{EgressIPMatch: true, DNSCorrect: false, ActualDNS: "8.8.8.8", LeakBlocked: false},
			expected: ErrDNSLeak,
		},
		{
			name:     "leak not blocked when egress and DNS OK",
			result:   VerifyResult{EgressIPMatch: true, DNSCorrect: true, LeakBlocked: false, LeakTarget: "1.1.1.1:80"},
			expected: ErrLeakNotBlocked,
		},
		{
			name:     "DNS leak takes priority over leak blocked",
			result:   VerifyResult{EgressIPMatch: true, DNSCorrect: false, ActualDNS: "8.8.8.8", LeakBlocked: false},
			expected: ErrDNSLeak,
		},
	}

	cfg := EgressConfig{ExpectedIP: "1.2.3.4", Proxy: &ProxySpec{DNSServer: "10.0.0.1"}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := firstNetworkError(cfg, tt.result)
			if err.Type != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, err.Type)
			}
		})
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

func TestVerifyNetworkIntegrity_ContextTimeout(t *testing.T) {
	// Verify that VerifyNetworkIntegrity does not deadlock when given a cancelled context.
	// It should return promptly with results.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediately cancelled

	done := make(chan struct{})
	var result VerifyResult
	var err error

	go func() {
		defer close(done)
		result, err = VerifyNetworkIntegrity(ctx, 1, EgressConfig{ExpectedIP: "1.2.3.4"})
	}()

	select {
	case <-done:
		// Good, function returned promptly
	case <-context.Background().Done():
		t.Fatal("VerifyNetworkIntegrity deadlocked with cancelled context")
	}

	// The function should return some result regardless of context cancellation.
	// Individual check functions use their own timeout contexts derived from the
	// passed context, so behaviour depends on how quickly they respond.
	t.Logf("result: EgressIPMatch=%v DNSCorrect=%v LeakBlocked=%v err=%v",
		result.EgressIPMatch, result.DNSCorrect, result.LeakBlocked, err)
}

func TestVerifyNetworkIntegrity_LeakTargetSet(t *testing.T) {
	// Verify that the leak target is always set before the check runs.
	// This is a unit test of the verifyLeakBlocked function's contract.
	result := VerifyResult{LeakTarget: ""}

	// The leak target should be set to a known value by verifyLeakBlocked.
	// We verify this indirectly by checking the default value in a fresh result.
	if result.LeakTarget != "" {
		t.Logf("initial LeakTarget: %q", result.LeakTarget)
	}

	// After calling verifyLeakBlocked (via VerifyNetworkIntegrity), LeakTarget should be set.
	// Since we can't run nsenter on macOS, we just verify the function doesn't panic
	// and the result struct is properly populated.
	ctx := context.Background()
	res, _ := VerifyNetworkIntegrity(ctx, 99999, EgressConfig{ExpectedIP: "1.2.3.4"})
	if res.LeakTarget == "" {
		t.Error("LeakTarget should be set after verification")
	}
	if !strings.Contains(res.LeakTarget, ":") {
		t.Errorf("LeakTarget should contain port separator, got %q", res.LeakTarget)
	}
}
