package cloudclaude

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/errcodes"
)

// MutagenSyncConfig 是 mountMutagen 的 per-call 入参，由 mount_strategy 拼装。
type MutagenSyncConfig struct {
	AlphaCwd        string
	BetaPath        string
	SSHUser         string
	SSHHost         string
	SSHPort         int
	Password        string
	ClaudeAccountID string
	SessionName     string
}

// MutagenStatus 供 Phase 34 doctor 复用的健康状态包装。
// 本阶段 mount_strategy 内部不直接消费，但导出以稳定 API。
type MutagenStatus struct {
	DaemonReady bool
	AgentReady  bool
	SyncReady   bool
	Conflicts   int
	Reason      string
}

// MutagenHealthCheck 把四个 bool/int 包装成 MutagenStatus + Reason 文案。
// Phase 34 doctor `mount` 子命令直接输出该 struct。
func MutagenHealthCheck(daemonReady, agentReady, syncReady bool, conflicts int) MutagenStatus {
	st := MutagenStatus{
		DaemonReady: daemonReady,
		AgentReady:  agentReady,
		SyncReady:   syncReady,
		Conflicts:   conflicts,
	}
	switch {
	case !daemonReady:
		st.Reason = "Mutagen daemon 未启动"
	case !agentReady:
		st.Reason = "Mutagen agent 未握手"
	case !syncReady:
		st.Reason = "Mutagen sync session 未就绪"
	case conflicts > 0:
		st.Reason = fmt.Sprintf("Mutagen sync 有 %d 个冲突", conflicts)
	default:
		st.Reason = "ok"
	}
	return st
}

// mountMutagenDeps 是 mountMutagen 的依赖注入集合，全字段 nil 时 defaultMutagenDeps() 兜底。
// 单测通过覆写其中字段进入受控路径。
type mountMutagenDeps struct {
	extractBinary  func(dst string) error
	runLocal       func(name string, args []string, env []string) (stdout string, err error)
	remoteRun      func(conn *ssh.Client, cmd string) (stdout string, err error)
	remoteVersion  func(conn *ssh.Client) (string, error)
	remoteFindBeta func(conn *ssh.Client, path string) (nonEmpty bool, err error)
	localDuBytes   func(path string) (int64, error)
	localDuTopN    func(path string) ([]string, error)
	writeIgnoreYML func(path string) error
	newAskpass     func() (*AskpassHelper, error)
	readDirEntries func(path string) ([]fs.DirEntry, error)

	// 以下是受测试影响的常量（mutagenBinPath / dataDir）；nil 时按真实 home 计算。
	mutagenBinPath string
	dataDir        string
	defaultsYml    string
}

// defaultMutagenDeps 返回生产路径的 deps（exec.Command 包装）。
func defaultMutagenDeps() mountMutagenDeps {
	return mountMutagenDeps{
		extractBinary: ExtractMutagenBinary,
		runLocal: func(name string, args []string, env []string) (string, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, name, args...)
			if env != nil {
				cmd.Env = env
			}
			out, err := cmd.CombinedOutput()
			return string(out), err
		},
		remoteRun: func(conn *ssh.Client, cmd string) (string, error) {
			sess, err := conn.NewSession()
			if err != nil {
				return "", err
			}
			defer sess.Close()
			out, err := sess.CombinedOutput(cmd)
			return string(out), err
		},
		remoteVersion: func(conn *ssh.Client) (string, error) {
			sess, err := conn.NewSession()
			if err != nil {
				return "", err
			}
			defer sess.Close()
			out, err := sess.CombinedOutput("cat /etc/cloud-claude/mutagen.version 2>/dev/null || true")
			return strings.TrimSpace(string(out)), err
		},
		remoteFindBeta: func(conn *ssh.Client, path string) (bool, error) {
			sess, err := conn.NewSession()
			if err != nil {
				return false, err
			}
			defer sess.Close()
			cmd := fmt.Sprintf("find %s -mindepth 1 -maxdepth 1 -not -name '.*' 2>/dev/null | head -1", shellQuote(path))
			out, err := sess.CombinedOutput(cmd)
			if err != nil {
				return false, nil
			}
			return strings.TrimSpace(string(out)) != "", nil
		},
		localDuBytes: func(path string) (int64, error) {
			var total int64
			err := filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
				if err != nil {
					return nil
				}
				if d.IsDir() {
					name := d.Name()
					if name == "node_modules" || name == "target" || name == "dist" ||
						name == ".venv" || name == "__pycache__" || name == ".next" ||
						name == "build" || name == ".cache" || name == ".git" {
						return fs.SkipDir
					}
					return nil
				}
				if info, e := d.Info(); e == nil {
					total += info.Size()
				}
				return nil
			})
			return total, err
		},
		localDuTopN: func(path string) ([]string, error) {
			entries, err := os.ReadDir(path)
			if err != nil {
				return nil, err
			}
			type sized struct {
				name string
				size int64
			}
			items := make([]sized, 0, len(entries))
			for _, e := range entries {
				if info, ierr := e.Info(); ierr == nil {
					items = append(items, sized{name: e.Name(), size: info.Size()})
				}
			}
			sort.Slice(items, func(i, j int) bool { return items[i].size > items[j].size })
			top := make([]string, 0, 3)
			for i := 0; i < len(items) && i < 3; i++ {
				top = append(top, fmt.Sprintf("%s (%dMB)", items[i].name, items[i].size/1024/1024))
			}
			return top, nil
		},
		writeIgnoreYML: writeMutagenDefaultsYML,
		newAskpass:     NewAskpassHelper,
		readDirEntries: func(path string) ([]fs.DirEntry, error) {
			return os.ReadDir(path)
		},
	}
}

