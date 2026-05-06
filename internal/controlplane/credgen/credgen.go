// Package credgen 提供控制面与种子流程共用的凭据生成原语：
//   - 随机短 ID / 入口密码 / 登录密码（基于 crypto/rand）
//   - ed25519 / RSA SSH 密钥对生成（OpenSSH 兼容格式）
//   - SSH 公钥指纹计算（SHA256:Base64Std）
//
// 该包被设计为零依赖外部状态、无副作用：上层（http handler / app.ensureSeedAdmin）
// 直接调用即可，避免 app -> http 的反向 import。
package credgen

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"math/big"
	"strings"

	"golang.org/x/crypto/ssh"
)

// GenerateRandomString 从指定 charset 中按 crypto/rand 选择 length 个字符返回字符串。
// 当 length<=0 或 charset 为空时返回 ""，调用方需保证语义正确。
func GenerateRandomString(length int, charset string) string {
	if length <= 0 || charset == "" {
		return ""
	}
	b := make([]byte, length)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		b[i] = charset[n.Int64()]
	}
	return string(b)
}

// GenerateShortID 生成 6 位 [a-z0-9] 短 ID，用于 user.short_id / host.short_id。
func GenerateShortID() string {
	return GenerateRandomString(6, "abcdefghijklmnopqrstuvwxyz0123456789")
}

// GenerateEntryPassword 生成 8 位 [a-zA-Z0-9] 入口密码，写入 users.entry_password。
// 容器内 SSH 登录使用此密码（chpasswd），须与 worker.buildCreateArgs 的非空守卫对齐。
func GenerateEntryPassword() string {
	return GenerateRandomString(8, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
}

// GenerateLoginPassword 生成 16 位带特殊字符的登录密码，用于网页登录入口。
func GenerateLoginPassword() string {
	return GenerateRandomString(16, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%&*")
}

// GenerateSSHKeyPair 按 keyType 生成 SSH 密钥对，返回 OpenSSH authorized_keys 格式公钥
// 与 PEM 编码私钥；当 keyType 不在 {"ed25519","rsa"} 时返回 error。
// 公钥末尾会以空格附加 comment（若非空），与 ssh-keygen -C 行为一致。
func GenerateSSHKeyPair(keyType, comment string) (publicKey, privateKey string, err error) {
	switch keyType {
	case "ed25519":
		return generateEd25519KeyPair(comment)
	case "rsa":
		return generateRSAKeyPair(comment)
	default:
		return "", "", fmt.Errorf("unsupported key type: %s", keyType)
	}
}

func generateEd25519KeyPair(comment string) (string, string, error) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("generate ed25519 key: %w", err)
	}

	sshPubKey, err := ssh.NewPublicKey(pubKey)
	if err != nil {
		return "", "", fmt.Errorf("convert ed25519 public key: %w", err)
	}
	pubKeyStr := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPubKey)))
	if comment != "" {
		pubKeyStr += " " + comment
	}

	privKeyBlock, err := ssh.MarshalPrivateKey(privKey, comment)
	if err != nil {
		return "", "", fmt.Errorf("marshal ed25519 private key: %w", err)
	}
	privKeyPEM := pem.EncodeToMemory(privKeyBlock)

	return pubKeyStr, string(privKeyPEM), nil
}

func generateRSAKeyPair(comment string) (string, string, error) {
	privKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return "", "", fmt.Errorf("generate rsa key: %w", err)
	}

	sshPubKey, err := ssh.NewPublicKey(&privKey.PublicKey)
	if err != nil {
		return "", "", fmt.Errorf("convert rsa public key: %w", err)
	}
	pubKeyStr := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPubKey)))
	if comment != "" {
		pubKeyStr += " " + comment
	}

	privKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privKey),
	})

	return pubKeyStr, string(privKeyPEM), nil
}

// ComputeFingerprint 解析 OpenSSH authorized_keys 格式公钥并返回 "SHA256:..." 指纹。
// 解析失败返回空字符串（调用方据此决定 400/500）。
func ComputeFingerprint(pubKeyStr string) string {
	pubKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(pubKeyStr))
	if err != nil {
		return ""
	}
	return ssh.FingerprintSHA256(pubKey)
}
