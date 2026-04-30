package cloudclaude

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/errcodes"
)

const (
	defaultHotSyncInterval = 1 * time.Second
	minHotSyncInterval     = 500 * time.Millisecond
	maxHotSyncInterval     = 5 * time.Second
	idleBackoffFactor      = 1.5
	activeSpeedupFactor    = 0.5
)

// HotSyncStatus 是热同步层返回的扩展状态。
// 供 banner / last-session.json 展示。
type HotSyncStatus struct {
	ConflictCount int
	// [Phase 36 D-07] initialSync 阶段填充：因 MaxFileBytes 熔断被跳过的单文件列表。
	// Path 为 cwd 相对路径（T-36-02-02 mitigate）；mount_strategy 透传到
	// snapshot.OversizedFiles 并按 D-08 在 stderr 一次性提示。
	OversizedFiles []OversizedFile
}

// HotSyncConfig 描述基于现有 SSH 隧道的热同步配置。
// 它替代外部同步工具，不再额外启动任何 ssh/scp/daemon 进程。
type HotSyncConfig struct {
	LocalDir       string
	RemoteDir      string
	ResetRemote    bool
	IgnorePatterns []string
	Logger         io.Writer
	Interval       time.Duration
	// [Phase 36 D-05] 单文件热同步大小上限（bytes）。
	// 零值表示不启用熔断；正值由 mount_strategy 注入
	// cfg.EffectiveHotSyncMaxFileMB() * 1024 * 1024。
	MaxFileBytes int64
	// Progress 提供极客风进度条 UI（可选，nil 时不输出）。
	Progress *ProgressUI
}

// syncFileState 是本地/远端单个文件的可比较快照。
// 仅保留 size + mtime（精度统一到秒），避免每轮轮询都做哈希。
type syncFileState struct {
	Size    int64
	ModTime time.Time
}

func normalizeSyncModTime(t time.Time) time.Time {
	return t.UTC().Truncate(time.Second)
}

type hotSyncErr struct {
	reason string
}

func newHotSyncErr(reason string) *hotSyncErr {
	return &hotSyncErr{reason: reason}
}

func (e *hotSyncErr) Error() string       { return errcodes.Format(errcodes.MOUNT_HOT_SYNC_FAILED, e.reason) }
func (e *hotSyncErr) Code() errcodes.Code { return errcodes.MOUNT_HOT_SYNC_FAILED }
func (e *hotSyncErr) Reason() string      { return e.reason }

// HotSyncEngine 在 connB 上维持一个 SFTP client，通过自适应轮询实现双向热同步。
// 约束：
//   - 不同步 ignore 命中的文件/目录（它们走冷层 sshfs）
//   - 只处理常规文件；符号链接和特殊文件跳过
//   - 启动期以本地目录为主，全量推到 remoteDir；运行期再做双向 reconcile
//   - 轮询间隔自适应：有变更时加速到 500ms，空闲时退避到 5s
//
// 自适应策略：
//   - 每次 syncOnce 检测到变更 → interval *= activeSpeedupFactor（最低 minHotSyncInterval）
//   - 连续 3 次无变更 → interval *= idleBackoffFactor（最高 maxHotSyncInterval）
//   - 避免固定 1s 轮询在大型仓库下浪费 CPU / 网络
type HotSyncEngine struct {
	connA     *ssh.Client
	connB     *ssh.Client
	client    *sftp.Client
	localDir  string
	remoteDir string
	logger    io.Writer
	interval  time.Duration
	matcher   *IgnoreMatcher
	resetRemote bool

	stopCh chan struct{}
	doneCh chan struct{}

	// last 是上一次成功同步后的统一状态快照。轮询时以它作为 base，
	// 判断 local/remote 哪一侧发生了变化。
	last map[string]syncFileState

	// [Phase 36 D-05] 从 HotSyncConfig.MaxFileBytes 复制；零值不熔断。
	maxFileBytes int64
	// [Phase 36 D-06/D-07] initialSync 阶段填充；run() goroutine 只读，
	// 通过 StartHotSync 返回值携带给 mount_strategy。
	oversized []OversizedFile

	// adaptive polling state
	idleStreak int // 连续无变更次数

	// progress 提供极客风进度条 UI（可选）。
	progress *ProgressUI
}

