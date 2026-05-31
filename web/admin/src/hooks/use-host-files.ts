import { useQuery } from "@tanstack/react-query";
import { apiFetch } from "@/lib/api";
import { isAbsolutePath } from "@/lib/path-utils";

export interface HostFileEntry {
  name: string;
  path: string;
  is_dir: boolean;
  size: number;
  mod_time: string;
}

export interface HostFilesResponse {
  entries: HostFileEntry[];
}

export function useHostFiles(path: string, hostId?: string) {
  const params = new URLSearchParams({ path });
  if (hostId) {
    params.set("host_id", hostId);
  }

  return useQuery<HostFilesResponse>({
    queryKey: ["host-files", hostId ?? "local", path],
    queryFn: () =>
      apiFetch<HostFilesResponse>(
        `/host-files?${params.toString()}`,
      ),
    enabled: path.length > 0 && isAbsolutePath(path),
    staleTime: 30_000,
  });
}
