package cloudclaude

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/errcodes"
)

// Mode 是 cloud-claude --mount-mode flag 的四档枚举（CONTEXT D-14）。
//
// 状态机契约：
//   - 入参允许 Auto / Full / MutagenOnly / SSHFSOnly
//   - 返回时永远是 Full / MutagenOnly / SSHFSOnly / Failed 之一（Auto 仅作为入参意图）
type Mode int

const (
	ModeAuto Mode = iota
	ModeFull
	ModeMutagenOnly
	ModeSSHFSOnly
	ModeFailed
)

// String 返回 cobra flag 字面值。
func (m Mode) String() string {
	switch m {
	case ModeAuto:
		return "auto"
	case ModeFull:
		return "full"
	case ModeMutagenOnly:
		return "mutagen-only"
	case ModeSSHFSOnly:
		return "sshfs-only"
	case ModeFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// ParseMode 把 cobra flag 字面值解析为 Mode。
// 非法值返回错误；调用方应在 main.go 入口直接 reject 并退出非 0。
func ParseMode(s string) (Mode, error) {
	switch s {
	case "auto", "":
		return ModeAuto, nil
	case "full":
		return ModeFull, nil
	case "mutagen-only":
		return ModeMutagenOnly, nil
	case "sshfs-only":
		return ModeSSHFSOnly, nil
	default:
		return ModeFailed, fmt.Errorf("非法 mount-mode 值: %q（应为 auto|full|mutagen-only|sshfs-only）", s)
	}
}

// MountConfig 是 MountWorkspace 的入参集合。
//
// 字段来源：
//   - Mode / NoColor / KeepAlive*：cobra flag / 环境变量
//   - ClaudeAccountID / ImageVersion / SupportsMutagen / SupportsMergerfs：Phase 30 AuthResponse
//   - Cwd / LastSessionPath / Logger：runtime 注入
//   - SyncSessionLock：Phase 32 多端冲突保护（本阶段默认 noop）
type MountConfig struct {
	Mode              Mode
	KeepAliveInterval time.Duration
	KeepAliveCountMax int
	ClaudeAccountID   string
	ImageVersion      string
	SupportsMutagen   bool
	SupportsMergerfs  bool
	Cwd               string
	NoColor           bool
	Logger            io.Writer
	LastSessionPath   string
	SyncSessionLock   func(accountID string) (release func(), err error)

	// [Phase 32 D-29] cobra --new-session / --take-over flag 透传 + os.Hostname() 注入。
	// 字段不参与 JSON 序列化（MountConfig 是运行时配置容器，无 JSON tag）。
	SessionShortID  string // --new-session 时 cmd 层生成的 8 字符 base64url；空 = 默认 session 命名（per-account_id）
	SessionTakeOver bool   // --take-over flag
	LocalHostname   string // os.Hostname()，session.go 文件注册表用

	// [Phase 32 Plan 03 新增] 由 ssh.go 注入的 SyncSessionLock 闭包在拿不到 flock
	// 时（ErrSyncLocked）置 true；session.go::runClaudeWithSession 据此把
	// last-session.json 的 ClientRole 写为 "secondary"（默认 "primary"）。
	IsSecondaryClient bool

	// 测试 hook：仅用于单测注入；生产路径 nil 时走真实实现。
	overrideCaseInsensitive *bool
	hooks                   *strategyHooks
}

// strategyHooks 让 mount_strategy_test.go 注入三层 mount 的 mock 实现，
// 不依赖真实 ssh / mutagen / mergerfs。
//
// Plan 03 把 tryMutagen 第二返回值从 int 升级为 MutagenSyncStatus，
// 让 mount_strategy 同时拿到 ConflictCount 与 LastError。
type strategyHooks struct {
	tryMutagen func() (cleanup func(), status MutagenSyncStatus, err error)
	trySSHFS   func() (cleanup func(), err error)
	tryMerge   func() (cleanup func(), err error)
}

// MountWorkspace 是 Phase 31 文件映射顶层入口。
//
// 调度逻辑（CONTEXT D-15）：
//  1. APFS 检测 → 写入 snapshot.APFSCaseInsensitive
//  2. 能力降级（cfg.SupportsMutagen=false → 强制 SSHFSOnly；SupportsMergerfs=false 且 Mode=Auto/Full → 降级 MutagenOnly）
//  3. 按 Mode 决定 try 顺序：
//     - Auto: [Full, MutagenOnly, SSHFSOnly]，每档失败 stderr 输出 MOUNT_AUTO_DOWNGRADED 后转下一档
//     - Force (Full/MutagenOnly/SSHFSOnly)：单档跑，失败 → MOUNT_FORCE_MODE_FAILED + ModeFailed
//  4. 三段式中文进度按最终决策的 mode 渲染
//  5. mount 全 ready 输出 banner [<mode>]（着色：full=green / 其它=yellow）
//  6. 写 last-session.json（成功 / 失败均写）
//
// cleanup LIFO 顺序：mergerfs → mutagen/sshfs → connections。
// 任何 error 已经被 errcodes.Format 包装为可直接 stderr 的字符串。
func MountWorkspace(connA, connB *ssh.Client, cfg MountConfig) (cleanup func(), finalMode Mode, err error) {
	if cfg.Logger == nil {
		cfg.Logger = os.Stderr
	}

	snapshot := LastSessionSnapshot{
		SchemaVersion:   1,
		Timestamp:       time.Now().UTC(),
		IntendedMode:    cfg.Mode.String(),
		DowngradeChain:  []DowngradeStep{},
		ClaudeAccountID: cfg.ClaudeAccountID,
		ImageVersion:    cfg.ImageVersion,
	}

	// 1) APFS 检测（macOS 默认 case-insensitive）
	isCI := false
	if cfg.overrideCaseInsensitive != nil {
		isCI = *cfg.overrideCaseInsensitive
	} else if runtime.GOOS == "darwin" && cfg.Cwd != "" {
		isCI = IsCaseInsensitiveFS(cfg.Cwd)
	}
	snapshot.APFSCaseInsensitive = isCI
	if isCI {
		fmt.Fprintln(cfg.Logger, errcodes.Format(errcodes.MOUNT_APFS_CASE_INSENSITIVE))
	}

	// 2) 能力降级（CONTEXT D-29）
	intended := cfg.Mode
	if !cfg.SupportsMutagen && (intended == ModeAuto || intended == ModeFull || intended == ModeMutagenOnly) {
		applyDowngrade(cfg.Logger, &snapshot, intended, ModeSSHFSOnly,
			errcodes.MOUNT_MUTAGEN_VERSION_SKEW, "remote 不支持 mutagen")
		intended = ModeSSHFSOnly
	}
	if !cfg.SupportsMergerfs && (intended == ModeAuto || intended == ModeFull) {
		applyDowngrade(cfg.Logger, &snapshot, intended, ModeMutagenOnly,
			errcodes.MOUNT_MERGERFS_FAILED, "remote 不支持 mergerfs")
		intended = ModeMutagenOnly
	}

	// 3) 决定 try 顺序
	var tryOrder []Mode
	switch intended {
	case ModeAuto:
		tryOrder = []Mode{ModeFull, ModeMutagenOnly, ModeSSHFSOnly}
	case ModeFull:
		tryOrder = []Mode{ModeFull}
	case ModeMutagenOnly:
		tryOrder = []Mode{ModeMutagenOnly}
	case ModeSSHFSOnly:
		tryOrder = []Mode{ModeSSHFSOnly}
	default:
		tryOrder = []Mode{ModeSSHFSOnly}
	}

	var lastErr error
	for i, mode := range tryOrder {
		// 三段式进度：先决策再打印（CONTEXT D-18 强约束 — 不出现「打了又改主意」）
		printProgress(cfg.Logger, mode)

		modeCleanup, mutagenStatus, mErr := tryMode(connA, connB, mode, cfg)
		if mErr == nil {
			snapshot.ActualMode = mode.String()
			snapshot.ConflictCount = mutagenStatus.ConflictCount

			printBanner(cfg.Logger, mode, cfg.NoColor)
			if mutagenStatus.ConflictCount > 0 {
				fmt.Fprintf(cfg.Logger, "⚠ 有 %d 个文件同步冲突，运行 cloud-claude sync conflicts 查看\n", mutagenStatus.ConflictCount)
			}

			writeLastSessionWarn(cfg.LastSessionPath, snapshot, cfg.Logger)
			return modeCleanup, mode, nil
		}

		lastErr = mErr
		code, reason := extractErrCodeAndReason(mErr)

		// Force mode 不允许降级
		if cfg.Mode != ModeAuto {
			snapshot.ActualMode = ModeFailed.String()
			writeLastSessionWarn(cfg.LastSessionPath, snapshot, cfg.Logger)
			wrap := errcodes.Format(errcodes.MOUNT_FORCE_MODE_FAILED, cfg.Mode.String(), mode.String(), reason)
			return func() {}, ModeFailed, fmt.Errorf("%s: %w", wrap, mErr)
		}

		// Auto 模式：打降级 banner + 转下一档
		if i+1 < len(tryOrder) {
			next := tryOrder[i+1]
			applyDowngrade(cfg.Logger, &snapshot, mode, next, code, reason)
		}
	}

	// 全部档位失败
	snapshot.ActualMode = ModeFailed.String()
	writeLastSessionWarn(cfg.LastSessionPath, snapshot, cfg.Logger)
	if lastErr == nil {
		lastErr = fmt.Errorf("文件映射全部档位失败")
	}
	return func() {}, ModeFailed, lastErr
}

// tryMode 按 mode 调度子层：
//   - Full = mutagen + sshfs + merge（任一失败即失败）
//   - MutagenOnly = mutagen 单层
//   - SSHFSOnly = sshfs 单层（v2.0 路径）
//
// 测试通过 cfg.hooks 注入 mock；生产走 tryModeReal。
func tryMode(connA, connB *ssh.Client, mode Mode, cfg MountConfig) (cleanup func(), status MutagenSyncStatus, err error) {
	if cfg.hooks != nil {
		return tryModeWithHooks(mode, cfg.hooks)
	}
	return tryModeReal(connA, connB, mode, cfg)
}

func tryModeWithHooks(mode Mode, h *strategyHooks) (cleanup func(), status MutagenSyncStatus, err error) {
	var cleanups []func()
	finalCleanup := func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
	}

	switch mode {
	case ModeFull:
		if h.tryMutagen != nil {
			cl, st, e := h.tryMutagen()
			if e != nil {
				finalCleanup()
				return nil, MutagenSyncStatus{}, e
			}
			status = st
			if cl != nil {
				cleanups = append(cleanups, cl)
			}
		}
		if h.trySSHFS != nil {
			cl, e := h.trySSHFS()
			if e != nil {
				finalCleanup()
				return nil, MutagenSyncStatus{}, e
			}
			if cl != nil {
				cleanups = append(cleanups, cl)
			}
		}
		if h.tryMerge != nil {
			cl, e := h.tryMerge()
			if e != nil {
				finalCleanup()
				return nil, MutagenSyncStatus{}, e
			}
			if cl != nil {
				cleanups = append(cleanups, cl)
			}
		}
		return finalCleanup, status, nil
	case ModeMutagenOnly:
		if h.tryMutagen != nil {
			cl, st, e := h.tryMutagen()
			if e != nil {
				return nil, MutagenSyncStatus{}, e
			}
			if cl == nil {
				cl = func() {}
			}
			return cl, st, nil
		}
		return func() {}, MutagenSyncStatus{}, errors.New("mock tryMutagen 未注入")
	case ModeSSHFSOnly:
		if h.trySSHFS != nil {
			cl, e := h.trySSHFS()
			if e != nil {
				return nil, MutagenSyncStatus{}, e
			}
			if cl == nil {
				cl = func() {}
			}
			return cl, MutagenSyncStatus{}, nil
		}
		return func() {}, MutagenSyncStatus{}, errors.New("mock trySSHFS 未注入")
	}
	return func() {}, MutagenSyncStatus{}, fmt.Errorf("未知 mode: %v", mode)
}

// tryModeReal 是生产路径：调用真实 mountSSHFS / mountMutagen / mountMerge。
// 本阶段对 Mutagen / Mergerfs 只做最小串接，整链路集成测试由 Plan 03 落地。
func tryModeReal(connA, connB *ssh.Client, mode Mode, cfg MountConfig) (cleanup func(), status MutagenSyncStatus, err error) {
	// SSHFSOnly：v2.0 路径，已稳定
	if mode == ModeSSHFSOnly {
		cl, e := mountSSHFS(connA, cfg.Cwd, cfg.Cwd)
		if e != nil {
			return nil, MutagenSyncStatus{}, e
		}
		// 启动 watcher（v2.0 行为不变；watcher 在 Plan 03 / Phase 32 进一步包装 ctx）
		return cl, MutagenSyncStatus{}, nil
	}

	// MutagenOnly / Full：调 mountMutagen
	mutagenCfg := MutagenSyncConfig{
		AlphaCwd:        cfg.Cwd,
		BetaPath:        "/workspace-hot",
		ClaudeAccountID: cfg.ClaudeAccountID,
		SessionName:     buildSessionName(cfg.ClaudeAccountID, cfg.Cwd),
	}

	mCleanup, mStatus, mErr := mountMutagen(connA, mutagenCfg, defaultMutagenDeps())
	if mErr != nil {
		return nil, MutagenSyncStatus{}, mErr
	}

	if mode == ModeMutagenOnly {
		return mCleanup, mStatus, nil
	}

	// Full：mutagen 起来后，挂 sshfs 到 /workspace-cold，再 mergerfs 合并到 /workspace
	sCleanup, sErr := mountSSHFS(connA, cfg.Cwd, "/workspace-cold")
	if sErr != nil {
		mCleanup()
		return nil, MutagenSyncStatus{}, sErr
	}

	branches := []string{"/workspace-hot=RW", "/workspace-cold=NC,RO"}
	mergeCleanup, mergeErr := mountMerge(connA, branches, "/workspace")
	if mergeErr != nil {
		sCleanup()
		mCleanup()
		return nil, MutagenSyncStatus{}, mergeErr
	}

	// 启动 sshfs_watcher：cold 抖动 → 摘除 cold branch
	ctx, cancel := context.WithCancel(context.Background())
	watcher := NewSSHFSWatcher(connA, "/workspace-cold", cfg.Logger, func() error {
		return RemoveBranch(connA, "/workspace-cold", "/workspace")
	})
	go watcher.Run(ctx)

	cleanup = func() {
		cancel()
		mergeCleanup()
		sCleanup()
		mCleanup()
	}
	return cleanup, mStatus, nil
}

// printProgress 按 finalMode 输出三段式中文进度（CONTEXT D-18）。
// 每段对应 mutagen / sshfs / merge 三层；非该层时打印 "跳过 (模式: <mode>)"。
func printProgress(w io.Writer, mode Mode) {
	switch mode {
	case ModeFull:
		fmt.Fprintln(w, "(1/3) 热同步源码中…")
		fmt.Fprintln(w, "(2/3) 启动冷兜底…")
		fmt.Fprintln(w, "(3/3) 合并视图…")
	case ModeMutagenOnly:
		fmt.Fprintln(w, "(1/3) 热同步源码中…")
		fmt.Fprintf(w, "(2/3) 跳过 sshfs（模式: %s）\n", mode.String())
		fmt.Fprintf(w, "(3/3) 跳过 mergerfs（模式: %s）\n", mode.String())
	case ModeSSHFSOnly:
		fmt.Fprintf(w, "(1/3) 跳过 Mutagen（模式: %s）\n", mode.String())
		fmt.Fprintln(w, "(2/3) 启动冷兜底…")
		fmt.Fprintf(w, "(3/3) 跳过 mergerfs（模式: %s）\n", mode.String())
	}
}

// printBanner 输出 mount ready banner，着色规则按 CONTEXT D-17。
func printBanner(w io.Writer, mode Mode, noColor bool) {
	enabled := false
	if fh, ok := w.(fdHolder); ok {
		enabled = colorEnabled(noColor, fh)
	}
	color := ansiYellow
	if mode == ModeFull {
		color = ansiGreen
	}
	text := fmt.Sprintf("✓ 文件映射就绪 [%s]", mode.String())
	fmt.Fprintln(w, colorize(text, color, enabled))
}

// applyDowngrade 输出降级 banner 到 stderr 并 append 到 snapshot.DowngradeChain。
// M13 防御「禁止静默降级」的核心实现。
func applyDowngrade(w io.Writer, snap *LastSessionSnapshot, from, to Mode, code errcodes.Code, reason string) {
	fmt.Fprintln(w, errcodes.Format(errcodes.MOUNT_AUTO_DOWNGRADED, from.String(), to.String(), string(code), reason))
	snap.DowngradeChain = append(snap.DowngradeChain, DowngradeStep{
		From:          from.String(),
		To:            to.String(),
		ReasonCode:    string(code),
		ReasonMessage: reason,
	})
}

// extractErrCodeAndReason 尝试从 error 中识别 errcodes.Code（通过 errors.As）。
// 若未实现 codedError 接口则返回通用 code = MOUNT_FORCE_MODE_FAILED + 错误文本。
func extractErrCodeAndReason(err error) (errcodes.Code, string) {
	var ce codedError
	if errors.As(err, &ce) {
		return ce.Code(), ce.Reason()
	}
	return errcodes.MOUNT_FORCE_MODE_FAILED, err.Error()
}

// codedError 是 mount_mutagen / mount_merge 内部 sentinel error 的通用接口。
// 通过 errors.As(err, &ce) 让 mount_strategy 拿到结构化的 Code + reason。
type codedError interface {
	error
	Code() errcodes.Code
	Reason() string
}

// writeLastSessionWarn 调用 WriteLastSession 失败时只打 warn，不阻断 mount。
func writeLastSessionWarn(path string, snap LastSessionSnapshot, w io.Writer) {
	if path == "" {
		return
	}
	if err := WriteLastSession(path, snap); err != nil {
		fmt.Fprintf(w, "warning: 写 last-session.json 失败: %v\n", err)
	}
}

// buildSessionName 生成 mutagen sync session 名：
//
//	cloud-claude-{account_id_or_anon}-{cwd_hash8}
func buildSessionName(accountID, cwd string) string {
	owner := accountID
	if owner == "" {
		owner = "anon"
	}
	h := simpleHash8(cwd)
	return fmt.Sprintf("cloud-claude-%s-%s", owner, h)
}

// simpleHash8 返回 cwd 的 8 字节 fnv64a hex 摘要（不要求加密强度）。
func simpleHash8(s string) string {
	const (
		offset64 = uint64(14695981039346656037)
		prime64  = uint64(1099511628211)
	)
	h := offset64
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= prime64
	}
	return fmt.Sprintf("%08x", uint32(h))
}
