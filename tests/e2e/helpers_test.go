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
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	bootstraperrors "github.com/zanel1u/cloud-cli-proxy/internal/controlplane/http"
)

// readLeakFixture 是 Phase 49 LEAK-* 纯函数单测的 fixture 加载入口。
// 走 runtime.Caller 反推项目根，禁绝对路径硬编码。
func readLeakFixture(t *testing.T, name string) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Dir(file)
	path := filepath.Join(root, "testdata", "leak", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(data)
}

// readKillswitchFixture 是 Phase 50 KILL-* 纯函数单测的 fixture 加载入口。
// 与 readLeakFixture 同模式，路径换到 testdata/killswitch/。
func readKillswitchFixture(t *testing.T, name string) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Dir(file)
	path := filepath.Join(root, "testdata", "killswitch", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(data)
}

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

// ─── Phase 49 / LEAK-* 纯函数单测（fixture-driven） ────────────────────

// ─── ClassifyLeakProbe 行为分支 ────────────────────────────────────────

func TestHelpersClassifyLeakProbe_NilInputInconclusive(t *testing.T) {
	if got := ClassifyLeakProbe(nil, true); got != LeakVerdictInconclusive {
		t.Fatalf("nil result expected Inconclusive, got %s", got)
	}
}

func TestHelpersClassifyLeakProbe_DockerExecErrorInconclusive(t *testing.T) {
	res := &LeakProbeResult{ExitCode: -1, Reason: ""}
	if got := ClassifyLeakProbe(res, true); got != LeakVerdictInconclusive {
		t.Fatalf("ExitCode=-1 + empty Reason expected Inconclusive, got %s", got)
	}
}

func TestHelpersClassifyLeakProbe_BlockedExpectedPass(t *testing.T) {
	res := &LeakProbeResult{Blocked: true, Reason: "dig_timeout", ExitCode: 9}
	if got := ClassifyLeakProbe(res, true); got != LeakVerdictPass {
		t.Fatalf("expected Pass, got %s", got)
	}
}

func TestHelpersClassifyLeakProbe_NotBlockedNotExpectedPass(t *testing.T) {
	res := &LeakProbeResult{Blocked: false, Reason: "ok", ExitCode: 0}
	if got := ClassifyLeakProbe(res, false); got != LeakVerdictPass {
		t.Fatalf("expected Pass when neither blocked nor expected, got %s", got)
	}
}

func TestHelpersClassifyLeakProbe_LeakIsFail(t *testing.T) {
	res := &LeakProbeResult{Blocked: false, Reason: "imds_responded_200", ExitCode: 0}
	if got := ClassifyLeakProbe(res, true); got != LeakVerdictFail {
		t.Fatalf("expected Fail when leaked, got %s", got)
	}
}

func TestHelpersLeakVerdict_String(t *testing.T) {
	cases := map[LeakVerdict]string{
		LeakVerdictUnknown:      "Unknown",
		LeakVerdictPass:         "Pass",
		LeakVerdictFail:         "Fail",
		LeakVerdictInconclusive: "Inconclusive",
	}
	for k, want := range cases {
		if got := k.String(); got != want {
			t.Fatalf("LeakVerdict(%d).String(): got %q want %q", k, got, want)
		}
	}
}

func TestHelpersLeakDangerousCaps_Locked(t *testing.T) {
	want := []string{CapNetRaw, CapNetAdmin, CapSysAdmin}
	if len(LeakDangerousCaps) != len(want) {
		t.Fatalf("LeakDangerousCaps len=%d want %d", len(LeakDangerousCaps), len(want))
	}
	for i, c := range want {
		if LeakDangerousCaps[i] != c {
			t.Fatalf("LeakDangerousCaps[%d]=%q want %q", i, LeakDangerousCaps[i], c)
		}
	}
}

// ─── ParseNftRules fixture 驱动 ────────────────────────────────────────

