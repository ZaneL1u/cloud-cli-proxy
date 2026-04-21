package http

import (
	"context"
	"errors"
	"log/slog"
	nethttp "net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/zanel1u/cloud-cli-proxy/internal/agentapi"
	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

// AdminClaudeAccountStore 暴露 Plan 02 仅需的最小集（与 AdminHostStore 风格一致）。
type AdminClaudeAccountStore interface {
	BeginTx(ctx context.Context) (pgx.Tx, error)
}

// HostActionRunner 抽象 host-agent 调用，让 handler 在 embedded（in-process worker）和
// 远端 host-agent 两种部署模式下都能工作。
//   - 远端模式：*agentapi.Client.RunHostAction 直接满足
//   - embedded 模式：用 EmbeddedHostActionRunner 适配 EmbeddedDispatcher.Dispatch（语义等价）
type HostActionRunner interface {
	RunHostAction(ctx context.Context, req agentapi.HostActionRequest) (agentapi.HostActionResponse, error)
}

// runHostAction 包级 var 便于测试注入 mock（沿用 syncContainerPassword 模式 admin_hosts.go）。
var runHostAction = func(ctx context.Context, client HostActionRunner, req agentapi.HostActionRequest) (agentapi.HostActionResponse, error) {
	return client.RunHostAction(ctx, req)
}

type AdminClaudeAccountsHandler struct {
	logger      *slog.Logger
	store       AdminClaudeAccountStore
	agentClient HostActionRunner
	events      EventRecorder
}

func NewAdminClaudeAccountsHandler(logger *slog.Logger, store AdminClaudeAccountStore, agentClient HostActionRunner, events EventRecorder) *AdminClaudeAccountsHandler {
	return &AdminClaudeAccountsHandler{logger: logger, store: store, agentClient: agentClient, events: events}
}

func (h *AdminClaudeAccountsHandler) Delete() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		accountID := r.PathValue("accountID")
		if parseForceFlag(r.URL.Query().Get("force")) {
			h.deleteForce(w, r, accountID)
			return
		}
		h.deleteStrict(w, r, accountID)
	})
}

func parseForceFlag(s string) bool {
	switch s {
	case "true", "1", "yes":
		return true
	}
	return false
}

// deleteStrict D-18 强一致路径：BEGIN → SELECT FOR UPDATE → 调 host-agent → 成功 DELETE+COMMIT；失败 ROLLBACK + 409。
func (h *AdminClaudeAccountsHandler) deleteStrict(w nethttp.ResponseWriter, r *nethttp.Request, accountID string) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	tx, err := h.store.BeginTx(ctx)
	if err != nil {
		h.logger.Error("begin tx failed", "error", err)
		writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "begin tx failed"})
		return
	}
	rollback := true
	defer func() {
		if rollback {
			_ = tx.Rollback(context.Background())
		}
	}()

	id, volumeName, err := repository.LockClaudeAccountForDelete(ctx, tx, accountID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, nethttp.StatusNotFound, map[string]string{"error": "claude_account not found"})
			return
		}
		h.logger.Error("lock claude_account failed", "id", accountID, "error", err)
		writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "lock claude_account failed"})
		return
	}

	if volumeName != "" {
		req := agentapi.HostActionRequest{
			Action:  agentapi.ActionVolumeRemove,
			Volumes: []agentapi.VolumeMount{{Name: volumeName}},
		}
		if _, err := runHostAction(ctx, h.agentClient, req); err != nil {
			h.recordEvent(r.Context(), "claude_account.delete_volume_rm_failed", map[string]any{
				"account_id":    id,
				"volume_name":   volumeName,
				"error_code":    "volume_in_use",
				"error_message": err.Error(),
			})
			writeJSON(w, nethttp.StatusConflict, map[string]any{
				"error": map[string]string{
					"code":        "STATE_VOLUME_IN_USE_001",
					"message":     "请先停止使用该账号的所有 host 后重试，或追加 ?force=true 强删 volume",
					"next_action": "停止 host → 重试 DELETE，或附加 ?force=true",
				},
			})
			return
		}
	}

	if err := repository.DeleteClaudeAccountTx(ctx, tx, id); err != nil {
		h.logger.Error("delete claude_account failed", "id", id, "error", err)
		writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "delete claude_account failed"})
		return
	}
	if err := tx.Commit(ctx); err != nil {
		h.logger.Error("commit failed", "error", err)
		writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "commit failed"})
		return
	}
	rollback = false

	h.recordEvent(r.Context(), "claude_account.deleted", map[string]any{
		"account_id":  id,
		"volume_name": volumeName,
		"force":       false,
	})
	writeJSON(w, nethttp.StatusOK, map[string]any{
		"deleted":   true,
		"volume_rm": "succeeded",
	})
}

// deleteForce D-19 最终一致路径：DB 先 COMMIT；rm 失败仅写 audit + 返回 200。
func (h *AdminClaudeAccountsHandler) deleteForce(w nethttp.ResponseWriter, r *nethttp.Request, accountID string) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	tx, err := h.store.BeginTx(ctx)
	if err != nil {
		h.logger.Error("begin tx failed (force)", "error", err)
		writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "begin tx failed"})
		return
	}
	id, volumeName, err := repository.LockClaudeAccountForDelete(ctx, tx, accountID)
	if err != nil {
		_ = tx.Rollback(ctx)
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, nethttp.StatusNotFound, map[string]string{"error": "claude_account not found"})
			return
		}
		writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "lock claude_account failed"})
		return
	}
	if err := repository.DeleteClaudeAccountTx(ctx, tx, id); err != nil {
		_ = tx.Rollback(ctx)
		h.logger.Error("delete claude_account failed (force)", "id", id, "error", err)
		writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "delete claude_account failed"})
		return
	}
	if err := tx.Commit(ctx); err != nil {
		h.logger.Error("commit failed (force)", "error", err)
		writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "commit failed"})
		return
	}

	resp := map[string]any{"deleted": true, "volume_rm": "skipped"}
	if volumeName != "" {
		req := agentapi.HostActionRequest{
			Action:  agentapi.ActionVolumeRemove,
			Volumes: []agentapi.VolumeMount{{Name: volumeName}},
			Labels:  map[string]string{"force": "true"},
		}
		if _, err := runHostAction(ctx, h.agentClient, req); err != nil {
			h.recordEvent(r.Context(), "claude_account.force_volume_rm_failed", map[string]any{
				"account_id":    id,
				"volume_name":   volumeName,
				"error_message": err.Error(),
			})
			resp["volume_rm"] = "failed"
			resp["next_action"] = "运维需手工 docker volume rm -f " + volumeName
		} else {
			resp["volume_rm"] = "succeeded"
		}
	}
	h.recordEvent(r.Context(), "claude_account.deleted", map[string]any{
		"account_id":  id,
		"volume_name": volumeName,
		"force":       true,
	})
	writeJSON(w, nethttp.StatusOK, resp)
}

func (h *AdminClaudeAccountsHandler) recordEvent(ctx context.Context, eventType string, metadata map[string]any) {
	if h.events == nil {
		return
	}
	if _, err := h.events.RecordEvent(ctx, repository.RecordEventParams{
		Level:    "info",
		Type:     eventType,
		Message:  "管理员删除 Claude 账号",
		Metadata: metadata,
	}); err != nil {
		h.logger.Error("record event failed", "type", eventType, "error", err)
	}
}
