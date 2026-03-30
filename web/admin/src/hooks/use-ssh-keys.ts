import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/lib/api";
import { portalApiFetch } from "@/lib/portal-api";

export interface SSHKeyData {
  public_key: string;
  private_key: string;
  key_type: string;
}

export function useAdminSSHKeys(userId: string) {
  return useQuery({
    queryKey: ["admin", "ssh-keys", userId],
    queryFn: () => apiFetch<SSHKeyData>(`/users/${userId}/ssh-keys`),
    enabled: !!userId,
  });
}

export function useAdminGenerateSSHKey() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      userId,
      keyType,
    }: {
      userId: string;
      keyType: "ed25519" | "rsa";
    }) =>
      apiFetch<SSHKeyData>(`/users/${userId}/ssh-keys/generate`, {
        method: "POST",
        body: JSON.stringify({ key_type: keyType }),
      }),
    onSuccess: (_data, variables) => {
      qc.invalidateQueries({
        queryKey: ["admin", "ssh-keys", variables.userId],
      });
    },
  });
}

export function useAdminSetSSHKey() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      userId,
      publicKey,
      privateKey,
    }: {
      userId: string;
      publicKey: string;
      privateKey: string;
    }) =>
      apiFetch<SSHKeyData>(`/users/${userId}/ssh-keys`, {
        method: "PUT",
        body: JSON.stringify({ public_key: publicKey, private_key: privateKey }),
      }),
    onSuccess: (_data, variables) => {
      qc.invalidateQueries({
        queryKey: ["admin", "ssh-keys", variables.userId],
      });
    },
  });
}

export function useAdminDeleteSSHKey() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (userId: string) =>
      apiFetch(`/users/${userId}/ssh-keys`, { method: "DELETE" }),
    onSuccess: (_data, userId) => {
      qc.invalidateQueries({ queryKey: ["admin", "ssh-keys", userId] });
    },
  });
}

export function useMySSHKeys() {
  return useQuery({
    queryKey: ["portal", "ssh-keys"],
    queryFn: () => portalApiFetch<SSHKeyData>("/ssh-keys"),
  });
}

export function useMyGenerateSSHKey() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (keyType: "ed25519" | "rsa") =>
      portalApiFetch<SSHKeyData>("/ssh-keys/generate", {
        method: "POST",
        body: JSON.stringify({ key_type: keyType }),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["portal", "ssh-keys"] });
    },
  });
}

export function useMySetSSHKey() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      publicKey,
      privateKey,
    }: {
      publicKey: string;
      privateKey: string;
    }) =>
      portalApiFetch<SSHKeyData>("/ssh-keys", {
        method: "PUT",
        body: JSON.stringify({ public_key: publicKey, private_key: privateKey }),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["portal", "ssh-keys"] });
    },
  });
}
