package http

import (
	"context"
	"fmt"
	"log/slog"
	nethttp "net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

type vncHostStore interface {
	GetHost(ctx context.Context, hostID string) (repository.Host, error)
}

type UserVNCProxyHandler struct {
	logger *slog.Logger
	store  vncHostStore
}

func NewUserVNCProxyHandler(logger *slog.Logger, store vncHostStore) *UserVNCProxyHandler {
	return &UserVNCProxyHandler{logger: logger, store: store}
}

func (h *UserVNCProxyHandler) ServeHTTP(w nethttp.ResponseWriter, r *nethttp.Request) {
	userID := UserIDFromContext(r.Context())
	role := RoleFromContext(r.Context())
	hostID := r.PathValue("hostID")

	if userID == "" {
		nethttp.Error(w, "unauthorized", nethttp.StatusUnauthorized)
		return
	}

	host, err := h.store.GetHost(r.Context(), hostID)
	if err != nil {
		nethttp.Error(w, "host not found", nethttp.StatusNotFound)
		return
	}

	if role != "admin" && host.UserID != userID {
		nethttp.Error(w, "forbidden", nethttp.StatusForbidden)
		return
	}

	if host.Status != "running" {
		nethttp.Error(w, "host is not running", nethttp.StatusConflict)
		return
	}

	containerName := fmt.Sprintf("cloudproxy-%s", hostID)
	containerIP, err := getContainerIP(r.Context(), containerName)
	if err != nil {
		h.logger.Error("get container IP failed", "container", containerName, "error", err)
		nethttp.Error(w, "cannot reach container", nethttp.StatusServiceUnavailable)
		return
	}

	// WebSocket(RFB)与普通 HTTP 统一交给标准库 ReverseProxy 处理，
	// 由其原生的 Connection: Upgrade 透传完成握手（详见 admin_vnc_proxy.go 的说明）。
	target, _ := url.Parse(fmt.Sprintf("http://%s:6080", containerIP))
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Director = func(req *nethttp.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.Host = target.Host

		originalPath := req.URL.Path
		if idx := strings.Index(originalPath, "/vnc/"); idx != -1 {
			req.URL.Path = "/" + originalPath[idx+5:]
		} else if strings.HasSuffix(originalPath, "/vnc") {
			req.URL.Path = "/"
		}
	}
	proxy.ErrorHandler = func(rw nethttp.ResponseWriter, req *nethttp.Request, err error) {
		h.logger.Error("user vnc proxy error", "error", err)
		rw.WriteHeader(nethttp.StatusBadGateway)
	}

	proxy.ServeHTTP(w, r)
}
