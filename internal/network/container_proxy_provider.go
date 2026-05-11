//go:build linux

package network

import (
	"context"
	"fmt"
	"hash/fnv"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/vishvananda/netns"
)

const gatewayTPProxyPort = 7892

// ContainerProxyProvider wires each worker container to a sidecar gateway that
// runs sing-box (tproxy + iptables). The worker image stays proxy-unaware.
type ContainerProxyProvider struct {
	logger *slog.Logger
}

func NewContainerProxyProvider(logger *slog.Logger) *ContainerProxyProvider {
	return &ContainerProxyProvider{logger: logger}
}

func (p *ContainerProxyProvider) PrepareHost(ctx context.Context, spec HostNetworkSpec) error {
	if spec.Egress == nil {
		p.logger.Info("container-proxy: no egress config, skipping", "host_id", spec.HostID)
		return nil
	}

	if spec.Egress.Proxy == nil {
		p.logger.Warn("container-proxy: no proxy config, skipping network setup", "host_id", spec.HostID)
		return nil
	}

	hostID := spec.HostID
	workerName := workerContainerName(hostID)
	netName := networkName(hostID)
	gwName := gatewayContainerName(hostID)

	third := subnetThirdOctet(hostID)
	subnet := fmt.Sprintf("10.99.%d.0/24", third)
	bridgeGW := fmt.Sprintf("10.99.%d.1", third)
	gwIP := fmt.Sprintf("10.99.%d.2", third)
	workerIP := fmt.Sprintf("10.99.%d.3", third)

	proxyRaw := spec.Egress.Proxy.OutboundConfig
	serverIP, _, err := extractProxyServer(proxyRaw)
	if err != nil {
		return fmt.Errorf("gateway: resolve proxy server: %w", err)
	}

	dnsServer := spec.Egress.Proxy.DNSServer

	configJSON, err := buildGatewaySingBoxConfig(proxyRaw, dnsServer, serverIP)
	if err != nil {
		return fmt.Errorf("gateway: build sing-box config: %w", err)
	}

	// Clean up any previous attempt for this host (会删配置目录，必须在写入之前)
	p.teardownGateway(ctx, hostID)

	configDir := gatewayConfigDir(hostID)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return fmt.Errorf("gateway: mkdir config dir: %w", err)
	}
	configPath := filepath.Join(configDir, "config.json")
	if err := os.WriteFile(configPath, configJSON, 0o644); err != nil {
		return fmt.Errorf("gateway: write config: %w", err)
	}

	if err := dockerNetworkCreate(ctx, netName, subnet, bridgeGW); err != nil {
		return fmt.Errorf("gateway: create network: %w", err)
	}

	img := GatewayImage()
	if err := dockerRunGateway(ctx, gwName, netName, gwIP, serverIP, configPath, img); err != nil {
		p.teardownGateway(ctx, hostID)
		return fmt.Errorf("gateway: start gateway container: %w", err)
	}

	// 网关也需要 bridge 网络才能访问互联网（连上游代理服务器）
	// eth0 = 隔离网络（TPROXY 只抓 eth0），eth1 = bridge（出互联网）
	if err := dockerNetworkConnect(ctx, "bridge", gwName, ""); err != nil {
		p.teardownGateway(ctx, hostID)
		return fmt.Errorf("gateway: connect gateway to bridge: %w", err)
	}

	if err := waitGatewayHealthy(ctx, gwName); err != nil {
		p.teardownGateway(ctx, hostID)
		return err
	}

	if err := dockerNetworkConnect(ctx, netName, workerName, workerIP); err != nil {
		p.teardownGateway(ctx, hostID)
		return fmt.Errorf("gateway: connect worker to network: %w", err)
	}

	// 所有平台统一断开 Worker 的 bridge 网络，防止 restart 后 default route 被 bridge 覆盖。
	// 这是防 IP 泄漏的关键机制：不断开 bridge 则容器内流量可通过 bridge 直接出网。
	// Linux 端口映射通过宿主机 iptables DNAT 到隔离网络 IP 实现（见 setupPortForwarding）。
	_ = exec.CommandContext(ctx, "docker", "network", "disconnect", "-f", "bridge", workerName).Run()

	// 等待隔离网络的接口就绪（disconnect 后可能有短暂延迟）
	time.Sleep(1 * time.Second)

	// 默认路由指向 gateway，所有流量必须经过 sing-box 代理隧道。
	// 端口映射回包由宿主机 SNAT 后源 IP 变为 bridgeGW，worker 防火墙允许来自 bridgeGW 的流量。
	// macOS: Docker Desktop vpnkit 处理端口映射。
	defaultGW := gwIP
	if runtime.GOOS != "linux" {
		defaultGW = gwIP
	}
	if err := configureWorkerEgress(ctx, workerName, defaultGW, workerIP); err != nil {
		p.teardownGateway(ctx, hostID)
		return fmt.Errorf("gateway: configure worker routes/DNS: %w", err)
	}

	// 在 worker 容器内设置严格 nftables 防火墙，确保所有流量必须经过 gateway。
	// 规则基于接口索引匹配，防止 Docker reconnect bridge 后新接口被滥用。
	if runtime.GOOS == "linux" {
		var allowedPorts []uint16
		for _, pm := range spec.PortMappings {
			if pm.ContainerPort > 0 {
				allowedPorts = append(allowedPorts, uint16(pm.ContainerPort))
			}
		}

		workerNS, workerPID, err := GetContainerNetNS(workerName)
		if err != nil {
			p.teardownGateway(ctx, hostID)
			return fmt.Errorf("gateway: get worker netns: %w", err)
		}
		defer workerNS.Close()

		if err := ApplyWorkerFirewallRules(workerNS, net.ParseIP(gwIP), net.ParseIP(bridgeGW), 22, allowedPorts); err != nil {
			p.teardownGateway(ctx, hostID)
			return fmt.Errorf("gateway: apply worker firewall: %w", err)
		}
		p.logger.Info("container-proxy: worker firewall rules applied", "host_id", hostID)

		// 网络验证：验证 egress IP、DNS、泄漏阻断
		result, verifyErr := VerifyNetworkIntegrity(ctx, workerPID, *spec.Egress)
		if verifyErr != nil {
			p.logger.Error("container-proxy: network verification failed",
				"host_id", hostID,
				"egress_ip_match", result.EgressIPMatch,
				"dns_correct", result.DNSCorrect,
				"leak_blocked", result.LeakBlocked,
				"actual_egress_ip", result.ActualEgressIP,
				"actual_dns", result.ActualDNS,
			)
			p.teardownGateway(ctx, hostID)
			if netErr, ok := verifyErr.(*NetworkError); ok {
				netErr.HostID = hostID
			}
			return verifyErr
		}
		p.logger.Info("container-proxy: network verification passed",
			"host_id", hostID,
			"egress_ip", result.ActualEgressIP,
			"dns_server", result.ActualDNS,
		)
	}

	// 宿主机 iptables 路由规则（端口映射 DNAT + SNAT + 策略路由到 gateway）。
	// 仅 Linux 有效；macOS Docker Desktop 由 vpnkit 处理。
	if len(spec.PortMappings) > 0 {
		if err := ensurePortMapChain(ctx); err != nil {
			p.teardownGateway(ctx, hostID)
			return fmt.Errorf("gateway: setup portmap chain: %w", err)
		}
		if err := setupPortForwarding(ctx, hostID, bridgeGW, gwIP, spec.PortMappings); err != nil {
			p.teardownGateway(ctx, hostID)
			return fmt.Errorf("gateway: setup port forwarding: %w", err)
		}
	}


	if cpID, _ := os.Hostname(); cpID != "" {
		if err := dockerNetworkConnect(ctx, netName, cpID, ""); err != nil {
			p.logger.Warn("container-proxy: connect control-plane to isolated network failed (VNC may not work)",
				"host_id", hostID, "error", err)
		}
	}

	p.logger.Info("container-proxy: sidecar gateway ready",
		"host_id", hostID,
		"network", netName,
		"gateway", gwName,
		"gateway_ip", gwIP,
		"worker_ip", workerIP,
		"image", img,
		"tproxy_port", gatewayTPProxyPort,
	)
	return nil
}

