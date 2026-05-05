package http

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	nethttp "net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/net/proxy"
)

const probeImage = "ghcr.io/sagernet/sing-box"

type ProbeResult struct {
	Status   string    `json:"status"`
	TestedAt time.Time `json:"tested_at"`
	Message  string    `json:"message,omitempty"`
	Results  struct {
		Connectivity ConnectivityCheckResult `json:"connectivity"`
		EgressIP     EgressIPCheckResult     `json:"egress_ip"`
		DNSLeak      DNSLeakCheckResult      `json:"dns_leak"`
	} `json:"results"`
}

type ConnectivityCheckResult struct {
	Status    string `json:"status"`
	LatencyMS int64  `json:"latency_ms,omitempty"`
	Error     string `json:"error,omitempty"`
}

type EgressIPCheckResult struct {
	Status  string            `json:"status"`
	IP      string            `json:"ip,omitempty"`
	Sources map[string]string `json:"sources,omitempty"`
	Error   string            `json:"error,omitempty"`
}

type DNSLeakCheckResult struct {
	Status             string   `json:"status"`
	DNSServersDetected []string `json:"dns_servers_detected,omitempty"`
	LocalDNSLeaked     bool     `json:"local_dns_leaked,omitempty"`
	Error              string   `json:"error,omitempty"`
}

type contextDialer interface {
	DialContext(ctx context.Context, network, addr string) (net.Conn, error)
}

func getProxyDialer(ctx context.Context, proxyConfig json.RawMessage) (dialer contextDialer, cleanup func(), err error) {
	var parsed map[string]any
	if err := json.Unmarshal(proxyConfig, &parsed); err != nil {
		return nil, nil, fmt.Errorf("parse proxy_config: %w", err)
	}

	outboundType, _ := parsed["type"].(string)
	server, _ := parsed["server"].(string)
	portF, _ := parsed["server_port"].(float64)
	port := int(portF)

	switch outboundType {
	case "socks":
		var auth *proxy.Auth
		if username, _ := parsed["username"].(string); username != "" {
			password, _ := parsed["password"].(string)
			auth = &proxy.Auth{User: username, Password: password}
		}
		d, dialErr := proxy.SOCKS5("tcp", fmt.Sprintf("%s:%d", server, port), auth, proxy.Direct)
		if dialErr != nil {
			return nil, nil, fmt.Errorf("create SOCKS5 dialer: %w", dialErr)
		}
		cd, ok := d.(contextDialer)
		if !ok {
			return nil, nil, fmt.Errorf("SOCKS5 dialer does not support DialContext")
		}
		return cd, nil, nil

	case "http":
		scheme := "http"
		if tlsCfg, ok := parsed["tls"].(map[string]any); ok {
			if enabled, _ := tlsCfg["enabled"].(bool); enabled {
				scheme = "https"
			}
		}
		proxyURL := &url.URL{
			Scheme: scheme,
			Host:   fmt.Sprintf("%s:%d", server, port),
		}
		if username, _ := parsed["username"].(string); username != "" {
			password, _ := parsed["password"].(string)
			proxyURL.User = url.UserPassword(username, password)
		}
		return &httpProxyDialer{proxyURL: proxyURL}, nil, nil

	case "vmess", "vless", "shadowsocks", "trojan":
		localPort, singboxCleanup, startErr := startLocalSingBox(ctx, proxyConfig)
		if startErr != nil {
			return nil, nil, startErr
		}
		d, dialErr := proxy.SOCKS5("tcp", fmt.Sprintf("127.0.0.1:%d", localPort), nil, proxy.Direct)
		if dialErr != nil {
			singboxCleanup()
			return nil, nil, fmt.Errorf("create SOCKS5 dialer to local sing-box: %w", dialErr)
		}
		cd, ok := d.(contextDialer)
		if !ok {
			singboxCleanup()
			return nil, nil, fmt.Errorf("SOCKS5 dialer does not support DialContext")
		}
		return cd, singboxCleanup, nil

	default:
		return nil, nil, fmt.Errorf("unsupported proxy type: %s", outboundType)
	}
}

type httpProxyDialer struct {
	proxyURL *url.URL
}