func TestHelpersParseNftRules_LinkLocalDropPresent(t *testing.T) {
	raw := readLeakFixture(t, "nft_ruleset_with_link_local_drop.txt")
	rules := ParseNftRules(raw)
	if !HasLinkLocalDropRule(rules) {
		t.Fatalf("expected at least one link-local drop rule, got rules=%+v", rules)
	}

	var sawIMDS, sawWideCIDR bool
	for _, r := range rules {
		if r.Action == "drop" && r.Dst == "169.254.169.254" {
			sawIMDS = true
		}
		if r.Action == "drop" && r.Dst == "169.254.0.0/16" {
			sawWideCIDR = true
		}
	}
	if !sawIMDS || !sawWideCIDR {
		t.Fatalf("expected both 169.254.169.254 and 169.254.0.0/16 drop rules; got rules=%+v", rules)
	}
}

func TestHelpersParseNftRules_LinkLocalAbsent(t *testing.T) {
	raw := readLeakFixture(t, "nft_ruleset_no_link_local.txt")
	rules := ParseNftRules(raw)
	if HasLinkLocalDropRule(rules) {
		t.Fatalf("expected NO link-local drop, but rules contain one: %+v", rules)
	}
	var sawMdns bool
	for _, r := range rules {
		if r.Action == "drop" && r.Proto == "udp" && r.Port == 5353 {
			sawMdns = true
		}
	}
	if !sawMdns {
		t.Fatalf("expected mdns 5353 drop, rules=%+v", rules)
	}
}

func TestHelpersParseNftRules_CountersFixture(t *testing.T) {
	raw := readLeakFixture(t, "nft_ruleset_with_counters.txt")
	rules := ParseNftRules(raw)
	if len(rules) < 4 {
		t.Fatalf("expected ≥4 rules, got %d (%+v)", len(rules), rules)
	}
	var sawDoT, sawPlainDNS bool
	for _, r := range rules {
		if r.Action == "drop" && r.Proto == "tcp" && r.Port == 853 {
			sawDoT = true
		}
		if r.Action == "drop" && r.Proto == "udp" && r.Port == 53 {
			sawPlainDNS = true
		}
	}
	if !sawDoT || !sawPlainDNS {
		t.Fatalf("expected DoT (tcp/853) and plain DNS (udp/53) drop rules, got %+v", rules)
	}
}

func TestHelpersParseNftRules_EmptyFixture(t *testing.T) {
	raw := readLeakFixture(t, "nft_ruleset_empty.txt")
	rules := ParseNftRules(raw)
	if len(rules) != 0 {
		t.Fatalf("expected 0 rules in empty fixture, got %d (%+v)", len(rules), rules)
	}
	if HasLinkLocalDropRule(nil) {
		t.Fatalf("HasLinkLocalDropRule(nil) must be false")
	}
}

func TestHelpersParseNftRules_EmptyRawString(t *testing.T) {
	rules := ParseNftRules("")
	if len(rules) != 0 {
		t.Fatalf("empty input must yield 0 rules, got %+v", rules)
	}
}

// ─── ParseNftCounters fixture 驱动 ─────────────────────────────────────

func TestHelpersParseNftCounters_WithCommentsKeyed(t *testing.T) {
	raw := readLeakFixture(t, "nft_ruleset_with_counters.txt")
	counters := ParseNftCounters(raw)
	if got, ok := counters["imds-drop"]; !ok || got != 7 {
		t.Fatalf("counter[imds-drop]=%d ok=%v want 7", got, ok)
	}
	if got, ok := counters["dot-drop"]; !ok || got != 4 {
		t.Fatalf("counter[dot-drop]=%d ok=%v want 4", got, ok)
	}
	if got, ok := counters["plain-dns-8888"]; !ok || got != 11 {
		t.Fatalf("counter[plain-dns-8888]=%d ok=%v want 11", got, ok)
	}
	if got, ok := counters["sbfw-tail"]; !ok || got != 99 {
		t.Fatalf("counter[sbfw-tail]=%d ok=%v want 99", got, ok)
	}
}

func TestHelpersParseNftCounters_EmptyFixture(t *testing.T) {
	raw := readLeakFixture(t, "nft_ruleset_empty.txt")
	counters := ParseNftCounters(raw)
	if len(counters) != 0 {
		t.Fatalf("empty fixture must yield 0 counters, got %+v", counters)
	}
}

