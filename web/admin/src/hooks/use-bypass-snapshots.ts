import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  previewBypass,
  applyBypass,
  rollbackBypass,
  effectiveBypass,
  auditLogBypass,
} from "@/lib/api/bypass";

/**
 * 46-04 快照 / 应用 / 回滚 / 生效 / 审计日志 hook 集。
 *
 * 风格与 use-bypass-rules.ts 一致：
 * - mutation 成功后 invalidate 相关 query key
 * - query key 前缀统一 `['bypass', ...]`，staleTime 30s
 */

export function usePreviewBypass(hostId: string) {
  return useMutation({
    mutationFn: () => previewBypass(hostId),
  });
}

export function useApplyBypass(hostId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (note?: string) => applyBypass(hostId, note),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["bypass", "bindings", hostId] });
      qc.invalidateQueries({ queryKey: ["bypass", "rules", hostId] });
      qc.invalidateQueries({ queryKey: ["bypass", "effective", hostId] });
      qc.invalidateQueries({ queryKey: ["bypass", "audit-log", hostId] });
    },
  });
}

export function useRollbackBypass(hostId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (targetSnapshotId: string) =>
      rollbackBypass(hostId, targetSnapshotId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["bypass"] });
    },
  });
}

export function useEffectiveBypass(hostId: string, enabled = true) {
  return useQuery({
    queryKey: ["bypass", "effective", hostId],
    queryFn: () => effectiveBypass(hostId),
    enabled,
    staleTime: 30_000,
  });
}

export function useBypassAuditLog(hostId: string, before?: string) {
  return useQuery({
    queryKey: ["bypass", "audit-log", hostId, before ?? ""],
    queryFn: () => auditLogBypass(hostId, { limit: 20, before }),
    staleTime: 30_000,
  });
}
