package tasks

import (
	"context"
	"strings"
	"testing"
)

// TestBuildCreateArgs_EmptyEntryPassword_ReturnsError 锁定 Phase 29.1：
// worker 不得再对空 entry_password 进行 "workspace" fallback；空值必须立即 error。
func TestBuildCreateArgs_EmptyEntryPassword_ReturnsError(t *testing.T) {
	w := &Worker{}
	req := minimalCreateHostRequest("h-empty-pw")
	req.EntryPassword = ""

	args, err := w.buildCreateArgs(req, "c1", "c1", nil)
	if err == nil {
		t.Fatal("expected error for empty EntryPassword, got nil")
	}
	if !strings.Contains(err.Error(), "entry_password is empty") {
		t.Errorf("error message should mention entry_password is empty; got: %v", err)
	}
	if len(args) != 0 {
		t.Errorf("expected empty args slice on error, got len=%d", len(args))
	}
}

// TestSyncContainerCredentials_EmptyEntryPassword_RecordsEventNoChpasswd：
// 空 entry_password 必须只记 runtime.entry_password_missing 事件、绝不执行 docker exec chpasswd。
// fc.log 由 setupInjectTest 注入的 fake execInContainer 收集；本路径的 fail-fast 分支
// 在 RecordEvent 之后立即 return，因此 fc.log 应为空（绝不出现 chpasswd 字样）。
func TestSyncContainerCredentials_EmptyEntryPassword_RecordsEventNoChpasswd(t *testing.T) {
	w, fc, repo := setupInjectTest(t, "")
	req := minimalCreateHostRequest("h-sync-empty")
	req.EntryPassword = ""

	w.syncContainerCredentials(context.Background(), req, "cx")

	if !hasEventType(repo.events, "runtime.entry_password_missing") {
		t.Errorf("expected runtime.entry_password_missing event when EntryPassword is empty; events=%+v", repo.events)
	}
	if strings.Contains(strings.Join(fc.log, "\n"), "chpasswd") {
		t.Error("syncContainerCredentials must not invoke chpasswd when EntryPassword is empty")
	}
}
