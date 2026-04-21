package doctor

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestDowngradeBannerRendersChain — RESEARCH §8.5 / ROADMAP §Phase 34 SC#6：
// 降级链必须逐条输出 `[降级] <from> → <to>` + 含 reason_code 字面量。
func TestDowngradeBannerRendersChain(t *testing.T) {
	banner := &DowngradeBanner{
		SnapshotAgeSeconds: 300,
		IntendedMode:       "full",
		ActualMode:         "sshfs-only",
		DowngradeChain: []DowngradeStep{
			{From: "full", To: "hot-only", ReasonCode: "MOUNT_MERGERFS_FAILED", ReasonMessage: "merge failed"},
			{From: "hot-only", To: "sshfs-only", ReasonCode: "MOUNT_AUTO_DOWNGRADED", ReasonMessage: "hot sync failed"},
		},
	}
	out := renderDowngradeBanner(banner)
	for _, want := range []string{
		"[降级] full → hot-only",
		"[降级] hot-only → sshfs-only",
		"MOUNT_MERGERFS_FAILED",
		"MOUNT_AUTO_DOWNGRADED",
		"意图=full",
		"实际=sshfs-only",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("banner 未包含 %q，实际:\n%s", want, out)
		}
	}
}

// TestDowngradeBannerNil_ShowsLastSessionMissing — CONTEXT D-13 fallback。
func TestDowngradeBannerNil_ShowsLastSessionMissing(t *testing.T) {
	out := renderDowngradeBanner(nil)
	if !strings.Contains(out, "STATE_LAST_SESSION_MISSING") {
		t.Errorf("nil banner 应提示 STATE_LAST_SESSION_MISSING，实际:\n%s", out)
	}
	if !strings.Contains(out, "[!]") {
		t.Errorf("nil banner 应含 [!] 符号（informational）:\n%s", out)
	}
}

// TestRenderTextContainsNextAction — RESEARCH §8.6 / M14 终验：所有 warn/fail 必含「建议:」子串。
func TestRenderTextContainsNextAction(t *testing.T) {
	r := &Report{
		SchemaVersion: 1,
		StartedAt:     time.Now(),
		Summary:       Summary{Total: 2, Fail: 1, Warn: 1},
		Checks: []Check{
			{Domain: "mount", Name: "mergerfs_branches", Status: StatusFail,
				Code: "MOUNT_MERGERFS_FAILED", Message: "参数缺失", NextAction: "doctor mount --fix"},
			{Domain: "network", Name: "dns_resolve", Status: StatusWarn,
				Code: "SYSTEM_DNS_RESOLVE_FAILED", Message: "查询失败", NextAction: "刷新 DNS 缓存"},
		},
	}
	out := RenderText(r, true /*noColor*/)
	for _, line := range strings.Split(out, "\n") {
		l := strings.TrimSpace(line)
		if strings.HasPrefix(l, "[fail]") || strings.HasPrefix(l, "[warn]") ||
			strings.HasPrefix(l, "[✗]") || strings.HasPrefix(l, "[!]") {
			// 放过降级 banner 中的 "[!]" 前缀（第一屏 informational，包含 "错误码:" 但可能不带 "建议:"）
			if strings.Contains(l, "未找到上次会话快照") {
				continue
			}
			if !strings.Contains(l, "建议:") {
				t.Errorf("warn/fail 行缺 '建议:'：%s", l)
			}
		}
	}
}

// TestJSONSchemaV1Lock — RESEARCH §5.1：schema_version 必须始终为 1，不带 omitempty。
func TestJSONSchemaV1Lock(t *testing.T) {
	r := &Report{SchemaVersion: 1, Summary: Summary{}, Checks: []Check{}}
	raw, err := RenderJSON(r)
	if err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"schema_version", "summary", "checks", "started_at"} {
		if _, ok := m[key]; !ok {
			t.Errorf("JSON 缺核心字段 %q；实际:\n%s", key, raw)
		}
	}
	if v, ok := m["schema_version"].(float64); !ok || v != 1 {
		t.Errorf("schema_version 必须为 1，实际 %v", m["schema_version"])
	}
}

// TestRenderText_PassCheck_NoNextActionSuffix — 纯 Pass check 不需要「建议:」子串。
func TestRenderText_PassCheck_NoNextActionSuffix(t *testing.T) {
	r := &Report{
		SchemaVersion: 1,
		Summary:       Summary{Total: 1, Pass: 1},
		Checks: []Check{
			{Domain: "network", Name: "dns_resolve", Status: StatusPass, Message: "OK"},
		},
	}
	out := RenderText(r, true)
	if strings.Contains(out, "建议:") {
		t.Errorf("Pass 行不应含 '建议:'：%s", out)
	}
}
