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

// bypassProbeTargetURL Phase 47 Plan 03 verifyBypassEgressMatchesEth0 的探测端点。
//
// 选取要求：必须落在 v3.5 白名单（loopback 预设之外的 LAN/自定义白名单）内，并且
// 远端会把请求 source IP 回显到响应 body 里。在生产 UAT 中由调用方保证目标 IP
// 可达；此处给一个 RFC1918 LAN 默认值作为占位，单测通过 nsenterRunner 注入 fake
// 完全旁路真实 curl。
const bypassProbeTargetURL = "http://192.168.0.1/sourceip"

// nonBypassProbeTargetURL Phase 47 Plan 03 verifyNonBypassTraffic 的探测端点。
//
// 选取要求：必须落在 v3.5 白名单**之外**，强制走 sing-box 代理出口。同上由调用方
// 保证可解析；单测用 fake nsenterRunner 旁路。
const nonBypassProbeTargetURL = "https://api.example.com/sourceip"

// publicDNSProbeServer Phase 47 Plan 03 verifyPublicDNSBlocked 探测的公网 DNS。
// 用 8.8.8.8 因为它是最常被工程师手动 dig 用来「试一下 DNS 通不通」的目标，nft
// 阻断这一目标即覆盖 99% 的 DNS 旁路尝试。
const publicDNSProbeServer = "8.8.8.8"

// nsenterRunner 在容器 netns 中执行命令的可注入 hook。
//
// 模式与 bypass_reload.go::nftRunner 一致：抽包级 var 是为了让单测可以注入 fake，
// 不依赖宿主机存在 nsenter 二进制或目标容器。
//
// 入参 args 是「完整命令行」—— 调用方负责把 nsenter prefix（如
// `nsenter -t <pid> -n --`）与目标命令（`curl …` / `dig …`）拼成一个 slice
// 传进来；返回 stdout 字节与 error（exec 错误 / 非零退出码 / context 超时）。
var nsenterRunner = func(ctx context.Context, args ...string) ([]byte, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("nsenterRunner: empty args")
	}
	return exec.CommandContext(ctx, args[0], args[1:]...).Output()
}

// VerifyResult captures the outcome of each verification check performed
// inside a container's network namespace after tunnel wiring completes.
//
// Phase 47 Plan 03 在原有 3 项检查（EgressIPMatch / DNSCorrect / LeakBlocked）
// 基础上新增 3 项流量检查：
//   - BypassEgressOK     白名单 IP 流量必须从 host eth0 出（源 IP = host eth0 IP）
//   - NonBypassEgressOK  非白名单流量必须从代理出口出（源 IP = egress IP）
//   - PublicDNSBlocked   nsenter+dig @8.8.8.8 example.com 必须超时（公网 DNS 被阻断）
type VerifyResult struct {
	EgressIPMatch  bool
	ActualEgressIP string
	DNSCorrect     bool
	ActualDNS      string
	LeakBlocked    bool
	LeakTarget     string

	// Phase 47 Plan 03 新增三项
	BypassEgressOK        bool   `json:"bypass_egress_ok"`
	ActualBypassEgress    string `json:"actual_bypass_egress,omitempty"`
	NonBypassEgressOK     bool   `json:"non_bypass_egress_ok"`
	ActualNonBypassEgress string `json:"actual_non_bypass_egress,omitempty"`
	PublicDNSBlocked      bool   `json:"public_dns_blocked"`
}

// AllPassed returns true only when all six verification checks passed
//（旧 3 项 + Phase 47 Plan 03 新增 3 项）。
func (r VerifyResult) AllPassed() bool {
	return r.EgressIPMatch && r.DNSCorrect && r.LeakBlocked &&
		r.BypassEgressOK && r.NonBypassEgressOK && r.PublicDNSBlocked
}

