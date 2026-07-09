package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// ErrSystemBypassPresetImmutable 表示尝试删除或修改 is_system=true 的预设。
var ErrSystemBypassPresetImmutable = errors.New("bypass preset is system preset and cannot be deleted or modified")

// scanner is a minimal interface satisfied by both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

// nullableUUIDArg 把 *string 转换为 SQL 友好的 any：
//   - nil 指针        → nil（SQL NULL）
//   - 指向空串的指针   → nil（SQL NULL）
func nullableUUIDArg(ptr *string) any {
	if ptr == nil || *ptr == "" {
		return nil
	}
	return *ptr
}

// ---------------------------------------------------------------------------
// SQL 常量（包级 const，供 queries_bypass_test.go 文本断言锁定）
// 所有占位符使用 ?，保持参数化查询。
// ---------------------------------------------------------------------------

const listBypassPresetsSQL = `
	SELECT id, slug, name, COALESCE(description, ''),
	       is_system, is_force_on, is_active, rules, created_at, updated_at
	FROM host_bypass_presets
	ORDER BY is_system DESC, slug ASC
`

const getBypassPresetBySlugSQL = `
	SELECT id, slug, name, COALESCE(description, ''),
	       is_system, is_force_on, is_active, rules, created_at, updated_at
	FROM host_bypass_presets WHERE slug = ?
`

const getBypassPresetByIDSQL = `
	SELECT id, slug, name, COALESCE(description, ''),
	       is_system, is_force_on, is_active, rules, created_at, updated_at
	FROM host_bypass_presets WHERE id = ?
`

const createBypassPresetSQL = `
	INSERT INTO host_bypass_presets (id, slug, name, description, is_force_on, is_active, rules)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	RETURNING id, slug, name, COALESCE(description, ''),
	          is_system, is_force_on, is_active, rules, created_at, updated_at
`

// updateBypassPresetSQL 用 COALESCE(?, col) 实现部分字段更新。
const updateBypassPresetSQL = `
	UPDATE host_bypass_presets SET
		name        = COALESCE(?, name),
		description = COALESCE(?, description),
		is_force_on = COALESCE(?, is_force_on),
		is_active   = COALESCE(?, is_active),
		rules       = COALESCE(?, rules),
		updated_at  = CURRENT_TIMESTAMP
	WHERE id = ? AND is_system = 0
	RETURNING id, slug, name, COALESCE(description, ''),
	          is_system, is_force_on, is_active, rules, created_at, updated_at
`

const deleteBypassPresetSQL = `DELETE FROM host_bypass_presets WHERE id = ? AND is_system = 0`

const checkBypassPresetIsSystemSQL = `SELECT is_system FROM host_bypass_presets WHERE id = ?`

const listBypassRulesGlobalOnlySQL = `
	SELECT id, scope, host_id, rule_type, value, COALESCE(note, ''),
	       is_risky, created_at, updated_at
	FROM host_bypass_rules
	WHERE scope = 'global'
	ORDER BY created_at ASC
`

const listBypassRulesGlobalOrHostSQL = `
	SELECT id, scope, host_id, rule_type, value, COALESCE(note, ''),
	       is_risky, created_at, updated_at
	FROM host_bypass_rules
	WHERE scope = 'global' OR (scope = 'host' AND host_id = ?)
	ORDER BY scope DESC, created_at ASC
`

const createBypassRuleSQL = `
	INSERT INTO host_bypass_rules (id, scope, host_id, rule_type, value, note, is_risky)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	RETURNING id, scope, host_id, rule_type, value, COALESCE(note, ''),
	          is_risky, created_at, updated_at
`

const updateBypassRuleSQL = `
	UPDATE host_bypass_rules SET
		value      = COALESCE(?, value),
		note       = COALESCE(?, note),
		is_risky   = COALESCE(?, is_risky),
		updated_at = CURRENT_TIMESTAMP
	WHERE id = ?
	RETURNING id, scope, host_id, rule_type, value, COALESCE(note, ''),
	          is_risky, created_at, updated_at
`

