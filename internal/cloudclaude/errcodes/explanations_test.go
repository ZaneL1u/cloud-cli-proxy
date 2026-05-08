package errcodes

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestAllCodesHaveExplanations 遍历 Registry：每条 Code 在 ExtendedExplanations 或 ExplainExempt 中至少出现一次；
// 非豁免码长说明 ≥ 200 中文字符（CONTEXT D-18 / RESEARCH §6.2）。
func TestAllCodesHaveExplanations(t *testing.T) {
	for code := range Registry() {
		if _, isExempt := ExplainExempt[code]; isExempt {
			continue
		}
		exp, ok := ExtendedExplanations[code]
		if !ok {
			t.Errorf("code %s 缺 ExtendedExplanations 且未登记到 ExplainExempt", code)
			continue
		}
		if n := utf8.RuneCountInString(exp); n < 200 {
			t.Errorf("code %s ExtendedExplanations 长度 %d < 200 中文字符", code, n)
		}
	}
}

// TestNoLegacyLowercaseCodes 防御 PITFALLS C8：v2.0 现网 lower-case 字面量禁出现在 Registry（CONTEXT D-22）。
func TestNoLegacyLowercaseCodes(t *testing.T) {
	legacy := []string{
		"auth_failed",
		"auth_expired",
		"entry_token_expired",
		"host_action_failed",
		"entry_config_missing",
	}
	for code := range Registry() {
		for _, bad := range legacy {
			if string(code) == bad {
				t.Errorf("v2.0 lower-case 字面量 %q 不应出现在 Registry（D-22）", bad)
			}
		}
	}
}

// TestAllDomainsClosed 断言 8 域闭合（CONTEXT D-23）：
//
//	DOMAIN ∈ {MOUNT, SESSION, NET, STATE, SYSTEM, SSH, AUTH, DISK}
func TestAllDomainsClosed(t *testing.T) {
	allowed := map[string]bool{
		"MOUNT":   true,
		"SESSION": true,
		"NET":     true,
		"STATE":   true,
		"SYSTEM":  true,
		"SSH":     true,
		"AUTH":    true,
		"DISK":    true,
	}
	for code := range Registry() {
		idx := strings.Index(string(code), "_")
		if idx < 0 {
			t.Errorf("code %s 不含下划线，无法解析 DOMAIN", code)
			continue
		}
		domain := string(code)[:idx]
		if !allowed[domain] {
			t.Errorf("code %s DOMAIN %q 未在 8 域闭合列表", code, domain)
		}
	}
}

// TestExplainExemptOnlyInformational 防御：ExplainExempt 中的 Code 必须在 Registry 有 SeverityInfo（或不存在，兼容未来扩展）；
// 禁止把高严重度 code 误放进豁免集合。
func TestExplainExemptOnlyInformational(t *testing.T) {
	for code := range ExplainExempt {
		entry, ok := Lookup(code)
		if !ok {
			// 未注册 code 放进 ExplainExempt 无害（但应删），仅 warn
			t.Logf("warn: ExplainExempt 中 %s 在 Registry 未找到（可考虑移除）", code)
			continue
		}
		if entry.Severity != SeverityInfo {
			t.Errorf("ExplainExempt 不应包含非 Info Code %s（Severity=%s）", code, entry.Severity)
		}
	}
}

func TestPhase41ExplainCoverage(t *testing.T) {
	// Phase 41 Warn/Error 码必须有 ≥ 200 字符的长说明。
	nonInfoCodes := []Code{
		SSH_VSCODE_PORT_NOT_LISTENING,
		SSH_FORWARDING_BLOCKED,
		DISK_VSCODE_SERVER_WARN,
		DISK_VSCODE_SERVER_BLOAT,
	}
	for _, c := range nonInfoCodes {
		exp, ok := ExtendedExplanations[c]
		if !ok {
			t.Errorf("Phase 41 code %q 缺 ExtendedExplanations", c)
			continue
		}
		if n := utf8.RuneCountInString(exp); n < 200 {
			t.Errorf("Phase 41 code %q ExtendedExplanations 长度 %d < 200", c, n)
		}
	}

	// Phase 41 Info 码必须在 ExplainExempt 中。
	infoCodes := []Code{
		SSH_VSCODE_SERVER_NOT_RUNNING,
		SSH_FORWARDING_SOCKET_MISSING,
	}
	for _, c := range infoCodes {
		if _, ok := ExplainExempt[c]; !ok {
			t.Errorf("Phase 41 Info code %q 应在 ExplainExempt 中", c)
		}
	}
}
