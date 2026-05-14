// helpers_test.go 是 Phase 46 纯函数 unit test。
//
// 这是 Phase 46 验收的 darwin 基线：
//   - 不带任何 build tag，darwin 上 `go test ./tests/e2e/ -run Helpers -count=1`
//     必须 100% 绿。
//   - 不依赖 docker / linux netns / testcontainers / 控制面子进程。
//   - 覆盖 Vote / ClassifyDNSResult / SummarizeDenyResults / DefaultDenyMatrix
//     /BuildDenyProbeCmd / BootstrapExitCodeContract 6 个纯函数 / 锁定表的
//     全部行为分支。
//
// Linux 真机 e2e 断言（FetchEgressIPInContainer / RunBootstrapScript /
// StartGoldenPath）由 CI runner 跑，列为 deferred-to-CI 项。

package e2e

import (
	"errors"
	"strings"
	"testing"
	"time"

	bootstraperrors "github.com/zanel1u/cloud-cli-proxy/internal/controlplane/http"
)

// ─── MVS-02 / Vote 多数派裁决 ─────────────────────────────────────────

func TestHelpersVote_Majority(t *testing.T) {
	got := Vote([]string{"1.2.3.4", "1.2.3.4", "5.6.7.8"})
	if !got.OK {
		t.Fatalf("expected ok=true, got %+v", got)
	}
	if got.Winner != "1.2.3.4" {
		t.Fatalf("expected winner=1.2.3.4, got %q", got.Winner)
	}
	if len(got.Dissent) != 1 || got.Dissent[0] != "5.6.7.8" {
		t.Fatalf("expected dissent=[5.6.7.8], got %v", got.Dissent)
	}
}

func TestHelpersVote_AllAgree(t *testing.T) {
	got := Vote([]string{"10.0.0.1", "10.0.0.1", "10.0.0.1"})
	if !got.OK || got.Winner != "10.0.0.1" || len(got.Dissent) != 0 {
		t.Fatalf("expected unanimous, got %+v", got)
	}
}

func TestHelpersVote_AllAbstain(t *testing.T) {
	got := Vote([]string{"", "", ""})
	if got.OK || got.Winner != "" || len(got.Dissent) != 0 {
		t.Fatalf("expected all-abstain, got %+v", got)
	}
}

func TestHelpersVote_NilInput(t *testing.T) {
	got := Vote(nil)
	if got.OK || got.Winner != "" || len(got.Dissent) != 0 {
		t.Fatalf("expected nil → all-abstain, got %+v", got)
	}
}

func TestHelpersVote_AllDistinct(t *testing.T) {
	got := Vote([]string{"a", "b", "c"})
	if got.OK {
		t.Fatalf("expected ok=false (no majority), got %+v", got)
	}
	if got.Winner != "" {
		t.Fatalf("expected winner empty on no-majority, got %q", got.Winner)
	}
	if len(got.Dissent) != 3 {
		t.Fatalf("expected dissent=3, got %v", got.Dissent)
	}
}

func TestHelpersVote_PartialAbstain(t *testing.T) {
	got := Vote([]string{"x", "x", ""})
	if !got.OK || got.Winner != "x" || len(got.Dissent) != 0 {
		t.Fatalf("expected ok=true winner=x dissent=[], got %+v", got)
	}
}

func TestHelpersEgressIPSources_Locked(t *testing.T) {
	got := EgressIPSources()
	want := []string{"https://ip.me", "https://ifconfig.io", "https://ipinfo.io/ip"}
	if len(got) != len(want) {
		t.Fatalf("expected %d sources, got %d", len(want), len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("source[%d]: want %q got %q", i, want[i], got[i])
		}
	}

	// 防御 mutate：修改返回切片不应污染后续调用。
	got[0] = "modified"
	again := EgressIPSources()
	if again[0] != "https://ip.me" {
		t.Fatalf("EgressIPSources mutated: got[0]=%q", again[0])
	}
}

// ─── MVS-03 / DNS 分类 ─────────────────────────────────────────────────

