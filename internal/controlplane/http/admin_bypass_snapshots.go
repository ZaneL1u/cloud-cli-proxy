package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	nethttp "net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/zanel1u/cloud-cli-proxy/internal/agentapi"
	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

// AdminBypassSnapshotStore 聚合 preview / apply / rollback / effective 4 个 handler
// 所需的全部 Repository 方法子集。测试通过 stub 实现该接口。
//
// 包含 GetHost 用于 hostID 存在性校验（404 + BYPASS_HOST_NOT_FOUND）；
// 包含 GetBypassSnapshotByID 用于 rollback 校验 target（WARN-4 关键依赖）；
// 不包含 UpdateBypassSnapshotStatus —— rollback 不修改 target 状态（WARN-4 核心）。
type AdminBypassSnapshotStore interface {
	// host 存在性校验
	GetHost(context.Context, string) (repository.Host, error)

	// render 输入聚合
	ListBypassBindingsByHost(context.Context, string) ([]repository.BypassBinding, error)
	GetBypassPresetByID(context.Context, string) (repository.BypassPreset, error)
	ListBypassRules(context.Context, *string) ([]repository.BypassRule, error)

	// snapshot 生命周期
	ListBypassSnapshotsByHost(context.Context, string, int) ([]repository.BypassSnapshot, error)
	CreateBypassSnapshot(context.Context, repository.CreateBypassSnapshotParams) (repository.BypassSnapshot, error)
	GetBypassSnapshotByID(context.Context, string) (repository.BypassSnapshot, error)
	GetLatestAppliedBypassSnapshot(context.Context, string) (repository.BypassSnapshot, error)

	// audit
	InsertBypassAuditLog(context.Context, repository.InsertBypassAuditLogParams) (string, error)
}

type AdminBypassSnapshotsHandler struct {
	logger  *slog.Logger
	store   AdminBypassSnapshotStore
	actions HostActionQueuer
	events  EventRecorder
}

func NewAdminBypassSnapshotsHandler(
	logger *slog.Logger,
	store AdminBypassSnapshotStore,
	actions HostActionQueuer,
	events EventRecorder,
) *AdminBypassSnapshotsHandler {
	return &AdminBypassSnapshotsHandler{logger: logger, store: store, actions: actions, events: events}
}

// collectRenderInput 把 host 的所有有效 binding 展开为 RenderBypassConfig 所需的输入。
//
// 启用规则：
//   - preset binding：binding.Enabled=true 且 preset.IsActive=true（或 preset.IsForceOn=true 强制）→ 进 Presets
//   - rule binding：binding.Enabled=true → 进 Rules
//
// 同时把 scope='host' 的 host 自身规则全部纳入（无需 binding）—— 这是 host 维度的兜底，
// admin 直接在 host 上加规则但忘了 bind 也算生效。
func (h *AdminBypassSnapshotsHandler) collectRenderInput(ctx context.Context, hostID string) (BypassRenderInput, error) {
	bindings, err := h.store.ListBypassBindingsByHost(ctx, hostID)
	if err != nil {
		return BypassRenderInput{}, fmt.Errorf("list bindings: %w", err)
	}

	input := BypassRenderInput{HostID: hostID}

	// 收集启用的 preset。
	for _, b := range bindings {
		if !b.Enabled {
			continue
		}
		if b.PresetID == nil || *b.PresetID == "" {
			continue
		}
		preset, perr := h.store.GetBypassPresetByID(ctx, *b.PresetID)
		if perr != nil {
			if errors.Is(perr, pgx.ErrNoRows) {
				// preset 被删但 binding 残留，跳过即可。
				continue
			}
			return BypassRenderInput{}, fmt.Errorf("get preset %s: %w", *b.PresetID, perr)
		}
		// 仅 active 或 force_on 视为生效。
		if !preset.IsActive && !preset.IsForceOn {
			continue
		}
		input.Presets = append(input.Presets, preset)
	}

	// 收集 binding 引用的全局规则 + host 自身规则。
	allRules, err := h.store.ListBypassRules(ctx, &hostID)
	if err != nil {
		return BypassRenderInput{}, fmt.Errorf("list rules: %w", err)
	}
	// 把 binding 引用的 rule_id 集合化便于 filter。
	boundRuleIDs := map[string]struct{}{}
	for _, b := range bindings {
		if !b.Enabled {
			continue
		}
		if b.RuleID != nil && *b.RuleID != "" {
			boundRuleIDs[*b.RuleID] = struct{}{}
		}
	}
	for _, r := range allRules {
		// host scope 自身规则：直接纳入（host 维度兜底）。
		if r.Scope == "host" && r.HostID != nil && *r.HostID == hostID {
			input.Rules = append(input.Rules, r)
			continue
		}
		// global 规则：仅当被 binding 显式引用才纳入。
		if r.Scope == "global" {
			if _, ok := boundRuleIDs[r.ID]; ok {
				input.Rules = append(input.Rules, r)
			}
		}
	}

	return input, nil
}

