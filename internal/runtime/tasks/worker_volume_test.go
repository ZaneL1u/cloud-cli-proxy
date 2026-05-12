package tasks

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/zanel1u/cloud-cli-proxy/internal/agentapi"
)

func minimalCreateHostRequest(hostID string) agentapi.HostActionRequest {
	return agentapi.HostActionRequest{
		TaskID:        "t1",
		HostID:        hostID,
		Action:        agentapi.ActionCreateHost,
		ImageName:     "img:local",
		DefaultUser:   "workspace",
		HomeMount:     "/workspace",
		ContainerName: "c-test",
		HomeDir:       "/tmp/cloudproxy-test-home-" + hostID,
		// Phase 29.1：buildCreateArgs/syncContainerCredentials 对空 EntryPassword fail-fast，
		// 工厂默认填占位密码以避免无关用例（如 volume 类）误触发空密码守卫；
		// 真要测试空密码分支的用例需显式 override 为 ""。
		EntryPassword: "pw-default-test",
	}
}

func TestHostActionRequest_VolumesOmitempty(t *testing.T) {
	t.Run("empty_volumes_not_serialized", func(t *testing.T) {
		req := agentapi.HostActionRequest{TaskID: "t1", HostID: "h1", Action: agentapi.ActionCreateHost}
		buf, err := json.Marshal(req)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if strings.Contains(string(buf), `"volumes"`) {
			t.Fatalf("empty Volumes must be omitempty, got: %s", buf)
		}
	})

	t.Run("non_empty_volumes_serialized", func(t *testing.T) {
		req := agentapi.HostActionRequest{
			TaskID: "t1", HostID: "h1", Action: agentapi.ActionCreateHost,
			Volumes: []agentapi.VolumeMount{
				{Name: "claude-state-abc", Target: "/var/lib/claude-persist"},
			},
		}
		buf, err := json.Marshal(req)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if !strings.Contains(string(buf), `"volumes"`) {
			t.Fatalf("non-empty Volumes must serialize, got: %s", buf)
		}

		var parsed agentapi.HostActionRequest
		if err := json.Unmarshal(buf, &parsed); err != nil {
			t.Fatalf("round-trip unmarshal: %v", err)
		}
		if len(parsed.Volumes) != 1 || parsed.Volumes[0].Name != "claude-state-abc" {
			t.Fatalf("round-trip lost data: %+v", parsed)
		}
	})

	t.Run("readonly_omitempty_behavior", func(t *testing.T) {
		rw := agentapi.VolumeMount{Name: "v1", Target: "/mnt/a"}
		ro := agentapi.VolumeMount{Name: "v2", Target: "/mnt/b", ReadOnly: true}

		bufRW, _ := json.Marshal(rw)
		bufRO, _ := json.Marshal(ro)

		if strings.Contains(string(bufRW), `"read_only"`) {
			t.Fatalf("ReadOnly=false must be omitempty, got: %s", bufRW)
		}
		if !strings.Contains(string(bufRO), `"read_only":true`) {
			t.Fatalf("ReadOnly=true must serialize, got: %s", bufRO)
		}
	})
}

func TestHostActionRequest_V2Compat(t *testing.T) {
	oldJSON := `{"task_id":"t","host_id":"h","action":"create_host","image_name":"img","default_user":"workspace","home_mount":"/workspace","rebuild_mode":"","container_name":"c","home_dir":"/d","labels":null,"timezone":"","hostname":""}`
	var req agentapi.HostActionRequest
	if err := json.Unmarshal([]byte(oldJSON), &req); err != nil {
		t.Fatalf("v2.0 JSON must unmarshal cleanly: %v", err)
	}
	if req.Volumes != nil {
		t.Fatalf("Volumes must be nil when absent in JSON, got: %+v", req.Volumes)
	}
	if req.ClaudeAccountID != "" {
		t.Fatalf("ClaudeAccountID must be empty when absent from v2 JSON, got: %q", req.ClaudeAccountID)
	}
}

// TestHostActionRequest_ClaudeAccountID_Omitempty 守护 Phase 30 D-09：
// 空值时不得序列化 claude_account_id，避免把「未分配账号」误判为「已分配空串」。
func TestHostActionRequest_ClaudeAccountID_Omitempty(t *testing.T) {
	req := agentapi.HostActionRequest{
		TaskID: "t1", HostID: "h1", Action: agentapi.ActionCreateHost,
	}
	buf, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(buf), `"claude_account_id"`) {
		t.Fatalf("empty ClaudeAccountID must be omitempty, got: %s", buf)
	}
}

// TestHostActionRequest_ClaudeAccountID_RoundTrip 守护 D-09：
// 非空 claude_account_id 必须完整 round-trip，供 Phase 33 worker 组装 volume/容器使用。
func TestHostActionRequest_ClaudeAccountID_RoundTrip(t *testing.T) {
	req := agentapi.HostActionRequest{
		TaskID: "t1", HostID: "h1", Action: agentapi.ActionCreateHost,
		ClaudeAccountID: "acct-42",
	}
	buf, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(buf), `"claude_account_id":"acct-42"`) {
		t.Fatalf("non-empty ClaudeAccountID must serialize as claude_account_id, got: %s", buf)
	}

	var parsed agentapi.HostActionRequest
	if err := json.Unmarshal(buf, &parsed); err != nil {
		t.Fatalf("round-trip unmarshal: %v", err)
	}
	if parsed.ClaudeAccountID != "acct-42" {
		t.Fatalf("round-trip lost ClaudeAccountID: got %q, want %q", parsed.ClaudeAccountID, "acct-42")
	}
}

