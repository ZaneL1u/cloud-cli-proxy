package repository

import (
	"reflect"
	"strings"
	"testing"
)

// expectedBypassMethod 列出 Repository 必须暴露的 Bypass* 方法签名。
// 不写完整 Type（pgx Pool 等 unexported 不便构造），用方法名 + 参数数 + 返回数 做最小契约断言。
type expectedBypassMethod struct {
	name   string
	numIn  int // 含 receiver
	numOut int
}

// TestBypassRepository_Signatures 反射锁定 19 个 Bypass* 方法签名，
// 让 Phase 46 admin handler 可以按签名直接调用，签名漂移立即被发现。
func TestBypassRepository_Signatures(t *testing.T) {
	expected := []expectedBypassMethod{
		{"ListBypassPresets", 2, 2},                // (r, ctx) -> ([]BypassPreset, error)
		{"GetBypassPresetBySlug", 3, 2},            // (r, ctx, slug) -> (BypassPreset, error)
		{"GetBypassPresetByID", 3, 2},              // (r, ctx, id)   -> (BypassPreset, error)
		{"CreateBypassPreset", 3, 2},               // (r, ctx, params) -> (BypassPreset, error)
		{"UpdateBypassPreset", 4, 2},               // (r, ctx, id, params) -> (BypassPreset, error)
		{"DeleteBypassPreset", 3, 1},               // (r, ctx, id) -> error
		{"ListBypassRules", 3, 2},                  // (r, ctx, *string) -> ([]BypassRule, error)
		{"CreateBypassRule", 3, 2},
		{"UpdateBypassRule", 4, 2},
		{"DeleteBypassRule", 3, 1},
		{"ListBypassBindingsByHost", 3, 2},
		{"CreateBypassBinding", 3, 2},
		{"DeleteBypassBinding", 3, 1},
		{"ListBypassSnapshotsByHost", 4, 2},        // (r, ctx, hostID, limit) -> ([]BypassSnapshot, error)
		{"CreateBypassSnapshot", 3, 2},
		{"UpdateBypassSnapshotStatus", 4, 2},       // (r, ctx, id, status) -> (BypassSnapshot, error)
		{"GetLatestAppliedBypassSnapshot", 3, 2},   // (r, ctx, hostID) -> (BypassSnapshot, error)
		{"InsertBypassAuditLog", 3, 2},             // (r, ctx, params) -> (string, error)
		{"ListBypassAuditLogByTarget", 4, 2},       // (r, ctx, kind, id) -> ([]BypassAuditLog, error)
	}
	typ := reflect.TypeOf(&Repository{})
	ctxIface := reflect.TypeOf((*interface{ Deadline() })(nil)) // 占位，下面真正用 String 比对
	_ = ctxIface

	for _, exp := range expected {
		m, ok := typ.MethodByName(exp.name)
		if !ok {
			t.Errorf("Repository 缺少方法 %s", exp.name)
			continue
		}
		if m.Type.NumIn() != exp.numIn {
			t.Errorf("方法 %s: NumIn=%d, want %d", exp.name, m.Type.NumIn(), exp.numIn)
		}
		if m.Type.NumOut() != exp.numOut {
			t.Errorf("方法 %s: NumOut=%d, want %d", exp.name, m.Type.NumOut(), exp.numOut)
		}
		// 第一个 in 是 receiver；第二个必须是 context.Context
		if m.Type.NumIn() >= 2 {
			if m.Type.In(1).String() != "context.Context" {
				t.Errorf("方法 %s 第二参数应为 context.Context, got %s", exp.name, m.Type.In(1).String())
			}
		}
		// 最后一个 out 必须是 error
		lastOut := m.Type.Out(m.Type.NumOut() - 1)
		if lastOut.String() != "error" {
			t.Errorf("方法 %s 最后返回应为 error, got %s", exp.name, lastOut.String())
		}
	}
}

