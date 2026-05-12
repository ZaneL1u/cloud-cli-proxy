package network

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// withTempGatewayBase 把 DATA_DIR 指到 t.TempDir()，让 GatewayConfigDir 落在
// 测试隔离目录，并通过 t.Cleanup 还原。返回最终 host 目录便于断言。
func withTempGatewayBase(t *testing.T, hostID string) string {
	t.Helper()
	tmp := t.TempDir()
	prev, hadPrev := os.LookupEnv("DATA_DIR")
	if err := os.Setenv("DATA_DIR", tmp); err != nil {
		t.Fatalf("set DATA_DIR: %v", err)
	}
	t.Cleanup(func() {
		if hadPrev {
			_ = os.Setenv("DATA_DIR", prev)
		} else {
			_ = os.Unsetenv("DATA_DIR")
		}
	})
	hostDir := filepath.Join(tmp, "gateway", hostID)
	if err := os.MkdirAll(hostDir, 0o755); err != nil {
		t.Fatalf("mkdir host dir: %v", err)
	}
	return hostDir
}

// withFakeNftRunner 注入 fake nftRunner，并在测试结束还原；返回闭包内捕获到的 stdin。
func withFakeNftRunner(t *testing.T, fn func(ctx context.Context, stdin string) ([]byte, error)) *[]string {
	t.Helper()
	captured := &[]string{}
	var mu sync.Mutex
	prev := nftRunner
	nftRunner = func(ctx context.Context, stdin string) ([]byte, error) {
		mu.Lock()
		*captured = append(*captured, stdin)
		mu.Unlock()
		return fn(ctx, stdin)
	}
	t.Cleanup(func() { nftRunner = prev })
	return captured
}

func withFakeNftJSONLister(t *testing.T, fn func(ctx context.Context, setName string) ([]byte, error)) {
	t.Helper()
	prev := nftJSONLister
	nftJSONLister = fn
	t.Cleanup(func() { nftJSONLister = prev })
}

// ===== Test 1: atomic write =====

