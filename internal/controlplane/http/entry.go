package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	nethttp "net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

type EntryStore interface {
	GetUserByShortID(context.Context, string) (repository.User, error)
	GetPrimaryHostByUserID(context.Context, string) (repository.Host, error)
	GetHostByShortID(context.Context, string) (repository.HostSSHAuth, error)
	GetUser(context.Context, string) (repository.User, error)
}

type EntryHandler struct {
	logger  *slog.Logger
	store   EntryStore
	baseURL string
}

func NewEntryHandler(logger *slog.Logger, store EntryStore, baseURL string) *EntryHandler {
	return &EntryHandler{logger: logger, store: store, baseURL: baseURL}
}

func (h *EntryHandler) Script() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		shortID := r.PathValue("shortId")
		if shortID == "" {
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
  exec sshpass -p "$SSH_PASS" ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -p "$SSH_PORT" "$SSH_USER@$SSH_HOST"
else
  ASKPASS=$(mktemp); trap "rm -f $ASKPASS" EXIT
  printf '#!/bin/sh\necho "%%s"\n' "$SSH_PASS" > "$ASKPASS"; chmod +x "$ASKPASS"
  export SSH_ASKPASS="$ASKPASS" SSH_ASKPASS_REQUIRE=force
  exec ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -p "$SSH_PORT" "$SSH_USER@$SSH_HOST" < /dev/tty
fi
`, base, shortID)

		w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
		w.WriteHeader(nethttp.StatusOK)
		fmt.Fprint(w, script)
	})
}

func (h *EntryHandler) Auth() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		shortID := r.PathValue("shortId")
		if shortID == "" {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "short_id is required"})
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
		var hostShortID, hostEntryPassword, hostStatus string

		hostAuth, hostErr := h.store.GetHostByShortID(r.Context(), shortID)
		if hostErr == nil {
			h.logger.Info("entry auth: resolved by host short_id",
				"short_id", shortID, "host_id", hostAuth.HostID, "host_status", hostAuth.HostStatus)
			u, err := h.store.GetUser(r.Context(), hostAuth.UserID)
			if err != nil {
				h.logger.Error("entry auth: lookup host owner failed", "host_id", hostAuth.HostID, "error", err)
				writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "internal error"})
				return
			}
			user = u
			hostShortID = hostAuth.HostShortID
			hostEntryPassword = hostAuth.EntryPassword
			hostStatus = hostAuth.HostStatus
		} else {
			u, err := h.store.GetUserByShortID(r.Context(), shortID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					writeJSON(w, nethttp.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
					return
				}
				h.logger.Error("entry auth: lookup user failed", "short_id", shortID, "error", err)
				writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "internal error"})
				return
			}
			user = u
			primaryHost, err := h.store.GetPrimaryHostByUserID(r.Context(), user.ID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
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
			h.logger.Info("entry auth: resolved by user short_id (fallback to primary host)",
				"short_id", shortID, "host_id", primaryHost.ID, "host_status", primaryHost.Status)
			hostShortID = primaryHost.ShortID
			hostEntryPassword = primaryHost.EntryPassword
			hostStatus = primaryHost.Status
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
			h.logger.Warn("entry auth: host not running", "short_id", shortID, "host_status", hostStatus)
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

		writeJSON(w, nethttp.StatusOK, map[string]any{
			"ssh_user": hostShortID,
			"ssh_pass": hostEntryPassword,
			"ssh_host": sshHost,
			"ssh_port": 2222,
			"status":   "ready",
		})
	})
}