const deleteBypassRuleSQL = `DELETE FROM host_bypass_rules WHERE id = ?`

const getBypassRuleByIDSQL = `
	SELECT id, scope, host_id, rule_type, value, COALESCE(note, ''),
	       is_risky, created_at, updated_at
	FROM host_bypass_rules WHERE id = ?
`

const listBypassBindingsByHostSQL = `
	SELECT id, host_id, preset_id, rule_id,
	       enabled, source, created_at
	FROM host_bypass_bindings
	WHERE host_id = ?
	ORDER BY created_at ASC
`

const createBypassBindingSQL = `
	INSERT INTO host_bypass_bindings (id, host_id, preset_id, rule_id, enabled, source)
	VALUES (?, ?, ?, ?, ?, ?)
	RETURNING id, host_id, preset_id, rule_id,
	          enabled, source, created_at
`

const deleteBypassBindingSQL = `DELETE FROM host_bypass_bindings WHERE id = ?`

const listBypassSnapshotsByHostSQL = `
	SELECT id, host_id, version, config_hash,
	       whitelist_cidrs_json, whitelist_domains_json,
	       applied_status, source, created_by, created_at
	FROM host_bypass_snapshots
	WHERE host_id = ?
	ORDER BY version DESC
	LIMIT ?
`

const createBypassSnapshotSQL = `
	INSERT INTO host_bypass_snapshots
		(id, host_id, version, config_hash, whitelist_cidrs_json, whitelist_domains_json, source, created_by)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	RETURNING id, host_id, version, config_hash,
	          whitelist_cidrs_json, whitelist_domains_json,
	          applied_status, source, created_by, created_at
`

const updateBypassSnapshotStatusSQL = `
	UPDATE host_bypass_snapshots SET applied_status = ? WHERE id = ?
	RETURNING id, host_id, version, config_hash,
	          whitelist_cidrs_json, whitelist_domains_json,
	          applied_status, source, created_by, created_at
`

const getLatestAppliedBypassSnapshotSQL = `
	SELECT id, host_id, version, config_hash,
	       whitelist_cidrs_json, whitelist_domains_json,
	       applied_status, source, created_by, created_at
	FROM host_bypass_snapshots
	WHERE host_id = ? AND applied_status = 'applied'
	ORDER BY version DESC
	LIMIT 1
`

const getBypassSnapshotByIDSQL = `
	SELECT id, host_id, version, config_hash,
	       whitelist_cidrs_json, whitelist_domains_json,
	       applied_status, source, created_by, created_at
	FROM host_bypass_snapshots
	WHERE id = ?
`

const insertBypassAuditLogSQL = `
	INSERT INTO host_bypass_audit_log
		(id, actor_id, actor_ip, action, target_kind, target_id, before, after, note)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	RETURNING id, created_at
`

const listBypassAuditLogByTargetSQL = `
	SELECT id, actor_id, COALESCE(actor_ip, ''), action, target_kind,
	       target_id, before, after, COALESCE(note, ''), created_at
	FROM host_bypass_audit_log
	WHERE target_kind = ? AND target_id = ?
	ORDER BY created_at DESC
`

// ---------------------------------------------------------------------------
// scan helpers — work with both *sql.Row (QueryRow) and *sql.Rows (rows.Scan)
// ---------------------------------------------------------------------------

func scanBypassPreset(row scanner, out *BypassPreset) error {
	var rawRules jsonText
	if err := row.Scan(
		&out.ID, &out.Slug, &out.Name, &out.Description,
		&out.IsSystem, &out.IsForceOn, &out.IsActive, &rawRules,
		&out.CreatedAt, &out.UpdatedAt,
	); err != nil {
		return err
	}
	if len(rawRules) > 0 {
		_ = json.Unmarshal([]byte(rawRules), &out.Rules)
	}
	return nil
}

func scanBypassRule(row scanner, out *BypassRule) error {
	return row.Scan(
		&out.ID, &out.Scope, &out.HostID, &out.RuleType, &out.Value, &out.Note,
		&out.IsRisky, &out.CreatedAt, &out.UpdatedAt,
	)
}