func TestHelpersClassifyDNS_TunneledOnExitZero(t *testing.T) {
	got := ClassifyDNSResult(0, "")
	if got != DNSResultTunneled {
		t.Fatalf("expected Tunneled, got %s", got)
	}
}

func TestHelpersClassifyDNS_DeniedConnectionRefused(t *testing.T) {
	got := ClassifyDNSResult(1, "curl: (7) Failed to connect: Connection refused")
	if got != DNSResultDenied {
		t.Fatalf("expected Denied, got %s", got)
	}
}

func TestHelpersClassifyDNS_DeniedTimeout(t *testing.T) {
	got := ClassifyDNSResult(28, "curl: (28) Operation timed out after 5000ms")
	if got != DNSResultDenied {
		t.Fatalf("expected Denied on timeout, got %s", got)
	}
}

func TestHelpersClassifyDNS_DeniedNameResolution(t *testing.T) {
	got := ClassifyDNSResult(6, "getaddrinfo: Name or service not known")
	if got != DNSResultDenied {
		t.Fatalf("expected Denied on name resolution failure, got %s", got)
	}
}

func TestHelpersClassifyDNS_DeniedNetworkUnreachable(t *testing.T) {
	got := ClassifyDNSResult(1, "connect: Network is unreachable")
	if got != DNSResultDenied {
		t.Fatalf("expected Denied on unreachable, got %s", got)
	}
}

func TestHelpersClassifyDNS_UnknownOnGenericError(t *testing.T) {
	got := ClassifyDNSResult(99, "something weird happened")
	if got != DNSResultUnknown {
		t.Fatalf("expected Unknown on unparseable error, got %s", got)
	}
}

func TestHelpersDNSProbeResult_StringForm(t *testing.T) {
	cases := map[DNSProbeResult]string{
		DNSResultUnknown:  "Unknown",
		DNSResultTunneled: "Tunneled",
		DNSResultDenied:   "Denied",
		DNSResultLeaked:   "Leaked",
	}
	for k, want := range cases {
		if got := k.String(); got != want {
			t.Fatalf("DNSProbeResult(%d).String(): want %q got %q", k, want, got)
		}
	}
}

// ─── MVS-04 / 默认拒绝矩阵 ─────────────────────────────────────────────

func TestHelpersDefaultDenyMatrix_Locked(t *testing.T) {
	want := []DenyTarget{
		{"1.1.1.1", 80},
		{"8.8.8.8", 443},
		{"9.9.9.9", 443},
		{"169.254.169.254", 80},
	}
	if len(DefaultDenyMatrix) != len(want) {
		t.Fatalf("DefaultDenyMatrix length: want %d got %d", len(want), len(DefaultDenyMatrix))
	}
	for i, w := range want {
		if DefaultDenyMatrix[i] != w {
			t.Fatalf("DefaultDenyMatrix[%d]: want %+v got %+v", i, w, DefaultDenyMatrix[i])
		}
	}
}

func TestHelpersBuildDenyProbeCmd_Shape(t *testing.T) {
	cmd := BuildDenyProbeCmd(DenyTarget{"1.1.1.1", 80}, 3)
	if len(cmd) != 5 {
		t.Fatalf("expected 5 argv, got %v", cmd)
	}
	if cmd[0] != "timeout" || cmd[1] != "3" || cmd[2] != "bash" || cmd[3] != "-c" {
		t.Fatalf("argv prefix mismatch: %v", cmd)
	}
	if !strings.Contains(cmd[4], "/dev/tcp/1.1.1.1/80") {
		t.Fatalf("expected /dev/tcp/1.1.1.1/80 in shell snippet, got %q", cmd[4])
	}
}

func TestHelpersBuildDenyProbeCmd_DefaultTimeout(t *testing.T) {
	cmd := BuildDenyProbeCmd(DenyTarget{"8.8.8.8", 443}, 0)
	if cmd[1] != "3" {
		t.Fatalf("expected default timeout=3, got %q", cmd[1])
	}
}

