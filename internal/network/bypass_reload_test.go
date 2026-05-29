package network

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

// withFakeDockerExecNft 注入 fake dockerExecNftHook，捕获调用参数。
type capturedDockerNftCall struct {
	HostID    string
	NftScript string
}

func withFakeDockerExecNft(t *testing.T, fn func(ctx context.Context, hostID, nftScript string) error) *[]capturedDockerNftCall {
	t.Helper()
	captured := &[]capturedDockerNftCall{}
	prev := dockerExecNftHook
	dockerExecNftHook = func(ctx context.Context, hostID, nftScript string) error {
		*captured = append(*captured, capturedDockerNftCall{HostID: hostID, NftScript: nftScript})
		return fn(ctx, hostID, nftScript)
	}
	t.Cleanup(func() { dockerExecNftHook = prev })
	return captured
}

// withFakeDockerWriteFile 注入 fake dockerWriteFileHook，捕获写入内容。
type capturedDockerWrite struct {
	HostID   string
	Filename string
	Data     []byte
}

func withFakeDockerWriteFile(t *testing.T, fn func(ctx context.Context, hostID, filename string, data []byte) error) *[]capturedDockerWrite {
	t.Helper()
	captured := &[]capturedDockerWrite{}
	prev := dockerWriteFileHook
	dockerWriteFileHook = func(ctx context.Context, hostID, filename string, data []byte) error {
		*captured = append(*captured, capturedDockerWrite{HostID: hostID, Filename: filename, Data: data})
		if fn != nil {
			return fn(ctx, hostID, filename, data)
		}
		return nil
	}
	t.Cleanup(func() { dockerWriteFileHook = prev })
	return captured
}

func withFakeDockerReadFile(t *testing.T, fn func(ctx context.Context, hostID, filename string) ([]byte, error)) {
	t.Helper()
	prev := dockerReadFileHook
	dockerReadFileHook = fn
	t.Cleanup(func() { dockerReadFileHook = prev })
}

func withFakeDockerListNftSet(t *testing.T, fn func(ctx context.Context, hostID string) ([]byte, error)) {
	t.Helper()
	prev := dockerListNftSetHook
	dockerListNftSetHook = fn
	t.Cleanup(func() { dockerListNftSetHook = prev })
}

// ===== Test 1: nft transaction script format =====

