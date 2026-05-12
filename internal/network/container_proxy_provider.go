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
	"strings"
	"time"
)

const gatewayTPProxyPort = 7892

// resolvConfContent 是 worker 容器 /etc/resolv.conf 的固定内容（v3.5 Phase 45 Plan 02）。
// 唯一 nameserver 指向 sing-box gateway 的 tun0 (172.19.0.1)；ndots:0 +
// single-request-reopen 避免无谓的 search-domain 查询与 SERVFAIL 复用问题。
// 必须以换行结尾以便 grep / 行匹配。
const resolvConfContent = "nameserver 172.19.0.1\noptions ndots:0 single-request-reopen\n"

// nsswitchConfContent 是 worker 容器 /etc/nsswitch.conf 的固定内容。
// hosts 行严格限定 "files dns"（禁用 mdns / myhostname / wins / nis_plus），
// 其余字段沿用 Linux 标准默认以保证 passwd / group / shadow 等查询正常工作。
// 用 "+" 字符串拼接避免 raw-string 缩进陷阱。
const nsswitchConfContent = "passwd:         compat\n" +
	"group:          compat\n" +
	"shadow:         compat\n" +
	"gshadow:        files\n" +
	"hosts:          files dns\n" +
	"networks:       files\n" +
	"protocols:      db files\n" +
	"services:       db files\n" +
	"ethers:         db files\n" +
	"rpc:            db files\n" +
	"netgroup:       nis\n"

type ContainerProxyProvider struct {
	logger *slog.Logger
}

func NewContainerProxyProvider(logger *slog.Logger) *ContainerProxyProvider {
	return &ContainerProxyProvider{logger: logger}
}

// PrepareGateway 在 worker 容器 docker create / start 之前调用。
// 它承担「create network + start gateway + 等 sing-box healthy + 写所有
// bind-mount 源文件（config.json / rule-set placeholder / resolv.conf /
// nsswitch.conf）」的职责，保证 worker 容器一旦 docker start，bind mount
// 引用的文件存在且 sing-box tun0 (172.19.0.1) 已经监听 DNS。
func (p *ContainerProxyProvider) PrepareGateway(ctx context.Context, spec HostNetworkSpec) error {
	if spec.Egress == nil {
		p.logger.Info("container-proxy: no egress config, skipping gateway prepare", "host_id", spec.HostID)
		return nil
	}
	if spec.Egress.Proxy == nil {
		p.logger.Warn("container-proxy: no proxy config, skipping gateway prepare", "host_id", spec.HostID)
		return nil
	}

	hostID := spec.HostID
	netName := networkName(hostID)
	gwName := gatewayContainerName(hostID)

	third := subnetThirdOctet(hostID)
	subnet := fmt.Sprintf("10.99.%d.0/24", third)
	bridgeGW := fmt.Sprintf("10.99.%d.1", third)
	gwIP := fmt.Sprintf("10.99.%d.2", third)

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

	p.teardownGateway(ctx, hostID)

	configDir := GatewayConfigDir(hostID)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return fmt.Errorf("gateway: mkdir config dir: %w", err)
	}
	configPath := filepath.Join(configDir, "config.json")
	if err := os.WriteFile(configPath, configJSON, 0o644); err != nil {
		return fmt.Errorf("gateway: write config: %w", err)
	}

	// Phase 45 Plan 01：sing-box 1.11+ 通过 route.rule_set[type=local] 引用这两个文件；
	// 内容为 v3 schema 空规则集占位（ruleSetPlaceholder 常量），Phase 47 由 host-agent 原子替换。
	cidrsPath := filepath.Join(configDir, "whitelist-cidrs.json")
	domainsPath := filepath.Join(configDir, "whitelist-domains.json")
	if err := os.WriteFile(cidrsPath, []byte(ruleSetPlaceholder), 0o644); err != nil {
		return fmt.Errorf("gateway: write whitelist-cidrs placeholder: %w", err)
	}
	if err := os.WriteFile(domainsPath, []byte(ruleSetPlaceholder), 0o644); err != nil {
		return fmt.Errorf("gateway: write whitelist-domains placeholder: %w", err)
	}

	// Phase 45 Plan 02：容器 DNS 入口锁源文件写盘。
	// worker 容器以 ro bind mount 把这两个文件挂入 /etc/resolv.conf 与 /etc/nsswitch.conf；
	// 必须在 worker docker create 之前写完，否则 buildCreateArgs 引用的源路径不存在。
	if err := WriteContainerDNSConfig(hostID); err != nil {
		return fmt.Errorf("gateway: write container DNS config: %w", err)
	}

	if err := dockerNetworkCreate(ctx, netName, subnet, bridgeGW); err != nil {
		return fmt.Errorf("gateway: create network: %w", err)
	}

	img := GatewayImage()
	if err := dockerRunGateway(ctx, gwName, netName, gwIP, serverIP, configPath, cidrsPath, domainsPath, img); err != nil {
		p.teardownGateway(ctx, hostID)
		return fmt.Errorf("gateway: start gateway container: %w", err)
	}

	if err := dockerNetworkConnect(ctx, "bridge", gwName, ""); err != nil {
		p.teardownGateway(ctx, hostID)
		return fmt.Errorf("gateway: connect gateway to bridge: %w", err)
	}

	if err := waitGatewayHealthy(ctx, gwName); err != nil {
		p.teardownGateway(ctx, hostID)
		return err
	}

	p.logger.Info("container-proxy: gateway prepared (network + sing-box healthy + DNS sources written)",
		"host_id", hostID,
		"network", netName,
		"gateway", gwName,
		"gateway_ip", gwIP,
		"image", img,
	)
	return nil
}

