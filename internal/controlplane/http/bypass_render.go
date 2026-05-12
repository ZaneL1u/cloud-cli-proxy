package http

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strings"

	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

// bypassRuleSetVersion 与 Phase 45 sing-box rule-set placeholder 锁定的 schema 版本一致。
// 见 internal/network/gateway_singbox_config.go::ruleSetPlaceholder。
const bypassRuleSetVersion = 3

// BypassRenderInput 聚合一次渲染所需的全部规则集。
// 调用方（admin_bypass_snapshots handler）通过 collectRenderInput 从 binding/preset/rule
// 三张表组装出该结构再交给 RenderBypassConfig。
type BypassRenderInput struct {
	HostID  string
	Presets []repository.BypassPreset
	// Rules 既包含 host scope 规则（直接关联 hostID），也包含 binding 引用的 global rule。
	Rules []repository.BypassRule
}

// BypassRenderOutput 是渲染层的纯数据返回，handler 直接序列化到 HTTP body。
//
//   - CIDRsJSON / DomainsJSON 与 Phase 45 sing-box rule-set 文件格式 100% 兼容
//     （{"version":3,"rules":[...]}）。同一规则集合在不同输入顺序下产出同一字节序列，
//     从而保证 ConfigHash 稳定。
//   - NftDiff 是相对 prevSnapshot 的简单 set diff：+ 新增 / - 删除 / unchanged 不输出。
//   - ConfigHash = sha256(CIDRsJSON + "\n" + DomainsJSON) 取 hex；apply 接口用它做幂等键。
//   - RiskyCount = 输入 Rules 里 IsRisky=true 的条数（前端用以触发二次确认）。
//   - Summary 是中文摘要，前端预览 Sheet 顶部展示。
type BypassRenderOutput struct {
	CIDRsJSON   json.RawMessage `json:"whitelist_cidrs_rendered"`
	DomainsJSON json.RawMessage `json:"whitelist_domains_rendered"`
	NftDiff     string          `json:"nft_diff"`
	ConfigHash  string          `json:"config_hash"`
	RiskyCount  int             `json:"risky_count"`
	Summary     string          `json:"summary"`
}

// ruleSetCIDRBucket 对应 cidrs JSON 的 rules[0]: {"ip_cidr":["10.0.0.0/8", ...]}。
type ruleSetCIDRBucket struct {
	IPCIDR []string `json:"ip_cidr"`
}

// ruleSetDomainBucket 对应 domains JSON 的 rules 中三类 key：
//   - {"domain":[...]}
//   - {"domain_suffix":[...]}
//   - {"domain_keyword":[...]}
//
// 三类各占一个对象元素，按 domain / domain_suffix / domain_keyword 顺序输出。
type ruleSetDomainBucket struct {
	Domain        []string `json:"domain,omitempty"`
	DomainSuffix  []string `json:"domain_suffix,omitempty"`
	DomainKeyword []string `json:"domain_keyword,omitempty"`
}

// ruleSetEnvelope 是 sing-box source format v3 的最外层包装。
type ruleSetEnvelope struct {
	Version int           `json:"version"`
	Rules   []interface{} `json:"rules"`
}

