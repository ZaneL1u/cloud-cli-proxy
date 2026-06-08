package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/zanel1u/cloud-cli-proxy/internal/agentapi"
	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

type mockReconcileStore struct {
	runningHosts   []repository.Host
	runningHostErr error
	updatedHosts   []struct{ ID, Status string }
	staleMarked    []repository.Task
	staleErr       error
	recordedEvents []repository.RecordEventParams
}

func (m *mockReconcileStore) ListRunningHosts(_ context.Context) ([]repository.Host, error) {
	return m.runningHosts, m.runningHostErr
}

func (m *mockReconcileStore) ListFailedHosts(_ context.Context) ([]repository.Host, error) {
	return nil, nil
}

func (m *mockReconcileStore) UpdateHostStatus(_ context.Context, id, status string) error {
	m.updatedHosts = append(m.updatedHosts, struct{ ID, Status string }{id, status})
	return nil
}

func (m *mockReconcileStore) MarkStaleTasks(_ context.Context, _ time.Duration) ([]repository.Task, error) {
	return m.staleMarked, m.staleErr
}

func (m *mockReconcileStore) RecordEvent(_ context.Context, params repository.RecordEventParams) (repository.Event, error) {
	m.recordedEvents = append(m.recordedEvents, params)
	return repository.Event{}, nil
}

type mockInspector struct {
	results map[string]agentapi.ContainerStatusResponse
	errors  map[string]error
}

func (m *mockInspector) InspectContainer(_ context.Context, name string) (agentapi.ContainerStatusResponse, error) {
	if err, ok := m.errors[name]; ok {
		return agentapi.ContainerStatusResponse{}, err
	}
	if resp, ok := m.results[name]; ok {
		return resp, nil
	}
	return agentapi.ContainerStatusResponse{}, nil
}

