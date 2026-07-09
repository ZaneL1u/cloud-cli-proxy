package http

import (
	"context"
	"fmt"
	"log/slog"
	nethttp "net/http"
	"net/http/httputil"
	"net/url"
	"os/exec"
	"strings"
)

type AdminVNCProxyHandler struct {
	logger *slog.Logger
	store  AdminHostStore
}

func NewAdminVNCProxyHandler(logger *slog.Logger, store AdminHostStore) *AdminVNCProxyHandler {
	return &AdminVNCProxyHandler{logger: logger, store: store}
}

func (h *AdminVNCProxyHandler) ServeHTTP(w nethttp.ResponseWriter, r *nethttp.Request) {
	hostID := r.PathValue("hostID")
	if hostID == "" {
		nethttp.Error(w, "missing host ID", nethttp.StatusBadRequest)
		return
	}

	host, err := h.store.GetHost(r.Context(), hostID)
	if err != nil {
		nethttp.Error(w, "host not found", nethttp.StatusNotFound)
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

	// WebSocket(RFB)与普通 HTTP 统一交给标准库 ReverseProxy。
	// ReverseProxy 自 Go 1.12 起原生支持 Connection: Upgrade 透传，会完整、正确地
	// 转发 Sec-WebSocket-* 握手头；早前手写的 proxyWebSocket 逐字段写裸连接且不校验
	// 写入错误，会在中途丢失 Sec-WebSocket-Key，导致 KasmVNC 无法升级、随机 1006，已移除。
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
		h.logger.Error("vnc proxy error", "error", err)
		rw.WriteHeader(nethttp.StatusBadGateway)
	}

	proxy.ServeHTTP(w, r)
}

func getContainerIP(ctx context.Context, containerName string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", "inspect", "-f",
		"{{range .NetworkSettings.Networks}}{{.IPAddress}} {{end}}", containerName)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("docker inspect: %w", err)
	}
	ips := strings.Fields(strings.TrimSpace(string(out)))
	if len(ips) == 0 {
		return "", fmt.Errorf("no IP found for container %s", containerName)
	}
	// Use the last IP — typically the isolated network, not the default bridge
	return ips[len(ips)-1], nil
}