// nextSnapshotVersion 取 host 现有最大 version+1；空表返回 1。
func (h *AdminBypassSnapshotsHandler) nextSnapshotVersion(ctx context.Context, hostID string) (int64, error) {
	list, err := h.store.ListBypassSnapshotsByHost(ctx, hostID, 1)
	if err != nil {
		return 0, err
	}
	if len(list) == 0 {
		return 1, nil
	}
	return list[0].Version + 1, nil
}

// findSnapshotByConfigHash 在 host 现存 snapshot 中按 config_hash 找回。
// apply handler 在 UNIQUE 冲突时调用，实现幂等 200。
// 为不引入额外 SQL，直接拉取若干最近 snapshot 在内存里 filter。
func (h *AdminBypassSnapshotsHandler) findSnapshotByConfigHash(ctx context.Context, hostID, hash string) (repository.BypassSnapshot, bool, error) {
	list, err := h.store.ListBypassSnapshotsByHost(ctx, hostID, 200)
	if err != nil {
		return repository.BypassSnapshot{}, false, err
	}
	for _, s := range list {
		if s.ConfigHash == hash {
			return s, true, nil
		}
	}
	return repository.BypassSnapshot{}, false, nil
}

// previewResponse 是 Preview handler 返回 body 的全部字段集合。
type previewResponse struct {
	ConfigHash               string          `json:"config_hash"`
	VersionCurrent           int64           `json:"version_current"`
	VersionNext              int64           `json:"version_next"`
	WhitelistCIDRsRendered   json.RawMessage `json:"whitelist_cidrs_rendered"`
	WhitelistDomainsRendered json.RawMessage `json:"whitelist_domains_rendered"`
	NftDiff                  string          `json:"nft_diff"`
	RiskyCount               int             `json:"risky_count"`
	Summary                  string          `json:"summary"`
}

// Preview 渲染当前 host 全部启用规则为 sing-box rule-set + nft diff，不落库。
// POST /v1/admin/hosts/{hostID}/bypass/preview
func (h *AdminBypassSnapshotsHandler) Preview() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		hostID := strings.TrimSpace(r.PathValue("hostID"))
		if hostID == "" {
			writeBypassError(w, nethttp.StatusBadRequest, ErrCodeBypassInvalidRequest, "hostID is required")
			return
		}
		if _, err := h.store.GetHost(r.Context(), hostID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeBypassError(w, nethttp.StatusNotFound, ErrCodeBypassHostNotFound, "host not found")
				return
			}
			h.logger.Error("preview: get host failed", "host_id", hostID, "error", err)
			writeBypassError(w, nethttp.StatusInternalServerError, "INTERNAL", "get host failed")
			return
		}

		input, err := h.collectRenderInput(r.Context(), hostID)
		if err != nil {
			h.logger.Error("preview: collect render input failed", "host_id", hostID, "error", err)
			writeBypassError(w, nethttp.StatusInternalServerError, "INTERNAL", "collect render input failed")
			return
		}

		// 取上一个 applied snapshot 做 nft diff 起点（找不到忽略）。
		var prev *repository.BypassSnapshot
		if latest, lerr := h.store.GetLatestAppliedBypassSnapshot(r.Context(), hostID); lerr == nil {
			prev = &latest
		}

		// 取当前最新 version（不限 applied 状态）。
		var versionCurrent int64
		if list, _ := h.store.ListBypassSnapshotsByHost(r.Context(), hostID, 1); len(list) > 0 {
			versionCurrent = list[0].Version
		}

		out, err := RenderBypassConfig(input, prev)
		if err != nil {
			h.logger.Error("preview: render failed", "host_id", hostID, "error", err)
			writeBypassError(w, nethttp.StatusInternalServerError, "INTERNAL", "render failed")
			return
		}

		writeJSON(w, nethttp.StatusOK, previewResponse{
			ConfigHash:               out.ConfigHash,
			VersionCurrent:           versionCurrent,
			VersionNext:              versionCurrent + 1,
			WhitelistCIDRsRendered:   out.CIDRsJSON,
			WhitelistDomainsRendered: out.DomainsJSON,
			NftDiff:                  out.NftDiff,
			RiskyCount:               out.RiskyCount,
			Summary:                  out.Summary,
		})
	})
}