// RenderBypassConfig 把 binding + preset + rule 集合渲染为可直接写盘的 sing-box rule-set
// 文件。它是纯函数：相同输入产生 byte-identical 输出。
//
// prevSnapshot != nil 时按 cidrs 桶生成简易 set diff 给前端展示；prevSnapshot == nil
// 表示这是 host 首次配置，diff 视作全量新增。
func RenderBypassConfig(input BypassRenderInput, prevSnapshot *repository.BypassSnapshot) (BypassRenderOutput, error) {
	// 把所有 preset.rules + host rules 按 rule_type 分桶聚合。
	cidrSet := map[string]struct{}{}
	domainSet := map[string]struct{}{}
	suffixSet := map[string]struct{}{}
	keywordSet := map[string]struct{}{}
	totalRules := 0
	riskyCount := 0

	addRule := func(ruleType, value string, isRisky bool) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		switch ruleType {
		case "ip":
			// 单 IP 转 /32 形式让 sing-box rule-set 统一作为 ip_cidr 处理。
			if ip := net.ParseIP(value); ip != nil {
				if ip.To4() != nil {
					cidrSet[value+"/32"] = struct{}{}
				} else {
					cidrSet[value+"/128"] = struct{}{}
				}
				totalRules++
				if isRisky {
					riskyCount++
				}
			}
		case "cidr":
			cidrSet[value] = struct{}{}
			totalRules++
			if isRisky {
				riskyCount++
			}
		case "domain":
			domainSet[value] = struct{}{}
			totalRules++
			if isRisky {
				riskyCount++
			}
		case "domain_suffix":
			// sing-box 接受带前导点或不带，统一存储为不带前导点形式。
			suffixSet[strings.TrimPrefix(value, ".")] = struct{}{}
			totalRules++
			if isRisky {
				riskyCount++
			}
		case "domain_keyword":
			keywordSet[value] = struct{}{}
			totalRules++
			if isRisky {
				riskyCount++
			}
		}
	}

	for _, p := range input.Presets {
		for _, pr := range p.Rules {
			addRule(pr.RuleType, pr.Value, false)
		}
	}
	for _, r := range input.Rules {
		addRule(r.RuleType, r.Value, r.IsRisky)
	}

	// 排序：固定字典序保证 deterministic byte output → ConfigHash 稳定。
	cidrs := sortedKeys(cidrSet)
	domains := sortedKeys(domainSet)
	suffixes := sortedKeys(suffixSet)
	keywords := sortedKeys(keywordSet)

	// 渲染 cidrs JSON：当 cidrs 列表为空时 rules 字段保持空数组，与 Phase 45 ruleSetPlaceholder 一致。
	cidrsEnvelope := ruleSetEnvelope{Version: bypassRuleSetVersion, Rules: []interface{}{}}
	if len(cidrs) > 0 {
		cidrsEnvelope.Rules = append(cidrsEnvelope.Rules, ruleSetCIDRBucket{IPCIDR: cidrs})
	}

	// 渲染 domains JSON：domain / domain_suffix / domain_keyword 三种 key 各占一个 rule 块。
	domainsEnvelope := ruleSetEnvelope{Version: bypassRuleSetVersion, Rules: []interface{}{}}
	if len(domains) > 0 {
		domainsEnvelope.Rules = append(domainsEnvelope.Rules, ruleSetDomainBucket{Domain: domains})
	}
	if len(suffixes) > 0 {
		domainsEnvelope.Rules = append(domainsEnvelope.Rules, ruleSetDomainBucket{DomainSuffix: suffixes})
	}
	if len(keywords) > 0 {
		domainsEnvelope.Rules = append(domainsEnvelope.Rules, ruleSetDomainBucket{DomainKeyword: keywords})
	}

	cidrsJSON, err := json.MarshalIndent(cidrsEnvelope, "", "  ")
	if err != nil {
		return BypassRenderOutput{}, fmt.Errorf("marshal cidrs rule-set: %w", err)
	}
	domainsJSON, err := json.MarshalIndent(domainsEnvelope, "", "  ")
	if err != nil {
		return BypassRenderOutput{}, fmt.Errorf("marshal domains rule-set: %w", err)
	}

	// ConfigHash = sha256(cidrsJSON + "\n" + domainsJSON) hex.
	hasher := sha256.New()
	hasher.Write(cidrsJSON)
	hasher.Write([]byte("\n"))
	hasher.Write(domainsJSON)
	configHash := hex.EncodeToString(hasher.Sum(nil))

	// nft diff（仅基于 cidrs 桶，domains 由 sing-box rule-set 直接处理不进 nft set）。
	diff := renderNftDiff(input.HostID, cidrs, prevSnapshot)

	return BypassRenderOutput{
		CIDRsJSON:   json.RawMessage(cidrsJSON),
		DomainsJSON: json.RawMessage(domainsJSON),
		NftDiff:     diff,
		ConfigHash:  configHash,
		RiskyCount:  riskyCount,
		Summary:     fmt.Sprintf("覆盖 %d 条规则，其中 %d 条高风险", totalRules, riskyCount),
	}, nil
}

// sortedKeys 把 set 的 key 按字典序返回，保证 marshal 输出稳定。
func sortedKeys(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// renderNftDiff 生成简单的 nft set diff 文本：
//
//   - 行格式 `+ <cidr>` / `- <cidr>`；
//   - 与 prevSnapshot.WhitelistCIDRsJSON 的 cidrs 列表做集合差；
//   - prevSnapshot == nil 视作首次配置，所有 cidrs 都是 `+` 新增。
//
// 该 diff 仅用于前端预览，不影响真实落盘；Phase 47 host-agent 会用同样的输入再独立计算。
func renderNftDiff(hostID string, currentCIDRs []string, prev *repository.BypassSnapshot) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# @whitelist_v4 set diff (host=%s)\n", hostID)

	prevCIDRs := extractPrevCIDRs(prev)
	prevSet := map[string]struct{}{}
	for _, c := range prevCIDRs {
		prevSet[c] = struct{}{}
	}
	currSet := map[string]struct{}{}
	for _, c := range currentCIDRs {
		currSet[c] = struct{}{}
	}

	// 删除项：prev 中有但 curr 中没。
	deletions := make([]string, 0)
	for c := range prevSet {
		if _, ok := currSet[c]; !ok {
			deletions = append(deletions, c)
		}
	}
	sort.Strings(deletions)
	for _, c := range deletions {
		fmt.Fprintf(&b, "- %s\n", c)
	}

	// 新增项：curr 中有但 prev 中没。
	additions := make([]string, 0)
	for c := range currSet {
		if _, ok := prevSet[c]; !ok {
			additions = append(additions, c)
		}
	}
	sort.Strings(additions)
	for _, c := range additions {
		fmt.Fprintf(&b, "+ %s\n", c)
	}

	return b.String()
}

// extractPrevCIDRs 从 prevSnapshot.WhitelistCIDRsJSON 抽出 cidrs 桶里的 ip_cidr 数组。
// nil snapshot / 空 JSON / 非 envelope 形态都返回空切片。
func extractPrevCIDRs(prev *repository.BypassSnapshot) []string {
	if prev == nil || len(prev.WhitelistCIDRsJSON) == 0 {
		return nil
	}
	var env struct {
		Version int                      `json:"version"`
		Rules   []map[string]interface{} `json:"rules"`
	}
	if err := json.Unmarshal(prev.WhitelistCIDRsJSON, &env); err != nil {
		return nil
	}
	out := make([]string, 0)
	for _, rule := range env.Rules {
		if raw, ok := rule["ip_cidr"].([]interface{}); ok {
			for _, v := range raw {
				if s, ok := v.(string); ok {
					out = append(out, s)
				}
			}
		}
	}
	return out
}