func (p *ContainerProxyProvider) CleanupHost(ctx context.Context, spec HostNetworkSpec) error {
	p.teardownGateway(ctx, spec.HostID)
	return nil
}

func (p *ContainerProxyProvider) teardownGateway(ctx context.Context, hostID string) {
	netName := networkName(hostID)
	gwName := gatewayContainerName(hostID)
	workerName := workerContainerName(hostID)

	// 先清理 worker 内部防火墙规则（worker 容器若仍在运行）
	if runtime.GOOS == "linux" {
		if pidOut, err := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.State.Pid}}", workerName).Output(); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(pidOut))); err == nil && pid > 0 {
				if ns, err := netns.GetFromPid(pid); err == nil {
					_ = CleanupWorkerFirewallRules(ns)
					ns.Close()
				}
			}
		}
	}

	// 清理宿主机 iptables 端口转发规则
	teardownPortForwarding(ctx, hostID)

	if cpID, _ := os.Hostname(); cpID != "" {
		_ = exec.CommandContext(ctx, "docker", "network", "disconnect", "-f", netName, cpID).Run()
	}
	_ = exec.CommandContext(ctx, "docker", "network", "disconnect", "-f", netName, workerName).Run()
	_ = exec.CommandContext(ctx, "docker", "rm", "-f", gwName).Run()
	_ = exec.CommandContext(ctx, "docker", "network", "rm", netName).Run()
	_ = os.RemoveAll(gatewayConfigDir(hostID))
}

