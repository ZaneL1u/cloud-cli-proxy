//go:build linux

package network

import (
	"context"
	"log/slog"
)

// RoutingProvider implements Provider by delegating to SingBoxProvider for
// proxy egress mode. This is the single injection point used by host-agent.
type RoutingProvider struct {
	singbox *SingBoxProvider
	logger  *slog.Logger
}

// NewRoutingProvider creates a RoutingProvider that owns a SingBoxProvider
// for proxy egress mode.
func NewRoutingProvider(logger *slog.Logger) *RoutingProvider {
	return &RoutingProvider{
		singbox: NewSingBoxProvider(logger),
		logger:  logger,
	}
}

// PrepareHost routes to the sing-box provider.
func (rp *RoutingProvider) PrepareHost(ctx context.Context, spec HostNetworkSpec) error {
	if spec.Egress == nil {
		return &NetworkError{
			Type:    ErrBindingMissing,
			Message: "PrepareHost called without egress config",
			HostID:  spec.HostID,
		}
	}

	rp.logger.Info("routing to sing-box provider", "host_id", spec.HostID)
	return rp.singbox.PrepareHost(ctx, spec)
}

// CleanupHost cleans up artifacts from the sing-box provider.
func (rp *RoutingProvider) CleanupHost(ctx context.Context, spec HostNetworkSpec) error {
	rp.singbox.CleanupHost(ctx, spec)
	return nil
}
