package http

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"database/sql"

	"github.com/zanel1u/cloud-cli-proxy/internal/agentapi"
	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

type stubHostStore struct {
	hosts           []repository.HostWithUsername
	detail          repository.HostDetail
	host            repository.Host
	runningHosts    []repository.Host
	hostWithCA      repository.HostWithClaudeAccount
	hostWithCAErr   error
	listErr         error
	detailErr       error
	hostErr         error
	runningErr      error
	updatedMemoryMB *int
	updatedCPU      *float64
	updatedPids     *int
}

func (s *stubHostStore) ListHostsWithUsername(_ context.Context) ([]repository.HostWithUsername, error) {
	return s.hosts, s.listErr
}

func (s *stubHostStore) GetHostDetail(_ context.Context, _ string) (repository.HostDetail, error) {
	return s.detail, s.detailErr
}

func (s *stubHostStore) GetHost(_ context.Context, _ string) (repository.Host, error) {
	return s.host, s.hostErr
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

func (s *stubHostStore) ListRunningHosts(_ context.Context) ([]repository.Host, error) {
	return s.runningHosts, s.runningErr
}

func (s *stubHostStore) ListHostsByUserID(_ context.Context, _ string) ([]repository.Host, error) {
	return nil, nil
}

func (s *stubHostStore) GetHostWithClaudeAccount(_ context.Context, _ string) (repository.HostWithClaudeAccount, error) {
	return s.hostWithCA, s.hostWithCAErr
}

func (s *stubHostStore) UpdateHostMounts(_ context.Context, _ string, _ repository.HostMounts) error {
	return nil
}

func (s *stubHostStore) UpdateHostResources(_ context.Context, _ string, memoryLimitMB *int, cpuLimit *float64, pidsLimit *int) error {
	s.updatedMemoryMB = memoryLimitMB
	s.updatedCPU = cpuLimit
	s.updatedPids = pidsLimit
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
		name       string
		method     string
		path       string
		hostStore  *stubHostStore
		queue      *stubQueuer
		wantStatus int
		wantField  string
		wantAction agentapi.HostAction
		wantQueued bool
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
			hostStore:  &stubHostStore{detailErr: sql.ErrNoRows},
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
			queue:      &stubQueuer{err: sql.ErrNoRows},
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
		{
			name:       "Restart VNC host 409 when not running",
			method:     "POST",
			path:       "/v1/admin/hosts/h1/vnc/restart",
			hostStore:  &stubHostStore{host: repository.Host{ID: "h1", Status: "stopped"}},
			queue:      &stubQueuer{},
			wantStatus: 409,
		},
		{
			name:       "Restart VNC host 404",
			method:     "POST",
			path:       "/v1/admin/hosts/missing/vnc/restart",
			hostStore:  &stubHostStore{hostErr: sql.ErrNoRows},
			queue:      &stubQueuer{},
			wantStatus: 404,
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

func TestAdminHostDetail_IncludesPersistentVolumeName_WhenAvailable(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	store := &stubHostStore{
		detail: repository.HostDetail{
			Host:     repository.Host{ID: "h-1", UserID: "u-1", Status: "running", CreatedAt: now, UpdatedAt: now},
			User:     repository.User{ID: "u-1", Username: "u", Status: "active", CreatedAt: now, UpdatedAt: now},
			Bindings: []repository.BindingWithIP{},
		},
		hostWithCA: repository.HostWithClaudeAccount{
			Host:                 repository.Host{ID: "h-1"},
			PersistentVolumeName: "claude-state-acct-1",
		},
	}
	router := adminTestRouter(t, Dependencies{
		Logger:        slog.Default(),
		AdminHosts:    store,
		HostActions:   &stubQueuer{},
		EventRecorder: &stubEventRecorder{},
	})
	srv := httptest.NewServer(router)
	defer srv.Close()

	req, _ := nethttp.NewRequest("GET", srv.URL+"/v1/admin/hosts/h-1", nil)
	req.Header.Set("Authorization", "Bearer "+validAdminToken(t))
	resp, err := nethttp.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if v, _ := body["persistent_volume_name"].(string); v != "claude-state-acct-1" {
		t.Errorf("persistent_volume_name=%q, want claude-state-acct-1; full=%v", v, body)
	}
}

func TestAdminHostDetail_OmitsPersistentVolumeName_WhenEmpty(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	store := &stubHostStore{
		detail: repository.HostDetail{
			Host:     repository.Host{ID: "h-1", UserID: "u-1", Status: "running", CreatedAt: now, UpdatedAt: now},
			User:     repository.User{ID: "u-1", Username: "u", Status: "active", CreatedAt: now, UpdatedAt: now},
			Bindings: []repository.BindingWithIP{},
		},
		hostWithCA: repository.HostWithClaudeAccount{
			Host:                 repository.Host{ID: "h-1"},
			PersistentVolumeName: "",
		},
	}
	router := adminTestRouter(t, Dependencies{
		Logger:        slog.Default(),
		AdminHosts:    store,
		HostActions:   &stubQueuer{},
		EventRecorder: &stubEventRecorder{},
	})
	srv := httptest.NewServer(router)
	defer srv.Close()

	req, _ := nethttp.NewRequest("GET", srv.URL+"/v1/admin/hosts/h-1", nil)
	req.Header.Set("Authorization", "Bearer "+validAdminToken(t))
	resp, err := nethttp.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := body["persistent_volume_name"]; ok {
		t.Errorf("persistent_volume_name key must be omitted when empty (omitempty); body=%v", body)
	}
}

func TestAdminHostList_DoesNotIncludePersistentVolumeName(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	store := &stubHostStore{
		hosts: []repository.HostWithUsername{
			{
				Host:     repository.Host{ID: "h-1", UserID: "u-1", Status: "running", CreatedAt: now, UpdatedAt: now},
				Username: "u",
			},
		},
	}
	router := adminTestRouter(t, Dependencies{
		Logger:        slog.Default(),
		AdminHosts:    store,
		HostActions:   &stubQueuer{},
		EventRecorder: &stubEventRecorder{},
	})
	srv := httptest.NewServer(router)
	defer srv.Close()

	req, _ := nethttp.NewRequest("GET", srv.URL+"/v1/admin/hosts", nil)
	req.Header.Set("Authorization", "Bearer "+validAdminToken(t))
	resp, err := nethttp.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	hosts, ok := body["hosts"].([]any)
	if !ok || len(hosts) == 0 {
		t.Fatalf("expected hosts[] non-empty; body=%v", body)
	}
	first, _ := hosts[0].(map[string]any)
	if _, has := first["persistent_volume_name"]; has {
		t.Errorf("OOS-A19: list endpoint must NOT include persistent_volume_name; got %v", first)
	}
}

func TestPatchResources_RunningHostAppliesDockerUpdateAndPersistsPidsLimit(t *testing.T) {
	orig := dockerUpdateHostResources
	var dockerContainer string
	var dockerMemory *int
	var dockerCPU *float64
	var dockerPids *int
	dockerUpdateHostResources = func(_ context.Context, containerName string, memoryLimitMB *int, cpuLimit *float64, pidsLimit *int) error {
		dockerContainer = containerName
		dockerMemory = memoryLimitMB
		dockerCPU = cpuLimit
		dockerPids = pidsLimit
		return nil
	}
	t.Cleanup(func() { dockerUpdateHostResources = orig })

	store := &stubHostStore{host: repository.Host{ID: "h-1", Status: "running"}}
	router := adminTestRouter(t, Dependencies{
		Logger:        slog.Default(),
		AdminHosts:    store,
		HostActions:   &stubQueuer{},
		EventRecorder: &stubEventRecorder{},
	})
	srv := httptest.NewServer(router)
	defer srv.Close()

	req, _ := nethttp.NewRequest("PATCH", srv.URL+"/v1/admin/hosts/h-1/resources", strings.NewReader(`{"pids_limit":2048}`))
	req.Header.Set("Authorization", "Bearer "+validAdminToken(t))
	req.Header.Set("Content-Type", "application/json")
	resp, err := nethttp.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		var body map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&body)
		t.Fatalf("status=%d, body=%v", resp.StatusCode, body)
	}
	if dockerContainer != "cloudproxy-h-1" {
		t.Fatalf("docker container=%q, want cloudproxy-h-1", dockerContainer)
	}
	if dockerMemory != nil || dockerCPU != nil {
		t.Fatalf("docker update should only receive pids limit, memory=%v cpu=%v", dockerMemory, dockerCPU)
	}
	if dockerPids == nil || *dockerPids != 2048 {
		t.Fatalf("docker pids=%v, want 2048", dockerPids)
	}
	if store.updatedPids == nil || *store.updatedPids != 2048 {
		t.Fatalf("stored pids=%v, want 2048", store.updatedPids)
	}
}
