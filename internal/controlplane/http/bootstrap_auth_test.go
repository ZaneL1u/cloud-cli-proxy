package http

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	nethttp "net/http"
	"net/http/httptest"
	"testing"

	"database/sql"
	"golang.org/x/crypto/bcrypt"

	"github.com/zanel1u/cloud-cli-proxy/internal/agentapi"
	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

type stubUserLookup struct {
	user repository.BootstrapUserAuth
	err  error
}

func (s *stubUserLookup) GetBootstrapUserByUsername(_ context.Context, _ string) (repository.BootstrapUserAuth, error) {
	return s.user, s.err
}

type stubHostLookup struct {
	host repository.Host
	err  error
}

func (s *stubHostLookup) GetPrimaryHostByUserID(_ context.Context, _ string) (repository.Host, error) {
	return s.host, s.err
}

type stubQueuer struct {
	task              repository.Task
	err               error
	called            bool
	calledAction      agentapi.HostAction
	calledRequestedBy string
}

func (s *stubQueuer) QueueHostAction(_ context.Context, _ string, action agentapi.HostAction, requestedBy string, _ string) (repository.Task, error) {
	s.called = true
	s.calledAction = action
	s.calledRequestedBy = requestedBy
	return s.task, s.err
}

func TestBootstrapAuthHandler(t *testing.T) {
	validHash, err := bcrypt.GenerateFromPassword([]byte("correctpassword"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name            string
		body            map[string]string
		userLookup      BootstrapUserLookup
		hostLookup      BootstrapHostLookup
		queue           *stubQueuer
		wantStatus      int
		wantErrorCode   string
		wantQueueCalled bool
	}{
		{
			name: "invalid credentials returns auth_invalid",
			body: map[string]string{"username": "user1", "password": "wrongpassword"},
			userLookup: &stubUserLookup{
				user: repository.BootstrapUserAuth{
					UserID: "uid1", Username: "user1",
					PasswordHash: string(validHash), Status: "active",
				},
			},
			hostLookup:      &stubHostLookup{},
			queue:           &stubQueuer{},
			wantStatus:      nethttp.StatusUnauthorized,
			wantErrorCode:   "auth_invalid",
			wantQueueCalled: false,
		},
		{
			name: "user not found returns auth_invalid",
			body: map[string]string{"username": "nouser", "password": "pass"},
			userLookup: &stubUserLookup{
				err: sql.ErrNoRows,
			},
			hostLookup:      &stubHostLookup{},
			queue:           &stubQueuer{},
			wantStatus:      nethttp.StatusUnauthorized,
			wantErrorCode:   "auth_invalid",
			wantQueueCalled: false,
		},
		{
			name: "disabled account returns account_disabled",
			body: map[string]string{"username": "user2", "password": "correctpassword"},
			userLookup: &stubUserLookup{
				user: repository.BootstrapUserAuth{
					UserID: "uid2", Username: "user2",
					PasswordHash: string(validHash), Status: "disabled",
				},
			},
			hostLookup:      &stubHostLookup{},
			queue:           &stubQueuer{},
			wantStatus:      nethttp.StatusForbidden,
			wantErrorCode:   "account_disabled",
			wantQueueCalled: false,
		},
		{
			name: "expired account returns account_expired",
			body: map[string]string{"username": "user3", "password": "correctpassword"},
			userLookup: &stubUserLookup{
				user: repository.BootstrapUserAuth{
					UserID: "uid3", Username: "user3",
					PasswordHash: string(validHash), Status: "expired",
				},
			},
			hostLookup:      &stubHostLookup{},
			queue:           &stubQueuer{},
			wantStatus:      nethttp.StatusForbidden,
			wantErrorCode:   "account_expired",
			wantQueueCalled: false,
		},
		{
			name: "successful auth queues start_host and returns task_id",
			body: map[string]string{"username": "user1", "password": "correctpassword"},
			userLookup: &stubUserLookup{
				user: repository.BootstrapUserAuth{
					UserID: "uid1", Username: "user1",
					PasswordHash: string(validHash), Status: "active",
				},
			},
			hostLookup: &stubHostLookup{
				host: repository.Host{ID: "host1", UserID: "uid1", SlotKey: "primary"},
			},
			queue: &stubQueuer{
				task: repository.Task{ID: "task1"},
			},
			wantStatus:      nethttp.StatusOK,
			wantQueueCalled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewBootstrapAuthHandler(BootstrapAuthDependencies{
				Logger: slog.Default(),
				Users:  tt.userLookup,
				Hosts:  tt.hostLookup,
				Queue:  tt.queue,
			})

			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(nethttp.MethodPost, "/v1/bootstrap/sessions", bytes.NewReader(body))
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}

			var resp map[string]any
			if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
				t.Fatalf("decode response: %v", err)
			}

			if tt.wantErrorCode != "" {
				if got, _ := resp["error_code"].(string); got != tt.wantErrorCode {
					t.Errorf("error_code = %q, want %q", got, tt.wantErrorCode)
				}
			}

			if tt.queue.called != tt.wantQueueCalled {
				t.Errorf("queue.called = %v, want %v", tt.queue.called, tt.wantQueueCalled)
			}

			if tt.wantQueueCalled {
				if tt.queue.calledAction != agentapi.ActionStartHost {
					t.Errorf("queue action = %v, want %v", tt.queue.calledAction, agentapi.ActionStartHost)
				}
				taskID, _ := resp["task_id"].(string)
				if taskID == "" {
					t.Error("expected non-empty task_id in response")
				}
				stageCode, _ := resp["stage_code"].(string)
				if stageCode != "auth_passed" {
					t.Errorf("stage_code = %q, want %q", stageCode, "auth_passed")
				}
			}
		})
	}
}
