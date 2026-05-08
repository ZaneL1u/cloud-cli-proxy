package doctor

import (
	"context"
	"fmt"
	"testing"
)

// multiRunner 返回按顺序预设的不同输出，用于测试多步骤 check 函数。
type multiRunner struct {
	outs []string
	idx  int
}

func (m *multiRunner) RunScript(name, script string) (string, string, error) {
	if m.idx >= len(m.outs) {
		return "", "", fmt.Errorf("multiRunner: no more outputs (call %d)", m.idx)
	}
	out := m.outs[m.idx]
	m.idx++
	return out, "", nil
}

// ─── vscode_server_process ────────────────────────────────────────────

func TestCheckVSCodeServerProcess_Running_Pass(t *testing.T) {
	r := &fakeRunner{out: "running\n"}
	c := checkVSCodeServerProcess(context.Background(), r)
	if c.Status != StatusPass {
		t.Errorf("running 应 Pass，实际 %s", c.Status)
	}
}

func TestCheckVSCodeServerProcess_Stopped_Skip(t *testing.T) {
	r := &fakeRunner{out: "stopped\n"}
	c := checkVSCodeServerProcess(context.Background(), r)
	if c.Status != StatusSkip {
		t.Errorf("stopped 应 Skip，实际 %s", c.Status)
	}
}

func TestCheckVSCodeServerProcess_NilRunner_Skip(t *testing.T) {
	c := checkVSCodeServerProcess(context.Background(), nil)
	if c.Status != StatusSkip {
		t.Errorf("nil runner 应 Skip，实际 %s", c.Status)
	}
}

func TestCheckVSCodeServerProcess_Error_Skip(t *testing.T) {
	r := &fakeRunner{err: fmt.Errorf("ssh broken")}
	c := checkVSCodeServerProcess(context.Background(), r)
	if c.Status != StatusSkip {
		t.Errorf("error 应 Skip，实际 %s", c.Status)
	}
}

// ─── vscode_server_port ──────────────────────────────────────────────

func TestCheckVSCodeServerPort_Listening_Pass(t *testing.T) {
	r := &fakeRunner{out: "listening\n"}
	c := checkVSCodeServerPort(context.Background(), r)
	if c.Status != StatusPass {
		t.Errorf("listening 应 Pass，实际 %s", c.Status)
	}
}

func TestCheckVSCodeServerPort_NotListening_Warn(t *testing.T) {
	r := &fakeRunner{out: "not_listening\n"}
	c := checkVSCodeServerPort(context.Background(), r)
	if c.Status != StatusWarn {
		t.Errorf("not_listening 应 Warn，实际 %s", c.Status)
	}
	if c.Code != "SSH_VSCODE_PORT_NOT_LISTENING" {
		t.Errorf("Code 应为 SSH_VSCODE_PORT_NOT_LISTENING，实际 %q", c.Code)
	}
}

func TestCheckVSCodeServerPort_NilRunner_Skip(t *testing.T) {
	c := checkVSCodeServerPort(context.Background(), nil)
	if c.Status != StatusSkip {
		t.Errorf("nil runner 应 Skip，实际 %s", c.Status)
	}
}

// ─── vscode_server_disk ──────────────────────────────────────────────

func TestCheckVSCodeServerDisk_Normal_Pass(t *testing.T) {
	r := &fakeRunner{out: "200M\t/home/user/.vscode-server/\n"}
	c := checkVSCodeServerDisk(context.Background(), r)
	if c.Status != StatusPass {
		t.Errorf("200M 应 Pass，实际 %s", c.Status)
	}
}

func TestCheckVSCodeServerDisk_Warn(t *testing.T) {
	r := &fakeRunner{out: "800M\t/home/user/.vscode-server/\n"}
	c := checkVSCodeServerDisk(context.Background(), r)
	if c.Status != StatusWarn {
		t.Errorf("800M 应 Warn，实际 %s", c.Status)
	}
	if c.Code != "DISK_VSCODE_SERVER_WARN" {
		t.Errorf("Code 应为 DISK_VSCODE_SERVER_WARN，实际 %q", c.Code)
	}
	if c.Details == nil {
		t.Fatal("Details 不应为 nil")
	}
	if _, ok := c.Details["cleanup_light"]; !ok {
		t.Error("Details 应包含 cleanup_light")
	}
}

