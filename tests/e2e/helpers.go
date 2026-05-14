// Package e2e 中的 helpers.go 提供 Phase 46 MVS 的纯函数 helpers。
//
// 设计原则：
//   - 本文件不带 build tag，darwin 上的 `go build ./tests/e2e/...` /
//     `go test ./tests/e2e/ -run Helpers` 必须能直接编译并跑通对应单测。
//   - 任何依赖 docker / linux netns / testcontainers 的接口都放在
//     helpers_linux.go（带 `//go:build e2e && linux`）。
//   - 三个 MVS 核心纯函数（Vote / ClassifyDNSResult / SummarizeDenyResults）
//     与一份 CLI 错误码契约表全部锁在此处，供 helpers_test.go 与 CI runner
//     共同消费。
//
// 引入历史：Phase 46 Plan 01（v3.6 milestone）。
package e2e

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ─── MVS-02 / 多数派裁决 ───────────────────────────────────────────────

// VoteResult 是 Vote 函数的返回值，便于 t.Logf 输出 + artifact dump。
type VoteResult struct {
	Winner  string
	OK      bool
	Dissent []string
}

// Vote 把多源回显结果合成一个多数派裁决（MVS-02）。
//
// 语义：
//   - results 中空字符串视为「弃权」（该源 timeout / dns fail / 非 200）。
//   - winner 为出现次数最多的非空字符串；ok=true 当且仅当 winner 计数 >= 2
//     （即「多数派达成」，CONTEXT §Area 2 决策）。
//   - dissent 为与 winner 不一致的非空回显（用于失败时 artifact dump）。
//   - 全部弃权 → winner="", ok=false, dissent=nil。
//
// 输入为 nil / 空切片 → 视为全部弃权。
func Vote(results []string) VoteResult {
	counts := map[string]int{}
	for _, r := range results {
		if r == "" {
			continue
		}
		counts[r]++
	}

	var winner string
	maxCount := 0
	for ip, c := range counts {
		if c > maxCount || (c == maxCount && ip < winner) {
			maxCount = c
			winner = ip
		}
	}

	ok := maxCount >= 2
	var dissent []string
	if ok {
		for _, r := range results {
			if r != "" && r != winner {
				dissent = append(dissent, r)
			}
		}
	} else {
		winner = ""
		for _, r := range results {
			if r != "" {
				dissent = append(dissent, r)
			}
		}
	}
	return VoteResult{Winner: winner, OK: ok, Dissent: dissent}
}

// egressIPSources 是 MVS-02 锁定的 3 个公网回显源。
// CONTEXT §Area 2 决策：固定 3 源避免每个用例自由发挥。
var egressIPSources = []string{
	"https://ip.me",
	"https://ifconfig.io",
	"https://ipinfo.io/ip",
}

// EgressIPSources 返回 MVS-02 锁定的 3 源副本（防止用例侧 mutate 影响其它 plan）。
func EgressIPSources() []string {
	cp := make([]string, len(egressIPSources))
	copy(cp, egressIPSources)
	return cp
}

// ─── MVS-03 / DNS 分类 ─────────────────────────────────────────────────

// DNSProbeResult 是 ClassifyDNSResult 的枚举返回值（MVS-03）。
//
// Tunneled：exit 0，认为 tun 接管并正常返回 A 记录或 HTTPS 握手成功。
// Denied：被防火墙拒绝（refused / timeout / unreachable / not permitted）。
// Leaked：明确证据走宿主机直连绕过 tun（暂留扩展点；本 phase 仅 Tunneled / Denied）。
// Unknown：分类不明，用例应 fail。
type DNSProbeResult int

const (
	DNSResultUnknown DNSProbeResult = iota
	DNSResultTunneled
	DNSResultDenied
	DNSResultLeaked
)

// String 让 DNSProbeResult 在 t.Log 输出可读。
func (r DNSProbeResult) String() string {
	switch r {
	case DNSResultTunneled:
		return "Tunneled"
	case DNSResultDenied:
		return "Denied"
	case DNSResultLeaked:
		return "Leaked"
	default:
		return "Unknown"
	}
}

// dnsDenyKeywords 是 stderr 出现即视为 Denied 的关键字集合（小写匹配）。
var dnsDenyKeywords = []string{
	"connection refused",
	"timed out",
	"timeout",
	"network is unreachable",
	"operation not permitted",
	"permanent failure in name resolution",
	"name or service not known",
	"no route to host",
}

// ClassifyDNSResult 把容器内 DNS probe（getent / dig / nslookup）的 exit code +
// stderr 文本映射到 DNSProbeResult 枚举（MVS-03）。
//
// 语义：
//   - exit 0 → Tunneled。
//   - exit != 0 且 stderr 含 dnsDenyKeywords 任一 → Denied。
//   - 其它 → Unknown，用例应 fail（避免悄悄假阳）。
//
// Leaked 不由本函数赋值；由调用方拿到「目标域名解析结果落到宿主机直连 IP」的
// 硬证据时另行 override（Phase 49 防泄漏对抗时会接管此分支）。
func ClassifyDNSResult(exitCode int, stderr string) DNSProbeResult {
	if exitCode == 0 {
		return DNSResultTunneled
	}
	lower := strings.ToLower(stderr)
	for _, kw := range dnsDenyKeywords {
		if strings.Contains(lower, kw) {
			return DNSResultDenied
		}
	}
	return DNSResultUnknown
}

