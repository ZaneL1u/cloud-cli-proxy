package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/zanel1u/cloud-cli-proxy/internal/agentapi"
	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

type mockExpiryStore struct {
	expiredUsers    []repository.User
	expiredUsersErr error
	updatedStatuses []struct{ ID, Status string }
	updateErr       error
	runningHosts    map[string][]repository.Host
	recordedEvents  []repository.RecordEventParams
}

func (m *mockExpiryStore) ListExpiredActiveUsers(_ context.Context) ([]repository.User, error) {
	return m.expiredUsers, m.expiredUsersErr
}

func (m *mockExpiryStore) UpdateUserStatus(_ context.Context, id, status string) (repository.User, error) {
	m.updatedStatuses = append(m.updatedStatuses, struct{ ID, Status string }{id, status})
	if m.updateErr != nil {
		return repository.User{}, m.updateErr
	}
	return repository.User{ID: id, Status: status}, nil
}

func (m *mockExpiryStore) ListRunningHostsByUserID(_ context.Context, userID string) ([]repository.Host, error) {
	hosts := m.runningHosts[userID]
	return hosts, nil
}

func (m *mockExpiryStore) RecordEvent(_ context.Context, params repository.RecordEventParams) (repository.Event, error) {
	m.recordedEvents = append(m.recordedEvents, params)
	return repository.Event{}, nil
}

type mockQueuer struct {
	queuedActions []struct {
		HostID      string
		Action      agentapi.HostAction
		RequestedBy string
	}
	err error
}

func (m *mockQueuer) QueueHostAction(_ context.Context, hostID string, action agentapi.HostAction, requestedBy string, _ string) (repository.Task, error) {
	m.queuedActions = append(m.queuedActions, struct {
		HostID      string
		Action      agentapi.HostAction
		RequestedBy string
	}{hostID, action, requestedBy})
	return repository.Task{}, m.err
}

func TestExpiryScanner(t *testing.T) {
	t.Run("Scan_NoExpiredUsers_DoesNothing", func(t *testing.T) {
		store := &mockExpiryStore{expiredUsers: []repository.User{}}
		queue := &mockQueuer{}
		scanner := NewExpiryScanner(slog.Default(), store, queue)

		err := scanner.Scan(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(store.updatedStatuses) != 0 {
			t.Errorf("expected no status updates, got %d", len(store.updatedStatuses))
		}
		if len(store.recordedEvents) != 0 {
			t.Errorf("expected no events, got %d", len(store.recordedEvents))
		}
		if len(queue.queuedActions) != 0 {
			t.Errorf("expected no queued actions, got %d", len(queue.queuedActions))
		}
	})

	t.Run("Scan_ExpiredUser_MarksExpiredAndRecordsEvent", func(t *testing.T) {
		store := &mockExpiryStore{
			expiredUsers: []repository.User{
				{ID: "u1", Username: "alice"},
			},
			runningHosts: map[string][]repository.Host{},
		}
		queue := &mockQueuer{}
		scanner := NewExpiryScanner(slog.Default(), store, queue)

		err := scanner.Scan(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(store.updatedStatuses) != 1 {
			t.Fatalf("expected 1 status update, got %d", len(store.updatedStatuses))
		}
		if store.updatedStatuses[0].ID != "u1" || store.updatedStatuses[0].Status != "expired" {
			t.Errorf("status update = %+v, want {ID:u1 Status:expired}", store.updatedStatuses[0])
		}

		foundUserExpired := false
		for _, ev := range store.recordedEvents {
			if ev.Type == "user.expired" {
				foundUserExpired = true
			}
		}
		if !foundUserExpired {
			t.Error("expected user.expired event, not found")
		}
	})

	t.Run("Scan_ExpiredUserWithRunningHost_StopsHost", func(t *testing.T) {
		store := &mockExpiryStore{
			expiredUsers: []repository.User{
				{ID: "u1", Username: "alice"},
			},
			runningHosts: map[string][]repository.Host{
				"u1": {{ID: "h1", UserID: "u1", Status: "running"}},
			},
		}
		queue := &mockQueuer{}
		scanner := NewExpiryScanner(slog.Default(), store, queue)

		err := scanner.Scan(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(queue.queuedActions) != 1 {
			t.Fatalf("expected 1 queued action, got %d", len(queue.queuedActions))
		}
		act := queue.queuedActions[0]
		if act.HostID != "h1" {
			t.Errorf("queued host = %q, want h1", act.HostID)
		}
		if act.Action != agentapi.ActionStopHost {
			t.Errorf("queued action = %v, want %v", act.Action, agentapi.ActionStopHost)
		}
		if act.RequestedBy != "system:expiry" {
			t.Errorf("requested_by = %q, want system:expiry", act.RequestedBy)
		}

		foundStopEvent := false
		for _, ev := range store.recordedEvents {
			if ev.Type == "host.stop.expired" {
				foundStopEvent = true
			}
		}
		if !foundStopEvent {
			t.Error("expected host.stop.expired event, not found")
		}
	})

	t.Run("Scan_StoreError_ReturnsError", func(t *testing.T) {
		store := &mockExpiryStore{
			expiredUsersErr: errors.New("db connection failed"),
		}
		queue := &mockQueuer{}
		scanner := NewExpiryScanner(slog.Default(), store, queue)

		err := scanner.Scan(context.Background())
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, store.expiredUsersErr) {
			t.Errorf("error = %v, want wrapped %v", err, store.expiredUsersErr)
		}
	})

	t.Run("Scan_MultipleUsers_ContinuesOnFailure", func(t *testing.T) {
		updateCallCount := 0
		store := &mockExpiryStore{
			expiredUsers: []repository.User{
				{ID: "u1", Username: "alice"},
				{ID: "u2", Username: "bob"},
			},
			runningHosts: map[string][]repository.Host{},
		}
		queue := &mockQueuer{}

		origStore := *store
		_ = origStore

		failStore := &failOnFirstUpdateStore{
			mockExpiryStore: store,
			callCount:       &updateCallCount,
		}

		scanner := NewExpiryScanner(slog.Default(), failStore, queue)
		err := scanner.Scan(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if updateCallCount != 2 {
			t.Errorf("UpdateUserStatus call count = %d, want 2", updateCallCount)
		}
	})
}

type failOnFirstUpdateStore struct {
	*mockExpiryStore
	callCount *int
}

func (f *failOnFirstUpdateStore) UpdateUserStatus(ctx context.Context, id, status string) (repository.User, error) {
	*f.callCount++
	if *f.callCount == 1 {
		return repository.User{}, errors.New("first call fails")
	}
	return f.mockExpiryStore.UpdateUserStatus(ctx, id, status)
}
