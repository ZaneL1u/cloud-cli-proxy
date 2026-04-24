package cloudclaude

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func Test_WriteLastSession_Schema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "last-session.json")

	snap := LastSessionSnapshot{
		IntendedMode: "auto",
		ActualMode:   "hot-only",
		DowngradeChain: []DowngradeStep{
			{From: "full", To: "hot-only", ReasonCode: "MOUNT_MERGERFS_FAILED", ReasonMessage: "mergerfs 失败"},
		},
		ConflictCount:       3,
		ClaudeAccountID:     "acc_xyz",
		ImageVersion:        "v3.0.0",
		APFSCaseInsensitive: true,
		Timestamp:           time.Date(2026, 4, 19, 9, 0, 0, 0, time.UTC),
	}

	if err := WriteLastSession(path, snap); err != nil {
		t.Fatalf("WriteLastSession error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if v, _ := got["schema_version"].(float64); v != 1 {
		t.Errorf("schema_version = %v, want 1", got["schema_version"])
	}
	if got["actual_mode"] != "hot-only" {
		t.Errorf("actual_mode = %v", got["actual_mode"])
	}
	if got["intended_mode"] != "auto" {
		t.Errorf("intended_mode = %v", got["intended_mode"])
	}
	if v, _ := got["conflict_count"].(float64); v != 3 {
		t.Errorf("conflict_count = %v, want 3", got["conflict_count"])
	}
	if got["apfs_case_insensitive"] != true {
		t.Errorf("apfs_case_insensitive = %v, want true", got["apfs_case_insensitive"])
	}
	if got["claude_account_id"] != "acc_xyz" {
		t.Errorf("claude_account_id = %v", got["claude_account_id"])
	}

	chain, ok := got["downgrade_chain"].([]interface{})
	if !ok || len(chain) != 1 {
		t.Fatalf("downgrade_chain 类型 / 长度异常: %v", got["downgrade_chain"])
	}
	step, ok := chain[0].(map[string]interface{})
	if !ok {
		t.Fatal("downgrade_chain[0] 不是 object")
	}
	if step["from"] != "full" || step["to"] != "hot-only" {
		t.Errorf("downgrade_chain[0] from/to 异常: %v", step)
	}
	if step["reason_code"] != "MOUNT_MERGERFS_FAILED" {
		t.Errorf("reason_code = %v", step["reason_code"])
	}
}

func Test_WriteLastSession_DefaultsSchemaVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ls.json")
	if err := WriteLastSession(path, LastSessionSnapshot{IntendedMode: "auto", ActualMode: "auto"}); err != nil {
		t.Fatalf("WriteLastSession: %v", err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `"schema_version": 1`) {
		t.Errorf("schema_version 默认值未写入: %s", data)
	}
}

func Test_WriteLastSession_Atomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "last-session.json")

	if err := WriteLastSession(path, LastSessionSnapshot{IntendedMode: "auto", ActualMode: "auto"}); err != nil {
		t.Fatalf("WriteLastSession: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("发现 .tmp 残留文件: %s", e.Name())
		}
	}
}

// Phase 32 D-27: 三个新字段 round-trip 验证。
func TestLastSessionSnapshot_NewFieldsRoundTrip(t *testing.T) {
	snap := LastSessionSnapshot{
		SchemaVersion:  1,
		Timestamp:      time.Now().UTC(),
		IntendedMode:   "auto",
		ActualMode:     "full",
		TmuxSession:    "claude-abc12345",
		ClientRole:     "primary",
		ReconnectCount: 3,
	}
	data, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(`"tmux_session":"claude-abc12345"`)) {
		t.Errorf("序列化丢失 tmux_session: %s", data)
	}
	if !bytes.Contains(data, []byte(`"client_role":"primary"`)) {
		t.Errorf("序列化丢失 client_role: %s", data)
	}
	if !bytes.Contains(data, []byte(`"reconnect_count":3`)) {
		t.Errorf("序列化丢失 reconnect_count: %s", data)
	}
	var back LastSessionSnapshot
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back.TmuxSession != "claude-abc12345" || back.ClientRole != "primary" || back.ReconnectCount != 3 {
		t.Errorf("反序列化字段丢失: %+v", back)
	}
}

// Phase 32 D-27: omitempty 隐藏空值。
func TestLastSessionSnapshot_OmitemptyForEmpty(t *testing.T) {
	snap := LastSessionSnapshot{SchemaVersion: 1}
	data, _ := json.Marshal(snap)
	for _, key := range []string{"tmux_session", "client_role", "reconnect_count"} {
		if bytes.Contains(data, []byte(`"`+key+`"`)) {
			t.Errorf("空字段 %s 应被 omitempty 隐藏: %s", key, data)
		}
	}
}

// Phase 36 D-09: OversizedFiles 序列化 round-trip。
func TestLastSession_OversizedFiles_Roundtrip(t *testing.T) {
	snap := LastSessionSnapshot{
		SchemaVersion: 1,
		OversizedFiles: []OversizedFile{
			{Path: "assets/video.mp4", SizeBytes: 60 * 1024 * 1024},
		},
	}
	data, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("Marshal 失败: %v", err)
	}
	var got LastSessionSnapshot
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal 失败: %v", err)
	}
	if got.SchemaVersion != 1 {
		t.Errorf("schema_version 应为 1，got %d", got.SchemaVersion)
	}
	if len(got.OversizedFiles) != 1 {
		t.Fatalf("OversizedFiles 长度应为 1，got %d", len(got.OversizedFiles))
	}
	if got.OversizedFiles[0].Path != "assets/video.mp4" {
		t.Errorf("Path 不匹配: %q", got.OversizedFiles[0].Path)
	}
	if got.OversizedFiles[0].SizeBytes != 60*1024*1024 {
		t.Errorf("SizeBytes 不匹配: %d", got.OversizedFiles[0].SizeBytes)
	}
}

// Phase 36 D-09: 空 OversizedFiles 应被 omitempty 省略。
func TestLastSession_OversizedFiles_OmitemptyEmpty(t *testing.T) {
	snap := LastSessionSnapshot{SchemaVersion: 1}
	data, _ := json.Marshal(snap)
	if strings.Contains(string(data), "oversized_files") {
		t.Error("空 OversizedFiles 应被 omitempty 省略，但 JSON 中出现了 oversized_files 键")
	}
}

// Phase 36 D-09: nil OversizedFiles 应被 omitempty 省略。
func TestLastSession_OversizedFiles_OmitemptyNil(t *testing.T) {
	snap := LastSessionSnapshot{SchemaVersion: 1, OversizedFiles: nil}
	data, _ := json.Marshal(snap)
	if strings.Contains(string(data), "oversized_files") {
		t.Error("nil OversizedFiles 应被 omitempty 省略")
	}
}

// Phase 37 D-12: 三个 promotion 字段 omitempty 验证。
func TestLastSession_PromotionFields_Omitempty(t *testing.T) {
	snap := LastSessionSnapshot{SchemaVersion: 1}
	data, _ := json.Marshal(snap)
	for _, key := range []string{"promotion_count", "promotion_bytes", "promotion_failed_count"} {
		if strings.Contains(string(data), key) {
			t.Errorf("空字段 %s 应被 omitempty 隐藏，但 JSON 中出现了: %s", key, data)
		}
	}
}
