package cloudclaude

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/errcodes"
)

// fakeDirEntry 是 fs.DirEntry 的最小实现，仅供 readDirEntries hook 测试。
type fakeDirEntry struct {
	name string
	dir  bool
}

func (e fakeDirEntry) Name() string               { return e.name }
func (e fakeDirEntry) IsDir() bool                { return e.dir }
func (e fakeDirEntry) Type() fs.FileMode          { return 0 }
func (e fakeDirEntry) Info() (fs.FileInfo, error) { return nil, nil }

// stubAskpass 在测试时替代真实 NewAskpassHelper —— 不写文件，cleanup 计数。
func stubAskpass(cleanups *int) func() (*AskpassHelper, error) {
	return func() (*AskpassHelper, error) {
		return &AskpassHelper{
			ScriptPath: "/tmp/fake-askpass.sh",
			cleanup:    func() { *cleanups++ },
		}, nil
	}
}

// baseDeps 构造 Mutagen happy-path deps：所有真实远端 / 本地命令均替换为可控 stub。
func baseDeps(t *testing.T, alphaDir string) (mountMutagenDeps, *int, *[]string) {
	t.Helper()
	cleanups := 0
	calls := []string{}
	tmp := t.TempDir()
	deps := mountMutagenDeps{
		extractBinary: func(dst string) error { return nil },
		runLocal: func(name string, args []string, env []string) (string, error) {
			calls = append(calls, name+" "+strings.Join(args, " "))
			return "", nil
		},
		remoteRun: func(conn *ssh.Client, cmd string) (string, error) { return "", nil },
		remoteVersion: func(conn *ssh.Client) (string, error) {
			return "0.18.1", nil
		},
		remoteFindBeta: func(conn *ssh.Client, path string) (bool, error) {
			return false, nil
		},
		localDuBytes:   func(path string) (int64, error) { return 1024, nil },
		localDuTopN:    func(path string) ([]string, error) { return nil, nil },
		writeIgnoreYML: func(path string) error { return nil },
		newAskpass:     stubAskpass(&cleanups),
		readDirEntries: func(path string) ([]fs.DirEntry, error) {
			return []fs.DirEntry{fakeDirEntry{name: "main.go"}}, nil
		},
		mutagenBinPath: filepath.Join(tmp, "mutagen"),
		dataDir:        filepath.Join(tmp, "data"),
		defaultsYml:    filepath.Join(tmp, "defaults.yml"),
	}
	return deps, &cleanups, &calls
}

func newSyncCfg(alphaDir string) MutagenSyncConfig {
	return MutagenSyncConfig{
		AlphaCwd:        alphaDir,
		BetaPath:        "/workspace-hot",
		SSHUser:         "u",
		SSHHost:         "h",
		SSHPort:         22,
		Password:        "secret",
		ClaudeAccountID: "acct-1",
		SessionName:     "cloud-claude-acct-1-aabbccdd",
	}
}

// asMutagenCode 解包 mountMutagen 返回的 sentinel error，提取 errcodes.Code。
func asMutagenCode(t *testing.T, err error) errcodes.Code {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var ce codedError
	if !errors.As(err, &ce) {
		t.Fatalf("error is not codedError: %v", err)
	}
	return ce.Code()
}

func Test_SafetyGuard_AlphaEmptyBetaNonEmpty(t *testing.T) {
	deps, _, _ := baseDeps(t, t.TempDir())
	deps.readDirEntries = func(path string) ([]fs.DirEntry, error) {
		return []fs.DirEntry{}, nil
	}
	deps.remoteFindBeta = func(conn *ssh.Client, path string) (bool, error) {
		return true, nil
	}
	syncCreated := false
	deps.runLocal = func(name string, args []string, env []string) (string, error) {
		if len(args) > 1 && args[0] == "sync" && args[1] == "create" {
			syncCreated = true
		}
		return "", nil
	}

	_, _, err := mountMutagen(&ssh.Client{}, newSyncCfg(t.TempDir()), deps)
	if got := asMutagenCode(t, err); got != errcodes.MOUNT_MUTAGEN_SAFETY_GUARD {
		t.Fatalf("got code %s, want MOUNT_MUTAGEN_SAFETY_GUARD", got)
	}
	if syncCreated {
		t.Fatal("sync create must not run when safety guard fires")
	}
}

func Test_50MBReject(t *testing.T) {
	deps, _, _ := baseDeps(t, t.TempDir())
	deps.localDuBytes = func(path string) (int64, error) {
		return 60_000_000, nil
	}
	deps.localDuTopN = func(path string) ([]string, error) {
		return []string{"node_modules (40MB)", "build (15MB)"}, nil
	}
	syncCreated := false
	deps.runLocal = func(name string, args []string, env []string) (string, error) {
		if len(args) > 1 && args[0] == "sync" && args[1] == "create" {
			syncCreated = true
		}
		return "", nil
	}

	_, _, err := mountMutagen(&ssh.Client{}, newSyncCfg(t.TempDir()), deps)
	if got := asMutagenCode(t, err); got != errcodes.MOUNT_MUTAGEN_WHITELIST_REJECT {
		t.Fatalf("got code %s, want MOUNT_MUTAGEN_WHITELIST_REJECT", got)
	}
	if syncCreated {
		t.Fatal("sync create must not run when 50MB reject fires")
	}
	if !strings.Contains(err.Error(), "node_modules") {
		t.Errorf("error should include top dir hints, got: %v", err)
	}
}

