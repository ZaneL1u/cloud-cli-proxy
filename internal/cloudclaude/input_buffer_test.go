package cloudclaude

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestBufferedStdin_ConnectedDirectWrite(t *testing.T) {
	src := strings.NewReader("hello")
	var state atomic.Int32
	state.Store(int32(StateConnected))
	var echo bytes.Buffer
	b, pipeR := NewBufferedStdin(src, &state, &echo, true, nil)

	go func() { _ = b.Run(context.Background()) }()

	got := make([]byte, 5)
	n, err := io.ReadFull(pipeR, got)
	if err != nil {
		t.Fatalf("io.ReadFull error = %v", err)
	}
	if n != 5 || !bytes.Equal(got, []byte("hello")) {
		t.Errorf("期望 pipeR 读到 hello，得 %q", got[:n])
	}
}

func TestBufferedStdin_ReconnectingBuffersAndGrayEchoes(t *testing.T) {
	src := strings.NewReader("ab")
	var state atomic.Int32
	state.Store(int32(StateReconnecting))
	var echo bytes.Buffer
	b, _ := NewBufferedStdin(src, &state, &echo, false, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_ = b.Run(ctx)

	if !bytes.Contains(echo.Bytes(), []byte(AnsiGray)) {
		t.Error("Reconnecting 状态应灰色 echo")
	}
	if string(b.ringBuf) != "ab" {
		t.Errorf("ringBuf = %q, want \"ab\"", string(b.ringBuf))
	}
}

func TestBufferedStdin_RingBufOverflowDropsAndWarns(t *testing.T) {
	data := bytes.Repeat([]byte("x"), 4097)
	src := bytes.NewReader(data)
	var state atomic.Int32
	state.Store(int32(StateReconnecting))
	var echo bytes.Buffer
	b, _ := NewBufferedStdin(src, &state, &echo, true, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_ = b.Run(ctx)
	if !bytes.Contains(echo.Bytes(), []byte("SESSION_BUFFER_OVERFLOW")) {
		t.Error("应输出 SESSION_BUFFER_OVERFLOW warning")
	}
}

func TestBufferedStdin_EnterTriggersOnEnter(t *testing.T) {
	src := strings.NewReader("hi\r")
	var state atomic.Int32
	state.Store(int32(StateReconnecting))
	triggered := atomic.Int32{}
	b, _ := NewBufferedStdin(src, &state, io.Discard, true, func() { triggered.Add(1) })
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_ = b.Run(ctx)
	if triggered.Load() < 1 {
		t.Error("\\r 应触发 onEnter")
	}
}

func TestBufferedStdin_FlushClearsBuffer(t *testing.T) {
	src := strings.NewReader("xyz")
	var state atomic.Int32
	state.Store(int32(StateReconnecting))
	b, pipeR := NewBufferedStdin(src, &state, io.Discard, true, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_ = b.Run(ctx)

	doneRead := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 3)
		n, _ := io.ReadFull(pipeR, buf)
		doneRead <- buf[:n]
	}()

	if err := b.Flush(); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-doneRead:
		if !bytes.Contains(got, []byte("xyz")) {
			t.Errorf("Flush 后 pipeR 应有 xyz，得 %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("flush 后 pipeR 未读到数据")
	}
	if len(b.ringBuf) != 0 {
		t.Errorf("Flush 后 ringBuf 应清空，得长度 %d", len(b.ringBuf))
	}
	_ = b.Close()
}
