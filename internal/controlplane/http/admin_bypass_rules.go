package http

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	nethttp "net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

// AdminBypassRuleStore 聚合 Rule CRUD 所需 Repository 方法子集。
type AdminBypassRuleStore interface {
	ListBypassRules(context.Context, *string) ([]repository.BypassRule, error)
	GetBypassRuleByID(context.Context, string) (repository.BypassRule, error)
	CreateBypassRule(context.Context, repository.CreateBypassRuleParams) (repository.BypassRule, error)
	UpdateBypassRule(context.Context, string, repository.UpdateBypassRuleParams) (repository.BypassRule, error)
	DeleteBypassRule(context.Context, string) error
	InsertBypassAuditLog(context.Context, repository.InsertBypassAuditLogParams) (string, error)
}

// AdminBypassProxyIPProvider 仅暴露 ListEgressIPs，让 rule handler 拿到 proxy IP
// 列表做 BYPASS_RULE_CONFLICT_PROXY 检查。运行时由 AdminEgressIPStore 直接满足。
type AdminBypassProxyIPProvider interface {
	ListEgressIPs(context.Context) ([]repository.EgressIP, error)
}

type AdminBypassRulesHandler struct {
	logger *slog.Logger
	store  AdminBypassRuleStore
	proxy  AdminBypassProxyIPProvider
	events EventRecorder
}

func NewAdminBypassRulesHandler(
	logger *slog.Logger,
	store AdminBypassRuleStore,
	proxy AdminBypassProxyIPProvider,
	events EventRecorder,
) *AdminBypassRulesHandler {
	return &AdminBypassRulesHandler{logger: logger, store: store, proxy: proxy, events: events}
}

func (h *AdminBypassRulesHandler) List() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		var hostIDPtr *string
		if hid := strings.TrimSpace(r.URL.Query().Get("host_id")); hid != "" {
			hostIDPtr = &hid
		}
		rules, err := h.store.ListBypassRules(r.Context(), hostIDPtr)
		if err != nil {
			h.logger.Error("list bypass rules failed", "host_id", hostIDPtr, "error", err)
			writeBypassError(w, nethttp.StatusInternalServerError, "INTERNAL", "list bypass rules failed")
			return
		}
		writeJSON(w, nethttp.StatusOK, map[string]any{"rules": rules})
	})
}

type bypassRuleRequest struct {
	Scope        string `json:"scope,omitempty"`
	HostID       string `json:"host_id,omitempty"`
	RuleType     string `json:"rule_type,omitempty"`
	Value        string `json:"value,omitempty"`
	Note         string `json:"note,omitempty"`
	Port         string `json:"port,omitempty"`
	ConfirmRisky bool   `json:"confirm_risky,omitempty"`
	IsRisky      *bool  `json:"is_risky,omitempty"`
}

// loadProxyIPs 拿到当前所有 EgressIP 的 ip_address 列表。
// 失败仅记日志并返回空，让护栏退化为「不检查 proxy 冲突」而非直接 500。
func (h *AdminBypassRulesHandler) loadProxyIPs(ctx context.Context) []string {
	if h.proxy == nil {
		return nil
	}
	ips, err := h.proxy.ListEgressIPs(ctx)
	if err != nil {
		h.logger.Warn("list egress ips for bypass guard failed", "error", err)
		return nil
	}
	out := make([]string, 0, len(ips))
	for _, ip := range ips {
		out = append(out, ip.IPAddress)
	}
	return out
}

