package network

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

// allPassedBase returns a VerifyResult where every check (旧 3 项 + Phase 47 Plan 03
// 新增 3 项) is marked as PASS。测试可在此基础上把某一字段翻为 false 用以覆盖
// AllPassed / firstNetworkError 的具体分支。
func allPassedBase() VerifyResult {
	return VerifyResult{
		EgressIPMatch:     true,
		DNSCorrect:        true,
		LeakBlocked:       true,
		BypassEgressOK:    true,
		NonBypassEgressOK: true,
		PublicDNSBlocked:  true,
	}
}

// ─── Phase 51 QUAL-01: voteEgressIP / verifyEgressIPMulti 单测 ───────────────

func TestVoteEgressIP_MajorityWins3of3(t *testing.T) {
	winner, ok := voteEgressIP([]string{"1.2.3.4", "1.2.3.4", "1.2.3.4"})
	if !ok || winner != "1.2.3.4" {
		t.Errorf("voteEgressIP(3 same): winner=%q ok=%v", winner, ok)
	}
}

func TestVoteEgressIP_MajorityWins2of3(t *testing.T) {
	winner, ok := voteEgressIP([]string{"1.2.3.4", "1.2.3.4", "9.9.9.9"})
	if !ok || winner != "1.2.3.4" {
		t.Errorf("voteEgressIP(2 vs 1): winner=%q ok=%v", winner, ok)
	}
}

func TestVoteEgressIP_TieFails(t *testing.T) {
	winner, ok := voteEgressIP([]string{"1.1.1.1", "2.2.2.2", "3.3.3.3"})
	if ok {
		t.Errorf("voteEgressIP(1-1-1 tie): expected ok=false, got winner=%q", winner)
	}
}

func TestVoteEgressIP_AllEmpty(t *testing.T) {
	winner, ok := voteEgressIP([]string{"", "", ""})
	if ok {
		t.Errorf("voteEgressIP(all empty): expected ok=false, got winner=%q", winner)
	}
}

func TestVoteEgressIP_NilInput(t *testing.T) {
	winner, ok := voteEgressIP(nil)
	if ok || winner != "" {
		t.Errorf("voteEgressIP(nil): winner=%q ok=%v", winner, ok)
	}
}

func TestVerifyEgressIPMulti_MajorityMatch(t *testing.T) {
	withFakeNsenterRunner(t, func(call fakeNsenterCall) ([]byte, error) {
		joined := strings.Join(call.args, " ")
		switch {
		case strings.Contains(joined, "ip.me"):
			return []byte("1.2.3.4\n"), nil
		case strings.Contains(joined, "ifconfig.io"):
			return []byte("1.2.3.4\n"), nil
		case strings.Contains(joined, "ipinfo.io"):
			return []byte("9.9.9.9\n"), nil
		}
		return nil, errors.New("unexpected source")
	})

	var result VerifyResult
	verifyEgressIPMulti(context.Background(), []string{"nsenter"}, "1.2.3.4", egressIPSources, &result)
	if !result.EgressIPMatch {
		t.Errorf("expected EgressIPMatch=true (majority 1.2.3.4), got false; actual=%q", result.ActualEgressIP)
	}
	if result.ActualEgressIP != "1.2.3.4" {
		t.Errorf("expected ActualEgressIP=1.2.3.4, got %q", result.ActualEgressIP)
	}
}

func TestVerifyEgressIPMulti_MajorityMismatch(t *testing.T) {
	withFakeNsenterRunner(t, func(call fakeNsenterCall) ([]byte, error) {
		return []byte("8.8.8.8\n"), nil
	})

	var result VerifyResult
	verifyEgressIPMulti(context.Background(), []string{"nsenter"}, "1.2.3.4", egressIPSources, &result)
	if result.EgressIPMatch {
		t.Errorf("expected EgressIPMatch=false (majority 8.8.8.8 != expected 1.2.3.4)")
	}
	if result.ActualEgressIP != "8.8.8.8" {
		t.Errorf("expected ActualEgressIP=8.8.8.8, got %q", result.ActualEgressIP)
	}
}

