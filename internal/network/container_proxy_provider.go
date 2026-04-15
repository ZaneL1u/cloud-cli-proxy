package network

import (
	"context"
	"fmt"
	"hash/fnv"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
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

	img := gatewayImage()
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

	// 断开 Worker 原来的 bridge 网络，让隔离网络成为唯一出口
	_ = exec.CommandContext(ctx, "docker", "network", "disconnect", "-f", "bridge", workerName).Run()

	// 等待隔离网络的接口就绪（disconnect 后可能有短暂延迟）
	time.Sleep(1 * time.Second)

	if err := configureWorkerEgress(ctx, workerName, gwIP, workerIP); err != nil {
		p.teardownGateway(ctx, hostID)
		return fmt.Errorf("gateway: configure worker routes/DNS: %w", err)
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

	if cpID, _ := os.Hostname(); cpID != "" {
		_ = exec.CommandContext(ctx, "docker", "network", "disconnect", "-f", netName, cpID).Run()
	}
	_ = exec.CommandContext(ctx, "docker", "network", "disconnect", "-f", netName, workerName).Run()
	_ = exec.CommandContext(ctx, "docker", "rm", "-f", gwName).Run()
	_ = exec.CommandContext(ctx, "docker", "network", "rm", netName).Run()
	_ = os.RemoveAll(gatewayConfigDir(hostID))
}

func gatewayImage() string {
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
	// workerIP 例如 "172.25.42.3"，从中提取网段前缀来匹配正确的接口
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
ip route del default 2>/dev/null || true
ip route add default via %s dev "$DEV"
echo 'nameserver 8.8.8.8' > /etc/resolv.conf
`, workerIP, workerIP, gwIP)

	cmd := exec.CommandContext(ctx, "docker", "exec", workerName, "sh", "-c", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}
