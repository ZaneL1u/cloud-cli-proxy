package http

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	nethttp "net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"database/sql"
	"golang.org/x/crypto/ssh"

	"github.com/zanel1u/cloud-cli-proxy/internal/controlplane/credgen"
	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

type SSHKeyStore interface {
	ListSSHKeysByUser(ctx context.Context, userID string) ([]repository.SSHKey, error)
	CreateSSHKey(ctx context.Context, userID, purpose, label, publicKey, privateKey, keyType, fingerprint string) (repository.SSHKey, error)
	DeleteSSHKey(ctx context.Context, keyID, userID string) error
	ListRunningHostsByUserID(ctx context.Context, userID string) ([]repository.Host, error)
	GetUser(ctx context.Context, userID string) (repository.User, error)
}

type sshKeyResponse struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	Purpose     string    `json:"purpose"`
	Label       string    `json:"label"`
	PublicKey   string    `json:"public_key"`
	PrivateKey  string    `json:"private_key,omitempty"`
	KeyType     string    `json:"key_type"`
	Fingerprint string    `json:"fingerprint"`
	CreatedAt   time.Time `json:"created_at"`
	Source      string    `json:"source"`
	Synced      bool      `json:"synced"`
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

		dbKeys, err := h.store.ListSSHKeysByUser(r.Context(), userID)
		if err != nil {
			h.logger.Error("list ssh keys failed", "user_id", userID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "查询密钥失败"})
			return
		}

		dbFingerprintSet := make(map[string]bool, len(dbKeys))
		for _, k := range dbKeys {
			if k.Purpose == "inbound" && k.Fingerprint != "" {
				dbFingerprintSet[k.Fingerprint] = true
			}
		}

		containerFingerprints := make(map[string]bool)
		hosts, _ := h.store.ListRunningHostsByUserID(r.Context(), userID)
		if len(hosts) > 0 {
			containerName := "cloudproxy-" + hosts[0].ID
			containerFingerprints = readContainerAuthorizedKeyFingerprints(r.Context(), containerName)
		}

		result := make([]sshKeyResponse, 0, len(dbKeys)+len(containerFingerprints))
		for _, k := range dbKeys {
			resp := sshKeyResponse{
				ID:          k.ID,
				UserID:      k.UserID,
				Purpose:     k.Purpose,
				Label:       k.Label,
				PublicKey:   k.PublicKey,
				KeyType:     k.KeyType,
				Fingerprint: k.Fingerprint,
				CreatedAt:   k.CreatedAt,
				Source:      "managed",
			}
			if k.Purpose == "outbound" {
				resp.PrivateKey = ""
			}
			if k.Purpose == "inbound" && k.Fingerprint != "" {
				resp.Synced = containerFingerprints[k.Fingerprint]
			} else {
				resp.Synced = true
			}
			result = append(result, resp)
		}

		containerKeys := readContainerAuthorizedKeys(r.Context(), hosts)
		for _, ck := range containerKeys {
			if dbFingerprintSet[ck.Fingerprint] {
				continue
			}
			result = append(result, sshKeyResponse{
				Purpose:     "inbound",
				Label:       "手动添加",
				PublicKey:   ck.PublicKey,
				Fingerprint: ck.Fingerprint,
				Source:      "container",
				Synced:      true,
			})
		}

		writeJSON(w, nethttp.StatusOK, map[string]any{"keys": result})
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
			pubKeyStr, privKeyStr, err := credgen.GenerateSSHKeyPair(req.KeyType, req.Label)
			if err != nil {
				h.logger.Error("generate ssh key pair failed", "user_id", userID, "error", err)
				writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "生成密钥对失败"})
				return
			}
			req.PublicKey = pubKeyStr
			req.PrivateKey = privKeyStr
		}

		if credgen.ComputeFingerprint(req.PublicKey) == "" {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "public_key 格式错误"})
			return
		}
		if req.Purpose == "outbound" && req.PrivateKey != "" {
			if err := validateSSHKeyPair(req.PublicKey, req.PrivateKey); err != nil {
				writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "private_key 无效或与 public_key 不匹配"})
				return
			}
		}

		fingerprint := credgen.ComputeFingerprint(req.PublicKey)

		key, err := h.store.CreateSSHKey(r.Context(), userID, req.Purpose, req.Label, req.PublicKey, req.PrivateKey, req.KeyType, fingerprint)
		if err != nil {
			h.logger.Error("create ssh key failed", "user_id", userID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "创建密钥失败"})
			return
		}

		go h.syncKeysToRunningHosts(userID)

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
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, nethttp.StatusNotFound, map[string]string{"error": "密钥不存在"})
				return
			}
			h.logger.Error("delete ssh key failed", "user_id", userID, "key_id", keyID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "删除密钥失败"})
			return
		}

		go h.syncKeysToRunningHosts(userID)

		w.WriteHeader(nethttp.StatusNoContent)
	})
}

