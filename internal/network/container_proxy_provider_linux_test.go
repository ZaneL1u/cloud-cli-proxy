//go:build linux

package network

import (
	"context"
	"strings"
	"testing"
)

func TestApplyWorkerFirewall_ErrorPaths(t *testing.T) {
	// Test with a nonexistent container - should return error, not panic
	ctx := context.Background()
	err := applyWorkerFirewall(ctx, "nonexistent-worker-12345", "10.99.1.2", "10.99.1.1", "")
	if err == nil {
		t.Error("expected error for nonexistent container")
	}
	// Error should mention the container or netns
	errStr := err.Error()
	if !strings.Contains(errStr, "worker") && !strings.Contains(errStr, "netns") &&
		!strings.Contains(errStr, "inspect") && !strings.Contains(errStr, "container") {
		t.Errorf("error should mention worker/netns/container, got: %v", err)
	}
}

func TestVerifyWorkerNetwork_InvalidContainer(t *testing.T) {
	// Test with a nonexistent container - should return error, not panic
	ctx := context.Background()
	result, err := verifyWorkerNetwork(ctx, "nonexistent-worker-67890", EgressConfig{ExpectedIP: "1.2.3.4"})
	if err == nil {
		t.Error("expected error for nonexistent container")
	}
	// Result should be zero value
	if result.EgressIPMatch || result.DNSCorrect || result.LeakBlocked {
		t.Error("expected zero-value result for failed verification")
	}
}

func TestCleanupWorkerFirewall_NonexistentContainer(t *testing.T) {
	// Test with a nonexistent container - should silently return without panic
	ctx := context.Background()

	// This should not panic even if the container doesn't exist
	cleanupWorkerFirewall(ctx, "nonexistent-worker-abcde")

	// If we get here without panic, the test passes
	t.Log("cleanupWorkerFirewall handled nonexistent container without panic")
}

func TestCleanupWorkerFirewall_EmptyName(t *testing.T) {
	// Test with empty container name - should silently return without panic
	ctx := context.Background()

	// This should not panic
	cleanupWorkerFirewall(ctx, "")

	// If we get here without panic, the test passes
	t.Log("cleanupWorkerFirewall handled empty name without panic")
}
