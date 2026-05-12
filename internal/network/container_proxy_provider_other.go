//go:build !linux

package network

import (
	"context"
)

func applyWorkerFirewall(_ context.Context, _, _, _, _ string) error {
	return nil
}

func verifyWorkerNetwork(_ context.Context, _ string, _ EgressConfig) (VerifyResult, error) {
	return VerifyResult{}, nil
}

func cleanupWorkerFirewall(_ context.Context, _ string) {}