func (h *SSHKeyHandler) syncKeysToRunningHosts(userID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	hosts, err := h.store.ListRunningHostsByUserID(ctx, userID)
	if err != nil {
		h.logger.Warn("sync ssh keys: list running hosts failed", "user_id", userID, "error", err)
		return
	}
	if len(hosts) == 0 {
		return
	}

	allKeys, err := h.store.ListSSHKeysByUser(ctx, userID)
	if err != nil {
		h.logger.Warn("sync ssh keys: list keys failed", "user_id", userID, "error", err)
		return
	}

	owner, err := h.store.GetUser(ctx, userID)
	if err != nil {
		h.logger.Warn("sync ssh keys: get user failed", "user_id", userID, "error", err)
		return
	}
	user := owner.Username
	if user == "" {
		user = "workspace"
	}

	for _, host := range hosts {
		containerName := "cloudproxy-" + host.ID
		syncInboundKeysToContainer(ctx, containerName, user, allKeys)
		syncOutboundKeysToContainer(ctx, containerName, user, allKeys)
		h.logger.Info("synced ssh keys to container", "host_id", host.ID, "container", containerName)
	}
}

func syncInboundKeysToContainer(ctx context.Context, containerName, user string, keys []repository.SSHKey) {
	sshDir := "/workspace/.ssh"
	var lines []string
	if proxyPub := loadProxyPublicKey(); proxyPub != "" {
		lines = append(lines, proxyPub)
	}
	for _, k := range keys {
		if k.Purpose == "inbound" && k.PublicKey != "" {
			lines = append(lines, strings.TrimSpace(k.PublicKey))
		}
	}

	content := ""
	if len(lines) > 0 {
		content = strings.Join(lines, "\n") + "\n"
	}

	script := fmt.Sprintf(
		"mkdir -p %s && cat > %s/authorized_keys && chmod 600 %s/authorized_keys && chown %s:%s %s/authorized_keys",
		sshDir, sshDir, sshDir, user, user, sshDir,
	)
	cmd := exec.CommandContext(ctx, "docker", "exec", "-i", containerName, "bash", "-c", script)
	cmd.Stdin = strings.NewReader(content)
	if out, err := cmd.CombinedOutput(); err != nil {
		slog.Warn("sync inbound keys to container failed",
			"container", containerName, "error", err, "output", strings.TrimSpace(string(out)))
	}
}

