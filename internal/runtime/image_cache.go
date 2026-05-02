package runtime

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// ImageCacheStatus 描述当前镜像缓存的状态。
type ImageCacheStatus struct {
	ImageName        string    `json:"image_name"`
	ImageVersion     string    `json:"image_version"`
	LocalDigest      string    `json:"local_digest"`
	LocalCreated     string    `json:"local_created"`
	LastRefreshAt    time.Time `json:"last_refresh_at"`
	LastRefreshError string    `json:"last_refresh_error,omitempty"`
	Refreshing       bool      `json:"refreshing"`
}

// ImageCache 管理 image.lock 中配置的镜像缓存状态。
// 通过定期 docker pull 刷新本地镜像，使 GetImageInfo 等比较逻辑基于真实最新镜像。
type ImageCache struct {
	mu       sync.RWMutex
	status   ImageCacheStatus
	logger   *slog.Logger
	specPath string
}

// NewImageCache 创建镜像缓存管理器。
func NewImageCache(logger *slog.Logger, specPath string) *ImageCache {
	if specPath == "" {
		specPath = DefaultImageLockPath
	}
	return &ImageCache{
		logger:   logger,
		specPath: specPath,
	}
}

// GetStatus 返回当前镜像缓存状态的只读副本。
func (c *ImageCache) GetStatus() ImageCacheStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.status
}

// Refresh 执行 docker pull 刷新本地镜像缓存，并更新状态。
// 即使 pull 失败（如 registry 不可达），也会更新 last_refresh_error，不会 panic。
func (c *ImageCache) Refresh(ctx context.Context) error {
	spec, err := LoadRuntimeSpec(c.specPath)
	if err != nil {
		return fmt.Errorf("load runtime spec: %w", err)
	}

	c.mu.Lock()
	c.status.ImageName = spec.ImageName
	c.status.ImageVersion = spec.ImageVersion
	c.status.Refreshing = true
	c.mu.Unlock()

	pullCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(pullCtx, "docker", "pull", spec.ImageName)
	output, err := cmd.CombinedOutput()

	c.mu.Lock()
	defer c.mu.Unlock()
	c.status.Refreshing = false
	c.status.LastRefreshAt = time.Now()

	if err != nil {
		c.status.LastRefreshError = strings.TrimSpace(string(output))
		if c.status.LastRefreshError == "" {
			c.status.LastRefreshError = err.Error()
		}
		c.logger.Warn("image cache refresh failed",
			"image", spec.ImageName,
			"error", err,
			"output", c.status.LastRefreshError)
		return fmt.Errorf("docker pull %s: %w", spec.ImageName, err)
	}

	// 刷新本地 digest
	digest, created, inspectErr := c.inspectLocal(pullCtx, spec.ImageName)
	if inspectErr != nil {
		c.logger.Warn("image cache refresh: inspect after pull failed",
			"image", spec.ImageName, "error", inspectErr)
		c.status.LocalDigest = ""
		c.status.LocalCreated = ""
	} else {
		c.status.LocalDigest = digest
		c.status.LocalCreated = created
	}

	c.status.LastRefreshError = ""
	c.logger.Info("image cache refreshed",
		"image", spec.ImageName,
		"digest", c.status.LocalDigest,
		"output", strings.TrimSpace(string(output)))
	return nil
}

// inspectLocal 查询本地镜像的 digest 和创建时间。
func (c *ImageCache) inspectLocal(ctx context.Context, imageName string) (digest, created string, err error) {
	cmd := exec.CommandContext(ctx, "docker", "inspect",
		"--format", "{{.Id}}|{{.Created}}", imageName)
	out, err := cmd.Output()
	if err != nil {
		return "", "", err
	}
	parts := strings.SplitN(strings.TrimSpace(string(out)), "|", 2)
	digest = shortImageID(parts[0])
	if len(parts) > 1 {
		created = parts[1]
	}
	return digest, created, nil
}

func shortImageID(id string) string {
	id = strings.TrimPrefix(id, "sha256:")
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
