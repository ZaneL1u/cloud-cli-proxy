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

// AdminBypassPresetStore 聚合 Preset CRUD 所需 Repository 方法子集。
// 测试通过 stub 实现此接口；运行时由 repository.Repository 实现。
type AdminBypassPresetStore interface {
	ListBypassPresets(context.Context) ([]repository.BypassPreset, error)
	GetBypassPresetByID(context.Context, string) (repository.BypassPreset, error)
	CreateBypassPreset(context.Context, repository.CreateBypassPresetParams) (repository.BypassPreset, error)
	UpdateBypassPreset(context.Context, string, repository.UpdateBypassPresetParams) (repository.BypassPreset, error)
	DeleteBypassPreset(context.Context, string) error
	InsertBypassAuditLog(context.Context, repository.InsertBypassAuditLogParams) (string, error)
}

type AdminBypassPresetsHandler struct {
	logger *slog.Logger
	store  AdminBypassPresetStore
	events EventRecorder
}

func NewAdminBypassPresetsHandler(logger *slog.Logger, store AdminBypassPresetStore, events EventRecorder) *AdminBypassPresetsHandler {
	return &AdminBypassPresetsHandler{logger: logger, store: store, events: events}
}

// writeBypassError 统一 {"code": "...", "message": "..."} 错误响应。
func writeBypassError(w nethttp.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{"code": code, "message": message})
}

func (h *AdminBypassPresetsHandler) List() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		presets, err := h.store.ListBypassPresets(r.Context())
		if err != nil {
			h.logger.Error("list bypass presets failed", "error", err)
			writeBypassError(w, nethttp.StatusInternalServerError, "INTERNAL", "list bypass presets failed")
			return
		}
		writeJSON(w, nethttp.StatusOK, map[string]any{"presets": presets})
	})
}

func (h *AdminBypassPresetsHandler) Get() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		presetID := r.PathValue("presetID")
		p, err := h.store.GetBypassPresetByID(r.Context(), presetID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeBypassError(w, nethttp.StatusNotFound, ErrCodeBypassPresetNotFound, "preset not found")
				return
			}
			h.logger.Error("get bypass preset failed", "preset_id", presetID, "error", err)
			writeBypassError(w, nethttp.StatusInternalServerError, "INTERNAL", "get bypass preset failed")
			return
		}
		writeJSON(w, nethttp.StatusOK, map[string]any{"preset": p})
	})
}

type bypassPresetRuleReq struct {
	RuleType string `json:"rule_type"`
	Value    string `json:"value"`
	Note     string `json:"note,omitempty"`
}

type createBypassPresetRequest struct {
	Slug        string                `json:"slug"`
	Name        string                `json:"name"`
	Description string                `json:"description"`
	IsForceOn   bool                  `json:"is_force_on"`
	IsActive    bool                  `json:"is_active"`
	Rules       []bypassPresetRuleReq `json:"rules"`
}

func (h *AdminBypassPresetsHandler) Create() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		var req createBypassPresetRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeBypassError(w, nethttp.StatusBadRequest, ErrCodeBypassInvalidRequest, "invalid request body")
			return
		}
		req.Slug = strings.TrimSpace(req.Slug)
		req.Name = strings.TrimSpace(req.Name)
		if req.Slug == "" || req.Name == "" {
			writeBypassError(w, nethttp.StatusBadRequest, ErrCodeBypassInvalidRequest, "slug and name are required")
			return
		}

		// preset 内置规则只做基本格式校验：无 proxy 冲突 / host limit 概念。
		rules := make([]repository.BypassPresetRule, 0, len(req.Rules))
		for i, r := range req.Rules {
			r.RuleType = strings.TrimSpace(r.RuleType)
			r.Value = strings.TrimSpace(r.Value)
			_, code, verr := ValidateBypassRule(r.RuleType, r.Value, "", false, nil, 0)
			if verr != nil {
				status := nethttp.StatusUnprocessableEntity
				if code == "" {
					status = nethttp.StatusBadRequest
					code = ErrCodeBypassInvalidRequest
				}
				h.logger.Info("create bypass preset rejected", "slug", req.Slug, "index", i, "code", code, "err", verr)
				writeBypassError(w, status, code, verr.Error())
				return
			}
			rules = append(rules, repository.BypassPresetRule{RuleType: r.RuleType, Value: r.Value, Note: r.Note})
		}

		p, err := h.store.CreateBypassPreset(r.Context(), repository.CreateBypassPresetParams{
			Slug:        req.Slug,
			Name:        req.Name,
			Description: req.Description,
			IsForceOn:   req.IsForceOn,
			IsActive:    req.IsActive,
			Rules:       rules,
		})
		if err != nil {
			if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
				writeBypassError(w, nethttp.StatusConflict, ErrCodeBypassInvalidRequest, "preset slug already exists")
				return
			}
			h.logger.Error("create bypass preset failed", "error", err)
			writeBypassError(w, nethttp.StatusInternalServerError, "INTERNAL", "create bypass preset failed")
			return
		}

		writeBypassAuditLog(r.Context(), h.logger, h.store, h.events, r, "create_preset", "preset", &p.ID, nil, p, "")
		writeJSON(w, nethttp.StatusCreated, map[string]any{"preset": p})
	})
}

