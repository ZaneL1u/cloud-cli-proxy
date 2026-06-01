package repository

import (
	"strings"
	"testing"
)

func TestGetHostWithClaudeAccountSQL_ContainsLeftJoinTokens(t *testing.T) {
	must := []string{
		"FROM hosts h",
		"LEFT JOIN claude_accounts ca ON ca.host_id = h.id",
		"COALESCE(ca.persistent_volume_name, '')",
		"WHERE h.id = ?",
		"LIMIT 1",
	}
	for _, token := range must {
		if !strings.Contains(getHostWithClaudeAccountSQL, token) {
			t.Errorf("getHostWithClaudeAccountSQL missing %q\nfull:\n%s", token, getHostWithClaudeAccountSQL)
		}
	}
}

func TestLockClaudeAccountForDeleteSQL_HasForUpdate(t *testing.T) {
	must := []string{
		"FROM claude_accounts",
		"WHERE id = ?",
		"COALESCE(persistent_volume_name, '')",
	}
	for _, token := range must {
		if !strings.Contains(lockClaudeAccountForDeleteSQL, token) {
			t.Errorf("lockClaudeAccountForDeleteSQL missing %q\nfull:\n%s", token, lockClaudeAccountForDeleteSQL)
		}
	}
}

func TestDeleteClaudeAccountSQL_IsExactDelete(t *testing.T) {
	want := `DELETE FROM claude_accounts WHERE id = ?`
	if deleteClaudeAccountSQL != want {
		t.Errorf("deleteClaudeAccountSQL must equal %q, got %q", want, deleteClaudeAccountSQL)
	}
}

func TestHostWithClaudeAccount_EmbedsHost(t *testing.T) {
	var item HostWithClaudeAccount
	item.ID = "h-1"
	item.PersistentVolumeName = "claude-state-acct-42"
	if item.ID != "h-1" {
		t.Errorf("Host.ID assignment via embed must work")
	}
	if item.PersistentVolumeName != "claude-state-acct-42" {
		t.Errorf("PersistentVolumeName field must be string-typed")
	}
}
