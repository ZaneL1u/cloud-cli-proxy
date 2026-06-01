package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	nethttp "net/http"
	"strings"

	"database/sql"
	"golang.org/x/crypto/bcrypt"

	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

// UserPasswordStore 用户自助修改登录密码
type UserPasswordStore interface {
	GetUser(context.Context, string) (repository.User, error)
	UpdateUserPassword(context.Context, string, string) error
}

type UserPasswordHandler struct {
	logger *slog.Logger
	store  UserPasswordStore
}

func NewUserPasswordHandler(logger *slog.Logger, store UserPasswordStore) *UserPasswordHandler {
	return &UserPasswordHandler{logger: logger, store: store}
}

type changePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

func (h *UserPasswordHandler) ChangePassword() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		userID := UserIDFromContext(r.Context())
		if userID == "" {
			writeJSON(w, nethttp.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "read body failed"})
			return
		}
		var req changePasswordRequest
		if err := json.Unmarshal(body, &req); err != nil {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		req.OldPassword = strings.TrimSpace(req.OldPassword)
		req.NewPassword = strings.TrimSpace(req.NewPassword)
		if req.OldPassword == "" || req.NewPassword == "" {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "old_password and new_password are required"})
			return
		}
		if len(req.NewPassword) < 8 || len(req.NewPassword) > 128 {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "new password must be 8-128 characters"})
			return
		}

		user, err := h.store.GetUser(r.Context(), userID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, nethttp.StatusNotFound, map[string]string{"error": "user not found"})
				return
			}
			h.logger.Error("change password: get user failed", "user_id", userID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.OldPassword)); err != nil {
			writeJSON(w, nethttp.StatusUnauthorized, map[string]string{"error": "invalid old password"})
			return
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
		if err != nil {
			h.logger.Error("change password: hash failed", "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}

		if err := h.store.UpdateUserPassword(r.Context(), userID, string(hash)); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, nethttp.StatusNotFound, map[string]string{"error": "user not found"})
				return
			}
			h.logger.Error("change password failed", "user_id", userID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "change password failed"})
			return
		}

		writeJSON(w, nethttp.StatusOK, map[string]string{"status": "ok"})
	})
}

