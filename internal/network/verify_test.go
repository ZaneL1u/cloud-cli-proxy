package network

import "testing"

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
