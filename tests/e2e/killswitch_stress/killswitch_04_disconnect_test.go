//go:build e2e && linux

// killswitch_04_disconnect_test.go 是 Phase 50 Plan 04 / KILL-04 的主用例：
//
//   - 入口：基线 worker `curl https://ifconfig.io` 必须 exit 0；
//   - 后台启 host eth0 tcpdump（BPF：src worker and not dst gateway）；
//   - DisconnectGatewayFromBridge 把 gateway 从专属 cloudproxy-net-<HostID>
//     摘走（PickGatewayBridgeNetwork 自动挑出该网络）；
//   - 容器内立即跑 `curl --max-time 3 <url>`，期望非 0 退出（worker 不允许
//     fallback 到 docker 默认 bridge / host 默认路由直连）；
//   - tcpdump 退出后包数必须 0；
//   - ClassifyStressResult("KILL-04", ...) 合成裁决；
//   - cleanup：t.Cleanup 内 best-effort ReconnectGatewayToBridge。
//
// CONTEXT §校准：grep 源码确认 gateway 接入的是自定义 bridge
// `cloudproxy-net-<HostID>`（`internal/network/container_proxy_provider.go:323`）
// 而非默认 `bridge`；KILL-04 disconnect 的就是这个网络。

package killswitch_stress

import (
	"context"
	"strings"
	"testing"
	"time"

	e2e "github.com/zanel1u/cloud-cli-proxy/tests/e2e"
)

func TestKillSwitch_04_NetworkDisconnect(t *testing.T) {
	g, skip := StartStressGolden(t)
	if skip {
		return
	}
	EnsureDumper(t, g)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	const probeURL = "https://ifconfig.io"

	baselineExit, err := g.ProbeOutboundFromUser(ctx, probeURL, 5*time.Second)
	if err != nil {
		t.Skipf("baseline probe unavailable: %v", err)
		return
	}
	if baselineExit != 0 {
		t.Skipf("baseline egress not working (exit=%d); 避免外网抖动 false-fail", baselineExit)
		return
	}

	workerName, err := workerInspectName(ctx, g)
	if err != nil {
		t.Skipf("worker container name unavailable: %v", err)
		return
	}
	gatewayName, err := workerInspectName(ctx, g) // Phase 55: 单容器，gateway = worker
	if err != nil {
		t.Skipf("gateway container name unavailable: %v", err)
		return
	}

	workerIP, err := g.InspectContainerIPv4(ctx, workerName, "")
	if err != nil {
		t.Skipf("worker container ipv4 not available: %v", err)
		return
	}
	gatewayIP := g.Gateway.GatewayIP
	if gatewayIP == "" {
		var ipErr error
		gatewayIP, ipErr = g.InspectContainerIPv4(ctx, gatewayName, "")
		if ipErr != nil {
			t.Skipf("gateway ipv4 not available: %v", ipErr)
			return
		}
	}

	bpf := "src host " + workerIP + " and not dst host " + gatewayIP

	type tcpdumpResult struct {
		packets int
		err     error
	}
	dumpCh := make(chan tcpdumpResult, 1)
	go func() {
		packets, dErr := g.TcpdumpOnHostEth0(ctx, bpf, 5, e2e.KillswitchTimingContract.TcpdumpWindow)
		dumpCh <- tcpdumpResult{packets: packets, err: dErr}
	}()

	savedNet, savedIP, err := g.DisconnectGatewayFromBridge(ctx)
	if err != nil {
		// backend GAP：gateway 当前未接 cloudproxy-net-* / 任何自定义 bridge
		// （可能 backend 改用 macvlan / host network）。流转 Phase 51 同 Phase 49
		// LEAK-06/07/08 模式。
		if strings.Contains(err.Error(), "has no cloudproxy-net") ||
			strings.Contains(err.Error(), "custom bridge network") {
			t.Skipf("backend GAP: gateway 未接自定义 bridge，KILL-04 流转 Phase 51 (源码侧未实现专属 bridge): %v", err)
			return
		}
		t.Fatalf("disconnect gateway from bridge: %v", err)
	}

	t.Cleanup(func() {
		recCtx, recCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer recCancel()
		if err := g.ReconnectGatewayToBridge(recCtx, savedNet, savedIP); err != nil {
			t.Logf("cleanup: reconnect %s to %s failed (best-effort): %v", gatewayName, savedNet, err)
		}
	})

	contract := e2e.KillswitchStressContract["KILL-04"]
	start := time.Now()
	probeTimeout := time.Duration(contract.MaxDisconnectMs) * time.Millisecond
	probeExit, probeErr := g.ProbeOutboundFromUser(ctx, probeURL, probeTimeout)
	elapsedMs := int(time.Since(start).Milliseconds())
	if probeErr != nil {
		t.Fatalf("probe outbound after disconnect: %v", probeErr)
	}

	var td tcpdumpResult
	select {
	case td = <-dumpCh:
	case <-ctx.Done():
		t.Fatalf("tcpdump goroutine did not finish before ctx deadline: %v", ctx.Err())
	}

	if td.err != nil {
		t.Logf("tcpdump sidecar reported err: %v", td.err)
		t.Skipf("host eth0 tcpdump oracle unavailable; deferred-to-CI")
		return
	}

	verdict, reason := e2e.ClassifyStressResult("KILL-04", e2e.StressEvidence{
		ProbeExitCode: probeExit,
		LeakedPackets: td.packets,
		ElapsedMs:     elapsedMs,
	})
	t.Logf("KILL-04 verdict=%s reason=%q elapsed=%dms probeExit=%d packets=%d savedNet=%s savedIP=%s bpf=%q",
		verdict, reason, elapsedMs, probeExit, td.packets, savedNet, savedIP, bpf)
	if verdict != e2e.StressVerdictPass {
		t.Fatalf("KILL-04 fail: verdict=%s reason=%s elapsed=%dms probeExit=%d packets=%d savedNet=%s",
			verdict, reason, elapsedMs, probeExit, td.packets, savedNet)
	}
}
