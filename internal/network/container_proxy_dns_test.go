package network

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestContainerProxy_WriteContainerDNSConfig_WritesFiles 验证
// WriteContainerDNSConfig 在 <DATA_DIR>/gateway/<host_id>/ 写出两个源文件，
// 且内容与 resolvConfContent / nsswitchConfContent 完全一致。
// 这两个文件是 worker 容器 ro bind mount 的源路径，内容错位 = 容器 DNS 断链。
func TestContainerProxy_WriteContainerDNSConfig_WritesFiles(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DATA_DIR", tmp)

	hostID := "test-host-dns"
	if err := WriteContainerDNSConfig(hostID); err != nil {
		t.Fatalf("WriteContainerDNSConfig failed: %v", err)
	}

	dir := GatewayConfigDir(hostID)
	expectDir := filepath.Join(tmp, "gateway", hostID)
	if dir != expectDir {
		t.Errorf("GatewayConfigDir = %q, want %q", dir, expectDir)
	}

	resolvPath := filepath.Join(dir, "resolv.conf")
	resolvData, err := os.ReadFile(resolvPath)
	if err != nil {
		t.Fatalf("read resolv.conf: %v", err)
	}
	if string(resolvData) != resolvConfContent {
		t.Errorf("resolv.conf content mismatch:\n got=%q\nwant=%q", string(resolvData), resolvConfContent)
	}
	// 双保险：第一行必须是 sing-box gateway tun0
	if !strings.HasPrefix(string(resolvData), "nameserver 172.19.0.1\n") {
		t.Errorf("resolv.conf must start with `nameserver 172.19.0.1` line, got:\n%s", resolvData)
	}

	nsswitchPath := filepath.Join(dir, "nsswitch.conf")
	nsswitchData, err := os.ReadFile(nsswitchPath)
	if err != nil {
		t.Fatalf("read nsswitch.conf: %v", err)
	}
	if string(nsswitchData) != nsswitchConfContent {
		t.Errorf("nsswitch.conf content mismatch:\n got=%q\nwant=%q", string(nsswitchData), nsswitchConfContent)
	}
	// hosts 行必须严格限定 files dns，禁用所有可能引入旁路 DNS 的 NSS 模块。
	if !strings.Contains(string(nsswitchData), "\nhosts:          files dns\n") {
		t.Errorf("nsswitch.conf must contain exact `hosts: files dns` line, got:\n%s", nsswitchData)
	}
	// Phase 45 WR-06：扩充禁用列表参考 RHEL/Debian 官方 NSS 模块名清单。
	// 任何一个出现都意味着该模块会被 glibc 在 `hosts:` 行实际加载，绕过 tun0
	// DNS 入口锁导致 BYPASS-DNS-03/04。
	for _, forbidden := range []string{
		"mdns", "myhostname", "wins", "nis_plus",
		"resolve", // systemd-resolved 的 libnss-resolve 入口
		"dns_sd",  // Avahi DNS-SD 入口
		"lwres",   // BIND lwresd 入口
	} {
		if strings.Contains(string(nsswitchData), forbidden) {
			t.Errorf("nsswitch.conf must NOT contain %q (BYPASS-DNS-04 mitigation)", forbidden)
		}
	}
}

// TestContainerProxy_ResolvConfBindMount 通过静态文本断言确认
// worker.go::buildCreateArgs 在 egressCfg.Proxy != nil 分支注入了
// /etc/resolv.conf 的 ro bind mount。任何「忘了 mount」「mount 写错路径」
// 「丢了 :ro 标记」的回归会在 CI 失败。
func TestContainerProxy_ResolvConfBindMount(t *testing.T) {
	src, err := os.ReadFile("../runtime/tasks/worker.go")
	if err != nil {
		t.Fatalf("read worker.go: %v", err)
	}
	content := string(src)

	needle := `gwDir+"/resolv.conf:/etc/resolv.conf:ro"`
	if !strings.Contains(content, needle) {
		t.Errorf("worker.go::buildCreateArgs must include resolv.conf ro bind mount, needle=%q not found", needle)
	}

	// 还要确认 mount 注入受 egressCfg.Proxy != nil 守卫，避免无代理环境
	// 因不存在的源文件失败。
	if !strings.Contains(content, "egressCfg != nil && egressCfg.Proxy != nil") {
		t.Errorf("worker.go must guard DNS bind mount with `egressCfg != nil && egressCfg.Proxy != nil`")
	}
}

// TestContainerProxy_NsswitchBindMount 通过静态文本断言确认
// worker.go::buildCreateArgs 在同分支注入了 /etc/nsswitch.conf 的 ro bind mount。
func TestContainerProxy_NsswitchBindMount(t *testing.T) {
	src, err := os.ReadFile("../runtime/tasks/worker.go")
	if err != nil {
		t.Fatalf("read worker.go: %v", err)
	}
	content := string(src)

	needle := `gwDir+"/nsswitch.conf:/etc/nsswitch.conf:ro"`
	if !strings.Contains(content, needle) {
		t.Errorf("worker.go::buildCreateArgs must include nsswitch.conf ro bind mount, needle=%q not found", needle)
	}

	// 必须使用 network.GatewayConfigDir 拼源路径（B3 cross-plan sync 要求）
	if !strings.Contains(content, "network.GatewayConfigDir(request.HostID)") {
		t.Errorf("worker.go must derive bind-mount source dir from network.GatewayConfigDir(request.HostID)")
	}
}
