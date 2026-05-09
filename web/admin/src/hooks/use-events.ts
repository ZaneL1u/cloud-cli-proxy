import { useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/lib/api";
import { useSSE } from "@/hooks/use-sse";

const eventTypeLabelMap: Record<string, string> = {
  "auth.success": "认证成功",
  "auth.failed": "认证失败",
  "user.expired": "用户过期",
  "host.stop.expired": "过期主机停止",
  "admin.user.created": "创建用户",
  "admin.user.updated": "修改用户",
  "admin.user.deleted": "删除用户",
  "admin.user.password_rotated": "轮换密码",
  "admin.binding.created": "创建绑定",
  "admin.binding.deleted": "删除绑定",
  "admin.host.action": "主机操作",
  "reconcile.host.drift": "主机漂移",
  "reconcile.task.stale": "陈旧任务",
};

export function eventTypeLabel(type: string) {
  return eventTypeLabelMap[type] ?? type;
}

export interface EventItem {
  id: string;
  type: string;
  level: string;
  message: string;
  user_id: string | null;
  host_id: string | null;
  task_id: string | null;
  metadata: Record<string, unknown>;
  created_at: string;
}

export interface EventsResponse {
  events: EventItem[];
  total: number;
  limit: number;
  offset: number;
}

export interface EventsParams {
  type?: string;
  userId?: string;
  hostId?: string;
  limit?: number;
  offset?: number;
}

export function useEvents(params: EventsParams = {}) {
  const qc = useQueryClient();
  const searchParams = new URLSearchParams();
  if (params.type) searchParams.set("type", params.type);
  if (params.userId) searchParams.set("user_id", params.userId);
  if (params.hostId) searchParams.set("host_id", params.hostId);
  if (params.limit) searchParams.set("limit", String(params.limit));
  if (params.offset) searchParams.set("offset", String(params.offset));

  const qs = searchParams.toString();
  const query = useQuery({
    queryKey: ["events", params],
    queryFn: () => apiFetch<EventsResponse>(`/events${qs ? `?${qs}` : ""}`),
    refetchInterval: 30000,
  });

  useSSE(`${window.location.origin}/v1/admin/sse?topics=events`, (msg) => {
    if (msg.topic === "events") {
      qc.invalidateQueries({ queryKey: ["events"] });
    }
  });

  return query;
}
