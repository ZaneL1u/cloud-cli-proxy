package cloudclaude

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/errcodes"
)

// fakeMutagenErr / fakeMergeErr 模拟生产路径的 codedError sentinel。
type fakeMutagenErr struct{ code errcodes.Code }

func (e *fakeMutagenErr) Error() string       { return string(e.code) }
func (e *fakeMutagenErr) Code() errcodes.Code { return e.code }
func (e *fakeMutagenErr) Reason() string      { return "fake mutagen failure" }

type fakeMergeErr struct{ code errcodes.Code }

func (e *fakeMergeErr) Error() string       { return string(e.code) }
func (e *fakeMergeErr) Code() errcodes.Code { return e.code }
func (e *fakeMergeErr) Reason() string      { return "fake merge failure" }

// stratCase 定义单个 mode×failure 组合。
type stratCase struct {
	name        string
	intended    Mode
	mutagenOK   bool
	sshfsOK     bool
	mergeOK     bool
	wantMode    Mode
	wantErr     bool
	wantBanners []string
}

func newHooks(c stratCase) *strategyHooks {
	h := &strategyHooks{}
	h.tryMutagen = func() (func(), MutagenSyncStatus, error) {
		if !c.mutagenOK {
			return nil, MutagenSyncStatus{}, &fakeMutagenErr{code: errcodes.MOUNT_MUTAGEN_DAEMON_UNAVAILABLE}
		}
		return func() {}, MutagenSyncStatus{}, nil
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
			name:     "Auto/Mutagen-fail/SSHFS-ok/Merge-ok→MutagenOnly?no→ActuallySSHFSOnly",
			intended: ModeAuto, mutagenOK: false, sshfsOK: true, mergeOK: true,
			wantMode:    ModeSSHFSOnly,
			wantBanners: []string{"MOUNT_AUTO_DOWNGRADED"},
		},
		{
			name:     "Auto/Mutagen-ok/SSHFS-fail/Merge-ok→MutagenOnly",
			intended: ModeAuto, mutagenOK: true, sshfsOK: false, mergeOK: true,
			wantMode:    ModeMutagenOnly,
			wantBanners: []string{"MOUNT_AUTO_DOWNGRADED"},
		},
		{
			name:     "Auto/Mutagen-ok/SSHFS-ok/Merge-fail→MutagenOnly",
			intended: ModeAuto, mutagenOK: true, sshfsOK: true, mergeOK: false,
			wantMode:    ModeMutagenOnly,
			wantBanners: []string{"MOUNT_AUTO_DOWNGRADED"},
		},
		{
			name:     "Auto/all-ok→Full",
			intended: ModeAuto, mutagenOK: true, sshfsOK: true, mergeOK: true,
			wantMode: ModeFull,
		},
		{
			name:     "Auto/all-fail→Failed",
			intended: ModeAuto, mutagenOK: false, sshfsOK: false, mergeOK: false,
			wantMode: ModeFailed, wantErr: true,
		},
		{
			name:     "Full/Mutagen-fail→Failed",
			intended: ModeFull, mutagenOK: false, sshfsOK: true, mergeOK: true,
			wantMode: ModeFailed, wantErr: true,
		},
		{
			name:     "Full/SSHFS-fail→Failed",
			intended: ModeFull, mutagenOK: true, sshfsOK: false, mergeOK: true,
			wantMode: ModeFailed, wantErr: true,
		},
		{
			name:     "Full/Merge-fail→Failed",
			intended: ModeFull, mutagenOK: true, sshfsOK: true, mergeOK: false,
			wantMode: ModeFailed, wantErr: true,
		},
		{
			name:     "MutagenOnly/ok→MutagenOnly",
			intended: ModeMutagenOnly, mutagenOK: true,
			wantMode: ModeMutagenOnly,
		},
		{
			name:     "MutagenOnly/fail→Failed",
			intended: ModeMutagenOnly, mutagenOK: false,
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
				SupportsMutagen:  true,
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
		printBanner(&buf, ModeMutagenOnly, true)
		if !strings.Contains(buf.String(), "✓ 文件映射就绪 [mutagen-only]") {
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
		SupportsMutagen:         true,
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
		SupportsMutagen:  true,
		SupportsMergerfs: true,
		NoColor:          true,
		Logger:           &buf,
		LastSessionPath:  filepath.Join(t.TempDir(), "last.json"),
		hooks: &strategyHooks{
			tryMutagen: func() (func(), MutagenSyncStatus, error) {
				return nil, MutagenSyncStatus{}, &fakeMutagenErr{code: errcodes.MOUNT_MUTAGEN_DAEMON_UNAVAILABLE}
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
	t.Run("SupportsMutagen=false forces SSHFSOnly", func(t *testing.T) {
		var buf bytes.Buffer
		cfg := MountConfig{
			Mode:             ModeAuto,
			SupportsMutagen:  false,
			SupportsMergerfs: true,
			NoColor:          true,
			Logger:           &buf,
			LastSessionPath:  filepath.Join(t.TempDir(), "last.json"),
			hooks: &strategyHooks{
				trySSHFS: func() (func(), error) { return func() {}, nil },
			},
		}
		cleanup, mode, err := MountWorkspace(nil, nil, cfg)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		cleanup()
		if mode != ModeSSHFSOnly {
			t.Errorf("mode=%s, want sshfs-only", mode)
		}
		if !strings.Contains(buf.String(), "MOUNT_AUTO_DOWNGRADED") {
			t.Error("missing capability downgrade banner")
		}
	})

	t.Run("SupportsMergerfs=false drops Auto to MutagenOnly", func(t *testing.T) {
		var buf bytes.Buffer
		cfg := MountConfig{
			Mode:             ModeAuto,
			SupportsMutagen:  true,
			SupportsMergerfs: false,
			NoColor:          true,
			Logger:           &buf,
			LastSessionPath:  filepath.Join(t.TempDir(), "last.json"),
			hooks: &strategyHooks{
				tryMutagen: func() (func(), MutagenSyncStatus, error) {
					return func() {}, MutagenSyncStatus{}, nil
				},
			},
		}
		cleanup, mode, err := MountWorkspace(nil, nil, cfg)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		cleanup()
		if mode != ModeMutagenOnly {
			t.Errorf("mode=%s, want mutagen-only", mode)
		}
	})
}

func Test_ParseMode(t *testing.T) {
	cases := map[string]Mode{
		"":             ModeAuto,
		"auto":         ModeAuto,
		"full":         ModeFull,
		"mutagen-only": ModeMutagenOnly,
		"sshfs-only":   ModeSSHFSOnly,
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
		SupportsMutagen:  true,
		SupportsMergerfs: true,
		NoColor:          true,
		Logger:           &buf,
		LastSessionPath:  filepath.Join(t.TempDir(), "last.json"),
		hooks: &strategyHooks{
			tryMutagen: func() (func(), MutagenSyncStatus, error) {
				return nil, MutagenSyncStatus{}, &fakeMutagenErr{code: errcodes.MOUNT_MUTAGEN_VERSION_SKEW}
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
