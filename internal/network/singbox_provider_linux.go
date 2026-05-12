//go:build linux

package network

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

// SingBoxProvider implements Provider by wiring each container with a sing-box
// tun-mode proxy, management veth, controlled DNS, and nftables default-deny rules.
type SingBoxProvider struct {
	logger *slog.Logger
}

// NewSingBoxProvider creates a SingBoxProvider backed by the given logger.
func NewSingBoxProvider(logger *slog.Logger) *SingBoxProvider {
	return &SingBoxProvider{logger: logger}
}

// PrepareGateway 在 SingBoxProvider 下为 no-op：
// SingBoxProvider 的整个流程（namespace + veth + sing-box 进程注入）依赖 worker
// 容器已经存在并提供 netns / PID，无法在 worker 容器 docker create 之前先起
// gateway。Phase 45 Plan 02 的容器 DNS 入口锁仅适用于 ContainerProxyProvider
// （sidecar gateway 容器模型）；SingBoxProvider 是历史遗留的进程注入模型，
// 这里保留接口对齐，PrepareHost 仍承担完整流程。
func (sp *SingBoxProvider) PrepareGateway(_ context.Context, spec HostNetworkSpec) error {
	if spec.Egress == nil {
		return &NetworkError{
			Type:    ErrBindingMissing,
			Message: "PrepareGateway called without egress config",
			HostID:  spec.HostID,
		}
	}
	sp.logger.Info("sing-box provider: PrepareGateway is no-op (process-injection model)", "host_id", spec.HostID)
	return nil
}