// StartHotSync 基于现有 SSH 连接启动热同步。
// connA 负责远端 mkdir / cleanup 等 shell 命令；connB 专供 SFTP 数据面。
func StartHotSync(connA, connB *ssh.Client, cfg HotSyncConfig) (cleanup func(), status HotSyncStatus, err error) {
	if connA == nil || connB == nil {
		return nil, HotSyncStatus{}, newHotSyncErr("hot sync 需要两条可用的 SSH 连接")
	}
	if cfg.LocalDir == "" || cfg.RemoteDir == "" {
		return nil, HotSyncStatus{}, newHotSyncErr("localDir / remoteDir 不能为空")
	}
	if cfg.Logger == nil {
		cfg.Logger = os.Stderr
	}
	if cfg.Interval <= 0 {
		cfg.Interval = defaultHotSyncInterval
	}

	client, err := sftp.NewClient(connB)
	if err != nil {
		return nil, HotSyncStatus{}, newHotSyncErr("创建 SFTP client 失败: " + err.Error())
	}

	engine := &HotSyncEngine{
		connA:        connA,
		connB:        connB,
		client:       client,
		localDir:     cfg.LocalDir,
		remoteDir:    cfg.RemoteDir,
		logger:       cfg.Logger,
		interval:     cfg.Interval,
		matcher:      NewIgnoreMatcher(cfg.LocalDir, cfg.IgnorePatterns),
		resetRemote:  cfg.ResetRemote,
		stopCh:       make(chan struct{}),
		doneCh:       make(chan struct{}),
		last:         make(map[string]syncFileState),
		maxFileBytes: cfg.MaxFileBytes,
		progress:     cfg.Progress,
	}

	if err := engine.prepareRemoteRoot(); err != nil {
		_ = client.Close()
		return nil, HotSyncStatus{}, newHotSyncErr("创建远端 hot 目录失败: " + err.Error())
	}

	if err := engine.initialSync(); err != nil {
		_ = client.Close()
		return nil, HotSyncStatus{}, err
	}

	go engine.run()

	cleanup = func() {
		close(engine.stopCh)
		<-engine.doneCh
		// 安全修复：退出时不做任何文件同步，直接关闭连接。
		// 之前的 syncOnce / syncOnceUploadOnly 在远程状态异常时
		// 会反向删除本地文件（applyRemote → deleteLocal），导致
		// 用户工作区数据丢失。取消映射即断开，不删文件。
		_ = engine.client.Close()
	}
	return cleanup, HotSyncStatus{
		OversizedFiles: engine.oversized,
	}, nil
}

