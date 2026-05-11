//go:build !linux

package network

import (
	"context"
	"log/slog"
)

// ContainerProxyProvider is a no-op stub on non-Linux platforms.
type ContainerProxyProvider struct {
	logger *slog.Logger
}

func NewContainerProxyProvider(logger *slog.Logger) *ContainerProxyProvider {
	return &ContainerProxyProvider{logger: logger}
}

func (p *ContainerProxyProvider) PrepareHost(_ context.Context, _ HostNetworkSpec) error {
	return nil
}

func (p *ContainerProxyProvider) CleanupHost(_ context.Context, _ HostNetworkSpec) error {
	return nil
}
