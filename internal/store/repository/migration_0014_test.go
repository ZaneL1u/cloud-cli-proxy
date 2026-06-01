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

// initialMigrationFile 是整合后的初始 migration 文件名。
const initialMigrationFile = "0001_initial.sql"

// TestMigration0014_FileContent 验证 claude_accounts 表的 persistent_volume_name 列语义（D-01/D-02/D-10）。
func TestMigration0014_FileContent(t *testing.T) {
	path := filepath.Join("..", "migrations", initialMigrationFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 migration 失败（%s）: %v", initialMigrationFile, err)
	}
	content := string(raw)

	mustContain := []string{
		"claude_accounts",
		"persistent_volume_name",
	}
	for _, token := range mustContain {
		if !strings.Contains(content, token) {
			t.Errorf("migration 必须包含 %q", token)
		}
	}

	forbidden := []string{
		"persistent_volume_name TEXT NOT NULL DEFAULT ''",
		"persistent_volume_name TEXT NOT NULL DEFAULT \"\"",
	}
	for _, token := range forbidden {
		if strings.Contains(content, token) {
			t.Errorf("migration 不得包含 %q（D-02：NULL 表示未分配，禁止三态）", token)
		}
	}
}

// TestClaudeAccount_PersistentVolumeNameNullable 验证仓储模型通过 *string 支持 NULL 语义。
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

func TestResolveClaudeAccountQueries_MatchD05(t *testing.T) {
	hostQuery := resolveClaudeAccountByHostSQL
	fallbackQuery := resolveClaudeAccountByUserFallbackSQL

	hostMust := []string{"claude_accounts", "host_id = ?", "ORDER BY created_at ASC", "LIMIT 1"}
	for _, token := range hostMust {
		if !strings.Contains(hostQuery, token) {
			t.Errorf("host-bound 查询必须包含 %q（D-05 第一步）；实际:\n%s", token, hostQuery)
		}
	}

	fallbackMust := []string{"claude_accounts", "user_id = ?", "host_id IS NULL", "ORDER BY created_at ASC", "LIMIT 1"}
	for _, token := range fallbackMust {
		if !strings.Contains(fallbackQuery, token) {
			t.Errorf("fallback 查询必须包含 %q（D-05 第二步）；实际:\n%s", token, fallbackQuery)
		}
	}
}

func TestResolveClaudeAccountIDForEntry_Signature(t *testing.T) {
	repoType := reflect.TypeOf((*Repository)(nil))
	method, ok := repoType.MethodByName("ResolveClaudeAccountIDForEntry")
	if !ok {
		t.Fatalf("Repository 必须暴露 ResolveClaudeAccountIDForEntry 方法")
	}

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

func TestWave1_DataLayerBoundary(t *testing.T) {
	migrationPath := filepath.Join("..", "migrations", initialMigrationFile)
	if _, err := os.Stat(migrationPath); err != nil {
		t.Fatalf("Wave 1 必须交付 %s：%v", migrationPath, err)
	}

	sqls := map[string]string{
		"resolveClaudeAccountByHostSQL":         resolveClaudeAccountByHostSQL,
		"resolveClaudeAccountByUserFallbackSQL": resolveClaudeAccountByUserFallbackSQL,
	}
	for name, q := range sqls {
		if strings.Contains(q, "fmt.Sprintf") || strings.Contains(q, "||") {
			t.Errorf("%s 疑似出现字符串拼接；数据层必须全部走参数化：\n%s", name, q)
		}
		if !strings.Contains(q, "?") {
			t.Errorf("%s 必须至少有一个占位符（?）；实际:\n%s", name, q)
		}
	}

	abs, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("resolve cwd: %v", err)
	}
	if !strings.Contains(abs, filepath.Join("internal", "store", "repository")) {
		t.Errorf("本测试必须归属 internal/store/repository（Wave 1 边界）；实际 %s", abs)
	}
}
