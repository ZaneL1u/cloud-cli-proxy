package http

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	nethttp "net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/zanel1u/cloud-cli-proxy/internal/agentapi"
	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

type stubHostStore struct {
	hosts     []repository.HostWithUsername
	detail    repository.HostDetail
	listErr   error
	detailErr error
}

func (s *stubHostStore) ListHostsWithUsername(_ context.Context) ([]repository.HostWithUsername, error) {
	return s.hosts, s.listErr
}

func (s *stubHostStore) GetHostDetail(_ context.Context, _ string) (repository.HostDetail, error) {
	return s.detail, s.detailErr
}

func (s *stubHostStore) GetHost(_ context.Context, _ string) (repository.Host, error) {
	return repository.Host{}, nil
}

func (s *stubHostStore) UpsertHost(_ context.Context, _ repository.UpsertHostParams) (repository.Host, error) {
	return repository.Host{}, nil
}

func (s *stubHostStore) GetUser(_ context.Context, _ string) (repository.User, error) {
	return repository.User{}, nil
}

func (s *stubHostStore) BindEgressIPToHost(_ context.Context, _ string, _ string) (repository.HostBinding, error) {
	return repository.HostBinding{}, nil
}

func (s *stubHostStore) DeleteHost(_ context.Context, _ string) error {
	return nil
}

func (s *stubHostStore) UpdateHostEntryPassword(_ context.Context, _ string, _ string) error {
	return nil
}

func TestAdminHostsHandler(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	sampleHost := repository.HostWithUsername{
		Host:     repository.Host{ID: "h1", UserID: "u1", Status: "stopped", CreatedAt: now, UpdatedAt: now},
		Username: "testuser",
	}
	sampleDetail := repository.HostDetail{
		Host:     repository.Host{ID: "h1", UserID: "u1", Status: "stopped", CreatedAt: now, UpdatedAt: now},
		User:     repository.User{ID: "u1", Username: "testuser", Status: "active", CreatedAt: now, UpdatedAt: now},
		Bindings: []repository.BindingWithIP{},
	}

	tests := []struct {
		name        string
		method      string
		path        string
		hostStore   *stubHostStore
		queue       *stubQueuer
		wantStatus  int
		wantField   string
		wantAction  agentapi.HostAction
		wantQueued  bool
	}{
		{
			name:       "List hosts 200",
			method:     "GET",
			path:       "/v1/admin/hosts",
			hostStore:  &stubHostStore{hosts: []repository.HostWithUsername{sampleHost}},
			queue:      &stubQueuer{},
			wantStatus: 200,
			wantField:  "hosts",
		},
		{
			name:       "List hosts store error 500",
			method:     "GET",
			path:       "/v1/admin/hosts",
			hostStore:  &stubHostStore{listErr: fmt.Errorf("db down")},
			queue:      &stubQueuer{},
			wantStatus: 500,
		},
		{
			name:       "Get host detail 200",
			method:     "GET",
			path:       "/v1/admin/hosts/h1",
			hostStore:  &stubHostStore{detail: sampleDetail},
			queue:      &stubQueuer{},
			wantStatus: 200,
			wantField:  "host",
		},
		{
			name:       "Get host 404",
			method:     "GET",
			path:       "/v1/admin/hosts/missing",
			hostStore:  &stubHostStore{detailErr: pgx.ErrNoRows},
			queue:      &stubQueuer{},
			wantStatus: 404,
		},
		{
			name:       "Start host 202",
			method:     "POST",
			path:       "/v1/admin/hosts/h1/start",
			hostStore:  &stubHostStore{},
			queue:      &stubQueuer{task: repository.Task{ID: "t1"}},
			wantStatus: 202,
			wantQueued: true,
			wantAction: agentapi.ActionStartHost,
		},
		{
			name:       "Start host 404",
			method:     "POST",
			path:       "/v1/admin/hosts/missing/start",
			hostStore:  &stubHostStore{},
			queue:      &stubQueuer{err: pgx.ErrNoRows},
			wantStatus: 404,
		},
		{
			name:       "Stop host 202",
			method:     "POST",
			path:       "/v1/admin/hosts/h1/stop",
			hostStore:  &stubHostStore{},
			queue:      &stubQueuer{task: repository.Task{ID: "t2"}},
			wantStatus: 202,
			wantQueued: true,
			wantAction: agentapi.ActionStopHost,
		},
		{
			name:       "Rebuild host 202",
			method:     "POST",
			path:       "/v1/admin/hosts/h1/rebuild",
			hostStore:  &stubHostStore{},
			queue:      &stubQueuer{task: repository.Task{ID: "t3"}},
			wantStatus: 202,
			wantQueued: true,
			wantAction: agentapi.ActionRebuildHost,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := &stubEventRecorder{}
			router := adminTestRouter(t, Dependencies{
				Logger:        slog.Default(),
				AdminHosts:    tt.hostStore,
				HostActions:   tt.queue,
				EventRecorder: events,
			})
			srv := httptest.NewServer(router)
			defer srv.Close()

			req, _ := nethttp.NewRequest(tt.method, srv.URL+tt.path, nil)
			req.Header.Set("Authorization", "Bearer "+validAdminToken(t))

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

			if tt.wantQueued && !tt.queue.called {
				t.Error("expected queue to be called")
			}
			if tt.wantQueued && tt.queue.calledAction != tt.wantAction {
				t.Errorf("queue action = %v, want %v", tt.queue.calledAction, tt.wantAction)
			}

			if tt.wantField != "" {
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
