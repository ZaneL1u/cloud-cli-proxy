package cloudclaude

import (
	"strings"
	"testing"
)

func TestSSHDoctor(t *testing.T) {
	t.Run("detect_private_key", func(t *testing.T) {
		cases := []struct {
			line string
			want bool
		}{
			{"-----BEGIN OPENSSH PRIVATE KEY-----", true},
			{"-----BEGIN EC PRIVATE KEY-----", true},
			{"-----BEGIN RSA PRIVATE KEY-----", true},
			{"  -----BEGIN OPENSSH PRIVATE KEY-----  ", true},
			{"ssh-ed25519 AAAA...", false},
			{"# comment", false},
			{"", false},
			{"-----BEGIN CERTIFICATE-----", false},
		}
		for _, c := range cases {
			if got := detectPrivateKey(c.line); got != c.want {
				t.Errorf("detectPrivateKey(%q) = %v, want %v", c.line, got, c.want)
			}
		}
	})

	t.Run("expected_mode", func(t *testing.T) {
		cases := map[string]string{
			"private":          "0600",
			"authorized_keys":  "0600",
			"known_hosts":      "0600",
			"config":           "0600",
			"public":           "0644",
			"other":            "",
			"":                 "",
		}
		for kind, want := range cases {
			if got := expectedMode(kind); got != want {
				t.Errorf("expectedMode(%q) = %q, want %q", kind, got, want)
			}
		}
	})

	t.Run("pem_ends_with_newline", func(t *testing.T) {
		if !pemEndsWithNewline('\n') {
			t.Error("expected '\\n' to be treated as EOL")
		}
		if pemEndsWithNewline('-') {
			t.Error("expected '-' to NOT be treated as EOL")
		}
		if pemEndsWithNewline(0) {
			t.Error("expected NUL to NOT be treated as EOL")
		}
	})

	t.Run("detect_kind_routing", func(t *testing.T) {
		cases := []struct {
			name, first, want string
		}{
			{"id_ed25519", "-----BEGIN OPENSSH PRIVATE KEY-----", "private"},
			{"vision", "-----BEGIN OPENSSH PRIVATE KEY-----", "private"},
			{"id_rsa.pub", "ssh-rsa AAAA", "public"},
			{"authorized_keys", "# managed", "authorized_keys"},
			{"known_hosts", "github.com ssh-rsa", "known_hosts"},
			{"known_hosts.old", "", "known_hosts"},
			{"config", "Host *", "config"},
			{"random.txt", "hello", "other"},
		}
		for _, c := range cases {
			if got := detectKind(c.name, c.first); got != c.want {
				t.Errorf("detectKind(%q, %q) = %q, want %q", c.name, c.first, got, c.want)
			}
		}
	})

	t.Run("parse_scan_output", func(t *testing.T) {
		raw := strings.Join([]string{
			"USER=workspace",
			"SUDO_OK=y",
			"FILE|id_ed25519|workspace:workspace|600|-----BEGIN OPENSSH PRIVATE KEY-----|0a",
			"FILE|id_ed25519.pub|workspace:workspace|644|ssh-ed25519 AAAA|0a",
			"FILE|vision|root:root|600|-----BEGIN OPENSSH PRIVATE KEY-----|3d",
			"FILE|authorized_keys|workspace:workspace|644|# >>> managed >>>|0a",
			"FILE|config|workspace:workspace|644|Host gitlab.zaneliu.me|0a",
			"",
		}, "\n")

		r := parseScanOutput(raw)

		if r.User != "workspace" {
			t.Fatalf("User = %q, want workspace", r.User)
		}
		if !r.SudoOK {
			t.Errorf("SudoOK = false, want true")
		}
		if r.Missing {
			t.Errorf("Missing = true, want false")
		}
		if len(r.Files) != 5 {
			t.Fatalf("Files len = %d, want 5 (got paths=%v)", len(r.Files), filePaths(r.Files))
		}

		byName := map[string]FileReport{}
		for _, f := range r.Files {
			byName[basename(f.Path)] = f
		}

		// id_ed25519: 托管私钥，应全绿
		id := byName["id_ed25519"]
		if id.Kind != "private" {
			t.Errorf("id_ed25519 kind = %q, want private", id.Kind)
		}
		if !id.OwnerOK {
			t.Errorf("id_ed25519 OwnerOK = false, want true (owner=%s, user=%s)", id.Owner, r.User)
		}
		if !id.ModeOK {
			t.Errorf("id_ed25519 ModeOK = false (mode=%s), want true", id.Mode)
		}
		if id.PEMEndsNL == nil || !*id.PEMEndsNL {
			t.Errorf("id_ed25519 PEMEndsNL = %v, want *true", id.PEMEndsNL)
		}

		// id_ed25519.pub: 公钥期望 0644
		pub := byName["id_ed25519.pub"]
		if pub.Kind != "public" {
			t.Errorf("pub kind = %q, want public", pub.Kind)
		}
		if !pub.ModeOK {
			t.Errorf("pub ModeOK = false (mode=%s), want true", pub.Mode)
		}
		if pub.PEMEndsNL != nil {
			t.Errorf("pub PEMEndsNL should be nil, got %v", pub.PEMEndsNL)
		}

		// vision: 用户自命名的私钥，owner 错（root:root），PEM 尾非 \n
		vision := byName["vision"]
		if vision.Kind != "private" {
			t.Errorf("vision kind = %q, want private", vision.Kind)
		}
		if vision.OwnerOK {
			t.Errorf("vision OwnerOK = true, want false (root != workspace)")
		}
		if !vision.ModeOK {
			t.Errorf("vision ModeOK = false (mode=%s), want true (0600 满足 private)", vision.Mode)
		}
		if vision.PEMEndsNL == nil || *vision.PEMEndsNL {
			t.Errorf("vision PEMEndsNL = %v, want *false", vision.PEMEndsNL)
		}

		// authorized_keys: 期望 0600, 实际 0644 → ModeOK false
		ak := byName["authorized_keys"]
		if ak.Kind != "authorized_keys" {
			t.Errorf("authorized_keys kind = %q", ak.Kind)
		}
		if ak.ModeOK {
			t.Errorf("authorized_keys ModeOK = true, want false (期望 0600, 实际 0644)")
		}

		// config: 期望 0600, 实际 0644 → ModeOK false
		cfg := byName["config"]
		if cfg.Kind != "config" {
			t.Errorf("config kind = %q", cfg.Kind)
		}
		if cfg.ModeOK {
			t.Errorf("config ModeOK = true, want false")
		}
	})

	t.Run("parse_handles_missing_dir", func(t *testing.T) {
		raw := "USER=workspace\nSUDO_OK=n\nSSHDIR_MISSING=/workspace/.ssh\n"
		r := parseScanOutput(raw)
		if !r.Missing {
			t.Errorf("Missing = false, want true")
		}
		if r.SudoOK {
			t.Errorf("SudoOK = true, want false")
		}
		if len(r.Files) != 0 {
			t.Errorf("Files len = %d, want 0", len(r.Files))
		}
	})

	t.Run("normalize_mode", func(t *testing.T) {
		cases := map[string]string{
			"600":  "0600",
			"0600": "0600",
			"644":  "0644",
			"":     "",
			"?":    "?",
		}
		for in, want := range cases {
			if got := normalizeMode(in); got != want {
				t.Errorf("normalizeMode(%q) = %q, want %q", in, got, want)
			}
		}
	})
}

func filePaths(files []FileReport) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.Path)
	}
	return out
}

func basename(p string) string {
	if idx := strings.LastIndexByte(p, '/'); idx >= 0 {
		return p[idx+1:]
	}
	return p
}