// ─── MVS-04 / 默认拒绝矩阵 ─────────────────────────────────────────────

// DenyTarget 是默认拒绝矩阵中的 host + port 组合（MVS-04）。
type DenyTarget struct {
	Host string
	Port int
}

// String 让 DenyTarget 在 t.Log / artifact dump 中可读。
func (t DenyTarget) String() string { return fmt.Sprintf("%s:%d", t.Host, t.Port) }

// DefaultDenyMatrix 是 CONTEXT §Area 2 锁定的 4 个默认拒绝 target。
// 修改本切片需同步更新 PLAN / VERIFICATION，并通知 Phase 48 / 49 复用方。
var DefaultDenyMatrix = []DenyTarget{
	{Host: "1.1.1.1", Port: 80},
	{Host: "8.8.8.8", Port: 443},
	{Host: "9.9.9.9", Port: 443},
	{Host: "169.254.169.254", Port: 80},
}

// BuildDenyProbeCmd 返回容器内执行的「直连探测」shell 命令。
//
// 命令形如：timeout <N> bash -c 'echo >/dev/tcp/HOST/PORT'
// 用 timeout(1) 把 bash /dev/tcp 探测裹一层硬超时，避免 nft drop 触发的
// TCP 长尾 retransmit 拖垮整个用例。
//
// timeoutSec <= 0 时回退到默认 3s。
func BuildDenyProbeCmd(t DenyTarget, timeoutSec int) []string {
	if timeoutSec <= 0 {
		timeoutSec = 3
	}
	return []string{
		"timeout", strconv.Itoa(timeoutSec),
		"bash", "-c",
		fmt.Sprintf("echo >/dev/tcp/%s/%d", t.Host, t.Port),
	}
}

// SummarizeDenyResults 把矩阵每个 target 的 exit code 合成裁决（MVS-04）。
//
//   - exit 0 → 连通 → leak（违反"默认拒绝"约束）。
//   - exit != 0 → Denied（refused / unreachable / timeout 都计入）。
//   - allDenied=true 当且仅当所有 target 都被拒绝。
//   - leaks 列表按 DefaultDenyMatrix 顺序遍历输入 map（保证稳定输出）。
func SummarizeDenyResults(results map[DenyTarget]int) (allDenied bool, leaks []DenyTarget) {
	allDenied = true
	for _, t := range DefaultDenyMatrix {
		code, ok := results[t]
		if !ok {
			continue
		}
		if code == 0 {
			allDenied = false
			leaks = append(leaks, t)
		}
	}
	return allDenied, leaks
}

// ─── MVS-05 / CLI 错误码契约 ──────────────────────────────────────────

// CLIErrorCase 是 bootstrap 脚本错误码场景的 table-driven 测试条目。
type CLIErrorCase struct {
	Name               string
	Username           string
	Password           string
	WantExitCode       int
	WantStderrContains string
}

// BootstrapExitCodeContract 是 MVS-05 锁定的 4 个错误码契约表。
//
// 数值与 internal/controlplane/http/bootstrap_errors.go BootstrapErrorEntries
// 中的 ExitCode 字段一致；helpers_test.go 中通过 import 该包做交叉断言，
// 任一漂移立即编译期 / 测试期失败。
//
// 与 ROADMAP §Phase 46 §Details 5 描述的差异：ROADMAP 写「真实 cloud-claude
// binary」，但 grep `cmd/cloud-claude/main.go` 实际定义的常量是
// exitAuthFailed=1 / exitNetworkError=2 等，不含 10-13；错误码 10-13 由
// `deploy/bootstrap/cloud-bootstrap.sh` 在 `case "$error_code"` 分支映射，
// 走的是 curl bootstrap 入口而非 cloud-claude binary。本表以源码为准。
var BootstrapExitCodeContract = map[string]int{
	"auth_invalid":     10,
	"account_disabled": 11,
	"account_expired":  12,
	"host_not_found":   13,
}

// ─── MVS-06 / 到期容器自动停止 + 审计事件 ────────────────────────────

// ExpiryEventType 是 Phase 47 Plan 01 锁定的「过期触发主机停止」审计事件类型。
//
// 源码 internal/controlplane/scheduler/expiry.go::expireUser 中写入：
//
//	store.RecordEvent(ctx, repository.RecordEventParams{
//	    Type: "host.stop.expired",
//	    Metadata: map[string]any{"reason": "user expired", ...},
//	})
//
// 与 ROADMAP §Phase 47 §Details 1 与 47-CONTEXT.md §Area 1 草案的差异：
// 文档草案曾写「host.stopped」事件；以源码为准，本表锁定为 host.stop.expired。
// 任一漂移立即在 darwin 单测层失败，并需同步更新 PLAN/SUMMARY。
const ExpiryEventType = "host.stop.expired"

// UserExpiredEventType 是 ExpiryScanner 在标记 user.status='expired' 之后
// 写入的用户级别审计事件类型。用例可用作「scanner 已触发」的快速断言。
const UserExpiredEventType = "user.expired"

