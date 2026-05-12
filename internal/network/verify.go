package network

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// containerExpectedDNS 是 worker 容器 /etc/resolv.conf 第一行必须出现的
// nameserver 地址。Phase 45 Plan 02 起 DNS 入口被 ro bind mount 固化为
// sing-box gateway 的 tun0 (172.19.0.1)，与 EgressConfig.Proxy.DNSServer
// 字段（描述 sing-box → 上游 DNS）是两个不同概念，所以这里用常量而非
// EgressConfig 字段。Phase 47 之前修改该常量必须同步调整 resolvConfContent
// 与 BypassRouterTun0IPv4。
const containerExpectedDNS = "172.19.0.1"

// VerifyResult captures the outcome of each verification check performed
// inside a container's network namespace after tunnel wiring completes.
type VerifyResult struct {
	EgressIPMatch  bool
	ActualEgressIP string
	DNSCorrect     bool
	ActualDNS      string
	LeakBlocked    bool
	LeakTarget     string
}

// AllPassed returns true only when all three verification checks passed.
func (r VerifyResult) AllPassed() bool {
	return r.EgressIPMatch && r.DNSCorrect && r.LeakBlocked
}

// VerifyNetworkIntegrity runs three independent checks inside the container's
// network namespace via nsenter:
//  1. Egress IP must match the expected binding (D-09)
//  2. DNS resolver must point to the tunnel-side DNS server (D-09)
//  3. Direct (non-tunnel) outbound connections must be blocked (D-09)
//
// All three checks run regardless of individual failures so the caller gets
// the complete verification state. The returned error (if any) is a typed
// NetworkError matching the highest-priority failing check.
func VerifyNetworkIntegrity(ctx context.Context, containerPID uint32, expected EgressConfig) (VerifyResult, error) {
	prefix := []string{"nsenter", "-t", strconv.FormatUint(uint64(containerPID), 10), "-n", "--"}

	var result VerifyResult

	// Check 1: egress IP matches binding
	verifyEgressIP(ctx, prefix, expected.ExpectedIP, &result)

	// Check 2: DNS resolver points to tunnel DNS
	// Phase 45 Plan 02：容器 /etc/resolv.conf 被 ro bind mount 锁死为 tun0
	// (172.19.0.1)，与 EgressConfig.Proxy.DNSServer（gateway → 上游 DNS）
	// 解耦，用包级常量 containerExpectedDNS 作为预期值。
	verifyDNS(ctx, prefix, containerExpectedDNS, &result)

	// Check 3: direct outbound is blocked by firewall
	verifyLeakBlocked(ctx, prefix, &result)

	if result.AllPassed() {
		return result, nil
	}

	return result, firstNetworkError(expected, result)
}

func verifyEgressIP(ctx context.Context, prefix []string, expectedIP string, result *VerifyResult) {
	checkCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	args := append(append([]string{}, prefix...), "curl", "-4", "--max-time", "10", "-s", "https://ip.me")
	out, err := exec.CommandContext(checkCtx, args[0], args[1:]...).Output()
	if err != nil {
		result.EgressIPMatch = false
		result.ActualEgressIP = ""
		return
	}

	actual := strings.TrimSpace(string(out))
	result.ActualEgressIP = actual
	result.EgressIPMatch = actual == expectedIP
}

func verifyDNS(ctx context.Context, prefix []string, expectedDNS string, result *VerifyResult) {
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	args := append(append([]string{}, prefix...), "cat", "/etc/resolv.conf")
	out, err := exec.CommandContext(checkCtx, args[0], args[1:]...).Output()
	if err != nil {
		result.DNSCorrect = false
		// Phase 45 WR-07：err 时区分「nsenter 失败 / resolv.conf 缺失」与
		// 「内容为空」，给运维一个可识别的 sentinel 而不是空串。
		result.ActualDNS = fmt.Sprintf("<read failed: %v>", err)
		return
	}

	// Phase 45 WR-07：旧实现只校验第一行 nameserver 是否等于 expectedDNS，
	// 任何附加 fallback nameserver（例如 `nameserver 8.8.8.8` 跟在后面）都
	// 会让 verifyDNS 通过，但实际容器 resolv.conf 在 172.19.0.1 超时后会
	// fallback 到 8.8.8.8，等同 DNS 入口锁失效。
	//
	// 修复：与 PrepareGateway 写盘的 resolvConfContent **整体逐字节相等**比对。
	// 任何额外行、注释、缺行都会立即识别为 DNS lock-in 被破坏。
	rawContent := string(out)

	// 同时抓出第一行 nameserver 用作 ActualDNS 字段，便于日志与上层 metadata。
	var firstNS string
	for _, line := range strings.Split(rawContent, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "nameserver") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				firstNS = fields[1]
				break
			}
		}
	}
	result.ActualDNS = firstNS

	if rawContent != resolvConfContent {
		result.DNSCorrect = false
		return
	}
	// 双保险：首行 nameserver 必须等于期望值（在内容完全相等的前提下永远成立，
	// 但保持显式断言以便未来 resolvConfContent 演进时仍能 catch 该不变量）。
	result.DNSCorrect = firstNS == expectedDNS
}

func verifyLeakBlocked(ctx context.Context, prefix []string, result *VerifyResult) {
	result.LeakTarget = "1.1.1.1:80"

	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	args := append(append([]string{}, prefix...), "timeout", "3", "bash", "-c", "echo >/dev/tcp/1.1.1.1/80")
	err := exec.CommandContext(checkCtx, args[0], args[1:]...).Run()

	// Connection failure means firewall is blocking direct outbound — that's the desired state.
	result.LeakBlocked = err != nil
}

// firstNetworkError returns the highest-priority NetworkError for the first
// failing check (egress IP > DNS > leak).
func firstNetworkError(expected EgressConfig, r VerifyResult) *NetworkError {
	hostID := "" // populated by caller context if needed

	if !r.EgressIPMatch {
		if r.ActualEgressIP == "" {
			return &NetworkError{
				Type:    ErrEgressUnreachable,
				Message: "egress connectivity check failed",
				HostID:  hostID,
			}
		}
		return &NetworkError{
			Type:    ErrEgressIPMismatch,
			Message: fmt.Sprintf("egress IP mismatch: expected %s, got %s", expected.ExpectedIP, r.ActualEgressIP),
			HostID:  hostID,
			Metadata: map[string]any{
				"expected": expected.ExpectedIP,
				"actual":   r.ActualEgressIP,
			},
		}
	}

	if !r.DNSCorrect {
		return &NetworkError{
			Type:    ErrDNSLeak,
			Message: fmt.Sprintf("DNS resolver mismatch: expected %s, got %s", containerExpectedDNS, r.ActualDNS),
			HostID:  hostID,
			Metadata: map[string]any{
				"expected_dns": containerExpectedDNS,
				"actual_dns":   r.ActualDNS,
			},
		}
	}

	return &NetworkError{
		Type:    ErrLeakNotBlocked,
		Message: fmt.Sprintf("direct outbound to %s was not blocked", r.LeakTarget),
		HostID:  hostID,
		Metadata: map[string]any{
			"target": r.LeakTarget,
		},
	}
}
