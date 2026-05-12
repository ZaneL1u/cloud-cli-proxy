package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ErrSystemBypassPresetImmutable 表示尝试删除或修改 is_system=true 的预设。
// 数据层做最小拦截，Phase 46 handler 会把它翻译成 HTTP 403 + 错误码
// BYPASS_PRESET_IMMUTABLE。SQL 层在 updateBypassPresetSQL / deleteBypassPresetSQL
// 同时附加 `is_system = FALSE` WHERE 兜底，即使绕过 Go 层校验也不会误删。
var ErrSystemBypassPresetImmutable = errors.New("bypass preset is system preset and cannot be deleted or modified")

// ---------------------------------------------------------------------------
// SQL 常量（包级 const，供 queries_bypass_test.go 文本断言锁定）
// 命名规范沿用 queries.go 已有模式（listHostsByUserIDSQL 等）。
// 所有 SELECT 用 `id::text` 把 UUID 转为 string；所有占位符使用 $N，禁止 fmt.Sprintf 拼接。
// ---------------------------------------------------------------------------

const listBypassPresetsSQL = `
	SELECT id::text, slug, name, COALESCE(description, ''),
	       is_system, is_force_on, is_active, rules, created_at, updated_at
	FROM host_bypass_presets
	ORDER BY is_system DESC, slug ASC
`

const getBypassPresetBySlugSQL = `
	SELECT id::text, slug, name, COALESCE(description, ''),
	       is_system, is_force_on, is_active, rules, created_at, updated_at
	FROM host_bypass_presets WHERE slug = $1
`

const getBypassPresetByIDSQL = `
	SELECT id::text, slug, name, COALESCE(description, ''),
	       is_system, is_force_on, is_active, rules, created_at, updated_at
	FROM host_bypass_presets WHERE id = $1
`

const createBypassPresetSQL = `
	INSERT INTO host_bypass_presets (slug, name, description, is_force_on, is_active, rules)
	VALUES ($1, $2, $3, $4, $5, $6)
	RETURNING id::text, slug, name, COALESCE(description, ''),
	          is_system, is_force_on, is_active, rules, created_at, updated_at
`

// updateBypassPresetSQL 用 COALESCE($N, col) 实现部分字段更新；
// 关键防御：`AND is_system = FALSE` 兜底，系统预设永不被改（即使 Go 层漏了）。
const updateBypassPresetSQL = `
	UPDATE host_bypass_presets SET
		name        = COALESCE($2, name),
		description = COALESCE($3, description),
		is_force_on = COALESCE($4, is_force_on),
		is_active   = COALESCE($5, is_active),
		rules       = COALESCE($6, rules),
		updated_at  = NOW()
	WHERE id = $1 AND is_system = FALSE
	RETURNING id::text, slug, name, COALESCE(description, ''),
	          is_system, is_force_on, is_active, rules, created_at, updated_at
`

// deleteBypassPresetSQL 同样附加 `AND is_system = FALSE` 兜底。
const deleteBypassPresetSQL = `DELETE FROM host_bypass_presets WHERE id = $1 AND is_system = FALSE`

// checkBypassPresetIsSystemSQL 供 Go 层先查 is_system 标志，决定返回
// ErrSystemBypassPresetImmutable 还是 ErrNoRows。
const checkBypassPresetIsSystemSQL = `SELECT is_system FROM host_bypass_presets WHERE id = $1`

// listBypassRulesGlobalOnlySQL 仅返回 scope='global' 的规则（hostID 入参为 nil 时使用）。
const listBypassRulesGlobalOnlySQL = `
	SELECT id::text, scope, host_id::text, rule_type, value, COALESCE(note, ''),
	       is_risky, created_at, updated_at
	FROM host_bypass_rules
	WHERE scope = 'global'
	ORDER BY created_at ASC
`

// listBypassRulesGlobalOrHostSQL 返回 scope='global' 或 scope='host' 且 host_id=$1 的规则。
const listBypassRulesGlobalOrHostSQL = `
	SELECT id::text, scope, host_id::text, rule_type, value, COALESCE(note, ''),
	       is_risky, created_at, updated_at
	FROM host_bypass_rules
	WHERE scope = 'global' OR (scope = 'host' AND host_id = $1)
	ORDER BY scope DESC, created_at ASC
`

const createBypassRuleSQL = `
	INSERT INTO host_bypass_rules (scope, host_id, rule_type, value, note, is_risky)
	VALUES ($1, $2, $3, $4, $5, $6)
	RETURNING id::text, scope, host_id::text, rule_type, value, COALESCE(note, ''),
	          is_risky, created_at, updated_at
`