type updateBypassPresetRequest struct {
	Name        *string                `json:"name,omitempty"`
	Description *string                `json:"description,omitempty"`
	IsForceOn   *bool                  `json:"is_force_on,omitempty"`
	IsActive    *bool                  `json:"is_active,omitempty"`
	Rules       *[]bypassPresetRuleReq `json:"rules,omitempty"`
}

func (h *AdminBypassPresetsHandler) Update() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		presetID := r.PathValue("presetID")

		// before 快照（用于 audit log diff）。
		before, getErr := h.store.GetBypassPresetByID(r.Context(), presetID)
		if getErr != nil {
			if errors.Is(getErr, sql.ErrNoRows) {
				writeBypassError(w, nethttp.StatusNotFound, ErrCodeBypassPresetNotFound, "preset not found")
				return
			}
			h.logger.Error("get bypass preset for update failed", "preset_id", presetID, "error", getErr)
			writeBypassError(w, nethttp.StatusInternalServerError, "INTERNAL", "get bypass preset failed")
			return
		}

		var req updateBypassPresetRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeBypassError(w, nethttp.StatusBadRequest, ErrCodeBypassInvalidRequest, "invalid request body")
			return
		}

		params := repository.UpdateBypassPresetParams{
			Name:        req.Name,
			Description: req.Description,
			IsForceOn:   req.IsForceOn,
			IsActive:    req.IsActive,
		}
		if req.Rules != nil {
			converted := make([]repository.BypassPresetRule, 0, len(*req.Rules))
			for i, rr := range *req.Rules {
				rr.RuleType = strings.TrimSpace(rr.RuleType)
				rr.Value = strings.TrimSpace(rr.Value)
				_, code, verr := ValidateBypassRule(rr.RuleType, rr.Value, "", false, nil, 0)
				if verr != nil {
					status := nethttp.StatusUnprocessableEntity
					if code == "" {
						status = nethttp.StatusBadRequest
						code = ErrCodeBypassInvalidRequest
					}
					h.logger.Info("update bypass preset rejected", "preset_id", presetID, "index", i, "code", code, "err", verr)
					writeBypassError(w, status, code, verr.Error())
					return
				}
				converted = append(converted, repository.BypassPresetRule{RuleType: rr.RuleType, Value: rr.Value, Note: rr.Note})
			}
			params.Rules = &converted
		}

		after, err := h.store.UpdateBypassPreset(r.Context(), presetID, params)
		if err != nil {
			if errors.Is(err, repository.ErrSystemBypassPresetImmutable) {
				writeBypassError(w, nethttp.StatusForbidden, ErrCodeBypassPresetImmutable, "system preset is immutable")
				return
			}
			if errors.Is(err, sql.ErrNoRows) {
				writeBypassError(w, nethttp.StatusNotFound, ErrCodeBypassPresetNotFound, "preset not found")
				return
			}
			h.logger.Error("update bypass preset failed", "preset_id", presetID, "error", err)
			writeBypassError(w, nethttp.StatusInternalServerError, "INTERNAL", "update bypass preset failed")
			return
		}

		writeBypassAuditLog(r.Context(), h.logger, h.store, h.events, r, "update_preset", "preset", &after.ID, before, after, "")
		writeJSON(w, nethttp.StatusOK, map[string]any{"preset": after})
	})
}

func (h *AdminBypassPresetsHandler) Delete() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		presetID := r.PathValue("presetID")

		// before 快照（用于 audit log diff）。
		before, getErr := h.store.GetBypassPresetByID(r.Context(), presetID)
		if getErr != nil {
			if errors.Is(getErr, sql.ErrNoRows) {
				writeBypassError(w, nethttp.StatusNotFound, ErrCodeBypassPresetNotFound, "preset not found")
				return
			}
			h.logger.Error("get bypass preset for delete failed", "preset_id", presetID, "error", getErr)
			writeBypassError(w, nethttp.StatusInternalServerError, "INTERNAL", "get bypass preset failed")
			return
		}

		if err := h.store.DeleteBypassPreset(r.Context(), presetID); err != nil {
			if errors.Is(err, repository.ErrSystemBypassPresetImmutable) {
				writeBypassError(w, nethttp.StatusForbidden, ErrCodeBypassPresetImmutable, "system preset is immutable")
				return
			}
			if errors.Is(err, sql.ErrNoRows) {
				writeBypassError(w, nethttp.StatusNotFound, ErrCodeBypassPresetNotFound, "preset not found")
				return
			}
			h.logger.Error("delete bypass preset failed", "preset_id", presetID, "error", err)
			writeBypassError(w, nethttp.StatusInternalServerError, "INTERNAL", "delete bypass preset failed")
			return
		}

		writeBypassAuditLog(r.Context(), h.logger, h.store, h.events, r, "delete_preset", "preset", &before.ID, before, nil, "")
		w.WriteHeader(nethttp.StatusNoContent)
	})
}