func TestCheckVSCodeServerDisk_Bloat_Fail(t *testing.T) {
	r := &fakeRunner{out: "3.0G\t/home/user/.vscode-server/\n"}
	c := checkVSCodeServerDisk(context.Background(), r)
	if c.Status != StatusFail {
		t.Errorf("3.0G 应 Fail，实际 %s", c.Status)
	}
	if c.Code != "DISK_VSCODE_SERVER_BLOAT" {
		t.Errorf("Code 应为 DISK_VSCODE_SERVER_BLOAT，实际 %q", c.Code)
	}
	if c.Details == nil {
		t.Fatal("Details 不应为 nil")
	}
	if _, ok := c.Details["cleanup_full"]; !ok {
		t.Error("Details 应包含 cleanup_full")
	}
}

func TestCheckVSCodeServerDisk_NotFound_Skip(t *testing.T) {
	r := &fakeRunner{out: "NOT_FOUND"}
	c := checkVSCodeServerDisk(context.Background(), r)
	if c.Status != StatusSkip {
		t.Errorf("NOT_FOUND 应 Skip，实际 %s", c.Status)
	}
}

func TestCheckVSCodeServerDisk_NilRunner_Skip(t *testing.T) {
	c := checkVSCodeServerDisk(context.Background(), nil)
	if c.Status != StatusSkip {
		t.Errorf("nil runner 应 Skip，实际 %s", c.Status)
	}
}

func TestCheckVSCodeServerDisk_Garbage_Pass(t *testing.T) {
	// "garbage" 无法被 parseDuHumanToMB 解析，返回 0MB → 视为空目录 → Pass
	r := &fakeRunner{out: "garbage"}
	c := checkVSCodeServerDisk(context.Background(), r)
	if c.Status != StatusPass {
		t.Errorf("garbage 解析为 0MB 应 Pass，实际 %s", c.Status)
	}
}

// ─── forwarding_socket ───────────────────────────────────────────────

func TestCheckForwardingSocket_Found_Pass(t *testing.T) {
	r := &fakeRunner{out: "found\n"}
	c := checkForwardingSocket(context.Background(), r)
	if c.Status != StatusPass {
		t.Errorf("found 应 Pass，实际 %s", c.Status)
	}
}

func TestCheckForwardingSocket_NotFound_Skip(t *testing.T) {
	r := &fakeRunner{out: "not_found\n"}
	c := checkForwardingSocket(context.Background(), r)
	if c.Status != StatusSkip {
		t.Errorf("not_found 应 Skip，实际 %s", c.Status)
	}
}

func TestCheckForwardingSocket_NilRunner_Skip(t *testing.T) {
	c := checkForwardingSocket(context.Background(), nil)
	if c.Status != StatusSkip {
		t.Errorf("nil runner 应 Skip，实际 %s", c.Status)
	}
}

// ─── forwarding_blocked ──────────────────────────────────────────────

func TestCheckForwardingBlocked_SocketMissing_Skip(t *testing.T) {
	// 第一次调用检查 socket → not_found → 跳过防火墙检测
	r := &fakeRunner{out: "not_found\n"}
	c := checkForwardingBlocked(context.Background(), r)
	if c.Status != StatusSkip {
		t.Errorf("socket 不存在应 Skip，实际 %s", c.Status)
	}
}

func TestCheckForwardingBlocked_NoDrop_Pass(t *testing.T) {
	// 两次调用都返回 "found"：第一次 socket 存在，第二次 Atoi("found")=0 → Pass
	r := &fakeRunner{out: "found\n"}
	c := checkForwardingBlocked(context.Background(), r)
	if c.Status != StatusPass {
		t.Errorf("无 DROP 规则应 Pass，实际 %s", c.Status)
	}
}

func TestCheckForwardingBlocked_WithDrop_Warn(t *testing.T) {
	// 第一次 "found"（socket 存在），第二次 "3"（3 条 DROP 规则）
	r := &multiRunner{outs: []string{"found\n", "3\n"}}
	c := checkForwardingBlocked(context.Background(), r)
	if c.Status != StatusWarn {
		t.Errorf("有 DROP 规则应 Warn，实际 %s", c.Status)
	}
	if c.Code != "SSH_FORWARDING_BLOCKED" {
		t.Errorf("Code 应为 SSH_FORWARDING_BLOCKED，实际 %q", c.Code)
	}
	if c.Details == nil {
		t.Fatal("Details 不应为 nil")
	}
	if c.Details["drop_rules"] != 3 {
		t.Errorf("drop_rules 应为 3，实际 %v", c.Details["drop_rules"])
	}
}

func TestCheckForwardingBlocked_NilRunner_Skip(t *testing.T) {
	c := checkForwardingBlocked(context.Background(), nil)
	if c.Status != StatusSkip {
		t.Errorf("nil runner 应 Skip，实际 %s", c.Status)
	}
}