const updateBypassRuleSQL = `
	UPDATE host_bypass_rules SET
		value      = COALESCE($2, value),
		note       = COALESCE($3, note),
		is_risky   = COALESCE($4, is_risky),
		updated_at = NOW()
	WHERE id = $1
	RETURNING id::text, scope, host_id::text, rule_type, value, COALESCE(note, ''),
	          is_risky, created_at, updated_at
`

const deleteBypassRuleSQL = `DELETE FROM host_bypass_rules WHERE id = $1`

const listBypassBindingsByHostSQL = `
	SELECT id::text, host_id::text, preset_id::text, rule_id::text,
	       enabled, source, created_at
	FROM host_bypass_bindings
	WHERE host_id = $1
	ORDER BY created_at ASC
`

const createBypassBindingSQL = `
	INSERT INTO host_bypass_bindings (host_id, preset_id, rule_id, enabled, source)
	VALUES ($1, $2, $3, $4, $5)
	RETURNING id::text, host_id::text, preset_id::text, rule_id::text,
	          enabled, source, created_at
`

const deleteBypassBindingSQL = `DELETE FROM host_bypass_bindings WHERE id = $1`

const listBypassSnapshotsByHostSQL = `
	SELECT id::text, host_id::text, version, config_hash,
	       whitelist_cidrs_json, whitelist_domains_json,
	       applied_status, created_by::text, created_at
	FROM host_bypass_snapshots
	WHERE host_id = $1
	ORDER BY version DESC
	LIMIT $2
`

const createBypassSnapshotSQL = `
	INSERT INTO host_bypass_snapshots
		(host_id, version, config_hash, whitelist_cidrs_json, whitelist_domains_json, created_by)
	VALUES ($1, $2, $3, $4, $5, $6)
	RETURNING id::text, host_id::text, version, config_hash,
	          whitelist_cidrs_json, whitelist_domains_json,
	          applied_status, created_by::text, created_at
`

const updateBypassSnapshotStatusSQL = `
	UPDATE host_bypass_snapshots SET applied_status = $2 WHERE id = $1
	RETURNING id::text, host_id::text, version, config_hash,
	          whitelist_cidrs_json, whitelist_domains_json,
	          applied_status, created_by::text, created_at
`

// getLatestAppliedBypassSnapshotSQL 返回 host 最近一次 applied_status='applied' 的 snapshot；
// version DESC + LIMIT 1 决定语义（Phase 47 rollback 需要它来定位回滚目标）。
const getLatestAppliedBypassSnapshotSQL = `
	SELECT id::text, host_id::text, version, config_hash,
	       whitelist_cidrs_json, whitelist_domains_json,
	       applied_status, created_by::text, created_at
	FROM host_bypass_snapshots
	WHERE host_id = $1 AND applied_status = 'applied'
	ORDER BY version DESC
	LIMIT 1
`

const insertBypassAuditLogSQL = `
	INSERT INTO host_bypass_audit_log
		(actor_id, actor_ip, action, target_kind, target_id, before, after, note)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	RETURNING id::text, created_at
`

const listBypassAuditLogByTargetSQL = `
	SELECT id::text, actor_id::text, COALESCE(actor_ip, ''), action, target_kind,
	       target_id::text, before, after, COALESCE(note, ''), created_at
	FROM host_bypass_audit_log
	WHERE target_kind = $1 AND target_id = $2
	ORDER BY created_at DESC
`

// ---------------------------------------------------------------------------
// 19 个 Repository 方法 — Task 2b 真实 pgx v5 实现。
// 签名与 SQL 常量在 Task 3 已通过反射 + 文本断言 lock 死，本步骤只填方法体。
// ---------------------------------------------------------------------------

// scanBypassPreset 将一行 host_bypass_presets SELECT 结果扫到 BypassPreset。
// 列顺序与所有 preset SQL 常量的 SELECT/RETURNING 段保持一致。
func scanBypassPreset(row pgx.Row, out *BypassPreset) error {
	var rawRules json.RawMessage
	if err := row.Scan(
		&out.ID, &out.Slug, &out.Name, &out.Description,
		&out.IsSystem, &out.IsForceOn, &out.IsActive, &rawRules,
		&out.CreatedAt, &out.UpdatedAt,
	); err != nil {
		return err
	}
	if len(rawRules) > 0 {
		_ = json.Unmarshal(rawRules, &out.Rules)
	}
	return nil
}

// scanBypassRule 同上，host_bypass_rules 列顺序对齐。
func scanBypassRule(row pgx.Row, out *BypassRule) error {
	return row.Scan(
		&out.ID, &out.Scope, &out.HostID, &out.RuleType, &out.Value, &out.Note,
		&out.IsRisky, &out.CreatedAt, &out.UpdatedAt,
	)
}

