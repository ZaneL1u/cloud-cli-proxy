package cloudclaude

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/errcodes"
)

// mergerfsOptions 是 mergerfs 命令的固定 -o 参数串（与 Phase 29 D-11 字符级一致）。
const mergerfsOptions = "category.create=ff,func.readdir=cor:4,cache.attr=30,cache.entry=30,cache.readdir=true,cache.files=off,inodecalc=path-hash"

// mountMerge 在 connA 上执行 sudo mergerfs 把 branches 合并挂载到 target。
//
// 默认 branches: ["/workspace-hot=RW", "/workspace-cold=RO"]，target=/workspace。
// 支持 CONTEXT D-26 的 CLOUD_CLAUDE_MERGERFS_BRANCHES 扩展点（由调用方解析后传入）。
//
// cleanup：sudo fusermount -uz <target>。
func mountMerge(connA *ssh.Client, branches []string, target string) (cleanup func(), err error) {
	if len(branches) == 0 {
		return nil, newMergeErr(errcodes.MOUNT_MERGERFS_FAILED, "branches 为空")
	}
	if target == "" {
		target = "/workspace"
	}

	// 远端 mkdir 目标（容器内已存在则 mkdir -p 自动 no-op）
	mkdirCmd := fmt.Sprintf(
		"mkdir -p %s 2>/dev/null || (sudo mkdir -p %s && sudo chown $(id -u):$(id -g) %s)",
		shellQuote(target), shellQuote(target), shellQuote(target),
	)
	if err := sshRun(connA, mkdirCmd); err != nil {
		return nil, newMergeErr(errcodes.MOUNT_MERGERFS_FAILED, "mkdir 失败: "+err.Error())
	}

	// 拼接 mergerfs 命令（branches 用 ":" 分隔）。
	// 注意：mergerfs 是长驻 FUSE 进程，不能像普通命令那样用 sshRun(session.Run)
	// 跑完即回收；否则 SSH session 结束时 FUSE 进程也会被带死。这里仿照 mountSSHFS
	// 的模式：单独起一个长期 session 持有 mergerfs，cleanup 时再显式 fusermount。
	// 另外 mergerfs 必须以当前 workspace 用户运行，不能 sudo 变成 root：
	// cold 分支是 workspace 用户挂起的 sshfs mount，root 访问会触发
	// std::filesystem status(Permission denied)。
	branchesStr := strings.Join(branches, ":")
	mountCmd := fmt.Sprintf("mergerfs -o %s %s %s",
		mergerfsOptions, branchesStr, shellQuote(target))

	session, err := connA.NewSession()
	if err != nil {
		return nil, newMergeErr(errcodes.MOUNT_MERGERFS_FAILED, "创建 mergerfs session 失败: "+err.Error())
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		session.Close()
		return nil, newMergeErr(errcodes.MOUNT_MERGERFS_FAILED, "获取 mergerfs stderr 失败: "+err.Error())
	}

	var stderrBuf bytes.Buffer
	go func() {
		_, _ = io.Copy(&stderrBuf, stderr)
	}()

	if err := session.Start(mountCmd); err != nil {
		session.Close()
		return nil, newMergeErr(errcodes.MOUNT_MERGERFS_FAILED, "启动 mergerfs 失败: "+err.Error())
	}

	done := make(chan error, 1)
	go func() {
		done <- session.Wait()
	}()

	check := func() error {
		return sshRun(connA, fmt.Sprintf("mountpoint -q %s", shellQuote(target)))
	}
	if err := waitForMount(target, check, 200*time.Millisecond, 10*time.Second); err != nil {
		_ = session.Close()
		<-done
		if stderrText := strings.TrimSpace(stderrBuf.String()); stderrText != "" {
			return nil, newMergeErr(errcodes.MOUNT_MERGERFS_FAILED, stderrText)
		}
		return nil, newMergeErr(errcodes.MOUNT_MERGERFS_FAILED, err.Error())
	}

	cleanup = func() {
		_ = sshRun(connA, fmt.Sprintf("fusermount -uz %s 2>/dev/null || sudo fusermount -uz %s 2>/dev/null || true", shellQuote(target), shellQuote(target)))
		_ = session.Close()
		<-done
	}
	return cleanup, nil
}

// RemoveBranch 摘除已挂载 mergerfs 的指定 branch（cold 抖动 watcher 触发）。
//
// 命令（与 RESEARCH §2.2 字符级一致）:
//
//	setfattr -n user.mergerfs.branches -v "-<branchPath>" <target>
//
// 失败先尝试无 sudo，再尝试 sudo 包装；皆失败返回 MOUNT_MERGERFS_FAILED。
func RemoveBranch(connA *ssh.Client, branchPath, target string) error {
	value := "-" + branchPath
	cmd := fmt.Sprintf("setfattr -n user.mergerfs.branches -v %s %s",
		shellQuote(value), shellQuote(target))
	if err := sshRun(connA, cmd); err == nil {
		return nil
	}
	sudoCmd := "sudo " + cmd
	if err := sshRun(connA, sudoCmd); err != nil {
		return newMergeErr(errcodes.MOUNT_MERGERFS_FAILED, "setfattr 摘除 branch 失败: "+err.Error())
	}
	return nil
}

// mergeErr 让 mount_strategy 通过 errors.As 识别 MOUNT_MERGERFS_FAILED。
type mergeErr struct {
	code errcodes.Code
	args []any
}

func newMergeErr(code errcodes.Code, args ...any) *mergeErr {
	return &mergeErr{code: code, args: args}
}

func (e *mergeErr) Error() string       { return errcodes.Format(e.code, e.args...) }
func (e *mergeErr) Code() errcodes.Code { return e.code }
func (e *mergeErr) Reason() string {
	if len(e.args) == 0 {
		return ""
	}
	parts := make([]string, 0, len(e.args))
	for _, a := range e.args {
		parts = append(parts, fmt.Sprint(a))
	}
	return strings.Join(parts, " ")
}