func scanBypassBinding(row scanner, out *BypassBinding) error {
	return row.Scan(
		&out.ID, &out.HostID, &out.PresetID, &out.RuleID,
		&out.Enabled, &out.Source, &out.CreatedAt,
	)
}

func scanBypassSnapshot(row scanner, out *BypassSnapshot) error {
	var cidrs, domains jsonText
	if err := row.Scan(
		&out.ID, &out.HostID, &out.Version, &out.ConfigHash,
		&cidrs, &domains,
		&out.AppliedStatus, &out.Source, &out.CreatedBy, &out.CreatedAt,
	); err != nil {
		return err
	}
	out.WhitelistCIDRsJSON = cidrs.RawMessage()
	out.WhitelistDomainsJSON = domains.RawMessage()
	return nil
}

// ---------------------------------------------------------------------------
// 19 个 Repository 方法 — database/sql 实现
// ---------------------------------------------------------------------------

func (r *Repository) ListBypassPresets(ctx context.Context) ([]BypassPreset, error) {
	rows, err := r.db.QueryContext(ctx, listBypassPresetsSQL)
	if err != nil {
		return nil, fmt.Errorf("query bypass presets: %w", err)
	}
	defer rows.Close()

	out := make([]BypassPreset, 0)
	for rows.Next() {
		var it BypassPreset
		if err := scanBypassPreset(rows, &it); err != nil {
			return nil, fmt.Errorf("scan bypass preset: %w", err)
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate bypass presets: %w", err)
	}
	return out, nil
}

func (r *Repository) GetBypassPresetBySlug(ctx context.Context, slug string) (BypassPreset, error) {
	var it BypassPreset
	if err := scanBypassPreset(r.db.QueryRowContext(ctx, getBypassPresetBySlugSQL, slug), &it); err != nil {
		return BypassPreset{}, fmt.Errorf("get bypass preset by slug: %w", err)
	}
	return it, nil
}

func (r *Repository) GetBypassPresetByID(ctx context.Context, id string) (BypassPreset, error) {
	var it BypassPreset
	if err := scanBypassPreset(r.db.QueryRowContext(ctx, getBypassPresetByIDSQL, id), &it); err != nil {
		return BypassPreset{}, fmt.Errorf("get bypass preset by id: %w", err)
	}
	return it, nil
}

func (r *Repository) CreateBypassPreset(ctx context.Context, params CreateBypassPresetParams) (BypassPreset, error) {
	rulesJSON, err := json.Marshal(params.Rules)
	if err != nil {
		return BypassPreset{}, fmt.Errorf("marshal bypass preset rules: %w", err)
	}
	if len(params.Rules) == 0 {
		rulesJSON = []byte("[]")
	}
	var it BypassPreset
	row := r.db.QueryRowContext(ctx, createBypassPresetSQL,
		uuid.NewString(),
		params.Slug, params.Name, nullIfEmpty(params.Description),
		params.IsForceOn, params.IsActive, rulesJSON,
	)
	if err := scanBypassPreset(row, &it); err != nil {
		return BypassPreset{}, fmt.Errorf("create bypass preset: %w", err)
	}
	return it, nil
}

func (r *Repository) UpdateBypassPreset(ctx context.Context, id string, params UpdateBypassPresetParams) (BypassPreset, error) {
	// 把 *T 转 any（nil → nil；COALESCE 命中 fallback 不改原列）。
	var nameArg, descArg, forceOnArg, activeArg, rulesArg any
	if params.Name != nil {
		nameArg = *params.Name
	}
	if params.Description != nil {
		descArg = *params.Description
	}
	if params.IsForceOn != nil {
		forceOnArg = *params.IsForceOn
	}
	if params.IsActive != nil {
		activeArg = *params.IsActive
	}
	if params.Rules != nil {
		raw, err := json.Marshal(*params.Rules)
		if err != nil {
			return BypassPreset{}, fmt.Errorf("marshal bypass preset rules: %w", err)
		}
		rulesArg = raw
	}

	var it BypassPreset
	row := r.db.QueryRowContext(ctx, updateBypassPresetSQL, nameArg, descArg, forceOnArg, activeArg, rulesArg, id)
	if err := scanBypassPreset(row, &it); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return BypassPreset{}, fmt.Errorf("update bypass preset: %w", err)
		}
		// 影响 0 行：区分「命中 is_system 兜底」与「行不存在」。
		var isSystem bool
		if e := r.db.QueryRowContext(ctx, checkBypassPresetIsSystemSQL, id).Scan(&isSystem); e != nil {
			if errors.Is(e, sql.ErrNoRows) {
				return BypassPreset{}, fmt.Errorf("update bypass preset: %w", sql.ErrNoRows)
			}
			return BypassPreset{}, fmt.Errorf("check bypass preset is_system: %w", e)
		}
		if isSystem {
			return BypassPreset{}, ErrSystemBypassPresetImmutable
		}
		return BypassPreset{}, fmt.Errorf("update bypass preset: %w", sql.ErrNoRows)
	}
	return it, nil
}

