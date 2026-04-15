import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/lib/api";

export interface EgressIP {
  id: string;
  label: string;
  ip_address: string;
  provider: string;
  status: string;
  proxy_config: Record<string, unknown> | null;
  created_at: string;
  updated_at: string;
}

export interface TestResult {
  status: "passed" | "partial" | "failed" | "error";
  tested_at: string;
  message?: string;
  results: {
    connectivity: {
      status: "pass" | "fail" | "error";
      latency_ms?: number;
      error?: string;
    };
    egress_ip: {
      status: "pass" | "fail" | "error";
      ip?: string;
      sources?: Record<string, string>;
      error?: string;
    };
    dns_leak: {
      status: "pass" | "fail" | "error" | "skip";
      dns_servers_detected?: string[];
      local_dns_leaked?: boolean;
      error?: string;
    };
  };
}

export function useEgressIPs() {
  return useQuery({
    queryKey: ["egress-ips"],
    queryFn: () => apiFetch<{ egress_ips: EgressIP[] }>("/egress-ips"),
  });
}

export function useEgressIP(ipId: string) {
  return useQuery({
    queryKey: ["egress-ips", ipId],
    queryFn: () => apiFetch<{ egress_ip: EgressIP }>(`/egress-ips/${ipId}`),
    enabled: !!ipId,
  });
}

export function useCreateEgressIP() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: Partial<EgressIP>) =>
      apiFetch<{ egress_ip: EgressIP }>("/egress-ips", {
        method: "POST",
        body: JSON.stringify(data),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["egress-ips"] });
      qc.invalidateQueries({ queryKey: ["dashboard-stats"] });
    },
  });
}

export function useUpdateEgressIP() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ ipId, data }: { ipId: string; data: Partial<EgressIP> }) =>
      apiFetch<{ egress_ip: EgressIP }>(`/egress-ips/${ipId}`, {
        method: "PUT",
        body: JSON.stringify(data),
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["egress-ips"] }),
  });
}

export function useDeleteEgressIP() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (ipId: string) =>
      apiFetch(`/egress-ips/${ipId}`, { method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["egress-ips"] });
      qc.invalidateQueries({ queryKey: ["dashboard-stats"] });
    },
  });
}

export function useTestEgressIP() {
  return useMutation({
    mutationFn: (ipId: string) =>
      apiFetch<TestResult>(`/egress-ips/${ipId}/test`, { method: "POST" }),
  });
}
