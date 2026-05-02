import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/lib/api";

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
  return useQuery({
    queryKey: ["image-status"],
    queryFn: () => apiFetch<ImageCacheStatus>("/image/status"),
    enabled,
    refetchInterval: (query) => {
      if (query.state.data?.refreshing) return 3000;
      return 30000;
    },
  });
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