// expiryEventListResponse 对应 GET /v1/admin/events 的响应 body 顶层结构。
// 仅取 type 字段；其它字段（id / created_at / metadata）通过 RawMessage 透传。
type expiryEventListResponse struct {
	Events []struct {
		Type string `json:"type"`
	} `json:"events"`
}

// ParseEventTypes 抽取 admin events API 响应 body 中的事件 type 列表（保留顺序）。
//
// 行为：
//   - 输入为 GET /v1/admin/events 的 JSON body 切片。
//   - 解析失败 → 返回 nil + err。
//   - 缺 events 字段 / 空数组 → 返回 []string{} + nil。
//   - 解析成功 → 返回每行 type 的有序切片（不去重，保留 backend 排序）。
//
// 不依赖 metadata 内容；上层用例自行根据 type 子串匹配 + metadata 二次过滤。
func ParseEventTypes(body []byte) ([]string, error) {
	if len(body) == 0 {
		return []string{}, fmt.Errorf("parse event types: empty body")
	}
	var parsed expiryEventListResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse event types: %w", err)
	}
	out := make([]string, 0, len(parsed.Events))
	for _, ev := range parsed.Events {
		out = append(out, ev.Type)
	}
	return out, nil
}

// ─── MVS-07 / 出口 IP 双绑互斥 ────────────────────────────────────────

// BindEgressIPResponse 是 POST /v1/admin/bindings 的解析后契约视图。
//
// ErrorMessage：当前源码 admin_bindings.go::Bind() 用 `{"error":"自由文本"}`
// 而非稳定 error code 枚举；本结构保留 raw message 子串，待 backend 引入
// 稳定 code（如 egress_ip_already_bound）后再扩展枚举字段（Phase 51 QUAL-04）。
type BindEgressIPResponse struct {
	Status       int
	ErrorMessage string
	RawBody      []byte
}

// bindErrorBody 对应 {"error":"..."}。
type bindErrorBody struct {
	Error string `json:"error"`
}

// ParseBindEgressIPResponse 把 admin bindings POST 的 status code + body 合成契约视图。
//
// 行为：
//   - body 非空且解析出 error 字段 → ErrorMessage 取该值。
//   - body 空 / 非 JSON / 缺 error 字段 → ErrorMessage="", err=nil（合法 2xx 路径）。
//   - 不消耗 body；RawBody 字段透传原 bytes 供上层 t.Logf。
func ParseBindEgressIPResponse(status int, body []byte) (BindEgressIPResponse, error) {
	out := BindEgressIPResponse{Status: status, RawBody: body}
	if len(body) == 0 {
		return out, nil
	}
	var parsed bindErrorBody
	if err := json.Unmarshal(body, &parsed); err == nil {
		out.ErrorMessage = parsed.Error
	}
	return out, nil
}

// EgressIPDoubleBindContract 锁定 MVS-07「期望」的拒绝行为。
//
// 当前源码 admin_bindings.go 在底层 BindEgressIPToHost 错误时返回 500，
// 因为 `host_egress_bindings` 仅 UNIQUE(host_id, egress_ip_id)，没有
// 「同一 egress_ip_id 只允许绑给一个 host」的硬约束。本契约定义「应有」
// 的行为；用例失败时把 backend 缺口落到 SUMMARY，建议 Phase 51 修源码。
var EgressIPDoubleBindContract = struct {
	WantStatus       int
	WantErrSubstring string
}{
	WantStatus:       409,
	WantErrSubstring: "already bound",
}

// ─── MVS-08 / host-agent 心跳与恢复 ───────────────────────────────────

// HostHealthStatus 是控制面 /healthz checks.agent 字段的枚举映射。
//
// 当前控制面没有 GET /v1/admin/hosts/{X}/health 端点（grep router.go 与
// admin_hosts.go），单宿主机 v1 部署用全局 /healthz 的 checks.agent 即可
// 表达 host-agent 进程级健康状态。多宿主机场景属未来 phase。
type HostHealthStatus int

const (
	HostHealthUnknown HostHealthStatus = iota
	HostHealthHealthy
	HostHealthUnhealthy
	HostHealthDegraded
)

// String 让 HostHealthStatus 在 t.Log / artifact dump 中可读。
func (s HostHealthStatus) String() string {
	switch s {
	case HostHealthHealthy:
		return "Healthy"
	case HostHealthUnhealthy:
		return "Unhealthy"
	case HostHealthDegraded:
		return "Degraded"
	default:
		return "Unknown"
	}
}

// controlPlaneHealthBody 对应 /healthz 响应顶层结构。
type controlPlaneHealthBody struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}

