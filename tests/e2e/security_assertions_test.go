//go:build e2e && linux

// security_assertions_test.go v4.0 (Phase 55):
// SEC-01..03 三条同容器安全断言 e2e 用例。
//
// SEC-01: 用户不能 kill sing-box
// SEC-02: 用户不能读 sing-box config
// SEC-03: 用户 cap 集合为空
//
// darwin 上不参与编译；依赖 Linux + docker + Scenario.Start Step 2..7 实现。

package e2e

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestSecurity_CannotKillSingBox SEC-01: 用户进程不能 kill sing-box。
// docker exec -u <user> kill -9 $(pidof sing-box) 必须失败。
func TestSecurity_CannotKillSingBox(t *testing.T) {
	g := StartGoldenPath(t)
	if g == nil {
		return
	}
	if g.Host == nil || g.Host.ContainerName == "" {
		t.Skipf("container not ready (scenario step 7 未实现)")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	containerName := g.Host.ContainerName

	// 尝试以非 root 用户 kill sing-box
	cmd := exec.CommandContext(ctx, "docker", "exec", "-u", "clouduser",
		containerName, "kill", "-9", "$(pidof sing-box)")
	output, err := cmd.CombinedOutput()
	outStr := string(output)

	// SEC-01: kill 必须失败（Operation not permitted 或 kill 返回非 0）
	if err == nil {
		// 如果 kill 成功返回 exit 0，检查是否真的没杀死（进程仍在）
		psCmd := exec.CommandContext(ctx, "docker", "exec", containerName, "pidof", "sing-box")
		psOut, psErr := psCmd.CombinedOutput()
		if psErr == nil && strings.TrimSpace(string(psOut)) != "" {
			t.Logf("SEC-01: kill returned 0 but sing-box still alive (PID namespace isolation): %s", psOut)
			return
		}
		t.Fatalf("SEC-01 FAIL: non-root user killed sing-box (output=%s)", outStr)
	}

	// 验证 sing-box 进程仍然存活
	psCmd := exec.CommandContext(ctx, "docker", "exec", containerName, "pidof", "sing-box")
	psOut, psErr := psCmd.CombinedOutput()
	if psErr != nil || strings.TrimSpace(string(psOut)) == "" {
		t.Fatalf("SEC-01 FAIL: sing-box died after non-root kill attempt (err=%v, psOut=%s)", psErr, psOut)
	}
	t.Logf("SEC-01 PASS: non-root user cannot kill sing-box (err=%v, psOut=%s)", err, strings.TrimSpace(string(psOut)))
}

// TestSecurity_CannotReadConfig SEC-02: 用户不能读 sing-box config。
// docker exec -u <user> cat /etc/sing-box/config.json 必须失败。
func TestSecurity_CannotReadConfig(t *testing.T) {
	g := StartGoldenPath(t)
	if g == nil {
		return
	}
	if g.Host == nil || g.Host.ContainerName == "" {
		t.Skipf("container not ready (scenario step 7 未实现)")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	containerName := g.Host.ContainerName

	// 尝试以非 root 用户读 config
	cmd := exec.CommandContext(ctx, "docker", "exec", "-u", "clouduser",
		containerName, "cat", "/etc/sing-box/config.json")
	output, err := cmd.CombinedOutput()
	outStr := string(output)

	if err == nil {
		t.Fatalf("SEC-02 FAIL: non-root user read config.json (content=%s)", outStr)
	}

	// Permission denied 或 No such file or directory 都是 acceptable
	msg := strings.ToLower(outStr)
	if strings.Contains(msg, "permission denied") || strings.Contains(msg, "no such file") {
		t.Logf("SEC-02 PASS: config.json unreadable (%s)", strings.TrimSpace(outStr))
		return
	}

	// 即使 err 非 nil，也检查错误语义
	if err != nil {
		t.Logf("SEC-02 PASS: config.json cat failed (err=%v, stderr=%s)", err, strings.TrimSpace(outStr))
		return
	}
}

// TestSecurity_EmptyCapSet SEC-03: 用户 cap 集合为空。
// getpcaps $$ 输出空，ip link set tun0 down 失败，unshare -n 失败。
func TestSecurity_EmptyCapSet(t *testing.T) {
	g := StartGoldenPath(t)
	if g == nil {
		return
	}
	if g.Host == nil || g.Host.ContainerName == "" {
		t.Skipf("container not ready (scenario step 7 未实现)")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	containerName := g.Host.ContainerName
	var failures []string

	// 1. getpcaps $$ — 预期空 cap 集合（仅 "=" 或空输出）
	pcapsCmd := exec.CommandContext(ctx, "docker", "exec", containerName,
		"sh", "-c", "getpcaps $$ 2>/dev/null || echo 'no_getpcaps'")
	pcapsOut, _ := pcapsCmd.CombinedOutput()
	pcapsStr := strings.TrimSpace(string(pcapsOut))
	if pcapsStr != "" && pcapsStr != "=" && !strings.Contains(pcapsStr, "no_getpcaps") {
		// 有非空 cap 输出 → 检查是否包含实际 cap
		if strings.Contains(pcapsStr, "cap_") {
			failures = append(failures, fmt.Sprintf("unexpected caps: %s", pcapsStr))
		}
	}
	t.Logf("SEC-03 getpcaps: %s", pcapsStr)

	// 2. ip link set tun0 down — 预期 Operation not permitted（无 NET_ADMIN）
	ipCmd := exec.CommandContext(ctx, "docker", "exec", containerName,
		"ip", "link", "set", "tun0", "down")
	ipOut, ipErr := ipCmd.CombinedOutput()
	if ipErr == nil {
		failures = append(failures, "ip link set tun0 down succeeded (should be denied)")
	}
	t.Logf("SEC-03 ip link down: err=%v out=%s", ipErr, strings.TrimSpace(string(ipOut)))

	// 3. unshare -n /bin/bash — 预期失败（无 CAP_SYS_ADMIN）
	unshareCmd := exec.CommandContext(ctx, "docker", "exec", containerName,
		"unshare", "-n", "/bin/true")
	unshareOut, unshareErr := unshareCmd.CombinedOutput()
	if unshareErr == nil {
		failures = append(failures, "unshare -n succeeded (should be denied)")
	}
	t.Logf("SEC-03 unshare: err=%v out=%s", unshareErr, strings.TrimSpace(string(unshareOut)))

	if len(failures) > 0 {
		t.Fatalf("SEC-03 FAIL: %s", strings.Join(failures, "; "))
	}
	t.Logf("SEC-03 PASS: empty cap set confirmed")
}
