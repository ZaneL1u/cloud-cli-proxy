package network

import (
	"context"
	"encoding/json"
	"fmt"
)

// EgressIPRecord is a storage-agnostic representation of an egress IP row.
// The network package defines its own record type so it never imports the store package.
type EgressIPRecord struct {
	ID          string
	IPAddress   string
	TunnelType  string
	ProxyConfig json.RawMessage
}

// EgressValidator abstracts the repository queries needed for binding validation.
type EgressValidator interface {
	GetEgressIPByHost(ctx context.Context, hostID string) (EgressIPRecord, error)
}

// ValidateEgressBinding checks that the host has a valid egress IP binding with
// complete tunnel configuration. Returns a fully-populated EgressConfig on success
// or a typed NetworkError on failure.
func ValidateEgressBinding(ctx context.Context, v EgressValidator, hostID string) (EgressConfig, error) {
	record, err := v.GetEgressIPByHost(ctx, hostID)
	if err != nil {
		return EgressConfig{}, &NetworkError{
			Type:    ErrBindingMissing,
			Message: "no egress IP bound to host",
			HostID:  hostID,
		}
	}

	return validateProxyBinding(record, hostID)
}

func validateProxyBinding(record EgressIPRecord, hostID string) (EgressConfig, error) {
	if record.ProxyConfig == nil || len(record.ProxyConfig) == 0 {
		return EgressConfig{}, &NetworkError{
			Type:    ErrTunnelSetupFailed,
			Message: "proxy type requires non-empty proxy_config",
			HostID:  hostID,
			Metadata: map[string]any{"egress_ip_id": record.ID},
		}
	}

	var parsed map[string]any
	if err := json.Unmarshal(record.ProxyConfig, &parsed); err != nil {
		return EgressConfig{}, &NetworkError{
			Type:    ErrTunnelSetupFailed,
			Message: fmt.Sprintf("proxy_config is not valid JSON: %v", err),
			HostID:  hostID,
			Metadata: map[string]any{"egress_ip_id": record.ID},
		}
	}

	dnsServer, _ := parsed["dns_server"].(string)

	return EgressConfig{
		EgressIPID: record.ID,
		ExpectedIP: record.IPAddress,
		TunnelType: TunnelTypeProxy,
		Proxy: &ProxySpec{
			OutboundConfig: record.ProxyConfig,
			DNSServer:      dnsServer,
		},
	}, nil
}

func truncateID(id string, n int) string {
	if len(id) <= n {
		return id
	}
	return id[:n]
}
