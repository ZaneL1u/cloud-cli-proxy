import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/lib/api";
import type { EgressIP } from "./use-egress-ips";

export interface HostWithUsername {
  id: string;
  user_id: string;
  status: string;
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
}

export interface HostDetail {
  host: {
    id: string;
    user_id: string;
    status: string;
    template_image_ref: string;
    home_volume_name: string;
    slot_key: string;
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
    mutationFn: (data: { user_id: string; egress_ip_id: string; timezone?: string }) =>
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
