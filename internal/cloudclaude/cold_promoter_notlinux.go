//go:build !linux

package cloudclaude

import (
	"errors"
)

func init() {
	promoterInitInotifyFn = notLinuxInitInotify
	promoterCloseInotifyFn = notLinuxCloseInotify
	promoterInotifyBufSize = 16 * 1024
	promoterReadEventsFn = notLinuxReadEvents
}

func notLinuxInitInotify(coldRoot string) (int, error) {
	return -1, errors.New("inotify 仅支持 Linux 平台")
}

func notLinuxCloseInotify(fd int) error {
	return nil
}

func notLinuxReadEvents(fd int, buf []byte, enqueue func(string)) error {
	return errors.New("inotify 不可用")
}
