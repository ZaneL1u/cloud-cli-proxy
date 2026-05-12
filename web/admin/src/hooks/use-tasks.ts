import { useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/lib/api";
import { getToken } from "@/lib/auth";
import { buildSSEUrl } from "@/lib/sse-manager";
import { useSSE } from "@/hooks/use-sse";

export interface Task {
  task_id: string;
  host_id?: string | null;
  kind: string;
  status: string;
  requested_by?: string;
  error_code?: string;
  error_message?: string;
  last_error_summary?: string;
  progress_percent?: number;
  progress_message?: string;
  created_at?: string;
  updated_at: string;
}

export function useTasks() {
  const qc = useQueryClient();

  const query = useQuery({
    queryKey: ["tasks"],
    queryFn: () => apiFetch<{ tasks: Task[] }>("/tasks"),
    refetchInterval: 10000,
  });

  // CR-07：EventSource 不能附 Authorization header，必须通过 ?token=... 鉴权；
  // 后端 /v1/admin/sse 现已套 adminGuard，无 token 会被 401 拒绝。
  useSSE(buildSSEUrl("/v1/admin/sse", "tasks", getToken()), (msg) => {
    if (msg.topic === "tasks") {
      qc.invalidateQueries({ queryKey: ["tasks"] });
      if (msg.id) {
        qc.invalidateQueries({ queryKey: ["tasks", msg.id] });
      }
    }
  });

  return query;
}

export function useTaskPolling(taskId: string | null) {
  return useQuery({
    queryKey: ["tasks", taskId],
    queryFn: async () => {
      const data = await apiFetch<{ tasks: Task[] }>("/tasks");
      return data.tasks.find((t) => t.task_id === taskId) ?? null;
    },
    enabled: !!taskId,
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      if (status === "succeeded" || status === "failed" || status === "canceled")
        return false;
      return 2000;
    },
  });
}
