package network

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// VerifyResult captures the outcome of each verification check performed
// inside a container's network namespace after tunnel wiring completes.
type VerifyResult struct {
	EgressIPMatch  bool
	ActualEgressIP string
	DNSCorrect     bool
	ActualDNS      string
	LeakBlocked    bool
	LeakTarget     string
}

// AllPassed returns true only when all three verification checks passed.
func (r VerifyResult) AllPassed() bool {
	return r.EgressIPMatch && r.DNSCorrect && r.LeakBlocked
}

// VerifyNetworkIntegrity runs three independent checks inside the container's
// network namespace via nsenter:
//  1. Egress IP must match the expected binding (D-09)
//  2. DNS resolver must point to the tunnel-side DNS server (D-09)
//  3. Direct (non-tunnel) outbound connections must be blocked (D-09)
//
// All three checks run regardless of individual failures so the caller gets
// the complete verification state. The returned error (if any) is a typed
// NetworkError matching the highest-priority failing check.
func VerifyNetworkIntegrity(ctx context.Context, containerPID uint32, expected EgressConfig) (VerifyResult, error) {
	prefix := []string{"nsenter", "-t", strconv.FormatUint(uint64(containerPID), 10), "-n", "--"}

	var result VerifyResult

	// Check 1: egress IP matches binding
	verifyEgressIP(ctx, prefix, expected.ExpectedIP, &result)

	// Check 2: DNS resolver points to tunnel DNS
	var expectedDNS string
	if expected.Proxy != nil {
		expectedDNS = expected.Proxy.DNSServer
	}
	verifyDNS(ctx, prefix, expectedDNS, &result)

	// Check 3: direct outbound is blocked by firewall
	verifyLeakBlocked(ctx, prefix, &result)

	if result.AllPassed() {
		return result, nil
	}

	return result, firstNetworkError(expected, result)
}

func verifyEgressIP(ctx context.Context, prefix []string, expectedIP string, result *VerifyResult) {
	checkCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	args := append(append([]string{}, prefix...), "curl", "-4", "--max-time", "10", "-s", "https://api.ipify.org")
	out, err := exec.CommandContext(checkCtx, args[0], args[1:]...).Output()
	if err != nil {
		result.EgressIPMatch = false
		result.ActualEgressIP = ""
		return
	}

	actual := strings.TrimSpace(string(out))
	result.ActualEgressIP = actual
	result.EgressIPMatch = actual == expectedIP
}

func verifyDNS(ctx context.Context, prefix []string, expectedDNS string, result *VerifyResult) {
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	args := append(append([]string{}, prefix...), "cat", "/etc/resolv.conf")
	out, err := exec.CommandContext(checkCtx, args[0], args[1:]...).Output()
	if err != nil {
		result.DNSCorrect = false
		result.ActualDNS = ""
		return
	}

	var firstNS string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "nameserver") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				firstNS = fields[1]
				break
			}
		}
	}

	result.ActualDNS = firstNS
	result.DNSCorrect = firstNS == expectedDNS
}

func verifyLeakBlocked(ctx context.Context, prefix []string, result *VerifyResult) {
	result.LeakTarget = "1.1.1.1:80"

	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	args := append(append([]string{}, prefix...), "timeout", "3", "bash", "-c", "echo >/dev/tcp/1.1.1.1/80")
	err := exec.CommandContext(checkCtx, args[0], args[1:]...).Run()

	// Connection failure means firewall is blocking direct outbound — that's the desired state.
	result.LeakBlocked = err != nil
}

// firstNetworkError returns the highest-priority NetworkError for the first
// failing check (egress IP > DNS > leak).
func firstNetworkError(expected EgressConfig, r VerifyResult) *NetworkError {
	hostID := "" // populated by caller context if needed

	if !r.EgressIPMatch {
		if r.ActualEgressIP == "" {
			return &NetworkError{
				Type:    ErrEgressUnreachable,
				Message: "egress connectivity check failed",
				HostID:  hostID,
			}
		}
		return &NetworkError{
			Type:    ErrEgressIPMismatch,
			Message: fmt.Sprintf("egress IP mismatch: expected %s, got %s", expected.ExpectedIP, r.ActualEgressIP),
			HostID:  hostID,
			Metadata: map[string]any{
				"expected": expected.ExpectedIP,
				"actual":   r.ActualEgressIP,
			},
		}
	}

	if !r.DNSCorrect {
		var expectedDNS string
		if expected.Proxy != nil {
			expectedDNS = expected.Proxy.DNSServer
		}
		return &NetworkError{
			Type:    ErrDNSLeak,
			Message: fmt.Sprintf("DNS resolver mismatch: expected %s, got %s", expectedDNS, r.ActualDNS),
			HostID:  hostID,
			Metadata: map[string]any{
				"expected_dns": expectedDNS,
				"actual_dns":   r.ActualDNS,
			},
		}
	}

	return &NetworkError{
		Type:    ErrLeakNotBlocked,
		Message: fmt.Sprintf("direct outbound to %s was not blocked", r.LeakTarget),
		HostID:  hostID,
		Metadata: map[string]any{
			"target": r.LeakTarget,
		},
	}
}