func TestHelpersParseNftCounters_FallbackChainIndex(t *testing.T) {
	// 不带 comment，应回落到 <chain>:<index>。
	raw := `table ip filter {
	chain output {
		ip daddr 169.254.169.254 counter packets 3 bytes 192 drop
		ip daddr 169.254.0.0/16 counter packets 8 bytes 512 drop
	}
}`
	counters := ParseNftCounters(raw)
	if got, ok := counters["output:0"]; !ok || got != 3 {
		t.Fatalf("counter[output:0]=%d ok=%v want 3 (counters=%+v)", got, ok, counters)
	}
	if got, ok := counters["output:1"]; !ok || got != 8 {
		t.Fatalf("counter[output:1]=%d ok=%v want 8 (counters=%+v)", got, ok, counters)
	}
}

// ─── KnownCapabilityBits 锁定 ──────────────────────────────────────────

func TestHelpersKnownCapabilityBits_LocksCriticalSubset(t *testing.T) {
	want := map[int]string{
		12: CapNetAdmin,
		13: CapNetRaw,
		21: CapSysAdmin,
	}
	for bit, name := range want {
		got, ok := KnownCapabilityBits[bit]
		if !ok {
			t.Errorf("KnownCapabilityBits missing bit %d (%s)", bit, name)
			continue
		}
		if got != name {
			t.Errorf("KnownCapabilityBits[%d]=%q want %q", bit, got, name)
		}
	}
	if len(KnownCapabilityBits) < 10 {
		t.Errorf("KnownCapabilityBits should cover ≥10 caps, got %d", len(KnownCapabilityBits))
	}
}

// ─── ParseProcCapabilities fixture 驱动 ────────────────────────────────

func TestHelpersParseProcCapabilities_Clean(t *testing.T) {
	raw := readLeakFixture(t, "proc_status_clean.txt")
	caps, err := ParseProcCapabilities(raw)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	for _, dangerous := range LeakDangerousCaps {
		if caps.Eff[dangerous] {
			t.Errorf("clean fixture must NOT have %s in CapEff, got=%v", dangerous, caps.Eff)
		}
		if caps.Bnd[dangerous] {
			t.Errorf("clean fixture must NOT have %s in CapBnd, got=%v", dangerous, caps.Bnd)
		}
	}
	if !caps.Eff[CapChown] {
		t.Errorf("clean fixture should still have CHOWN in CapEff, got=%v", caps.Eff)
	}
}

func TestHelpersParseProcCapabilities_DirtyHasDangerous(t *testing.T) {
	raw := readLeakFixture(t, "proc_status_dirty.txt")
	caps, err := ParseProcCapabilities(raw)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	for _, dangerous := range []string{CapNetRaw, CapNetAdmin, CapSysAdmin} {
		if !caps.Eff[dangerous] {
			t.Errorf("dirty fixture must have %s in CapEff, got=%v", dangerous, caps.Eff)
		}
		if !caps.Bnd[dangerous] {
			t.Errorf("dirty fixture must have %s in CapBnd, got=%v", dangerous, caps.Bnd)
		}
	}
}

func TestHelpersParseProcCapabilities_PartialAllBoundOnly(t *testing.T) {
	// Inh/Prm/Eff 全 0；Bnd=0x3fffffffff（位 0..37 都置 1）。
	raw := readLeakFixture(t, "proc_status_partial.txt")
	caps, err := ParseProcCapabilities(raw)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	for _, dangerous := range LeakDangerousCaps {
		if caps.Eff[dangerous] {
			t.Errorf("partial fixture: %s should NOT be in Eff (eff is 0)", dangerous)
		}
		if !caps.Bnd[dangerous] {
			t.Errorf("partial fixture: %s should be in Bnd (bnd is all-ones)", dangerous)
		}
	}
}

func TestHelpersParseProcCapabilities_CorruptHexErr(t *testing.T) {
	raw := readLeakFixture(t, "proc_status_corrupt.txt")
	_, err := ParseProcCapabilities(raw)
	if err == nil {
		t.Fatalf("expected err on corrupt hex, got nil")
	}
}

