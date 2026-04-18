package repository

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// migration0014Filename 是 Phase 30 Plan 01 落地的 migration 文件名。
// 对齐 30-CONTEXT D-10（紧随现存 0013 之后，无跳号）。
const migration0014Filename = "0014_claude_account_persistent_volume.sql"

// TestMigration0014_FileContent 验证 migration 0014 的内容语义（D-01/D-02/D-10）。
//   - 使用 ADD COLUMN IF NOT EXISTS 保持幂等，兼容空库与 v2.0 升级库。
//   - 列类型 TEXT，且不能使用空字符串默认值（避免三态，NULL = 未分配）。
//   - 文件中必须包含 down 路径注释（DROP COLUMN IF EXISTS）以便后续回滚指令清晰。
func TestMigration0014_FileContent(t *testing.T) {
	path := filepath.Join("..", "migrations", migration0014Filename)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 migration 失败（%s）: %v", migration0014Filename, err)
	}
	content := string(raw)

	mustContain := []string{
		"claude_accounts",
		"persistent_volume_name",
		"ADD COLUMN IF NOT EXISTS persistent_volume_name",
		"TEXT",
		"DROP COLUMN IF EXISTS persistent_volume_name",
	}
	for _, token := range mustContain {
		if !strings.Contains(content, token) {
			t.Errorf("migration 0014 必须包含 %q，当前内容:\n%s", token, content)
		}
	}

	forbidden := []string{
		"DEFAULT ''",
		"DEFAULT \"\"",
		"NOT NULL",
	}
	for _, token := range forbidden {
		if strings.Contains(content, token) {
			t.Errorf("migration 0014 不得包含 %q（D-02：NULL 表示未分配，禁止三态）", token)
		}
	}
}

// TestClaudeAccount_PersistentVolumeNameNullable 验证仓储模型通过 *string 支持 NULL 语义。
// D-02：NULL = 未分配；非空字符串 = 已分配的 Docker named volume 名称。
func TestClaudeAccount_PersistentVolumeNameNullable(t *testing.T) {
	typ := reflect.TypeOf(ClaudeAccount{})
	field, ok := typ.FieldByName("PersistentVolumeName")
	if !ok {
		t.Fatalf("ClaudeAccount 必须新增 PersistentVolumeName 字段（D-01/D-02）")
	}
	if field.Type.Kind() != reflect.Ptr || field.Type.Elem().Kind() != reflect.String {
		t.Fatalf("PersistentVolumeName 必须是 *string 以承载 NULL 语义；实际为 %s", field.Type.String())
	}
	jsonTag := field.Tag.Get("json")
	if !strings.Contains(jsonTag, "persistent_volume_name") {
		t.Errorf("json tag 必须为 persistent_volume_name；实际为 %q", jsonTag)
	}
	if !strings.Contains(jsonTag, "omitempty") {
		t.Errorf("json tag 必须包含 omitempty，未分配时省略字段；实际为 %q", jsonTag)
	}

	// JSON 回路：未分配序列化应省略字段；已分配应序列化为字符串。
	unassigned := ClaudeAccount{ID: "a"}
	b, err := json.Marshal(unassigned)
	if err != nil {
		t.Fatalf("marshal unassigned: %v", err)
	}
	if strings.Contains(string(b), "persistent_volume_name") {
		t.Errorf("未分配（nil）时 JSON 不得出现 persistent_volume_name；实际：%s", string(b))
	}

	name := "claude-state-abc"
	assigned := ClaudeAccount{ID: "b", PersistentVolumeName: &name}
	b2, err := json.Marshal(assigned)
	if err != nil {
		t.Fatalf("marshal assigned: %v", err)
	}
	if !strings.Contains(string(b2), "\"persistent_volume_name\":\"claude-state-abc\"") {
		t.Errorf("已分配时 JSON 必须携带 persistent_volume_name；实际：%s", string(b2))
	}
}

// TestHostSSHAuth_HasTemplateImageRef 验证 HostSSHAuth 暴露 TemplateImageRef，
// 供 Wave 2 的 Entry API 推导 image_version / supports_* 能力字段（D-05/D-06/D-07）。
func TestHostSSHAuth_HasTemplateImageRef(t *testing.T) {
	typ := reflect.TypeOf(HostSSHAuth{})
	field, ok := typ.FieldByName("TemplateImageRef")
	if !ok {
		t.Fatalf("HostSSHAuth 必须新增 TemplateImageRef（Wave 2 能力字段推导入口）")
	}
	if field.Type.Kind() != reflect.String {
		t.Fatalf("TemplateImageRef 必须为 string；实际为 %s", field.Type.String())
	}
}

