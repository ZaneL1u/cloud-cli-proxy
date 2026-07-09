package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	nethttp "net/http"
	"strings"

	"database/sql"
	"golang.org/x/crypto/bcrypt"

	"github.com/zanel1u/cloud-cli-proxy/internal/runtime"
	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

type EntryStore interface {
	GetUserByUsername(context.Context, string) (repository.User, error)
	GetPrimaryHostByUserID(context.Context, string) (repository.Host, error)
	GetHostByUsername(context.Context, string) (repository.HostSSHAuth, error)
	GetUser(context.Context, string) (repository.User, error)
	// ResolveClaudeAccountIDForEntry 供 ready 响应填充 claude_account_id（Phase 30 D-05）。
	// 未命中返回 ok=false，而非错误。
	ResolveClaudeAccountIDForEntry(ctx context.Context, userID, hostID string) (string, bool, error)
}

// deriveEntryCapabilities 按 D-06/D-07 从 template_image_ref 推导 Entry API 能力字段。
// 规则：
//  1. 整体 trim 空白；
//  2. 取最后一个 ":" 之后的 tag；若不存在 ":"，整串视为 tag；
//  3. 再对 tag trim 空白（兼容异常配置）；
//  4. supports_mergerfs = imageLockSupportsMergerfs || (tag == imageLockVersion)。
//     imageLockSupportsMergerfs 来自 image.lock 的显式声明，优先于 tag 推导，
//     imageLockVersion 来自 image.lock 的 image_version 字段，用于替代硬编码基线。
func deriveEntryCapabilities(templateImageRef string, imageLockSupportsMergerfs bool, imageLockVersion string) (imageVersion string, supportsMergerfs bool) {
	if imageLockSupportsMergerfs {
		tag := strings.TrimSpace(templateImageRef)
		if idx := strings.LastIndex(tag, ":"); idx != -1 {
			tag = tag[idx+1:]
		}
		tag = strings.TrimSpace(tag)
		return tag, true
	}
	tag := strings.TrimSpace(templateImageRef)
	if idx := strings.LastIndex(tag, ":"); idx != -1 {
		tag = tag[idx+1:]
	}
	tag = strings.TrimSpace(tag)
	supports := tag == imageLockVersion
	return tag, supports
}

type EntryHandler struct {
	logger        *slog.Logger
	store         EntryStore
	baseURL       string
	imageLockPath string
}

func NewEntryHandler(logger *slog.Logger, store EntryStore, baseURL, imageLockPath string) *EntryHandler {
	return &EntryHandler{logger: logger, store: store, baseURL: baseURL, imageLockPath: imageLockPath}
}

func (h *EntryHandler) Script() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		username := r.PathValue("username")
		if username == "" {
			nethttp.NotFound(w, r)
			return
		}

		base := h.baseURL
		if base == "" {
			scheme := "https"
			if r.TLS == nil {
				scheme = "http"
			}
			base = fmt.Sprintf("%s://%s", scheme, r.Host)
		}

		script := fmt.Sprintf(`#!/bin/bash
set -e
read -sp "Password: " PASS < /dev/tty; echo
RESP=$(curl -sf -X POST "%s/v1/entry/%s/auth" \
  -H "Content-Type: application/json" -d "{\"password\":\"$PASS\"}")
if [ $? -ne 0 ]; then echo "Authentication failed"; exit 1; fi
SSH_USER=$(echo "$RESP" | grep -o '"ssh_user":"[^"]*"' | cut -d'"' -f4)
SSH_PASS=$(echo "$RESP" | grep -o '"ssh_pass":"[^"]*"' | cut -d'"' -f4)
SSH_PORT=$(echo "$RESP" | grep -o '"ssh_port":[0-9]*' | cut -d: -f2)
SSH_HOST=$(echo "$RESP" | grep -o '"ssh_host":"[^"]*"' | cut -d'"' -f4)
STATUS=$(echo "$RESP" | grep -o '"status":"[^"]*"' | cut -d'"' -f4)
if [ "$STATUS" != "ready" ]; then
  MSG=$(echo "$RESP" | grep -o '"message":"[^"]*"' | cut -d'"' -f4)
  echo "${MSG:-Your machine is not ready yet. Please try again later.}"
  exit 1
fi
echo "Connecting to your cloud machine..."
if command -v sshpass >/dev/null 2>&1; then
  exec sshpass -p "$SSH_PASS" ssh -tt -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -p "$SSH_PORT" "$SSH_USER@$SSH_HOST" < /dev/tty
else
  ASKPASS=$(mktemp); trap "rm -f $ASKPASS" EXIT
  printf '#!/bin/sh\necho "%%s"\n' "$SSH_PASS" > "$ASKPASS"; chmod +x "$ASKPASS"
  export SSH_ASKPASS="$ASKPASS" SSH_ASKPASS_REQUIRE=force
  exec ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -p "$SSH_PORT" "$SSH_USER@$SSH_HOST" < /dev/tty
fi
`, base, username)

		w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
		w.WriteHeader(nethttp.StatusOK)
		fmt.Fprint(w, script)
	})
}