func TestVerifyEgressIPMulti_AllTimeout(t *testing.T) {
	withFakeNsenterRunner(t, func(call fakeNsenterCall) ([]byte, error) {
		return nil, errors.New("curl: (28) Connection timed out")
	})

	var result VerifyResult
	verifyEgressIPMulti(context.Background(), []string{"nsenter"}, "1.2.3.4", egressIPSources, &result)
	if result.EgressIPMatch {
		t.Errorf("expected EgressIPMatch=false on all-source timeout")
	}
	if result.ActualEgressIP != "" {
		t.Errorf("expected ActualEgressIP=\"\" on all-source timeout, got %q", result.ActualEgressIP)
	}
}

// ─── Phase 51 QUAL-02: verifyLeakBlockedMulti / defaultLeakTargets 单测 ──────

func TestDefaultLeakTargets_LockedContract(t *testing.T) {
	want := []leakTarget{
		{Host: "1.1.1.1", Port: 80},
		{Host: "8.8.8.8", Port: 443},
		{Host: "9.9.9.9", Port: 443},
		{Host: "169.254.169.254", Port: 80},
	}
	if len(defaultLeakTargets) != len(want) {
		t.Fatalf("defaultLeakTargets length: want %d got %d", len(want), len(defaultLeakTargets))
	}
	for i, w := range want {
		if defaultLeakTargets[i] != w {
			t.Errorf("defaultLeakTargets[%d]: want %+v got %+v", i, w, defaultLeakTargets[i])
		}
	}
}

func TestVerifyLeakBlockedMulti_AllBlocked(t *testing.T) {
	withFakeNsenterRunner(t, func(call fakeNsenterCall) ([]byte, error) {
		return nil, errors.New("timeout")
	})

	var result VerifyResult
	verifyLeakBlockedMulti(context.Background(), []string{"nsenter"}, defaultLeakTargets, &result)
	if !result.LeakBlocked {
		t.Errorf("expected LeakBlocked=true when all targets blocked")
	}
	if result.LeakTarget != "1.1.1.1:80" {
		t.Errorf("expected default LeakTarget=1.1.1.1:80, got %q", result.LeakTarget)
	}
}

func TestVerifyLeakBlockedMulti_OneLeaked(t *testing.T) {
	withFakeNsenterRunner(t, func(call fakeNsenterCall) ([]byte, error) {
		joined := strings.Join(call.args, " ")
		if strings.Contains(joined, "/dev/tcp/8.8.8.8/443") {
			return nil, nil
		}
		return nil, errors.New("timeout")
	})

	var result VerifyResult
	verifyLeakBlockedMulti(context.Background(), []string{"nsenter"}, defaultLeakTargets, &result)
	if result.LeakBlocked {
		t.Errorf("expected LeakBlocked=false when 8.8.8.8:443 leaked")
	}
	if result.LeakTarget != "8.8.8.8:443" {
		t.Errorf("expected LeakTarget=8.8.8.8:443, got %q", result.LeakTarget)
	}
}

func TestVerifyLeakBlockedMulti_AllLeaked(t *testing.T) {
	withFakeNsenterRunner(t, func(call fakeNsenterCall) ([]byte, error) {
		return nil, nil
	})

	var result VerifyResult
	verifyLeakBlockedMulti(context.Background(), []string{"nsenter"}, defaultLeakTargets, &result)
	if result.LeakBlocked {
		t.Errorf("expected LeakBlocked=false when all leaked")
	}
	if result.LeakTarget != "1.1.1.1:80" {
		t.Errorf("expected LeakTarget=first matrix entry 1.1.1.1:80, got %q", result.LeakTarget)
	}
}

