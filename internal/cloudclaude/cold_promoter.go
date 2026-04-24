package cloudclaude

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/errcodes"
)

const promotionDedupWindow = 5 * time.Second

// ColdPromoter 在容器内常驻，通过 inotify 监听 cold 分支的文件读事件，
// 将命中的文件经 SFTP 异步拉取到 hot 分支（晋升），实现"读一次变热"。
//
// PromotionEngine 内部行为：
//   - 5s 去重窗口：同 path 在 5s 内重复入队只触发 1 次实际 SFTP 拉取
//   - 失败退避：按 1/2/4s 指数退避重试，第 3 次仍失败时熔断
//   - 熔断：失败 3 次后将文件加入熔断集合，本次会话不再尝试
//
// 公开 API 用于 doctor 可观测（QueueDepth / Stats / Wait）。
type ColdPromoter struct {
	connB    *ssh.Client
	coldRoot string
	hotRoot  string
	logger   io.Writer
	pidFile  string

	ctx    context.Context
	cancel context.CancelFunc
	doneCh chan struct{}

	// PromotionEngine 内部状态
	mu             sync.Mutex
	queue          map[string]*promotionEntry
	events         chan string
	circuitBreaker map[string]struct{}

	// 统计（atomic）
	promotionCount       atomic.Int64
	promotionBytes       atomic.Int64
	promotionFailedCount atomic.Int64
}

type promotionEntry struct {
	path          string
	lastEnqueued  time.Time
}

// NewColdPromoter 构造 ColdPromoter，绑定 connB（SFTP 数据面）+ coldRoot（inotify 监听源）+
// hotRoot（晋升目标）。logger 用于 stderr 输出，pidFile 用于写入 PID（doctor promoter_alive
// check 依赖）。
func NewColdPromoter(connB *ssh.Client, coldRoot, hotRoot string, logger io.Writer, pidFile string) *ColdPromoter {
	ctx, cancel := context.WithCancel(context.Background())
	cp := &ColdPromoter{
		connB:          connB,
		coldRoot:       coldRoot,
		hotRoot:        hotRoot,
		logger:         logger,
		pidFile:        pidFile,
		ctx:            ctx,
		cancel:         cancel,
		doneCh:         make(chan struct{}),
		queue:          make(map[string]*promotionEntry),
		events:         make(chan string, 1024),
		circuitBreaker: make(map[string]struct{}),
	}
	return cp
}

// ---------- inotify 初始化与生命周期（平台相关，通过包级 var 注入） ----------

// promoterInitInotify 由平台文件（cold_promoter_linux.go / cold_promoter_notlinux.go）在 init() 中赋值。
var promoterInitInotifyFn = func(coldRoot string) (int, error) {
	return -1, fmt.Errorf("inotify 未初始化（平台适配缺失）")
}

// promoterCloseInotify 关闭 inotify fd。Linux 上使用 unix.Close；其它平台为 noop。
var promoterCloseInotifyFn = func(fd int) error {
	return nil
}

// promoterInotifyBufSize 是 inotify 读取缓冲区大小（单位字节）。
// Linux 上为 unix.SizeofInotifyEvent * 1024；其它平台同样值。
var promoterInotifyBufSize = 16 * 1024

// promoterReadEvents 从 fd 读取 inotify 事件并处理（解析 → 入队）。
// 平台文件注入：Linux 上使用 unix.Read + unix.InotifyEvent 解析；
// 其它平台为 noop 实现（直接返回）。
var promoterReadEventsFn = func(fd int, buf []byte, enqueue func(string)) error {
	return nil
}

// initInotify 是 ColdPromoter.inotify 初始化的包装，调用平台注入的 promoterInitInotifyFn。
func (cp *ColdPromoter) initInotify() (int, error) {
	return promoterInitInotifyFn(cp.coldRoot)
}

// ---------- PID file ----------

func (cp *ColdPromoter) writePIDFile() error {
	return os.WriteFile(cp.pidFile, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0o644)
}

// ---------- 公开 API ----------

// QueueDepth 返回 PromotionEngine 内部队列深度（doctor 可观测）。
func (cp *ColdPromoter) QueueDepth() int {
	return len(cp.events)
}

// Stats 返回晋升统计（promotionCount, promotionBytes, promotionFailedCount）。
func (cp *ColdPromoter) Stats() (int, int64, int) {
	return int(cp.promotionCount.Load()), cp.promotionBytes.Load(), int(cp.promotionFailedCount.Load())
}

// Wait 等待 Run 返回（用于 cleanup LIFO）。
func (cp *ColdPromoter) Wait() {
	<-cp.doneCh
}

// ---------- Run ----------