func syncOutboundKeysToContainer(ctx context.Context, containerName, user string, keys []repository.SSHKey) {
	sshDir := "/workspace/.ssh"

	outboundIdx := 0
	for _, key := range keys {
		if key.Purpose != "outbound" {
			continue
		}

		var keyFile, pubFile string
		if outboundIdx == 0 {
			if key.KeyType == "rsa" || strings.Contains(key.PublicKey, "ssh-rsa") {
				keyFile = sshDir + "/id_rsa"
				pubFile = sshDir + "/id_rsa.pub"
			} else {
				keyFile = sshDir + "/id_ed25519"
				pubFile = sshDir + "/id_ed25519.pub"
			}
		} else {
			safeName := key.Label
			if safeName == "" {
				safeName = fmt.Sprintf("id_%d", outboundIdx)
			}
			keyFile = sshDir + "/" + safeName
			pubFile = sshDir + "/" + safeName + ".pub"
		}

		if key.PrivateKey != "" {
			script := fmt.Sprintf(
				"mkdir -p %s && cat > %s && chmod 600 %s && chown %s:%s %s",
				sshDir, keyFile, keyFile, user, user, keyFile,
			)
			cmd := exec.CommandContext(ctx, "docker", "exec", "-i", containerName, "bash", "-c", script)
			cmd.Stdin = strings.NewReader(key.PrivateKey)
			if out, err := cmd.CombinedOutput(); err != nil {
				slog.Warn("sync outbound private key failed",
					"container", containerName, "file", keyFile, "error", err, "output", strings.TrimSpace(string(out)))
			}
		}

		if key.PublicKey != "" {
			script := fmt.Sprintf(
				"mkdir -p %s && cat > %s && chmod 644 %s && chown %s:%s %s",
				sshDir, pubFile, pubFile, user, user, pubFile,
			)
			cmd := exec.CommandContext(ctx, "docker", "exec", "-i", containerName, "bash", "-c", script)
			cmd.Stdin = strings.NewReader(key.PublicKey)
			if out, err := cmd.CombinedOutput(); err != nil {
				slog.Warn("sync outbound public key failed",
					"container", containerName, "file", pubFile, "error", err, "output", strings.TrimSpace(string(out)))
			}
		}

		outboundIdx++
	}
}

type containerKeyEntry struct {
	PublicKey   string
	Fingerprint string
}

func readContainerAuthorizedKeys(ctx context.Context, hosts []repository.Host) []containerKeyEntry {
	if len(hosts) == 0 {
		return nil
	}
	containerName := "cloudproxy-" + hosts[0].ID
	timeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(timeout, "docker", "exec", "-i", containerName, "cat", "/workspace/.ssh/authorized_keys")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	var entries []containerKeyEntry
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fp := credgen.ComputeFingerprint(line)
		if fp == "" {
			continue
		}
		entries = append(entries, containerKeyEntry{PublicKey: line, Fingerprint: fp})
	}
	return entries
}

func readContainerAuthorizedKeyFingerprints(ctx context.Context, containerName string) map[string]bool {
	timeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(timeout, "docker", "exec", "-i", containerName, "cat", "/workspace/.ssh/authorized_keys")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	result := make(map[string]bool)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fp := credgen.ComputeFingerprint(line)
		if fp != "" {
			result[fp] = true
		}
	}
	return result
}

func loadProxyPublicKey() string {
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "/var/lib/cloud-cli-proxy"
	}
	pubKeyPath := dataDir + "/ssh_host_ed25519_key.pub"
	data, err := os.ReadFile(pubKeyPath)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func validateSSHKeyPair(publicKeyStr, privateKeyStr string) error {
	pubKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(strings.TrimSpace(publicKeyStr)))
	if err != nil {
		return fmt.Errorf("parse public key: %w", err)
	}

	privateKey, err := ssh.ParseRawPrivateKey([]byte(privateKeyStr))
	if err != nil {
		return fmt.Errorf("parse private key: %w", err)
	}

	var rawPublicKey any
	switch k := privateKey.(type) {
	case ed25519.PrivateKey:
		rawPublicKey = k.Public()
	case *ed25519.PrivateKey:
		rawPublicKey = k.Public()
	case *rsa.PrivateKey:
		rawPublicKey = &k.PublicKey
	case *ecdsa.PrivateKey:
		rawPublicKey = &k.PublicKey
	default:
		return fmt.Errorf("unsupported private key type: %T", privateKey)
	}

	derivedPublicKey, err := ssh.NewPublicKey(rawPublicKey)
	if err != nil {
		return fmt.Errorf("derive public key from private key: %w", err)
	}
	if !bytes.Equal(pubKey.Marshal(), derivedPublicKey.Marshal()) {
		return errors.New("private key does not match public key")
	}
	return nil
}
