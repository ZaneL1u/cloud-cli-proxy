package http

import (
	"strings"
	"testing"

	"github.com/zanel1u/cloud-cli-proxy/internal/controlplane/credgen"
)

func TestGenerateEd25519KeyPairProducesOpenSSHPrivateKey(t *testing.T) {
	publicKey, privateKey, err := credgen.GenerateSSHKeyPair("ed25519", "test@example")
	if err != nil {
		t.Fatalf("credgen.GenerateSSHKeyPair() error = %v", err)
	}

	if !strings.Contains(privateKey, "BEGIN OPENSSH PRIVATE KEY") {
		t.Fatalf("private key is not in OpenSSH format:\n%s", privateKey)
	}

	if err := validateSSHKeyPair(publicKey, privateKey); err != nil {
		t.Fatalf("validateSSHKeyPair() error = %v", err)
	}
}

func TestValidateSSHKeyPairRejectsMismatch(t *testing.T) {
	publicKey, _, err := credgen.GenerateSSHKeyPair("ed25519", "pub-only")
	if err != nil {
		t.Fatalf("generate first ed25519 key pair: %v", err)
	}

	_, otherPrivateKey, err := credgen.GenerateSSHKeyPair("ed25519", "priv-only")
	if err != nil {
		t.Fatalf("generate second ed25519 key pair: %v", err)
	}

	if err := validateSSHKeyPair(publicKey, otherPrivateKey); err == nil {
		t.Fatal("validateSSHKeyPair() expected mismatch error, got nil")
	}
}