// ParseControlPlaneHealth 解析 GET /healthz 响应 body，返回 (overall, agent) 二元组。
//
// 映射（参见 internal/controlplane/http/router.go::/healthz handler）：
//
//	overall:
//	  "ok"       → HostHealthHealthy
//	  "warning"  → HostHealthUnhealthy（含 agent unreachable）
//	  "degraded" → HostHealthDegraded（含 database 故障）
//	  其它/缺失   → HostHealthUnknown
//
//	agent (取自 checks.agent 字段)：
//	  "ok"           → HostHealthHealthy
//	  "unreachable"  → HostHealthUnhealthy
//	  缺失（embedded 模式不暴露 agent 字段）→ HostHealthUnknown
//	  其它字面量      → HostHealthUnknown
//
// 解析失败 → 返回 (Unknown, Unknown, err)。
func ParseControlPlaneHealth(body []byte) (HostHealthStatus, HostHealthStatus, error) {
	if len(body) == 0 {
		return HostHealthUnknown, HostHealthUnknown, fmt.Errorf("parse health: empty body")
	}
	var parsed controlPlaneHealthBody
	if err := json.Unmarshal(body, &parsed); err != nil {
		return HostHealthUnknown, HostHealthUnknown, fmt.Errorf("parse health: %w", err)
	}
	overall := HostHealthUnknown
	switch parsed.Status {
	case "ok":
		overall = HostHealthHealthy
	case "warning":
		overall = HostHealthUnhealthy
	case "degraded":
		overall = HostHealthDegraded
	}
	agent := HostHealthUnknown
	if parsed.Checks != nil {
		switch parsed.Checks["agent"] {
		case "ok":
			agent = HostHealthHealthy
		case "unreachable":
			agent = HostHealthUnhealthy
		case "":
			agent = HostHealthUnknown
		}
	}
	return overall, agent, nil
}

// HostHealthRecoveryContract 锁定 MVS-08 时间窗。
//
// 任一阈值漂移 → 同步修 47-03-PLAN / 47-03-SUMMARY，避免静默放宽 SLA。
var HostHealthRecoveryContract = struct {
	UnhealthyWithin time.Duration
	HealthyWithin   time.Duration
}{
	UnhealthyWithin: 30 * time.Second,
	HealthyWithin:   60 * time.Second,
}

// ─── MVS-09 / Kill-switch：sing-box 崩溃断网 ──────────────────────────

// KillswitchVerdict 是 ClassifyKillswitchResult 的返回枚举（MVS-09 / Phase 48 Plan 01）。
//
// 语义对照表：
//
//	probeExitCode != 0 && leakedPackets == 0 → KillswitchOK
//	probeExitCode == 0 && leakedPackets == 0 → KillswitchProbeUnexpectedlySucceeded
//	probeExitCode != 0 && leakedPackets > 0  → KillswitchPacketLeak
//	probeExitCode == 0 && leakedPackets > 0  → KillswitchBoth
type KillswitchVerdict int

const (
	KillswitchUnknown KillswitchVerdict = iota
	KillswitchOK
	KillswitchProbeUnexpectedlySucceeded
	KillswitchPacketLeak
	KillswitchBoth
)

// String 让 KillswitchVerdict 在 t.Log / artifact dump 中可读。
func (v KillswitchVerdict) String() string {
	switch v {
	case KillswitchOK:
		return "OK"
	case KillswitchProbeUnexpectedlySucceeded:
		return "ProbeUnexpectedlySucceeded"
	case KillswitchPacketLeak:
		return "PacketLeak"
	case KillswitchBoth:
		return "Both"
	default:
		return "Unknown"
	}
}

// ClassifyKillswitchResult 把容器内 curl 退出码 + host eth0 tcpdump 计数合成
// kill-switch 裁决（MVS-09）。
//
// 入参：
//   - probeExitCode：容器内 `curl --max-time N <url>` 退出码。0 = 仍能出网。
//   - leakedPackets：host eth0 在 BPF `src host <worker> and not dst <gateway>`
//     下捕获的「绕过 gateway」包数。
//
// 任一非零包 + curl 0 都视为违例；只有两者同时干净才 KillswitchOK。
func ClassifyKillswitchResult(probeExitCode int, leakedPackets int) KillswitchVerdict {
	switch {
	case probeExitCode != 0 && leakedPackets == 0:
		return KillswitchOK
	case probeExitCode == 0 && leakedPackets == 0:
		return KillswitchProbeUnexpectedlySucceeded
	case probeExitCode != 0 && leakedPackets > 0:
		return KillswitchPacketLeak
	default:
		return KillswitchBoth
	}
}

// KillswitchTimingContract 锁定 MVS-09 与 Phase 50 KILL-01 共享的 timing 不变量。
//
//   - ProbeMaxLatency：kill gateway 后允许容器内 curl 探测的最大等待时间。
//   - TcpdumpWindow：host eth0 抓包 oracle 的捕获窗口（让 curl 在这个窗口内发完包）。
//
// 任一漂移 → darwin 单测立即 fail，Phase 50 必须同步修。
var KillswitchTimingContract = struct {
	ProbeMaxLatency time.Duration
	TcpdumpWindow   time.Duration
}{
	ProbeMaxLatency: 3 * time.Second,
	TcpdumpWindow:   5 * time.Second,
}

// tcpdumpCountRe 抓 tcpdump stderr 中 `N packets captured` 字面量。
// tcpdump 退出时输出形如：
//
//	5 packets captured
//	5 packets received by filter
//	0 packets dropped by kernel
//
// 大小写不敏感（部分 BSD tcpdump 用大写）。
var tcpdumpCountRe = regexp.MustCompile(`(?i)(\d+)\s+packets?\s+captured`)