// TestResolveClaudeAccountQueries_MatchD05 锁定 D-05 的 SQL 解析顺序，
// 避免后续改动悄悄破坏「host 绑定优先、user fallback」的确定性。
func TestResolveClaudeAccountQueries_MatchD05(t *testing.T) {
	hostQuery := resolveClaudeAccountByHostSQL
	fallbackQuery := resolveClaudeAccountByUserFallbackSQL

	hostMust := []string{"claude_accounts", "host_id = $1", "ORDER BY created_at ASC", "LIMIT 1"}
	for _, token := range hostMust {
		if !strings.Contains(hostQuery, token) {
			t.Errorf("host-bound 查询必须包含 %q（D-05 第一步）；实际:\n%s", token, hostQuery)
		}
	}

	fallbackMust := []string{"claude_accounts", "user_id = $1", "host_id IS NULL", "ORDER BY created_at ASC", "LIMIT 1"}
	for _, token := range fallbackMust {
		if !strings.Contains(fallbackQuery, token) {
			t.Errorf("fallback 查询必须包含 %q（D-05 第二步）；实际:\n%s", token, fallbackQuery)
		}
	}
}

// TestGetHostByShortID_QueryIncludesTemplateImageRef 保证 GetHostByShortID
// 已经把 template_image_ref 纳入 SELECT 列，供 Wave 2 Entry API 消费。
func TestGetHostByShortID_QueryIncludesTemplateImageRef(t *testing.T) {
	if !strings.Contains(getHostByShortIDSQL, "template_image_ref") {
		t.Errorf("GetHostByShortID 查询必须选择 template_image_ref；实际:\n%s", getHostByShortIDSQL)
	}
}

// TestResolveClaudeAccountIDForEntry_Signature 使用反射确认 Repository 暴露
// ResolveClaudeAccountIDForEntry(ctx, userID, hostID) (string, bool, error) 签名，
// 以契约形式让 Wave 2 的 Entry API 可直接消费。
func TestResolveClaudeAccountIDForEntry_Signature(t *testing.T) {
	repoType := reflect.TypeOf((*Repository)(nil))
	method, ok := repoType.MethodByName("ResolveClaudeAccountIDForEntry")
	if !ok {
		t.Fatalf("Repository 必须暴露 ResolveClaudeAccountIDForEntry 方法")
	}

	// 方法集包含 receiver；预期签名为 (*Repository, context.Context, string, string) (string, bool, error)。
	mt := method.Type
	if mt.NumIn() != 4 {
		t.Fatalf("ResolveClaudeAccountIDForEntry 参数数量错误：want 4 (含 receiver)，got %d", mt.NumIn())
	}
	ctxIface := reflect.TypeOf((*context.Context)(nil)).Elem()
	if !mt.In(1).Implements(ctxIface) {
		t.Errorf("第一个参数必须是 context.Context；实际 %s", mt.In(1))
	}
	if mt.In(2).Kind() != reflect.String || mt.In(3).Kind() != reflect.String {
		t.Errorf("userID/hostID 参数必须是 string；实际 %s / %s", mt.In(2), mt.In(3))
	}

	if mt.NumOut() != 3 {
		t.Fatalf("返回值数量错误：want 3 (accountID,string; ok,bool; err,error)，got %d", mt.NumOut())
	}
	if mt.Out(0).Kind() != reflect.String {
		t.Errorf("返回值 0 必须是 string；实际 %s", mt.Out(0))
	}
	if mt.Out(1).Kind() != reflect.Bool {
		t.Errorf("返回值 1 必须是 bool（未命中 => false 而非 error）；实际 %s", mt.Out(1))
	}
	errIface := reflect.TypeOf((*error)(nil)).Elem()
	if !mt.Out(2).Implements(errIface) {
		t.Errorf("返回值 2 必须是 error；实际 %s", mt.Out(2))
	}
}

// TestWave1_DataLayerBoundary 保护 Phase 30 Plan 01 只覆盖 migration + repository 变更，
// 避免在数据层提前引入 Wave 2（HTTP / cloudclaude）的职责。
// 任一违约都说明波次边界被破坏（D-11/D-12，ROADMAP Phase 30 Scope）。
func TestWave1_DataLayerBoundary(t *testing.T) {
	// 1) Wave 1 的关键产物必须存在且位于 internal/store/**。
	migrationPath := filepath.Join("..", "migrations", migration0014Filename)
	if _, err := os.Stat(migrationPath); err != nil {
		t.Fatalf("Wave 1 必须交付 %s：%v", migrationPath, err)
	}

	// 2) D-05 两条查询必须保持参数化、非字符串拼接（T-30-01 缓解）。
	sqls := map[string]string{
		"resolveClaudeAccountByHostSQL":         resolveClaudeAccountByHostSQL,
		"resolveClaudeAccountByUserFallbackSQL": resolveClaudeAccountByUserFallbackSQL,
		"getHostByShortIDSQL":                   getHostByShortIDSQL,
	}
	for name, q := range sqls {
		if strings.Contains(q, "fmt.Sprintf") || strings.Contains(q, "||") {
			t.Errorf("%s 疑似出现字符串拼接；数据层必须全部走 pgx 参数化：\n%s", name, q)
		}
		if !strings.Contains(q, "$1") {
			t.Errorf("%s 必须至少有一个占位符（$1）；实际:\n%s", name, q)
		}
	}

	// 3) 数据层只应位于 internal/store/**；断言当前测试文件的归属路径。
	abs, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("resolve cwd: %v", err)
	}
	if !strings.Contains(abs, filepath.Join("internal", "store", "repository")) {
		t.Errorf("本测试必须归属 internal/store/repository（Wave 1 边界）；实际 %s", abs)
	}
}
