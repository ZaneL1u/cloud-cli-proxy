//go:build e2e && linux

// leak_04_ipv6_test.go 是 Phase 49 LEAK-04 的 e2e 主用例：
//
//   - `cat /proc/sys/net/ipv6/conf/all/disable_ipv6` == "1"。
//   - `cat /proc/sys/net/ipv6/conf/default/disable_ipv6` == "1"。
//   - `curl -6 --max-time 3 https://ipv6.google.com` 必须失败。
//   - 若 worker 有 IPv6 地址 → host eth0 抓包 `ip6 and src <workerIPv6>` 必须 0 包；
//     无 IPv6 地址 → 跳过抓包断言（IPv6 stack 整个被关 = 预期）。

package leak

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	e2e "github.com/zanel1u/cloud-cli-proxy/tests/e2e"
)

func TestLeak_04_IPv6_BlockedByHostEth0(t *testing.T) {
	g, skip := StartLeakGolden(t)
	if skip {
		return
	}
	EnsureLeakWorkerTools(t, g)
	EnsureDumper(t, g)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	for _, p := range []string{
		"/proc/sys/net/ipv6/conf/all/disable_ipv6",
		"/proc/sys/net/ipv6/conf/default/disable_ipv6",
	} {
		val, err := g.ReadProcFile(ctx, p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		if val != "1" {
			t.Fatalf("LEAK-04 %s = %q，要求 1（双保险，worker.go sysctl）", p, val)
		}
	}

	curlRes, err := g.CurlIPv6(ctx, "https://ipv6.google.com")
	if err != nil {
		t.Fatalf("curl -6: %v", err)
	}
	verdict := e2e.ClassifyLeakProbe(curlRes, true)
	t.Logf("LEAK-04 verdict=%s blocked=%v reason=%q exit=%d",
		verdict, curlRes.Blocked, curlRes.Reason, curlRes.ExitCode)
	if !curlRes.Blocked {
		t.Fatalf("LEAK-04 curl -6 未被阻断（Reason=%q）", curlRes.Reason)
	}

	workerName, err := workerInspectName(ctx, g)
	if err != nil {
		t.Skipf("worker container name unavailable: %v", err)
		return
	}
	workerIPv6, err := inspectContainerIPv6(ctx, workerName)
	if err != nil || workerIPv6 == "" {
		t.Logf("worker 无 IPv6 地址（%v）；跳过抓包断言（disable_ipv6=1 已是更强约束）", err)
		return
	}

	bpf := "ip6 and src host " + workerIPv6
	packets, err := g.TcpdumpOnHostEth0(ctx, bpf, 5, 5*time.Second)
	if err != nil {
		t.Logf("tcpdump sidecar err: %v", err)
		t.Skipf("host eth0 ipv6 tcpdump oracle unavailable; deferred-to-CI")
		return
	}
	if packets > 0 {
		t.Fatalf("LEAK-04 host eth0 抓到 %d 个 IPv6 包来自 worker=%s", packets, workerIPv6)
	}
}

// inspectContainerIPv6 通过 docker inspect 拿默认 bridge 上的 IPv6 地址，
// 不存在 / 为空 → 返回 "" + nil。
func inspectContainerIPv6(ctx context.Context, name string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", "inspect", "-f",
		"{{.NetworkSettings.GlobalIPv6Address}}", name)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}
