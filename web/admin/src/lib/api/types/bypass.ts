// 与后端 internal/store/repository/models.go 的 Bypass* 结构对齐。
// 字段全部 snake_case，handler 不做大小写转换。

export type BypassRuleType =
  | "ip"
  | "cidr"
  | "domain"
  | "domain_suffix"
  | "domain_keyword";

export interface BypassPreset {
  id: string;
  slug: string; // loopback / lan / cn-dev …
  name: string; // 中文展示名
  description: string | null;
  is_system: boolean;
  is_forced: boolean; // loopback 为 true，强制启用
  rule_count: number;
  sample_rules: BypassRuleSample[]; // 用于 Popover 展示
  created_at: string;
  updated_at: string;
}

export interface BypassRuleSample {
  rule_type: BypassRuleType;
  value: string;
}

export interface BypassRule {
  id: string;
  host_id: string;
  rule_type: BypassRuleType;
  value: string;
  port: string | null; // 单端口 "80" 或范围 "80-443"
  is_risky: boolean;
  note: string | null;
  created_at: string;
  updated_at: string;
}

export interface BypassBinding {
  id: string;
  host_id: string;
  preset_id: string;
  preset?: BypassPreset;
  created_at: string;
}

export interface BypassRuleCreatePayload {
  rule_type: BypassRuleType;
  value: string;
  port?: string;
  note?: string;
  confirm_risky?: boolean;
}

export interface BypassRuleUpdatePayload {
  value?: string;
  port?: string;
  note?: string;
  confirm_risky?: boolean;
}

export interface BypassErrorPayload {
  code: string;
  message: string;
}

// ===== 46-04 扩展：快照 / 应用 / 回滚 / 生效快照 / 审计日志 =====
// 与后端 Plan 02 SUMMARY 的响应体 100% 对齐。

export interface BypassRenderedRuleSet {
  version: number;
  rules: unknown[];
}

export interface BypassPreviewResponse {
  config_hash: string;
  version_current: number;
  version_next: number;
  whitelist_cidrs_rendered: BypassRenderedRuleSet;
  whitelist_domains_rendered: BypassRenderedRuleSet;
  nft_diff: string;
  risky_count: number;
  summary: string;
}

export type BypassAppliedStatus =
  | "pending"
  | "applied"
  | "failed"
  | "rolled_back";

export interface BypassApplyResponse {
  snapshot_id: string;
  version: number;
  config_hash: string;
  applied_status: BypassAppliedStatus;
  task_id: string;
  message: string;
}

export interface BypassRollbackResponse {
  snapshot_id: string;
  task_id: string;
  message: string;
}

export interface BypassEffectiveResponse {
  presets_active: BypassPreset[];
  rules_active: BypassRule[];
  whitelist_cidrs_rendered: BypassRenderedRuleSet;
  whitelist_domains_rendered: BypassRenderedRuleSet;
}

export interface BypassAuditLogEntry {
  id: string;
  actor_id: string | null;
  actor_ip: string;
  action: string;
  target_kind: string;
  target_id: string | null;
  before: unknown;
  after: unknown;
  note: string;
  created_at: string;
}

export interface BypassAuditLogResponse {
  audit_log: BypassAuditLogEntry[];
  next_before: string;
}