// TestHostActionRequest_ClaudeAccountID_ForwardCompat 守护 v2→v3 前向兼容：
// v3 服务器返回 claude_account_id 时，旧版消费者不应解析失败。
func TestHostActionRequest_ClaudeAccountID_ForwardCompat(t *testing.T) {
	newJSON := `{"task_id":"t","host_id":"h","action":"create_host","image_name":"img","default_user":"workspace","home_mount":"/workspace","rebuild_mode":"","container_name":"c","home_dir":"/d","labels":null,"timezone":"","hostname":"","claude_account_id":"acct-99"}`
	var req agentapi.HostActionRequest
	if err := json.Unmarshal([]byte(newJSON), &req); err != nil {
		t.Fatalf("v3 JSON with claude_account_id must unmarshal cleanly: %v", err)
	}
	if req.ClaudeAccountID != "acct-99" {
		t.Fatalf("ClaudeAccountID must preserve value across unmarshal, got: %q", req.ClaudeAccountID)
	}
	if req.Volumes != nil {
		t.Fatalf("Volumes must remain nil when absent, got: %+v", req.Volumes)
	}
}

func TestBuildCreateArgs_VolumesMount(t *testing.T) {
	w := &Worker{}
	req := minimalCreateHostRequest("hostvol")
	req.Volumes = []agentapi.VolumeMount{
		{Name: "claude-state-abc", Target: "/var/lib/claude-persist"},
		{Name: "ro-cache", Target: "/mnt/ro", ReadOnly: true},
	}
	args, err := w.buildCreateArgs(req, "c1", "c1", nil)
	if err != nil {
		t.Fatalf("buildCreateArgs: %v", err)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--mount type=volume,src=claude-state-abc,dst=/var/lib/claude-persist") {
		t.Fatalf("missing rw volume mount, args=%v", args)
	}
	if !strings.Contains(joined, "--mount type=volume,src=ro-cache,dst=/mnt/ro,readonly") {
		t.Fatalf("missing ro volume mount, args=%v", args)
	}
}

func TestBuildCreateArgs_EmptyVolumes_NoExtraArgs(t *testing.T) {
	w := &Worker{}
	reqNil := minimalCreateHostRequest("e1")
	argsNil, err := w.buildCreateArgs(reqNil, "cloudproxy-e1", "cloudproxy-e1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(argsNil, " "), "type=volume") {
		t.Fatalf("nil Volumes must not add --mount type=volume args: %v", argsNil)
	}

	reqEmpty := minimalCreateHostRequest("e2")
	reqEmpty.Volumes = []agentapi.VolumeMount{}
	argsEmpty, err := w.buildCreateArgs(reqEmpty, "cloudproxy-e2", "cloudproxy-e2", nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(argsEmpty, " "), "type=volume") {
		t.Fatalf("empty Volumes slice must not add --mount type=volume args: %v", argsEmpty)
	}
}

func TestBuildCreateArgs_InvalidVolumeMount(t *testing.T) {
	w := &Worker{}
	req := minimalCreateHostRequest("bad")
	req.Volumes = []agentapi.VolumeMount{{Name: "", Target: "/mnt"}}
	if _, err := w.buildCreateArgs(req, "c1", "c1", nil); err == nil {
		t.Fatal("expected error for empty volume name")
	}
}

// TestHostActionRequest_VolumeRemove_RoundTrip 守护 D-13/D-25.4：
// Action=volume_remove + Volumes 字段必须完整 round-trip，供 Plan 02 admin handler 触发 host-agent 删 volume。
func TestHostActionRequest_VolumeRemove_RoundTrip(t *testing.T) {
	req := agentapi.HostActionRequest{
		TaskID: "t1", HostID: "h1", Action: agentapi.ActionVolumeRemove,
		Volumes: []agentapi.VolumeMount{{Name: "claude-state-abc"}},
		Labels:  map[string]string{"force": "true"},
	}
	buf, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(buf), `"action":"volume_remove"`) {
		t.Fatalf("ActionVolumeRemove must serialize as volume_remove, got: %s", buf)
	}
	if !strings.Contains(string(buf), `"name":"claude-state-abc"`) {
		t.Fatalf("VolumeMount.Name must round-trip, got: %s", buf)
	}

	var parsed agentapi.HostActionRequest
	if err := json.Unmarshal(buf, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.Action != agentapi.ActionVolumeRemove {
		t.Fatalf("Action lost: got %q, want %q", parsed.Action, agentapi.ActionVolumeRemove)
	}
	if len(parsed.Volumes) != 1 || parsed.Volumes[0].Name != "claude-state-abc" {
		t.Fatalf("Volumes lost: got %+v", parsed.Volumes)
	}
	if parsed.Labels["force"] != "true" {
		t.Fatalf("Labels[force] lost: got %q", parsed.Labels["force"])
	}
}

// TestActionVolumeRemove_StringValue 守护协议契约（host-agent 端用字符串 switch 比较）。
func TestActionVolumeRemove_StringValue(t *testing.T) {
	if string(agentapi.ActionVolumeRemove) != "volume_remove" {
		t.Fatalf("ActionVolumeRemove must equal \"volume_remove\", got %q", agentapi.ActionVolumeRemove)
	}
}