func (r *Repository) DeleteBypassPreset(ctx context.Context, id string) error {
	// 先查 is_system；命中 → sentinel。即使 Go 层漏检，SQL `AND is_system = 0`
	// 也兜底保证不会真的 DELETE。
	var isSystem bool
	if err := r.db.QueryRowContext(ctx, checkBypassPresetIsSystemSQL, id).Scan(&isSystem); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("delete bypass preset: %w", sql.ErrNoRows)
		}
		return fmt.Errorf("check bypass preset is_system: %w", err)
	}
	if isSystem {
		return ErrSystemBypassPresetImmutable
	}
	tag, err := r.db.ExecContext(ctx, deleteBypassPresetSQL, id)
	if err != nil {
		return fmt.Errorf("delete bypass preset: %w", err)
	}
	if n, _ := tag.RowsAffected(); n == 0 {
		// 极小概率：is_system 查询与 DELETE 之间被改为 system。
		return fmt.Errorf("delete bypass preset: %w", sql.ErrNoRows)
	}
	return nil
}

func (r *Repository) ListBypassRules(ctx context.Context, hostID *string) ([]BypassRule, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if hostID == nil {
		rows, err = r.db.QueryContext(ctx, listBypassRulesGlobalOnlySQL)
	} else {
		rows, err = r.db.QueryContext(ctx, listBypassRulesGlobalOrHostSQL, *hostID)
	}
	if err != nil {
		return nil, fmt.Errorf("query bypass rules: %w", err)
	}
	defer rows.Close()

	out := make([]BypassRule, 0)
	for rows.Next() {
		var it BypassRule
		if err := scanBypassRule(rows, &it); err != nil {
			return nil, fmt.Errorf("scan bypass rule: %w", err)
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate bypass rules: %w", err)
	}
	return out, nil
}

func (r *Repository) CreateBypassRule(ctx context.Context, params CreateBypassRuleParams) (BypassRule, error) {
	if params.Scope == "host" && (params.HostID == nil || *params.HostID == "") {
		return BypassRule{}, fmt.Errorf("create bypass rule: scope=host requires non-empty host_id")
	}
	if params.Scope == "global" && params.HostID != nil && *params.HostID != "" {
		return BypassRule{}, fmt.Errorf("create bypass rule: scope=global must not carry host_id")
	}
	var it BypassRule
	row := r.db.QueryRowContext(ctx, createBypassRuleSQL,
		uuid.NewString(),
		params.Scope, nullableUUIDArg(params.HostID), params.RuleType, params.Value, nullIfEmpty(params.Note), params.IsRisky,
	)
	if err := scanBypassRule(row, &it); err != nil {
		return BypassRule{}, fmt.Errorf("create bypass rule: %w", err)
	}
	return it, nil
}

func (r *Repository) UpdateBypassRule(ctx context.Context, id string, params UpdateBypassRuleParams) (BypassRule, error) {
	var valueArg, noteArg, riskyArg any
	if params.Value != nil {
		valueArg = *params.Value
	}
	if params.Note != nil {
		noteArg = *params.Note
	}
	if params.IsRisky != nil {
		riskyArg = *params.IsRisky
	}
	var it BypassRule
	row := r.db.QueryRowContext(ctx, updateBypassRuleSQL, valueArg, noteArg, riskyArg, id)
	if err := scanBypassRule(row, &it); err != nil {
		return BypassRule{}, fmt.Errorf("update bypass rule: %w", err)
	}
	return it, nil
}

func (r *Repository) DeleteBypassRule(ctx context.Context, id string) error {
	tag, err := r.db.ExecContext(ctx, deleteBypassRuleSQL, id)
	if err != nil {
		return fmt.Errorf("delete bypass rule: %w", err)
	}
	if n, _ := tag.RowsAffected(); n == 0 {
		return fmt.Errorf("delete bypass rule: %w", sql.ErrNoRows)
	}
	return nil
}

func (r *Repository) GetBypassRuleByID(ctx context.Context, id string) (BypassRule, error) {
	var it BypassRule
	if err := scanBypassRule(r.db.QueryRowContext(ctx, getBypassRuleByIDSQL, id), &it); err != nil {
		return BypassRule{}, fmt.Errorf("get bypass rule by id: %w", err)
	}
	return it, nil
}

func (r *Repository) ListBypassBindingsByHost(ctx context.Context, hostID string) ([]BypassBinding, error) {
	rows, err := r.db.QueryContext(ctx, listBypassBindingsByHostSQL, hostID)
	if err != nil {
		return nil, fmt.Errorf("query bypass bindings: %w", err)
	}
	defer rows.Close()

	out := make([]BypassBinding, 0)
	for rows.Next() {
		var it BypassBinding
		if err := scanBypassBinding(rows, &it); err != nil {
			return nil, fmt.Errorf("scan bypass binding: %w", err)
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate bypass bindings: %w", err)
	}
	return out, nil
}

func (r *Repository) CreateBypassBinding(ctx context.Context, params CreateBypassBindingParams) (BypassBinding, error) {
	source := params.Source
	if source == "" {
		source = "admin"
	}
	var it BypassBinding
	row := r.db.QueryRowContext(ctx, createBypassBindingSQL,
		uuid.NewString(),
		params.HostID,
		nullableUUIDArg(params.PresetID),
		nullableUUIDArg(params.RuleID),
		params.Enabled, source,
	)
	if err := scanBypassBinding(row, &it); err != nil {
		return BypassBinding{}, fmt.Errorf("create bypass binding: %w", err)
	}
	return it, nil
}

func (r *Repository) DeleteBypassBinding(ctx context.Context, id string) error {
	tag, err := r.db.ExecContext(ctx, deleteBypassBindingSQL, id)
	if err != nil {
		return fmt.Errorf("delete bypass binding: %w", err)
	}
	if n, _ := tag.RowsAffected(); n == 0 {
		return fmt.Errorf("delete bypass binding: %w", sql.ErrNoRows)
	}
	return nil
}

func (r *Repository) ListBypassSnapshotsByHost(ctx context.Context, hostID string, limit int) ([]BypassSnapshot, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.QueryContext(ctx, listBypassSnapshotsByHostSQL, hostID, limit)
	if err != nil {
		return nil, fmt.Errorf("query bypass snapshots: %w", err)
	}
	defer rows.Close()

	out := make([]BypassSnapshot, 0)
	for rows.Next() {
		var it BypassSnapshot
		if err := scanBypassSnapshot(rows, &it); err != nil {
			return nil, fmt.Errorf("scan bypass snapshot: %w", err)
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate bypass snapshots: %w", err)
	}
	return out, nil
}

func (r *Repository) CreateBypassSnapshot(ctx context.Context, params CreateBypassSnapshotParams) (BypassSnapshot, error) {
	cidrs := params.WhitelistCIDRsJSON
	if len(cidrs) == 0 {
		cidrs = json.RawMessage(`{"version":3,"rules":[]}`)
	}
	domains := params.WhitelistDomainsJSON
	if len(domains) == 0 {
		domains = json.RawMessage(`{"version":3,"rules":[]}`)
	}
	source := params.Source
	if source == "" {
		source = "apply"
	}
	var it BypassSnapshot
	row := r.db.QueryRowContext(ctx, createBypassSnapshotSQL,
		uuid.NewString(),
		params.HostID, params.Version, params.ConfigHash,
		[]byte(cidrs), []byte(domains),
		source,
		nullableUUIDArg(params.CreatedBy),
	)
	if err := scanBypassSnapshot(row, &it); err != nil {
		return BypassSnapshot{}, fmt.Errorf("create bypass snapshot: %w", err)
	}
	return it, nil
}

func (r *Repository) UpdateBypassSnapshotStatus(ctx context.Context, id string, status string) (BypassSnapshot, error) {
	var it BypassSnapshot
	row := r.db.QueryRowContext(ctx, updateBypassSnapshotStatusSQL, status, id)
	if err := scanBypassSnapshot(row, &it); err != nil {
		return BypassSnapshot{}, fmt.Errorf("update bypass snapshot status: %w", err)
	}
	return it, nil
}

func (r *Repository) GetLatestAppliedBypassSnapshot(ctx context.Context, hostID string) (BypassSnapshot, error) {
	var it BypassSnapshot
	row := r.db.QueryRowContext(ctx, getLatestAppliedBypassSnapshotSQL, hostID)
	if err := scanBypassSnapshot(row, &it); err != nil {
		return BypassSnapshot{}, fmt.Errorf("get latest applied bypass snapshot: %w", err)
	}
	return it, nil
}

func (r *Repository) GetBypassSnapshotByID(ctx context.Context, id string) (BypassSnapshot, error) {
	var it BypassSnapshot
	row := r.db.QueryRowContext(ctx, getBypassSnapshotByIDSQL, id)
	if err := scanBypassSnapshot(row, &it); err != nil {
		return BypassSnapshot{}, fmt.Errorf("get bypass snapshot by id: %w", err)
	}
	return it, nil
}

func (r *Repository) InsertBypassAuditLog(ctx context.Context, params InsertBypassAuditLogParams) (string, error) {
	var beforeArg, afterArg any
	if len(params.Before) > 0 {
		beforeArg = []byte(params.Before)
	}
	if len(params.After) > 0 {
		afterArg = []byte(params.After)
	}
	var id string
	var createdAt any // 占位接住 RETURNING 的 created_at，调用方不需要它。
	if err := r.db.QueryRowContext(ctx, insertBypassAuditLogSQL,
		uuid.NewString(),
		nullableUUIDArg(params.ActorID), nullIfEmpty(params.ActorIP),
		params.Action, params.TargetKind,
		nullableUUIDArg(params.TargetID),
		beforeArg, afterArg, nullIfEmpty(params.Note),
	).Scan(&id, &createdAt); err != nil {
		return "", fmt.Errorf("insert bypass audit log: %w", err)
	}
	return id, nil
}

func (r *Repository) ListBypassAuditLogByTarget(ctx context.Context, targetKind, targetID string) ([]BypassAuditLog, error) {
	rows, err := r.db.QueryContext(ctx, listBypassAuditLogByTargetSQL, targetKind, targetID)
	if err != nil {
		return nil, fmt.Errorf("query bypass audit log: %w", err)
	}
	defer rows.Close()

	out := make([]BypassAuditLog, 0)
	for rows.Next() {
		var it BypassAuditLog
		var before, after jsonText
		if err := rows.Scan(
			&it.ID, &it.ActorID, &it.ActorIP, &it.Action, &it.TargetKind,
			&it.TargetID, &before, &after, &it.Note, &it.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan bypass audit log: %w", err)
		}
		it.Before = before.RawMessage()
		it.After = after.RawMessage()
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate bypass audit log: %w", err)
	}
	return out, nil
}
