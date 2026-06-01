package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	nethttp "net/http"
	"net/http/httptest"
	"testing"
	"time"

	"database/sql"

	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

type stubEgressIPStore struct {
	ips       []repository.EgressIP
	ip        repository.EgressIP
	err       error
	createIP  repository.EgressIP
	createErr error
	updateIP  repository.EgressIP
	updateErr error
	deleteErr error
}

func (s *stubEgressIPStore) ListEgressIPs(_ context.Context) ([]repository.EgressIP, error) {
	return s.ips, s.err
}

func (s *stubEgressIPStore) GetEgressIP(_ context.Context, _ string) (repository.EgressIP, error) {
	if s.err != nil {
		return repository.EgressIP{}, s.err
	}
	return s.ip, nil
}

func (s *stubEgressIPStore) CreateEgressIP(_ context.Context, _ repository.CreateEgressIPParams) (repository.EgressIP, error) {
	if s.createErr != nil {
		return repository.EgressIP{}, s.createErr
	}
	return s.createIP, nil
}

func (s *stubEgressIPStore) UpdateEgressIP(_ context.Context, _ string, _ repository.UpdateEgressIPParams) (repository.EgressIP, error) {
	if s.updateErr != nil {
		return repository.EgressIP{}, s.updateErr
	}
	return s.updateIP, nil
}

func (s *stubEgressIPStore) DeleteEgressIP(_ context.Context, _ string) error {
	return s.deleteErr
}

func (s *stubEgressIPStore) UpdateEgressIPDetectedAddress(_ context.Context, _ string, _ string) error {
	return nil
}

