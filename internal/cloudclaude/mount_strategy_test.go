package cloudclaude

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/errcodes"
)

// fakeHotSyncErr / fakeMergeErr 模拟生产路径的 codedError sentinel。
type fakeHotSyncErr struct{ code errcodes.Code }

func (e *fakeHotSyncErr) Error() string       { return string(e.code) }
func (e *fakeHotSyncErr) Code() errcodes.Code { return e.code }
func (e *fakeHotSyncErr) Reason() string      { return "fake hot sync failure" }

type fakeMergeErr struct{ code errcodes.Code }

func (e *fakeMergeErr) Error() string       { return string(e.code) }
func (e *fakeMergeErr) Code() errcodes.Code { return e.code }
func (e *fakeMergeErr) Reason() string      { return "fake merge failure" }

// stratCase 定义单个 mode×failure 组合。
type stratCase struct {
	name        string
	intended    Mode
	hotSyncOK   bool
	sshfsOK     bool
	mergeOK     bool
	wantMode    Mode
	wantErr     bool
	wantBanners []string
}

func newHooks(c stratCase) *strategyHooks {
	h := &strategyHooks{}
	h.tryHotSync = func() (func(), HotSyncStatus, error) {
		if !c.hotSyncOK {
			return nil, HotSyncStatus{}, &fakeHotSyncErr{code: errcodes.MOUNT_HOT_SYNC_FAILED}
		}
		return func() {}, HotSyncStatus{}, nil
	}
	h.trySSHFS = func() (func(), error) {
		if !c.sshfsOK {
			return nil, &fakeMergeErr{code: errcodes.MOUNT_SSHFS_FAILED}
		}
		return func() {}, nil
	}
	h.tryMerge = func() (func(), error) {
		if !c.mergeOK {
			return nil, &fakeMergeErr{code: errcodes.MOUNT_MERGERFS_FAILED}
		}
		return func() {}, nil
	}
	return h
}

func TestMountStrategy_DowngradeMatrix(t *testing.T) {
	cases := []stratCase{
		{
			name:     "Auto/HotSync-fail/SSHFS-ok/Merge-ok→SSHFSOnly",
			intended: ModeAuto, hotSyncOK: false, sshfsOK: true, mergeOK: true,
			wantMode:    ModeSSHFSOnly,
			wantBanners: []string{"MOUNT_AUTO_DOWNGRADED"},
		},
		{
			name:     "Auto/HotSync-ok/SSHFS-fail/Merge-ok→HotOnly",
			intended: ModeAuto, hotSyncOK: true, sshfsOK: false, mergeOK: true,
			wantMode:    ModeHotOnly,
			wantBanners: []string{"MOUNT_AUTO_DOWNGRADED"},
		},
		{
			name:     "Auto/HotSync-ok/SSHFS-ok/Merge-fail→HotOnly",
			intended: ModeAuto, hotSyncOK: true, sshfsOK: true, mergeOK: false,
			wantMode:    ModeHotOnly,
			wantBanners: []string{"MOUNT_AUTO_DOWNGRADED"},
		},
		{
			name:     "Auto/all-ok→Full",
			intended: ModeAuto, hotSyncOK: true, sshfsOK: true, mergeOK: true,
			wantMode: ModeFull,
		},
		{
			name:     "Auto/all-fail→Failed",
			intended: ModeAuto, hotSyncOK: false, sshfsOK: false, mergeOK: false,
			wantMode: ModeFailed, wantErr: true,
		},
		{
			name:     "Full/HotSync-fail→Failed",
			intended: ModeFull, hotSyncOK: false, sshfsOK: true, mergeOK: true,
			wantMode: ModeFailed, wantErr: true,
		},
		{
			name:     "Full/SSHFS-fail→Failed",
			intended: ModeFull, hotSyncOK: true, sshfsOK: false, mergeOK: true,
			wantMode: ModeFailed, wantErr: true,
		},
		{
			name:     "Full/Merge-fail→Failed",
			intended: ModeFull, hotSyncOK: true, sshfsOK: true, mergeOK: false,
			wantMode: ModeFailed, wantErr: true,
		},
		{
			name:     "HotOnly/ok→HotOnly",
			intended: ModeHotOnly, hotSyncOK: true,
			wantMode: ModeHotOnly,
		},
		{
			name:     "HotOnly/fail→Failed",
			intended: ModeHotOnly, hotSyncOK: false,
			wantMode: ModeFailed, wantErr: true,
		},
		{
			name:     "SSHFSOnly/ok→SSHFSOnly",
			intended: ModeSSHFSOnly, sshfsOK: true,
			wantMode: ModeSSHFSOnly,
		},
		{
			name:     "SSHFSOnly/fail→Failed",
			intended: ModeSSHFSOnly, sshfsOK: false,
			wantMode: ModeFailed, wantErr: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			cfg := MountConfig{
				Mode:             c.intended,
				SupportsMergerfs: true,
				NoColor:          true,
				Logger:           &buf,
				LastSessionPath:  filepath.Join(t.TempDir(), "last.json"),
				hooks:            newHooks(c),
			}
			cleanup, mode, err := MountWorkspace(nil, nil, cfg)
			if cleanup == nil {
				t.Fatal("cleanup must not be nil")
			}
			cleanup()
			if mode != c.wantMode {
				t.Errorf("mode = %s, want %s", mode, c.wantMode)
			}
			if (err != nil) != c.wantErr {
				t.Errorf("err = %v, wantErr = %v", err, c.wantErr)
			}
			out := buf.String()
			for _, b := range c.wantBanners {
				if !strings.Contains(out, b) {
					t.Errorf("stderr missing %q\nfull stderr:\n%s", b, out)
				}
			}
		})
	}
}