// ErrTcpdumpCountNotFound 表示 stderr 中找不到 `N packets captured` 字样。
// 调用方应据此向上层冒泡（不能默默当成 0）。
var ErrTcpdumpCountNotFound = errors.New("tcpdump count: pattern not found in stderr")

// ParseTcpdumpCountOutput 从 tcpdump 退出后写入 stderr 的统计行中抽出捕获包数。
//
// 行为：
//   - 命中 tcpdumpCountRe 第一组 → 返回 (N, nil)。
//   - 空 stderr / 缺关键字 → 返回 (0, ErrTcpdumpCountNotFound)。
//   - 数字解析失败（极少见，但兜底） → 返回 (0, err)。
//
// 不做 trim/lower 之外的归一化，避免吞掉 tcpdump 真实错误。
func ParseTcpdumpCountOutput(stderr string) (int, error) {
	if strings.TrimSpace(stderr) == "" {
		return 0, ErrTcpdumpCountNotFound
	}
	m := tcpdumpCountRe.FindStringSubmatch(stderr)
	if len(m) < 2 {
		return 0, ErrTcpdumpCountNotFound
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, fmt.Errorf("tcpdump count: parse %q: %w", m[1], err)
	}
	return n, nil
}

// ─── MVS-10 / Kill-switch：resolv.conf 篡改免疫 ───────────────────────

// ResolvConfTamperResult 是 GoldenPath.TamperResolvConf 的返回枚举（MVS-10）。
//
//   - TamperApplied：worker 容器内 `echo > /etc/resolv.conf` 成功覆写，
//     `grep` 能命中新 nameserver。这是「绕过 ro bind mount」成功的合法分支。
//   - TamperRejected：写入失败（EROFS / EBUSY / permission denied），
//     系统侧抗住了，这也是合法分支（CONTEXT §Area 2）。
//   - TamperUnknown：exec 本身报错，调用方需 t.Fatalf。
type ResolvConfTamperResult int

const (
	TamperUnknown ResolvConfTamperResult = iota
	TamperApplied
	TamperRejected
)

// String 让 ResolvConfTamperResult 在 t.Log 输出可读。
func (r ResolvConfTamperResult) String() string {
	switch r {
	case TamperApplied:
		return "Applied"
	case TamperRejected:
		return "Rejected"
	default:
		return "Unknown"
	}
}

// ResolvConfTamperContract 锁定 MVS-10 篡改用的目标 nameserver。
//
// 与 host eth0 BPF filter（`udp port 53 and dst 8.8.8.8`）必须保持一致，
// 任一漂移会导致抓包 oracle 抓不到泄漏包。
var ResolvConfTamperContract = struct {
	Nameserver string
}{
	Nameserver: "8.8.8.8",
}

// ClassifyResolvConfDNSOutcome 把 (tamper 结果, DNS probe 分类, 抓包计数)
// 合成 MVS-10 最终裁决。
//
// 行为表（来源：48-02-PLAN.md §Steps Step 3）：
//
//	Rejected | *                  | *     | true
//	Applied  | Tunneled / Denied  | 0     | true
//	Applied  | Tunneled / Denied  | >0    | false（抓包 oracle 发现绕过）
//	Applied  | Leaked             | *     | false
//	Applied  | Unknown            | *     | false
//
// 返回 (ok, reason)。reason 用于 t.Logf / artifact dump，便于排障。
func ClassifyResolvConfDNSOutcome(tamper ResolvConfTamperResult, dnsResult DNSProbeResult, leakedPackets int) (bool, string) {
	if tamper == TamperRejected {
		return true, "ro bind mount 抗住了 resolv.conf 写入（系统侧防御生效）"
	}
	if tamper != TamperApplied {
		return false, fmt.Sprintf("tamper result=%s 不在合法分支（Applied/Rejected）内", tamper)
	}
	if leakedPackets > 0 {
		return false, fmt.Sprintf("host eth0 抓到 %d 个 UDP/53 → %s 包，存在直连泄漏",
			leakedPackets, ResolvConfTamperContract.Nameserver)
	}
	switch dnsResult {
	case DNSResultTunneled:
		return true, "tun 接管 DNS，用户态改写不生效"
	case DNSResultDenied:
		return true, "防火墙明确拒绝直连 DNS"
	case DNSResultLeaked:
		return false, "DNS 被分类为 Leaked，明确证据走宿主机直连"
	default:
		return false, fmt.Sprintf("DNS 分类不明（%s），需 t.Logf stderr 排障", dnsResult)
	}
}

// ─── Phase 49 / LEAK-01..08 共享类型与纯函数 ────────────────────────────

// LeakProbeResult 是各 LEAK-* 用例容器内探测命令（dig / ping / curl / python -c
// SOCK_RAW 等）的统一结果结构（CONTEXT §Specifics 锁定）。
//
// 字段语义：
//   - Blocked：是否被防御层阻断（true = 防泄漏成功，可能是 timeout / refused /
//     drop / permission_denied）。
//   - Reason：阻断原因的简短归类，便于 t.Logf / artifact dump 检索（如
//     "dig_timeout", "raw_socket_denied", "imds_http_error"）。
//   - RawStdout / RawStderr：原始输出，留给失败排障；本结构本身不解释。
//   - ExitCode：子进程退出码；docker exec 失败 / 句柄不存在 → -1。
//   - Duration：从发起 docker exec 到拿到结果的端到端耗时。
type LeakProbeResult struct {
	Blocked   bool
	Reason    string
	RawStdout string
	RawStderr string
	ExitCode  int
	Duration  time.Duration
}

