import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/lib/api";
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
    host_mounts?: HostMount[];
    created_at: string;
    updated_at: string;
  };
  user: HostUser;
  bindings: HostBinding[];
  connection_info?: ConnectionInfo;
}

export function useHosts() {
  return useQuery({
    queryKey: ["hosts"],
    queryFn: () => apiFetch<{ hosts: HostWithUsername[] }>("/hosts"),
  });
}

export function useHostDetail(hostId: string) {
  return useQuery({
    queryKey: ["hosts", hostId],
    queryFn: () => apiFetch<HostDetail>(`/hosts/${hostId}`),
    enabled: !!hostId,
  });
}

export function useCreateHost() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: { user_id: string; egress_ip_id: string; timezone?: string; host_mounts?: HostMount[] }) =>
      apiFetch<{ host: HostWithUsername; task_id: string; short_id: string; entry_password: string }>("/hosts", {
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
    onSuccess: () => {
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

export function useRotateHostSSHPassword() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (hostId: string) =>
      apiFetch<{ new_password: string }>(`/hosts/${hostId}/rotate-ssh-password`, {
        method: "POST",
      }),
    onSuccess: (_data, hostId) => {
      qc.invalidateQueries({ queryKey: ["hosts"] });
      qc.invalidateQueries({ queryKey: ["hosts", hostId] });
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

export function useExportHostConfig() {
  return useMutation({
    mutationFn: async (hostId: string) => {
      const token = localStorage.getItem("token");
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
      const token = localStorage.getItem("token");
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