type applyRequest struct {
	Note string `json:"note,omitempty"`
}

type applyResponse struct {
	SnapshotID    string `json:"snapshot_id"`
	Version       int64  `json:"version"`
	ConfigHash    string `json:"config_hash"`
	AppliedStatus string `json:"applied_status"`
	TaskID        string `json:"task_id,omitempty"`
	Message       string `json:"message"`
}

// Apply 把当前规则集落为一行 host_bypass_snapshots（applied_status=pending），
// 同步 dispatch ActionReloadHostBypass（Phase 47 接管真实下发）。
//
// config_hash 重复 POST 返回 200 + 现有 snapshot id（幂等，不重写 audit，不重 dispatch）。
//
// POST /v1/admin/hosts/{hostID}/bypass/apply
func (h *AdminBypassSnapshotsHandler) Apply() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		hostID := strings.TrimSpace(r.PathValue("hostID"))
		if hostID == "" {
			writeBypassError(w, nethttp.StatusBadRequest, ErrCodeBypassInvalidRequest, "hostID is required")
			return
		}
		if _, err := h.store.GetHost(r.Context(), hostID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeBypassError(w, nethttp.StatusNotFound, ErrCodeBypassHostNotFound, "host not found")
				return
			}
			h.logger.Error("apply: get host failed", "host_id", hostID, "error", err)
			writeBypassError(w, nethttp.StatusInternalServerError, "INTERNAL", "get host failed")
			return
		}

		var req applyRequest
		if r.ContentLength > 0 {
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeBypassError(w, nethttp.StatusBadRequest, ErrCodeBypassInvalidRequest, "invalid request body")
				return
			}
		}

		input, err := h.collectRenderInput(r.Context(), hostID)
		if err != nil {
			h.logger.Error("apply: collect render input failed", "host_id", hostID, "error", err)
			writeBypassError(w, nethttp.StatusInternalServerError, "INTERNAL", "collect render input failed")
			return
		}

		var prev *repository.BypassSnapshot
		if latest, lerr := h.store.GetLatestAppliedBypassSnapshot(r.Context(), hostID); lerr == nil {
			prev = &latest
		}

		out, err := RenderBypassConfig(input, prev)
		if err != nil {
			h.logger.Error("apply: render failed", "host_id", hostID, "error", err)
			writeBypassError(w, nethttp.StatusInternalServerError, "INTERNAL", "render failed")
			return
		}

		nextVer, err := h.nextSnapshotVersion(r.Context(), hostID)
		if err != nil {
			h.logger.Error("apply: next version failed", "host_id", hostID, "error", err)
			writeBypassError(w, nethttp.StatusInternalServerError, "INTERNAL", "next version failed")
			return
		}

		snap, err := h.store.CreateBypassSnapshot(r.Context(), repository.CreateBypassSnapshotParams{
			HostID:               hostID,
			Version:              nextVer,
			ConfigHash:           out.ConfigHash,
			WhitelistCIDRsJSON:   out.CIDRsJSON,
			WhitelistDomainsJSON: out.DomainsJSON,
			Source:               "apply",
			CreatedBy:            actorIDPtr(r.Context()),
		})
		if err != nil {
			// UNIQUE(host_id, config_hash) 冲突 → 幂等命中：找回原 snapshot 直接 200。
			// 同时补一次 dispatch —— worker 占位实现是幂等的（Phase 47 接管真实 reload）。
			// CR-06：原本只 writeJSON 不 dispatch，TaskID 为零值 ""；前端 setTaskId("")
			// 不被 useTaskPolling 的 enabled 守卫 trigger，stageStatuses 永远卡在 dispatch
			// active，dialog 既不自动关闭也不报错。
			if isUniqueViolation(err) {
				if existing, found, ferr := h.findSnapshotByConfigHash(r.Context(), hostID, out.ConfigHash); ferr == nil && found {
					var taskID string
					if h.actions != nil {
						task, qErr := h.actions.QueueHostAction(r.Context(), hostID, agentapi.ActionReloadHostBypass, existing.ID)
						if qErr != nil {
							h.logger.Error("apply (idempotent): queue host action failed", "host_id", hostID, "snapshot_id", existing.ID, "error", qErr)
						} else {
							taskID = task.ID
						}
					}
					writeJSON(w, nethttp.StatusOK, applyResponse{
						SnapshotID:    existing.ID,
						Version:       existing.Version,
						ConfigHash:    existing.ConfigHash,
						AppliedStatus: existing.AppliedStatus,
						TaskID:        taskID,
						Message:       "config_hash 已存在，返回现有 snapshot（幂等）",
					})
					return
				}
			}
			h.logger.Error("apply: create snapshot failed", "host_id", hostID, "error", err)
			writeBypassError(w, nethttp.StatusInternalServerError, "INTERNAL", "create snapshot failed")
			return
		}

		// dispatch ActionReloadHostBypass —— Phase 47 worker 接管真实 reload。
		// dispatch 失败不阻塞主请求（snapshot 已落 pending，可后续重试）。
		var taskID string
		if h.actions != nil {
			task, qErr := h.actions.QueueHostAction(r.Context(), hostID, agentapi.ActionReloadHostBypass, snap.ID)
			if qErr != nil {
				h.logger.Error("apply: queue host action failed", "host_id", hostID, "snapshot_id", snap.ID, "error", qErr)
			} else {
				taskID = task.ID
			}
		}

		// 双轨审计：host_bypass_audit_log + events.RecordEvent("bypass.apply")。
		writeBypassAuditLog(r.Context(), h.logger, h.store, h.events, r, "apply", "snapshot", &snap.ID, prev, snap, req.Note)

		writeJSON(w, nethttp.StatusCreated, applyResponse{
			SnapshotID:    snap.ID,
			Version:       snap.Version,
			ConfigHash:    snap.ConfigHash,
			AppliedStatus: snap.AppliedStatus,
			TaskID:        taskID,
			Message:       "白名单变更不影响现有 TCP 连接，新连接才用新规则",
		})
	})
}