// PrepareHost executes the full network wiring pipeline for a proxy-mode
// container that was started with --network=none.
func (sp *SingBoxProvider) PrepareHost(ctx context.Context, spec HostNetworkSpec) error {
	if spec.Egress == nil {
		return &NetworkError{
			Type:    ErrBindingMissing,
			Message: "PrepareHost called without egress config",
			HostID:  spec.HostID,
		}
	}

	if spec.Egress.Proxy == nil {
		return &NetworkError{
			Type:    ErrTunnelSetupFailed,
			Message: "SingBoxProvider requires proxy config",
			HostID:  spec.HostID,
		}
	}

	// Step 1: get container netns + PID
	containerName := fmt.Sprintf("cloudproxy-%s", spec.HostID)
	containerNS, pid, err := GetContainerNetNS(containerName)
	if err != nil {
		return err
	}
	defer containerNS.Close()
	spec.ContainerPID = pid

	// Step 2: get host netns
	hostNS, err := netns.Get()
	if err != nil {
		return &NetworkError{
			Type:    ErrTunnelSetupFailed,
			Message: fmt.Sprintf("get host netns: %v", err),
			HostID:  spec.HostID,
		}
	}
	defer hostNS.Close()

	// Step 3: inject management veth
	hostIP, containerIP, err := InjectManagementVeth(hostNS, containerNS, spec.HostID)
	if err != nil {
		return err
	}
	sp.logger.Info("management veth injected", "host_id", spec.HostID, "host_ip", hostIP, "container_ip", containerIP)

	// Step 4: extract proxy server address for host route and firewall whitelist
	serverIP, serverPort, err := extractProxyServer(spec.Egress.Proxy.OutboundConfig)
	if err != nil {
		return &NetworkError{
			Type:    ErrTunnelSetupFailed,
			Message: fmt.Sprintf("extract proxy server: %v", err),
			HostID:  spec.HostID,
		}
	}

	// Step 5: enable host IP forwarding
	if err := ensureIPForwarding(ctx); err != nil {
		return &NetworkError{
			Type:    ErrTunnelSetupFailed,
			Message: fmt.Sprintf("enable ip forwarding: %v", err),
			HostID:  spec.HostID,
		}
	}

	// Step 6: ensure host masquerade for mgmt veth subnet
	if err := ensureHostMasquerade(ctx); err != nil {
		return &NetworkError{
			Type:    ErrTunnelSetupFailed,
			Message: fmt.Sprintf("ensure masquerade: %v", err),
			HostID:  spec.HostID,
		}
	}

	// Step 7: add host route for proxy server via mgmt veth inside container
	if err := addProxyServerRoute(containerNS, hostNS, serverIP, hostIP); err != nil {
		return &NetworkError{
			Type:    ErrTunnelSetupFailed,
			Message: fmt.Sprintf("add proxy server route: %v", err),
			HostID:  spec.HostID,
		}
	}
	sp.logger.Info("proxy server route added", "host_id", spec.HostID, "server_ip", serverIP, "gateway", hostIP)

	// Step 8: generate sing-box config
	configJSON, err := buildSingBoxConfig(*spec.Egress.Proxy)
	if err != nil {
		return &NetworkError{
			Type:    ErrTunnelSetupFailed,
			Message: fmt.Sprintf("build sing-box config: %v", err),
			HostID:  spec.HostID,
		}
	}

	// Step 9: write config to host-local path
	configPath, err := writeSingBoxConfig(spec.HostID, configJSON)
	if err != nil {
		return err
	}

	// Step 10: start sing-box in container's network namespace only
	if err := startSingBox(ctx, spec.ContainerPID, configPath); err != nil {
		return &NetworkError{
			Type:    ErrTunnelSetupFailed,
			Message: fmt.Sprintf("start sing-box: %v", err),
			HostID:  spec.HostID,
		}
	}
	sp.logger.Info("sing-box started", "host_id", spec.HostID)

	// Step 11: wait for tun0 interface to appear
	if err := waitForTun0(ctx, spec.ContainerPID, 10*time.Second); err != nil {
		return &NetworkError{
			Type:    ErrTunnelSetupFailed,
			Message: fmt.Sprintf("wait for tun0: %v", err),
			HostID:  spec.HostID,
		}
	}
	sp.logger.Info("tun0 interface ready", "host_id", spec.HostID)

	// Step 12: configure container DNS
	if err := ConfigureContainerDNS(pid, spec.Egress.Proxy.DNSServer); err != nil {
		return err
	}
	sp.logger.Info("container DNS configured", "host_id", spec.HostID, "dns_server", spec.Egress.Proxy.DNSServer)

	// Step 13: resolve container interface indexes for firewall rules
	tunIdx, loIdx, mgmtIdx, err := resolveProxyIfIndexes(containerNS, hostNS)
	if err != nil {
		return &NetworkError{
			Type:    ErrTunnelSetupFailed,
			Message: fmt.Sprintf("resolve container interfaces: %v", err),
			HostID:  spec.HostID,
		}
	}

	// Step 14: apply proxy firewall rules
	if err := ApplyProxyFirewallRules(containerNS, tunIdx, loIdx, mgmtIdx, net.ParseIP(serverIP), serverPort); err != nil {
		return err
	}
	sp.logger.Info("proxy firewall rules applied", "host_id", spec.HostID)

	// Step 15: triple verification
	result, verifyErr := VerifyNetworkIntegrity(ctx, spec.ContainerPID, *spec.Egress)
	if verifyErr != nil {
		sp.logger.Error("network verification failed",
			"host_id", spec.HostID,
			"egress_ip_match", result.EgressIPMatch,
			"dns_correct", result.DNSCorrect,
			"leak_blocked", result.LeakBlocked,
			"actual_egress_ip", result.ActualEgressIP,
			"actual_dns", result.ActualDNS,
		)
		if netErr, ok := verifyErr.(*NetworkError); ok {
			netErr.HostID = spec.HostID
		}
		return verifyErr
	}

	sp.logger.Info("network verification passed",
		"host_id", spec.HostID,
		"egress_ip", result.ActualEgressIP,
		"dns_server", result.ActualDNS,
	)

	return nil
}

// CleanupHost removes host-side network artifacts for a proxy-mode container.
// Errors are logged but not returned to prevent cleanup failures from
// blocking container rebuild operations.
func (sp *SingBoxProvider) CleanupHost(_ context.Context, spec HostNetworkSpec) error {
	mgmtName := fmt.Sprintf("mgmt-%s", truncateID(spec.HostID, 8))
	if link, err := netlink.LinkByName(mgmtName); err == nil {
		if err := netlink.LinkDel(link); err != nil {
			sp.logger.Warn("failed to delete management veth", "name", mgmtName, "error", err)
		}
	}

	killCmd := exec.Command("pkill", "-f", fmt.Sprintf("sing-box.*%s", spec.HostID))
	if err := killCmd.Run(); err != nil {
		sp.logger.Warn("failed to kill sing-box process", "host_id", spec.HostID, "error", err)
	}

	configDir := singBoxConfigDir(spec.HostID)
	if err := os.RemoveAll(configDir); err != nil {
		sp.logger.Warn("failed to remove sing-box config", "host_id", spec.HostID, "error", err)
	}

	return nil
}

