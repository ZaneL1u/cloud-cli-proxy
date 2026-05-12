//go:build linux

package network

import (
	"context"
	"fmt"
	"net"
)

func applyWorkerFirewall(ctx context.Context, workerName, gwIP, bridgeGW, proxyIP string) error {
	containerNS, _, err := GetContainerNetNS(workerName)
	if err != nil {
		return fmt.Errorf("get worker netns: %w", err)
	}
	defer containerNS.Close()

	gw := net.ParseIP(gwIP)
	bgw := net.ParseIP(bridgeGW)
	// proxyIP 允许为空（Phase 1+ 兼容路径，无代理 IP 时 ConfigureBypassFirewall skip uid 锁）
	var pip net.IP
	if proxyIP != "" {
		pip = net.ParseIP(proxyIP)
	}

	if err := ApplyWorkerFirewallRules(containerNS, gw, bgw, pip, 22); err != nil {
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
