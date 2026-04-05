package http

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	nethttp "net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/ssh"

	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

type SSHKeyStore interface {
	ListSSHKeysByUser(ctx context.Context, userID string) ([]repository.SSHKey, error)
	CreateSSHKey(ctx context.Context, userID, purpose, label, publicKey, privateKey, keyType, fingerprint string) (repository.SSHKey, error)
	DeleteSSHKey(ctx context.Context, keyID, userID string) error
}

type SSHKeyHandler struct {
	logger *slog.Logger
	store  SSHKeyStore
}

func NewSSHKeyHandler(logger *slog.Logger, store SSHKeyStore) *SSHKeyHandler {
	return &SSHKeyHandler{logger: logger, store: store}
}

func (h *SSHKeyHandler) List() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		userID := r.PathValue("userID")
		if userID == "" {
			userID = UserIDFromContext(r.Context())
		}
		if userID == "" {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "缺少 user_id"})
			return
		}

		keys, err := h.store.ListSSHKeysByUser(r.Context(), userID)
		if err != nil {
			h.logger.Error("list ssh keys failed", "user_id", userID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "查询密钥失败"})
			return
		}

		sanitized := make([]repository.SSHKey, 0, len(keys))
		for _, k := range keys {
			if k.Purpose == "outbound" {
				k.PrivateKey = ""
			}
			sanitized = append(sanitized, k)
		}

		writeJSON(w, nethttp.StatusOK, map[string]any{
			"keys": sanitized,
		})
	})
}

func (h *SSHKeyHandler) Create() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		userID := r.PathValue("userID")
		if userID == "" {
			userID = UserIDFromContext(r.Context())
		}
		if userID == "" {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "缺少 user_id"})
			return
		}

		var req struct {
			Purpose    string `json:"purpose"`
			Label      string `json:"label"`
			KeyType    string `json:"key_type"`
			PublicKey  string `json:"public_key"`
			PrivateKey string `json:"private_key"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "请求体格式错误"})
			return
		}

		req.Purpose = strings.TrimSpace(req.Purpose)
		req.PublicKey = strings.TrimSpace(req.PublicKey)
		req.PrivateKey = strings.TrimSpace(req.PrivateKey)

		if req.Purpose != "inbound" && req.Purpose != "outbound" {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "purpose 必须为 inbound 或 outbound"})
			return
		}
		if req.KeyType == "" {
			req.KeyType = "ed25519"
		}
		if req.KeyType != "ed25519" && req.KeyType != "rsa" {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "key_type 必须为 ed25519 或 rsa"})
			return
		}

		if req.Purpose == "inbound" {
			if req.PublicKey == "" {
				writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "入站密钥必须提供 public_key"})
				return
			}
			req.PrivateKey = ""
		}

		if req.Purpose == "outbound" && req.PublicKey == "" {
			pubKeyStr, privKeyStr, err := generateSSHKeyPair(req.KeyType, req.Label)
			if err != nil {
				h.logger.Error("generate ssh key pair failed", "user_id", userID, "error", err)
				writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "生成密钥对失败"})
				return
			}
			req.PublicKey = pubKeyStr
			req.PrivateKey = privKeyStr
		}

		fingerprint := computeFingerprint(req.PublicKey)

		key, err := h.store.CreateSSHKey(r.Context(), userID, req.Purpose, req.Label, req.PublicKey, req.PrivateKey, req.KeyType, fingerprint)
		if err != nil {
			h.logger.Error("create ssh key failed", "user_id", userID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "创建密钥失败"})
			return
		}

		respKey := key
		if req.Purpose == "inbound" {
			respKey.PrivateKey = ""
		}

		writeJSON(w, nethttp.StatusCreated, map[string]any{
			"key": respKey,
		})
	})
}

func (h *SSHKeyHandler) Delete() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		userID := r.PathValue("userID")
		if userID == "" {
			userID = UserIDFromContext(r.Context())
		}
		if userID == "" {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "缺少 user_id"})
			return
		}

		keyID := r.PathValue("keyID")
		if keyID == "" {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "缺少 keyID"})
			return
		}

		if err := h.store.DeleteSSHKey(r.Context(), keyID, userID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeJSON(w, nethttp.StatusNotFound, map[string]string{"error": "密钥不存在"})
				return
			}
			h.logger.Error("delete ssh key failed", "user_id", userID, "key_id", keyID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "删除密钥失败"})
			return
		}

		w.WriteHeader(nethttp.StatusNoContent)
	})
}

func computeFingerprint(pubKeyStr string) string {
	pubKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(pubKeyStr))
	if err != nil {
		return ""
	}
	return ssh.FingerprintSHA256(pubKey)
}

func generateSSHKeyPair(keyType, comment string) (publicKey, privateKey string, err error) {
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

	privKeyBytes, err := x509.MarshalPKCS8PrivateKey(privKey)
	if err != nil {
		return "", "", fmt.Errorf("marshal ed25519 private key: %w", err)
	}
	privKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privKeyBytes,
	})

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