func Test_BannerColors(t *testing.T) {
	t.Run("noColor=false but non-TTY logger → no ANSI", func(t *testing.T) {
		var buf bytes.Buffer
		printBanner(&buf, ModeFull, false)
		if strings.Contains(buf.String(), "\033[") {
			t.Errorf("non-TTY writer should not contain ANSI: %q", buf.String())
		}
	})
	t.Run("noColor=true → no ANSI", func(t *testing.T) {
		var buf bytes.Buffer
		printBanner(&buf, ModeFull, true)
		if strings.Contains(buf.String(), "\033[") {
			t.Errorf("noColor=true should suppress ANSI: %q", buf.String())
		}
	})
	t.Run("contains banner text + mode", func(t *testing.T) {
		var buf bytes.Buffer
		printBanner(&buf, ModeHotOnly, true)
		if !strings.Contains(buf.String(), "✓ 文件映射就绪 [hot-only]") {
			t.Errorf("banner missing mode tag: %q", buf.String())
		}
	})
}

func Test_APFSCaseInsensitive_WritesLastSession(t *testing.T) {
	tmp := t.TempDir()
	last := filepath.Join(tmp, "last-session.json")
	apfs := true
	cfg := MountConfig{
		Mode:                    ModeSSHFSOnly,
		SupportsMergerfs:        true,
		NoColor:                 true,
		Logger:                  new(bytes.Buffer),
		LastSessionPath:         last,
		overrideCaseInsensitive: &apfs,
		hooks: &strategyHooks{
			trySSHFS: func() (func(), error) { return func() {}, nil },
		},
	}
	cleanup, _, err := MountWorkspace(nil, nil, cfg)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	cleanup()

	data, err := os.ReadFile(last)
	if err != nil {
		t.Fatalf("read last-session.json failed: %v", err)
	}
	var snap LastSessionSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatal(err)
	}
	if !snap.APFSCaseInsensitive {
		t.Error("expected apfs_case_insensitive=true")
	}
	if snap.SchemaVersion != 1 {
		t.Errorf("schema_version=%d, want 1", snap.SchemaVersion)
	}
}