func ensureIPForwarding(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "sysctl", "-w", "net.ipv4.ip_forward=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("enable ip forwarding: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func ensureHostMasquerade(ctx context.Context) error {
	check := exec.CommandContext(ctx, "iptables", "-t", "nat", "-C", "POSTROUTING",
		"-s", "10.99.0.0/16", "-j", "MASQUERADE")
	if check.Run() == nil {
		return nil
	}
	add := exec.CommandContext(ctx, "iptables", "-t", "nat", "-A", "POSTROUTING",
		"-s", "10.99.0.0/16", "-j", "MASQUERADE")
	if out, err := add.CombinedOutput(); err != nil {
		return fmt.Errorf("add masquerade rule: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func mustParseUint(b []byte) uint64 {
	s := string(b)
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	v, _ := strconv.ParseUint(s, 10, 64)
	return v
}

// addProxyServerRoute adds a host route for the proxy server IP via the
// management veth gateway inside the container network namespace.
func addProxyServerRoute(containerNS, hostNS netns.NsHandle, proxyServerIP string, mgmtGateway string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := netns.Set(containerNS); err != nil {
		return fmt.Errorf("set container netns: %w", err)
	}
	defer netns.Set(hostNS) //nolint:errcheck

	mgmtLink, err := netlink.LinkByName("mgmt0")
	if err != nil {
		return fmt.Errorf("find mgmt0: %w", err)
	}

	dstIP := net.ParseIP(proxyServerIP)
	gwIP := net.ParseIP(mgmtGateway)

	route := &netlink.Route{
		Dst:       &net.IPNet{IP: dstIP, Mask: net.CIDRMask(32, 32)},
		Gw:        gwIP,
		LinkIndex: mgmtLink.Attrs().Index,
	}

	if err := netlink.RouteAdd(route); err != nil {
		return fmt.Errorf("add route %s/32 via %s dev mgmt0: %w", proxyServerIP, mgmtGateway, err)
	}

	return nil
}

// singBoxConfigDir returns the host-local directory for a container's sing-box config.
func singBoxConfigDir(hostID string) string {
	return filepath.Join("/var/lib/cloud-cli-proxy/sing-box", hostID)
}

// writeSingBoxConfig writes the sing-box JSON configuration to a host-local
// directory, keeping it out of the user container's filesystem.
func writeSingBoxConfig(hostID string, config []byte) (string, error) {
	dir := singBoxConfigDir(hostID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", &NetworkError{
			Type:    ErrTunnelSetupFailed,
			Message: fmt.Sprintf("create sing-box config dir: %v", err),
		}
	}

	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, config, 0o644); err != nil {
		return "", &NetworkError{
			Type:    ErrTunnelSetupFailed,
			Message: fmt.Sprintf("write sing-box config: %v", err),
		}
	}

	return configPath, nil
}

// startSingBox launches the sing-box process in the container's network
// namespace only (-n). The binary and config stay on the host side.
func startSingBox(ctx context.Context, containerPID uint32, configPath string) error {
	pidStr := strconv.FormatUint(uint64(containerPID), 10)
	cmd := exec.CommandContext(ctx,
		"nsenter", "-t", pidStr, "-n", "--",
		"/usr/local/bin/sing-box", "run", "-c", configPath,
	)

	if err := cmd.Start(); err != nil {
		return err
	}

	go cmd.Wait() //nolint:errcheck

	return nil
}

// waitForTun0 polls the container network namespace until the tun0 interface
// appears or the timeout expires.
func waitForTun0(ctx context.Context, containerPID uint32, timeout time.Duration) error {
	pidStr := strconv.FormatUint(uint64(containerPID), 10)
	deadline := time.After(timeout)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("tun0 not ready after %s", timeout)
		case <-ticker.C:
			cmd := exec.Command("nsenter", "-t", pidStr, "-n", "--", "ip", "link", "show", "tun0")
			if cmd.Run() == nil {
				return nil
			}
		}
	}
}

// resolveProxyIfIndexes looks up interface indexes inside the container
// namespace for tun0, loopback, and management veth.
func resolveProxyIfIndexes(containerNS, hostNS netns.NsHandle) (tunIdx, loIdx, mgmtIdx int, err error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := netns.Set(containerNS); err != nil {
		return 0, 0, 0, err
	}
	defer netns.Set(hostNS) //nolint:errcheck

	tunLink, err := netlink.LinkByName("tun0")
	if err != nil {
		return 0, 0, 0, fmt.Errorf("find tun0: %w", err)
	}

	loLink, err := netlink.LinkByName("lo")
	if err != nil {
		return 0, 0, 0, fmt.Errorf("find lo: %w", err)
	}

	mgmtLink, err := netlink.LinkByName("mgmt0")
	if err != nil {
		return 0, 0, 0, fmt.Errorf("find mgmt0: %w", err)
	}

	return tunLink.Attrs().Index, loLink.Attrs().Index, mgmtLink.Attrs().Index, nil
}
