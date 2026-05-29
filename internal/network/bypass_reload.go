package network

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// BypassNftFamily / BypassNftTable / bypassNftSetName 与 worker_firewall_linux.go
// applyWorkerIPv4Rules 中创建的表保持一致：
//
//	conn.AddTable(&nftables.Table{Family: nftables.TableFamilyIPv4, Name: "cloudproxy"})
//
// 对应 nft CLI 字面值是 `ip cloudproxy`（不是 `inet sbfw`）。
// bypassNftSetName 与 types.go::BypassNftSetName 保持同源（`whitelist_v4`）。
//
// 这一组常量是 ApplyBypassRuleSet / VerifyBypassConsistency 拼 nft 命令的唯一事实源，
// 凡涉及 family/table/set 字面值的地方必须经此处。
const (
	bypassNftFamily  = "ip"
	bypassNftTable   = "cloudproxy"
	bypassNftSetName = BypassNftSetName
)

// ConsistencyResult 表示对账结果。
//   - OK=true 表示 ruleset 文件与 nft set 的「CIDR 集合归一化 sha256」一致。
//   - OK=false 时 Detail 给出第一项差异描述（便于运维定位）。
//   - 两个 hash 字段始终都填充，方便上层日志 / API JSON 直接序列化。
type ConsistencyResult struct {
	OK            bool   `json:"ok"`
	RuleSetSHA256 string `json:"ruleset_sha256"`
	NftSetSHA256  string `json:"nft_set_sha256"`
	Detail        string `json:"detail,omitempty"`
}

// atomicWriteFile 把 data 写到 path：先写到同目录下 <name>.tmp.<pid>.<nanos>，
// 然后 os.Rename 到目标。失败先 os.Remove(tmp)，保证不留半成品 / 不污染原文件。
func atomicWriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	tmp, err := os.CreateTemp(dir, base+".tmp.*")
	if err != nil {
		return fmt.Errorf("create tmp: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = os.Remove(tmpName)
	}

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("sync tmp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close tmp: %w", err)
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		cleanup()
		return fmt.Errorf("chmod tmp: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return fmt.Errorf("rename tmp: %w", err)
	}
	return nil
}

// extractCIDRsFromRuleSetJSON 从 sing-box rule-set source v3 文件解析全部 ip_cidr。
// 入参允许的 schema：{"version":3,"rules":[{"ip_cidr":["10.0.0.0/8", ...]}, ...]}。
// 空 rules / 缺失 ip_cidr 都视为合法（返回空切片），保留 v3 schema 的扩展性。
func extractCIDRsFromRuleSetJSON(raw json.RawMessage) ([]string, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, fmt.Errorf("empty rule-set json")
	}
	var env struct {
		Version int                      `json:"version"`
		Rules   []map[string]interface{} `json:"rules"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("unmarshal rule-set envelope: %w", err)
	}
	if env.Version != 3 {
		return nil, fmt.Errorf("unsupported rule-set version %d (want 3)", env.Version)
	}
	var out []string
	for _, rule := range env.Rules {
		raw, ok := rule["ip_cidr"]
		if !ok {
			continue
		}
		arr, ok := raw.([]interface{})
		if !ok {
			return nil, fmt.Errorf("rule.ip_cidr is not array: %T", raw)
		}
		for _, item := range arr {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("rule.ip_cidr element is not string: %T", item)
			}
			out = append(out, s)
		}
	}
	return out, nil
}

// extractCIDRsFromNftJSON 解析 `nft -j list set ip cloudproxy whitelist_v4` 的输出。
// nft 9.x 输出 schema 大致为：
//
//	{"nftables":[
//	  {"metainfo":{...}},
//	  {"set":{"family":"ip","table":"cloudproxy","name":"whitelist_v4",
//	          "type":"ipv4_addr","flags":["interval"],
//	          "elem":[{"prefix":{"addr":"10.0.0.0","len":8}}, "192.168.1.1", ...]}}
//	]}
//
// 元素既可能是 {"prefix":{"addr","len"}}（CIDR）也可能是裸 string（/32 单 IP）。
// 这两种形态都还原为 "addr/len" 或 "addr/32"，与 rule-set 文件中的写法对齐。
func extractCIDRsFromNftJSON(raw []byte) ([]string, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil
	}
	var env struct {
		Nftables []map[string]json.RawMessage `json:"nftables"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("unmarshal nftables envelope: %w", err)
	}
	var out []string
	for _, obj := range env.Nftables {
		setRaw, ok := obj["set"]
		if !ok {
			continue
		}
		var setObj struct {
			Elem []json.RawMessage `json:"elem"`
		}
		if err := json.Unmarshal(setRaw, &setObj); err != nil {
			return nil, fmt.Errorf("unmarshal set: %w", err)
		}
		for _, e := range setObj.Elem {
			cidr, err := decodeNftElement(e)
			if err != nil {
				return nil, err
			}
			if cidr != "" {
				out = append(out, cidr)
			}
		}
	}
	return out, nil
}