func TestHelpersParseProcCapabilities_MissingLines(t *testing.T) {
	// 只给 2 行，缺 Eff / Bnd。
	raw := "CapInh:\t0000000000000000\nCapPrm:\t0000000000000000\n"
	_, err := ParseProcCapabilities(raw)
	if err == nil {
		t.Fatalf("expected err on missing CapEff/CapBnd, got nil")
	}
}

func TestHelpersParseNftRules_AcceptRulesNotMissed(t *testing.T) {
	raw := readLeakFixture(t, "nft_ruleset_with_link_local_drop.txt")
	rules := ParseNftRules(raw)
	var sawAccept bool
	for _, r := range rules {
		if r.Action == "accept" {
			sawAccept = true
			break
		}
	}
	if !sawAccept {
		t.Fatalf("expected at least one accept rule, got %+v", rules)
	}
}

func TestHelpersParseNftRules_TableChainContextPropagated(t *testing.T) {
	raw := readLeakFixture(t, "nft_ruleset_with_link_local_drop.txt")
	rules := ParseNftRules(raw)
	for _, r := range rules {
		if r.Table == "" {
			t.Fatalf("rule missing Table context: %+v", r)
		}
		if r.Chain == "" {
			t.Fatalf("rule missing Chain context: %+v", r)
		}
	}
}

func TestHelpersHasLinkLocalDropRule_RejectsAcceptOnly(t *testing.T) {
	rules := []NftRule{
		{Action: "accept", Dst: "169.254.0.0/16"},
		{Action: "drop", Dst: "1.1.1.1"},
	}
	if HasLinkLocalDropRule(rules) {
		t.Fatalf("accept-only link-local must NOT count as drop")
	}
}

func TestHelpersHasLinkLocalDropRule_AcceptsExactIMDS(t *testing.T) {
	rules := []NftRule{
		{Action: "drop", Dst: "169.254.169.254/32"},
	}
	if !HasLinkLocalDropRule(rules) {
		t.Fatalf("expected exact 169.254.169.254/32 drop to count")
	}
}

func TestHelpersParseProcCapabilities_TabSeparator(t *testing.T) {
	raw := "CapInh:\t0000000000000000\n" +
		"CapPrm:\t00000000a80405fb\n" +
		"CapEff:\t00000000a80405fb\n" +
		"CapBnd:\t00000000a80405fb\n" +
		"CapAmb:\t0000000000000000\n"
	caps, err := ParseProcCapabilities(raw)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !caps.Eff[CapChown] {
		t.Errorf("expected CHOWN in Eff with tab separator, got %v", caps.Eff)
	}
}

func TestHelpersParseProcCapabilities_AllZerosNoCaps(t *testing.T) {
	raw := "CapInh:\t0000000000000000\n" +
		"CapPrm:\t0000000000000000\n" +
		"CapEff:\t0000000000000000\n" +
		"CapBnd:\t0000000000000000\n"
	caps, err := ParseProcCapabilities(raw)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(caps.Eff) != 0 {
		t.Errorf("all-zero hex must yield empty Eff set, got %v", caps.Eff)
	}
	if len(caps.Bnd) != 0 {
		t.Errorf("all-zero hex must yield empty Bnd set, got %v", caps.Bnd)
	}
}

func TestHelpersParseProcCapabilities_NetAdminBitOnly(t *testing.T) {
	// 0x1000 = 1<<12 = NET_ADMIN
	raw := "CapInh:\t0000000000000000\n" +
		"CapPrm:\t0000000000001000\n" +
		"CapEff:\t0000000000001000\n" +
		"CapBnd:\t0000000000001000\n"
	caps, err := ParseProcCapabilities(raw)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !caps.Eff[CapNetAdmin] {
		t.Errorf("expected CAP_NET_ADMIN in Eff for hex 0x1000, got %v", caps.Eff)
	}
	if caps.Eff[CapNetRaw] || caps.Eff[CapSysAdmin] {
		t.Errorf("only NET_ADMIN should be set, got %v", caps.Eff)
	}
}