func (h *AdminBypassRulesHandler) Create() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		var req bypassRuleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeBypassError(w, nethttp.StatusBadRequest, ErrCodeBypassInvalidRequest, "invalid request body")
			return
		}
		req.Scope = strings.TrimSpace(req.Scope)
		req.RuleType = strings.TrimSpace(req.RuleType)
		req.Value = strings.TrimSpace(req.Value)
		req.HostID = strings.TrimSpace(req.HostID)

		if req.Scope != "global" && req.Scope != "host" {
			writeBypassError(w, nethttp.StatusBadRequest, ErrCodeBypassInvalidRequest, "scope must be global or host")
			return
		}
		if req.Scope == "host" && req.HostID == "" {
			writeBypassError(w, nethttp.StatusBadRequest, ErrCodeBypassInvalidRequest, "host_id is required for scope=host")
			return
		}

		proxyIPs := h.loadProxyIPs(r.Context())

		currentCount := 0
		if req.Scope == "host" {
			hostID := req.HostID
			existing, err := h.store.ListBypassRules(r.Context(), &hostID)
			if err != nil {
				h.logger.Error("list bypass rules for limit check failed", "host_id", hostID, "error", err)
				writeBypassError(w, nethttp.StatusInternalServerError, "INTERNAL", "list rules failed")
				return
			}
			for _, e := range existing {
				if e.Scope == "host" {
					currentCount++
				}
			}
		}

		isRisky, code, verr := ValidateBypassRule(req.RuleType, req.Value, req.Port, req.ConfirmRisky, proxyIPs, currentCount)
		if verr != nil {
			status := nethttp.StatusUnprocessableEntity
			switch code {
			case "":
				status = nethttp.StatusBadRequest
				code = ErrCodeBypassInvalidRequest
			case ErrCodeBypassKeywordTooShort:
				// 软拦截 → 400（前端判断 + confirm_risky 重试）
				status = nethttp.StatusBadRequest
			}
			h.logger.Info("create bypass rule rejected", "code", code, "err", verr)
			writeBypassError(w, status, code, verr.Error())
			return
		}

		var hostPtr *string
		if req.Scope == "host" {
			hp := req.HostID
			hostPtr = &hp
		}

		created, err := h.store.CreateBypassRule(r.Context(), repository.CreateBypassRuleParams{
			Scope:    req.Scope,
			HostID:   hostPtr,
			RuleType: req.RuleType,
			Value:    req.Value,
			Note:     req.Note,
			IsRisky:  isRisky,
		})
		if err != nil {
			h.logger.Error("create bypass rule failed", "error", err)
			writeBypassError(w, nethttp.StatusInternalServerError, "INTERNAL", "create bypass rule failed")
			return
		}

		note := ""
		if req.RuleType == "domain_keyword" && len(req.Value) < 4 && req.ConfirmRisky {
			// WARN-1：记录 confirm_risky 选择，方便审计回查。
			note = "confirm_risky_accepted"
		}
		writeBypassAuditLog(r.Context(), h.logger, h.store, h.events, r, "create_rule", "rule", &created.ID, nil, created, note)
		writeJSON(w, nethttp.StatusCreated, map[string]any{"rule": created})
	})
}

func (h *AdminBypassRulesHandler) Update() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		ruleID := r.PathValue("ruleID")
		before, err := h.store.GetBypassRuleByID(r.Context(), ruleID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeBypassError(w, nethttp.StatusNotFound, ErrCodeBypassRuleNotFound, "rule not found")
				return
			}
			h.logger.Error("get bypass rule for update failed", "rule_id", ruleID, "error", err)
			writeBypassError(w, nethttp.StatusInternalServerError, "INTERNAL", "get bypass rule failed")
			return
		}

		var req bypassRuleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeBypassError(w, nethttp.StatusBadRequest, ErrCodeBypassInvalidRequest, "invalid request body")
			return
		}

		// 仅 value / note / is_risky 可更新；rule_type / scope / host_id 不可改。
		params := repository.UpdateBypassRuleParams{}
		newValue := before.Value
		if req.Value != "" {
			newValue = strings.TrimSpace(req.Value)
			params.Value = &newValue
		}
		if req.Note != "" {
			n := req.Note
			params.Note = &n
		}

		// 若 value 改变，跑护栏（用原 rule_type）。
		if params.Value != nil {
			proxyIPs := h.loadProxyIPs(r.Context())
			isRisky, code, verr := ValidateBypassRule(before.RuleType, newValue, req.Port, req.ConfirmRisky, proxyIPs, 0)
			if verr != nil {
				status := nethttp.StatusUnprocessableEntity
				switch code {
				case "":
					status = nethttp.StatusBadRequest
					code = ErrCodeBypassInvalidRequest
				case ErrCodeBypassKeywordTooShort:
					status = nethttp.StatusBadRequest
				}
				h.logger.Info("update bypass rule rejected", "rule_id", ruleID, "code", code, "err", verr)
				writeBypassError(w, status, code, verr.Error())
				return
			}
			// is_risky 跟随 value 校验结果；若 caller 显式传 is_risky，以 caller 为准。
			if req.IsRisky != nil {
				params.IsRisky = req.IsRisky
			} else {
				params.IsRisky = &isRisky
			}
		} else if req.IsRisky != nil {
			params.IsRisky = req.IsRisky
		}

		after, err := h.store.UpdateBypassRule(r.Context(), ruleID, params)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeBypassError(w, nethttp.StatusNotFound, ErrCodeBypassRuleNotFound, "rule not found")
				return
			}
			h.logger.Error("update bypass rule failed", "rule_id", ruleID, "error", err)
			writeBypassError(w, nethttp.StatusInternalServerError, "INTERNAL", "update bypass rule failed")
			return
		}

		writeBypassAuditLog(r.Context(), h.logger, h.store, h.events, r, "update_rule", "rule", &after.ID, before, after, "")
		writeJSON(w, nethttp.StatusOK, map[string]any{"rule": after})
	})
}

