package network

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type mockValidator struct {
	egressIP  EgressIPRecord
	egressErr error
}

func (m *mockValidator) GetEgressIPByHost(_ context.Context, _ string) (EgressIPRecord, error) {
	return m.egressIP, m.egressErr
}

func TestValidateEgressBinding_MissingBinding(t *testing.T) {
	v := &mockValidator{
		egressErr: errors.New("no rows"),
	}

	_, err := ValidateEgressBinding(context.Background(), v, "host-1")
	if err == nil {
		t.Fatal("expected error for missing binding")
	}

	var netErr *NetworkError
	if !errors.As(err, &netErr) {
		t.Fatalf("expected *NetworkError, got %T", err)
	}
	if netErr.Type != ErrBindingMissing {
		t.Errorf("expected ErrBindingMissing, got %s", netErr.Type)
	}
}

func TestValidateEgressBinding_ProxySuccess(t *testing.T) {
	proxyConfig := json.RawMessage(`{"type":"socks","server":"proxy.example.com","server_port":1080,"dns_server":"10.0.0.1"}`)

	v := &mockValidator{
		egressIP: EgressIPRecord{
			ID:          "eip-proxy-1",
			IPAddress:   "5.6.7.8",
			TunnelType:  TunnelTypeProxy,
			ProxyConfig: proxyConfig,
		},
	}

	cfg, err := ValidateEgressBinding(context.Background(), v, "host-proxy-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.TunnelType != TunnelTypeProxy {
		t.Errorf("TunnelType: got %q, want %q", cfg.TunnelType, TunnelTypeProxy)
	}
	if cfg.EgressIPID != "eip-proxy-1" {
		t.Errorf("EgressIPID: got %q, want %q", cfg.EgressIPID, "eip-proxy-1")
	}
	if cfg.ExpectedIP != "5.6.7.8" {
		t.Errorf("ExpectedIP: got %q, want %q", cfg.ExpectedIP, "5.6.7.8")
	}
	if cfg.Proxy == nil {
		t.Fatal("Proxy should not be nil for proxy type")
	}
	if cfg.Proxy.OutboundConfig == nil {
		t.Error("Proxy.OutboundConfig should not be nil")
	}
	if cfg.Proxy.DNSServer != "10.0.0.1" {
		t.Errorf("Proxy.DNSServer: got %q, want %q", cfg.Proxy.DNSServer, "10.0.0.1")
	}
}

func TestValidateEgressBinding_ProxyMissingConfig(t *testing.T) {
	v := &mockValidator{
		egressIP: EgressIPRecord{
			ID:          "eip-proxy-2",
			IPAddress:   "5.6.7.8",
			TunnelType:  TunnelTypeProxy,
			ProxyConfig: nil,
		},
	}

	_, err := ValidateEgressBinding(context.Background(), v, "host-proxy-2")
	if err == nil {
		t.Fatal("expected error for proxy type with nil proxy_config")
	}

	var netErr *NetworkError
	if !errors.As(err, &netErr) {
		t.Fatalf("expected *NetworkError, got %T", err)
	}
	if netErr.Type != ErrTunnelSetupFailed {
		t.Errorf("expected ErrTunnelSetupFailed, got %s", netErr.Type)
	}
}

func TestValidateEgressBinding_ProxyInvalidJSON(t *testing.T) {
	v := &mockValidator{
		egressIP: EgressIPRecord{
			ID:          "eip-proxy-3",
			IPAddress:   "5.6.7.8",
			TunnelType:  TunnelTypeProxy,
			ProxyConfig: json.RawMessage(`{invalid json`),
		},
	}

	_, err := ValidateEgressBinding(context.Background(), v, "host-proxy-3")
	if err == nil {
		t.Fatal("expected error for invalid proxy_config JSON")
	}

	var netErr *NetworkError
	if !errors.As(err, &netErr) {
		t.Fatalf("expected *NetworkError, got %T", err)
	}
	if netErr.Type != ErrTunnelSetupFailed {
		t.Errorf("expected ErrTunnelSetupFailed, got %s", netErr.Type)
	}
}