func (d *httpProxyDialer) DialContext(ctx context.Context, _ string, addr string) (net.Conn, error) {
	proxyAddr := d.proxyURL.Host
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", proxyAddr)
	if err != nil {
		return nil, fmt.Errorf("connect to HTTP proxy: %w", err)
	}

	connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n", addr, addr)
	if d.proxyURL.User != nil {
		creds := base64.StdEncoding.EncodeToString([]byte(d.proxyURL.User.String()))
		connectReq += fmt.Sprintf("Proxy-Authorization: Basic %s\r\n", creds)
	}
	connectReq += "\r\n"

	if _, err := conn.Write([]byte(connectReq)); err != nil {
		conn.Close()
		return nil, fmt.Errorf("send CONNECT: %w", err)
	}

	br := bufio.NewReader(conn)
	resp, err := nethttp.ReadResponse(br, nil)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("read CONNECT response: %w", err)
	}
	if resp.StatusCode != 200 {
		conn.Close()
		return nil, fmt.Errorf("HTTP proxy CONNECT failed: %s", resp.Status)
	}
	return conn, nil
}

func buildSingBoxConfig(proxyConfig json.RawMessage, listenAddr string, listenPort int) ([]byte, error) {
	var outbound map[string]any
	if err := json.Unmarshal(proxyConfig, &outbound); err != nil {
		return nil, fmt.Errorf("parse proxy config: %w", err)
	}
	outbound["tag"] = "proxy-out"

	if tlsCfg, ok := outbound["tls"].(map[string]any); ok {
		if realityCfg, ok := tlsCfg["reality"].(map[string]any); ok {
			if enabled, _ := realityCfg["enabled"].(bool); enabled {
				if _, hasUtls := tlsCfg["utls"]; !hasUtls {
					tlsCfg["utls"] = map[string]any{"enabled": true, "fingerprint": "chrome"}
				}
			}
		}
	}

	outboundJSON, err := json.Marshal(outbound)
	if err != nil {
		return nil, fmt.Errorf("marshal outbound: %w", err)
	}

	config := map[string]any{
		"log": map[string]any{"level": "error"},
		"inbounds": []map[string]any{
			{"type": "socks", "tag": "socks-in", "listen": listenAddr, "listen_port": listenPort},
		},
		"outbounds": []json.RawMessage{outboundJSON},
	}
	return json.MarshalIndent(config, "", "  ")
}

func startLocalSingBox(ctx context.Context, proxyConfig json.RawMessage) (port int, cleanup func(), err error) {
	listener, listenErr := net.Listen("tcp", "127.0.0.1:0")
	if listenErr != nil {
		return 0, nil, fmt.Errorf("find free port: %w", listenErr)
	}
	port = listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	if _, lookErr := exec.LookPath("docker"); lookErr == nil {
		return startSingBoxDocker(ctx, proxyConfig, port)
	}
	if _, lookErr := exec.LookPath("sing-box"); lookErr == nil {
		return startSingBoxNative(ctx, proxyConfig, port)
	}
	return 0, nil, fmt.Errorf("需要 Docker 或 sing-box 来测试此协议，两者均未安装")
}

func probeConfigDir() string {
	base := os.Getenv("DATA_DIR")
	if base == "" {
		base = "/var/lib/cloud-cli-proxy"
	}
	return filepath.Join(base, "probe")
}

// CleanupOrphanProbes removes leftover sing-box probe containers and temp
// config files from a previous control-plane run that exited without cleanup.
func CleanupOrphanProbes(logger *slog.Logger) {
	cmd := exec.Command("docker", "ps", "-a",
		"--filter", "name=singbox-probe-",
		"--format", "{{.Names}}")
	out, err := cmd.Output()
	if err != nil {
		return
	}
	for _, name := range strings.Fields(strings.TrimSpace(string(out))) {
		if rmErr := exec.Command("docker", "rm", "-f", name).Run(); rmErr == nil {
			logger.Info("cleaned up orphan probe container", "name", name)
		}
	}

	dir := probeConfigDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "singbox-probe-") && strings.HasSuffix(e.Name(), ".json") {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
	if len(entries) > 0 {
		logger.Info("cleaned up orphan probe config files", "count", len(entries))
	}
}

// hostnameProvider 抽象 os.Hostname，便于测试注入。
var hostnameProvider = os.Hostname

// dockerInspectRunner 抽象 `docker inspect` 调用，便于测试注入。
// 真实实现使用 2s 超时的 exec.CommandContext，输出为容器 ID（或错误）。
var dockerInspectRunner = func(ctx context.Context, name string) ([]byte, error) {
	ctx2, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return exec.CommandContext(ctx2, "docker", "inspect", "--format", "{{.Id}}", name).Output()
}

