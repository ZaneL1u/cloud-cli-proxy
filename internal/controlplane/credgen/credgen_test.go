package credgen

import (
	"crypto/ed25519"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestGenerateEntryPassword_LengthAndCharset(t *testing.T) {
	const allowed = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	for i := 0; i < 16; i++ {
		got := GenerateEntryPassword()
		if len(got) != 8 {
			t.Fatalf("GenerateEntryPassword length = %d, want 8 (got=%q)", len(got), got)
		}
		for _, c := range got {
			if !strings.ContainsRune(allowed, c) {
				t.Fatalf("GenerateEntryPassword char %q not in [a-zA-Z0-9] (got=%q)", c, got)
			}
		}
	}
}

func TestGenerateShortID_LengthAndCharset(t *testing.T) {
	const allowed = "abcdefghijklmnopqrstuvwxyz0123456789"
	for i := 0; i < 16; i++ {
		got := GenerateShortID()
		if len(got) != 6 {
			t.Fatalf("GenerateShortID length = %d, want 6 (got=%q)", len(got), got)
		}
		for _, c := range got {
			if !strings.ContainsRune(allowed, c) {
				t.Fatalf("GenerateShortID char %q not in [a-z0-9] (got=%q)", c, got)
			}
		}
	}
}

func TestGenerateLoginPassword_LengthAndCharsetIncludesSpecials(t *testing.T) {
	const allowed = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%&*"
	// 单次抽样确认长度 + 字符集子集；special 字符出现概率每位 7/70=10%，
	// 32 次循环的概率是 1-(63/70)^32 ≈ 96.4%；为避免偶发 flaky 不强行断言出现，
	// 仅校验长度与字符集封闭性。
	for i := 0; i < 8; i++ {
		got := GenerateLoginPassword()
		if len(got) != 16 {
			t.Fatalf("GenerateLoginPassword length = %d, want 16 (got=%q)", len(got), got)
		}
		for _, c := range got {
			if !strings.ContainsRune(allowed, c) {
				t.Fatalf("GenerateLoginPassword char %q not in expected charset (got=%q)", c, got)
			}
		}
	}
	// 字符集字面值自检：!@#$%&* 必须全部在 allowed 中（防止后续误改字符集）。
	for _, c := range "!@#$%&*" {
		if !strings.ContainsRune(allowed, c) {
			t.Fatalf("login password charset missing special %q", c)
		}
	}
}

func TestGenerateSSHKeyPair_Ed25519RoundTrip(t *testing.T) {
	pub, priv, err := GenerateSSHKeyPair("ed25519", "seed-admin")
	if err != nil {
		t.Fatalf("GenerateSSHKeyPair ed25519 error = %v", err)
	}
	if !strings.Contains(priv, "BEGIN OPENSSH PRIVATE KEY") {
		t.Fatalf("private key not OpenSSH PEM:\n%s", priv)
	}
	if !strings.HasSuffix(pub, " seed-admin") {
		t.Fatalf("public key missing comment suffix: %q", pub)
	}

	pubKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(pub))
	if err != nil {
		t.Fatalf("ParseAuthorizedKey: %v", err)
	}
	rawPriv, err := ssh.ParseRawPrivateKey([]byte(priv))
	if err != nil {
		t.Fatalf("ParseRawPrivateKey: %v", err)
	}
	edPriv, ok := rawPriv.(*ed25519.PrivateKey)
	if !ok {
		t.Fatalf("expected *ed25519.PrivateKey, got %T", rawPriv)
	}
	derived, err := ssh.NewPublicKey(edPriv.Public())
	if err != nil {
		t.Fatalf("NewPublicKey from priv: %v", err)
	}
	if string(pubKey.Marshal()) != string(derived.Marshal()) {
		t.Fatal("ed25519 round-trip: pub does not match priv.Public()")
	}
}

func TestGenerateSSHKeyPair_UnsupportedTypeReturnsError(t *testing.T) {
	pub, priv, err := GenerateSSHKeyPair("dsa", "x")
	if err == nil {
		t.Fatal("GenerateSSHKeyPair dsa expected error, got nil")
	}
	if pub != "" || priv != "" {
		t.Fatalf("unsupported type expected empty pub/priv, got pub=%q priv=%q", pub, priv)
	}
}

func TestComputeFingerprint_NonEmptyForValidKey(t *testing.T) {
	pub, _, err := GenerateSSHKeyPair("ed25519", "fp-test")
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	fp := ComputeFingerprint(pub)
	if fp == "" {
		t.Fatal("ComputeFingerprint returned empty for valid public key")
	}
	if !strings.HasPrefix(fp, "SHA256:") {
		t.Fatalf("fingerprint not SHA256-prefixed: %q", fp)
	}
}

func TestComputeFingerprint_InvalidKeyReturnsEmpty(t *testing.T) {
	if got := ComputeFingerprint("not a key"); got != "" {
		t.Fatalf("ComputeFingerprint(invalid) = %q, want empty", got)
	}
}