func decodeNftElement(raw json.RawMessage) (string, error) {
	trim := bytes.TrimSpace(raw)
	if len(trim) == 0 {
		return "", nil
	}
	// 形态 A：裸字符串 "10.0.0.1"
	if trim[0] == '"' {
		var s string
		if err := json.Unmarshal(trim, &s); err != nil {
			return "", fmt.Errorf("unmarshal nft elem string: %w", err)
		}
		if s == "" {
			return "", nil
		}
		if strings.Contains(s, "/") {
			return s, nil
		}
		return s + "/32", nil
	}
	// 形态 B：{"prefix":{"addr","len"}}
	var withPrefix struct {
		Prefix *struct {
			Addr string `json:"addr"`
			Len  int    `json:"len"`
		} `json:"prefix"`
	}
	if err := json.Unmarshal(raw, &withPrefix); err != nil {
		return "", fmt.Errorf("unmarshal nft elem prefix: %w", err)
	}
	if withPrefix.Prefix != nil {
		return fmt.Sprintf("%s/%d", withPrefix.Prefix.Addr, withPrefix.Prefix.Len), nil
	}
	return "", nil
}

// normalizedSHA256 把 CIDR 列表归一化（去重 + 字典序 sort + 换行 join），
// 再算 sha256 hex。这样语义等价的两个集合不会因为顺序差异 hash 不同。
func normalizedSHA256(cidrs []string) string {
	uniq := make(map[string]struct{}, len(cidrs))
	for _, c := range cidrs {
		uniq[c] = struct{}{}
	}
	keys := make([]string, 0, len(uniq))
	for k := range uniq {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	joined := strings.Join(keys, "\n")
	sum := sha256.Sum256([]byte(joined))
	return hex.EncodeToString(sum[:])
}

// buildNftWhitelistUpdateScript 拼 nft 事务 stdin：
//
//	flush set ip cloudproxy whitelist_v4
//	add element ip cloudproxy whitelist_v4 { 10.0.0.0/8, 192.168.0.0/16 }
//
// 空 cidrs 时只发 flush，把 set 清空（这是合法语义，不是错误）。
// IPv6 留 placeholder 注释，等后续阶段（whitelist_v6 set）落地再加 add 元素。
func buildNftWhitelistUpdateScript(cidrs []string) string {
	uniq := make(map[string]struct{}, len(cidrs))
	keys := make([]string, 0, len(cidrs))
	for _, c := range cidrs {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		// whitelist_v4 只接受 IPv4，IPv6 地址暂跳过（后续 whitelist_v6 set 落地）
		if strings.Contains(c, ":") {
			continue
		}
		if _, dup := uniq[c]; dup {
			continue
		}
		uniq[c] = struct{}{}
		keys = append(keys, c)
	}
	sort.Strings(keys)

	var b strings.Builder
	fmt.Fprintf(&b, "flush set %s %s %s\n", bypassNftFamily, bypassNftTable, bypassNftSetName)
	if len(keys) > 0 {
		fmt.Fprintf(&b, "add element %s %s %s { %s }\n", bypassNftFamily, bypassNftTable, bypassNftSetName, strings.Join(keys, ", "))
	}
	// IPv6 placeholder：当 whitelist_v6 set 落地时，按相同模式在此追加。
	return b.String()
}