// resolveProbeNetworking 根据控制面运行环境（容器内 vs 宿主机直跑）选择
// docker run 时使用的网络参数。
//
// 决策规则：
//   - 当 hostname 非空且 `docker inspect <hostname>` 成功并返回非空容器 ID 时，
//     说明控制面就跑在该容器里，复用其 namespace（in-container 模式）。
//   - 其它情况（hostname 为空 / inspect 失败 / 输出为空）一律 fallback
//     到宿主机直跑模式：用 bridge 网络 + 端口映射，让控制面通过
//     127.0.0.1:<port> 访问 sing-box 探针。
func resolveProbeNetworking(ctx context.Context, port int) []string {
	hostname, _ := hostnameProvider()
	if hostname != "" {
		out, err := dockerInspectRunner(ctx, hostname)
		if err == nil && len(strings.TrimSpace(string(out))) > 0 {
			return []string{"--network", "container:" + hostname}
		}
	}
	return []string{"--network", "bridge", "-p", fmt.Sprintf("127.0.0.1:%d:%d", port, port)}
}

func startSingBoxDocker(ctx context.Context, proxyConfig json.RawMessage, port int) (int, func(), error) {
	configJSON, err := buildSingBoxConfig(proxyConfig, "0.0.0.0", port)
	if err != nil {
		return 0, nil, err
	}

	dir := probeConfigDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, nil, fmt.Errorf("create probe config dir: %w", err)
	}
	tmpFile, err := os.CreateTemp(dir, "singbox-probe-*.json")
	if err != nil {
		return 0, nil, fmt.Errorf("create temp config: %w", err)
	}
	if _, err := tmpFile.Write(configJSON); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return 0, nil, fmt.Errorf("write config: %w", err)
	}
	tmpFile.Close()

	containerName := fmt.Sprintf("singbox-probe-%d", port)

	// 确保探针镜像已拉取，避免 docker run 隐式拉取导致超时或报错不清
	pullCmd := exec.CommandContext(ctx, "docker", "pull", probeImage)
	pullOutput, pullErr := pullCmd.CombinedOutput()
	if pullErr != nil {
		os.Remove(tmpFile.Name())
		return 0, nil, fmt.Errorf("拉取探针镜像失败: %s: %w", strings.TrimSpace(string(pullOutput)), pullErr)
	}

	netArgs := resolveProbeNetworking(ctx, port)

	args := []string{"run", "-d", "--name", containerName}
	args = append(args, netArgs...)
	args = append(args, "-v", tmpFile.Name()+":/etc/sing-box/config.json:ro", probeImage, "run", "-c", "/etc/sing-box/config.json")
	cmd := exec.CommandContext(ctx, "docker", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		os.Remove(tmpFile.Name())
		return 0, nil, fmt.Errorf("start sing-box container: %s: %w", strings.TrimSpace(string(output)), err)
	}

	cleanupFn := func() {
		rmCmd := exec.Command("docker", "rm", "-f", containerName)
		rmCmd.Run()
		os.Remove(tmpFile.Name())
	}

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		conn, dialErr := net.DialTimeout("tcp", addr, 300*time.Millisecond)
		if dialErr == nil {
			conn.Close()
			return port, cleanupFn, nil
		}
		time.Sleep(300 * time.Millisecond)
	}

	logsCmd := exec.Command("docker", "logs", containerName)
	logsOut, _ := logsCmd.CombinedOutput()
	cleanupFn()
	return 0, nil, fmt.Errorf("sing-box 启动超时（端口 %d）: %s", port, strings.TrimSpace(string(logsOut)))
}

func startSingBoxNative(ctx context.Context, proxyConfig json.RawMessage, port int) (int, func(), error) {
	configJSON, err := buildSingBoxConfig(proxyConfig, "127.0.0.1", port)
	if err != nil {
		return 0, nil, err
	}

	tmpFile, err := os.CreateTemp("", "singbox-probe-*.json")
	if err != nil {
		return 0, nil, fmt.Errorf("create temp config: %w", err)
	}
	if _, err := tmpFile.Write(configJSON); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return 0, nil, fmt.Errorf("write config: %w", err)
	}
	tmpFile.Close()

	cmd := exec.CommandContext(ctx, "sing-box", "run", "-c", tmpFile.Name())
	if err := cmd.Start(); err != nil {
		os.Remove(tmpFile.Name())
		return 0, nil, fmt.Errorf("start sing-box: %w", err)
	}

	cleanupFn := func() {
		cmd.Process.Kill()
		cmd.Wait()
		os.Remove(tmpFile.Name())
	}

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		conn, dialErr := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if dialErr == nil {
			conn.Close()
			return port, cleanupFn, nil
		}
		time.Sleep(200 * time.Millisecond)
	}

	cleanupFn()
	return 0, nil, fmt.Errorf("sing-box failed to start within 5 seconds on port %d", port)
}