func (h *EntryHandler) Auth() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		username := r.PathValue("username")
		if username == "" {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "username is required"})
			return
		}

		var body struct {
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Password == "" {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "password is required"})
			return
		}

		var user repository.User
		var hostID, hostEntryPassword, hostStatus, templateImageRef string

		hostAuth, hostErr := h.store.GetHostByUsername(r.Context(), username)
		if hostErr == nil {
			h.logger.Info("entry auth: resolved by username",
				"username", username, "host_id", hostAuth.HostID, "host_status", hostAuth.HostStatus)
			u, err := h.store.GetUser(r.Context(), hostAuth.UserID)
			if err != nil {
				h.logger.Error("entry auth: lookup host owner failed", "host_id", hostAuth.HostID, "error", err)
				writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "internal error"})
				return
			}
			user = u
			hostID = hostAuth.HostID
			hostEntryPassword = hostAuth.EntryPassword
			hostStatus = hostAuth.HostStatus
			templateImageRef = hostAuth.TemplateImageRef
		} else {
			// 一期兼容：username 查不到时，尝试按旧 short_id 查 user 再 fallback 到 primary host
			u, err := h.store.GetUserByUsername(r.Context(), username)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					writeJSON(w, nethttp.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
					return
				}
				h.logger.Error("entry auth: lookup user failed", "username", username, "error", err)
				writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "internal error"})
				return
			}
			user = u
			primaryHost, err := h.store.GetPrimaryHostByUserID(r.Context(), user.ID)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					writeJSON(w, nethttp.StatusNotFound, map[string]string{
						"error":  "no host assigned",
						"status": "no_host",
					})
					return
				}
				h.logger.Error("entry auth: lookup primary host failed", "user_id", user.ID, "error", err)
				writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "internal error"})
				return
			}
			h.logger.Info("entry auth: resolved by username (fallback to primary host)",
				"username", username, "host_id", primaryHost.ID, "host_status", primaryHost.Status)
			hostID = primaryHost.ID
			hostEntryPassword = user.EntryPassword
			hostStatus = primaryHost.Status
			templateImageRef = primaryHost.TemplateImageRef
		}

		if user.Status != "active" {
			writeJSON(w, nethttp.StatusForbidden, map[string]string{"error": "account is not active"})
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(body.Password)); err != nil {
			writeJSON(w, nethttp.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
			return
		}

		if hostStatus != "running" {
			h.logger.Warn("entry auth: host not running", "username", username, "host_status", hostStatus)
			writeJSON(w, nethttp.StatusOK, map[string]any{
				"status":  "not_ready",
				"message": "Your machine is not running. Please contact admin.",
			})
			return
		}

		sshHost := r.Host
		if idx := strings.Index(sshHost, ":"); idx != -1 {
			sshHost = sshHost[:idx]
		}

		// Phase 30 D-06/D-07：依据 template_image_ref + image.lock 显式声明推导能力字段。
		// image.lock 的 supports_mergerfs 优先，解决重建主机后 DB 字段未同步问题。
		spec, specErr := runtime.LoadRuntimeSpec(h.imageLockPath)
		if specErr != nil {
			h.logger.Error("load image.lock failed", "error", specErr)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "image.lock load failed"})
			return
		}
		imageVersion, supportsMergerfs := deriveEntryCapabilities(templateImageRef, spec.SupportsMergerfs, spec.ImageVersion)

		resp := map[string]any{
			"ssh_user":          user.Username,
			"ssh_pass":          hostEntryPassword,
			"ssh_host":          sshHost,
			"ssh_port":          2222,
			"status":            "ready",
			"image_version":     imageVersion,
			"supports_mergerfs": supportsMergerfs,
		}

		// Phase 30 D-05：ready 路径按账号解析结果追加 claude_account_id；
		// 未命中 -> 省略字段（omitempty 语义），报错 -> 500 fail-fast，
		// 避免把"数据库错误"静默降级成"无账号"。
		accountID, ok, err := h.store.ResolveClaudeAccountIDForEntry(r.Context(), user.ID, hostID)
		if err != nil {
			h.logger.Error("entry auth: resolve claude account failed",
				"host_id", hostID, "user_id", user.ID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
		if ok {
			resp["claude_account_id"] = accountID
		}

		writeJSON(w, nethttp.StatusOK, resp)
	})
}