func (e *HotSyncEngine) initialSync() error {
	scanCount := 0
	localFiles, err := scanLocalSyncFiles(e.localDir, e.matcher, func(path string) {
		scanCount++
		if e.progress != nil {
			e.progress.Scanning(path, scanCount)
		}
	})
	if err != nil {
		return newHotSyncErr("扫描本地目录失败: " + err.Error())
	}
	if e.progress != nil {
		e.progress.ScanDone(len(localFiles))
	}

	// 统计总数（熔断前）。
	var totalFiles, totalBytes int64
	for _, state := range localFiles {
		totalFiles++
		totalBytes += state.Size
	}

	// [Phase 36 D-06] 单文件大小熔断（第二层）。
	// 注意：scanLocalSyncFiles 内部的 IgnoreMatcher 已经第一层过滤掉 ignore 命中文件；
	// Phase 31 D-11 整目录级 SkipDir（在 scanLocalSyncFiles 内部）保持不动，
	// 与本处单文件级熔断互补，executor 不得删除或合并。
	oversizedSet := e.applyOversizedFilter(localFiles, true)

	// 统计 hot/cold。
	var hotFiles, hotBytes int64
	for _, state := range localFiles {
		hotFiles++
		hotBytes += state.Size
	}
	coldFiles := totalFiles - hotFiles
	coldBytes := totalBytes - hotBytes
	if e.progress != nil {
		e.progress.Distribution(hotFiles, hotBytes, coldFiles, coldBytes)
	}

	// 隐藏 staging 路径允许重置；直接映射到 cfg.Cwd 的 hot-only 路径则必须保守，
	// 不能清空用户可见目录。
	if e.resetRemote {
		if err := removeRemoteTree(e.client, e.remoteDir); err != nil {
			return newHotSyncErr("清理远端 hot staging 失败: " + err.Error())
		}
		if err := e.client.MkdirAll(e.remoteDir); err != nil {
			return newHotSyncErr("重建远端 hot staging 失败: " + err.Error())
		}
		rels := make([]string, 0, len(localFiles))
		for rel := range localFiles {
			rels = append(rels, rel)
		}
		sort.Strings(rels)
		for i, rel := range rels {
			if e.progress != nil {
				e.progress.Syncing(i, len(rels), rel)
			}
			if err := e.copyLocalToRemote(rel, localFiles[rel]); err != nil {
				return newHotSyncErr("初始化上传失败: " + err.Error())
			}
			e.last[rel] = localFiles[rel]
		}
		if e.progress != nil {
			e.progress.SyncDone(len(rels), len(rels))
		}
		return nil
	}

	remoteFiles, err := scanRemoteSyncFiles(e.client, e.remoteDir, e.matcher)
	if err != nil {
		return newHotSyncErr("扫描远端目录失败: " + err.Error())
	}
	// CR-01 修复：从 remoteFiles 中剔除 oversized 集合，避免 chooseConflictWinner
	// 在「本地被 filter 删除 + 远端有旧版本」场景命中 "remote" 分支后
	// applyRemote → copyRemoteToLocal 反向覆盖本地大文件。
	for rel := range oversizedSet {
		delete(remoteFiles, rel)
	}
	paths := make(map[string]struct{}, len(localFiles)+len(remoteFiles))
	for p := range localFiles {
		paths[p] = struct{}{}
	}
	for p := range remoteFiles {
		paths[p] = struct{}{}
	}
	rels := make([]string, 0, len(paths))
	for p := range paths {
		rels = append(rels, p)
	}
	sort.Strings(rels)
	for i, rel := range rels {
		if e.progress != nil {
			e.progress.Syncing(i, len(rels), rel)
		}
		localState, hasLocal := localFiles[rel]
		remoteState, hasRemote := remoteFiles[rel]
		switch chooseConflictWinner(localState, hasLocal, remoteState, hasRemote) {
		case "local":
			if err := e.applyLocal(rel, localState, hasLocal); err != nil {
				return newHotSyncErr("初始化双向收敛失败: " + err.Error())
			}
			if hasLocal {
				e.last[rel] = localState
			}
		default:
			if err := e.applyRemote(rel, remoteState, hasRemote); err != nil {
				return newHotSyncErr("初始化双向收敛失败: " + err.Error())
			}
			if hasRemote {
				e.last[rel] = remoteState
			}
		}
	}
	if e.progress != nil {
		e.progress.SyncDone(len(rels), len(rels))
	}
	return nil
}

func (e *HotSyncEngine) run() {
	defer close(e.doneCh)
	interval := e.interval
	if interval < minHotSyncInterval {
		interval = minHotSyncInterval
	}
	if interval > maxHotSyncInterval {
		interval = maxHotSyncInterval
	}
	timer := time.NewTimer(interval)
	defer timer.Stop()
	for {
		select {
		case <-e.stopCh:
			return
		case <-timer.C:
			hasChanges, err := e.syncOnceAdaptive(true)
			if err != nil && e.logger != nil {
				fmt.Fprintln(e.logger, "[!] 热同步轮询失败: "+err.Error())
			}
			interval = e.nextInterval(interval, hasChanges)
			timer.Reset(interval)
		}
	}
}

// nextInterval 根据是否有变更计算下一轮轮询间隔。
// 有变更 → 加速（缩短间隔）；连续无变更 → 退避（拉长间隔）。
func (e *HotSyncEngine) nextInterval(current time.Duration, hasChanges bool) time.Duration {
	if hasChanges {
		e.idleStreak = 0
		next := time.Duration(float64(current) * activeSpeedupFactor)
		if next < minHotSyncInterval {
			return minHotSyncInterval
		}
		return next
	}
	e.idleStreak++
	if e.idleStreak >= 3 {
		next := time.Duration(float64(current) * idleBackoffFactor)
		if next > maxHotSyncInterval {
			return maxHotSyncInterval
		}
		return next
	}
	return current
}