func (h *AdminBypassRulesHandler) Delete() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		ruleID := r.PathValue("ruleID")
		before, err := h.store.GetBypassRuleByID(r.Context(), ruleID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeBypassError(w, nethttp.StatusNotFound, ErrCodeBypassRuleNotFound, "rule not found")
				return
			}
			h.logger.Error("get bypass rule for delete failed", "rule_id", ruleID, "error", err)
			writeBypassError(w, nethttp.StatusInternalServerError, "INTERNAL", "get bypass rule failed")
			return
		}

		if err := h.store.DeleteBypassRule(r.Context(), ruleID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeBypassError(w, nethttp.StatusNotFound, ErrCodeBypassRuleNotFound, "rule not found")
				return
			}
			h.logger.Error("delete bypass rule failed", "rule_id", ruleID, "error", err)
			writeBypassError(w, nethttp.StatusInternalServerError, "INTERNAL", "delete bypass rule failed")
			return
		}

		writeBypassAuditLog(r.Context(), h.logger, h.store, h.events, r, "delete_rule", "rule", &before.ID, before, nil, "")
		w.WriteHeader(nethttp.StatusNoContent)
	})
}

// Validate dry-run 端点：不落库、不写 audit、不调 CreateBypassRule，
// 仅返回 ValidateBypassRule 的结果，供前端「即时校验」UI 使用。
func (h *AdminBypassRulesHandler) Validate() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		var req bypassRuleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeBypassError(w, nethttp.StatusBadRequest, ErrCodeBypassInvalidRequest, "invalid request body")
			return
		}
		req.RuleType = strings.TrimSpace(req.RuleType)
		req.Value = strings.TrimSpace(req.Value)

		proxyIPs := h.loadProxyIPs(r.Context())

		currentCount := 0
		if req.Scope == "host" && req.HostID != "" {
			hostID := req.HostID
			if existing, err := h.store.ListBypassRules(r.Context(), &hostID); err == nil {
				for _, e := range existing {
					if e.Scope == "host" {
						currentCount++
					}
				}
			}
		}

		isRisky, code, verr := ValidateBypassRule(req.RuleType, req.Value, req.Port, req.ConfirmRisky, proxyIPs, currentCount)
		if verr != nil {
			status := nethttp.StatusUnprocessableEntity
			switch code {
			case "":
				status = nethttp.StatusBadRequest
				code = ErrCodeBypassInvalidRequest
			case ErrCodeBypassKeywordTooShort:
				status = nethttp.StatusBadRequest
			}
			writeJSON(w, status, map[string]any{
				"valid":    false,
				"is_risky": isRisky,
				"code":     code,
				"message":  verr.Error(),
			})
			return
		}
		writeJSON(w, nethttp.StatusOK, map[string]any{
			"valid":    true,
			"is_risky": isRisky,
			"code":     "",
			"message":  "",
		})
	})
}
