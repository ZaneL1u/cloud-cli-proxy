import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { portalApiFetch } from "@/lib/portal-api";

export interface PortalHost {
  id: string;
  hostname: string;
  status: string;
  egress_ip: string;
  created_at: string;
}

export interface PortalEgressBinding {
  ip_address: string;
}

export interface ConnectionInfo {
  curl_command: string;
  ssh_command: string;
  ssh_port: number;
  vnc_url?: string;
}

export interface PortalHostDetail {
  id: string;
  hostname: string;
  status: string;
  timezone: string;
  created_at: string;
  updated_at: string;
  egress_bindings: PortalEgressBinding[];
  connection_info?: ConnectionInfo;
}

export function useMyHosts() {
  return useQuery({
    queryKey: ["portal", "hosts"],
    queryFn: () => portalApiFetch<{ hosts: PortalHost[] }>("/hosts"),
  });
}

export function useMyHostDetail(
  hostId: string,
  options?: {
    refetchInterval?:
      | number
      | false
      | ((query: { state: { data?: PortalHostDetail } }) => number | false);
  },
) {
  return useQuery({
    queryKey: ["portal", "hosts", hostId],
    queryFn: () => portalApiFetch<PortalHostDetail>(`/hosts/${hostId}`),
    enabled: !!hostId,
    refetchInterval: options?.refetchInterval,
  });
}

export function useRebuildHost() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (hostId: string) =>
      portalApiFetch<{ task_id: string }>(`/hosts/${hostId}/rebuild`, {
        method: "POST",
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["portal", "hosts"] });
    },
  });
}

export function useRestartMyHostVNC() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (hostId: string) =>
      portalApiFetch<{ status: string }>(`/hosts/${hostId}/vnc/restart`, {
        method: "POST",
      }),
    onSuccess: (_data, hostId) => {
      qc.invalidateQueries({ queryKey: ["portal", "hosts", hostId] });
    },
  });
}
