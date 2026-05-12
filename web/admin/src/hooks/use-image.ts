import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/lib/api";
import { getToken } from "@/lib/auth";
import { buildSSEUrl } from "@/lib/sse-manager";
import { useSSE } from "@/hooks/use-sse";

export interface ImageCacheStatus {
  image_name: string;
  image_version: string;
  local_digest: string;
  local_created: string;
  last_refresh_at: string;
  last_refresh_error?: string;
  refreshing: boolean;
}

export function useImageStatus(enabled = true) {
  const qc = useQueryClient();

  const query = useQuery({
    queryKey: ["image-status"],
    queryFn: () => apiFetch<ImageCacheStatus>("/image/status"),
    enabled,
    refetchInterval: (query) => {
      if (query.state.data?.refreshing) return 3000;
      return 30000;
    },
  });

  useSSE(buildSSEUrl("/v1/admin/sse", "image-status", getToken()), (msg) => {
    if (msg.topic === "image-status") {
      qc.invalidateQueries({ queryKey: ["image-status"] });
    }
  });

  return query;
}

export function useRefreshImage() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () =>
      apiFetch<{ status: string; message: string }>("/image/refresh", {
        method: "POST",
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["image-status"] });
    },
  });
}