func TestReconciler(t *testing.T) {
	t.Run("Run_RunningContainers_QueuesPrepareHost", func(t *testing.T) {
		store := &mockReconcileStore{
			runningHosts: []repository.Host{
				{ID: "h1"},
				{ID: "h2"},
			},
		}
		inspector := &mockInspector{
			results: map[string]agentapi.ContainerStatusResponse{
				"cloudproxy-h1": {Exists: true, Running: true},
				"cloudproxy-h2": {Exists: true, Running: true},
			},
		}
		queuer := &mockQueuer{}
		r := NewReconciler(slog.Default(), store, inspector, queuer, 10*time.Minute)

		err := r.Run(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(store.updatedHosts) != 0 {
			t.Errorf("expected no host updates, got %d", len(store.updatedHosts))
		}
		if len(queuer.queuedActions) != 2 {
			t.Fatalf("expected 2 queued actions, got %d", len(queuer.queuedActions))
		}
		for _, queued := range queuer.queuedActions {
			if queued.Action != agentapi.ActionPrepareHost || queued.RequestedBy != "system" {
				t.Errorf("queued = %+v, want prepare_host by system", queued)
			}
		}
		prepareEvents := 0
		for _, ev := range store.recordedEvents {
			if ev.Type == "reconcile.host.network_prepare" {
				prepareEvents++
			}
		}
		if prepareEvents != 2 {
			t.Errorf("network prepare event count = %d, want 2", prepareEvents)
		}
	})

	t.Run("Run_ContainerStopped_QueuesStartHost", func(t *testing.T) {
		store := &mockReconcileStore{
			runningHosts: []repository.Host{{ID: "h1"}},
		}
		inspector := &mockInspector{
			results: map[string]agentapi.ContainerStatusResponse{
				"cloudproxy-h1": {Exists: true, Running: false},
			},
		}
		queuer := &mockQueuer{}
		r := NewReconciler(slog.Default(), store, inspector, queuer, 10*time.Minute)

		err := r.Run(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(queuer.queuedActions) != 1 {
			t.Fatalf("expected 1 queued action, got %d", len(queuer.queuedActions))
		}
		if queuer.queuedActions[0].HostID != "h1" || queuer.queuedActions[0].Action != agentapi.ActionStartHost || queuer.queuedActions[0].RequestedBy != "system" {
			t.Errorf("queued = %+v, want {h1 start_host system}", queuer.queuedActions[0])
		}

		if len(store.updatedHosts) != 0 {
			t.Errorf("expected no host status update when auto-recovering, got %d", len(store.updatedHosts))
		}

		foundRecover := false
		for _, ev := range store.recordedEvents {
			if ev.Type == "reconcile.host.auto_recover" {
				foundRecover = true
				actual, _ := ev.Metadata["previous_actual_status"].(string)
				if actual != "stopped" {
					t.Errorf("previous_actual_status = %q, want stopped", actual)
				}
			}
		}
		if !foundRecover {
			t.Error("expected reconcile.host.auto_recover event, not found")
		}
	})

	t.Run("Run_ContainerNotFound_QueuesStartHost", func(t *testing.T) {
		store := &mockReconcileStore{
			runningHosts: []repository.Host{{ID: "h1"}},
		}
		inspector := &mockInspector{
			results: map[string]agentapi.ContainerStatusResponse{
				"cloudproxy-h1": {Exists: false, Running: false},
			},
		}
		queuer := &mockQueuer{}
		r := NewReconciler(slog.Default(), store, inspector, queuer, 10*time.Minute)

		err := r.Run(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(queuer.queuedActions) != 1 {
			t.Fatalf("expected 1 queued action, got %d", len(queuer.queuedActions))
		}
		if queuer.queuedActions[0].HostID != "h1" || queuer.queuedActions[0].Action != agentapi.ActionStartHost {
			t.Errorf("queued = %+v, want {h1 start_host}", queuer.queuedActions[0])
		}

		foundRecover := false
		for _, ev := range store.recordedEvents {
			if ev.Type == "reconcile.host.auto_recover" {
				foundRecover = true
				actual, _ := ev.Metadata["previous_actual_status"].(string)
				if actual != "not_found" {
					t.Errorf("previous_actual_status = %q, want not_found", actual)
				}
			}
		}
		if !foundRecover {
			t.Error("expected reconcile.host.auto_recover event, not found")
		}
	})

	t.Run("Run_QueuerNil_FallsBackToDrift", func(t *testing.T) {
		store := &mockReconcileStore{
			runningHosts: []repository.Host{{ID: "h1"}},
		}
		inspector := &mockInspector{
			results: map[string]agentapi.ContainerStatusResponse{
				"cloudproxy-h1": {Exists: true, Running: false},
			},
		}
		r := NewReconciler(slog.Default(), store, inspector, nil, 10*time.Minute)

		err := r.Run(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(store.updatedHosts) != 1 {
			t.Fatalf("expected 1 host update, got %d", len(store.updatedHosts))
		}
		if store.updatedHosts[0].ID != "h1" || store.updatedHosts[0].Status != "stopped" {
			t.Errorf("update = %+v, want {ID:h1 Status:stopped}", store.updatedHosts[0])
		}

		foundDrift := false
		for _, ev := range store.recordedEvents {
			if ev.Type == "reconcile.host.drift" {
				foundDrift = true
				actual, _ := ev.Metadata["actual_status"].(string)
				if actual != "stopped" {
					t.Errorf("actual_status = %q, want stopped", actual)
				}
			}
		}
		if !foundDrift {
			t.Error("expected reconcile.host.drift event, not found")
		}
	})

	t.Run("Run_QueuerError_FallsBackToDrift", func(t *testing.T) {
		store := &mockReconcileStore{
			runningHosts: []repository.Host{{ID: "h1"}},
		}
		inspector := &mockInspector{
			results: map[string]agentapi.ContainerStatusResponse{
				"cloudproxy-h1": {Exists: true, Running: false},
			},
		}
		queuer := &mockQueuer{err: errors.New("queue full")}
		r := NewReconciler(slog.Default(), store, inspector, queuer, 10*time.Minute)

		err := r.Run(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(queuer.queuedActions) != 1 {
			t.Fatalf("expected 1 queued action attempt, got %d", len(queuer.queuedActions))
		}

		if len(store.updatedHosts) != 1 {
			t.Fatalf("expected 1 host update on fallback, got %d", len(store.updatedHosts))
		}
		if store.updatedHosts[0].Status != "stopped" {
			t.Errorf("status = %q, want stopped", store.updatedHosts[0].Status)
		}

		foundDrift := false
		for _, ev := range store.recordedEvents {
			if ev.Type == "reconcile.host.drift" {
				foundDrift = true
			}
		}
		if !foundDrift {
			t.Error("expected reconcile.host.drift event on fallback, not found")
		}
	})

	t.Run("Run_InspectError_SkipsHost", func(t *testing.T) {
		store := &mockReconcileStore{
			runningHosts: []repository.Host{{ID: "h1"}},
		}
		inspector := &mockInspector{
			errors: map[string]error{
				"cloudproxy-h1": errors.New("agent unreachable"),
			},
		}
		queuer := &mockQueuer{}
		r := NewReconciler(slog.Default(), store, inspector, queuer, 10*time.Minute)

		err := r.Run(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(store.updatedHosts) != 0 {
			t.Errorf("expected no host updates when inspect fails, got %d", len(store.updatedHosts))
		}
		if len(queuer.queuedActions) != 0 {
			t.Errorf("expected no queued actions when inspect fails, got %d", len(queuer.queuedActions))
		}
	})

	t.Run("Run_StaleTasks_MarkedAndRecorded", func(t *testing.T) {
		hostID := "h1"
		store := &mockReconcileStore{
			staleMarked: []repository.Task{
				{ID: "t1", HostID: &hostID, Kind: "start_host"},
				{ID: "t2", HostID: &hostID, Kind: "stop_host"},
			},
		}
		inspector := &mockInspector{}
		queuer := &mockQueuer{}
		r := NewReconciler(slog.Default(), store, inspector, queuer, 10*time.Minute)

		err := r.Run(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		staleCount := 0
		for _, ev := range store.recordedEvents {
			if ev.Type == "reconcile.task.stale" {
				staleCount++
			}
		}
		if staleCount != 2 {
			t.Errorf("stale event count = %d, want 2", staleCount)
		}
	})

	t.Run("Run_NoRunningHosts_NoAction", func(t *testing.T) {
		store := &mockReconcileStore{
			runningHosts: []repository.Host{},
		}
		inspector := &mockInspector{}
		queuer := &mockQueuer{}
		r := NewReconciler(slog.Default(), store, inspector, queuer, 10*time.Minute)

		err := r.Run(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(store.updatedHosts) != 0 {
			t.Errorf("expected no updates, got %d", len(store.updatedHosts))
		}
		if len(store.recordedEvents) != 0 {
			t.Errorf("expected no events, got %d", len(store.recordedEvents))
		}
		if len(queuer.queuedActions) != 0 {
			t.Errorf("expected no queued actions, got %d", len(queuer.queuedActions))
		}
	})
}