type rollbackRequest struct {
	TargetSnapshotID string `json:"target_snapshot_id"`
}

type rollbackResponse struct {
	SnapshotID               string `json:"snapshot_id"`
	RollbackTargetSnapshotID string `json:"rollback_target_snapshot_id"`
	Version                  int64  `json:"version"`
	AppliedStatus            string `json:"applied_status"`
	TaskID                   string `json:"task_id,omitempty"`
	Message                  string `json:"message"`
}

// Rollback WARN-4 修复版本：不修改 target 状态，新建一行 source='rollback' 的 pending snapshot。
//
//   - target 必须存在 + host_id 匹配 + applied_status='applied'，否则 404 / 409
//   - 若 target.ID == current latest applied → 幂等 200（不新建行，不 dispatch，不写 audit）
//   - 否则新建 snapshot：version+1、复制 target.ConfigHash + cidrs/domains、source='rollback'
//   - 若 UNIQUE(host_id, config_hash) 冲突（target hash 与现存 snapshot 同 hash），
//     退一步用 target.ConfigHash + ":rollback:" + version_str 绕开，保证总能产生新历史行
//   - 不调用 UpdateBypassSnapshotStatus(target.ID, ...) —— target.AppliedStatus 不变
//   - dispatch ActionReloadHostBypass payload 是新 snapshot.ID，不是 target.ID
//   - audit note 含 `rollback_target_snapshot_id=<id>` 前缀
//
// POST /v1/admin/hosts/{hostID}/bypass/rollback
func (h *AdminBypassSnapshotsHandler) Rollback() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		hostID := strings.TrimSpace(r.PathValue("hostID"))
		if hostID == "" {
			writeBypassError(w, nethttp.StatusBadRequest, ErrCodeBypassInvalidRequest, "hostID is required")
			return
		}
		if _, err := h.store.GetHost(r.Context(), hostID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeBypassError(w, nethttp.StatusNotFound, ErrCodeBypassHostNotFound, "host not found")
				return
			}
			h.logger.Error("rollback: get host failed", "host_id", hostID, "error", err)
			writeBypassError(w, nethttp.StatusInternalServerError, "INTERNAL", "get host failed")
			return
		}

		var req rollbackRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeBypassError(w, nethttp.StatusBadRequest, ErrCodeBypassInvalidRequest, "invalid request body")
			return
		}
		req.TargetSnapshotID = strings.TrimSpace(req.TargetSnapshotID)
		if req.TargetSnapshotID == "" {
			writeBypassError(w, nethttp.StatusBadRequest, ErrCodeBypassInvalidRequest, "target_snapshot_id is required")
			return
		}

		// 1. 取 target snapshot
		target, err := h.store.GetBypassSnapshotByID(r.Context(), req.TargetSnapshotID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeBypassError(w, nethttp.StatusNotFound, ErrCodeBypassSnapshotNotFound, "snapshot not found")
				return
			}
			h.logger.Error("rollback: get target failed", "host_id", hostID, "target_id", req.TargetSnapshotID, "error", err)
			writeBypassError(w, nethttp.StatusInternalServerError, "INTERNAL", "get target snapshot failed")
			return
		}
		// 2. 跨 host 校验（防越权 → 404 不暴露存在性）
		if target.HostID != hostID {
			writeBypassError(w, nethttp.StatusNotFound, ErrCodeBypassSnapshotNotFound, "snapshot not found")
			return
		}
		// 3. 校验 target.AppliedStatus
		if target.AppliedStatus != "applied" {
			writeBypassError(w, nethttp.StatusConflict, ErrCodeBypassSnapshotConflict, "只能回滚到曾经应用过的快照")
			return
		}
		// 4. 幂等：current latest applied 就是 target → 200，不新建，不 dispatch，不写 audit
		current, currentErr := h.store.GetLatestAppliedBypassSnapshot(r.Context(), hostID)
		if currentErr == nil && current.ID == target.ID {
			writeJSON(w, nethttp.StatusOK, rollbackResponse{
				SnapshotID:               current.ID,
				RollbackTargetSnapshotID: target.ID,
				Version:                  current.Version,
				AppliedStatus:            current.AppliedStatus,
				Message:                  "已在该快照上，无需回滚",
			})
			return
		}

		// 5. 新建 snapshot：version+1，复制 target config / hash，source='rollback'
		nextVer, err := h.nextSnapshotVersion(r.Context(), hostID)
		if err != nil {
			h.logger.Error("rollback: next version failed", "host_id", hostID, "error", err)
			writeBypassError(w, nethttp.StatusInternalServerError, "INTERNAL", "next version failed")
			return
		}

		params := repository.CreateBypassSnapshotParams{
			HostID:               hostID,
			Version:              nextVer,
			ConfigHash:           target.ConfigHash,
			WhitelistCIDRsJSON:   target.WhitelistCIDRsJSON,
			WhitelistDomainsJSON: target.WhitelistDomainsJSON,
			Source:               "rollback",
			CreatedBy:            actorIDPtr(r.Context()),
		}
		newSnap, err := h.store.CreateBypassSnapshot(r.Context(), params)
		if err != nil {
			if isUniqueViolation(err) {
				// UNIQUE(host_id, config_hash) 兜底：用 target hash + 后缀绕开
				// 这是已知 trade-off，确保 rollback 总能产生新行（plan WARN-4 trade-off 描述）
				params.ConfigHash = fmt.Sprintf("%s:rollback:%d", target.ConfigHash, nextVer)
				newSnap, err = h.store.CreateBypassSnapshot(r.Context(), params)
				if err != nil {
					h.logger.Error("rollback: create snapshot (with suffix) failed", "host_id", hostID, "error", err)
					writeBypassError(w, nethttp.StatusInternalServerError, "INTERNAL", "create rollback snapshot failed")
					return
				}
			} else {
				h.logger.Error("rollback: create snapshot failed", "host_id", hostID, "error", err)
				writeBypassError(w, nethttp.StatusInternalServerError, "INTERNAL", "create rollback snapshot failed")
				return
			}
		}

		// 6. dispatch ActionReloadHostBypass —— payload 是 new snapshot.ID，不是 target.ID
		var taskID string
		if h.actions != nil {
			task, qErr := h.actions.QueueHostAction(r.Context(), hostID, agentapi.ActionReloadHostBypass, newSnap.ID)
			if qErr != nil {
				h.logger.Error("rollback: queue host action failed", "host_id", hostID, "snapshot_id", newSnap.ID, "error", qErr)
			} else {
				taskID = task.ID
			}
		}

		// 7. 双轨审计：note 含 rollback_target_snapshot_id 前缀。
		note := "rollback_target_snapshot_id=" + target.ID
		var currentForAudit any
		if currentErr == nil {
			currentForAudit = current
		}
		writeBypassAuditLog(r.Context(), h.logger, h.store, h.events, r, "rollback", "snapshot", &newSnap.ID, currentForAudit, target, note)

		writeJSON(w, nethttp.StatusOK, rollbackResponse{
			SnapshotID:               newSnap.ID,
			RollbackTargetSnapshotID: target.ID,
			Version:                  newSnap.Version,
			AppliedStatus:            newSnap.AppliedStatus,
			TaskID:                   taskID,
			Message:                  fmt.Sprintf("回滚请求已下发，新版本 v%d 将复用 v%d 的配置", newSnap.Version, target.Version),
		})
	})
}