func TestHelpersBuildDenyProbeCmd_NegativeTimeout(t *testing.T) {
	cmd := BuildDenyProbeCmd(DenyTarget{"9.9.9.9", 443}, -5)
	if cmd[1] != "3" {
		t.Fatalf("expected negative timeout fallback=3, got %q", cmd[1])
	}
}

func TestHelpersSummarizeDeny_AllDenied(t *testing.T) {
	results := map[DenyTarget]int{
		DefaultDenyMatrix[0]: 1,
		DefaultDenyMatrix[1]: 124,
		DefaultDenyMatrix[2]: 28,
		DefaultDenyMatrix[3]: 142,
	}
	allDenied, leaks := SummarizeDenyResults(results)
	if !allDenied {
		t.Fatalf("expected allDenied=true, got leaks=%v", leaks)
	}
	if len(leaks) != 0 {
		t.Fatalf("expected no leaks, got %v", leaks)
	}
}

func TestHelpersSummarizeDeny_OneLeak(t *testing.T) {
	results := map[DenyTarget]int{
		DefaultDenyMatrix[0]: 1,
		DefaultDenyMatrix[1]: 0, // leak
		DefaultDenyMatrix[2]: 124,
		DefaultDenyMatrix[3]: 1,
	}
	allDenied, leaks := SummarizeDenyResults(results)
	if allDenied {
		t.Fatalf("expected allDenied=false on single leak, got %v", leaks)
	}
	if len(leaks) != 1 || leaks[0] != DefaultDenyMatrix[1] {
		t.Fatalf("expected leak=%v, got %v", DefaultDenyMatrix[1], leaks)
	}
}

func TestHelpersSummarizeDeny_AllLeaks(t *testing.T) {
	results := map[DenyTarget]int{
		DefaultDenyMatrix[0]: 0,
		DefaultDenyMatrix[1]: 0,
		DefaultDenyMatrix[2]: 0,
		DefaultDenyMatrix[3]: 0,
	}
	allDenied, leaks := SummarizeDenyResults(results)
	if allDenied {
		t.Fatalf("expected allDenied=false on all leaks")
	}
	if len(leaks) != len(DefaultDenyMatrix) {
		t.Fatalf("expected %d leaks, got %v", len(DefaultDenyMatrix), leaks)
	}
}

// ─── MVS-05 / Bootstrap 错误码契约 ─────────────────────────────────────

// TestHelpersBootstrapExitCodeContract_AlignsWithSourceOfTruth 是 MVS-05 的
// 锁定门：本地 BootstrapExitCodeContract 必须与
// internal/controlplane/http.BootstrapErrorEntries 中对应 entry 的 ExitCode
// 字段完全一致。任一漂移立即在 darwin 单测层失败，杜绝悄悄改 source-of-truth
// 又忘了同步契约表的回归。
func TestHelpersBootstrapExitCodeContract_AlignsWithSourceOfTruth(t *testing.T) {
	for code, want := range BootstrapExitCodeContract {
		entry, ok := bootstraperrors.BootstrapErrorEntries[code]
		if !ok {
			t.Errorf("source-of-truth missing entry for %q", code)
			continue
		}
		if entry.ExitCode != want {
			t.Errorf("contract[%q]=%d but BootstrapErrorEntries[%q].ExitCode=%d",
				code, want, code, entry.ExitCode)
		}
	}
}

// TestHelpersBootstrapErrorEntries_AtLeastSeven 防止源码里 entry 被误删
// （当前应有 auth_invalid/account_disabled/account_expired/host_not_found/
// start_failed/ssh_not_ready/egress_binding_missing 共 7 条）。
func TestHelpersBootstrapErrorEntries_AtLeastSeven(t *testing.T) {
	got := len(bootstraperrors.BootstrapErrorEntries)
	if got < 7 {
		t.Errorf("expected BootstrapErrorEntries len >= 7, got %d", got)
	}
}

// ─── MVS-06 / 到期治理事件 ─────────────────────────────────────────────

