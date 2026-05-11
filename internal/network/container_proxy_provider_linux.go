//go:build linux

package network

import (
	"context"
	"fmt"
	"net"
)

func applyWorkerFirewall(ctx context.Context, workerName, gwIP, bridgeGW string) error {
	containerNS, _, err := GetContainerNetNS(workerName)
	if err != nil {
		return fmt.Errorf("get worker netns: %w", err)
	}
	defer containerNS.Close()

	gw := net.ParseIP(gwIP)
	bgw := net.ParseIP(bridgeGW)

	if err := ApplyWorkerFirewallRules(containerNS, gw, bgw, 22); err != nil {
		return fmt.Errorf("apply worker firewall rules: %w", err)
	}
	return nil
}

func verifyWorkerNetwork(ctx context.Context, workerName string, egress EgressConfig) (VerifyResult, error) {
	_, pid, err := GetContainerNetNS(workerName)
	if err != nil {
		return VerifyResult{}, fmt.Errorf("get worker pid: %w", err)
	}
	return VerifyNetworkIntegrity(ctx, pid, egress)
}

func cleanupWorkerFirewall(ctx context.Context, workerName string) {
	containerNS, _, err := GetContainerNetNS(workerName)
	if err != nil {
		return
	}
	defer containerNS.Close()

	_ = CleanupWorkerFirewallRules(containerNS)
}