func testConnectivity(ctx context.Context, client *nethttp.Client) ConnectivityCheckResult {
	start := time.Now()
	req, err := nethttp.NewRequestWithContext(ctx, "GET", "http://connectivitycheck.gstatic.com/generate_204", nil)
	if err != nil {
		return ConnectivityCheckResult{Status: "fail", Error: err.Error()}
	}
	resp, err := client.Do(req)
	if err != nil {
		return ConnectivityCheckResult{Status: "fail", Error: err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode == 204 {
		return ConnectivityCheckResult{Status: "pass", LatencyMS: time.Since(start).Milliseconds()}
	}
	return ConnectivityCheckResult{Status: "fail", Error: fmt.Sprintf("expected 204, got %d", resp.StatusCode)}
}

func testEgressIP(ctx context.Context, client *nethttp.Client) EgressIPCheckResult {
	type source struct {
		name string
		url  string
	}

	sources := []source{
		{"ipify", "https://api.ipify.org?format=text"},
		{"ip.me", "https://ip.me"},
		{"ifconfig.me", "https://ifconfig.me"},
		{"myip.ipip.net", "https://myip.ipip.net"},
		{"ip.cn", "https://ip.useragentinfo.com/json"},
	}

	results := make(map[string]string)
	var detectedIP string

	for _, src := range sources {
		req, err := nethttp.NewRequestWithContext(ctx, "GET", src.url, nil)
		if err != nil {
			continue
		}
		req.Header.Set("Accept", "text/plain")
		req.Header.Set("User-Agent", "curl/8.0")
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			continue
		}
		raw := strings.TrimSpace(string(body))
		if raw == "" || strings.HasPrefix(raw, "<") {
			continue
		}

		ip := raw
		if strings.HasPrefix(raw, "{") {
			var obj map[string]any
			if json.Unmarshal([]byte(raw), &obj) == nil {
				if v, ok := obj["ip"].(string); ok {
					ip = v
				}
			}
		}

		ip = strings.TrimSpace(ip)
		if ip == "" {
			continue
		}
		results[src.name] = ip
		if detectedIP == "" {
			detectedIP = ip
		}
	}

	if detectedIP == "" {
		return EgressIPCheckResult{
			Status: "error",
			Error:  "所有 IP 检测源均不可达",
		}
	}

	return EgressIPCheckResult{
		Status:  "pass",
		IP:      detectedIP,
		Sources: results,
	}
}

func (h *AdminEgressIPsHandler) TestProxy() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		ipID := r.PathValue("ipID")
		ip, err := h.store.GetEgressIP(ctx, ipID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeJSON(w, nethttp.StatusNotFound, map[string]string{"error": "egress ip not found"})
				return
			}
			h.logger.Error("get egress ip for test failed", "ip_id", ipID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "get egress ip failed"})
			return
		}

		if ip.ProxyConfig == nil {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "proxy_config is empty"})
			return
		}

		dialer, proxyCleanup, err := getProxyDialer(ctx, ip.ProxyConfig)
		if proxyCleanup != nil {
			defer proxyCleanup()
		}
		if err != nil {
			writeJSON(w, nethttp.StatusOK, ProbeResult{
				Status:   "error",
				TestedAt: time.Now().UTC(),
				Message:  fmt.Sprintf("无法建立代理连接: %v", err),
			})
			return
		}

		httpClient := &nethttp.Client{
			Transport: &nethttp.Transport{
				DialContext:       dialer.DialContext,
				DisableKeepAlives: true,
			},
			Timeout: 25 * time.Second,
		}

		result := ProbeResult{TestedAt: time.Now().UTC()}
		result.Results.Connectivity = testConnectivity(ctx, httpClient)
		result.Results.EgressIP = testEgressIP(ctx, httpClient)
		result.Results.DNSLeak = DNSLeakCheckResult{
			Status: "skip",
			Error:  "DNS 泄漏检测仅在容器运行时进行，探针测试不适用",
		}

		connOK := result.Results.Connectivity.Status == "pass"
		ipOK := result.Results.EgressIP.Status == "pass"
		if connOK && ipOK {
			result.Status = "passed"
		} else if connOK {
			result.Status = "partial"
		} else {
			result.Status = "failed"
		}

		writeJSON(w, nethttp.StatusOK, result)
	})
}