// syncOnceAdaptive 执行一次双向同步并返回是否检测到变更。
// 供 run() 自适应轮询使用；cleanup 最终收敛仍调用 syncOnce(false)。
func (e *HotSyncEngine) syncOnceAdaptive(logConflicts bool) (hasChanges bool, err error) {
	localFiles, err := scanLocalSyncFiles(e.localDir, e.matcher, nil)
	if err != nil {
		return false, fmt.Errorf("扫描本地目录失败: %w", err)
	}
	oversizedSet := e.applyOversizedFilter(localFiles, false)
	remoteFiles, err := scanRemoteSyncFiles(e.client, e.remoteDir, e.matcher)
	if err != nil {
		return false, fmt.Errorf("扫描远端目录失败: %w", err)
	}
	for rel := range oversizedSet {
		delete(remoteFiles, rel)
	}

	// 快速路径：无任何文件变化时直接返回
	if len(localFiles) == 0 && len(remoteFiles) == 0 && len(e.last) == 0 {
		return false, nil
	}

	paths := make(map[string]struct{}, len(e.last)+len(localFiles)+len(remoteFiles))
	for p := range e.last {
		paths[p] = struct{}{}
	}
	for p := range localFiles {
		paths[p] = struct{}{}
	}
	for p := range remoteFiles {
		paths[p] = struct{}{}
	}

	names := make([]string, 0, len(paths))
	for p := range paths {
		names = append(names, p)
	}
	sort.Strings(names)

	next := make(map[string]syncFileState, len(paths))
	changeCount := 0
	for _, rel := range names {
		localState, hasLocal := localFiles[rel]
		remoteState, hasRemote := remoteFiles[rel]
		baseState, hasBase := e.last[rel]

		localChanged := !sameSyncState(localState, hasLocal, baseState, hasBase)
		remoteChanged := !sameSyncState(remoteState, hasRemote, baseState, hasBase)

		switch {
		case !localChanged && !remoteChanged:
			if hasBase {
				next[rel] = baseState
			}
		case localChanged && !remoteChanged:
			if err := e.applyLocal(rel, localState, hasLocal); err != nil {
				return false, err
			}
			changeCount++
			if hasLocal {
				next[rel] = localState
			}
		case !localChanged && remoteChanged:
			if err := e.applyRemote(rel, remoteState, hasRemote); err != nil {
				return false, err
			}
			changeCount++
			if hasRemote {
				next[rel] = remoteState
			}
		default:
			winner := chooseConflictWinner(localState, hasLocal, remoteState, hasRemote)
			if logConflicts && e.logger != nil {
				fmt.Fprintf(e.logger, "⚠ 热同步冲突已自动解决：%s（保留 %s 侧）\n", rel, winner)
			}
			changeCount++
			if winner == "local" {
				if err := e.applyLocal(rel, localState, hasLocal); err != nil {
					return false, err
				}
				if hasLocal {
					next[rel] = localState
				}
			} else {
				if err := e.applyRemote(rel, remoteState, hasRemote); err != nil {
					return false, err
				}
				if hasRemote {
					next[rel] = remoteState
				}
			}
		}
	}

	e.last = next
	return changeCount > 0, nil
}