func TestHelpersParseProcCapabilities_SysAdminBitOnly(t *testing.T) {
	// 0x200000 = 1<<21 = SYS_ADMIN
	raw := "CapInh:\t0000000000000000\n" +
		"CapPrm:\t0000000000200000\n" +
		"CapEff:\t0000000000200000\n" +
		"CapBnd:\t0000000000200000\n"
	caps, err := ParseProcCapabilities(raw)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !caps.Eff[CapSysAdmin] {
		t.Errorf("expected CAP_SYS_ADMIN in Eff for hex 0x200000, got %v", caps.Eff)
	}
}

func TestHelpersExpandCapBits_NetRawBit(t *testing.T) {
	// bit 13 = CAP_NET_RAW
	caps, err := ParseProcCapabilities(
		"CapInh:\t0000000000000000\n" +
			"CapPrm:\t0000000000002000\n" +
			"CapEff:\t0000000000002000\n" +
			"CapBnd:\t0000000000002000\n")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !caps.Eff[CapNetRaw] {
		t.Errorf("expected CAP_NET_RAW in Eff for hex 0x2000, got %v", caps.Eff)
	}
	if caps.Eff[CapNetAdmin] || caps.Eff[CapSysAdmin] {
		t.Errorf("only NET_RAW should be set, got %v", caps.Eff)
	}
}

// ─── Phase 50 / KILL-01..04 压力测试纯函数单测 ─────────────────────────

func TestHelpersStressVerdict_String(t *testing.T) {
	cases := map[StressVerdict]string{
		StressVerdictUnknown:      "Unknown",
		StressVerdictPass:         "Pass",
		StressVerdictFail:         "Fail",
		StressVerdictInconclusive: "Inconclusive",
	}
	for k, want := range cases {
		if got := k.String(); got != want {
			t.Fatalf("StressVerdict(%d).String(): got %q want %q", k, got, want)
		}
	}
}

func TestHelpersKillswitchStressContract_Locked(t *testing.T) {
	want := map[string]struct {
		MaxDisconnectMs   int
		SSHAlive          bool
		AllowInconclusive bool
	}{
		"KILL-01": {3000, false, false},
		"KILL-02": {3000, false, false},
		"KILL-03": {0, true, true},
		"KILL-04": {3000, false, false},
	}
	if len(KillswitchStressContract) != len(want) {
		t.Fatalf("contract len=%d want %d", len(KillswitchStressContract), len(want))
	}
	for kill, w := range want {
		got, ok := KillswitchStressContract[kill]
		if !ok {
			t.Errorf("contract missing %s", kill)
			continue
		}
		if got.MaxDisconnectMs != w.MaxDisconnectMs {
			t.Errorf("%s MaxDisconnectMs=%d want %d", kill, got.MaxDisconnectMs, w.MaxDisconnectMs)
		}
		if got.SSHAlive != w.SSHAlive {
			t.Errorf("%s SSHAlive=%v want %v", kill, got.SSHAlive, w.SSHAlive)
		}
		if got.AllowInconclusive != w.AllowInconclusive {
			t.Errorf("%s AllowInconclusive=%v want %v", kill, got.AllowInconclusive, w.AllowInconclusive)
		}
	}
}

func TestHelpersClassifyStress_UnknownKill(t *testing.T) {
	v, reason := ClassifyStressResult("KILL-99", StressEvidence{})
	if v != StressVerdictUnknown {
		t.Fatalf("expected Unknown, got %s reason=%q", v, reason)
	}
	if !strings.Contains(reason, "unknown") {
		t.Fatalf("reason should mention unknown, got %q", reason)
	}
}

func TestHelpersClassifyStress_KILL01_PassOnDisconnect(t *testing.T) {
	v, reason := ClassifyStressResult("KILL-01", StressEvidence{
		ProbeExitCode: 28, LeakedPackets: 0, ElapsedMs: 1200,
	})
	if v != StressVerdictPass {
		t.Fatalf("expected Pass, got %s reason=%q", v, reason)
	}
}