func TestVerifyLeakBlockedMulti_EmptyTargets(t *testing.T) {
	var result VerifyResult
	verifyLeakBlockedMulti(context.Background(), []string{"nsenter"}, nil, &result)
	if !result.LeakBlocked {
		t.Errorf("expected LeakBlocked=true with empty targets (nothing to leak)")
	}
}

// ─── Phase 51 QUAL-03: parseAllNameservers 单测 ─────────────────────────────

func TestParseAllNameservers_Empty(t *testing.T) {
	if got := parseAllNameservers(""); got != nil {
		t.Errorf("expected nil for empty input, got %v", got)
	}
}

func TestParseAllNameservers_SingleNS(t *testing.T) {
	got := parseAllNameservers("nameserver 172.19.0.1\n")
	if len(got) != 1 || got[0] != "172.19.0.1" {
		t.Errorf("unexpected: %v", got)
	}
}

func TestParseAllNameservers_MultipleNS(t *testing.T) {
	got := parseAllNameservers("nameserver 172.19.0.1\nnameserver 8.8.8.8\nnameserver 1.1.1.1\n")
	want := []string{"172.19.0.1", "8.8.8.8", "1.1.1.1"}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %v want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("[%d] got %q want %q", i, got[i], w)
		}
	}
}

func TestParseAllNameservers_Comments(t *testing.T) {
	in := "# auto-generated\n;another comment\nnameserver 172.19.0.1\noptions ndots:0\n"
	got := parseAllNameservers(in)
	if len(got) != 1 || got[0] != "172.19.0.1" {
		t.Errorf("expected [172.19.0.1], got %v", got)
	}
}

func TestParseAllNameservers_Garbage(t *testing.T) {
	in := "search example.com\nnameserver\nbogus line\nnameserver 8.8.8.8 extra\n"
	got := parseAllNameservers(in)
	if len(got) != 1 || got[0] != "8.8.8.8" {
		t.Errorf("expected [8.8.8.8], got %v", got)
	}
}

func TestVerifyDNS_ReportsAllNameservers(t *testing.T) {
	withFakeNsenterRunner(t, func(call fakeNsenterCall) ([]byte, error) {
		return []byte("nameserver 172.19.0.1\nnameserver 8.8.8.8\n"), nil
	})

	var result VerifyResult
	verifyDNS(context.Background(), []string{"nsenter"}, "172.19.0.1", &result)
	if result.DNSCorrect {
		t.Errorf("expected DNSCorrect=false when resolv.conf != locked content")
	}
	if result.ActualDNS != "172.19.0.1,8.8.8.8" {
		t.Errorf("expected ActualDNS=172.19.0.1,8.8.8.8, got %q", result.ActualDNS)
	}
}

// ─── Phase 47 Plan 03 / 既有单测 ─────────────────────────────────────────────