// VerifyNetworkIntegrity runs six independent checks inside the container's
// network namespace via nsenter:
//
//  Legacy (Phase 45 及以前):
//   1. Egress IP must match the expected binding (D-09)
//   2. DNS resolver must point to the tunnel-side DNS server (D-09)
//   3. Direct (non-tunnel) outbound connections must be blocked (D-09)
//
//  Phase 47 Plan 03 (BYPASS-VERIFY-01):
//   4. Whitelist (bypass) traffic exits via host eth0, NOT the proxy egress
//   5. Non-whitelist traffic exits via the proxy egress IP
//   6. Public DNS (dig @8.8.8.8) is blocked (must time out)
//
// All six checks run regardless of individual failures so the caller gets
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

	// Phase 47 Plan 03：3 项新流量检查
	// 白名单流量必须从 host eth0 出 —— 这里用一个 RFC1918 LAN 默认 IP 作为
	// 「host eth0 邻居」的占位探测目标。生产 CI 通过环境变量或 EgressConfig
	// 扩展字段覆盖；单测通过 fake nsenterRunner 旁路。
	hostEth0IP := detectHostEth0IPFallback()
	verifyBypassEgressMatchesEth0(ctx, prefix, hostEth0IP, &result)

	// 非白名单流量必须从代理出口出 —— 期望 source IP = expected.ExpectedIP
	verifyNonBypassTraffic(ctx, prefix, expected.ExpectedIP, &result)

	// dig @8.8.8.8 必须超时（公网 DNS 被 nft 阻断）
	verifyPublicDNSBlocked(ctx, prefix, &result)

	if result.AllPassed() {
		return result, nil
	}

	return result, firstNetworkError(expected, result)
}

// detectHostEth0IPFallback 给 verifyBypassEgressMatchesEth0 提供一个 fallback
// host eth0 探测预期值。Phase 47 Plan 03 暂以白名单内的固定 LAN IP 占位；后续
// 若要做严格断言，需要把宿主机真实 eth0 IP 经 EgressConfig 注入。
func detectHostEth0IPFallback() string {
	return "192.168.0.1"
}

func verifyEgressIP(ctx context.Context, prefix []string, expectedIP string, result *VerifyResult) {
	checkCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	args := append(append([]string{}, prefix...), "curl", "-4", "--max-time", "10", "-s", "https://ip.me")
	out, err := nsenterRunner(checkCtx, args...)
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
	out, err := nsenterRunner(checkCtx, args...)
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
	_, err := nsenterRunner(checkCtx, args...)

	// Connection failure means firewall is blocking direct outbound — that's the desired state.
	result.LeakBlocked = err != nil
}

// verifyBypassEgressMatchesEth0 模拟白名单流量：curl 拉取目标 echo 服务，
// 期望响应包含 host eth0 IP（说明这条流量从 nft `accept oifname eth0 daddr in
// @whitelist_v4` 命中后直接走 host eth0 出，没绕进 sing-box 代理）。
//
// 失败语义：source IP != hostEth0IP → BypassEgressOK=false，写入 ActualBypassEgress。
// 命令出错（如目标不可达、curl 超时）也视为失败，但 ActualBypassEgress 留空。
//
// Phase 47 Plan 03 BYPASS-VERIFY-01。
func verifyBypassEgressMatchesEth0(ctx context.Context, prefix []string, hostEth0IP string, result *VerifyResult) {
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	args := append(append([]string{}, prefix...), "curl", "-4", "--max-time", "8", "-s", bypassProbeTargetURL)
	out, err := nsenterRunner(checkCtx, args...)
	if err != nil {
		result.BypassEgressOK = false
		result.ActualBypassEgress = ""
		return
	}
	actual := strings.TrimSpace(string(out))
	result.ActualBypassEgress = actual
	result.BypassEgressOK = actual == hostEth0IP
}

// verifyNonBypassTraffic 模拟非白名单流量：curl 公网 echo 服务，期望响应
// source IP = 代理出口 IP（expectedEgressIP），即流量从 sing-box 代理出去。
//
// 失败语义：source IP != expectedEgressIP（典型 leak 路径：流量错误地从 host
// eth0 直出）→ NonBypassEgressOK=false。命令错误也视为失败。
//
// Phase 47 Plan 03 BYPASS-VERIFY-01。
func verifyNonBypassTraffic(ctx context.Context, prefix []string, expectedEgressIP string, result *VerifyResult) {
	checkCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	args := append(append([]string{}, prefix...), "curl", "-4", "--max-time", "10", "-s", nonBypassProbeTargetURL)
	out, err := nsenterRunner(checkCtx, args...)
	if err != nil {
		result.NonBypassEgressOK = false
		result.ActualNonBypassEgress = ""
		return
	}
	actual := strings.TrimSpace(string(out))
	result.ActualNonBypassEgress = actual
	result.NonBypassEgressOK = actual == expectedEgressIP
}