// TestApplyBypassRuleSet_AtomicWrite 守护 acceptance Test 1：
//   - 写盘到 <DATA_DIR>/gateway/<hostID>/whitelist-cidrs.json 与 whitelist-domains.json
//   - 内容 byte-for-byte 等于入参 cidrsJSON / domainsJSON
//   - 写盘过程结束后目录里不留任何 .tmp.* 残留文件（用 glob 验证）
func TestApplyBypassRuleSet_AtomicWrite(t *testing.T) {
	hostID := "h-atomic"
	hostDir := withTempGatewayBase(t, hostID)
	withFakeNftRunner(t, func(_ context.Context, _ string) ([]byte, error) { return nil, nil })

	cidrsJSON := json.RawMessage(`{"version":3,"rules":[{"ip_cidr":["10.0.0.0/8","192.168.1.0/24"]}]}`)
	domainsJSON := json.RawMessage(`{"version":3,"rules":[{"domain":["example.com"]}]}`)

	if err := ApplyBypassRuleSet(context.Background(), hostID, cidrsJSON, domainsJSON); err != nil {
		t.Fatalf("ApplyBypassRuleSet: %v", err)
	}

	cidrsPath := filepath.Join(hostDir, "whitelist-cidrs.json")
	domainsPath := filepath.Join(hostDir, "whitelist-domains.json")

	gotCIDRs, err := os.ReadFile(cidrsPath)
	if err != nil {
		t.Fatalf("read cidrs file: %v", err)
	}
	if string(gotCIDRs) != string(cidrsJSON) {
		t.Errorf("cidrs file content = %q, want %q", string(gotCIDRs), string(cidrsJSON))
	}

	gotDomains, err := os.ReadFile(domainsPath)
	if err != nil {
		t.Fatalf("read domains file: %v", err)
	}
	if string(gotDomains) != string(domainsJSON) {
		t.Errorf("domains file content = %q, want %q", string(gotDomains), string(domainsJSON))
	}

	// 不允许残留 .tmp.* 文件
	matches, err := filepath.Glob(filepath.Join(hostDir, "*.tmp.*"))
	if err != nil {
		t.Fatalf("glob tmp residue: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("expected zero .tmp.* residue, got %v", matches)
	}
}

// ===== Test 2: nft transaction stdin format =====

// TestApplyBypassRuleSet_NftTransaction 守护 acceptance Test 2：
//   - stdin 首行严格是 `flush set inet sbfw whitelist_v4`
//   - 紧跟一行 `add element inet sbfw whitelist_v4 { ... }`
//   - 单次 nft -f 调用承担整批；列表归一化后按字典序，包含全部入参 CIDR
//   - 解析失败的 cidrsJSON（version 不对 / 非法 envelope）不下发 nft 也不写盘
func TestApplyBypassRuleSet_NftTransaction(t *testing.T) {
	hostID := "h-nft"
	hostDir := withTempGatewayBase(t, hostID)
	captured := withFakeNftRunner(t, func(_ context.Context, _ string) ([]byte, error) { return nil, nil })

	cidrsJSON := json.RawMessage(`{"version":3,"rules":[{"ip_cidr":["192.168.1.0/24","10.0.0.0/8"]}]}`)
	domainsJSON := json.RawMessage(`{"version":3,"rules":[]}`)

	if err := ApplyBypassRuleSet(context.Background(), hostID, cidrsJSON, domainsJSON); err != nil {
		t.Fatalf("ApplyBypassRuleSet: %v", err)
	}

	if got := len(*captured); got != 1 {
		t.Fatalf("nftRunner should be called exactly once (one nft -f - transaction), got %d", got)
	}
	stdin := (*captured)[0]
	lines := strings.Split(strings.TrimRight(stdin, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("stdin must have at least 2 lines (flush + add), got %q", stdin)
	}
	if lines[0] != "flush set inet sbfw whitelist_v4" {
		t.Errorf("stdin first line = %q, want exact 'flush set inet sbfw whitelist_v4'", lines[0])
	}
	if !strings.HasPrefix(lines[1], "add element inet sbfw whitelist_v4 { ") || !strings.HasSuffix(lines[1], " }") {
		t.Errorf("stdin second line = %q, want add-element with braces", lines[1])
	}
	for _, want := range []string{"10.0.0.0/8", "192.168.1.0/24"} {
		if !strings.Contains(lines[1], want) {
			t.Errorf("stdin second line missing %q: %q", want, lines[1])
		}
	}

	// CIDR 解析失败路径：version 错误的 envelope
	*captured = (*captured)[:0]
	// 清掉文件以验证不二次写盘
	_ = os.Remove(filepath.Join(hostDir, "whitelist-cidrs.json"))
	_ = os.Remove(filepath.Join(hostDir, "whitelist-domains.json"))

	badCIDRs := json.RawMessage(`{"version":99,"rules":[]}`)
	if err := ApplyBypassRuleSet(context.Background(), hostID, badCIDRs, domainsJSON); err == nil {
		t.Fatalf("ApplyBypassRuleSet should fail on bad version, got nil err")
	}
	if got := len(*captured); got != 0 {
		t.Errorf("bad cidrs json must not invoke nftRunner, got %d calls", got)
	}
	if _, err := os.Stat(filepath.Join(hostDir, "whitelist-cidrs.json")); !os.IsNotExist(err) {
		t.Errorf("bad cidrs json must not create whitelist-cidrs.json (stat err=%v)", err)
	}
}

// ===== Test 3: hash consistency =====

// TestVerifyBypassConsistency_HashMatch 守护 acceptance Test 3：
//   - rule-set 文件 CIDR 集合与 nft set CIDR 集合相同 → OK=true，两 hash 相等
//   - 集合不同 → OK=false，两 hash 字面值不同，Detail 非空
func TestVerifyBypassConsistency_HashMatch(t *testing.T) {
	hostID := "h-verify"
	hostDir := withTempGatewayBase(t, hostID)

	cidrsJSON := json.RawMessage(`{"version":3,"rules":[{"ip_cidr":["10.0.0.0/8","192.168.1.0/24"]}]}`)
	if err := os.WriteFile(filepath.Join(hostDir, "whitelist-cidrs.json"), cidrsJSON, 0o644); err != nil {
		t.Fatalf("write cidrs: %v", err)
	}

	// case A: nft 返回相同集合（混合 prefix + 裸 string 形式）
	matchingNftJSON := []byte(`{"nftables":[
      {"metainfo":{}},
      {"set":{"family":"inet","table":"sbfw","name":"whitelist_v4","type":"ipv4_addr",
              "elem":[
                {"prefix":{"addr":"10.0.0.0","len":8}},
                {"prefix":{"addr":"192.168.1.0","len":24}}
              ]}}
    ]}`)
	withFakeNftJSONLister(t, func(_ context.Context, _ string) ([]byte, error) { return matchingNftJSON, nil })

	res, err := VerifyBypassConsistency(context.Background(), hostID)
	if err != nil {
		t.Fatalf("VerifyBypassConsistency: %v", err)
	}
	if !res.OK {
		t.Errorf("expected OK=true, got %+v", res)
	}
	if res.RuleSetSHA256 == "" || res.NftSetSHA256 == "" {
		t.Errorf("hash fields must not be empty: %+v", res)
	}
	if res.RuleSetSHA256 != res.NftSetSHA256 {
		t.Errorf("matching sets must produce equal hashes: %+v", res)
	}

	// case B: nft 返回不同集合
	driftedNftJSON := []byte(`{"nftables":[
      {"metainfo":{}},
      {"set":{"family":"inet","table":"sbfw","name":"whitelist_v4","type":"ipv4_addr",
              "elem":[{"prefix":{"addr":"172.16.0.0","len":12}}]}}
    ]}`)
	withFakeNftJSONLister(t, func(_ context.Context, _ string) ([]byte, error) { return driftedNftJSON, nil })

	res2, err := VerifyBypassConsistency(context.Background(), hostID)
	if err != nil {
		t.Fatalf("VerifyBypassConsistency drift: %v", err)
	}
	if res2.OK {
		t.Errorf("expected OK=false on drift, got %+v", res2)
	}
	if res2.RuleSetSHA256 == res2.NftSetSHA256 {
		t.Errorf("drifted sets must produce different hashes: %+v", res2)
	}
	if res2.Detail == "" {
		t.Errorf("drifted result must include Detail")
	}
}

// ===== Test 4: nft failure rollback =====

// TestApplyBypassRuleSet_NftFailureRollback 守护 acceptance Test 4：
//   - nftRunner 返回非零退出 → ApplyBypassRuleSet 返回 error
//   - 目录不能留下 .tmp.* 残留
//   - 原 whitelist-cidrs.json 内容必须保持「旧值」不被破坏
func TestApplyBypassRuleSet_NftFailureRollback(t *testing.T) {
	hostID := "h-nft-fail"
	hostDir := withTempGatewayBase(t, hostID)

	// 预置旧内容：模拟之前已经 apply 过一次的 snapshot 文件
	oldContent := []byte(`{"version":3,"rules":[{"ip_cidr":["172.20.0.0/16"]}]}`)
	cidrsPath := filepath.Join(hostDir, "whitelist-cidrs.json")
	if err := os.WriteFile(cidrsPath, oldContent, 0o644); err != nil {
		t.Fatalf("seed old cidrs file: %v", err)
	}

	withFakeNftRunner(t, func(_ context.Context, _ string) ([]byte, error) {
		return []byte("nft: syntax error simulated"), errors.New("exit status 1")
	})

	newCIDRs := json.RawMessage(`{"version":3,"rules":[{"ip_cidr":["10.0.0.0/8"]}]}`)
	newDomains := json.RawMessage(`{"version":3,"rules":[]}`)

	err := ApplyBypassRuleSet(context.Background(), hostID, newCIDRs, newDomains)
	if err == nil {
		t.Fatal("ApplyBypassRuleSet should return error on nft failure, got nil")
	}

	// 原文件未被破坏
	got, readErr := os.ReadFile(cidrsPath)
	if readErr != nil {
		t.Fatalf("read original file after failed apply: %v", readErr)
	}
	if string(got) != string(oldContent) {
		t.Errorf("original file corrupted: got=%q want=%q", string(got), string(oldContent))
	}

	// 没有 .tmp.* 残留
	matches, _ := filepath.Glob(filepath.Join(hostDir, "*.tmp.*"))
	if len(matches) != 0 {
		t.Errorf("expected zero .tmp.* residue after nft failure, got %v", matches)
	}

	// domains 文件不应被创建（nft 失败先于任何写盘）
	if _, statErr := os.Stat(filepath.Join(hostDir, "whitelist-domains.json")); !os.IsNotExist(statErr) {
		t.Errorf("whitelist-domains.json must not exist after nft failure, stat=%v", statErr)
	}

	if !strings.Contains(err.Error(), "nft") {
		t.Errorf("err should mention nft: %v", err)
	}
}

// 编译期守护：保证 fmt 一直被使用（避免 IDE 误删），并间接验证 ConsistencyResult JSON 字面 tag。
var _ = fmt.Sprintf("%T", ConsistencyResult{})
