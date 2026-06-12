import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { apiFetch } from "@/lib/api";
import { getToken } from "@/lib/auth";
import { buildSSEUrl } from "@/lib/sse-manager";
import { useSSE } from "@/hooks/use-sse";
import type { EgressIP } from "./use-egress-ips";

export interface HostWithUsername {
  id: string;
  user_id: string;
  status: string;
  short_id: string;
  template_image_ref: string;
  home_volume_name: string;
  slot_key: string;
  timezone: string;
  hostname: string;
  username: string;
  egress_ip_label: string | null;
  egress_ip_address: string | null;
  egress_ip_detected_address: string | null;
  docker_status: string;
  created_at: string;
  updated_at: string;
}

export interface HostUser {
  id: string;
  username: string;
  status: string;
  created_at: string;
  updated_at: string;
}

export interface HostBinding {
  binding_id: string;
  egress_ip: EgressIP;
  created_at: string;
}

export interface ConnectionInfo {
  curl_command: string;
  ssh_command: string;
  ssh_port: number;
  vnc_url?: string;
}

export interface HostMount {
  source: string;
  target: string;
  read_only?: boolean;
}


export interface HostDetail {
  host: {
    id: string;
    user_id: string;
    status: string;
    short_id: string;
    template_image_ref: string;
    home_volume_name: string;
    slot_key: string;
    timezone: string;
    hostname: string;
    memory_limit_mb: number | null;
    cpu_limit: number | null;
    pids_limit: number | null;
    host_mounts?: HostMount[];
    
    created_at: string;
    updated_at: string;
  };
  user: HostUser;
  bindings: HostBinding[];
  connection_info?: ConnectionInfo;
}

export function useHosts() {
  const qc = useQueryClient();

  const query = useQuery({
    queryKey: ["hosts"],
    queryFn: () => apiFetch<{ hosts: HostWithUsername[] }>("/hosts"),
    refetchInterval: 30000,
  });

  useSSE(buildSSEUrl("/v1/admin/sse", "hosts", getToken()), (msg) => {
    if (msg.topic === "hosts") {
      qc.invalidateQueries({ queryKey: ["hosts"] });
      if (msg.id) {
        qc.invalidateQueries({ queryKey: ["hosts", msg.id] });
      }
    }
  });

  return query;
}

export function useHostDetail(hostId: string) {
  const qc = useQueryClient();

  const query = useQuery({
    queryKey: ["hosts", hostId],
    queryFn: () => apiFetch<HostDetail>(`/hosts/${hostId}`),
    enabled: !!hostId,
    refetchInterval: 30000,
  });

  useSSE(
    hostId ? buildSSEUrl("/v1/admin/sse", "hosts", getToken()) : "",
    (msg) => {
      if (msg.topic === "hosts" && msg.id === hostId) {
        qc.invalidateQueries({ queryKey: ["hosts", hostId] });
      }
    },
  );

  return query;
}

export function useCreateHost() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: { user_id: string; egress_ip_id: string; timezone?: string; pids_limit?: number | null; memory_limit_mb?: number | null; cpu_limit?: number | null; host_mounts?: HostMount[] }) =>
      apiFetch<{ host: HostWithUsername; task_id: string }>("/hosts", {
        method: "POST",
        body: JSON.stringify(data),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["hosts"] });
      qc.invalidateQueries({ queryKey: ["dashboard-stats"] });
      qc.invalidateQueries({ queryKey: ["tasks"] });
    },
  });
}

export function useUpdateHostMounts(hostId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (mounts: HostMount[]) =>
      apiFetch(`/hosts/${hostId}/mounts`, {
        method: "PUT",
        body: JSON.stringify({ mounts }),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["hosts", hostId] });
    },
  });
}

export function usePatchHostResources(hostId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: {
      pids_limit?: number | null;
      memory_limit_mb?: number | null;
      cpu_limit?: number | null;
    }) =>
      apiFetch(`/hosts/${hostId}/resources`, {
        method: "PATCH",
        body: JSON.stringify(data),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["hosts", hostId] });
      toast.success("资源限制已更新");
    },
    onError: (err: Error) => {
      toast.error(err.message || "更新资源限制失败");
    },
  });
}


export function useHostAction() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      hostId,
      action,
      body,
    }: {
      hostId: string;
      action: "start" | "stop" | "rebuild";
      body?: Record<string, unknown>;
    }) =>
      apiFetch(`/hosts/${hostId}/${action}`, {
        method: "POST",
        ...(body ? { body: JSON.stringify(body) } : {}),
      }),
    onSuccess: (_data, { hostId }) => {
      qc.invalidateQueries({ queryKey: ["hosts", hostId] });
      qc.invalidateQueries({ queryKey: ["hosts"] });
      qc.invalidateQueries({ queryKey: ["dashboard-stats"] });
      qc.invalidateQueries({ queryKey: ["tasks"] });
    },
  });
}

export function useDeleteHost() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ hostId, force }: { hostId: string; force?: boolean }) =>
      apiFetch(`/hosts/${hostId}${force ? "?force=true" : ""}`, {
        method: "DELETE",
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["hosts"] });
      qc.invalidateQueries({ queryKey: ["dashboard-stats"] });
    },
  });
}

export function useRestartHostVNC() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (hostId: string) =>
      apiFetch<{ status: string }>(`/hosts/${hostId}/vnc/restart`, {
        method: "POST",
      }),
    onSuccess: (_data, hostId) => {
      qc.invalidateQueries({ queryKey: ["hosts", hostId] });
    },
  });
}