// mountMutagen 是 Mutagen 单层挂载入口。
//
// 执行序：
//  1. 抽取 mutagen 二进制到 ~/.cloud-claude/bin/mutagen
//  2. mutagen daemon start（"daemon already started" 视为 OK）
//  3. 版本握手（local v0.18.1 vs remote /etc/cloud-claude/mutagen.version）
//  4. 50MB 体积检查（cwd > 52428800 → MOUNT_MUTAGEN_WHITELIST_REJECT）
//  5. 安全门（alpha 空目录 + remote /workspace-hot 非空 → MOUNT_MUTAGEN_SAFETY_GUARD，**不可降级**）
//  6. 写 ~/.cloud-claude/mutagen-defaults.yml
//  7. 创建 askpass helper（密码不进 argv）
//  8. mutagen sync create（conn-C 由 mutagen 内部 fork ssh 子进程承担）
//
// cleanup：mutagen sync terminate <SessionName> + helper.Cleanup()。daemon 不停（CONTEXT D-05）。
func mountMutagen(connA *ssh.Client, cfg MutagenSyncConfig, deps mountMutagenDeps) (cleanup func(), conflicts int, err error) {
	deps = mergeMutagenDeps(deps)

	// 1) 抽取二进制
	binPath := deps.mutagenBinPath
	if err := deps.extractBinary(binPath); err != nil {
		return nil, 0, newMutagenErr(errcodes.MOUNT_MUTAGEN_TRANSPORT_FAILED, err.Error())
	}

	// 2) daemon start（幂等）
	env := []string{"MUTAGEN_DATA_DIRECTORY=" + deps.dataDir}
	out, derr := deps.runLocal(binPath, []string{"daemon", "start"}, env)
	if derr != nil && !strings.Contains(out, "daemon already started") && !strings.Contains(out, "already running") {
		return nil, 0, newMutagenErr(errcodes.MOUNT_MUTAGEN_DAEMON_UNAVAILABLE, derr.Error())
	}

	// 3) 版本握手
	if connA != nil {
		remoteVer, _ := deps.remoteVersion(connA)
		if remoteVer != "" && !strings.Contains(remoteVer, strings.TrimPrefix(MutagenBinaryVersion, "v")) {
			return nil, 0, newMutagenErr(errcodes.MOUNT_MUTAGEN_VERSION_SKEW, MutagenBinaryVersion, remoteVer)
		}
	}

	// 4) 50MB 体积检查（REQ-F1-D） — 阈值 50 * 1024 * 1024 = 52428800
	const fiftyMB = int64(52428800)
	size, _ := deps.localDuBytes(cfg.AlphaCwd)
	if size > fiftyMB {
		top, _ := deps.localDuTopN(cfg.AlphaCwd)
		topStr := strings.Join(top, ", ")
		if topStr == "" {
			topStr = "(无大子目录信息)"
		}
		return nil, 0, newMutagenErr(errcodes.MOUNT_MUTAGEN_WHITELIST_REJECT,
			cfg.AlphaCwd, size/1024/1024, topStr)
	}

	// 5) 安全门（C5 / D-13）— Fatal，不可降级
	entries, _ := deps.readDirEntries(cfg.AlphaCwd)
	if isAlphaEmpty(entries) && connA != nil {
		nonEmpty, _ := deps.remoteFindBeta(connA, "/workspace-hot")
		if nonEmpty {
			return nil, 0, newMutagenErr(errcodes.MOUNT_MUTAGEN_SAFETY_GUARD, cfg.AlphaCwd)
		}
	}

	// 6) 写 ignore yaml
	if err := deps.writeIgnoreYML(deps.defaultsYml); err != nil {
		return nil, 0, newMutagenErr(errcodes.MOUNT_MUTAGEN_SYNC_FAILED, "写 mutagen-defaults.yml 失败: "+err.Error())
	}

	// 7) askpass helper
	helper, herr := deps.newAskpass()
	if herr != nil {
		return nil, 0, newMutagenErr(errcodes.MOUNT_MUTAGEN_TRANSPORT_FAILED, "创建 askpass helper 失败: "+herr.Error())
	}

	// 8) mutagen sync create
	beta := fmt.Sprintf("%s@%s:%d:/workspace-hot", cfg.SSHUser, cfg.SSHHost, cfg.SSHPort)
	createArgs := []string{
		"sync", "create",
		"--name=" + cfg.SessionName,
		"--mode=two-way-resolved",
		"--default-owner-beta=id:1000",
		"--default-group-beta=id:1000",
		"--ignore-vcs",
		"--global-config=" + deps.defaultsYml,
		cfg.AlphaCwd,
		beta,
	}
	createEnv := append([]string{"MUTAGEN_DATA_DIRECTORY=" + deps.dataDir}, helper.Env(cfg.Password)...)
	createOut, cerr := deps.runLocal(binPath, createArgs, createEnv)
	if cerr != nil {
		helper.Cleanup()
		return nil, 0, newMutagenErr(errcodes.MOUNT_MUTAGEN_SYNC_FAILED,
			fmt.Sprintf("%s: %s", cerr.Error(), strings.TrimSpace(createOut)))
	}

	cleanup = func() {
		_, _ = deps.runLocal(binPath, []string{"sync", "terminate", cfg.SessionName},
			[]string{"MUTAGEN_DATA_DIRECTORY=" + deps.dataDir})
		helper.Cleanup()
	}
	return cleanup, 0, nil
}

