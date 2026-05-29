//go:build e2e && linux

// killswitch_03_netem_delay_test.go 是 Phase 50 Plan 03 / KILL-03 的主用例：
//
//   - 入口：基线 worker `curl https://ifconfig.io` 必须 exit 0；
//   - 解析基线出口 IP（三源投票，必须多数派达成 winner）；
//   - InjectPumbaNetem 给 gateway 注入 `delay 1000ms --duration 30s`；
//   - SSH 22 端口 banner 探测（必须存活）；
//   - 出口 IP 二次投票（允许全弃权 → Inconclusive 而非 Fail）；
//   - ClassifyStressResult("KILL-03", ...) 合成裁决。
//
// 与 KILL-01/02/04 行为契约不同：本用例不做断网 timing 断言（contract
// MaxDisconnectMs=0），只锁「SSH 控制流存活 + 不允许给错误的出口 IP」。

package killswitch_stress

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	e2e "github.com/zanel1u/cloud-cli-proxy/tests/e2e"

	"github.com/zanel1u/cloud-cli-proxy/tests/e2e/harness"
)

func TestKillSwitch_03_NetemDelay(t *testing.T) {
	g, skip := StartStressGolden(t)
	if skip {
		return
	}
	EnsureDumper(t, g)

	if _, err := exec.LookPath("docker"); err != nil {
		t.Skipf("docker not in PATH; KILL-03 Pumba sidecar 不可用: %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
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

	// 基线 SSH 探测：22 必须先 listen，否则 netem 后探测无意义。
	sshCtx, sshCancel := context.WithTimeout(ctx, 10*time.Second)
	if err := g.ProbeSSHBanner(sshCtx, 5*time.Second); err != nil {
		sshCancel()
		t.Skipf("baseline ssh banner unavailable (worker 22 未起): %v", err)
		return
	}
	sshCancel()

	// 基线出口 IP：必须多数派达成 winner，否则用例无 ground truth。
	workerHandle := newWorkerExecHandle(workerName)
	baselineRaw := e2e.FetchEgressIPInContainer(ctx, workerHandle)
	baselineVote := e2e.Vote(baselineRaw)
	if !baselineVote.OK {
		t.Skipf("baseline egress IP vote not stable (raw=%v dissent=%v); 跳过 KILL-03",
			baselineRaw, baselineVote.Dissent)
		return
	}
	expectedIP := baselineVote.Winner

	// 注入 Pumba netem delay 1000ms / duration 30s。
	cleanup, err := g.InjectPumbaNetem(ctx, gatewayName, e2e.PumbaNetemParams{
		Mode:     "delay",
		DelayMs:  1000,
		Duration: 30 * time.Second,
	})
	if err != nil {
		// 区分 ImageMissing / DaemonDown 兜底 → t.Skipf；其它 → t.Fatalf。
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "image") || strings.Contains(msg, "manifest") ||
			strings.Contains(msg, "pull") || strings.Contains(msg, "daemon") {
			t.Skipf("Pumba sidecar 启动失败（image / daemon 不可用，CI 网络白名单未覆盖）: %v", err)
			return
		}
		t.Fatalf("inject pumba: %v", err)
	}
	defer cleanup()

	// netem 注入后给 tc 规则 1s 收敛窗口（用 harness.WaitFor 守门，不裸 sleep）。
	convergeCtx, convergeCancel := context.WithTimeout(ctx, 5*time.Second)
	_ = harness.WaitFor(convergeCtx, "pumba-tc-converge", func(ctx context.Context) error {
		// 通过 SSH banner 探测兼作 tc 规则就绪信号：能拿到 banner 说明 sing-box
		// 控制流仍存活；用最短超时反复试，直到 tc 真正生效（延迟可观察）。
		return g.ProbeSSHBanner(ctx, 2*time.Second)
	},
		harness.WithTimeout(5*time.Second),
		harness.WithPollInterval(500*time.Millisecond),
	)
	convergeCancel()

	// SSH 存活：netem 延迟下 banner 仍应在 10s 内拿到。
	sshAliveCtx, sshAliveCancel := context.WithTimeout(ctx, 12*time.Second)
	sshErr := g.ProbeSSHBanner(sshAliveCtx, 10*time.Second)
	sshAliveCancel()
	sshAlive := sshErr == nil

	// 二次出口 IP 投票：允许 timeout 全弃权（contract AllowInconclusive=true）。
	voteCtx, voteCancel := context.WithTimeout(ctx, 30*time.Second)
	stressRaw := e2e.FetchEgressIPInContainer(voteCtx, workerHandle)
	voteCancel()
	stressVote := e2e.Vote(stressRaw)

	verdict, reason := e2e.ClassifyStressResult("KILL-03", e2e.StressEvidence{
		SSHAlive:         sshAlive,
		EgressIPVote:     stressVote,
		ExpectedEgressIP: expectedIP,
	})
	t.Logf("KILL-03 verdict=%s reason=%q sshAlive=%v expected=%s vote=%+v stressRaw=%v sshErr=%v",
		verdict, reason, sshAlive, expectedIP, stressVote, stressRaw, sshErr)

	switch verdict {
	case e2e.StressVerdictPass:
		return
	case e2e.StressVerdictInconclusive:
		t.Skipf("KILL-03 inconclusive (allowed): %s", reason)
		return
	default:
		t.Fatalf("KILL-03 fail: verdict=%s reason=%s sshAlive=%v expected=%s vote=%+v",
			verdict, reason, sshAlive, expectedIP, stressVote)
	}
}