export function useChangeRootPassword() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ hostId, password }: { hostId: string; password: string }) =>
      apiFetch(`/hosts/${hostId}/change-root-password`, {
        method: "POST",
        body: JSON.stringify({ password }),
      }),
    onSuccess: (_data, { hostId }) => {
      qc.invalidateQueries({ queryKey: ["hosts", hostId] });
    },
  });
}

export function useClaudeSettings(hostId: string, enabled = true) {
  return useQuery({
    queryKey: ["hosts", hostId, "claude-settings"],
    queryFn: () =>
      apiFetch<{ settings: Record<string, unknown> }>(
        `/hosts/${hostId}/claude/settings`,
      ),
    enabled: !!hostId && enabled,
  });
}

export function useUpdateClaudeSettings() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      hostId,
      settings,
    }: {
      hostId: string;
      settings: Record<string, unknown>;
    }) =>
      apiFetch(`/hosts/${hostId}/claude/settings`, {
        method: "PUT",
        body: JSON.stringify({ settings }),
      }),
    onSuccess: (_data, { hostId }) => {
      qc.invalidateQueries({
        queryKey: ["hosts", hostId, "claude-settings"],
      });
    },
  });
}

export interface ClaudeInfoResponse {
  claude_json: Record<string, unknown>;
  project_settings: Record<string, unknown>;
  uname: string;
  hostname: string;
  node: string;
}

export function useClaudeInfo(hostId: string, enabled = true) {
  return useQuery({
    queryKey: ["hosts", hostId, "claude-info"],
    queryFn: () =>
      apiFetch<ClaudeInfoResponse>(
        `/hosts/${hostId}/claude/info`,
      ),
    enabled: !!hostId && enabled,
  });
}

export interface ClaudeProcess {
  pid: number;
  work_dir: string;
  elapsed_seconds: number;
}

export interface ClaudeStatusResponse {
  running_instances: number;
  version: string;
  processes: ClaudeProcess[];
}

export function useClaudeStatus(hostId: string, enabled = true) {
  return useQuery({
    queryKey: ["hosts", hostId, "claude-status"],
    queryFn: () =>
      apiFetch<ClaudeStatusResponse>(
        `/hosts/${hostId}/claude/status`,
      ),
    enabled: !!hostId && enabled,
    refetchInterval: 30_000,
  });
}

export function useUpdateClaude() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (hostId: string) =>
      apiFetch<{ status: string; version: string }>(
        `/hosts/${hostId}/claude/update`,
        { method: "POST" },
      ),
    onSuccess: (_data, hostId) => {
      qc.invalidateQueries({
        queryKey: ["hosts", hostId, "claude-status"],
      });
    },
  });
}

export interface HostImageInfo {
  container_image_id: string;
  container_created: string;
  latest_image_id: string;
  latest_image_name: string;
  latest_created: string;
  update_available: boolean;
  container_available: boolean;
}

export function useHostImageInfo(hostId: string, enabled = true) {
  return useQuery({
    queryKey: ["hosts", hostId, "image-info"],
    queryFn: () =>
      apiFetch<HostImageInfo>(`/hosts/${hostId}/image-info`),
    enabled: !!hostId && enabled,
    refetchInterval: 60_000,
  });
}

export function useBindEgressIP() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: { host_id: string; egress_ip_id: string }) =>
      apiFetch("/bindings", { method: "POST", body: JSON.stringify(data) }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["hosts"] });
      qc.invalidateQueries({ queryKey: ["egress-ips"] });
    },
  });
}

export function useUnbindEgressIP() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (bindingId: string) =>
      apiFetch(`/bindings/${bindingId}`, { method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["hosts"] });
      qc.invalidateQueries({ queryKey: ["egress-ips"] });
    },
  });
}

export function useHostLogs(hostId: string, refetchInterval: number | false = 5000) {
  return useQuery({
    queryKey: ["hosts", hostId, "logs"],
    queryFn: () =>
      apiFetch<{ host_id: string; container_name: string; tail: number; logs: string; error?: string }>(
        `/hosts/${hostId}/logs?tail=200`,
      ),
    enabled: !!hostId,
    refetchInterval,
  });
}

export function useExportHostConfig() {
  return useMutation({
    mutationFn: async (hostId: string) => {
      const token = getToken();
      const resp = await fetch(`/v1/admin/hosts/${hostId}/config/export`, {
        headers: {
          ...(token ? { Authorization: `Bearer ${token}` } : {}),
        },
      });
      if (!resp.ok) {
        const data = await resp.json().catch(() => ({ error: "导出失败" }));
        throw new Error(data.error || "导出失败");
      }
      const blob = await resp.blob();
      const cd = resp.headers.get("Content-Disposition");
      let filename = `host-${hostId}-config.tar.gz`;
      if (cd) {
        const match = cd.match(/filename="?([^"]+)"?/);
        if (match) filename = match[1];
      }
      const url = window.URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = filename;
      document.body.appendChild(a);
      a.click();
      a.remove();
      window.URL.revokeObjectURL(url);
    },
  });
}

export function useImportHostConfig() {
  return useMutation({
    mutationFn: async ({ hostId, file }: { hostId: string; file: File }) => {
      const token = getToken();
      const formData = new FormData();
      formData.append("file", file);
      const resp = await fetch(`/v1/admin/hosts/${hostId}/config/import`, {
        method: "POST",
        headers: {
          ...(token ? { Authorization: `Bearer ${token}` } : {}),
        },
        body: formData,
      });
      if (!resp.ok) {
        const data = await resp.json().catch(() => ({ error: "导入失败" }));
        throw new Error(data.error || "导入失败");
      }
      return resp.json();
    },
    onSuccess: () => {
      toast.success("配置导入成功");
    },
  });
}