// scanBypassBinding 同上，host_bypass_bindings 列顺序对齐。
func scanBypassBinding(row pgx.Row, out *BypassBinding) error {
	return row.Scan(
		&out.ID, &out.HostID, &out.PresetID, &out.RuleID,
		&out.Enabled, &out.Source, &out.CreatedAt,
	)
}

// scanBypassSnapshot 同上，host_bypass_snapshots 列顺序对齐。
func scanBypassSnapshot(row pgx.Row, out *BypassSnapshot) error {
	return row.Scan(
		&out.ID, &out.HostID, &out.Version, &out.ConfigHash,
		&out.WhitelistCIDRsJSON, &out.WhitelistDomainsJSON,
		&out.AppliedStatus, &out.CreatedBy, &out.CreatedAt,
	)
}

func (r *Repository) ListBypassPresets(ctx context.Context) ([]BypassPreset, error) {
	rows, err := r.db.Query(ctx, listBypassPresetsSQL)
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
	if err := scanBypassPreset(r.db.QueryRow(ctx, getBypassPresetBySlugSQL, slug), &it); err != nil {
		return BypassPreset{}, fmt.Errorf("get bypass preset by slug: %w", err)
	}
	return it, nil
}

func (r *Repository) GetBypassPresetByID(ctx context.Context, id string) (BypassPreset, error) {
	var it BypassPreset
	if err := scanBypassPreset(r.db.QueryRow(ctx, getBypassPresetByIDSQL, id), &it); err != nil {
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
	row := r.db.QueryRow(ctx, createBypassPresetSQL,
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
	row := r.db.QueryRow(ctx, updateBypassPresetSQL, id, nameArg, descArg, forceOnArg, activeArg, rulesArg)
	if err := scanBypassPreset(row, &it); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return BypassPreset{}, fmt.Errorf("update bypass preset: %w", err)
		}
		// 影响 0 行：区分「命中 is_system 兜底」与「行不存在」。
		var isSystem bool
		if e := r.db.QueryRow(ctx, checkBypassPresetIsSystemSQL, id).Scan(&isSystem); e != nil {
			if errors.Is(e, pgx.ErrNoRows) {
				return BypassPreset{}, fmt.Errorf("update bypass preset: %w", pgx.ErrNoRows)
			}
			return BypassPreset{}, fmt.Errorf("check bypass preset is_system: %w", e)
		}
		if isSystem {
			return BypassPreset{}, ErrSystemBypassPresetImmutable
		}
		return BypassPreset{}, fmt.Errorf("update bypass preset: %w", pgx.ErrNoRows)
	}
	return it, nil
}

func (r *Repository) DeleteBypassPreset(ctx context.Context, id string) error {
	// 先查 is_system；命中 → sentinel。即使 Go 层漏检，SQL `AND is_system = FALSE`
	// 也兜底保证不会真的 DELETE。
	var isSystem bool
	if err := r.db.QueryRow(ctx, checkBypassPresetIsSystemSQL, id).Scan(&isSystem); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("delete bypass preset: %w", pgx.ErrNoRows)
		}
		return fmt.Errorf("check bypass preset is_system: %w", err)
	}
	if isSystem {
		return ErrSystemBypassPresetImmutable
	}
	tag, err := r.db.Exec(ctx, deleteBypassPresetSQL, id)
	if err != nil {
		return fmt.Errorf("delete bypass preset: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// 极小概率：is_system 查询与 DELETE 之间被改为 system。
		return fmt.Errorf("delete bypass preset: %w", pgx.ErrNoRows)
	}
	return nil
}

