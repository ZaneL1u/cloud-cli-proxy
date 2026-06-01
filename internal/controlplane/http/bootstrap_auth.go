package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	nethttp "net/http"

	"database/sql"
	"golang.org/x/crypto/bcrypt"

	"github.com/zanel1u/cloud-cli-proxy/internal/agentapi"
	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

type BootstrapUserLookup interface {
	GetBootstrapUserByUsername(context.Context, string) (repository.BootstrapUserAuth, error)
}

type BootstrapHostLookup interface {
	GetPrimaryHostByUserID(context.Context, string) (repository.Host, error)
}

type BootstrapAuthDependencies struct {
	Logger *slog.Logger
	Users  BootstrapUserLookup
	Hosts  BootstrapHostLookup
	Queue  HostActionQueuer
	Events EventRecorder
}

type bootstrapAuthHandler struct {
	logger *slog.Logger
	users  BootstrapUserLookup
	hosts  BootstrapHostLookup
	queue  HostActionQueuer
	events EventRecorder
}

func NewBootstrapAuthHandler(deps BootstrapAuthDependencies) nethttp.Handler {
	return &bootstrapAuthHandler{
		logger: deps.Logger,
		users:  deps.Users,
		hosts:  deps.Hosts,
		queue:  deps.Queue,
		events: deps.Events,
	}
}

type bootstrapAuthRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *bootstrapAuthHandler) ServeHTTP(w nethttp.ResponseWriter, r *nethttp.Request) {
	var req bootstrapAuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBootstrapError(w, nethttp.StatusBadRequest, "invalid_request", "请求格式无效")
		return
	}

	if req.Username == "" || req.Password == "" {
		writeBootstrapError(w, nethttp.StatusBadRequest, "invalid_request", "用户名和密码不能为空")
		return
	}

	user, err := h.users.GetBootstrapUserByUsername(r.Context(), req.Username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			h.recordAuthEvent(r.Context(), nil, req.Username, "invalid_credentials")
			writeBootstrapError(w, nethttp.StatusUnauthorized, "auth_invalid", "用户名或密码错误")
			return
		}
		h.logger.Error("bootstrap user lookup failed", "username", req.Username, "error", err)
		writeBootstrapError(w, nethttp.StatusInternalServerError, "internal_error", "认证服务暂时不可用，请稍后重试")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		h.recordAuthEvent(r.Context(), &user.UserID, req.Username, "invalid_credentials")
		writeBootstrapError(w, nethttp.StatusUnauthorized, "auth_invalid", "用户名或密码错误")
		return
	}

	switch user.Status {
	case "active":
		// ok
	case "disabled":
		h.recordAuthEvent(r.Context(), &user.UserID, req.Username, "account_disabled")
		writeBootstrapError(w, nethttp.StatusForbidden, "account_disabled", "账号已被停用，请联系管理员")
		return
	case "expired":
		h.recordAuthEvent(r.Context(), &user.UserID, req.Username, "account_expired")
		writeBootstrapError(w, nethttp.StatusForbidden, "account_expired", "账号已过期，请联系管理员续期")
		return
	default:
		h.recordAuthEvent(r.Context(), &user.UserID, req.Username, "account_"+user.Status)
		writeBootstrapError(w, nethttp.StatusForbidden, "account_disabled", "账号状态异常，请联系管理员")
		return
	}

	host, err := h.hosts.GetPrimaryHostByUserID(r.Context(), user.UserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeBootstrapError(w, nethttp.StatusNotFound, "host_not_found", "未找到可用主机，请联系管理员分配")
			return
		}
		h.logger.Error("bootstrap host lookup failed", "user_id", user.UserID, "error", err)
		writeBootstrapError(w, nethttp.StatusInternalServerError, "internal_error", "查询主机信息失败，请稍后重试")
		return
	}

	requestedBy := fmt.Sprintf("bootstrap:%s", user.Username)
	task, err := h.queue.QueueHostAction(r.Context(), host.ID, agentapi.ActionStartHost, requestedBy, "")
	if err != nil {
		h.logger.Error("bootstrap queue start_host failed", "host_id", host.ID, "error", err)
		writeBootstrapError(w, nethttp.StatusInternalServerError, "start_failed", "启动任务创建失败，请稍后重试")
		return
	}

	if h.events != nil {
		if _, err := h.events.RecordEvent(r.Context(), repository.RecordEventParams{
			UserID:   &user.UserID,
			Level:    "info",
			Type:     "auth.success",
			Message:  "Bootstrap 认证成功",
			Metadata: map[string]any{"operator": "bootstrap", "username": req.Username},
		}); err != nil {
			h.logger.Error("record event failed", "type", "auth.success", "error", err)
		}
	}

	writeJSON(w, nethttp.StatusOK, map[string]any{
		"task_id":    task.ID,
		"status_url": fmt.Sprintf("/v1/bootstrap/tasks/%s", task.ID),
		"stage_code": "auth_passed",
		"stage_text": "认证通过，主机启动中",
	})
}

func (h *bootstrapAuthHandler) recordAuthEvent(ctx context.Context, userID *string, username, reason string) {
	if h.events == nil {
		return
	}
	if _, err := h.events.RecordEvent(ctx, repository.RecordEventParams{
		UserID:   userID,
		Level:    "warn",
		Type:     "auth.failed",
		Message:  "Bootstrap 认证失败",
		Metadata: map[string]any{"operator": "bootstrap", "username": username, "reason": reason},
	}); err != nil {
		h.logger.Error("record event failed", "type", "auth.failed", "error", err)
	}
}

func writeBootstrapError(w nethttp.ResponseWriter, status int, errorCode, message string) {
	writeJSON(w, status, map[string]string{
		"error_code": errorCode,
		"message":    message,
	})
}