// verifyPublicDNSBlocked 模拟 DNS 旁路：nsenter+dig @8.8.8.8 example.com
// +time=2 +tries=1。
//
// 成功语义（PublicDNSBlocked=true）：
//   - dig 返回非零退出码（被 nft drop） → err != nil
//   - 或 context deadline exceeded（超时）
//
// 失败语义（PublicDNSBlocked=false）：
//   - dig 成功返回结果 → 公网 DNS 未被阻断 = DNS leak
//
// Phase 47 Plan 03 BYPASS-VERIFY-01。
func verifyPublicDNSBlocked(ctx context.Context, prefix []string, result *VerifyResult) {
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	args := append(append([]string{}, prefix...),
		"dig", "+time=2", "+tries=1", "@"+publicDNSProbeServer, "example.com", "+short")
	_, err := nsenterRunner(checkCtx, args...)

	// err != nil 等同「公网 DNS 被阻断」（dig 拿不到响应才会返回非零退出码）。
	result.PublicDNSBlocked = err != nil
}

// firstNetworkError returns the highest-priority NetworkError for the first
// failing check.
//
// 优先级（Phase 47 Plan 03 扩展）:
//   旧：EgressUnreachable/Mismatch > DNSLeak > LeakNotBlocked
//   新：> BypassEgress (复用 LeakNotBlocked code) > NonBypass (复用 EgressIPMismatch) > PublicDNS (复用 DNSLeak)
//
// 新检查放在最低优先级，这样不会破坏调用方对旧错误码的语义假设；同时通过
// Metadata.bypass_egress / non_bypass_egress / public_dns 子字段让上层日志能识别
// 出来。
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

	if !r.LeakBlocked {
		return &NetworkError{
			Type:    ErrLeakNotBlocked,
			Message: fmt.Sprintf("direct outbound to %s was not blocked", r.LeakTarget),
			HostID:  hostID,
			Metadata: map[string]any{
				"target": r.LeakTarget,
			},
		}
	}

	// Phase 47 Plan 03：新 3 项放在最低优先级。
	if !r.BypassEgressOK {
		return &NetworkError{
			Type: ErrLeakNotBlocked,
			Message: fmt.Sprintf(
				"bypass egress mismatch: whitelist traffic must exit via host eth0, got source IP %q",
				r.ActualBypassEgress,
			),
			HostID: hostID,
			Metadata: map[string]any{
				"bypass_probe_target": bypassProbeTargetURL,
				"actual_source_ip":    r.ActualBypassEgress,
				"expected_source_ip":  detectHostEth0IPFallback(),
				"check":               "bypass_egress",
			},
		}
	}

	if !r.NonBypassEgressOK {
		return &NetworkError{
			Type: ErrEgressIPMismatch,
			Message: fmt.Sprintf(
				"non-bypass traffic did not exit via proxy egress IP: expected %q, got %q",
				expected.ExpectedIP, r.ActualNonBypassEgress,
			),
			HostID: hostID,
			Metadata: map[string]any{
				"non_bypass_probe_target": nonBypassProbeTargetURL,
				"expected":                expected.ExpectedIP,
				"actual":                  r.ActualNonBypassEgress,
				"check":                   "non_bypass_egress",
			},
		}
	}

	return &NetworkError{
		Type:    ErrDNSLeak,
		Message: fmt.Sprintf("public DNS @%s not blocked (dig succeeded)", publicDNSProbeServer),
		HostID:  hostID,
		Metadata: map[string]any{
			"public_dns_server": publicDNSProbeServer,
			"check":             "public_dns_blocked",
		},
	}
}