func TestHelpersClassifyStress_KILL01_FailOnProbeSucceeded(t *testing.T) {
	v, reason := ClassifyStressResult("KILL-01", StressEvidence{
		ProbeExitCode: 0, LeakedPackets: 0, ElapsedMs: 100,
	})
	if v != StressVerdictFail {
		t.Fatalf("expected Fail on probe succeeded, got %s reason=%q", v, reason)
	}
	if !strings.Contains(reason, "probe unexpectedly") {
		t.Fatalf("reason mismatch: %q", reason)
	}
}

func TestHelpersClassifyStress_KILL01_FailOnLeakPackets(t *testing.T) {
	v, reason := ClassifyStressResult("KILL-01", StressEvidence{
		ProbeExitCode: 28, LeakedPackets: 5, ElapsedMs: 1500,
	})
	if v != StressVerdictFail {
		t.Fatalf("expected Fail on leaked packets, got %s reason=%q", v, reason)
	}
	if !strings.Contains(reason, "5") {
		t.Fatalf("reason should include packet count, got %q", reason)
	}
}

func TestHelpersClassifyStress_KILL01_FailOnLatency(t *testing.T) {
	v, reason := ClassifyStressResult("KILL-01", StressEvidence{
		ProbeExitCode: 28, LeakedPackets: 0, ElapsedMs: 5500,
	})
	if v != StressVerdictFail {
		t.Fatalf("expected Fail on latency, got %s reason=%q", v, reason)
	}
	if !strings.Contains(reason, "exceeds") {
		t.Fatalf("reason should mention threshold, got %q", reason)
	}
}

func TestHelpersClassifyStress_KILL02_Pass(t *testing.T) {
	v, _ := ClassifyStressResult("KILL-02", StressEvidence{
		ProbeExitCode: 124, LeakedPackets: 0, ElapsedMs: 2000,
	})
	if v != StressVerdictPass {
		t.Fatalf("expected Pass, got %s", v)
	}
}

func TestHelpersClassifyStress_KILL04_FailOnPacketLeak(t *testing.T) {
	v, reason := ClassifyStressResult("KILL-04", StressEvidence{
		ProbeExitCode: 28, LeakedPackets: 1, ElapsedMs: 500,
	})
	if v != StressVerdictFail {
		t.Fatalf("expected Fail on single leaked packet, got %s reason=%q", v, reason)
	}
}

func TestHelpersClassifyStress_KILL03_PassWithVote(t *testing.T) {
	v, _ := ClassifyStressResult("KILL-03", StressEvidence{
		SSHAlive:         true,
		EgressIPVote:     VoteResult{Winner: "203.0.113.5", OK: true},
		ExpectedEgressIP: "203.0.113.5",
	})
	if v != StressVerdictPass {
		t.Fatalf("expected Pass when vote matches expected, got %s", v)
	}
}

func TestHelpersClassifyStress_KILL03_FailOnSSHDead(t *testing.T) {
	v, reason := ClassifyStressResult("KILL-03", StressEvidence{
		SSHAlive: false,
	})
	if v != StressVerdictFail {
		t.Fatalf("expected Fail when SSH dead, got %s reason=%q", v, reason)
	}
	if !strings.Contains(reason, "ssh") {
		t.Fatalf("reason should mention ssh, got %q", reason)
	}
}

func TestHelpersClassifyStress_KILL03_FailOnWrongIP(t *testing.T) {
	v, reason := ClassifyStressResult("KILL-03", StressEvidence{
		SSHAlive:         true,
		EgressIPVote:     VoteResult{Winner: "1.2.3.4", OK: true},
		ExpectedEgressIP: "5.6.7.8",
	})
	if v != StressVerdictFail {
		t.Fatalf("expected Fail when vote winner != expected, got %s reason=%q", v, reason)
	}
	if !strings.Contains(reason, "1.2.3.4") || !strings.Contains(reason, "5.6.7.8") {
		t.Fatalf("reason should include both winner and expected, got %q", reason)
	}
}

func TestHelpersClassifyStress_KILL03_InconclusiveOnAbstain(t *testing.T) {
	v, reason := ClassifyStressResult("KILL-03", StressEvidence{
		SSHAlive:         true,
		EgressIPVote:     VoteResult{Winner: "", OK: false, Dissent: nil},
		ExpectedEgressIP: "5.6.7.8",
	})
	if v != StressVerdictInconclusive {
		t.Fatalf("expected Inconclusive on all-abstain, got %s reason=%q", v, reason)
	}
	if !strings.Contains(reason, "abstain") {
		t.Fatalf("reason should mention abstain, got %q", reason)
	}
}

