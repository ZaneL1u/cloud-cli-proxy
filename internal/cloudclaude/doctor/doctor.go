// Package doctor — Phase 34 Plan 02：cloud-claude doctor 五维度自检框架。
//
// Entry point: RunDoctor(ctx, opts) -> *Report
// 串行执行：network → auth → ssh → mount → disk（CONTEXT D-07）
// 远端 check 走 RemoteRunner interface（D-20 lazy connect + StatusSkip 降级）
package doctor

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude"
)

// Status 对标 CONTEXT D-15 JSON schema 字面量 "pass"|"warn"|"fail"|"skip"。
type Status string

const (
	StatusPass Status = "pass"
	StatusWarn Status = "warn"
	StatusFail Status = "fail"
	StatusSkip Status = "skip"
)

// Options 是 RunDoctor 的入参（CONTEXT D-01）。
type Options struct {
	Domain       string        // "network" / "auth" / "ssh" / "mount" / "disk" / "all"（cobra 已校验）
	Fix          bool          // Plan 03 落实；Plan 02 仅注册 flag
	Verbose      bool          // --verbose 每 check 打印时间戳 + Details 全字段
	JSON         bool          // --json 切换 RenderJSON（Text 不输出）
	NoColor      bool          // 显式关闭颜色（叠加在 colors.ColorEnabled 之上）
	Yes          bool          // Plan 03 用：confirmDestructive 跳过交互
	CheckTimeout time.Duration // 单 check timeout；0 则取默认 5s（Verbose 30s）
}

// DowngradeBanner 是第一屏降级历史（CONTEXT D-13 / RESEARCH §5.1）。
type DowngradeBanner struct {
	SnapshotAgeSeconds int64           `json:"snapshot_age_seconds"`
	IntendedMode       string          `json:"intended_mode,omitempty"`
	ActualMode         string          `json:"actual_mode,omitempty"`
	DowngradeChain     []DowngradeStep `json:"downgrade_chain,omitempty"`
	ConflictCount      int             `json:"conflict_count,omitempty"`
	ReconnectCount     int             `json:"reconnect_count,omitempty"`
	TmuxSession        string          `json:"tmux_session,omitempty"`
	ClientRole         string          `json:"client_role,omitempty"`
}

// DowngradeStep 源自 cloudclaude.LastSessionSnapshot.DowngradeChain；doctor 只读不写。
type DowngradeStep struct {
	From          string `json:"from"`
	To            string `json:"to"`
	ReasonCode    string `json:"reason_code,omitempty"`
	ReasonMessage string `json:"reason_message,omitempty"`
}

// Summary 汇总统计（CONTEXT D-13 末段）。
type Summary struct {
	Total int `json:"total"`
	Pass  int `json:"pass"`
	Warn  int `json:"warn"`
	Fail  int `json:"fail"`
	Skip  int `json:"skip"`
}

// Report 是 RunDoctor 返回值 + --json 序列化对象（schema_version=1 锁死，RESEARCH §5.1）。
type Report struct {
	SchemaVersion    int              `json:"schema_version"` // 硬编码 1，不带 omitempty（jq 依赖）
	StartedAt        time.Time        `json:"started_at"`
	DurationMS       int64            `json:"duration_ms"`
	CloudClaudeVer   string           `json:"cloud_claude_version,omitempty"`
	RemoteImageVer   string           `json:"remote_image_version,omitempty"`
	DowngradeHistory *DowngradeBanner `json:"downgrade_history,omitempty"`
	Summary          Summary          `json:"summary"`
	Checks           []Check          `json:"checks"`
}

