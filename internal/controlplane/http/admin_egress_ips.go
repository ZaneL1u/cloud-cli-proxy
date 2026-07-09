package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	nethttp "net/http"
	"strings"

	"database/sql"

	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

type AdminEgressIPStore interface {
	ListEgressIPs(context.Context) ([]repository.EgressIP, error)
	GetEgressIP(context.Context, string) (repository.EgressIP, error)
	CreateEgressIP(context.Context, repository.CreateEgressIPParams) (repository.EgressIP, error)
	UpdateEgressIP(context.Context, string, repository.UpdateEgressIPParams) (repository.EgressIP, error)
	UpdateEgressIPDetectedAddress(ctx context.Context, egressIPID string, detectedIP string) error
	DeleteEgressIP(context.Context, string) error
}

type AdminEgressIPsHandler struct {
	logger *slog.Logger
	store  AdminEgressIPStore
	events EventRecorder
}

func NewAdminEgressIPsHandler(logger *slog.Logger, store AdminEgressIPStore, events EventRecorder) *AdminEgressIPsHandler {
	return &AdminEgressIPsHandler{logger: logger, store: store, events: events}
}

var allowedOutboundTypes = map[string]bool{
	"socks": true, "vmess": true, "vless": true, "shadowsocks": true, "trojan": true, "http": true,
}

func validateProxyConfig(raw json.RawMessage) error {
	if len(raw) == 0 {
		return fmt.Errorf("proxy_config is required for proxy tunnel type")
	}

	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return fmt.Errorf("proxy_config is not valid JSON: %w", err)
	}

	outboundType, _ := parsed["type"].(string)
	if !allowedOutboundTypes[outboundType] {
		return fmt.Errorf("unsupported outbound type %q, allowed: socks, vmess, vless, shadowsocks, trojan, http", outboundType)
	}

	server, _ := parsed["server"].(string)
	if server == "" {
		return fmt.Errorf("proxy_config.server is required")
	}

	port, _ := parsed["server_port"].(float64)
	if port <= 0 || port > 65535 {
		return fmt.Errorf("proxy_config.server_port must be a positive integer (1-65535)")
	}

	switch outboundType {
	case "vmess", "vless":
		if uuid, _ := parsed["uuid"].(string); uuid == "" {
			return fmt.Errorf("proxy_config.uuid is required for %s", outboundType)
		}
	case "shadowsocks":
		if method, _ := parsed["method"].(string); method == "" {
			return fmt.Errorf("proxy_config.method is required for shadowsocks")
		}
		if password, _ := parsed["password"].(string); password == "" {
			return fmt.Errorf("proxy_config.password is required for shadowsocks")
		}
	case "trojan":
		if password, _ := parsed["password"].(string); password == "" {
			return fmt.Errorf("proxy_config.password is required for trojan")
		}
	}

	return nil
}

func sanitizeProxyConfig(raw json.RawMessage) json.RawMessage {
	if raw == nil {
		return nil
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil
	}
	if _, ok := parsed["password"]; ok {
		parsed["password"] = "***"
	}
	sanitized, _ := json.Marshal(parsed)
	return sanitized
}

func sanitizeEgressIP(ip *repository.EgressIP) {
	ip.ProxyConfig = sanitizeProxyConfig(ip.ProxyConfig)
}

func mergeProxyPassword(ctx context.Context, store AdminEgressIPStore, ipID string, incoming json.RawMessage) json.RawMessage {
	var newCfg map[string]any
	if err := json.Unmarshal(incoming, &newCfg); err != nil {
		return incoming
	}

	pwd, hasPwd := newCfg["password"]
	if !hasPwd || pwd == "***" {
		existing, err := store.GetEgressIP(ctx, ipID)
		if err != nil || existing.ProxyConfig == nil {
			return incoming
		}
		var oldCfg map[string]any
		if err := json.Unmarshal(existing.ProxyConfig, &oldCfg); err != nil {
			return incoming
		}
		if origPwd, ok := oldCfg["password"]; ok {
			newCfg["password"] = origPwd
			merged, _ := json.Marshal(newCfg)
			return merged
		}
	}
	return incoming
}

func (h *AdminEgressIPsHandler) List() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		ips, err := h.store.ListEgressIPs(r.Context())
		if err != nil {
			h.logger.Error("list egress ips failed", "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "list egress ips failed"})
			return
		}
		for i := range ips {
			sanitizeEgressIP(&ips[i])
		}
		writeJSON(w, nethttp.StatusOK, map[string]any{"egress_ips": ips})
	})
}

func (h *AdminEgressIPsHandler) Get() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		ipID := r.PathValue("ipID")
		ip, err := h.store.GetEgressIP(r.Context(), ipID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, nethttp.StatusNotFound, map[string]string{"error": "egress ip not found"})
				return
			}
			h.logger.Error("get egress ip failed", "ip_id", ipID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "get egress ip failed"})
			return
		}
		sanitizeEgressIP(&ip)
		writeJSON(w, nethttp.StatusOK, map[string]any{"egress_ip": ip})
	})
}

type createEgressIPRequest struct {
	Label       string          `json:"label"`
	IPAddress   string          `json:"ip_address"`
	Provider    string          `json:"provider"`
	ProxyConfig json.RawMessage `json:"proxy_config,omitempty"`
}