// ─── Phase 50 / Pumba 参数构建 + 输出解析 ──────────────────────────────

func TestHelpersBuildPumbaNetemArgs_DelayDefaults(t *testing.T) {
	argv := BuildPumbaNetemArgs("cloudproxy-gw-alpha", PumbaNetemParams{})
	if len(argv) == 0 {
		t.Fatalf("expected non-empty argv")
	}
	if argv[0] != "docker" || argv[1] != "run" || argv[2] != "--rm" {
		t.Fatalf("argv must start with `docker run --rm`, got %v", argv)
	}
	joined := strings.Join(argv, " ")
	if !strings.Contains(joined, "gaiaadm/pumba:0.10.0") {
		t.Fatalf("expected default Pumba image, got %q", joined)
	}
	if !strings.Contains(joined, "--duration 30s") {
		t.Fatalf("expected default duration 30s, got %q", joined)
	}
	if !strings.Contains(joined, "delay --time 1000") {
		t.Fatalf("expected default delay 1000ms, got %q", joined)
	}
	if argv[len(argv)-1] != "cloudproxy-gw-alpha" {
		t.Fatalf("target must be last arg, got %q", argv[len(argv)-1])
	}
}

func TestHelpersBuildPumbaNetemArgs_CustomImage(t *testing.T) {
	argv := BuildPumbaNetemArgs("gw", PumbaNetemParams{
		Mode:     "delay",
		DelayMs:  500,
		Duration: 10 * time.Second,
		Image:    "private.registry/pumba:custom",
		TcImage:  "private.registry/iproute2:1",
	})
	joined := strings.Join(argv, " ")
	if !strings.Contains(joined, "private.registry/pumba:custom") {
		t.Fatalf("expected custom image, got %q", joined)
	}
	if !strings.Contains(joined, "--tc-image private.registry/iproute2:1") {
		t.Fatalf("expected custom tc-image, got %q", joined)
	}
	if !strings.Contains(joined, "--duration 10s") {
		t.Fatalf("expected duration 10s, got %q", joined)
	}
	if !strings.Contains(joined, "delay --time 500") {
		t.Fatalf("expected delay 500ms, got %q", joined)
	}
}

func TestHelpersBuildPumbaNetemArgs_EmptyTarget(t *testing.T) {
	argv := BuildPumbaNetemArgs("   ", PumbaNetemParams{})
	if argv != nil {
		t.Fatalf("expected nil argv on empty target, got %v", argv)
	}
}

func TestHelpersBuildPumbaNetemArgs_LossMode(t *testing.T) {
	argv := BuildPumbaNetemArgs("gw", PumbaNetemParams{Mode: "loss", LossPct: 30})
	joined := strings.Join(argv, " ")
	if !strings.Contains(joined, "loss --percent 30") {
		t.Fatalf("expected loss 30%%, got %q", joined)
	}
}

func TestHelpersBuildPumbaNetemArgs_UnknownMode(t *testing.T) {
	argv := BuildPumbaNetemArgs("gw", PumbaNetemParams{Mode: "garbage"})
	if argv != nil {
		t.Fatalf("expected nil argv on unknown mode, got %v", argv)
	}
}

func TestHelpersParsePumbaOutput_Applied(t *testing.T) {
	raw := readKillswitchFixture(t, "pumba_applied.txt")
	got := ParsePumbaOutput(raw, "")
	if got != PumbaOutcomeApplied {
		t.Fatalf("expected Applied, got %s", got)
	}
}

func TestHelpersParsePumbaOutput_ImageMissing(t *testing.T) {
	raw := readKillswitchFixture(t, "pumba_image_missing.txt")
	got := ParsePumbaOutput("", raw)
	if got != PumbaOutcomeImageMissing {
		t.Fatalf("expected ImageMissing, got %s", got)
	}
}

