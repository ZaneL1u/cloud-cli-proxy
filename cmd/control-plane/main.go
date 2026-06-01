package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zanel1u/cloud-cli-proxy/internal/controlplane/app"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg := app.Config{
		Addr:           envOrDefault("CONTROL_PLANE_ADDR", ":8080"),
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		AdminUsername:   envOrDefault("ADMIN_USERNAME", "admin"),
		AdminPassword:  os.Getenv("ADMIN_PASSWORD"),
		AdminJWTSecret: os.Getenv("ADMIN_JWT_SECRET"),

		SSHProxyAddr:              envOrDefault("SSH_PROXY_ADDR", ":2222"),
		SSHProxyContainerUser:     envOrDefault("SSH_PROXY_CONTAINER_USER", "workspace"),
		SSHProxyContainerPassword: envOrDefault("SSH_PROXY_CONTAINER_PASSWORD", "workspace"),
		SSHProxyHostKeyPath:       envOrDefault("SSH_PROXY_HOST_KEY_PATH", "/var/lib/cloud-cli-proxy/ssh_host_ed25519_key"),
	}

	// Phase 47 Plan 01：允许通过 EXPIRY_SCAN_INTERVAL 环境变量缩短到期扫描周期，
	// 主要服务 e2e/Linux runner 在 60s 默认下做不到的快速断言。生产部署可不设。
	// 解析失败仅 warn，落回 app.Run 内部的 60s 默认。
	if v := os.Getenv("EXPIRY_SCAN_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.ExpiryScanInterval = d
		} else {
			slog.Warn("invalid EXPIRY_SCAN_INTERVAL, falling back to default",
				"value", v, "error", err)
		}
	}

	if cfg.DatabaseURL == "" {
		slog.Error("DATABASE_URL is required")
		os.Exit(1)
	}

	if cfg.AdminJWTSecret == "" {
		slog.Warn("ADMIN_JWT_SECRET not set, admin API disabled")
	}

	controlPlane, err := app.New(ctx, cfg)
	if err != nil {
		slog.Error("failed to initialize control-plane", "error", err)
		os.Exit(1)
	}

	if err := controlPlane.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("control-plane stopped with error", "error", err)
		os.Exit(1)
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}
