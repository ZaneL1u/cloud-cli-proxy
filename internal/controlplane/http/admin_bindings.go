package http

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	nethttp "net/http"

	"database/sql"

	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

type AdminBindingStore interface {
	GetHost(context.Context, string) (repository.Host, error)
	BindEgressIPToHost(context.Context, string, string) (repository.HostBinding, error)
	UnbindEgressIPFromHost(context.Context, string) error
	GetBindingHostID(context.Context, string) (string, error)
	// GetBindingHostIDByEgressIP Phase 51 Plan 09：返回该 egress IP 当前绑定
	// 的 host_id；row 不存在时返回 sql.ErrNoRows。
	GetBindingHostIDByEgressIP(context.Context, string) (string, error)
}

// ErrCodeEgressIPAlreadyBound Phase 51 Plan 09 / 闭 Phase 47 D-47-3：当
// 双绑互斥被拦截时 admin Bind API 响应中的稳定 error_code 字段值。
// 与 Phase 47 helpers `EgressIPDoubleBindContract` / `ParseBindEgressIPResponse`
// 锁定的契约对齐（机器可读 + 中文 message）。
const ErrCodeEgressIPAlreadyBound = "egress_ip_already_bound"

type AdminBindingsHandler struct {
	logger *slog.Logger
	store  AdminBindingStore
	events EventRecorder
}

func NewAdminBindingsHandler(logger *slog.Logger, store AdminBindingStore, events EventRecorder) *AdminBindingsHandler {
	return &AdminBindingsHandler{logger: logger, store: store, events: events}
}

type bindRequest struct {
	HostID     string `json:"host_id"`
	EgressIPID string `json:"egress_ip_id"`
}

func (h *AdminBindingsHandler) Bind() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		var req bindRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}

		if req.HostID == "" || req.EgressIPID == "" {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "host_id and egress_ip_id are required"})
			return
		}

		host, err := h.store.GetHost(r.Context(), req.HostID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, nethttp.StatusNotFound, map[string]string{"error": "host not found"})
				return
			}
			h.logger.Error("get host for binding check failed", "host_id", req.HostID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "check host status failed"})
			return
		}

		if host.Status == "running" {
			writeJSON(w, nethttp.StatusConflict, map[string]string{"error": "cannot bind egress IP to running host, stop host first"})
			return
		}

		// Phase 51 Plan 09 / 闭 Phase 47 D-47-3：双绑互斥 pre-check。
		// 如果该 egress IP 已绑定到另一台 host → 409 + 稳定 error_code +
		// 中文 message + 英文 message 子串（兼容 Phase 47 helpers 既有断言）。
		// 同 host 重新绑定同 IP 时跳过 pre-check，走原 INSERT 路径，由
		// host_egress_bindings 表的 UNIQUE (host_id, egress_ip_id) 复合键兜底
		// 重复 row（行为不变）。
		existingHostID, lookupErr := h.store.GetBindingHostIDByEgressIP(r.Context(), req.EgressIPID)
		if lookupErr != nil && !errors.Is(lookupErr, sql.ErrNoRows) {
			h.logger.Error("check existing binding by egress ip failed",
				"egress_ip_id", req.EgressIPID, "error", lookupErr)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{
				"error": "check existing binding failed",
			})
			return
		}
		if lookupErr == nil && existingHostID != "" && existingHostID != req.HostID {
			writeJSON(w, nethttp.StatusConflict, map[string]any{
				"error":        "出口 IP 已绑定到其它宿主机 (egress IP already bound to another host)",
				"error_code":   ErrCodeEgressIPAlreadyBound,
				"host_id":      existingHostID,
				"egress_ip_id": req.EgressIPID,
			})
			return
		}

		binding, err := h.store.BindEgressIPToHost(r.Context(), req.HostID, req.EgressIPID)
		if err != nil {
			h.logger.Error("bind egress ip failed", "host_id", req.HostID, "egress_ip_id", req.EgressIPID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "bind egress ip failed"})
			return
		}

		if h.events != nil {
			if _, err := h.events.RecordEvent(r.Context(), repository.RecordEventParams{
				HostID:   &req.HostID,
				Level:    "info",
				Type:     "admin.binding.created",
				Message:  "管理员创建出口 IP 绑定",
				Metadata: map[string]any{"operator": "admin", "egress_ip_id": req.EgressIPID, "binding_id": binding.BindingID},
			}); err != nil {
				h.logger.Error("record event failed", "type", "admin.binding.created", "error", err)
			}
		}

		writeJSON(w, nethttp.StatusCreated, map[string]any{"binding": binding})
	})
}

func (h *AdminBindingsHandler) Unbind() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		bindingID := r.PathValue("bindingID")

		hostID, err := h.store.GetBindingHostID(r.Context(), bindingID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, nethttp.StatusNotFound, map[string]string{"error": "binding not found"})
				return
			}
			h.logger.Error("get binding host id failed", "binding_id", bindingID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "check binding failed"})
			return
		}

		host, err := h.store.GetHost(r.Context(), hostID)
		if err != nil {
			h.logger.Error("get host for unbind check failed", "host_id", hostID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "check host status failed"})
			return
		}

		if host.Status == "running" {
			writeJSON(w, nethttp.StatusConflict, map[string]string{"error": "cannot unbind egress IP from running host, stop host first"})
			return
		}

		if err := h.store.UnbindEgressIPFromHost(r.Context(), bindingID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, nethttp.StatusNotFound, map[string]string{"error": "binding not found"})
				return
			}
			h.logger.Error("unbind egress ip failed", "binding_id", bindingID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "unbind egress ip failed"})
			return
		}

		if h.events != nil {
			if _, err := h.events.RecordEvent(r.Context(), repository.RecordEventParams{
				HostID:   &hostID,
				Level:    "info",
				Type:     "admin.binding.deleted",
				Message:  "管理员删除出口 IP 绑定",
				Metadata: map[string]any{"operator": "admin", "binding_id": bindingID},
			}); err != nil {
				h.logger.Error("record event failed", "type", "admin.binding.deleted", "error", err)
			}
		}

		w.WriteHeader(nethttp.StatusNoContent)
	})
}