// TestApplyBypassRuleSet_NftTransaction 守护 acceptance Test 2：
//   - nft 脚本首行严格是 `flush set ip cloudproxy whitelist_v4`
//   - 紧跟一行 `add element ip cloudproxy whitelist_v4 { ... }`
//   - 列表归一化后按字典序，包含全部入参 CIDR
//   - 文件写入通过 dockerWriteFileHook 进行
//   - 解析失败的 cidrsJSON 不下发 nft 也不写盘
func TestApplyBypassRuleSet_NftTransaction(t *testing.T) {
	hostID := "h-nft"
	nftCalls := withFakeDockerExecNft(t, func(_ context.Context, _ string, _ string) error { return nil })
	fileCalls := withFakeDockerWriteFile(t, nil)

	cidrsJSON := json.RawMessage(`{"version":3,"rules":[{"ip_cidr":["192.168.1.0/24","10.0.0.0/8"]}]}`)
	domainsJSON := json.RawMessage(`{"version":3,"rules":[]}`)

	if err := ApplyBypassRuleSet(context.Background(), hostID, cidrsJSON, domainsJSON); err != nil {
		t.Fatalf("ApplyBypassRuleSet: %v", err)
	}

	if got := len(*nftCalls); got != 1 {
		t.Fatalf("dockerExecNftHook should be called exactly once, got %d", got)
	}
	call := (*nftCalls)[0]
	if call.HostID != hostID {
		t.Errorf("hostID = %q, want %q", call.HostID, hostID)
	}

	lines := strings.Split(strings.TrimRight(call.NftScript, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("nft script must have at least 2 lines (flush + add), got %q", call.NftScript)
	}
	if lines[0] != "flush set ip cloudproxy whitelist_v4" {
		t.Errorf("first line = %q, want 'flush set ip cloudproxy whitelist_v4'", lines[0])
	}
	if !strings.HasPrefix(lines[1], "add element ip cloudproxy whitelist_v4 { ") || !strings.HasSuffix(lines[1], " }") {
		t.Errorf("second line = %q, want add-element with braces", lines[1])
	}
	for _, want := range []string{"10.0.0.0/8", "192.168.1.0/24"} {
		if !strings.Contains(lines[1], want) {
			t.Errorf("second line missing %q: %q", want, lines[1])
		}
	}

	// 验证文件写入
	if got := len(*fileCalls); got != 2 {
		t.Fatalf("dockerWriteFileHook should be called twice (cidrs + domains), got %d", got)
	}
	if (*fileCalls)[0].Filename != "whitelist-cidrs.json" {
		t.Errorf("first write filename = %q, want whitelist-cidrs.json", (*fileCalls)[0].Filename)
	}
	if (*fileCalls)[1].Filename != "whitelist-domains.json" {
		t.Errorf("second write filename = %q, want whitelist-domains.json", (*fileCalls)[1].Filename)
	}

	// CIDR 解析失败路径
	*nftCalls = (*nftCalls)[:0]
	*fileCalls = (*fileCalls)[:0]

	badCIDRs := json.RawMessage(`{"version":99,"rules":[]}`)
	if err := ApplyBypassRuleSet(context.Background(), hostID, badCIDRs, domainsJSON); err == nil {
		t.Fatal("ApplyBypassRuleSet should fail on bad version, got nil err")
	}
	if got := len(*nftCalls); got != 0 {
		t.Errorf("bad cidrs json must not invoke nft hook, got %d calls", got)
	}
	if got := len(*fileCalls); got != 0 {
		t.Errorf("bad cidrs json must not write files, got %d writes", got)
	}
}

// ===== Test 2: nft failure =====

// TestApplyBypassRuleSet_NftFailure 守护 acceptance Test 4：
//   - dockerExecNftHook 返回错误 → ApplyBypassRuleSet 返回 error
//   - 文件写入不应发生（nft 失败先于写盘）
func TestApplyBypassRuleSet_NftFailure(t *testing.T) {
	hostID := "h-nft-fail"
	withFakeDockerExecNft(t, func(_ context.Context, _ string, _ string) error {
		return errors.New("nft: syntax error simulated")
	})
	fileCalls := withFakeDockerWriteFile(t, nil)

	cidrsJSON := json.RawMessage(`{"version":3,"rules":[{"ip_cidr":["10.0.0.0/8"]}]}`)
	domainsJSON := json.RawMessage(`{"version":3,"rules":[]}`)

	err := ApplyBypassRuleSet(context.Background(), hostID, cidrsJSON, domainsJSON)
	if err == nil {
		t.Fatal("ApplyBypassRuleSet should return error on nft failure, got nil")
	}
	if got := len(*fileCalls); got != 0 {
		t.Errorf("file writes should not happen after nft failure, got %d", got)
	}
	if !strings.Contains(err.Error(), "nft") {
		t.Errorf("err should mention nft: %v", err)
	}
}

// ===== Test 3: file write failure =====

func TestApplyBypassRuleSet_FileWriteFailure(t *testing.T) {
	hostID := "h-file-fail"
	withFakeDockerExecNft(t, func(_ context.Context, _ string, _ string) error { return nil })
	withFakeDockerWriteFile(t, func(_ context.Context, _, filename string, _ []byte) error {
		if filename == "whitelist-domains.json" {
			return errors.New("docker exec failed: permission denied")
		}
		return nil
	})

	cidrsJSON := json.RawMessage(`{"version":3,"rules":[{"ip_cidr":["10.0.0.0/8"]}]}`)
	domainsJSON := json.RawMessage(`{"version":3,"rules":[]}`)

	err := ApplyBypassRuleSet(context.Background(), hostID, cidrsJSON, domainsJSON)
	if err == nil {
		t.Fatal("ApplyBypassRuleSet should return error on file write failure")
	}
	if !strings.Contains(err.Error(), "whitelist-domains.json") {
		t.Errorf("err should mention domains file: %v", err)
	}
}

// ===== Test 4: hash consistency =====

func TestVerifyBypassConsistency_HashMatch(t *testing.T) {
	hostID := "h-verify"

	cidrsJSON := json.RawMessage(`{"version":3,"rules":[{"ip_cidr":["10.0.0.0/8","192.168.1.0/24"]}]}`)

	matchingNftJSON := []byte(`{"nftables":[
      {"metainfo":{}},
      {"set":{"family":"ip","table":"cloudproxy","name":"whitelist_v4","type":"ipv4_addr",
              "elem":[
                {"prefix":{"addr":"10.0.0.0","len":8}},
                {"prefix":{"addr":"192.168.1.0","len":24}}
              ]}}
    ]}`)

	withFakeDockerReadFile(t, func(_ context.Context, hid, filename string) ([]byte, error) {
		if hid != hostID {
			return nil, fmt.Errorf("unexpected hostID: %s", hid)
		}
		if filename != "whitelist-cidrs.json" {
			return nil, fmt.Errorf("unexpected file: %s", filename)
		}
		return cidrsJSON, nil
	})
	withFakeDockerListNftSet(t, func(_ context.Context, _ string) ([]byte, error) {
		return matchingNftJSON, nil
	})

	res, err := VerifyBypassConsistency(context.Background(), hostID)
	if err != nil {
		t.Fatalf("VerifyBypassConsistency: %v", err)
	}
	if !res.OK {
		t.Errorf("expected OK=true, got %+v", res)
	}
	if res.RuleSetSHA256 != res.NftSetSHA256 {
		t.Errorf("matching sets must produce equal hashes: %+v", res)
	}

	// case B: drift
	driftedNftJSON := []byte(`{"nftables":[
      {"metainfo":{}},
      {"set":{"family":"ip","table":"cloudproxy","name":"whitelist_v4","type":"ipv4_addr",
              "elem":[{"prefix":{"addr":"172.16.0.0","len":12}}]}}
    ]}`)
	withFakeDockerListNftSet(t, func(_ context.Context, _ string) ([]byte, error) {
		return driftedNftJSON, nil
	})

	res2, err := VerifyBypassConsistency(context.Background(), hostID)
	if err != nil {
		t.Fatalf("VerifyBypassConsistency drift: %v", err)
	}
	if res2.OK {
		t.Errorf("expected OK=false on drift, got %+v", res2)
	}
	if res2.Detail == "" {
		t.Errorf("drifted result must include Detail")
	}
}

// ===== Test 5: docker exec hook not called on bad input =====

func TestApplyBypassRuleSet_BadInputNoSideEffects(t *testing.T) {
	hostID := "h-bad-input"
	nftCalls := withFakeDockerExecNft(t, func(_ context.Context, _ string, _ string) error {
		t.Fatal("nft hook must not be called on bad input")
		return nil
	})
	fileCalls := withFakeDockerWriteFile(t, nil)

	// empty JSON
	err := ApplyBypassRuleSet(context.Background(), hostID, json.RawMessage(`{}`), json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("should fail on bad cidrs json")
	}
	if len(*nftCalls) != 0 {
		t.Errorf("nft hook called %d times on bad input", len(*nftCalls))
	}
	if len(*fileCalls) != 0 {
		t.Errorf("file hook called %d times on bad input", len(*fileCalls))
	}
}

// 编译期守护：保证 fmt 一直被使用（避免 IDE 误删）。
var _ = fmt.Sprintf("%T", ConsistencyResult{})

// 编译期守护：保证 exec 和 errors import 不被误删。
var _ = (*exec.Cmd)(nil)
var _ = errors.New