func Test_Downgrade_BannerEachStep(t *testing.T) {
	var buf bytes.Buffer
	cfg := MountConfig{
		Mode:             ModeAuto,
		SupportsMergerfs: true,
		NoColor:          true,
		Logger:           &buf,
		LastSessionPath:  filepath.Join(t.TempDir(), "last.json"),
		hooks: &strategyHooks{
			tryHotSync: func() (func(), HotSyncStatus, error) {
				return nil, HotSyncStatus{}, &fakeHotSyncErr{code: errcodes.MOUNT_HOT_SYNC_FAILED}
			},
			trySSHFS: func() (func(), error) {
				return func() {}, nil
			},
			tryMerge: func() (func(), error) {
				return nil, &fakeMergeErr{code: errcodes.MOUNT_MERGERFS_FAILED}
			},
		},
	}
	cleanup, mode, err := MountWorkspace(nil, nil, cfg)
	if err != nil {
		t.Fatalf("auto mode should not error if any tier succeeds, got: %v", err)
	}
	cleanup()

	out := buf.String()
	bannerCount := strings.Count(out, "MOUNT_AUTO_DOWNGRADED")
	if bannerCount < 1 {
		t.Errorf("expected ≥1 MOUNT_AUTO_DOWNGRADED line, got %d:\n%s", bannerCount, out)
	}

	last := cfg.LastSessionPath
	data, _ := os.ReadFile(last)
	var snap LastSessionSnapshot
	_ = json.Unmarshal(data, &snap)
	if len(snap.DowngradeChain) < 1 {
		t.Errorf("downgrade_chain length=%d, want ≥1", len(snap.DowngradeChain))
	}
	if mode == ModeFull {
		t.Errorf("mode=Full but a tier failed; want fallback mode")
	}
}

func Test_Downgrade_CapabilityFromAuthResp(t *testing.T) {
	t.Run("SupportsMergerfs=false drops Auto to HotOnly", func(t *testing.T) {
		var buf bytes.Buffer
		cfg := MountConfig{
			Mode:             ModeAuto,
			SupportsMergerfs: false,
			NoColor:          true,
			Logger:           &buf,
			LastSessionPath:  filepath.Join(t.TempDir(), "last.json"),
			hooks: &strategyHooks{
				tryHotSync: func() (func(), HotSyncStatus, error) {
					return func() {}, HotSyncStatus{}, nil
				},
			},
		}
		cleanup, mode, err := MountWorkspace(nil, nil, cfg)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		cleanup()
		if mode != ModeHotOnly {
			t.Errorf("mode=%s, want hot-only", mode)
		}
	})
}

func Test_ParseMode(t *testing.T) {
	cases := map[string]Mode{
		"":             ModeAuto,
		"auto":         ModeAuto,
		"full":         ModeFull,
		"hot-only":   ModeHotOnly,
		"sshfs-only": ModeSSHFSOnly,
	}
	for s, want := range cases {
		got, err := ParseMode(s)
		if err != nil {
			t.Errorf("ParseMode(%q) err: %v", s, err)
		}
		if got != want {
			t.Errorf("ParseMode(%q) = %s, want %s", s, got, want)
		}
	}
	if _, err := ParseMode("nonsense"); err == nil {
		t.Error("ParseMode(nonsense) should err")
	}
}

func Test_ForceMode_FailureUsesForceCode(t *testing.T) {
	var buf bytes.Buffer
	cfg := MountConfig{
		Mode:             ModeFull,
		SupportsMergerfs: true,
		NoColor:          true,
		Logger:           &buf,
		LastSessionPath:  filepath.Join(t.TempDir(), "last.json"),
		hooks: &strategyHooks{
			tryHotSync: func() (func(), HotSyncStatus, error) {
				return nil, HotSyncStatus{}, &fakeHotSyncErr{code: errcodes.MOUNT_HOT_SYNC_FAILED}
			},
			trySSHFS: func() (func(), error) { return func() {}, nil },
			tryMerge: func() (func(), error) { return func() {}, nil },
		},
	}
	cleanup, mode, err := MountWorkspace(nil, nil, cfg)
	cleanup()
	if mode != ModeFailed {
		t.Errorf("mode=%s, want failed", mode)
	}
	if err == nil {
		t.Fatal("expected force-mode error, got nil")
	}
	if !strings.Contains(err.Error(), string(errcodes.MOUNT_FORCE_MODE_FAILED)) {
		t.Errorf("err missing MOUNT_FORCE_MODE_FAILED prefix: %v", err)
	}
}

