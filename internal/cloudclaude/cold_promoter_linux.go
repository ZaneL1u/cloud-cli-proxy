//go:build linux

package cloudclaude

import (
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/unix"
)

func init() {
	promoterInitInotifyFn = linuxInitInotify
	promoterCloseInotifyFn = linuxCloseInotify
	promoterInotifyBufSize = unix.SizeofInotifyEvent * 1024
	promoterReadEventsFn = linuxReadEvents
}

func linuxInitInotify(coldRoot string) (int, error) {
	fd, err := unix.InotifyInit()
	if err != nil {
		return -1, fmt.Errorf("inotify_init 失败: %w", err)
	}
	_, err = unix.InotifyAddWatch(fd, coldRoot, unix.IN_OPEN|unix.IN_ACCESS|unix.IN_CLOSE_NOWRITE)
	if err != nil {
		unix.Close(fd)
		return -1, fmt.Errorf("inotify_add_watch 失败: %w", err)
	}
	return fd, nil
}

func linuxCloseInotify(fd int) error {
	return unix.Close(fd)
}

func linuxReadEvents(fd int, buf []byte, enqueue func(string)) error {
	n, err := unix.Read(fd, buf)
	if err != nil {
		return err
	}
	processInotifyEvents(buf[:n], enqueue)
	return nil
}

// processInotifyEvents 解析 buf 中的 inotify 事件，提取文件名后调用 enqueue。
func processInotifyEvents(buf []byte, enqueue func(string)) {
	for offset := 0; offset < len(buf); {
		event := (*unix.InotifyEvent)(unsafe.Pointer(&buf[offset]))
		if event.Len > 0 {
			nameStart := offset + unix.SizeofInotifyEvent
			nameEnd := nameStart + int(event.Len)
			if nameEnd <= len(buf) {
				nameBytes := buf[nameStart:nameEnd]
				name := strings.TrimRight(string(nameBytes), "\x00")
				if name != "" {
					enqueue(name)
				}
			}
		}
		offset += unix.SizeofInotifyEvent + int(event.Len)
	}
}