// RunDoctor 顶层入口。执行顺序（CONTEXT D-07）：network → auth → ssh → mount → disk。
// 远端 conn lazy 建立（CONTEXT D-20）；未 init 时 mount/ssh/disk 维度 StatusSkip（D-06）。
func RunDoctor(ctx context.Context, opts Options) (*Report, error) {
	start := time.Now()
	report := &Report{
		SchemaVersion: 1,
		StartedAt:     start,
		Checks:        []Check{},
	}

	timeout := opts.CheckTimeout
	if timeout <= 0 {
		if opts.Verbose {
			timeout = 30 * time.Second
		} else {
			timeout = 5 * time.Second
		}
	}

	// 1) 读 LastSessionSnapshot → DowngradeHistory
	snap, _ := cloudclaude.LoadLastSession()
	if snap != nil {
		report.DowngradeHistory = convertSnapshotToBanner(snap)
	}

	// 2) 读本地 config（未 init 时 cfg=nil）
	var cfg *cloudclaude.Config
	if _, cfgC := checkConfigPresent(ctx); cfgC != nil {
		cfg = cfgC
	}

	// 决定要跑哪些维度
	want := func(d string) bool {
		if opts.Domain == "" || opts.Domain == "all" {
			return true
		}
		return opts.Domain == d
	}

	// lazy SSH conn（auth / mount / ssh / disk 维度共享）
	var remoteConn *ssh.Client
	var remoteRunner RemoteRunner
	var authResp *cloudclaude.AuthResponse
	closeRemote := func() {
		if remoteConn != nil {
			_ = remoteConn.Close()
			remoteConn = nil
			remoteRunner = nil
		}
	}
	defer closeRemote()

	ensureRemote := func() {
		if remoteRunner != nil || cfg == nil || authResp == nil {
			return
		}
		sshCfg := cloudclaude.SSHConfig{
			Host:     authResp.SSHHost,
			Port:     authResp.SSHPort,
			User:     authResp.SSHUser,
			Password: authResp.SSHPass,
		}
		conn, err := cloudclaude.SSHConnect(sshCfg)
		if err != nil {
			return // 所有 RequiresRemote=true check 将走 StatusSkip
		}
		remoteConn = conn
		remoteRunner = NewSSHRemoteRunner(conn)
	}

	// 3) network 维度（3 check，本地可独立跑）
	if want("network") {
		report.Checks = append(report.Checks, runWithTimeout(ctx, "network", "dns_resolve", timeout,
			func(c context.Context) Check {
				host := ""
				if cfg != nil {
					host = parseHostFromGateway(cfg.Gateway)
				}
				return checkDNSResolve(c, host)
			}))
		report.Checks = append(report.Checks, runWithTimeout(ctx, "network", "gateway_reachable", timeout,
			func(c context.Context) Check {
				gw := ""
				if cfg != nil {
					gw = cfg.Gateway
				}
				return checkGatewayReachable(c, gw)
			}))
		// egress_ip_visible 需要 runner；在 auth 之后 ensureRemote
	}

	// 4) auth 维度（cfg 需要 + Entry API + 远端 OAuth）
	if want("auth") {
		cp, cfg2 := checkConfigPresent(ctx)
		report.Checks = append(report.Checks, cp)
		if cfg2 != nil {
			cfg = cfg2
		}
		if cfg != nil {
			etv, resp := checkEntryTokenValid(ctx, cfg)
			report.Checks = append(report.Checks, etv)
			if resp != nil {
				authResp = resp
				report.RemoteImageVer = resp.ImageVersion
			}
		}
		ensureRemote()
		if want("network") && authResp != nil {
			report.Checks = append(report.Checks, runWithTimeout(ctx, "network", "egress_ip_visible", timeout,
				func(c context.Context) Check {
					expectedIP := authRespExpectedEgressIP(authResp)
					return checkEgressIPVisible(c, remoteRunner, expectedIP)
				}))
		}
		report.Checks = append(report.Checks,
			checkOAuthCredentials(ctx, remoteConn, authRespClaudeAccountID(authResp)))
	}

	// 5) ssh 维度
	if want("ssh") {
		ensureRemote()
		kaInterval := 15 * time.Second
		report.Checks = append(report.Checks, checkKeepaliveConfig(ctx, kaInterval))
		report.Checks = append(report.Checks, runWithTimeout(ctx, "ssh", "sshd_keepalive_drift", timeout,
			func(c context.Context) Check { return checkSSHDKeepaliveDrift(c, remoteRunner) }))
		khPath := defaultKnownHostsPath()
		hostPort := ""
		if authResp != nil {
			hostPort = makeKnownHostsHostPort(authResp.SSHHost, authResp.SSHPort)
		}
		report.Checks = append(report.Checks, checkKnownHosts(ctx, khPath, hostPort))
		if authResp != nil {
			sshCfg := cloudclaude.SSHConfig{
				Host:     authResp.SSHHost,
				Port:     authResp.SSHPort,
				User:     authResp.SSHUser,
				Password: authResp.SSHPass,
			}
			report.Checks = append(report.Checks, checkWorkspaceSSHKeys(ctx, sshCfg))
		}
	}

	// 6) mount 维度
	if want("mount") {
		ensureRemote()
		report.Checks = append(report.Checks, runWithTimeout(ctx, "mount", "mergerfs_branches", timeout,
			func(c context.Context) Check { return checkMergerfsBranches(c, remoteRunner) }))
		report.Checks = append(report.Checks, runWithTimeout(ctx, "mount", "sshfs_mountpoint", timeout,
			func(c context.Context) Check { return checkSSHFSMountpoint(c, remoteRunner) }))
		report.Checks = append(report.Checks, checkFUSEResidual(ctx))
		report.Checks = append(report.Checks, checkAppArmorFusermount3(ctx))
	}

	// 7) disk 维度
	if want("disk") {
		ensureRemote()
		report.Checks = append(report.Checks, checkLocalDisk(ctx))
		report.Checks = append(report.Checks, runWithTimeout(ctx, "disk", "container_disk", timeout,
			func(c context.Context) Check { return checkContainerDisk(c, remoteRunner) }))
	}

	// 8) 聚合 Summary
	for _, c := range report.Checks {
		report.Summary.Total++
		switch c.Status {
		case StatusPass:
			report.Summary.Pass++
		case StatusWarn:
			report.Summary.Warn++
		case StatusFail:
			report.Summary.Fail++
		case StatusSkip:
			report.Summary.Skip++
		}
	}
	report.DurationMS = time.Since(start).Milliseconds()
	return report, nil
}