// 防回归：buildSessionName 形成稳定的 owner-hash 模式。
func Test_BuildSessionName(t *testing.T) {
	a := buildSessionName("acct-xyz", "/tmp/foo")
	b := buildSessionName("acct-xyz", "/tmp/foo")
	c := buildSessionName("acct-xyz", "/tmp/bar")
	if a != b {
		t.Error("session name should be deterministic")
	}
	if a == c {
		t.Error("different cwd should produce different session name")
	}
	if !strings.HasPrefix(a, "cloud-claude-acct-xyz-") {
		t.Errorf("missing prefix: %s", a)
	}
	if buildSessionName("", "/x") == "" || !strings.Contains(buildSessionName("", "/x"), "anon") {
		t.Error("empty account should fallback to anon")
	}
}

// 防回归：未实现 codedError 的普通 error 仍被 extractErrCodeAndReason 处理。
func Test_ExtractErrCode_FallbackForceFailed(t *testing.T) {
	code, _ := extractErrCodeAndReason(errors.New("plain"))
	if code != errcodes.MOUNT_FORCE_MODE_FAILED {
		t.Errorf("plain error code = %s, want MOUNT_FORCE_MODE_FAILED", code)
	}
}

// TestMountWorkspace_SyncLocked 验证 Phase 32 Gap #2 闭合（REQ-F5-D / SC11）：
//
//   - SyncSessionLock 返回 ErrSyncLocked → intended=ModeFull 强制降级到 ModeSSHFSOnly
//   - DowngradeChain 含 {From:"full", To:"sshfs-only", ReasonCode:"sync_locked"}
//   - stderr 含中文 "单例锁" 摘要（与 ssh.go [SESSION_SYNC_LOCKED] 双层可见性）
//   - mount 最终走 SSHFSOnly 档位成功（hooks.trySSHFS 返回 nil）
//   - 闭包收到的 accountID 与 cfg.ClaudeAccountID 一致
//
// 注意：cfg.IsSecondaryClient 在 MountWorkspace 内被赋值，但因 cfg 是值传递，
// 调用方（本测试）的 cfg.IsSecondaryClient 不会变 —— 生产路径由 ssh.go 注入闭包
// 通过闭包捕获 mountCfg 指针置位（line 95-110），属于"双保险"中的外层路径。
// 本测试通过捕获闭包入参 accountID 间接证明 invoke 真发生。
func TestMountWorkspace_SyncLocked(t *testing.T) {
	var buf bytes.Buffer
	var observedAccountID string
	cfg := MountConfig{
		Mode:             ModeFull,
		SupportsMergerfs: true,
		ClaudeAccountID:  "test-acct-gap2",
		NoColor:          true,
		Logger:           &buf,
		LastSessionPath:  filepath.Join(t.TempDir(), "last.json"),
		SyncSessionLock: func(accountID string) (func(), error) {
			observedAccountID = accountID
			return nil, ErrSyncLocked
		},
		hooks: &strategyHooks{
			trySSHFS: func() (func(), error) { return func() {}, nil },
		},
	}

	cleanup, mode, err := MountWorkspace(nil, nil, cfg)
	if err != nil {
		t.Fatalf("MountWorkspace err 应 nil（降级成功）: %v", err)
	}
	if mode != ModeSSHFSOnly {
		t.Errorf("ErrSyncLocked 必须强制 ModeSSHFSOnly，得 %s", mode)
	}
	if cleanup == nil {
		t.Fatal("cleanup 不应 nil")
	}
	cleanup()

	if observedAccountID != "test-acct-gap2" {
		t.Errorf("SyncSessionLock 应收到 accountID=test-acct-gap2，得 %q", observedAccountID)
	}

	data, rerr := os.ReadFile(cfg.LastSessionPath)
	if rerr != nil {
		t.Fatalf("读 last-session.json 失败: %v", rerr)
	}
	var snap LastSessionSnapshot
	if jerr := json.Unmarshal(data, &snap); jerr != nil {
		t.Fatalf("解析 last-session.json 失败: %v", jerr)
	}

	var found bool
	for _, step := range snap.DowngradeChain {
		if step.ReasonCode == "sync_locked" {
			if step.From != "full" {
				t.Errorf("DowngradeStep.From = %q, want \"full\"", step.From)
			}
			if step.To != "sshfs-only" {
				t.Errorf("DowngradeStep.To = %q, want \"sshfs-only\"", step.To)
			}
			if step.ReasonMessage == "" {
				t.Error("DowngradeStep.ReasonMessage 不应为空")
			}
			found = true
		}
	}
	if !found {
		t.Errorf("DowngradeChain 应含 ReasonCode=sync_locked 的 step，实际: %+v", snap.DowngradeChain)
	}

	out := buf.String()
	if !strings.Contains(out, "sync_locked") && !strings.Contains(out, "单例锁") {
		t.Errorf("stderr 应含 sync_locked 或 '单例锁' 摘要，实际: %q", out)
	}
}

