package http

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

func TestRenderBypassConfig(t *testing.T) {
	t.Run("empty input produces version=3 empty rule-sets and stable hash", func(t *testing.T) {
		out, err := RenderBypassConfig(BypassRenderInput{HostID: "h1"}, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var env map[string]interface{}
		if err := json.Unmarshal(out.CIDRsJSON, &env); err != nil {
			t.Fatalf("unmarshal cidrs: %v", err)
		}
		if env["version"].(float64) != 3 {
			t.Errorf("expected version=3, got %v", env["version"])
		}
		if rules, ok := env["rules"].([]interface{}); !ok || len(rules) != 0 {
			t.Errorf("expected empty rules array, got %v", env["rules"])
		}
		if out.ConfigHash == "" {
			t.Error("expected non-empty ConfigHash for empty input")
		}
		// 再渲染一次，hash 必须相同。
		out2, _ := RenderBypassConfig(BypassRenderInput{HostID: "h1"}, nil)
		if out.ConfigHash != out2.ConfigHash {
			t.Errorf("expected stable hash for empty input, got %s vs %s", out.ConfigHash, out2.ConfigHash)
		}
	})

	t.Run("single cidr 10.0.0.0/8 lands in cidrs bucket, domains empty", func(t *testing.T) {
		input := BypassRenderInput{
			HostID: "h1",
			Rules: []repository.BypassRule{
				{ID: "r1", Scope: "host", RuleType: "cidr", Value: "10.0.0.0/8"},
			},
		}
		out, err := RenderBypassConfig(input, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(string(out.CIDRsJSON), "10.0.0.0/8") {
			t.Errorf("cidrs JSON missing 10.0.0.0/8: %s", out.CIDRsJSON)
		}
		// domains rules 数组为空。
		var domainsEnv map[string]interface{}
		if err := json.Unmarshal(out.DomainsJSON, &domainsEnv); err != nil {
			t.Fatalf("unmarshal domains: %v", err)
		}
		if rules, _ := domainsEnv["rules"].([]interface{}); len(rules) != 0 {
			t.Errorf("expected empty domains rules, got %v", rules)
		}
	})

	t.Run("multi domain + domain_suffix split into two rule blocks", func(t *testing.T) {
		input := BypassRenderInput{
			HostID: "h1",
			Rules: []repository.BypassRule{
				{ID: "r1", Scope: "host", RuleType: "domain", Value: "api.internal.corp"},
				{ID: "r2", Scope: "host", RuleType: "domain_suffix", Value: "corp.internal"},
			},
		}
		out, err := RenderBypassConfig(input, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var env map[string]interface{}
		if err := json.Unmarshal(out.DomainsJSON, &env); err != nil {
			t.Fatalf("unmarshal domains: %v", err)
		}
		rules, ok := env["rules"].([]interface{})
		if !ok || len(rules) != 2 {
			t.Fatalf("expected 2 domain rule blocks, got %v", env["rules"])
		}
		// 第一块应含 domain 字段，第二块 domain_suffix 字段。
		first := rules[0].(map[string]interface{})
		second := rules[1].(map[string]interface{})
		if _, ok := first["domain"]; !ok {
			t.Errorf("first block expected 'domain' key, got %v", first)
		}
		if _, ok := second["domain_suffix"]; !ok {
			t.Errorf("second block expected 'domain_suffix' key, got %v", second)
		}
	})

	t.Run("order-stable: same set in different input order → same hash", func(t *testing.T) {
		inputA := BypassRenderInput{
			HostID: "h1",
			Rules: []repository.BypassRule{
				{ID: "ra", Scope: "host", RuleType: "cidr", Value: "10.0.0.0/8"},
				{ID: "rb", Scope: "host", RuleType: "cidr", Value: "192.168.0.0/16"},
			},
		}
		inputB := BypassRenderInput{
			HostID: "h1",
			Rules: []repository.BypassRule{
				{ID: "rb", Scope: "host", RuleType: "cidr", Value: "192.168.0.0/16"},
				{ID: "ra", Scope: "host", RuleType: "cidr", Value: "10.0.0.0/8"},
			},
		}
		outA, _ := RenderBypassConfig(inputA, nil)
		outB, _ := RenderBypassConfig(inputB, nil)
		if outA.ConfigHash != outB.ConfigHash {
			t.Errorf("expected hash stability across input order; A=%s B=%s", outA.ConfigHash, outB.ConfigHash)
		}
		if string(outA.CIDRsJSON) != string(outB.CIDRsJSON) {
			t.Errorf("expected byte-identical cidrs JSON; A=\n%s\nB=\n%s", outA.CIDRsJSON, outB.CIDRsJSON)
		}
	})

	t.Run("prevSnapshot=nil produces all-plus nft diff", func(t *testing.T) {
		input := BypassRenderInput{
			HostID: "h1",
			Rules: []repository.BypassRule{
				{ID: "r1", Scope: "host", RuleType: "cidr", Value: "10.0.0.0/8"},
				{ID: "r2", Scope: "host", RuleType: "ip", Value: "8.8.8.8"},
			},
		}
		out, _ := RenderBypassConfig(input, nil)
		if !strings.Contains(out.NftDiff, "+ 10.0.0.0/8") {
			t.Errorf("expected '+ 10.0.0.0/8' in diff: %s", out.NftDiff)
		}
		if !strings.Contains(out.NftDiff, "+ 8.8.8.8/32") {
			t.Errorf("expected '+ 8.8.8.8/32' in diff: %s", out.NftDiff)
		}
		if strings.Contains(out.NftDiff, "- ") {
			t.Errorf("expected no deletions when prev=nil: %s", out.NftDiff)
		}
	})

	t.Run("prevSnapshot with 1.1.1.1 → diff shows delete and add", func(t *testing.T) {
		prevCIDRsJSON := json.RawMessage(`{"version":3,"rules":[{"ip_cidr":["1.1.1.1/32"]}]}`)
		prev := &repository.BypassSnapshot{WhitelistCIDRsJSON: prevCIDRsJSON}
		input := BypassRenderInput{
			HostID: "h1",
			Rules: []repository.BypassRule{
				{ID: "r1", Scope: "host", RuleType: "ip", Value: "8.8.8.8"},
			},
		}
		out, _ := RenderBypassConfig(input, prev)
		if !strings.Contains(out.NftDiff, "- 1.1.1.1/32") {
			t.Errorf("expected '- 1.1.1.1/32' deletion line, got: %s", out.NftDiff)
		}
		if !strings.Contains(out.NftDiff, "+ 8.8.8.8/32") {
			t.Errorf("expected '+ 8.8.8.8/32' addition line, got: %s", out.NftDiff)
		}
	})

	t.Run("RiskyCount counts IsRisky=true rules", func(t *testing.T) {
		input := BypassRenderInput{
			HostID: "h1",
			Rules: []repository.BypassRule{
				{ID: "r1", Scope: "host", RuleType: "domain_keyword", Value: "abc", IsRisky: true},
				{ID: "r2", Scope: "host", RuleType: "cidr", Value: "10.0.0.0/8", IsRisky: false},
			},
		}
		out, _ := RenderBypassConfig(input, nil)
		if out.RiskyCount != 1 {
			t.Errorf("expected RiskyCount=1, got %d", out.RiskyCount)
		}
		if !strings.Contains(out.Summary, "覆盖 2 条规则") || !strings.Contains(out.Summary, "1 条高风险") {
			t.Errorf("unexpected summary: %s", out.Summary)
		}
	})

	t.Run("preset rules also flow into rendered output", func(t *testing.T) {
		input := BypassRenderInput{
			HostID: "h1",
			Presets: []repository.BypassPreset{
				{
					ID: "p1", Slug: "loopback", IsActive: true, IsForceOn: true,
					Rules: []repository.BypassPresetRule{
						{RuleType: "cidr", Value: "127.0.0.0/8"},
					},
				},
			},
		}
		out, _ := RenderBypassConfig(input, nil)
		if !strings.Contains(string(out.CIDRsJSON), "127.0.0.0/8") {
			t.Errorf("preset rules expected in rendered cidrs JSON: %s", out.CIDRsJSON)
		}
	})
}
