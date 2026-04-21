package cloudclaude

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/errcodes"
)

// RingBufCapacity 是默认环形缓冲容量；planner 已选 4096
// （RESEARCH §4.5 调到 8192 的预留容量）。
const RingBufCapacity = 4096

// BufferedStdin 把 os.Stdin 用 io.Pipe 中转：
//   - state==Connected → 直写 pipeW
//   - state==Reconnecting → 4KB ringBuf + 灰色 ANSI echo 到 localEcho
//   - state==GaveUp → 丢弃
//
// ringBuf 满 → 丢最早 1KB + [SESSION_BUFFER_OVERFLOW] warning（CONTEXT D-06）。
type BufferedStdin struct {
	src     io.Reader
	pipeW   io.WriteCloser
	state   *atomic.Int32 // 共享 Reconnector.state
	ringBuf []byte
	ringMu  sync.Mutex
	// echoMu 保护 grayOpen + pipeW.Write + closeGrayIfOpen 跨 goroutine 调用
	// （Phase 32 Gap #1 / WR-04 co-fix —— Run 与 Flush 并发执行时避免 data race）。
	// 锁顺序：echoMu 外 / ringMu 内（仅 Flush 嵌套，handleReconnecting 串行不嵌套）。
	echoMu    sync.Mutex
	localEcho io.Writer
	noColor   bool
	onEnter   func()
	grayOpen  bool // 是否已 echo 过开头 \x1b[90m（在 echoMu 下读写）
}

// NewBufferedStdin 用 io.Pipe 拿到 (pipeR, pipeW)；返回的 io.Reader 直接喂给 ssh.Session.Stdin。
//
//   - state 指针必须由调用方共享给 reconnect.Reconnector（同一 atomic.Int32）。
//   - localEcho 是 os.Stdout（断网期间灰色未确认渲染目标）。
//   - onEnter 在 state==Reconnecting 且检测到 \r/\n 时调用（通常 = reconnect.Trigger）。
func NewBufferedStdin(src io.Reader, state *atomic.Int32, localEcho io.Writer,
	noColor bool, onEnter func(),
) (*BufferedStdin, io.Reader) {
	pr, pw := io.Pipe()
	b := &BufferedStdin{
		src:       src,
		pipeW:     pw,
		state:     state,
		ringBuf:   make([]byte, 0, RingBufCapacity),
		localEcho: localEcho,
		noColor:   noColor,
		onEnter:   onEnter,
	}
	return b, pr
}

// Run 阻塞读 src 字节；按 state 分发处理。
func (b *BufferedStdin) Run(ctx context.Context) error {
	buf := make([]byte, 1)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		n, err := b.src.Read(buf)
		if n > 0 {
			c := buf[0]
			switch ConnState(b.state.Load()) {
			case StateConnected:
				// echoMu 保护 closeGrayIfOpen + pipeW.Write 与 Flush / handleReconnecting 并发安全。
				b.echoMu.Lock()
				b.closeGrayIfOpenLocked()
				_, werr := b.pipeW.Write(buf[:n])
				b.echoMu.Unlock()
				if werr != nil {
					return werr
				}
			case StateReconnecting:
				b.handleReconnecting(c)
			case StateGaveUp:
				// 丢弃
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

func (b *BufferedStdin) handleReconnecting(c byte) {
	b.ringMu.Lock()
	if len(b.ringBuf) >= RingBufCapacity {
		// 丢最早 1024 字节（CONTEXT D-06 第 2 条）
		drop := 1024
		if drop > len(b.ringBuf) {
			drop = len(b.ringBuf)
		}
		b.ringBuf = b.ringBuf[drop:]
		if b.localEcho != nil {
			fmt.Fprintln(b.localEcho, errcodes.Format(errcodes.SESSION_BUFFER_OVERFLOW))
		}
	}
	b.ringBuf = append(b.ringBuf, c)
	b.ringMu.Unlock()

	if b.localEcho != nil {
		// 进入 Reconnecting 时一次性 echo \x1b[90m，退出时 \x1b[0m（不逐字节包；RESEARCH §4.3）。
		// echoMu 保护 grayOpen 与 localEcho 写入，与 Run-Connected / Flush 并发安全（WR-04 co-fix）。
		b.echoMu.Lock()
		if !b.grayOpen && !b.noColor {
			fmt.Fprint(b.localEcho, AnsiGray)
			b.grayOpen = true
		}
		fmt.Fprintf(b.localEcho, "%c", c)
		b.echoMu.Unlock()
	}
	if c == '\r' || c == '\n' {
		if b.onEnter != nil {
			b.onEnter()
		}
	}
}

// closeGrayIfOpen 对外 API，自己管锁；用于不在 echoMu 内调用的场景。
func (b *BufferedStdin) closeGrayIfOpen() {
	b.echoMu.Lock()
	defer b.echoMu.Unlock()
	b.closeGrayIfOpenLocked()
}

// closeGrayIfOpenLocked 调用方必须已持有 echoMu。
func (b *BufferedStdin) closeGrayIfOpenLocked() {
	if b.grayOpen && b.localEcho != nil && !b.noColor {
		fmt.Fprint(b.localEcho, AnsiReset)
		b.grayOpen = false
	}
}

// Flush 把 ringBuf 内容按序写 pipeW；reconnect 成功的 onReconnected 回调中调用。
//
// 锁顺序：echoMu → ringMu（嵌套）；与 handleReconnecting（串行两锁）/ Run-Connected
// （仅 echoMu）兼容无死锁。
func (b *BufferedStdin) Flush() error {
	b.echoMu.Lock()
	defer b.echoMu.Unlock()
	b.closeGrayIfOpenLocked()

	b.ringMu.Lock()
	defer b.ringMu.Unlock()
	if len(b.ringBuf) == 0 {
		return nil
	}
	if _, err := b.pipeW.Write(b.ringBuf); err != nil {
		return err
	}
	b.ringBuf = b.ringBuf[:0]
	return nil
}

// Close 关闭底层 pipeW（cleanup 用）。
func (b *BufferedStdin) Close() error {
	if pc, ok := b.pipeW.(io.Closer); ok {
		return pc.Close()
	}
	return nil
}
