package repository

import (
	"context"
	"strings"
	"testing"
)

func TestUpsertClaudeAccountPersistentVolumeNameSQL_ContainsKeyTokens(t *testing.T) {
	must := []string{
		"UPDATE claude_accounts",
		"SET persistent_volume_name = ?",
		"updated_at = NOW()",
		"WHERE id = ? AND persistent_volume_name IS NULL",
	}
	for _, token := range must {
		if !strings.Contains(upsertClaudeAccountPersistentVolumeNameSQL, token) {
			t.Errorf("upsertClaudeAccountPersistentVolumeNameSQL missing %q\nfull:\n%s", token, upsertClaudeAccountPersistentVolumeNameSQL)
		}
	}
}

func TestCheckClaudeAccountPersistentVolumeNameSQL_ContainsKeyTokens(t *testing.T) {
	must := []string{
		"COALESCE(persistent_volume_name, '')",
		"FROM claude_accounts",
		"WHERE id = ?",
	}
	for _, token := range must {
		if !strings.Contains(checkClaudeAccountPersistentVolumeNameSQL, token) {
			t.Errorf("checkClaudeAccountPersistentVolumeNameSQL missing %q\nfull:\n%s", token, checkClaudeAccountPersistentVolumeNameSQL)
		}
	}
}

func TestUpsertClaudeAccountPersistentVolumeName_EmptyArgs_ReturnsError(t *testing.T) {
	r := &Repository{} // nil db is OK because empty-arg branch returns before touching db
	if err := r.UpsertClaudeAccountPersistentVolumeName(context.Background(), "", "x"); err == nil {
		t.Error("empty accountID must return error")
	}
	if err := r.UpsertClaudeAccountPersistentVolumeName(context.Background(), "x", ""); err == nil {
		t.Error("empty volumeName must return error")
	}
}