func TestHelpersExpiryEventType_Locked(t *testing.T) {
	if ExpiryEventType != "host.stop.expired" {
		t.Fatalf("ExpiryEventType drifted: got %q want %q", ExpiryEventType, "host.stop.expired")
	}
	if UserExpiredEventType != "user.expired" {
		t.Fatalf("UserExpiredEventType drifted: got %q want %q", UserExpiredEventType, "user.expired")
	}
}

func TestHelpersParseEventTypes_Empty(t *testing.T) {
	_, err := ParseEventTypes(nil)
	if err == nil {
		t.Fatalf("expected error on nil body")
	}
}

func TestHelpersParseEventTypes_EmptyArray(t *testing.T) {
	got, err := ParseEventTypes([]byte(`{"events":[]}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty slice, got %v", got)
	}
}

func TestHelpersParseEventTypes_SingleEvent(t *testing.T) {
	got, err := ParseEventTypes([]byte(`{"events":[{"type":"host.stop.expired","id":"x"}]}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0] != "host.stop.expired" {
		t.Fatalf("expected [host.stop.expired], got %v", got)
	}
}

func TestHelpersParseEventTypes_MultiPreservesOrder(t *testing.T) {
	body := []byte(`{"events":[{"type":"a"},{"type":"b"},{"type":"c"}]}`)
	got, err := ParseEventTypes(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("len=%d want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("idx=%d got %q want %q", i, got[i], want[i])
		}
	}
}

func TestHelpersParseEventTypes_InvalidJSON(t *testing.T) {
	if _, err := ParseEventTypes([]byte(`not a json`)); err == nil {
		t.Fatalf("expected error on invalid JSON")
	}
}

// ─── MVS-07 / 出口 IP 双绑互斥 ──────────────────────────────────────────

func TestHelpersEgressIPDoubleBindContract_Locked(t *testing.T) {
	if EgressIPDoubleBindContract.WantStatus != 409 {
		t.Fatalf("WantStatus drifted: got %d want 409", EgressIPDoubleBindContract.WantStatus)
	}
	if EgressIPDoubleBindContract.WantErrSubstring != "already bound" {
		t.Fatalf("WantErrSubstring drifted: got %q", EgressIPDoubleBindContract.WantErrSubstring)
	}
}

func TestHelpersParseBindEgressIPResponse_Success2xx(t *testing.T) {
	got, err := ParseBindEgressIPResponse(201, []byte(`{"binding":{"id":"x"}}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != 201 || got.ErrorMessage != "" {
		t.Fatalf("expected status=201 empty err, got %+v", got)
	}
	if len(got.RawBody) == 0 {
		t.Fatalf("expected raw body preserved")
	}
}

func TestHelpersParseBindEgressIPResponse_ConflictWithError(t *testing.T) {
	got, err := ParseBindEgressIPResponse(409, []byte(`{"error":"cannot bind egress IP to running host"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != 409 {
		t.Fatalf("expected status=409, got %d", got.Status)
	}
	if !strings.Contains(got.ErrorMessage, "running host") {
		t.Fatalf("error message lost substring: %q", got.ErrorMessage)
	}
}

func TestHelpersParseBindEgressIPResponse_EmptyBody(t *testing.T) {
	got, err := ParseBindEgressIPResponse(204, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != 204 || got.ErrorMessage != "" {
		t.Fatalf("expected empty err on empty body, got %+v", got)
	}
}

func TestHelpersParseBindEgressIPResponse_NonJSONBody(t *testing.T) {
	got, err := ParseBindEgressIPResponse(502, []byte("upstream timeout"))
	if err != nil {
		t.Fatalf("expected nil err (non-JSON tolerated), got %v", err)
	}
	if got.Status != 502 || got.ErrorMessage != "" {
		t.Fatalf("non-JSON body should yield empty ErrorMessage, got %+v", got)
	}
	if string(got.RawBody) != "upstream timeout" {
		t.Fatalf("raw body lost: %q", string(got.RawBody))
	}
}

// ─── MVS-08 / host-agent 心跳与恢复 ─────────────────────────────────────

func TestHelpersHostHealthStatus_String(t *testing.T) {
	cases := map[HostHealthStatus]string{
		HostHealthUnknown:   "Unknown",
		HostHealthHealthy:   "Healthy",
		HostHealthUnhealthy: "Unhealthy",
		HostHealthDegraded:  "Degraded",
	}
	for k, want := range cases {
		if got := k.String(); got != want {
			t.Fatalf("HostHealthStatus(%d).String(): got %q want %q", k, got, want)
		}
	}
}

func TestHelpersParseControlPlaneHealth_OKAgentOK(t *testing.T) {
	body := []byte(`{"status":"ok","checks":{"database":"ok","agent":"ok"}}`)
	overall, agent, err := ParseControlPlaneHealth(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if overall != HostHealthHealthy {
		t.Fatalf("overall=%s want Healthy", overall)
	}
	if agent != HostHealthHealthy {
		t.Fatalf("agent=%s want Healthy", agent)
	}
}

func TestHelpersParseControlPlaneHealth_WarningAgentUnreachable(t *testing.T) {
	body := []byte(`{"status":"warning","checks":{"database":"ok","agent":"unreachable"}}`)
	overall, agent, err := ParseControlPlaneHealth(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if overall != HostHealthUnhealthy {
		t.Fatalf("overall=%s want Unhealthy", overall)
	}
	if agent != HostHealthUnhealthy {
		t.Fatalf("agent=%s want Unhealthy", agent)
	}
}

func TestHelpersParseControlPlaneHealth_DegradedDBError(t *testing.T) {
	body := []byte(`{"status":"degraded","checks":{"database":"connection refused"}}`)
	overall, agent, err := ParseControlPlaneHealth(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if overall != HostHealthDegraded {
		t.Fatalf("overall=%s want Degraded", overall)
	}
	if agent != HostHealthUnknown {
		t.Fatalf("agent=%s want Unknown when checks.agent missing", agent)
	}
}

func TestHelpersParseControlPlaneHealth_MissingChecks(t *testing.T) {
	body := []byte(`{"status":"ok"}`)
	overall, agent, err := ParseControlPlaneHealth(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if overall != HostHealthHealthy {
		t.Fatalf("overall=%s want Healthy", overall)
	}
	if agent != HostHealthUnknown {
		t.Fatalf("agent=%s want Unknown without checks field", agent)
	}
}

func TestHelpersParseControlPlaneHealth_InvalidJSON(t *testing.T) {
	_, _, err := ParseControlPlaneHealth([]byte("not json"))
	if err == nil {
		t.Fatalf("expected error on invalid JSON")
	}
}

func TestHelpersHostHealthRecoveryContract_Locked(t *testing.T) {
	if HostHealthRecoveryContract.UnhealthyWithin != 30*time.Second {
		t.Fatalf("UnhealthyWithin drifted: %v", HostHealthRecoveryContract.UnhealthyWithin)
	}
	if HostHealthRecoveryContract.HealthyWithin != 60*time.Second {
		t.Fatalf("HealthyWithin drifted: %v", HostHealthRecoveryContract.HealthyWithin)
	}
}

// ─── MVS-09 / Kill-switch：sing-box 崩溃断网 ──────────────────────────

func TestHelpersParseTcpdumpCount_FivePackets(t *testing.T) {
	stderr := "tcpdump: listening on eth0, link-type EN10MB, capture size 262144 bytes\n" +
		"5 packets captured\n" +
		"5 packets received by filter\n" +
		"0 packets dropped by kernel\n"
	got, err := ParseTcpdumpCountOutput(stderr)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != 5 {
		t.Fatalf("expected 5, got %d", got)
	}
}

func TestHelpersParseTcpdumpCount_ZeroPackets(t *testing.T) {
	stderr := "tcpdump: listening on eth0\n0 packets captured\n0 packets received by filter\n"
	got, err := ParseTcpdumpCountOutput(stderr)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
}

func TestHelpersParseTcpdumpCount_SingularPacket(t *testing.T) {
	stderr := "1 packet captured\n1 packet received by filter\n"
	got, err := ParseTcpdumpCountOutput(stderr)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != 1 {
		t.Fatalf("expected 1, got %d", got)
	}
}

func TestHelpersParseTcpdumpCount_EmptyStderr(t *testing.T) {
	_, err := ParseTcpdumpCountOutput("")
	if !errors.Is(err, ErrTcpdumpCountNotFound) {
		t.Fatalf("expected ErrTcpdumpCountNotFound, got %v", err)
	}
}

func TestHelpersParseTcpdumpCount_NoMatchSubstring(t *testing.T) {
	stderr := "tcpdump: permission denied (cannot open BPF device)\n"
	_, err := ParseTcpdumpCountOutput(stderr)
	if !errors.Is(err, ErrTcpdumpCountNotFound) {
		t.Fatalf("expected ErrTcpdumpCountNotFound on permission denied stderr, got %v", err)
	}
}

func TestHelpersClassifyKillswitch_OK(t *testing.T) {
	got := ClassifyKillswitchResult(28, 0)
	if got != KillswitchOK {
		t.Fatalf("expected KillswitchOK, got %s", got)
	}
}

func TestHelpersClassifyKillswitch_ProbeUnexpectedlySucceeded(t *testing.T) {
	got := ClassifyKillswitchResult(0, 0)
	if got != KillswitchProbeUnexpectedlySucceeded {
		t.Fatalf("expected KillswitchProbeUnexpectedlySucceeded, got %s", got)
	}
}

func TestHelpersClassifyKillswitch_PacketLeak(t *testing.T) {
	got := ClassifyKillswitchResult(28, 3)
	if got != KillswitchPacketLeak {
		t.Fatalf("expected KillswitchPacketLeak, got %s", got)
	}
}

func TestHelpersClassifyKillswitch_Both(t *testing.T) {
	got := ClassifyKillswitchResult(0, 5)
	if got != KillswitchBoth {
		t.Fatalf("expected KillswitchBoth, got %s", got)
	}
}

func TestHelpersKillswitchVerdict_String(t *testing.T) {
	cases := map[KillswitchVerdict]string{
		KillswitchUnknown:                    "Unknown",
		KillswitchOK:                         "OK",
		KillswitchProbeUnexpectedlySucceeded: "ProbeUnexpectedlySucceeded",
		KillswitchPacketLeak:                 "PacketLeak",
		KillswitchBoth:                       "Both",
	}
	for k, want := range cases {
		if got := k.String(); got != want {
			t.Fatalf("KillswitchVerdict(%d).String(): got %q want %q", k, got, want)
		}
	}
}

func TestHelpersKillswitchTimingContract_Locked(t *testing.T) {
	if KillswitchTimingContract.ProbeMaxLatency != 3*time.Second {
		t.Fatalf("ProbeMaxLatency drifted: %v", KillswitchTimingContract.ProbeMaxLatency)
	}
	if KillswitchTimingContract.TcpdumpWindow != 5*time.Second {
		t.Fatalf("TcpdumpWindow drifted: %v", KillswitchTimingContract.TcpdumpWindow)
	}
}

// ─── MVS-10 / Kill-switch：resolv.conf 篡改免疫 ───────────────────────

func TestHelpersResolvConfTamperResult_String(t *testing.T) {
	cases := map[ResolvConfTamperResult]string{
		TamperUnknown:  "Unknown",
		TamperApplied:  "Applied",
		TamperRejected: "Rejected",
	}
	for k, want := range cases {
		if got := k.String(); got != want {
			t.Fatalf("ResolvConfTamperResult(%d).String(): got %q want %q", k, got, want)
		}
	}
}

func TestHelpersResolvConfTamperContract_Locked(t *testing.T) {
	if ResolvConfTamperContract.Nameserver != "8.8.8.8" {
		t.Fatalf("Nameserver drifted: got %q want 8.8.8.8", ResolvConfTamperContract.Nameserver)
	}
}

func TestHelpersClassifyResolvConfDNS_RejectedAlwaysOK(t *testing.T) {
	// 即使后续 dnsResult / packets 异常，只要篡改被系统拒绝就 ok。
	ok, reason := ClassifyResolvConfDNSOutcome(TamperRejected, DNSResultUnknown, 7)
	if !ok {
		t.Fatalf("expected ok=true on TamperRejected, reason=%q", reason)
	}
	if !strings.Contains(reason, "ro bind mount") {
		t.Fatalf("reason 应当解释 ro bind mount 生效，got %q", reason)
	}
}

func TestHelpersClassifyResolvConfDNS_AppliedTunneledNoLeak(t *testing.T) {
	ok, reason := ClassifyResolvConfDNSOutcome(TamperApplied, DNSResultTunneled, 0)
	if !ok {
		t.Fatalf("expected ok=true, reason=%q", reason)
	}
}

func TestHelpersClassifyResolvConfDNS_AppliedDeniedNoLeak(t *testing.T) {
	ok, reason := ClassifyResolvConfDNSOutcome(TamperApplied, DNSResultDenied, 0)
	if !ok {
		t.Fatalf("expected ok=true on Denied, reason=%q", reason)
	}
}

func TestHelpersClassifyResolvConfDNS_AppliedTunneledWithLeak(t *testing.T) {
	// 抓包 oracle 发现绕过：即使 dig 走 tun 也算 fail。
	ok, reason := ClassifyResolvConfDNSOutcome(TamperApplied, DNSResultTunneled, 2)
	if ok {
		t.Fatalf("expected ok=false on packet leak, reason=%q", reason)
	}
	if !strings.Contains(reason, "8.8.8.8") {
		t.Fatalf("reason 应当含 nameserver 字面，got %q", reason)
	}
}

func TestHelpersClassifyResolvConfDNS_AppliedLeaked(t *testing.T) {
	ok, reason := ClassifyResolvConfDNSOutcome(TamperApplied, DNSResultLeaked, 0)
	if ok {
		t.Fatalf("expected ok=false on DNSResultLeaked, reason=%q", reason)
	}
}

func TestHelpersClassifyResolvConfDNS_AppliedUnknown(t *testing.T) {
	ok, reason := ClassifyResolvConfDNSOutcome(TamperApplied, DNSResultUnknown, 0)
	if ok {
		t.Fatalf("expected ok=false on DNSResultUnknown, reason=%q", reason)
	}
}

func TestHelpersClassifyResolvConfDNS_TamperUnknownIsFail(t *testing.T) {
	ok, reason := ClassifyResolvConfDNSOutcome(TamperUnknown, DNSResultTunneled, 0)
	if ok {
		t.Fatalf("expected ok=false on TamperUnknown, reason=%q", reason)
	}
}

// TestHelpersCLIErrorCases_Wellformed 验证 CLIErrorCases 4 条用例的
// 退出码与 stderr 关键字结构正确，避免下游 table-driven 用例拿到坏数据。
func TestHelpersCLIErrorCases_Wellformed(t *testing.T) {
	wantNames := []string{"auth_invalid", "account_disabled", "account_expired", "host_not_found"}
	if len(CLIErrorCases) != len(wantNames) {
		t.Fatalf("expected %d CLIErrorCases, got %d", len(wantNames), len(CLIErrorCases))
	}
	for i, c := range CLIErrorCases {
		if c.Name != wantNames[i] {
			t.Errorf("case[%d].Name=%q want %q", i, c.Name, wantNames[i])
		}
		if c.WantExitCode != BootstrapExitCodeContract[c.Name] {
			t.Errorf("case[%d].WantExitCode=%d mismatches contract[%q]=%d",
				i, c.WantExitCode, c.Name, BootstrapExitCodeContract[c.Name])
		}
		if c.WantStderrContains == "" {
			t.Errorf("case[%d].WantStderrContains empty", i)
		}
		if c.Username == "" || c.Password == "" {
			t.Errorf("case[%d] missing credentials placeholders", i)
		}
	}
}