// LeakVerdict 是 ClassifyLeakProbe 的三值返回。
//
//   - LeakVerdictPass：实际行为与预期一致（防泄漏生效或预期未阻断）。
//   - LeakVerdictFail：实际行为与预期相反（最坏情况：预期阻断但实际通了）。
//   - LeakVerdictInconclusive：探测本身报错或结果模糊，调用方应 t.Skip 或
//     补充证据后重判。
type LeakVerdict int

const (
	LeakVerdictUnknown LeakVerdict = iota
	LeakVerdictPass
	LeakVerdictFail
	LeakVerdictInconclusive
)

// String 让 LeakVerdict 在 t.Log / artifact dump 中可读。
func (v LeakVerdict) String() string {
	switch v {
	case LeakVerdictPass:
		return "Pass"
	case LeakVerdictFail:
		return "Fail"
	case LeakVerdictInconclusive:
		return "Inconclusive"
	default:
		return "Unknown"
	}
}

// ClassifyLeakProbe 把 LeakProbeResult 与「预期是否被阻断」合成裁决。
//
// 行为表：
//
//	result == nil                                    → Inconclusive
//	result.ExitCode == -1 && result.Reason == ""     → Inconclusive（docker exec 报错）
//	result.Blocked == expectBlocked                  → Pass
//	result.Blocked != expectBlocked                  → Fail
//
// CONTEXT §Area 4：失败时 Reason 字面量与 RawStderr 末尾必须打到 t.Logf，便于
// LeakDumpHook 自动归档时携带上下文。
func ClassifyLeakProbe(result *LeakProbeResult, expectBlocked bool) LeakVerdict {
	if result == nil {
		return LeakVerdictInconclusive
	}
	if result.ExitCode == -1 && result.Reason == "" {
		return LeakVerdictInconclusive
	}
	if result.Blocked == expectBlocked {
		return LeakVerdictPass
	}
	return LeakVerdictFail
}

// ─── Phase 49 LEAK-07 / nft 规则与 counter 解析 ─────────────────────────

// NftRule 是 ParseNftRules 抽出的规则规范化视图（CONTEXT §Specifics）。
//
// 不强求覆盖 nft 全语法；只关心 LEAK-* 用例需要的字段：destination CIDR /
// 协议 / 端口 / verdict（drop / accept / reject）。
type NftRule struct {
	Table   string
	Chain   string
	Action  string // drop / accept / reject / counter / log
	Dst     string // ip daddr 字面量；如 169.254.0.0/16 / 1.1.1.1/32 / "" = 任意
	Proto   string // tcp / udp / icmp / ""
	Port    int    // dport 数值；0 = 任意
	Comment string // nft `comment "..."` 抽取
	RawLine string // 原始行，便于 fixture 调试
}

// nftTableRe 匹配 `table inet|ip|ip6 <name> {`。
var nftTableRe = regexp.MustCompile(`^\s*table\s+(\w+)\s+(\w+)\s+\{`)

// nftChainRe 匹配 `chain <name> {`。
var nftChainRe = regexp.MustCompile(`^\s*chain\s+(\w+)\s+\{`)

// nftIPDaddrRe 匹配 `ip daddr <字面量>`（CIDR 或单 IP）。
var nftIPDaddrRe = regexp.MustCompile(`ip\s+daddr\s+([0-9a-fA-F:.]+(?:/\d+)?)`)

// nftDportRe 匹配 `(tcp|udp) dport <num>`。
var nftDportRe = regexp.MustCompile(`(tcp|udp)\s+dport\s+(\d+)`)

// nftCommentRe 匹配 `comment "<text>"`。
var nftCommentRe = regexp.MustCompile(`comment\s+"([^"]*)"`)

// nftCounterRe 匹配 `counter packets <N> bytes <M>`。
var nftCounterRe = regexp.MustCompile(`counter\s+packets\s+(\d+)\s+bytes\s+(\d+)`)