func (e *HotSyncEngine) syncOnce(logConflicts bool) error {
	localFiles, err := scanLocalSyncFiles(e.localDir, e.matcher, nil)
	if err != nil {
		return fmt.Errorf("扫描本地目录失败: %w", err)
	}
	// [Phase 36 L3] syncOnce 也跳过大文件，防止用户中途新增大文件被推到 hot。
	// 仅静默跳过，不更新 e.oversized：初始扫描列表已写入 last-session.json，
	// 轮询阶段不刷屏（D-22）。
	oversizedSet := e.applyOversizedFilter(localFiles, false)
	remoteFiles, err := scanRemoteSyncFiles(e.client, e.remoteDir, e.matcher)
	if err != nil {
		return fmt.Errorf("扫描远端目录失败: %w", err)
	}
	// CR-01 修复：syncOnce 也要把 oversized 从 remoteFiles 中剔除，
	// 防止 paths union 命中「本地不存在 + 远端存在 + base 不存在」分支后
	// applyRemote → copyRemoteToLocal 把远端旧版本写回本地。
	for rel := range oversizedSet {
		delete(remoteFiles, rel)
	}

	paths := make(map[string]struct{}, len(e.last)+len(localFiles)+len(remoteFiles))
	for p := range e.last {
		paths[p] = struct{}{}
	}
	for p := range localFiles {
		paths[p] = struct{}{}
	}
	for p := range remoteFiles {
		paths[p] = struct{}{}
	}

	names := make([]string, 0, len(paths))
	for p := range paths {
		names = append(names, p)
	}
	sort.Strings(names)

	next := make(map[string]syncFileState, len(paths))
	for _, rel := range names {
		localState, hasLocal := localFiles[rel]
		remoteState, hasRemote := remoteFiles[rel]
		baseState, hasBase := e.last[rel]

		localChanged := !sameSyncState(localState, hasLocal, baseState, hasBase)
		remoteChanged := !sameSyncState(remoteState, hasRemote, baseState, hasBase)

		switch {
		case !localChanged && !remoteChanged:
			if hasBase {
				next[rel] = baseState
			}
		case localChanged && !remoteChanged:
			if err := e.applyLocal(rel, localState, hasLocal); err != nil {
				return err
			}
			if hasLocal {
				next[rel] = localState
			}
		case !localChanged && remoteChanged:
			if err := e.applyRemote(rel, remoteState, hasRemote); err != nil {
				return err
			}
			if hasRemote {
				next[rel] = remoteState
			}
		default:
			winner := chooseConflictWinner(localState, hasLocal, remoteState, hasRemote)
			if logConflicts && e.logger != nil {
				fmt.Fprintf(e.logger, "⚠ 热同步冲突已自动解决：%s（保留 %s 侧）\n", rel, winner)
			}
			if winner == "local" {
				if err := e.applyLocal(rel, localState, hasLocal); err != nil {
					return err
				}
				if hasLocal {
					next[rel] = localState
				}
			} else {
				if err := e.applyRemote(rel, remoteState, hasRemote); err != nil {
					return err
				}
				if hasRemote {
					next[rel] = remoteState
				}
			}
		}
	}

	e.last = next
	return nil
}

// syncOnceUploadOnly 只把本地变更上传到远端，不做反向下载。
// 用于 Full 模式退出收敛，防止远端 staging 的意外修改反向覆盖本地。
func (e *HotSyncEngine) syncOnceUploadOnly() error {
	localFiles, err := scanLocalSyncFiles(e.localDir, e.matcher, nil)
	if err != nil {
		return fmt.Errorf("扫描本地目录失败: %w", err)
	}
	// 跳过大文件（不更新 e.oversized，静默跳过）
	e.applyOversizedFilter(localFiles, false)

	// 上传新增/修改的文件
	for rel, localState := range localFiles {
		baseState, hasBase := e.last[rel]
		if sameSyncState(localState, true, baseState, hasBase) {
			continue
		}
		if err := e.copyLocalToRemote(rel, localState); err != nil {
			return fmt.Errorf("最终上传 %s 失败: %w", rel, err)
		}
		e.last[rel] = localState
	}

	// 删除远端已不存在的文件
	for rel := range e.last {
		if _, hasLocal := localFiles[rel]; !hasLocal {
			if err := e.deleteRemote(rel); err != nil {
				return fmt.Errorf("最终删除远端 %s 失败: %w", rel, err)
			}
			delete(e.last, rel)
		}
	}

	return nil
}

func (e *HotSyncEngine) applyLocal(rel string, state syncFileState, exists bool) error {
	if !exists {
		return e.deleteRemote(rel)
	}
	return e.copyLocalToRemote(rel, state)
}

func (e *HotSyncEngine) applyRemote(rel string, state syncFileState, exists bool) error {
	if !exists {
		// 安全修复：远程文件不存在时，不再自动删除本地文件。
		// 防止容器重启、mergerfs 异常卸载等场景导致远程目录暂时为空，
		// 从而批量删除用户本地工作区。改为记录日志并保留本地。
		if e.logger != nil {
			fmt.Fprintf(e.logger, "[!] 热同步保留本地文件：%s（远程已不存在，跳过删除）\n", rel)
		}
		return nil
	}
	return e.copyRemoteToLocal(rel, state)
}