func TestAdminEgressIPsHandler(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	sampleIP := repository.EgressIP{
		ID: "ip1", Label: "test-ip", IPAddress: "1.2.3.4",
		Provider: "manual", Status: "available",
		CreatedAt: now, UpdatedAt: now,
	}

	tests := []struct {
		name       string
		method     string
		path       string
		body       any
		store      *stubEgressIPStore
		wantStatus int
		wantField  string
	}{
		{
			name:       "List egress IPs 200",
			method:     "GET",
			path:       "/v1/admin/egress-ips",
			store:      &stubEgressIPStore{ips: []repository.EgressIP{sampleIP}},
			wantStatus: 200,
			wantField:  "egress_ips",
		},
		{
			name:       "List egress IPs store error 500",
			method:     "GET",
			path:       "/v1/admin/egress-ips",
			store:      &stubEgressIPStore{err: fmt.Errorf("db down")},
			wantStatus: 500,
		},
		{
			name:   "Create egress IP 201",
			method: "POST",
			path:   "/v1/admin/egress-ips",
			body: map[string]any{
				"label": "new-ip", "ip_address": "5.6.7.8",
				"provider": "aws",
				"proxy_config": map[string]any{
					"type": "socks", "server": "proxy.example.com", "server_port": 1080,
				},
			},
			store:      &stubEgressIPStore{createIP: sampleIP},
			wantStatus: 201,
			wantField:  "egress_ip",
		},
		{
			name:       "Create egress IP missing label 400",
			method:     "POST",
			path:       "/v1/admin/egress-ips",
			body:       map[string]string{"ip_address": "5.6.7.8"},
			store:      &stubEgressIPStore{},
			wantStatus: 400,
		},
		{
			name:       "Create egress IP invalid IP 400",
			method:     "POST",
			path:       "/v1/admin/egress-ips",
			body:       map[string]string{"label": "bad", "ip_address": "not-an-ip"},
			store:      &stubEgressIPStore{},
			wantStatus: 400,
		},
		{
			name:       "Get egress IP 200",
			method:     "GET",
			path:       "/v1/admin/egress-ips/ip1",
			store:      &stubEgressIPStore{ip: sampleIP},
			wantStatus: 200,
			wantField:  "egress_ip",
		},
		{
			name:       "Get egress IP 404",
			method:     "GET",
			path:       "/v1/admin/egress-ips/missing",
			store:      &stubEgressIPStore{err: sql.ErrNoRows},
			wantStatus: 404,
		},
		{
			name:   "Update egress IP 200",
			method: "PUT",
			path:   "/v1/admin/egress-ips/ip1",
			body: map[string]any{
				"label": "updated", "ip_address": "5.6.7.8",
				"provider": "aws", "status": "available",
				"proxy_config": map[string]any{
					"type": "socks", "server": "proxy.example.com", "server_port": 1080,
				},
			},
			store:      &stubEgressIPStore{updateIP: sampleIP},
			wantStatus: 200,
			wantField:  "egress_ip",
		},
		{
			name:   "Update egress IP 404",
			method: "PUT",
			path:   "/v1/admin/egress-ips/missing",
			body: map[string]any{
				"label": "x",
				"proxy_config": map[string]any{
					"type": "socks", "server": "proxy.example.com", "server_port": 1080,
				},
			},
			store:      &stubEgressIPStore{updateErr: sql.ErrNoRows},
			wantStatus: 404,
		},
		{
			name:       "Delete egress IP 204",
			method:     "DELETE",
			path:       "/v1/admin/egress-ips/ip1",
			store:      &stubEgressIPStore{},
			wantStatus: 204,
		},
		{
			name:       "Delete egress IP 404",
			method:     "DELETE",
			path:       "/v1/admin/egress-ips/missing",
			store:      &stubEgressIPStore{deleteErr: sql.ErrNoRows},
			wantStatus: 404,
		},
		{
			name:   "Create proxy egress IP 201",
			method: "POST",
			path:   "/v1/admin/egress-ips",
			body: map[string]any{
				"label":      "proxy-ip",
				"ip_address": "5.6.7.8",
				"provider":   "manual",
				"proxy_config": map[string]any{
					"type": "socks", "server": "proxy.example.com", "server_port": 1080,
				},
			},
			store: &stubEgressIPStore{createIP: repository.EgressIP{
				ID: "ip2", Label: "proxy-ip", IPAddress: "5.6.7.8",
				Provider: "manual", Status: "available",
				CreatedAt: now, UpdatedAt: now,
			}},
			wantStatus: 201,
			wantField:  "egress_ip",
		},
		{
			name:   "Create missing proxy_config 400",
			method: "POST",
			path:   "/v1/admin/egress-ips",
			body: map[string]any{
				"label": "no-config", "ip_address": "1.2.3.4",
			},
			store:      &stubEgressIPStore{},
			wantStatus: 400,
		},
		{
			name:   "Create unsupported outbound type 400",
			method: "POST",
			path:   "/v1/admin/egress-ips",
			body: map[string]any{
				"label": "bad-type", "ip_address": "1.2.3.4",
				"proxy_config": map[string]any{
					"type": "unsupported", "server": "example.com", "server_port": 1080,
				},
			},
			store:      &stubEgressIPStore{},
			wantStatus: 400,
		},
		{
			name:   "Create missing server 400",
			method: "POST",
			path:   "/v1/admin/egress-ips",
			body: map[string]any{
				"label": "no-server", "ip_address": "1.2.3.4",
				"proxy_config": map[string]any{
					"type": "socks", "server_port": 1080,
				},
			},
			store:      &stubEgressIPStore{},
			wantStatus: 400,
		},
		{
			name:   "Create vmess missing uuid 400",
			method: "POST",
			path:   "/v1/admin/egress-ips",
			body: map[string]any{
				"label": "no-uuid", "ip_address": "1.2.3.4",
				"proxy_config": map[string]any{
					"type": "vmess", "server": "proxy.example.com", "server_port": 443,
				},
			},
			store:      &stubEgressIPStore{},
			wantStatus: 400,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := &stubEventRecorder{}
			router := adminTestRouter(t, Dependencies{
				Logger:         slog.Default(),
				AdminEgressIPs: tt.store,
				EventRecorder:  events,
			})
			srv := httptest.NewServer(router)
			defer srv.Close()

			var body []byte
			if tt.body != nil {
				body, _ = json.Marshal(tt.body)
			}

			req, _ := nethttp.NewRequest(tt.method, srv.URL+tt.path, bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+validAdminToken(t))
			req.Header.Set("Content-Type", "application/json")

			resp, err := nethttp.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.wantStatus {
				var respBody map[string]any
				json.NewDecoder(resp.Body).Decode(&respBody)
				t.Errorf("status = %d, want %d; body = %v", resp.StatusCode, tt.wantStatus, respBody)
				return
			}

			if tt.wantField != "" && resp.StatusCode != 204 {
				var respBody map[string]any
				if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
					t.Fatalf("decode response: %v", err)
				}
				if _, ok := respBody[tt.wantField]; !ok {
					t.Errorf("response missing field %q: %v", tt.wantField, respBody)
				}
			}
		})
	}
}