// PrepareHost 在 worker 容器 docker start 之后调用。
// Phase 45 Plan 02 重构后只承担：
//   - 把 worker 容器 connect 到 gateway 自定义 bridge 网络
//   - 从 worker 容器中断开默认 docker bridge（避免双 default route）
//   - 在 worker netns 内 configure 默认路由
//   - apply worker firewall（fail-closed nft 兜底，留给 Phase 47）
//   - 跑 verifyWorkerNetwork（egress IP / DNS / leak block 三检）
//   - 把控制面（如果有 hostname）接入隔离网络以便 SSH 转发
//
// gateway 容器与所有 bind-mount 源文件由 PrepareGateway 在 worker 容器创建
// 之前完成；PrepareHost 内不再有 dockerNetworkCreate / dockerRunGateway /
// waitGatewayHealthy / WriteContainerDNSConfig / writeRuleSetPlaceholder /
// os.WriteFile(configPath,...) 任何调用。
func (p *ContainerProxyProvider) PrepareHost(ctx context.Context, spec HostNetworkSpec) error {
	if spec.Egress == nil {
		p.logger.Info("container-proxy: no egress config, skipping host wiring", "host_id", spec.HostID)
		return nil
	}
	if spec.Egress.Proxy == nil {
		p.logger.Warn("container-proxy: no proxy config, skipping host wiring", "host_id", spec.HostID)
		return nil
	}

	hostID := spec.HostID
	workerName := workerContainerName(hostID)
	netName := networkName(hostID)

	third := subnetThirdOctet(hostID)
	bridgeGW := fmt.Sprintf("10.99.%d.1", third)
	gwIP := fmt.Sprintf("10.99.%d.2", third)
	workerIP := fmt.Sprintf("10.99.%d.3", third)

	if err := dockerNetworkConnect(ctx, netName, workerName, workerIP); err != nil {
		p.teardownGateway(ctx, hostID)
		return fmt.Errorf("gateway: connect worker to network: %w", err)
	}

	_ = exec.CommandContext(ctx, "docker", "network", "disconnect", "-f", "bridge", workerName).Run()
	time.Sleep(1 * time.Second)

	defaultGW := gwIP
	if runtime.GOOS != "linux" {
		defaultGW = gwIP
	}
	if err := configureWorkerEgress(ctx, workerName, defaultGW, workerIP); err != nil {
		p.teardownGateway(ctx, hostID)
		return fmt.Errorf("gateway: configure worker routes: %w", err)
	}

	if err := applyWorkerFirewall(ctx, workerName, gwIP, bridgeGW); err != nil {
		p.teardownGateway(ctx, hostID)
		return fmt.Errorf("gateway: apply worker firewall: %w", err)
	}
	p.logger.Info("container-proxy: worker firewall rules applied", "host_id", hostID)

	result, verifyErr := verifyWorkerNetwork(ctx, workerName, *spec.Egress)
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

	if cpID, _ := os.Hostname(); cpID != "" {
		if err := dockerNetworkConnect(ctx, netName, cpID, ""); err != nil {
			p.logger.Warn("container-proxy: connect control-plane to isolated network failed (VNC may not work)",
				"host_id", hostID, "error", err)
		}
	}

	p.logger.Info("container-proxy: sidecar gateway ready",
		"host_id", hostID,
		"network", netName,
		"gateway_ip", gwIP,
		"worker_ip", workerIP,
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

	cleanupWorkerFirewall(ctx, workerName)

	if cpID, _ := os.Hostname(); cpID != "" {
		_ = exec.CommandContext(ctx, "docker", "network", "disconnect", "-f", netName, cpID).Run()
	}
	_ = exec.CommandContext(ctx, "docker", "network", "disconnect", "-f", netName, workerName).Run()
	_ = exec.CommandContext(ctx, "docker", "rm", "-f", gwName).Run()
	_ = exec.CommandContext(ctx, "docker", "network", "rm", netName).Run()
	_ = os.RemoveAll(GatewayConfigDir(hostID))
}

func GatewayImage() string {
	if v := os.Getenv("CLOUD_CLI_PROXY_GATEWAY_IMAGE"); v != "" {
		return v
	}
	return "cloud-cli-proxy-sing-gateway:local"
}

// GatewayConfigDir 返回 host 专属的 sing-box 配置 / placeholder / DNS 源文件目录。
// 路径规则：<DATA_DIR>/gateway/<host_id>/。Phase 45 Plan 02 起导出为包级函数，
// 便于 internal/runtime/tasks 等外部包在 worker 容器 docker create 之前拼出
// ro bind mount 的源路径。
func GatewayConfigDir(hostID string) string {
	base := os.Getenv("DATA_DIR")
	if base == "" {
		base = "/var/lib/cloud-cli-proxy"
	}
	return filepath.Join(base, "gateway", hostID)
}

// WriteContainerDNSConfig 把 v3.5 容器 DNS 入口锁的两个源文件写到
// <DATA_DIR>/gateway/<host_id>/resolv.conf 与 nsswitch.conf。
// 这两个文件随后由 worker 容器以 ro bind mount 挂入 /etc/resolv.conf 与
// /etc/nsswitch.conf。必须在 worker docker create 之前调用。
func WriteContainerDNSConfig(hostID string) error {
	dir := GatewayConfigDir(hostID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir container dns config dir: %w", err)
	}
	resolvPath := filepath.Join(dir, "resolv.conf")
	if err := os.WriteFile(resolvPath, []byte(resolvConfContent), 0o644); err != nil {
		return fmt.Errorf("write resolv.conf: %w", err)
	}
	nsswitchPath := filepath.Join(dir, "nsswitch.conf")
	if err := os.WriteFile(nsswitchPath, []byte(nsswitchConfContent), 0o644); err != nil {
		return fmt.Errorf("write nsswitch.conf: %w", err)
	}
	return nil
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

func dockerRunGateway(ctx context.Context, gwName, netName, gwIP, proxyServerIP, configPath, cidrsPath, domainsPath, image string) error {
	args := []string{
		"run", "-d",
		"--name", gwName,
		"--network", netName,
		"--ip", gwIP,
		"--cap-add", "NET_ADMIN",
		"--device", "/dev/net/tun:/dev/net/tun",
		"--sysctl", "net.ipv4.ip_forward=1",
		"-v", configPath + ":/etc/sing-box/config.json:ro",
		"-v", cidrsPath + ":/etc/sing-box/whitelist-cidrs.json:ro",
		"-v", domainsPath + ":/etc/sing-box/whitelist-domains.json:ro",
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

func configureWorkerEgress(ctx context.Context, workerName, defaultGW, workerIP string) error {
	const maxRetry = 3
	var lastErr error
	for attempt := 1; attempt <= maxRetry; attempt++ {
		if err := tryConfigureWorkerEgress(ctx, workerName, defaultGW, workerIP); err == nil {
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

func tryConfigureWorkerEgress(ctx context.Context, workerName, defaultGW, workerIP string) error {
	// Phase 45 Plan 02：/etc/resolv.conf 已被 PrepareGateway 写盘 + worker docker
	// create 时 ro bind mount 接管，这里**不再** docker exec 写 resolv.conf；
	// 任何写盘尝试都会被 ro mount 拒绝。本脚本只负责 default route。
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
  gw=$(echo "$line" | grep -oP 'via \K[^ ]+' || true)
  dev=$(echo "$line" | grep -oP 'dev \K[^ ]+' || true)
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
`, workerIP, workerIP, defaultGW, defaultGW)

	cmd := exec.CommandContext(ctx, "docker", "exec", workerName, "sh", "-c", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}