// ParseNftRules 把 `nft list ruleset` 文本展平为 []NftRule。
//
// 解析策略：
//   - 维护 (table, chain) 上下文，遇 `}` 出栈。
//   - 每行尝试抽 daddr / dport / verdict / comment；命中 verdict 之一才算
//     一条 NftRule（避免把 `type filter hook output ...` 之类元行也算进来）。
//   - verdict 优先级：drop > reject > accept；行内 `counter` 视为附属，不单独
//     算一条 verdict。
//   - LEAK-07 用例只关心 drop 规则的 Dst/Proto/Port，因此覆盖率以这三项为准；
//     accept 规则的解析仅用于完整性，不强保证字段完整。
func ParseNftRules(raw string) []NftRule {
	var out []NftRule
	var curTable, curChain string
	depth := 0

	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if m := nftTableRe.FindStringSubmatch(line); len(m) == 3 {
			curTable = m[1] + " " + m[2]
			depth++
			continue
		}
		if m := nftChainRe.FindStringSubmatch(line); len(m) == 2 {
			curChain = m[1]
			depth++
			continue
		}
		if trimmed == "}" {
			depth--
			if depth == 1 {
				curChain = ""
			} else if depth == 0 {
				curTable = ""
			}
			continue
		}

		var action string
		switch {
		case strings.Contains(trimmed, " drop") || strings.HasSuffix(trimmed, "drop"):
			action = "drop"
		case strings.Contains(trimmed, " reject"):
			action = "reject"
		case strings.HasSuffix(trimmed, " accept") || strings.HasSuffix(trimmed, "accept"):
			action = "accept"
		default:
			continue
		}

		rule := NftRule{
			Table:   curTable,
			Chain:   curChain,
			Action:  action,
			RawLine: trimmed,
		}
		if m := nftIPDaddrRe.FindStringSubmatch(trimmed); len(m) == 2 {
			rule.Dst = m[1]
		}
		if m := nftDportRe.FindStringSubmatch(trimmed); len(m) == 3 {
			rule.Proto = m[1]
			if p, perr := strconv.Atoi(m[2]); perr == nil {
				rule.Port = p
			}
		}
		if m := nftCommentRe.FindStringSubmatch(trimmed); len(m) == 2 {
			rule.Comment = m[1]
		}
		out = append(out, rule)
	}
	return out
}

