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
)

type SSHKeyStore interface {
	GetUserSSHKeys(ctx context.Context, userID string) (publicKey, privateKey, keyType string, err error)
	UpdateUserSSHKeys(ctx context.Context, userID, publicKey, privateKey, keyType string) error
}

type SSHKeyHandler struct {
	logger *slog.Logger
	store  SSHKeyStore
}

func NewSSHKeyHandler(logger *slog.Logger, store SSHKeyStore) *SSHKeyHandler {
	return &SSHKeyHandler{logger: logger, store: store}
}

func (h *SSHKeyHandler) Generate() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		userID := r.PathValue("userID")
		if userID == "" {
			userID = UserIDFromContext(r.Context())
		}
		if userID == "" {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "user_id is required"})
			return
		}

		var req struct {
			KeyType string `json:"key_type"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			req.KeyType = "ed25519"
		}
		if req.KeyType == "" {
			req.KeyType = "ed25519"
		}
		if req.KeyType != "ed25519" && req.KeyType != "rsa" {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "key_type must be ed25519 or rsa"})
			return
		}

		pubKeyStr, privKeyStr, err := generateSSHKeyPair(req.KeyType, userID)
		if err != nil {
			h.logger.Error("generate ssh key pair failed", "user_id", userID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "generate key pair failed"})
			return
		}

		if err := h.store.UpdateUserSSHKeys(r.Context(), userID, pubKeyStr, privKeyStr, req.KeyType); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeJSON(w, nethttp.StatusNotFound, map[string]string{"error": "user not found"})
				return
			}
			h.logger.Error("save ssh keys failed", "user_id", userID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "save keys failed"})
			return
		}

		writeJSON(w, nethttp.StatusOK, map[string]any{
			"public_key":  pubKeyStr,
			"private_key": privKeyStr,
			"key_type":    req.KeyType,
		})
	})
}

func (h *SSHKeyHandler) Set() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		userID := r.PathValue("userID")
		if userID == "" {
			userID = UserIDFromContext(r.Context())
		}
		if userID == "" {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "user_id is required"})
			return
		}

		var req struct {
			PublicKey  string `json:"public_key"`
			PrivateKey string `json:"private_key"`
			KeyType    string `json:"key_type"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}

		req.PublicKey = strings.TrimSpace(req.PublicKey)
		req.PrivateKey = strings.TrimSpace(req.PrivateKey)
		if req.PublicKey == "" {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "public_key is required"})
			return
		}

		if req.KeyType == "" {
			if strings.Contains(req.PublicKey, "ssh-ed25519") {
				req.KeyType = "ed25519"
			} else if strings.Contains(req.PublicKey, "ssh-rsa") {
				req.KeyType = "rsa"
			} else {
				req.KeyType = "custom"
			}
		}

		if err := h.store.UpdateUserSSHKeys(r.Context(), userID, req.PublicKey, req.PrivateKey, req.KeyType); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeJSON(w, nethttp.StatusNotFound, map[string]string{"error": "user not found"})
				return
			}
			h.logger.Error("set ssh keys failed", "user_id", userID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "save keys failed"})
			return
		}

		writeJSON(w, nethttp.StatusOK, map[string]any{
			"public_key": req.PublicKey,
			"key_type":   req.KeyType,
			"has_private": req.PrivateKey != "",
		})
	})
}

func (h *SSHKeyHandler) Get() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		userID := r.PathValue("userID")
		if userID == "" {
			userID = UserIDFromContext(r.Context())
		}
		if userID == "" {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "user_id is required"})
			return
		}

		pubKey, privKey, keyType, err := h.store.GetUserSSHKeys(r.Context(), userID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeJSON(w, nethttp.StatusNotFound, map[string]string{"error": "user not found"})
				return
			}
			h.logger.Error("get ssh keys failed", "user_id", userID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "get keys failed"})
			return
		}

		writeJSON(w, nethttp.StatusOK, map[string]any{
			"public_key":  pubKey,
			"private_key": privKey,
			"key_type":    keyType,
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
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "user_id is required"})
			return
		}

		if err := h.store.UpdateUserSSHKeys(r.Context(), userID, "", "", ""); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeJSON(w, nethttp.StatusNotFound, map[string]string{"error": "user not found"})
				return
			}
			h.logger.Error("delete ssh keys failed", "user_id", userID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "delete keys failed"})
			return
		}

		w.WriteHeader(nethttp.StatusNoContent)
	})
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