// TestBypassRepository_SQLConstants 逐一断言每个 SQL 常量包含必要的关键字。
// 这是 Phase 46 handler 与数据层之间的「文本契约」：Task 2b 替换方法体时
// 也不允许修改这些常量；如有变更必须先改本测试，迫使 reviewer 注意。
func TestBypassRepository_SQLConstants(t *testing.T) {
	cases := map[string]struct {
		sql      string
		mustHave []string
	}{
		"listBypassPresetsSQL":              {listBypassPresetsSQL, []string{"host_bypass_presets", "SELECT"}},
		"getBypassPresetBySlugSQL":          {getBypassPresetBySlugSQL, []string{"host_bypass_presets", "WHERE", "slug"}},
		"getBypassPresetByIDSQL":            {getBypassPresetByIDSQL, []string{"host_bypass_presets", "WHERE", "id"}},
		"createBypassPresetSQL":             {createBypassPresetSQL, []string{"INSERT INTO host_bypass_presets", "RETURNING"}},
		"updateBypassPresetSQL":             {updateBypassPresetSQL, []string{"UPDATE host_bypass_presets", "is_system = FALSE"}},
		"deleteBypassPresetSQL":             {deleteBypassPresetSQL, []string{"DELETE FROM host_bypass_presets", "is_system = FALSE"}},
		"checkBypassPresetIsSystemSQL":      {checkBypassPresetIsSystemSQL, []string{"SELECT is_system", "host_bypass_presets"}},
		"listBypassRulesGlobalOnlySQL":      {listBypassRulesGlobalOnlySQL, []string{"host_bypass_rules", "scope = 'global'"}},
		"listBypassRulesGlobalOrHostSQL":    {listBypassRulesGlobalOrHostSQL, []string{"host_bypass_rules", "host_id = $1"}},
		"createBypassRuleSQL":               {createBypassRuleSQL, []string{"INSERT INTO host_bypass_rules", "RETURNING"}},
		"updateBypassRuleSQL":               {updateBypassRuleSQL, []string{"UPDATE host_bypass_rules"}},
		"deleteBypassRuleSQL":               {deleteBypassRuleSQL, []string{"DELETE FROM host_bypass_rules"}},
		"listBypassBindingsByHostSQL":       {listBypassBindingsByHostSQL, []string{"host_bypass_bindings", "host_id"}},
		"createBypassBindingSQL":            {createBypassBindingSQL, []string{"INSERT INTO host_bypass_bindings"}},
		"deleteBypassBindingSQL":            {deleteBypassBindingSQL, []string{"DELETE FROM host_bypass_bindings"}},
		"listBypassSnapshotsByHostSQL":      {listBypassSnapshotsByHostSQL, []string{"host_bypass_snapshots", "host_id", "ORDER BY version DESC"}},
		"createBypassSnapshotSQL":           {createBypassSnapshotSQL, []string{"INSERT INTO host_bypass_snapshots"}},
		"updateBypassSnapshotStatusSQL":     {updateBypassSnapshotStatusSQL, []string{"UPDATE host_bypass_snapshots", "applied_status"}},
		"getLatestAppliedBypassSnapshotSQL": {getLatestAppliedBypassSnapshotSQL, []string{"host_bypass_snapshots", "applied_status = 'applied'", "ORDER BY version DESC", "LIMIT 1"}},
		"insertBypassAuditLogSQL":           {insertBypassAuditLogSQL, []string{"INSERT INTO host_bypass_audit_log", "RETURNING"}},
		"listBypassAuditLogByTargetSQL":     {listBypassAuditLogByTargetSQL, []string{"host_bypass_audit_log", "target_kind", "target_id"}},
	}
	for name, c := range cases {
		for _, tok := range c.mustHave {
			if !strings.Contains(c.sql, tok) {
				t.Errorf("%s 缺少 %q\nSQL:\n%s", name, tok, c.sql)
			}
		}
	}
}

// TestBypassRepository_ErrSystemPresetImmutable 锁定 sentinel error 文案的关键词，
// Phase 46 handler 会用 errors.Is 比对它来决定是否返回 HTTP 403。
func TestBypassRepository_ErrSystemPresetImmutable(t *testing.T) {
	if ErrSystemBypassPresetImmutable == nil {
		t.Fatal("ErrSystemBypassPresetImmutable 必须为非空 error")
	}
	msg := ErrSystemBypassPresetImmutable.Error()
	for _, want := range []string{"system", "cannot"} {
		if !strings.Contains(msg, want) {
			t.Errorf("ErrSystemBypassPresetImmutable.Error() 应包含 %q, got: %s", want, msg)
		}
	}
}