func TestHelpersParsePumbaOutput_DaemonDown(t *testing.T) {
	raw := readKillswitchFixture(t, "pumba_daemon_down.txt")
	got := ParsePumbaOutput("", raw)
	if got != PumbaOutcomeDaemonDown {
		t.Fatalf("expected DaemonDown, got %s", got)
	}
}

func TestHelpersParsePumbaOutput_Failed(t *testing.T) {
	raw := readKillswitchFixture(t, "pumba_failed.txt")
	got := ParsePumbaOutput(raw, "")
	if got != PumbaOutcomeFailed {
		t.Fatalf("expected Failed, got %s", got)
	}
}

func TestHelpersParsePumbaOutput_Unknown(t *testing.T) {
	got := ParsePumbaOutput("", "")
	if got != PumbaOutcomeUnknown {
		t.Fatalf("expected Unknown on empty, got %s", got)
	}
	got = ParsePumbaOutput("random garbage text\n", "")
	if got != PumbaOutcomeUnknown {
		t.Fatalf("expected Unknown on garbage, got %s", got)
	}
}

func TestHelpersPumbaOutcome_String(t *testing.T) {
	cases := map[PumbaOutcome]string{
		PumbaOutcomeUnknown:      "Unknown",
		PumbaOutcomeApplied:      "Applied",
		PumbaOutcomeFailed:       "Failed",
		PumbaOutcomeImageMissing: "ImageMissing",
		PumbaOutcomeDaemonDown:   "DaemonDown",
	}
	for k, want := range cases {
		if got := k.String(); got != want {
			t.Fatalf("PumbaOutcome(%d).String(): got %q want %q", k, got, want)
		}
	}
}

// ─── Phase 50 / KILL-04 网络选择纯函数 ─────────────────────────────────

func TestHelpersPickGatewayBridgeNetwork_CloudproxyPreferred(t *testing.T) {
	raw := "cloudproxy-net-alpha=10.99.0.2;bridge=172.17.0.5;"
	net, ip := PickGatewayBridgeNetwork(raw)
	if net != "cloudproxy-net-alpha" {
		t.Fatalf("expected cloudproxy-net-alpha, got %q", net)
	}
	if ip != "10.99.0.2" {
		t.Fatalf("expected ip 10.99.0.2, got %q", ip)
	}
}

func TestHelpersPickGatewayBridgeNetwork_OnlyBridgeReturnsEmpty(t *testing.T) {
	raw := "bridge=172.17.0.5;"
	net, ip := PickGatewayBridgeNetwork(raw)
	if net != "" || ip != "" {
		t.Fatalf("expected empty on bridge-only, got net=%q ip=%q", net, ip)
	}
}

func TestHelpersPickGatewayBridgeNetwork_NonCloudproxyCustomFallback(t *testing.T) {
	raw := "my-custom-net=10.0.0.5;bridge=172.17.0.2;"
	net, ip := PickGatewayBridgeNetwork(raw)
	if net != "my-custom-net" {
		t.Fatalf("expected fallback to my-custom-net, got %q", net)
	}
	if ip != "10.0.0.5" {
		t.Fatalf("expected ip 10.0.0.5, got %q", ip)
	}
}

func TestHelpersPickGatewayBridgeNetwork_EmptyInput(t *testing.T) {
	net, ip := PickGatewayBridgeNetwork("")
	if net != "" || ip != "" {
		t.Fatalf("expected empty on empty input, got net=%q ip=%q", net, ip)
	}
	net, ip = PickGatewayBridgeNetwork("   \n  ")
	if net != "" || ip != "" {
		t.Fatalf("expected empty on whitespace input, got net=%q ip=%q", net, ip)
	}
}

func TestHelpersPickGatewayBridgeNetwork_MultiCloudproxyTakesFirst(t *testing.T) {
	raw := "cloudproxy-net-alpha=10.99.0.2;cloudproxy-net-beta=10.99.1.2;bridge=172.17.0.5;"
	net, _ := PickGatewayBridgeNetwork(raw)
	if net != "cloudproxy-net-alpha" {
		t.Fatalf("expected first cloudproxy-net-*, got %q", net)
	}
}