// ParseNftCounters 提取规则集中所有 `counter packets N bytes M` 计数。
//
// key 优先用 `comment "<x>"`；缺失 comment 时退回 `<chain>:<index>` 复合键
// （index 是 chain 内 counter 出现序号，从 0 起）。
//
// 返回 map[key]packets。bytes 信息丢弃（LEAK 用例只关心是否命中，不做带宽
// 测算）。空输入 / 无 counter → 返回空 map（非 nil），便于调用方安全
// `for k, v := range`。
func ParseNftCounters(raw string) map[string]uint64 {
	out := map[string]uint64{}
	var curChain string
	chainIdx := map[string]int{}

	for _, line := range strings.Split(raw, "\n") {
		if m := nftChainRe.FindStringSubmatch(line); len(m) == 2 {
			curChain = m[1]
			continue
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		m := nftCounterRe.FindStringSubmatch(trimmed)
		if len(m) < 3 {
			continue
		}
		packets, err := strconv.ParseUint(m[1], 10, 64)
		if err != nil {
			continue
		}
		var key string
		if cm := nftCommentRe.FindStringSubmatch(trimmed); len(cm) == 2 {
			key = cm[1]
		} else {
			idx := chainIdx[curChain]
			key = fmt.Sprintf("%s:%d", curChain, idx)
			chainIdx[curChain] = idx + 1
		}
		out[key] = packets
	}
	return out
}

// HasLinkLocalDropRule 扫一组 NftRule，判断是否至少存在一条 destination 为
// `169.254.x.x` 前缀的 drop 规则（LEAK-07 主断言）。
//
// 行为：精确字面量前缀匹配 `169.254`，避免被 `169.2540.x` 之类反例误命中。
// CIDR 子集（`169.254.169.254/32`）与超集（`169.254.0.0/16`）都视为命中。
func HasLinkLocalDropRule(rules []NftRule) bool {
	for _, r := range rules {
		if r.Action != "drop" {
			continue
		}
		if strings.HasPrefix(r.Dst, "169.254") {
			return true
		}
	}
	return false
}

// ─── Phase 49 LEAK-08 / capability 解析 ────────────────────────────────

// Capability 是 worker 容器进程 1 的 capability 字符串常量（CONTEXT §Specifics）。
//
// 锁定 ≥10 个常用 cap 名字 + 位序号，覆盖 LEAK-08 关心的 NET_RAW / NET_ADMIN
// / SYS_ADMIN，以及若干常见 cap（便于 SUMMARY 调试时打全名而非位掩码）。
//
// 位序号严格遵照 Linux <uapi/linux/capability.h>，任一漂移立即在 darwin 单测
// 失败（防止 backend 切到非标准 capability 名字）。
const (
	CapNetRaw          = "CAP_NET_RAW"
	CapNetAdmin        = "CAP_NET_ADMIN"
	CapSysAdmin        = "CAP_SYS_ADMIN"
	CapSysPtrace       = "CAP_SYS_PTRACE"
	CapDacReadSearch   = "CAP_DAC_READ_SEARCH"
	CapNetBindService  = "CAP_NET_BIND_SERVICE"
	CapChown           = "CAP_CHOWN"
	CapSetuid          = "CAP_SETUID"
	CapSetgid          = "CAP_SETGID"
	CapKill            = "CAP_KILL"
	CapSysChroot       = "CAP_SYS_CHROOT"
	CapMknod           = "CAP_MKNOD"
	CapAuditWrite      = "CAP_AUDIT_WRITE"
	CapSetfcap         = "CAP_SETFCAP"
)

// KnownCapabilityBits 锁定 bit index → cap name 映射。Phase 49 只锁 LEAK-08
// 关心的 ≥10 个 cap，不追求覆盖完整 Linux cap 集（v2 范围）。
var KnownCapabilityBits = map[int]string{
	0:  CapChown,
	2:  CapDacReadSearch,
	5:  CapKill,
	6:  CapSetgid,
	7:  CapSetuid,
	10: CapNetBindService,
	12: CapNetAdmin,
	13: CapNetRaw,
	18: CapSysChroot,
	19: CapSysPtrace,
	21: CapSysAdmin,
	27: CapMknod,
	29: CapAuditWrite,
	31: CapSetfcap,
}

// ProcCapabilities 是 ParseProcCapabilities 的返回结构。
//
// 4 个字段对应 /proc/<pid>/status 中的 4 行 capability bitmask；map[capName]bool
// 形式便于用例直接 `if caps.Eff[CapNetRaw] { ... }` 断言。
type ProcCapabilities struct {
	Inh map[string]bool
	Prm map[string]bool
	Eff map[string]bool
	Bnd map[string]bool
}

// procCapLineRe 匹配 `Cap{Inh,Prm,Eff,Bnd}:\s+<16 hex chars>`。
var procCapLineRe = regexp.MustCompile(`(?m)^Cap(Inh|Prm|Eff|Bnd):\s+([0-9a-fA-F]+)\s*$`)

// ParseProcCapabilities 解析 `/proc/<pid>/status` 中 4 行 capability 掩码。
//
// 行为：
//   - 4 行（CapInh / CapPrm / CapEff / CapBnd）必须全部命中；缺任一 → 返回 err。
//   - hex 长度 ≤ 16 字符（最大 0xffff_ffff_ffff_ffff）；超长或非 hex → err。
//   - 结果 map 仅包含 KnownCapabilityBits 表中已知的 cap；未知位被忽略
//     （v3 范围只锁 LEAK-08 关心 cap）。
//
// 不依赖 setpcaps / getpcaps 可执行；纯字符串解析，darwin 上可直接跑单测。
func ParseProcCapabilities(raw string) (ProcCapabilities, error) {
	caps := ProcCapabilities{
		Inh: map[string]bool{},
		Prm: map[string]bool{},
		Eff: map[string]bool{},
		Bnd: map[string]bool{},
	}
	matches := procCapLineRe.FindAllStringSubmatch(raw, -1)
	if len(matches) < 4 {
		return caps, fmt.Errorf("parse proc caps: expected 4 Cap lines, got %d", len(matches))
	}
	seen := map[string]bool{}
	for _, m := range matches {
		field, hexVal := m[1], m[2]
		if seen[field] {
			continue
		}
		if len(hexVal) > 16 {
			return caps, fmt.Errorf("parse proc caps: %s hex too long: %q", field, hexVal)
		}
		bits, err := strconv.ParseUint(hexVal, 16, 64)
		if err != nil {
			return caps, fmt.Errorf("parse proc caps: %s: %w", field, err)
		}
		expanded := expandCapBits(bits)
		switch field {
		case "Inh":
			caps.Inh = expanded
		case "Prm":
			caps.Prm = expanded
		case "Eff":
			caps.Eff = expanded
		case "Bnd":
			caps.Bnd = expanded
		}
		seen[field] = true
	}
	for _, want := range []string{"Inh", "Prm", "Eff", "Bnd"} {
		if !seen[want] {
			return caps, fmt.Errorf("parse proc caps: missing Cap%s line", want)
		}
	}
	return caps, nil
}

// expandCapBits 把 64 位掩码按 KnownCapabilityBits 展开成已知 cap 名集合。
func expandCapBits(bits uint64) map[string]bool {
	out := map[string]bool{}
	for bit, name := range KnownCapabilityBits {
		if bits&(1<<uint(bit)) != 0 {
			out[name] = true
		}
	}
	return out
}

// LeakDangerousCaps 是 LEAK-08 严格禁止出现在 worker 进程 CapEff/CapBnd 的
// capability 集合。任一命中 → 用例 fail。
var LeakDangerousCaps = []string{CapNetRaw, CapNetAdmin, CapSysAdmin}

// ─── 既有锁定表（保留） ────────────────────────────────────────────────

// CLIErrorCases 是 MVS-05 的 4 个 table-driven 用例预设（用例代码可直接 range）。
// stderr 关键字与 bootstrap_errors.go 中 Message 字段保持一致的子串。
var CLIErrorCases = []CLIErrorCase{
	{
		Name:               "auth_invalid",
		Username:           "alice",
		Password:           "wrong-password",
		WantExitCode:       BootstrapExitCodeContract["auth_invalid"],
		WantStderrContains: "用户名或密码错误",
	},
	{
		Name:               "account_disabled",
		Username:           "disabled-user",
		Password:           "secret-placeholder-pw",
		WantExitCode:       BootstrapExitCodeContract["account_disabled"],
		WantStderrContains: "账号已被停用",
	},
	{
		Name:               "account_expired",
		Username:           "expired-user",
		Password:           "secret-placeholder-pw",
		WantExitCode:       BootstrapExitCodeContract["account_expired"],
		WantStderrContains: "账号已过期",
	},
	{
		Name:               "host_not_found",
		Username:           "user-no-host",
		Password:           "secret-placeholder-pw",
		WantExitCode:       BootstrapExitCodeContract["host_not_found"],
		WantStderrContains: "未找到可用主机",
	},
}