// isAlphaEmpty 过滤 ignore 列表后判断 alpha 目录是否「业务上为空」。
// node_modules / .git / dist 等不算业务文件。
func isAlphaEmpty(entries []fs.DirEntry) bool {
	skip := map[string]bool{
		"node_modules": true, "target": true, "dist": true, ".venv": true,
		"__pycache__": true, ".next": true, "build": true, ".cache": true,
		".git": true, ".DS_Store": true,
	}
	for _, e := range entries {
		name := e.Name()
		if skip[name] {
			continue
		}
		if strings.HasPrefix(name, ".") && !strings.HasPrefix(name, ".env") {
			continue
		}
		return false
	}
	return true
}

// writeMutagenDefaultsYML 在 path 写入 ignore yaml 模板（CONTEXT D-08 列表）。
func writeMutagenDefaultsYML(path string) error {
	const yml = `sync:
  defaults:
    ignore:
      vcs: true
      paths:
        - "node_modules/"
        - "target/"
        - "dist/"
        - "*.pyc"
        - ".venv/"
        - "__pycache__/"
        - ".next/"
        - "build/"
        - ".cache/"
        - ".DS_Store"
`
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(yml), 0o600)
}

// mergeMutagenDeps 用 defaultMutagenDeps 兜底用户传入的 deps 中 nil 的字段。
// 让单测只覆写感兴趣的字段。
func mergeMutagenDeps(d mountMutagenDeps) mountMutagenDeps {
	def := defaultMutagenDeps()
	if d.extractBinary == nil {
		d.extractBinary = def.extractBinary
	}
	if d.runLocal == nil {
		d.runLocal = def.runLocal
	}
	if d.remoteRun == nil {
		d.remoteRun = def.remoteRun
	}
	if d.remoteVersion == nil {
		d.remoteVersion = def.remoteVersion
	}
	if d.remoteFindBeta == nil {
		d.remoteFindBeta = def.remoteFindBeta
	}
	if d.localDuBytes == nil {
		d.localDuBytes = def.localDuBytes
	}
	if d.localDuTopN == nil {
		d.localDuTopN = def.localDuTopN
	}
	if d.writeIgnoreYML == nil {
		d.writeIgnoreYML = def.writeIgnoreYML
	}
	if d.newAskpass == nil {
		d.newAskpass = def.newAskpass
	}
	if d.readDirEntries == nil {
		d.readDirEntries = def.readDirEntries
	}
	if d.mutagenBinPath == "" {
		home, _ := os.UserHomeDir()
		d.mutagenBinPath = filepath.Join(home, ".cloud-claude", "bin", "mutagen")
	}
	if d.dataDir == "" {
		home, _ := os.UserHomeDir()
		d.dataDir = filepath.Join(home, ".cloud-claude", "mutagen")
	}
	if d.defaultsYml == "" {
		home, _ := os.UserHomeDir()
		d.defaultsYml = filepath.Join(home, ".cloud-claude", "mutagen-defaults.yml")
	}
	_ = runtime.GOOS // 仅保留 import 以便未来 platform-specific 行为
	return d
}

// mutagenErr 是 codedError 的具体实现，让 mount_strategy 通过 errors.As 拿到 Code + Reason。
//
// args 与 errcodes.Format 透传，支持多 placeholder 模板。
type mutagenErr struct {
	code errcodes.Code
	args []any
}

func newMutagenErr(code errcodes.Code, args ...any) *mutagenErr {
	return &mutagenErr{code: code, args: args}
}

func (e *mutagenErr) Error() string {
	return errcodes.Format(e.code, e.args...)
}

func (e *mutagenErr) Code() errcodes.Code { return e.code }
func (e *mutagenErr) Reason() string {
	if len(e.args) == 0 {
		return ""
	}
	parts := make([]string, 0, len(e.args))
	for _, a := range e.args {
		parts = append(parts, fmt.Sprint(a))
	}
	return strings.Join(parts, " ")
}
