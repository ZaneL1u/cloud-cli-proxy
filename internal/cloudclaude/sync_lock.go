// Package cloudclaude — Phase 32 Plan 03 账号级 Mutagen 单例锁。
//
// 在远端容器内通过 flock(1) 创建账号级单例：同一 claude_account 在
// 容器内同一时刻只能有一个 cloud-claude 后端持有锁。后端拿不到锁即
// 视为 "secondary client"，由 ssh.go 注入的 wrapper 输出
// [SESSION_SYNC_LOCKED] 错误码并把 ErrSyncLocked 透传给
// mount_strategy.MountWorkspace，让 Phase 31 既有降级逻辑切到
// sshfs-only（M15 双写防御）。
//
// 设计要点（CONTEXT D-17 / D-18 / D-19；RESEARCH §6）：
//   - 锁路径 /tmp/cloud-claude/locks/sync-<accountID>.lock
//     （ubuntu:24.04 系统级 lock 目录默认 root-only，UID 1000 mkdir EACCES — 见 RESEARCH §6.2）
//   - 远程命令 `flock -n -E 99 -F <path> -c 'echo $$; exec sleep infinity' & echo $!`
//     - -n: 非阻塞拿锁；拿不到立即退；
//     - -E 99: 自定义 lock 被占退出码（与 flock 默认 1 区分系统错误）；
//     - -F: no-fork，让 sleep infinity 直接持有 fd，SSH 关闭时收割 sleep
//       即等价收割 lock；缺 -F → flock 父进程仍持锁，SSH 断后锁不释放（RESEARCH §6.1）。
//   - accountID == "" → 返回 noop release（D-19 anon 路径不上锁）。
package cloudclaude

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"al.essio.dev/pkg/shellescape"
	"golang.org/x/crypto/ssh"
)

// ErrSyncLocked 表示另一端 cloud-claude 已持有同一 claude_account 的 Mutagen 单例锁。
//
// 调用方（mount_strategy.go::MountWorkspace 经 mountCfg.SyncSessionLock）应当：
//  1. 视为非致命，降级到 sshfs-only（只读视图）；
//  2. 写 last-session.json client_role="secondary"；
//  3. stderr 输出 [SESSION_SYNC_LOCKED] 错误码（已在 ssh.go 注入 wrapper 内统一处理）。
var ErrSyncLocked = errors.New("sync session locked by another cloud-claude")

// AcquireSyncLock 在远端容器内创建账号级单例锁。
//
// 行为：
//   - accountID == "" → 返回 (noop release, nil)（CONTEXT D-19 anon 路径跳过锁）
//   - 拿到锁 → 返回 (release func, nil)；release 通过另一个 ssh.Session 远程
//     `kill <pid> 2>/dev/null || true`，pid 来自 echo $! 输出。
//     ssh.Session.Close 也会自然收割 sleep infinity；release 是显式 cleanup。
//   - 锁被占 → 返回 (nil, ErrSyncLocked)（远端 flock 退 99）
//   - 其它 ssh / cmd 错误 → 返回 (nil, fmt.Errorf 包装)
//
// 远程命令模板（RESEARCH §6.3）：
//
//	mkdir -p /tmp/cloud-claude/locks 2>/dev/null && \
//	  flock -n -E 99 -F <lockPath_q> -c 'echo $$; exec sleep infinity' & echo $!
//
// stdout 末行 = bash 后台 PID（用于 release kill）。
func AcquireSyncLock(conn *ssh.Client, accountID string) (func(), error) {
	if accountID == "" {
		return func() {}, nil
	}
	if conn == nil {
		return nil, fmt.Errorf("AcquireSyncLock: nil ssh.Client")
	}

	lockPath := fmt.Sprintf("/tmp/cloud-claude/locks/sync-%s.lock", accountID)
	cmd := fmt.Sprintf(
		"mkdir -p /tmp/cloud-claude/locks 2>/dev/null && "+
			"flock -n -E 99 -F %s -c 'echo $$; exec sleep infinity' &\necho $!",
		shellescape.Quote(lockPath),
	)

	sess, err := conn.NewSession()
	if err != nil {
		return nil, fmt.Errorf("AcquireSyncLock 创建 SSH session 失败: %w", err)
	}
	out, runErr := sess.CombinedOutput(cmd)
	sess.Close()

	if runErr != nil {
		var exitErr *ssh.ExitError
		if errors.As(runErr, &exitErr) && exitErr.ExitStatus() == 99 {
			return nil, ErrSyncLocked
		}
		return nil, fmt.Errorf("AcquireSyncLock flock 启动失败 (output: %s): %w",
			strings.TrimSpace(string(out)), runErr)
	}

	pid := parseLastInt(string(out))
	if pid <= 0 {
		return func() {}, nil
	}

	release := func() {
		killSess, e := conn.NewSession()
		if e != nil {
			return
		}
		defer killSess.Close()
		_ = killSess.Run(fmt.Sprintf("kill %d 2>/dev/null || true", pid))
	}
	return release, nil
}

// parseLastInt 从多行字符串末尾提取最后一个连续数字行（PID）。
//
// 容错：空行 / 'lock acquired' 等非数字 echo / 负数 / 非法字符。
// 返回 0 表示未找到合法 PID（caller 自行兜底 — 通常视为已拿到锁但没解析到 PID，
// 极端边界用 noop release）。
//
// 暴露用于 sync_lock_test.go 边界覆盖（不再小写 unexport）。
func parseLastInt(s string) int {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			continue
		}
		n, err := strconv.Atoi(trimmed)
		if err == nil && n > 0 {
			return n
		}
	}
	return 0
}
