import { apiFetch } from "@/lib/api";
import type {
  BypassPreset,
  BypassRule,
  BypassBinding,
  BypassRuleCreatePayload,
  BypassRuleUpdatePayload,
  BypassPreviewResponse,
  BypassApplyResponse,
  BypassRollbackResponse,
  BypassEffectiveResponse,
  BypassAuditLogResponse,
} from "./types/bypass";

// 系统预设列表（admin 全局可用预设）
export function listBypassPresets() {
  return apiFetch<{ presets: BypassPreset[] }>("/bypass/presets");
}

// host 维度规则集：后端路由是 GET /v1/admin/bypass/rules?host_id=...
// （host 维度通过 query 参数过滤，没有 /hosts/{hostId}/bypass/rules 路由）
export function listBypassRules(hostId: string) {
  return apiFetch<{ rules: BypassRule[] }>(
    `/bypass/rules?host_id=${encodeURIComponent(hostId)}`,
  );
}

export function createBypassRule(
  hostId: string,
  payload: BypassRuleCreatePayload,
) {
  return apiFetch<{ rule: BypassRule }>(`/hosts/${hostId}/bypass/rules`, {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export function updateBypassRule(
  hostId: string,
  ruleId: string,
  payload: BypassRuleUpdatePayload,
) {
  return apiFetch<{ rule: BypassRule }>(
    `/hosts/${hostId}/bypass/rules/${ruleId}`,
    {
      method: "PUT",
      body: JSON.stringify(payload),
    },
  );
}

export function deleteBypassRule(hostId: string, ruleId: string) {
  return apiFetch<void>(`/hosts/${hostId}/bypass/rules/${ruleId}`, {
    method: "DELETE",
  });
}

// host 与预设绑定
export function listBypassBindings(hostId: string) {
  return apiFetch<{ bindings: BypassBinding[] }>(
    `/hosts/${hostId}/bypass/bindings`,
  );
}

export function createBypassBinding(hostId: string, presetId: string) {
  return apiFetch<{ binding: BypassBinding }>(
    `/hosts/${hostId}/bypass/bindings`,
    {
      method: "POST",
      body: JSON.stringify({ preset_id: presetId }),
    },
  );
}

export function deleteBypassBinding(hostId: string, bindingId: string) {
  return apiFetch<void>(`/hosts/${hostId}/bypass/bindings/${bindingId}`, {
    method: "DELETE",
  });
}

// ===== 46-04 扩展：preview / apply / rollback / effective / auditLog =====

export function previewBypass(hostId: string) {
  return apiFetch<BypassPreviewResponse>(`/hosts/${hostId}/bypass/preview`, {
    method: "POST",
    body: "{}",
  });
}

export function applyBypass(hostId: string, note?: string) {
  return apiFetch<BypassApplyResponse>(`/hosts/${hostId}/bypass/apply`, {
    method: "POST",
    body: JSON.stringify({ note: note ?? "" }),
  });
}

export function rollbackBypass(hostId: string, targetSnapshotId: string) {
  return apiFetch<BypassRollbackResponse>(`/hosts/${hostId}/bypass/rollback`, {
    method: "POST",
    body: JSON.stringify({ target_snapshot_id: targetSnapshotId }),
  });
}

export function effectiveBypass(hostId: string) {
  return apiFetch<BypassEffectiveResponse>(`/hosts/${hostId}/bypass/effective`);
}

export function auditLogBypass(
  hostId: string,
  opts?: { limit?: number; before?: string },
) {
  const q = new URLSearchParams();
  if (opts?.limit) q.set("limit", String(opts.limit));
  if (opts?.before) q.set("before", opts.before);
  const qs = q.toString();
  return apiFetch<BypassAuditLogResponse>(
    `/hosts/${hostId}/bypass/audit-log${qs ? "?" + qs : ""}`,
  );
}