func GatewayImage() string {
	if v := os.Getenv("CLOUD_CLI_PROXY_GATEWAY_IMAGE"); v != "" {
		return v
	}
	return "cloud-cli-proxy-sing-gateway:local"
}

func gatewayConfigDir(hostID string) string {
	base := os.Getenv("DATA_DIR")
	if base == "" {
		base = "/var/lib/cloud-cli-proxy"
	}
	return filepath.Join(base, "gateway", hostID)
}

func networkName(hostID string) string {
	return "cloudproxy-net-" + hostID
}

func gatewayContainerName(hostID string) string {
	return "cloudproxy-gw-" + hostID
}

func workerContainerName(hostID string) string {
	return "cloudproxy-" + hostID
}

func subnetThirdOctet(hostID string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(hostID))
	return int(h.Sum32()%200) + 20
}

func dockerNetworkCreate(ctx context.Context, name, subnet, gateway string) error {
	cmd := exec.CommandContext(ctx, "docker", "network", "create",
		"--driver", "bridge",
		"--subnet", subnet,
		"--gateway", gateway,
		name,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func dockerRunGateway(ctx context.Context, gwName, netName, gwIP, proxyServerIP, configPath, image string) error {
	args := []string{
		"run", "-d",
		"--name", gwName,
		"--network", netName,
		"--ip", gwIP,
		"--cap-add", "NET_ADMIN",
		"--device", "/dev/net/tun:/dev/net/tun",
		"--sysctl", "net.ipv4.ip_forward=1",
		"-v", configPath + ":/etc/sing-box/config.json:ro",
		"--label", "cloud-cli-proxy.role=gateway",
		"--label", "cloud-cli-proxy.managed=true",
		"--restart", "no",
		image,
	}
	cmd := exec.CommandContext(ctx, "docker", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func dockerNetworkConnect(ctx context.Context, netName, containerName, staticIP string) error {
	args := []string{"network", "connect"}
	if staticIP != "" {
		args = append(args, "--ip", staticIP)
	}
	args = append(args, netName, containerName)
	cmd := exec.CommandContext(ctx, "docker", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker network connect: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func waitGatewayHealthy(ctx context.Context, gwName string) error {
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		cmd := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.State.Running}}", gwName)
		out, err := cmd.Output()
		if err == nil && strings.TrimSpace(string(out)) == "true" {
			logs, _ := exec.CommandContext(ctx, "docker", "logs", "--tail", "120", gwName).CombinedOutput()
			s := string(logs)
			if strings.Contains(s, "FATAL") || strings.Contains(s, "panic:") {
				return fmt.Errorf("gateway sing-box failed: %s", strings.TrimSpace(s))
			}
			time.Sleep(500 * time.Millisecond)
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	logs, _ := exec.CommandContext(ctx, "docker", "logs", gwName).CombinedOutput()
	return fmt.Errorf("gateway container not healthy in time: %s", strings.TrimSpace(string(logs)))
}

func configureWorkerEgress(ctx context.Context, workerName, gwIP, workerIP string) error {
	const maxRetry = 3
	var lastErr error
	for attempt := 1; attempt <= maxRetry; attempt++ {
		if err := tryConfigureWorkerEgress(ctx, workerName, gwIP, workerIP); err == nil {
			return nil
		} else {
			lastErr = err
			if attempt < maxRetry {
				time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
			}
		}
	}
	return fmt.Errorf("configureWorkerEgress failed after %d attempts: %w", maxRetry, lastErr)
}

func tryConfigureWorkerEgress(ctx context.Context, workerName, gwIP, workerIP string) error {
	// 默认路由指向 gateway 容器（10.99.X.2），所有流量必须经过 sing-box 代理隧道。
	// 端口映射回包由宿主机 SNAT 后源 IP 变为 bridgeGW，worker 防火墙允许来自 bridgeGW 的流量。
	script := fmt.Sprintf(`set -e
# 等待网络接口就绪
for i in 1 2 3 4 5; do
  DEV=$(ip -o addr show | grep '%s' | awk '{print $2}' | head -1)
  [ -n "$DEV" ] && break
  sleep 1
done
if [ -z "$DEV" ]; then
  echo "waiting for interface with IP %s timed out"
  ip -o addr show >&2
  exit 1
fi
# 删除所有现有 default 路由
ip route show default | while read -r line; do
  gw=$(echo "$line" | grep -oP 'via \\K[^ ]+' || true)
  dev=$(echo "$line" | grep -oP 'dev \\K[^ ]+' || true)
  if [ -n "$gw" ] && [ -n "$dev" ]; then
    ip route del default via "$gw" dev "$dev" 2>/dev/null || true
  fi
done
ip route del default 2>/dev/null || true
# 默认路由指向 gateway，所有流量必须经过 sing-box 代理隧道
ip route add default via %s dev "$DEV" metric 0
# 立即 verify
default_route=$(ip route show default | head -1)
echo "$default_route" | grep -q "via %s"
echo 'nameserver 8.8.8.8' > /etc/resolv.conf
`, workerIP, workerIP, gwIP, gwIP)

	cmd := exec.CommandContext(ctx, "docker", "exec", workerName, "sh", "-c", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}


// getWorkerNetNS 通过 docker inspect 获取 worker 容器的 PID，然后获取其网络命名空间。
func getWorkerNetNS(ctx context.Context, workerName string) (netns.NsHandle, error) {
	out, err := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.State.Pid}}", workerName).Output()
	if err != nil {
		return 0, fmt.Errorf("inspect worker %s pid: %w", workerName, err)
	}

	pid, err := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 32)
	if err != nil {
		return 0, fmt.Errorf("parse worker pid: %w", err)
	}
	if pid == 0 {
		return 0, fmt.Errorf("worker %s not running (pid=0)", workerName)
	}

	ns, err := netns.GetFromPid(int(pid))
	if err != nil {
		return 0, fmt.Errorf("get netns from pid %d: %w", pid, err)
	}

	return ns, nil
}

// getWorkerPID 通过 docker inspect 获取 worker 容器的 PID。
func getWorkerPID(ctx context.Context, workerName string) (uint32, error) {
	out, err := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.State.Pid}}", workerName).Output()
	if err != nil {
		return 0, fmt.Errorf("inspect worker %s pid: %w", workerName, err)
	}

	pid, err := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 32)
	if err != nil {
		return 0, fmt.Errorf("parse worker pid: %w", err)
	}
	if pid == 0 {
		return 0, fmt.Errorf("worker %s not running (pid=0)", workerName)
	}

	return uint32(pid), nil
}
