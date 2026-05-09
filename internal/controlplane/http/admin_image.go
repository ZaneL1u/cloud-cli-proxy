package http

import (
	"context"
	"log/slog"
	nethttp "net/http"
	"time"

	"github.com/zanel1u/cloud-cli-proxy/internal/broadcast"
	"github.com/zanel1u/cloud-cli-proxy/internal/runtime"
)

// ImageCacheManager 定义镜像缓存管理器的接口。
type ImageCacheManager interface {
	GetStatus() runtime.ImageCacheStatus
	Refresh(ctx context.Context) error
}

type AdminImageHandler struct {
	logger *slog.Logger
	cache  ImageCacheManager
}

func NewAdminImageHandler(logger *slog.Logger, cache ImageCacheManager) *AdminImageHandler {
	return &AdminImageHandler{logger: logger, cache: cache}
}

// Status 返回当前镜像缓存状态。
func (h *AdminImageHandler) Status() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		status := h.cache.GetStatus()
		writeJSON(w, nethttp.StatusOK, status)
	})
}

// Refresh 手动触发镜像缓存刷新。
func (h *AdminImageHandler) Refresh() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		// 检查是否已在刷新中
		if h.cache.GetStatus().Refreshing {
			writeJSON(w, nethttp.StatusConflict, map[string]string{
				"error": "镜像刷新正在进行中，请稍后再试",
			})
			return
		}

		// 异步执行刷新，避免 HTTP 超时
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			if err := h.cache.Refresh(ctx); err != nil {
				h.logger.Warn("admin manual image refresh failed", "error", err)
			}
			broadcast.Broadcast("image-status", "update", "")
		}()

		writeJSON(w, nethttp.StatusAccepted, map[string]string{
			"status":  "accepted",
			"message": "镜像刷新已启动",
		})
	})
}
