package http

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	nethttp "net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/zanel1u/cloud-cli-proxy/internal/agentapi"
	"github.com/zanel1u/cloud-cli-proxy/internal/network"
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

// actorIDOrAdmin 把 r.Context() 里的 actor user-id 取出来作为 task.requested_by 写入。
// 没有 actor（例如纯内部触发或测试） → fallback "admin"。Phase 47 Plan 01 引入：
// 把 Phase 46 旧 hack「把 snapshot ID 借用 requestedBy 形参传」拆开 —— 现在
// QueueHostAction 的第 4 参是真实 actor，第 5 参才是 snapshot ID。
func actorIDOrAdmin(ctx context.Context) string {
	if id := actorIDPtr(ctx); id != nil && *id != "" {
		return *id
	}
	return "admin"
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
	// WR-01：caller 层基于 rule.ID 做 dedup。RenderBypassConfig 自己有 set 去重，
	// 但 input.Rules 重复会让 totalRules 多计一次、summary 计数虚高。理论上
	// host scope 规则 + global scope 同 ID 不会冲突（不同表语义），但 binding.rule_id
	// 是任意 FK，留个统一 guard 不会有坏处。
	seen := map[string]struct{}{}
	for _, r := range allRules {
		if _, ok := seen[r.ID]; ok {
			continue
		}
		// host scope 自身规则：直接纳入（host 维度兜底）。
		if r.Scope == "host" && r.HostID != nil && *r.HostID == hostID {
			input.Rules = append(input.Rules, r)
			seen[r.ID] = struct{}{}
			continue
		}
		// global 规则：仅当被 binding 显式引用才纳入。
		if r.Scope == "global" {
			if _, ok := boundRuleIDs[r.ID]; ok {
				input.Rules = append(input.Rules, r)
				seen[r.ID] = struct{}{}
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
						task, qErr := h.actions.QueueHostAction(r.Context(), hostID, agentapi.ActionReloadHostBypass, actorIDOrAdmin(r.Context()), existing.ID)
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
		// WR-02：dispatch 失败时把错误写进 audit note，让运维能从 audit_log /
		// events 流里发现 pending 孤儿 snapshot。
		var taskID string
		var dispatchErr error
		if h.actions != nil {
			task, qErr := h.actions.QueueHostAction(r.Context(), hostID, agentapi.ActionReloadHostBypass, actorIDOrAdmin(r.Context()), snap.ID)
			if qErr != nil {
				h.logger.Error("apply: queue host action failed", "host_id", hostID, "snapshot_id", snap.ID, "error", qErr)
				dispatchErr = qErr
			} else {
				taskID = task.ID
			}
		}

		// 双轨审计：host_bypass_audit_log + events.RecordEvent("bypass.apply")。
		auditAction := "apply"
		auditNote := req.Note
		if dispatchErr != nil {
			auditAction = "apply_dispatch_failed"
			if auditNote != "" {
				auditNote += "; "
			}
			auditNote += "dispatch_error=" + dispatchErr.Error()
		}
		writeBypassAuditLog(r.Context(), h.logger, h.store, h.events, r, auditAction, "snapshot", &snap.ID, prev, snap, auditNote)

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
//     退一步用 target.ConfigHash + 8 字节 hex 后缀绕开（WR-05），保持整串为合法
//     hex 字符不污染 nft set 标识；audit note 标记 rollback_hash_suffixed=true
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
		rollbackHashSuffixed := false
		if err != nil {
			if isUniqueViolation(err) {
				// UNIQUE(host_id, config_hash) 兜底：用 target hash + 8 字节 hex 后缀绕开。
				// WR-05：原版用 ":rollback:<version>" 含冒号，破坏「ConfigHash 是纯 sha256
				// hex」的稳定语义（Phase 47 reload 可能用它做磁盘文件名 / nft set 标识）。
				// 改成 hex 后缀让整个串仍是合法 hex 字符；同时把这一决策记入 audit note。
				var rnd [8]byte
				if _, randErr := rand.Read(rnd[:]); randErr != nil {
					h.logger.Error("rollback: rand suffix failed", "host_id", hostID, "error", randErr)
					writeBypassError(w, nethttp.StatusInternalServerError, "INTERNAL", "create rollback snapshot failed")
					return
				}
				params.ConfigHash = target.ConfigHash + hex.EncodeToString(rnd[:])
				rollbackHashSuffixed = true
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
			task, qErr := h.actions.QueueHostAction(r.Context(), hostID, agentapi.ActionReloadHostBypass, actorIDOrAdmin(r.Context()), newSnap.ID)
			if qErr != nil {
				h.logger.Error("rollback: queue host action failed", "host_id", hostID, "snapshot_id", newSnap.ID, "error", qErr)
			} else {
				taskID = task.ID
			}
		}

		// 7. 双轨审计：note 含 rollback_target_snapshot_id 前缀。
		note := "rollback_target_snapshot_id=" + target.ID
		if rollbackHashSuffixed {
			// WR-05：标记本次 rollback 因 UNIQUE 兜底走了 hex 后缀路径，方便审计回查
			// 「为什么这条 snapshot 的 config_hash 比正常 hash 长 16 字符」。
			note += "; rollback_hash_suffixed=true"
		}
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

// verifyConsistencyHook 是 Consistency handler 的注入点，默认绑定到真实实现
// network.VerifyBypassConsistency。单测把它替换为 fake 闭包，避免依赖宿主机 nft。
var verifyConsistencyHook = network.VerifyBypassConsistency

// consistencyTimeout 限制单次对账 RPC 上限。3s 与 worker 健康检查上限对齐，
// 防止 admin endpoint 因 nft 命令卡死永远 hold（T-47-05 D DoS mitigation）。
const consistencyTimeout = 3 * time.Second

// Consistency 返回 GET /v1/admin/hosts/{hostID}/bypass/consistency 的 handler。
//
// 行为：
//   - hostID 缺失 → 400 BYPASS_INVALID_REQUEST
//   - host 不存在 → 404 BYPASS_HOST_NOT_FOUND
//   - 调 network.VerifyBypassConsistency（受 consistencyTimeout 包裹）：
//     · context.DeadlineExceeded → 504 BYPASS_CONSISTENCY_TIMEOUT
//     · 其它 error → 500 BYPASS_CONSISTENCY_ERROR
//     · OK=false（hash 不一致）→ 409 + ConsistencyResult JSON
//     · OK=true → 200 + ConsistencyResult JSON
//
// adminGuard 已在 router 层守好 admin-only，本 handler 不重复鉴权。
func (h *AdminBypassSnapshotsHandler) Consistency() nethttp.Handler {
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
			h.logger.Error("consistency: get host failed", "host_id", hostID, "error", err)
			writeBypassError(w, nethttp.StatusInternalServerError, "INTERNAL", "get host failed")
			return
		}

		checkCtx, cancel := context.WithTimeout(r.Context(), consistencyTimeout)
		defer cancel()

		res, err := verifyConsistencyHook(checkCtx, hostID)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(checkCtx.Err(), context.DeadlineExceeded) {
				writeBypassError(w, nethttp.StatusGatewayTimeout, "BYPASS_CONSISTENCY_TIMEOUT", "consistency check timeout")
				return
			}
			h.logger.Error("consistency: verify failed", "host_id", hostID, "error", err)
			writeBypassError(w, nethttp.StatusInternalServerError, "BYPASS_CONSISTENCY_ERROR", err.Error())
			return
		}
		if !res.OK {
			writeJSON(w, nethttp.StatusConflict, res)
			return
		}
		writeJSON(w, nethttp.StatusOK, res)
	})
}

// isUniqueViolation 识别 pgx / pq 返回的 UNIQUE 约束冲突错误。
// apply / rollback 幂等路径依赖该判定。
//
// WR-04：优先按 SQLSTATE 23505 严格匹配；若 wrap 链里能取到 pgconn.PgError
// 就用结构化判定，避免字符串匹配把其他 PG 错误（信息含 "unique" / "duplicate"
// 字样）误判为本约束冲突。字符串回退仅匹配 SQLSTATE 23505 + 具体约束名，
// 不再无差别匹配关键字。测试场景把 fake error 串构造为 "duplicate key
// value violates unique constraint host_bypass_snapshots_host_id_config_hash_key"
// 仍可命中。
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	msg := err.Error()
	return strings.Contains(msg, "23505") ||
		strings.Contains(msg, "host_bypass_snapshots_host_id_config_hash_key")
}