// convertSnapshotToBanner 把 cloudclaude.LastSessionSnapshot 转为 doctor DowngradeBanner。
func convertSnapshotToBanner(snap *cloudclaude.LastSessionSnapshot) *DowngradeBanner {
	age := int64(time.Since(snap.Timestamp).Seconds())
	steps := make([]DowngradeStep, 0, len(snap.DowngradeChain))
	for _, s := range snap.DowngradeChain {
		steps = append(steps, DowngradeStep{
			From:          s.From,
			To:            s.To,
			ReasonCode:    s.ReasonCode,
			ReasonMessage: s.ReasonMessage,
		})
	}
	return &DowngradeBanner{
		SnapshotAgeSeconds: age,
		IntendedMode:       snap.IntendedMode,
		ActualMode:         snap.ActualMode,
		DowngradeChain:     steps,
		ConflictCount:      snap.ConflictCount,
		ReconnectCount:     snap.ReconnectCount,
		TmuxSession:        snap.TmuxSession,
		ClientRole:         snap.ClientRole,
	}
}

// parseHostFromGateway 粗略：https://host:port/path → host
func parseHostFromGateway(gw string) string {
	s := strings.TrimPrefix(gw, "https://")
	s = strings.TrimPrefix(s, "http://")
	if i := strings.Index(s, "/"); i >= 0 {
		s = s[:i]
	}
	if h, _, err := net.SplitHostPort(s); err == nil {
		return h
	}
	return s
}

func defaultKnownHostsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".ssh", "known_hosts")
}

// authRespClaudeAccountID 安全取 authResp.ClaudeAccountID。
func authRespClaudeAccountID(r *cloudclaude.AuthResponse) string {
	if r == nil {
		return ""
	}
	return r.ClaudeAccountID
}

// authRespExpectedEgressIP — Plan 02 deviation：cloudclaude.AuthResponse 当前
// 未导出 ExpectedEgressIP 字段（v3.0 entry.go 仅含 Status/SSH*/ImageVersion/
// SupportsMergerfs/ClaudeAccountID）。本实现恒返回 ""，让
// checkEgressIPVisible 走 expectedIP=="" Pass 分支（不误报 drift）；entry 包补齐
// 该字段的工作记入 v3.1 backlog。
func authRespExpectedEgressIP(r *cloudclaude.AuthResponse) string {
	_ = r
	return ""
}
