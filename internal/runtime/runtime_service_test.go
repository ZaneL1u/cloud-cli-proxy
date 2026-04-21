package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zanel1u/cloud-cli-proxy/internal/agentapi"
	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

// stubQueueRepo 实现 QueueHostActionRepo 最小子集，便于测试 ClaudeAccountID 注入路径。
type stubQueueRepo struct {
	host             repository.Host
	user             repository.User
	resolveAccountID string
	resolveFound     bool
	resolveErr       error
	createdTask      repository.Task
	resolveCalls     int
}

func (s *stubQueueRepo) GetHost(_ context.Context, id string) (repository.Host, error) {
	if s.host.ID != id {
		return repository.Host{}, errors.New("host not found")
	}
	return s.host, nil
}
func (s *stubQueueRepo) GetUser(_ context.Context, id string) (repository.User, error) {
	if s.user.ID != id {
		return repository.User{}, errors.New("user not found")
	}
	return s.user, nil
}
func (s *stubQueueRepo) CreateTask(_ context.Context, params repository.CreateTaskParams) (repository.Task, error) {
	s.createdTask = repository.Task{
		ID:          "task-1",
		HostID:      params.HostID,
		Kind:        params.Kind,
		Status:      params.Status,
		RequestedBy: params.RequestedBy,
	}
	return s.createdTask, nil
}
func (s *stubQueueRepo) ListSSHKeysByUser(_ context.Context, _ string) ([]repository.SSHKey, error) {
	return nil, nil
}
func (s *stubQueueRepo) RecordEvent(_ context.Context, _ repository.RecordEventParams) (repository.Event, error) {
	return repository.Event{}, nil
}
func (s *stubQueueRepo) ResolveClaudeAccountIDForEntry(_ context.Context, userID, hostID string) (string, bool, error) {
	s.resolveCalls++
	return s.resolveAccountID, s.resolveFound, s.resolveErr
}

// captureDispatcher 抓住最后一次 Dispatch 接收到的 request；不真正派发到 worker。
type captureDispatcher struct {
	mu      sync.Mutex
	last    agentapi.HostActionRequest
	called  chan struct{}
	calledN int
}

func newCaptureDispatcher() *captureDispatcher {
	return &captureDispatcher{called: make(chan struct{}, 1)}
}
func (d *captureDispatcher) Dispatch(_ context.Context, req agentapi.HostActionRequest) (agentapi.HostActionResponse, error) {
	d.mu.Lock()
	d.last = req
	d.calledN++
	d.mu.Unlock()
	select {
	case d.called <- struct{}{}:
	default:
	}
	return agentapi.HostActionResponse{}, nil
}
func (d *captureDispatcher) waitFor(t *testing.T, timeout time.Duration) agentapi.HostActionRequest {
	t.Helper()
	select {
	case <-d.called:
	case <-time.After(timeout):
		t.Fatalf("dispatcher.Dispatch was not called within %v", timeout)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.last
}

func writeImageLock(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "image.lock")
	body := []byte(strings.Join([]string{
		"image_name: test/managed-user:latest",
		"local_dev_image_name: test/managed-user:latest",
		"base_image: ubuntu:24.04",
		"pull_policy: never-implicit-latest",
		"ssh_port: 22",
		"home_mount: /workspace",
		"default_user: workspace",
		"rebuild_mode_default: preserve-home",
		"factory_reset_mode: wipe-/workspace",
		"image_version: v3.0.0-test",
		"mergerfs_version: 2.41.1",
		"tmux_version_min: \"3.4\"",
		"supports_mergerfs: true",
	}, "\n"))
	if err := os.WriteFile(p, body, 0o644); err != nil {
		t.Fatalf("write image.lock: %v", err)
	}
	return p
}

func newTestService(t *testing.T, repo *stubQueueRepo) (*Service, *captureDispatcher) {
	t.Helper()
	disp := newCaptureDispatcher()
	svc := NewService(repo, disp, writeImageLock(t))
	return svc, disp
}

// TestQueueHostAction_InjectsClaudeAccountID 守护 Phase 33 D-04/D-07 闭环：
// QueueHostAction 必须调 ResolveClaudeAccountIDForEntry 并把非空账号 ID 塞进 request.ClaudeAccountID，
// 让 worker.createHost 自动补 claude-state-<id> volume。
func TestQueueHostAction_InjectsClaudeAccountID(t *testing.T) {
	repo := &stubQueueRepo{
		host: repository.Host{
			ID:            "h1",
			UserID:        "u1",
			Status:        "running",
			EntryPassword: "secret",
			SlotKey:       "primary",
			Hostname:      "test-host",
		},
		user:             repository.User{ID: "u1", Username: "alice"},
		resolveAccountID: "acct-42",
		resolveFound:     true,
	}
	svc, disp := newTestService(t, repo)

	if _, err := svc.QueueHostAction(context.Background(), "h1", agentapi.ActionRebuildHost, "admin"); err != nil {
		t.Fatalf("QueueHostAction: %v", err)
	}

	got := disp.waitFor(t, 2*time.Second)
	if got.ClaudeAccountID != "acct-42" {
		t.Errorf("request.ClaudeAccountID = %q, want %q", got.ClaudeAccountID, "acct-42")
	}
	if repo.resolveCalls != 1 {
		t.Errorf("ResolveClaudeAccountIDForEntry should be called exactly once, got %d", repo.resolveCalls)
	}
}

// TestQueueHostAction_NoClaudeAccount_FallsBack 守护 D-07：未解析到 account 时 ClaudeAccountID 留空，
// 不阻塞容器启动（v2.0 旧 host 重建路径）。
func TestQueueHostAction_NoClaudeAccount_FallsBack(t *testing.T) {
	repo := &stubQueueRepo{
		host:         repository.Host{ID: "h1", UserID: "u1", EntryPassword: "secret", Hostname: "h1"},
		user:         repository.User{ID: "u1", Username: "alice"},
		resolveFound: false,
	}
	svc, disp := newTestService(t, repo)

	if _, err := svc.QueueHostAction(context.Background(), "h1", agentapi.ActionRebuildHost, "admin"); err != nil {
		t.Fatalf("QueueHostAction: %v", err)
	}
	got := disp.waitFor(t, 2*time.Second)
	if got.ClaudeAccountID != "" {
		t.Errorf("D-07 fallback: ClaudeAccountID must be empty, got %q", got.ClaudeAccountID)
	}
}

// TestQueueHostAction_ResolveError_DoesNotBlockQueue 守护 D-07 同款语义：
// resolve 抛错时只 log warn，不阻塞排队（运维侧由 audit event + log 兜底）。
func TestQueueHostAction_ResolveError_DoesNotBlockQueue(t *testing.T) {
	repo := &stubQueueRepo{
		host:       repository.Host{ID: "h1", UserID: "u1", EntryPassword: "secret", Hostname: "h1"},
		user:       repository.User{ID: "u1", Username: "alice"},
		resolveErr: errors.New("simulated db error"),
	}
	svc, disp := newTestService(t, repo)

	if _, err := svc.QueueHostAction(context.Background(), "h1", agentapi.ActionRebuildHost, "admin"); err != nil {
		t.Fatalf("QueueHostAction must not return error on resolve failure: %v", err)
	}
	got := disp.waitFor(t, 2*time.Second)
	if got.ClaudeAccountID != "" {
		t.Errorf("on resolve error, ClaudeAccountID must be empty, got %q", got.ClaudeAccountID)
	}
}