func (r *Repository) ListBypassRules(ctx context.Context, hostID *string) ([]BypassRule, error) {
	var (
		rows pgx.Rows
		err  error
	)
	if hostID == nil {
		rows, err = r.db.Query(ctx, listBypassRulesGlobalOnlySQL)
	} else {
		rows, err = r.db.Query(ctx, listBypassRulesGlobalOrHostSQL, *hostID)
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
	var hostArg any
	if params.HostID != nil {
		hostArg = *params.HostID
	}
	var it BypassRule
	row := r.db.QueryRow(ctx, createBypassRuleSQL,
		params.Scope, hostArg, params.RuleType, params.Value, nullIfEmpty(params.Note), params.IsRisky,
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
	row := r.db.QueryRow(ctx, updateBypassRuleSQL, id, valueArg, noteArg, riskyArg)
	if err := scanBypassRule(row, &it); err != nil {
		return BypassRule{}, fmt.Errorf("update bypass rule: %w", err)
	}
	return it, nil
}

func (r *Repository) DeleteBypassRule(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, deleteBypassRuleSQL, id)
	if err != nil {
		return fmt.Errorf("delete bypass rule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("delete bypass rule: %w", pgx.ErrNoRows)
	}
	return nil
}

func (r *Repository) ListBypassBindingsByHost(ctx context.Context, hostID string) ([]BypassBinding, error) {
	rows, err := r.db.Query(ctx, listBypassBindingsByHostSQL, hostID)
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
	var presetArg, ruleArg any
	if params.PresetID != nil {
		presetArg = *params.PresetID
	}
	if params.RuleID != nil {
		ruleArg = *params.RuleID
	}
	source := params.Source
	if source == "" {
		source = "admin"
	}
	var it BypassBinding
	row := r.db.QueryRow(ctx, createBypassBindingSQL,
		params.HostID, presetArg, ruleArg, params.Enabled, source,
	)
	if err := scanBypassBinding(row, &it); err != nil {
		return BypassBinding{}, fmt.Errorf("create bypass binding: %w", err)
	}
	return it, nil
}

func (r *Repository) DeleteBypassBinding(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, deleteBypassBindingSQL, id)
	if err != nil {
		return fmt.Errorf("delete bypass binding: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("delete bypass binding: %w", pgx.ErrNoRows)
	}
	return nil
}

func (r *Repository) ListBypassSnapshotsByHost(ctx context.Context, hostID string, limit int) ([]BypassSnapshot, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.Query(ctx, listBypassSnapshotsByHostSQL, hostID, limit)
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
	var createdByArg any
	if params.CreatedBy != nil {
		createdByArg = *params.CreatedBy
	}
	var it BypassSnapshot
	row := r.db.QueryRow(ctx, createBypassSnapshotSQL,
		params.HostID, params.Version, params.ConfigHash, []byte(cidrs), []byte(domains), createdByArg,
	)
	if err := scanBypassSnapshot(row, &it); err != nil {
		return BypassSnapshot{}, fmt.Errorf("create bypass snapshot: %w", err)
	}
	return it, nil
}

func (r *Repository) UpdateBypassSnapshotStatus(ctx context.Context, id string, status string) (BypassSnapshot, error) {
	var it BypassSnapshot
	row := r.db.QueryRow(ctx, updateBypassSnapshotStatusSQL, id, status)
	if err := scanBypassSnapshot(row, &it); err != nil {
		return BypassSnapshot{}, fmt.Errorf("update bypass snapshot status: %w", err)
	}
	return it, nil
}

func (r *Repository) GetLatestAppliedBypassSnapshot(ctx context.Context, hostID string) (BypassSnapshot, error) {
	var it BypassSnapshot
	row := r.db.QueryRow(ctx, getLatestAppliedBypassSnapshotSQL, hostID)
	if err := scanBypassSnapshot(row, &it); err != nil {
		return BypassSnapshot{}, fmt.Errorf("get latest applied bypass snapshot: %w", err)
	}
	return it, nil
}

func (r *Repository) InsertBypassAuditLog(ctx context.Context, params InsertBypassAuditLogParams) (string, error) {
	var actorArg, targetArg, beforeArg, afterArg any
	if params.ActorID != nil {
		actorArg = *params.ActorID
	}
	if params.TargetID != nil {
		targetArg = *params.TargetID
	}
	if len(params.Before) > 0 {
		beforeArg = []byte(params.Before)
	}
	if len(params.After) > 0 {
		afterArg = []byte(params.After)
	}
	var id string
	var createdAt any // 占位接住 RETURNING 的 created_at，调用方不需要它。
	if err := r.db.QueryRow(ctx, insertBypassAuditLogSQL,
		actorArg, nullIfEmpty(params.ActorIP), params.Action, params.TargetKind,
		targetArg, beforeArg, afterArg, nullIfEmpty(params.Note),
	).Scan(&id, &createdAt); err != nil {
		return "", fmt.Errorf("insert bypass audit log: %w", err)
	}
	return id, nil
}

func (r *Repository) ListBypassAuditLogByTarget(ctx context.Context, targetKind, targetID string) ([]BypassAuditLog, error) {
	rows, err := r.db.Query(ctx, listBypassAuditLogByTargetSQL, targetKind, targetID)
	if err != nil {
		return nil, fmt.Errorf("query bypass audit log: %w", err)
	}
	defer rows.Close()

	out := make([]BypassAuditLog, 0)
	for rows.Next() {
		var it BypassAuditLog
		if err := rows.Scan(
			&it.ID, &it.ActorID, &it.ActorIP, &it.Action, &it.TargetKind,
			&it.TargetID, &it.Before, &it.After, &it.Note, &it.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan bypass audit log: %w", err)
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate bypass audit log: %w", err)
	}
	return out, nil
}