// Run 阻塞运行 inotify 事件循环 + PromotionEngine；ctx cancel 后 drain 剩余事件并返回。
func (cp *ColdPromoter) Run(ctx context.Context) {
	defer close(cp.doneCh)
	defer os.Remove(cp.pidFile)

	fd, err := cp.initInotify()
	if err != nil {
		fmt.Fprintln(cp.logger, errcodes.Format(errcodes.MOUNT_PROMOTER_FAILED, err.Error()))
		return
	}
	defer promoterCloseInotifyFn(fd)

	if err := cp.writePIDFile(); err != nil {
		fmt.Fprintln(cp.logger, errcodes.Format(errcodes.MOUNT_PROMOTER_FAILED, err.Error()))
		return
	}

	// 启动 PromotionEngine 消费循环
	go cp.runPromotionLoop(ctx)

	// inotify 主循环
	buf := make([]byte, promoterInotifyBufSize)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if err := promoterReadEventsFn(fd, buf, cp.enqueue); err != nil {
			return
		}
	}
}

// ---------- Enqueue（入队 + 去重） ----------

// enqueue 将 path 加入晋升队列。5s 去重窗口内重复入队只更新 lastEnqueued，
// 不触发新的 SFTP 拉取。events channel 满时静默丢弃（不阻塞 inotify 循环）。
func (cp *ColdPromoter) enqueue(path string) {
	cp.mu.Lock()
	entry, exists := cp.queue[path]
	now := time.Now()
	if exists {
		if now.Sub(entry.lastEnqueued) < promotionDedupWindow {
			entry.lastEnqueued = now
			cp.mu.Unlock()
			return // 5s 窗口内重复，丢弃
		}
		entry.lastEnqueued = now
	} else {
		cp.queue[path] = &promotionEntry{path: path, lastEnqueued: now}
	}
	cp.mu.Unlock()
	// 异步入队
	select {
	case cp.events <- path:
	default:
		// events channel 满了，丢弃（buffer=1024，溢出说明晋升积压严重）
	}
}

// ---------- runPromotionLoop（消费循环） ----------

func (cp *ColdPromoter) runPromotionLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case path := <-cp.events:
			go cp.promotePath(path) // 异步 goroutine，不阻塞循环
		}
	}
}

// ---------- promotePath（SFTP 拉取 + 重试 + 熔断） ----------

func (cp *ColdPromoter) promotePath(relPath string) {
	// 熔断检查
	cp.mu.Lock()
	if _, blocked := cp.circuitBreaker[relPath]; blocked {
		cp.mu.Unlock()
		return
	}
	cp.mu.Unlock()

	remotePath := filepath.Join(cp.coldRoot, relPath)
	localPath := filepath.Join(cp.hotRoot, relPath)

	backoffs := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(backoffs[attempt-1])
		}
		n, err := cp.copyFromCold(remotePath, localPath)
		if err == nil {
			cp.promotionCount.Add(1)
			cp.promotionBytes.Add(n)
			return
		}
		lastErr = err
	}
	// 3 次失败：stderr + 熔断
	fmt.Fprintf(cp.logger, "[!] 晋升失败 %s: %v\n", relPath, lastErr)
	cp.promotionFailedCount.Add(1)
	cp.mu.Lock()
	cp.circuitBreaker[relPath] = struct{}{}
	cp.mu.Unlock()
}

// ---------- copyFromCold（SFTP 拉取实现） ----------

// promoterSFTPClientFactory 由测试注入 fake SFTP client。
// 默认使用 sftp.NewClient。
var promoterSFTPClientFactory = func(conn *ssh.Client) (*sftp.Client, error) {
	return sftp.NewClient(conn)
}

// copyFromCold 通过 SFTP 从 remotePath 拉取文件到 localPath。
// 返回写入的字节数和可能的错误。
func (cp *ColdPromoter) copyFromCold(remotePath, localPath string) (int64, error) {
	client, err := promoterSFTPClientFactory(cp.connB)
	if err != nil {
		return 0, fmt.Errorf("创建 SFTP client 失败: %w", err)
	}
	defer client.Close()

	src, err := client.Open(remotePath)
	if err != nil {
		return 0, fmt.Errorf("打开远端文件 %s 失败: %w", remotePath, err)
	}
	defer src.Close()

	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		return 0, fmt.Errorf("创建本地目录 %s 失败: %w", filepath.Dir(localPath), err)
	}
	dst, err := os.Create(localPath)
	if err != nil {
		return 0, fmt.Errorf("创建本地文件 %s 失败: %w", localPath, err)
	}
	defer dst.Close()

	n, err := io.Copy(dst, src)
	if err != nil {
		return 0, fmt.Errorf("下载远端文件 %s 失败: %w", remotePath, err)
	}
	if err := dst.Close(); err != nil {
		return n, fmt.Errorf("关闭本地文件 %s 失败: %w", localPath, err)
	}
	return n, nil
}