func (h *AdminEgressIPsHandler) Create() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		var req createEgressIPRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}

		req.Label = strings.TrimSpace(req.Label)
		if req.Label == "" {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "label is required"})
			return
		}
		req.IPAddress = strings.TrimSpace(req.IPAddress)
		if req.IPAddress != "" && net.ParseIP(req.IPAddress) == nil {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "invalid ip address"})
			return
		}
		if req.Provider == "" {
			req.Provider = "manual"
		}

		if err := validateProxyConfig(req.ProxyConfig); err != nil {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		ip, err := h.store.CreateEgressIP(r.Context(), repository.CreateEgressIPParams{
			Label:       req.Label,
			IPAddress:   req.IPAddress,
			Provider:    req.Provider,
			ProxyConfig: req.ProxyConfig,
		})
		if err != nil {
			errMsg := strings.ToLower(err.Error())
			if strings.Contains(errMsg, "unique") || strings.Contains(errMsg, "constraint failed") || strings.Contains(errMsg, "duplicate") {
				writeJSON(w, nethttp.StatusConflict, map[string]string{"error": "label already exists"})
				return
			}
			h.logger.Error("create egress ip failed", "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "create egress ip failed"})
			return
		}

		if h.events != nil {
			if _, err := h.events.RecordEvent(r.Context(), repository.RecordEventParams{
				Level:    "info",
				Type:     "admin.egress_ip.created",
				Message:  "管理员创建出口 IP 资源",
				Metadata: map[string]any{"operator": "admin", "egress_ip_id": ip.ID, "label": ip.Label, "ip_address": ip.IPAddress},
			}); err != nil {
				h.logger.Error("record event failed", "type", "admin.egress_ip.created", "error", err)
			}
		}

		sanitizeEgressIP(&ip)
		writeJSON(w, nethttp.StatusCreated, map[string]any{"egress_ip": ip})
	})
}

type updateEgressIPRequest struct {
	Label       string          `json:"label"`
	IPAddress   string          `json:"ip_address"`
	Provider    string          `json:"provider"`
	Status      string          `json:"status"`
	ProxyConfig json.RawMessage `json:"proxy_config,omitempty"`
}

func (h *AdminEgressIPsHandler) Update() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		ipID := r.PathValue("ipID")

		var req updateEgressIPRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}

		if req.Status != "" && req.Status != "available" && req.Status != "disabled" {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "status must be available or disabled"})
			return
		}

		req.ProxyConfig = mergeProxyPassword(r.Context(), h.store, ipID, req.ProxyConfig)
		if err := validateProxyConfig(req.ProxyConfig); err != nil {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		ip, err := h.store.UpdateEgressIP(r.Context(), ipID, repository.UpdateEgressIPParams{
			Label:       req.Label,
			IPAddress:   req.IPAddress,
			Provider:    req.Provider,
			Status:      req.Status,
			ProxyConfig: req.ProxyConfig,
		})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, nethttp.StatusNotFound, map[string]string{"error": "egress ip not found"})
				return
			}
			errMsg := strings.ToLower(err.Error())
			if strings.Contains(errMsg, "unique") || strings.Contains(errMsg, "constraint failed") || strings.Contains(errMsg, "duplicate") {
				writeJSON(w, nethttp.StatusConflict, map[string]string{"error": "label already exists"})
				return
			}
			h.logger.Error("update egress ip failed", "ip_id", ipID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "update egress ip failed"})
			return
		}

		if h.events != nil {
			if _, err := h.events.RecordEvent(r.Context(), repository.RecordEventParams{
				Level:    "info",
				Type:     "admin.egress_ip.updated",
				Message:  "管理员更新出口 IP 资源",
				Metadata: map[string]any{"operator": "admin", "egress_ip_id": ipID},
			}); err != nil {
				h.logger.Error("record event failed", "type", "admin.egress_ip.updated", "error", err)
			}
		}

		sanitizeEgressIP(&ip)
		writeJSON(w, nethttp.StatusOK, map[string]any{"egress_ip": ip})
	})
}

func (h *AdminEgressIPsHandler) Delete() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		ipID := r.PathValue("ipID")

		if err := h.store.DeleteEgressIP(r.Context(), ipID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, nethttp.StatusNotFound, map[string]string{"error": "egress ip not found"})
				return
			}
			if strings.Contains(err.Error(), "violates foreign key") || strings.Contains(err.Error(), "restrict") {
				writeJSON(w, nethttp.StatusConflict, map[string]string{"error": "egress IP is bound to a host, unbind first"})
				return
			}
			h.logger.Error("delete egress ip failed", "ip_id", ipID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "delete egress ip failed"})
			return
		}

		if h.events != nil {
			if _, err := h.events.RecordEvent(r.Context(), repository.RecordEventParams{
				Level:    "info",
				Type:     "admin.egress_ip.deleted",
				Message:  "管理员删除出口 IP 资源",
				Metadata: map[string]any{"operator": "admin", "egress_ip_id": ipID},
			}); err != nil {
				h.logger.Error("record event failed", "type", "admin.egress_ip.deleted", "error", err)
			}
		}

		w.WriteHeader(nethttp.StatusNoContent)
	})
}
