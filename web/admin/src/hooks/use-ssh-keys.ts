import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/lib/api";
import { portalApiFetch } from "@/lib/portal-api";

export interface SSHKey {
  id: string;
  user_id: string;
  purpose: "inbound" | "outbound";
  label: string;
  public_key: string;
  private_key: string;
  key_type: string;
  fingerprint: string;
  created_at: string;
  source?: "managed" | "container";
  synced?: boolean;
}

// Admin hooks
export function useAdminSSHKeys(userId: string) {
  return useQuery({
    queryKey: ["admin", "ssh-keys", userId],
    queryFn: () => apiFetch<{ keys: SSHKey[] }>(`/users/${userId}/ssh-keys`),
    enabled: !!userId,
  });
}

export function useAdminCreateSSHKey() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      userId,
      purpose,
      label,
      keyType,
      publicKey,
      privateKey,
    }: {
      userId: string;
      purpose: "inbound" | "outbound";
      label: string;
      keyType?: "ed25519" | "rsa";
      publicKey?: string;
      privateKey?: string;
    }) =>
      apiFetch<{ key: SSHKey }>(`/users/${userId}/ssh-keys`, {
        method: "POST",
        body: JSON.stringify({
          purpose,
          label,
          key_type: keyType || "ed25519",
          public_key: publicKey,
          private_key: privateKey,
        }),
      }),
    onSuccess: async (_data, variables) => {
      await qc.refetchQueries({
        queryKey: ["admin", "ssh-keys", variables.userId],
      });
    },
  });
}

export function useAdminDeleteSSHKey() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ userId, keyId }: { userId: string; keyId: string }) =>
      apiFetch(`/users/${userId}/ssh-keys/${keyId}`, { method: "DELETE" }),
    onSuccess: async (_data, variables) => {
      await qc.refetchQueries({
        queryKey: ["admin", "ssh-keys", variables.userId],
      });
    },
  });
}

// User portal hooks
export function useMySSHKeys() {
  return useQuery({
    queryKey: ["portal", "ssh-keys"],
    queryFn: () => portalApiFetch<{ keys: SSHKey[] }>("/ssh-keys"),
  });
}

export function useMyCreateSSHKey() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      purpose,
      label,
      keyType,
      publicKey,
      privateKey,
    }: {
      purpose: "inbound" | "outbound";
      label: string;
      keyType?: "ed25519" | "rsa";
      publicKey?: string;
      privateKey?: string;
    }) =>
      portalApiFetch<{ key: SSHKey }>("/ssh-keys", {
        method: "POST",
        body: JSON.stringify({
          purpose,
          label,
          key_type: keyType || "ed25519",
          public_key: publicKey,
          private_key: privateKey,
        }),
      }),
    onSuccess: async () => {
      await qc.refetchQueries({ queryKey: ["portal", "ssh-keys"] });
    },
  });
}

export function useMyDeleteSSHKey() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (keyId: string) =>
      portalApiFetch(`/ssh-keys/${keyId}`, { method: "DELETE" }),
    onSuccess: async () => {
      await qc.refetchQueries({ queryKey: ["portal", "ssh-keys"] });
    },
  });
}