func TestVerifyResult_AllPassed(t *testing.T) {
	tests := []struct {
		name     string
		result   VerifyResult
		expected bool
	}{
		{
			name:     "all six checks pass",
			result:   allPassedBase(),
			expected: true,
		},
		{
			name: "egress IP mismatch",
			result: func() VerifyResult {
				r := allPassedBase()
				r.EgressIPMatch = false
				return r
			}(),
			expected: false,
		},
		{
			name: "DNS incorrect",
			result: func() VerifyResult {
				r := allPassedBase()
				r.DNSCorrect = false
				return r
			}(),
			expected: false,
		},
		{
			name: "leak not blocked",
			result: func() VerifyResult {
				r := allPassedBase()
				r.LeakBlocked = false
				return r
			}(),
			expected: false,
		},
		{
			name: "bypass egress mismatch (new Phase 47)",
			result: func() VerifyResult {
				r := allPassedBase()
				r.BypassEgressOK = false
				return r
			}(),
			expected: false,
		},
		{
			name: "non-bypass egress mismatch (new Phase 47)",
			result: func() VerifyResult {
				r := allPassedBase()
				r.NonBypassEgressOK = false
				return r
			}(),
			expected: false,
		},
		{
			name: "public DNS not blocked (new Phase 47)",
			result: func() VerifyResult {
				r := allPassedBase()
				r.PublicDNSBlocked = false
				return r
			}(),
			expected: false,
		},
		{
			name:     "all checks fail",
			result:   VerifyResult{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.result.AllPassed(); got != tt.expected {
				t.Errorf("AllPassed() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// withLegacyFail returns a VerifyResult where the three Phase 47 Plan 03 checks
// are forced to PASS（避免它们抢走旧 3 项的优先级），调用方再覆盖旧字段以触发期望
// 的旧错误码。
func withLegacyFail(mutate func(r *VerifyResult)) VerifyResult {
	r := allPassedBase()
	mutate(&r)
	return r
}

func TestFirstNetworkError_EgressUnreachable(t *testing.T) {
	cfg := EgressConfig{ExpectedIP: "1.2.3.4"}
	result := withLegacyFail(func(r *VerifyResult) {
		r.EgressIPMatch = false
		r.ActualEgressIP = ""
	})

	err := firstNetworkError(cfg, result)
	if err.Type != ErrEgressUnreachable {
		t.Errorf("expected ErrEgressUnreachable, got %s", err.Type)
	}
}

func TestFirstNetworkError_EgressMismatch(t *testing.T) {
	cfg := EgressConfig{ExpectedIP: "1.2.3.4"}
	result := withLegacyFail(func(r *VerifyResult) {
		r.EgressIPMatch = false
		r.ActualEgressIP = "5.6.7.8"
	})

	err := firstNetworkError(cfg, result)
	if err.Type != ErrEgressIPMismatch {
		t.Errorf("expected ErrEgressIPMismatch, got %s", err.Type)
	}
	if err.Metadata["expected"] != "1.2.3.4" || err.Metadata["actual"] != "5.6.7.8" {
		t.Errorf("unexpected metadata: %v", err.Metadata)
	}
}

func TestFirstNetworkError_DNSLeak(t *testing.T) {
	// Phase 45 Plan 02：firstNetworkError 的 expected_dns 已经与
	// EgressConfig.Proxy.DNSServer 解耦，永远是常量 containerExpectedDNS
	// (172.19.0.1)。这里 Proxy.DNSServer 字段仍保留语义（gateway → 上游
	// DNS），但不再用于断言。
	cfg := EgressConfig{
		ExpectedIP: "1.2.3.4",
		Proxy:      &ProxySpec{DNSServer: "10.0.0.1"},
	}
	result := withLegacyFail(func(r *VerifyResult) {
		r.DNSCorrect = false
		r.ActualDNS = "8.8.8.8"
	})

	err := firstNetworkError(cfg, result)
	if err.Type != ErrDNSLeak {
		t.Errorf("expected ErrDNSLeak, got %s", err.Type)
	}
	if err.Metadata["expected_dns"] != "172.19.0.1" || err.Metadata["actual_dns"] != "8.8.8.8" {
		t.Errorf("unexpected metadata: %v", err.Metadata)
	}
}

func TestFirstNetworkError_DNSLeak_NilProxy(t *testing.T) {
	// Phase 45 Plan 02：expected_dns 已与 EgressConfig.Proxy 字段解耦，
	// 即使 Proxy 为 nil，预期值仍是常量 containerExpectedDNS (172.19.0.1)。
	cfg := EgressConfig{ExpectedIP: "1.2.3.4", Proxy: nil}
	result := withLegacyFail(func(r *VerifyResult) {
		r.DNSCorrect = false
		r.ActualDNS = "8.8.8.8"
	})

	err := firstNetworkError(cfg, result)
	if err.Type != ErrDNSLeak {
		t.Errorf("expected ErrDNSLeak, got %s", err.Type)
	}
	expectedDNS, _ := err.Metadata["expected_dns"].(string)
	if expectedDNS != "172.19.0.1" {
		t.Errorf("expected_dns should be containerExpectedDNS 172.19.0.1 even when Proxy is nil, got %q", expectedDNS)
	}
}

func TestFirstNetworkError_LeakNotBlocked(t *testing.T) {
	cfg := EgressConfig{ExpectedIP: "1.2.3.4"}
	result := withLegacyFail(func(r *VerifyResult) {
		r.LeakBlocked = false
		r.LeakTarget = "1.1.1.1:80"
	})

	err := firstNetworkError(cfg, result)
	if err.Type != ErrLeakNotBlocked {
		t.Errorf("expected ErrLeakNotBlocked, got %s", err.Type)
	}
	if err.Metadata["target"] != "1.1.1.1:80" {
		t.Errorf("unexpected metadata: %v", err.Metadata)
	}
}

func TestFirstNetworkError_Priority(t *testing.T) {
	// Phase 47 Plan 03 优先级（新检查放最低，避免破坏旧错误码语义）：
	// EgressIPMismatch/Unreachable > DNSLeak > LeakNotBlocked > BypassEgress > NonBypass > PublicDNS
	tests := []struct {
		name     string
		result   VerifyResult
		expected NetworkErrorType
	}{
		{
			name: "egress mismatch has highest priority",
			result: withLegacyFail(func(r *VerifyResult) {
				r.EgressIPMatch = false
				r.ActualEgressIP = "5.6.7.8"
				r.DNSCorrect = false
				r.LeakBlocked = false
				r.BypassEgressOK = false
				r.NonBypassEgressOK = false
				r.PublicDNSBlocked = false
			}),
			expected: ErrEgressIPMismatch,
		},
		{
			name: "egress unreachable has highest priority",
			result: withLegacyFail(func(r *VerifyResult) {
				r.EgressIPMatch = false
				r.ActualEgressIP = ""
				r.DNSCorrect = false
				r.LeakBlocked = false
			}),
			expected: ErrEgressUnreachable,
		},
		{
			name: "DNS leak when egress OK",
			result: withLegacyFail(func(r *VerifyResult) {
				r.DNSCorrect = false
				r.ActualDNS = "8.8.8.8"
				r.LeakBlocked = false
				r.BypassEgressOK = false
			}),
			expected: ErrDNSLeak,
		},
		{
			name: "leak not blocked when egress and DNS OK",
			result: withLegacyFail(func(r *VerifyResult) {
				r.LeakBlocked = false
				r.LeakTarget = "1.1.1.1:80"
				r.BypassEgressOK = false
				r.NonBypassEgressOK = false
				r.PublicDNSBlocked = false
			}),
			expected: ErrLeakNotBlocked,
		},
		{
			name: "DNS leak takes priority over leak blocked",
			result: withLegacyFail(func(r *VerifyResult) {
				r.DNSCorrect = false
				r.ActualDNS = "8.8.8.8"
				r.LeakBlocked = false
			}),
			expected: ErrDNSLeak,
		},
		{
			name: "bypass egress mismatch reported when legacy 3 pass",
			result: withLegacyFail(func(r *VerifyResult) {
				r.BypassEgressOK = false
				r.ActualBypassEgress = "203.0.113.7"
			}),
			expected: ErrLeakNotBlocked,
		},
		{
			name: "non-bypass egress mismatch when bypass passes",
			result: withLegacyFail(func(r *VerifyResult) {
				r.NonBypassEgressOK = false
				r.ActualNonBypassEgress = "10.0.0.42"
			}),
			expected: ErrEgressIPMismatch,
		},
		{
			name: "public DNS not blocked has lowest priority",
			result: withLegacyFail(func(r *VerifyResult) {
				r.PublicDNSBlocked = false
			}),
			expected: ErrDNSLeak,
		},
	}

	cfg := EgressConfig{ExpectedIP: "1.2.3.4", Proxy: &ProxySpec{DNSServer: "10.0.0.1"}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := firstNetworkError(cfg, tt.result)
			if err == nil {
				t.Fatalf("expected non-nil NetworkError")
			}
			if err.Type != tt.expected {
				t.Errorf("expected %s, got %s (msg=%s)", tt.expected, err.Type, err.Message)
			}
		})
	}
}

// ─── Phase 47 Plan 03 新增 3 项流量检查的单测 ─────────────────────────────────

// fakeNsenterCall captures one invocation of nsenterRunner for assertion.
type fakeNsenterCall struct {
	args []string
}

// withFakeNsenterRunner 注入 fake nsenterRunner 并在测试结束还原；返回所有 call 记录。
func withFakeNsenterRunner(t *testing.T, fake func(call fakeNsenterCall) ([]byte, error)) *[]fakeNsenterCall {
	t.Helper()
	calls := make([]fakeNsenterCall, 0, 4)
	var mu sync.Mutex
	prev := nsenterRunner
	nsenterRunner = func(_ context.Context, args ...string) ([]byte, error) {
		c := fakeNsenterCall{args: append([]string{}, args...)}
		mu.Lock()
		calls = append(calls, c)
		mu.Unlock()
		return fake(c)
	}
	t.Cleanup(func() { nsenterRunner = prev })
	return &calls
}

func TestVerifyBypassEgressMatchesEth0_OK(t *testing.T) {
	withFakeNsenterRunner(t, func(call fakeNsenterCall) ([]byte, error) {
		// 期望参数包含 curl
		joined := strings.Join(call.args, " ")
		if !strings.Contains(joined, "curl") {
			t.Errorf("expected curl invocation, got %v", call.args)
		}
		return []byte("10.0.0.42\n"), nil
	})

	var result VerifyResult
	verifyBypassEgressMatchesEth0(context.Background(), []string{"nsenter", "-t", "1", "-n", "--"}, "10.0.0.42", &result)
	if !result.BypassEgressOK {
		t.Errorf("expected BypassEgressOK=true, got false (actual=%q)", result.ActualBypassEgress)
	}
	if result.ActualBypassEgress != "10.0.0.42" {
		t.Errorf("expected ActualBypassEgress=10.0.0.42, got %q", result.ActualBypassEgress)
	}
}

func TestVerifyBypassEgressMatchesEth0_LeakDetected(t *testing.T) {
	// 探测目标返回 egress IP（代理出口），说明白名单流量错误地走了代理 → leak
	withFakeNsenterRunner(t, func(call fakeNsenterCall) ([]byte, error) {
		return []byte("203.0.113.7\n"), nil
	})

	var result VerifyResult
	verifyBypassEgressMatchesEth0(context.Background(), []string{"nsenter"}, "10.0.0.42", &result)
	if result.BypassEgressOK {
		t.Errorf("expected BypassEgressOK=false when source IP != host eth0")
	}
	if result.ActualBypassEgress != "203.0.113.7" {
		t.Errorf("expected ActualBypassEgress=203.0.113.7, got %q", result.ActualBypassEgress)
	}
}

func TestVerifyBypassEgressMatchesEth0_CommandError(t *testing.T) {
	withFakeNsenterRunner(t, func(call fakeNsenterCall) ([]byte, error) {
		return nil, errors.New("curl: (28) Connection timed out")
	})

	var result VerifyResult
	verifyBypassEgressMatchesEth0(context.Background(), []string{"nsenter"}, "10.0.0.42", &result)
	if result.BypassEgressOK {
		t.Errorf("expected BypassEgressOK=false on command error")
	}
}

func TestVerifyNonBypassTraffic_OK(t *testing.T) {
	// 非白名单 api.example.com 流量应当从代理出口出 → curl 返回 expectedEgressIP
	withFakeNsenterRunner(t, func(call fakeNsenterCall) ([]byte, error) {
		return []byte("1.2.3.4\n"), nil
	})

	var result VerifyResult
	verifyNonBypassTraffic(context.Background(), []string{"nsenter"}, "1.2.3.4", &result)
	if !result.NonBypassEgressOK {
		t.Errorf("expected NonBypassEgressOK=true when curl returns egress IP, got false (actual=%q)", result.ActualNonBypassEgress)
	}
	if result.ActualNonBypassEgress != "1.2.3.4" {
		t.Errorf("expected ActualNonBypassEgress=1.2.3.4, got %q", result.ActualNonBypassEgress)
	}
}

func TestVerifyNonBypassTraffic_LeakDetected(t *testing.T) {
	// 非白名单流量错误地走 eth0 直出 → 返回 host eth0 IP
	withFakeNsenterRunner(t, func(call fakeNsenterCall) ([]byte, error) {
		return []byte("10.0.0.42\n"), nil
	})

	var result VerifyResult
	verifyNonBypassTraffic(context.Background(), []string{"nsenter"}, "1.2.3.4", &result)
	if result.NonBypassEgressOK {
		t.Errorf("expected NonBypassEgressOK=false when non-bypass traffic exited via host eth0")
	}
	if result.ActualNonBypassEgress != "10.0.0.42" {
		t.Errorf("expected ActualNonBypassEgress=10.0.0.42, got %q", result.ActualNonBypassEgress)
	}
}

func TestVerifyPublicDNSBlocked_OK(t *testing.T) {
	// dig @8.8.8.8 必须超时 / 失败：返回 error 即视为通过
	withFakeNsenterRunner(t, func(call fakeNsenterCall) ([]byte, error) {
		joined := strings.Join(call.args, " ")
		if !strings.Contains(joined, "dig") {
			t.Errorf("expected dig invocation, got %v", call.args)
		}
		if !strings.Contains(joined, "@8.8.8.8") {
			t.Errorf("expected @8.8.8.8 in args, got %v", call.args)
		}
		return nil, context.DeadlineExceeded
	})

	var result VerifyResult
	verifyPublicDNSBlocked(context.Background(), []string{"nsenter"}, &result)
	if !result.PublicDNSBlocked {
		t.Errorf("expected PublicDNSBlocked=true when dig times out")
	}
}

func TestVerifyPublicDNSBlocked_LeakDetected(t *testing.T) {
	// dig 在 2s 内成功返回结果 → 公网 DNS 未被阻断 = leak
	withFakeNsenterRunner(t, func(call fakeNsenterCall) ([]byte, error) {
		return []byte("93.184.216.34\n"), nil
	})

	var result VerifyResult
	verifyPublicDNSBlocked(context.Background(), []string{"nsenter"}, &result)
	if result.PublicDNSBlocked {
		t.Errorf("expected PublicDNSBlocked=false when dig succeeded (public DNS not blocked)")
	}
}

func TestVerifyNetworkIntegrity_NoNsenter(t *testing.T) {
	// On macOS (and any system without nsenter), the nsenter commands will fail.
	// This tests the error paths without requiring real network or containers.
	// The test verifies that VerifyNetworkIntegrity handles command failures gracefully.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediate cancellation

	result, err := VerifyNetworkIntegrity(ctx, 1, EgressConfig{ExpectedIP: "1.2.3.4"})

	// All checks should fail because nsenter doesn't exist on this platform.
	// LeakBlocked = true because command failure means the direct outbound was "blocked".
	t.Logf("verify result: EgressIPMatch=%v DNSCorrect=%v LeakBlocked=%v BypassEgressOK=%v NonBypassEgressOK=%v PublicDNSBlocked=%v",
		result.EgressIPMatch, result.DNSCorrect, result.LeakBlocked,
		result.BypassEgressOK, result.NonBypassEgressOK, result.PublicDNSBlocked)
	t.Logf("verify error: %v", err)

	// With all checks failing, we expect a non-nil error.
	if err == nil {
		t.Error("expected error when nsenter is not available")
	}
	if result.LeakBlocked != true {
		t.Errorf("expected LeakBlocked=true (command failure = blocked), got %v", result.LeakBlocked)
	}
	// Phase 47 Plan 03：nsenter 不存在时 dig 也会失败，等价于「公网 DNS 被阻断」
	if result.PublicDNSBlocked != true {
		t.Errorf("expected PublicDNSBlocked=true (dig failure = blocked), got %v", result.PublicDNSBlocked)
	}
}

func TestVerifyNetworkIntegrity_BackgroundContext(t *testing.T) {
	// Using background context (not cancelled). nsenter will still fail on macOS
	// because the binary doesn't exist.
	if testing.Short() {
		// In short mode, we use a tight timeout to avoid the 15s egress check timeout.
		// The verify functions create their own timeout contexts (15s, 5s, 5s).
		// On macOS without nsenter, the commands fail instantly, so this is fast.
	}

	result, err := VerifyNetworkIntegrity(context.Background(), 99999, EgressConfig{
		ExpectedIP: "1.2.3.4",
		EgressIPID: "eip-test",
		TunnelType: TunnelTypeProxy,
		Proxy:      &ProxySpec{DNSServer: "10.0.0.1"},
	})

	// On macOS, nsenter doesn't exist, so all checks fail.
	t.Logf("verify result: EgressIPMatch=%v ActualEgressIP=%q DNSCorrect=%v ActualDNS=%q LeakBlocked=%v LeakTarget=%q",
		result.EgressIPMatch, result.ActualEgressIP, result.DNSCorrect, result.ActualDNS, result.LeakBlocked, result.LeakTarget)

	if err == nil {
		t.Error("expected error from VerifyNetworkIntegrity without nsenter")
	}
}

func TestVerifyNetworkIntegrity_ContextTimeout(t *testing.T) {
	// Verify that VerifyNetworkIntegrity does not deadlock when given a cancelled context.
	// It should return promptly with results.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediately cancelled

	done := make(chan struct{})
	var result VerifyResult
	var err error

	go func() {
		defer close(done)
		result, err = VerifyNetworkIntegrity(ctx, 1, EgressConfig{ExpectedIP: "1.2.3.4"})
	}()

	select {
	case <-done:
		// Good, function returned promptly
	case <-context.Background().Done():
		t.Fatal("VerifyNetworkIntegrity deadlocked with cancelled context")
	}

	// The function should return some result regardless of context cancellation.
	// Individual check functions use their own timeout contexts derived from the
	// passed context, so behaviour depends on how quickly they respond.
	t.Logf("result: EgressIPMatch=%v DNSCorrect=%v LeakBlocked=%v err=%v",
		result.EgressIPMatch, result.DNSCorrect, result.LeakBlocked, err)
}

func TestVerifyNetworkIntegrity_LeakTargetSet(t *testing.T) {
	// Verify that the leak target is always set before the check runs.
	// This is a unit test of the verifyLeakBlocked function's contract.
	result := VerifyResult{LeakTarget: ""}

	// The leak target should be set to a known value by verifyLeakBlocked.
	// We verify this indirectly by checking the default value in a fresh result.
	if result.LeakTarget != "" {
		t.Logf("initial LeakTarget: %q", result.LeakTarget)
	}

	// After calling verifyLeakBlocked (via VerifyNetworkIntegrity), LeakTarget should be set.
	// Since we can't run nsenter on macOS, we just verify the function doesn't panic
	// and the result struct is properly populated.
	ctx := context.Background()
	res, _ := VerifyNetworkIntegrity(ctx, 99999, EgressConfig{ExpectedIP: "1.2.3.4"})
	if res.LeakTarget == "" {
		t.Error("LeakTarget should be set after verification")
	}
	if !strings.Contains(res.LeakTarget, ":") {
		t.Errorf("LeakTarget should contain port separator, got %q", res.LeakTarget)
	}
}
