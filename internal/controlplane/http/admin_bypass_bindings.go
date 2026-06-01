package http

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	nethttp "net/http"
	"strings"

	"database/sql"

	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

// AdminBypassBindingStore 聚合 Binding CRUD 所需 Repository 方法子集。
// 同时复用 InsertBypassAuditLog 写双轨审计。
type AdminBypassBindingStore interface {
	ListBypassBindingsByHost(context.Context, string) ([]repository.BypassBinding, error)
	CreateBypassBinding(context.Context, repository.CreateBypassBindingParams) (repository.BypassBinding, error)
	DeleteBypassBinding(context.Context, string) error
	InsertBypassAuditLog(context.Context, repository.InsertBypassAuditLogParams) (string, error)
}

type AdminBypassBindingsHandler struct {
	logger *slog.Logger
	store  AdminBypassBindingStore
	events EventRecorder
}

func NewAdminBypassBindingsHandler(
	logger *slog.Logger,
	store AdminBypassBindingStore,
	events EventRecorder,
) *AdminBypassBindingsHandler {
	return &AdminBypassBindingsHandler{logger: logger, store: store, events: events}
}

// ListByHost 返回某宿主机所有 binding。路由路径 /v1/admin/hosts/{hostID}/bypass。
func (h *AdminBypassBindingsHandler) ListByHost() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		hostID := strings.TrimSpace(r.PathValue("hostID"))
		if hostID == "" {
			writeBypassError(w, nethttp.StatusBadRequest, ErrCodeBypassInvalidRequest, "hostID is required")
			return
		}
		bindings, err := h.store.ListBypassBindingsByHost(r.Context(), hostID)
		if err != nil {
			h.logger.Error("list bypass bindings failed", "host_id", hostID, "error", err)
			writeBypassError(w, nethttp.StatusInternalServerError, "INTERNAL", "list bypass bindings failed")
			return
		}
		writeJSON(w, nethttp.StatusOK, map[string]any{"bindings": bindings})
	})
}

type bypassBindingRequest struct {
	PresetID string `json:"preset_id,omitempty"`
	RuleID   string `json:"rule_id,omitempty"`
	Enabled  *bool  `json:"enabled,omitempty"`
	Source   string `json:"source,omitempty"`
}

// Bind 为某宿主机绑定一个 preset 或 rule。preset_id / rule_id 必须二选一，
// 同传或都不传都返回 400 BYPASS_INVALID_REQUEST。
// 路由路径 /v1/admin/hosts/{hostID}/bypass。
func (h *AdminBypassBindingsHandler) Bind() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		hostID := strings.TrimSpace(r.PathValue("hostID"))
		if hostID == "" {
			writeBypassError(w, nethttp.StatusBadRequest, ErrCodeBypassInvalidRequest, "hostID is required")
			return
		}

		var req bypassBindingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeBypassError(w, nethttp.StatusBadRequest, ErrCodeBypassInvalidRequest, "invalid request body")
			return
		}
		req.PresetID = strings.TrimSpace(req.PresetID)
		req.RuleID = strings.TrimSpace(req.RuleID)

		// preset_id / rule_id 互斥且二选一。
		if req.PresetID == "" && req.RuleID == "" {
			writeBypassError(w, nethttp.StatusBadRequest, ErrCodeBypassInvalidRequest, "preset_id or rule_id is required")
			return
		}
		if req.PresetID != "" && req.RuleID != "" {
			writeBypassError(w, nethttp.StatusBadRequest, ErrCodeBypassInvalidRequest, "preset_id and rule_id are mutually exclusive")
			return
		}

		enabled := true
		if req.Enabled != nil {
			enabled = *req.Enabled
		}
		source := strings.TrimSpace(req.Source)
		if source == "" {
			source = "admin"
		}

		params := repository.CreateBypassBindingParams{
			HostID:  hostID,
			Enabled: enabled,
			Source:  source,
		}
		if req.PresetID != "" {
			p := req.PresetID
			params.PresetID = &p
		}
		if req.RuleID != "" {
			rid := req.RuleID
			params.RuleID = &rid
		}

		created, err := h.store.CreateBypassBinding(r.Context(), params)
		if err != nil {
			h.logger.Error("create bypass binding failed", "host_id", hostID, "error", err)
			writeBypassError(w, nethttp.StatusInternalServerError, "INTERNAL", "create bypass binding failed")
			return
		}

		writeBypassAuditLog(r.Context(), h.logger, h.store, h.events, r, "bind", "binding", &created.ID, nil, created, "")
		writeJSON(w, nethttp.StatusCreated, map[string]any{"binding": created})
	})
}

// Unbind 删除一条 binding。路由路径 /v1/admin/bypass/bindings/{bindingID}。
func (h *AdminBypassBindingsHandler) Unbind() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		bindingID := strings.TrimSpace(r.PathValue("bindingID"))
		if bindingID == "" {
			writeBypassError(w, nethttp.StatusBadRequest, ErrCodeBypassInvalidRequest, "bindingID is required")
			return
		}
		if err := h.store.DeleteBypassBinding(r.Context(), bindingID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeBypassError(w, nethttp.StatusNotFound, ErrCodeBypassBindingNotFound, "binding not found")
				return
			}
			h.logger.Error("delete bypass binding failed", "binding_id", bindingID, "error", err)
			writeBypassError(w, nethttp.StatusInternalServerError, "INTERNAL", "delete bypass binding failed")
			return
		}

		writeBypassAuditLog(r.Context(), h.logger, h.store, h.events, r, "unbind", "binding", &bindingID, nil, nil, "")
		w.WriteHeader(nethttp.StatusNoContent)
	})
}