type effectiveResponse struct {
	PresetsActive            []repository.BypassPreset `json:"presets_active"`
	RulesActive              []repository.BypassRule   `json:"rules_active"`
	WhitelistCIDRsRendered   json.RawMessage           `json:"whitelist_cidrs_rendered"`
	WhitelistDomainsRendered json.RawMessage           `json:"whitelist_domains_rendered"`
}

// Effective 返回 host 当前所有启用 preset / rule + 渲染后的 whitelist 文件内容。
// GET /v1/admin/hosts/{hostID}/bypass/effective
func (h *AdminBypassSnapshotsHandler) Effective() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		hostID := strings.TrimSpace(r.PathValue("hostID"))
		if hostID == "" {
			writeBypassError(w, nethttp.StatusBadRequest, ErrCodeBypassInvalidRequest, "hostID is required")
			return
		}
		if _, err := h.store.GetHost(r.Context(), hostID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeBypassError(w, nethttp.StatusNotFound, ErrCodeBypassHostNotFound, "host not found")
				return
			}
			h.logger.Error("effective: get host failed", "host_id", hostID, "error", err)
			writeBypassError(w, nethttp.StatusInternalServerError, "INTERNAL", "get host failed")
			return
		}

		input, err := h.collectRenderInput(r.Context(), hostID)
		if err != nil {
			h.logger.Error("effective: collect render input failed", "host_id", hostID, "error", err)
			writeBypassError(w, nethttp.StatusInternalServerError, "INTERNAL", "collect render input failed")
			return
		}
		out, err := RenderBypassConfig(input, nil)
		if err != nil {
			h.logger.Error("effective: render failed", "host_id", hostID, "error", err)
			writeBypassError(w, nethttp.StatusInternalServerError, "INTERNAL", "render failed")
			return
		}
		presets := input.Presets
		if presets == nil {
			presets = []repository.BypassPreset{}
		}
		rules := input.Rules
		if rules == nil {
			rules = []repository.BypassRule{}
		}
		writeJSON(w, nethttp.StatusOK, effectiveResponse{
			PresetsActive:            presets,
			RulesActive:              rules,
			WhitelistCIDRsRendered:   out.CIDRsJSON,
			WhitelistDomainsRendered: out.DomainsJSON,
		})
	})
}

// isUniqueViolation 识别 pgx / pq 返回的 UNIQUE 约束冲突错误。
// 走字符串匹配兼容 wrapped error；apply / rollback 幂等路径依赖该判定。
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "unique") ||
		strings.Contains(msg, "duplicate") ||
		strings.Contains(msg, "host_bypass_snapshots_host_id_config_hash_key") ||
		strings.Contains(msg, "23505")
}
