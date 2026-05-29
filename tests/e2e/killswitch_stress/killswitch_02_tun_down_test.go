//go:build e2e && linux

// killswitch_02_tun_down_test.go 是 Phase 50 Plan 02 / KILL-02 的主用例：
//
//   - 入口：基线 worker `curl https://ifconfig.io` 必须 exit 0；
//   - 后台启 host eth0 tcpdump（BPF：src worker and not dst gateway）；
//   - `docker exec <gateway> ip link set tun0 down` 软故障注入；
//   - 容器内立即跑 `curl --max-time 3 <url>`，期望非 0 退出；
//   - tcpdump 退出后包数必须 0；
//   - ClassifyStressResult("KILL-02", ...) 合成裁决；
//   - cleanup：t.Cleanup 内 best-effort `ip link set tun0 up`。
//
// 与 KILL-01 互补：KILL-01 测 SIGKILL 硬故障，KILL-02 测 tun 设备 down 软故障；
// 同一 ≤ 3000ms 断网契约。

package killswitch_stress

import (
	"context"
	"testing"
	"time"

	e2e "github.com/zanel1u/cloud-cli-proxy/tests/e2e"
)

func TestKillSwitch_02_TunDevDown(t *testing.T) {
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
	// Phase 55: 单容器架构，gateway 内嵌于 worker，gatewayIP = workerIP
	gatewayIP := workerIP

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

	t.Cleanup(func() {
		upCtx, upCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer upCancel()
		if err := g.SetTunDevUp(upCtx); err != nil {
			t.Logf("cleanup: set tun0 up failed (best-effort): %v", err)
		}
	})

	contract := e2e.KillswitchStressContract["KILL-02"]
	start := time.Now()
	if err := g.SetTunDevDown(ctx); err != nil {
		t.Fatalf("set tun0 down: %v", err)
	}
	probeTimeout := time.Duration(contract.MaxDisconnectMs) * time.Millisecond
	probeExit, probeErr := g.ProbeOutboundFromUser(ctx, probeURL, probeTimeout)
	elapsedMs := int(time.Since(start).Milliseconds())
	if probeErr != nil {
		t.Fatalf("probe outbound after tun0 down: %v", probeErr)
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

	verdict, reason := e2e.ClassifyStressResult("KILL-02", e2e.StressEvidence{
		ProbeExitCode: probeExit,
		LeakedPackets: td.packets,
		ElapsedMs:     elapsedMs,
	})
	t.Logf("KILL-02 verdict=%s reason=%q elapsed=%dms probeExit=%d packets=%d worker=%s gateway=%s bpf=%q",
		verdict, reason, elapsedMs, probeExit, td.packets, workerIP, gatewayIP, bpf)
	if verdict != e2e.StressVerdictPass {
		t.Fatalf("KILL-02 fail: verdict=%s reason=%s elapsed=%dms probeExit=%d packets=%d",
			verdict, reason, elapsedMs, probeExit, td.packets)
	}
}