func Test_VersionSkew(t *testing.T) {
	deps, _, _ := baseDeps(t, t.TempDir())
	deps.remoteVersion = func(conn *ssh.Client) (string, error) {
		return "0.99.0", nil
	}

	_, _, err := mountMutagen(&ssh.Client{}, newSyncCfg(t.TempDir()), deps)
	if got := asMutagenCode(t, err); got != errcodes.MOUNT_MUTAGEN_VERSION_SKEW {
		t.Fatalf("got code %s, want MOUNT_MUTAGEN_VERSION_SKEW", got)
	}
}

func Test_DaemonStartIdempotent(t *testing.T) {
	deps, _, _ := baseDeps(t, t.TempDir())
	deps.runLocal = func(name string, args []string, env []string) (string, error) {
		if len(args) > 0 && args[0] == "daemon" {
			return "daemon already started", errors.New("exit status 1")
		}
		return "", nil
	}

	_, _, err := mountMutagen(&ssh.Client{}, newSyncCfg(t.TempDir()), deps)
	if err != nil {
		t.Fatalf("daemon already started should be OK, got error: %v", err)
	}
}

func Test_MutagenHappyPath_CleansUpOnTerminate(t *testing.T) {
	deps, helperCleanups, calls := baseDeps(t, t.TempDir())
	cleanup, status, err := mountMutagen(&ssh.Client{}, newSyncCfg(t.TempDir()), deps)
	if err != nil {
		t.Fatalf("happy path failed: %v", err)
	}
	if status.ConflictCount != 0 {
		t.Errorf("happy path ConflictCount=%d, want 0", status.ConflictCount)
	}
	if status.SessionName != "cloud-claude-acct-1-aabbccdd" {
		t.Errorf("status.SessionName = %q, want fixture name", status.SessionName)
	}
	if cleanup == nil {
		t.Fatal("cleanup must not be nil")
	}
	cleanup()
	if *helperCleanups != 1 {
		t.Errorf("askpass cleanup called %d times, want 1", *helperCleanups)
	}
	// Plan 03 在 sync create 后追加了 sync list --template 调用，
	// 因此 happy path 总调用数 ≥4：daemon + sync create + sync list + sync terminate。
	if len(*calls) < 4 {
		t.Errorf("expected ≥4 runLocal calls (daemon + sync create + sync list + terminate), got %d: %v", len(*calls), *calls)
	}
}

func Test_MutagenHealthCheck_ReasonsCorrect(t *testing.T) {
	cases := []struct {
		name                string
		daemon, agent, sync bool
		conflicts           int
		wantReasonContains  string
	}{
		{"daemon down", false, false, false, 0, "daemon"},
		{"agent down", true, false, false, 0, "agent"},
		{"sync down", true, true, false, 0, "sync"},
		{"conflicts", true, true, true, 3, "冲突"},
		{"all ok", true, true, true, 0, "ok"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st := MutagenHealthCheck(c.daemon, c.agent, c.sync, c.conflicts)
			if !strings.Contains(st.Reason, c.wantReasonContains) {
				t.Errorf("reason=%q, want contains %q", st.Reason, c.wantReasonContains)
			}
		})
	}
}

func Test_WriteMutagenDefaultsYML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "defaults.yml")
	if err := writeMutagenDefaultsYML(path); err != nil {
		t.Fatalf("write yml failed: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"node_modules/", "target/", ".venv/", "vcs: true"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("yaml missing %q", want)
		}
	}
}

// 防回归：确保 daemon 调用不会在版本检测之前 nil-deref（connA == nil 路径）。
func Test_VersionSkewSkippedWhenConnNil(t *testing.T) {
	deps, _, _ := baseDeps(t, t.TempDir())
	deps.remoteVersion = func(conn *ssh.Client) (string, error) {
		t.Fatal("remoteVersion should not be called when conn is nil")
		return "", nil
	}

	_, _, err := mountMutagen(nil, newSyncCfg(t.TempDir()), deps)
	if err != nil {
		t.Fatalf("nil conn should skip remote version check: %v", err)
	}
}

// 防回归：cleanup 调用 mutagen sync terminate 后 askpass 也清理，不悬挂临时文件。
func Test_CleanupRunsTerminateAndAskpass(t *testing.T) {
	deps, helperCleanups, _ := baseDeps(t, t.TempDir())
	terminateSeen := false
	deps.runLocal = func(name string, args []string, env []string) (string, error) {
		if len(args) > 1 && args[0] == "sync" && args[1] == "terminate" {
			terminateSeen = true
		}
		return "", nil
	}
	cleanup, _, err := mountMutagen(&ssh.Client{}, newSyncCfg(t.TempDir()), deps)
	if err != nil {
		t.Fatalf("happy path failed: %v", err)
	}
	cleanup()
	if !terminateSeen {
		t.Error("cleanup did not invoke mutagen sync terminate")
	}
	if *helperCleanups != 1 {
		t.Errorf("askpass cleanup count=%d, want 1", *helperCleanups)
	}
	_ = time.Now
}