func (e *HotSyncEngine) copyLocalToRemote(rel string, state syncFileState) error {
	localPath := filepath.Join(e.localDir, rel)
	remotePath := remoteJoin(e.remoteDir, rel)

	src, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("打开本地文件 %s 失败: %w", rel, err)
	}
	defer src.Close()

	if err := e.client.MkdirAll(filepath.ToSlash(filepath.Dir(remotePath))); err != nil {
		return fmt.Errorf("创建远端目录 %s 失败: %w", rel, err)
	}

	dst, err := e.client.Create(remotePath)
	if err != nil {
		return fmt.Errorf("创建远端文件 %s 失败: %w", rel, err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return fmt.Errorf("上传远端文件 %s 失败: %w", rel, err)
	}
	if err := dst.Close(); err != nil {
		return fmt.Errorf("关闭远端文件 %s 失败: %w", rel, err)
	}
	_ = e.client.Chtimes(remotePath, state.ModTime, state.ModTime)
	return nil
}

func (e *HotSyncEngine) copyRemoteToLocal(rel string, state syncFileState) error {
	remotePath := remoteJoin(e.remoteDir, rel)
	localPath := filepath.Join(e.localDir, rel)

	src, err := e.client.Open(remotePath)
	if err != nil {
		return fmt.Errorf("打开远端文件 %s 失败: %w", rel, err)
	}
	defer src.Close()

	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		return fmt.Errorf("创建本地目录 %s 失败: %w", rel, err)
	}
	dst, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("创建本地文件 %s 失败: %w", rel, err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return fmt.Errorf("下载远端文件 %s 失败: %w", rel, err)
	}
	if err := dst.Close(); err != nil {
		return fmt.Errorf("关闭本地文件 %s 失败: %w", rel, err)
	}
	_ = os.Chtimes(localPath, state.ModTime, state.ModTime)
	return nil
}

func (e *HotSyncEngine) deleteRemote(rel string) error {
	remotePath := remoteJoin(e.remoteDir, rel)
	if err := e.client.Remove(remotePath); err != nil && !isSFTPNotExist(err) {
		return fmt.Errorf("删除远端文件 %s 失败: %w", rel, err)
	}
	return nil
}

func (e *HotSyncEngine) deleteLocal(rel string) error {
	localPath := filepath.Join(e.localDir, rel)
	if err := os.Remove(localPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除本地文件 %s 失败: %w", rel, err)
	}
	return nil
}

// applyOversizedFilter 对 scanLocalSyncFiles 返回的 map 做单文件大小熔断。
//
// recordOversized=true 时把命中文件追加到 e.oversized（initialSync 路径）；
// recordOversized=false 时仅静默 delete（syncOnce 路径，避免刷屏）。
//
// MaxFileBytes <= 0 时整段为 no-op，等价于 Phase 36 之前行为（不熔断）。
//
// CR-01 修复：除了 delete(localFiles, rel)，还必须 delete(e.last, rel)，
// 否则 HotOnly 模式（resetRemote=false）下，syncOnce 会把「本地不存在 +
// base 中存在」误判为本地删除 → applyLocal({},false) → deleteRemote → 远端
// 大文件被静默删除。同时返回 oversizedSet 给调用方，让 paths union 与
// remoteFiles 都跳过这些 rel，避免 initialSync 阶段的 chooseConflictWinner
// 把 remote 旧版本反向覆盖到本地。
func (e *HotSyncEngine) applyOversizedFilter(localFiles map[string]syncFileState, recordOversized bool) map[string]struct{} {
	if e.maxFileBytes <= 0 {
		return nil
	}
	oversizedSet := make(map[string]struct{})
	for rel, state := range localFiles {
		if state.Size >= e.maxFileBytes {
			if recordOversized {
				e.oversized = append(e.oversized, OversizedFile{Path: rel, SizeBytes: state.Size})
			}
			delete(localFiles, rel)
			delete(e.last, rel)
			oversizedSet[rel] = struct{}{}
		}
	}
	return oversizedSet
}

func scanLocalSyncFiles(root string, matcher *IgnoreMatcher, onFile func(path string)) (map[string]syncFileState, error) {
	files := make(map[string]syncFileState)
	count := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if path == root {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)

		if d.IsDir() {
			if isHardcodedSkipDir(d.Name()) || matcher.IsIgnoredRel(rel, true) {
				return fs.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if matcher.IsIgnoredRel(rel, false) {
			return nil
		}
		info, err := d.Info()
		if err != nil || !info.Mode().IsRegular() {
			return nil
		}
		files[rel] = syncFileState{
			Size:    info.Size(),
			ModTime: normalizeSyncModTime(info.ModTime()),
		}
		count++
		if onFile != nil {
			onFile(rel)
		}
		return nil
	})
	return files, err
}