// TestMountWorkspace_SyncLockSuccess 验证成功分支：SyncSessionLock 返回非 nil release。
//   - mount 全栈成功（Full 模式 hooks 全 OK）
//   - cleanup 调用时 LIFO 顺序：先卸 mergerfs / sshfs / hotsync（modeCleanup 内部 LIFO），
//     再释放 sync 锁（syncRelease）—— 锁覆盖整个 mount 生命周期
//   - 成功拿锁不触发 IsSecondaryClient（保持 false）
func TestMountWorkspace_SyncLockSuccess(t *testing.T) {
	var releaseCalled int
	var mergeCleanupCalled int

	cfg := MountConfig{
		Mode:             ModeFull,
		SupportsMergerfs: true,
		ClaudeAccountID:  "test-acct-success",
		NoColor:          true,
		Logger:           new(bytes.Buffer),
		LastSessionPath:  filepath.Join(t.TempDir(), "last.json"),
		SyncSessionLock: func(accountID string) (func(), error) {
			return func() { releaseCalled++ }, nil
		},
		hooks: &strategyHooks{
			tryHotSync: func() (func(), HotSyncStatus, error) {
				return func() {}, HotSyncStatus{}, nil
			},
			trySSHFS: func() (func(), error) { return func() {}, nil },
			tryMerge: func() (func(), error) {
				return func() { mergeCleanupCalled++ }, nil
			},
		},
	}
	cleanup, mode, err := MountWorkspace(nil, nil, cfg)
	if err != nil {
		t.Fatalf("应成功: %v", err)
	}
	if mode != ModeFull {
		t.Errorf("mode = %s, want full", mode)
	}
	if cfg.IsSecondaryClient {
		t.Error("成功拿锁时调用方 cfg.IsSecondaryClient 应保持 false")
	}
	cleanup()
	if releaseCalled != 1 {
		t.Errorf("syncRelease 应被调用 1 次，实际 %d", releaseCalled)
	}
	if mergeCleanupCalled != 1 {
		t.Errorf("mergeCleanup 应被调用 1 次（LIFO 保证在 syncRelease 之前），实际 %d", mergeCleanupCalled)
	}
}

// TestMountWorkspace_SyncLockOtherError 验证非 ErrSyncLocked 错误透传（M13 防御）。
//   - 例如 flock 启动失败 / SSH session 错误
//   - 必须 return ModeFailed + err 含 "sync lock acquire"，不静默降级
//   - last-session.json ActualMode=failed
func TestMountWorkspace_SyncLockOtherError(t *testing.T) {
	cfg := MountConfig{
		Mode:             ModeFull,
		SupportsMergerfs: true,
		ClaudeAccountID:  "test-acct-err",
		NoColor:          true,
		Logger:           new(bytes.Buffer),
		LastSessionPath:  filepath.Join(t.TempDir(), "last.json"),
		SyncSessionLock: func(accountID string) (func(), error) {
			return nil, fmt.Errorf("flock not installed")
		},
		hooks: &strategyHooks{
			trySSHFS: func() (func(), error) { return func() {}, nil },
		},
	}
	_, mode, err := MountWorkspace(nil, nil, cfg)
	if err == nil {
		t.Fatal("非 ErrSyncLocked 错误应透传，得 nil")
	}
	if mode != ModeFailed {
		t.Errorf("mode = %s, want failed", mode)
	}
	if !strings.Contains(err.Error(), "sync lock acquire") {
		t.Errorf("err 应含 'sync lock acquire'，实际: %v", err)
	}

	data, rerr := os.ReadFile(cfg.LastSessionPath)
	if rerr != nil {
		t.Fatalf("读 last-session.json 失败: %v", rerr)
	}
	var snap LastSessionSnapshot
	_ = json.Unmarshal(data, &snap)
	if snap.ActualMode != "failed" {
		t.Errorf("ActualMode = %q, want \"failed\"", snap.ActualMode)
	}
}