func scanRemoteSyncFiles(client *sftp.Client, root string, matcher *IgnoreMatcher) (map[string]syncFileState, error) {
	files := make(map[string]syncFileState)
	if _, err := client.Stat(root); err != nil {
		if isSFTPNotExist(err) {
			return files, nil
		}
		return nil, err
	}

	var walk func(dir, relBase string) error
	walk = func(dir, relBase string) error {
		entries, err := client.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			rel := entry.Name()
			if relBase != "" {
				rel = relBase + "/" + entry.Name()
			}
			if entry.IsDir() {
				if isHardcodedSkipDir(entry.Name()) || matcher.IsIgnoredRel(rel, true) {
					continue
				}
				if err := walk(remoteJoin(dir, entry.Name()), rel); err != nil {
					return err
				}
				continue
			}
			if !entry.Mode().IsRegular() {
				continue
			}
			if matcher.IsIgnoredRel(rel, false) {
				continue
			}
			files[rel] = syncFileState{
				Size:    entry.Size(),
				ModTime: normalizeSyncModTime(entry.ModTime()),
			}
		}
		return nil
	}

	if err := walk(root, ""); err != nil {
		return nil, err
	}
	return files, nil
}

func sameSyncState(a syncFileState, hasA bool, b syncFileState, hasB bool) bool {
	if hasA != hasB {
		return false
	}
	if !hasA {
		return true
	}
	return a.Size == b.Size && a.ModTime.Equal(b.ModTime)
}

func chooseConflictWinner(local syncFileState, hasLocal bool, remote syncFileState, hasRemote bool) string {
	switch {
	case hasLocal && !hasRemote:
		return "local"
	case !hasLocal && hasRemote:
		return "remote"
	case !hasLocal && !hasRemote:
		return "local"
	case local.ModTime.After(remote.ModTime):
		return "local"
	case remote.ModTime.After(local.ModTime):
		return "remote"
	case local.Size >= remote.Size:
		return "local"
	default:
		return "remote"
	}
}

func ensureRemoteWritableDir(conn *ssh.Client, path string) error {
	cmd := fmt.Sprintf(
		"mkdir -p %s 2>/dev/null || (sudo mkdir -p %s && sudo chown $(id -u):$(id -g) %s)",
		shellQuote(path), shellQuote(path), shellQuote(path),
	)
	return sshRun(conn, cmd)
}

func (e *HotSyncEngine) prepareRemoteRoot() error {
	// Full 模式的隐藏 staging 在 /tmp 下，可直接通过 SFTP 目录 API 管理，
	// 避免再占用 connA 上的 shell session。
	if isHiddenStagePath(e.remoteDir) {
		return e.client.MkdirAll(e.remoteDir)
	}
	return ensureRemoteWritableDir(e.connA, e.remoteDir)
}

func remoteJoin(base, rel string) string {
	base = strings.TrimRight(base, "/")
	rel = strings.TrimPrefix(filepath.ToSlash(rel), "/")
	if rel == "" {
		return base
	}
	return base + "/" + rel
}

func isSFTPNotExist(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not exist") || strings.Contains(msg, "no such file")
}

func buildStagePaths(cwd string) (base, hot, cold string) {
	base = "/tmp/.cloud-claude-mounts/" + simpleHash8(cwd)
	hot = base + "/hot"
	cold = base + "/cold"
	return base, hot, cold
}

func isHiddenStagePath(path string) bool {
	return strings.HasPrefix(path, "/tmp/.cloud-claude-mounts/")
}

func removeRemoteTree(client *sftp.Client, root string) error {
	info, err := client.Stat(root)
	if err != nil {
		if isSFTPNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return client.Remove(root)
	}
	entries, err := client.ReadDir(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		child := remoteJoin(root, entry.Name())
		if entry.IsDir() {
			if err := removeRemoteTree(client, child); err != nil {
				return err
			}
			continue
		}
		if err := client.Remove(child); err != nil && !isSFTPNotExist(err) {
			return err
		}
	}
	if err := client.RemoveDirectory(root); err != nil && !isSFTPNotExist(err) {
		return err
	}
	return nil
}
