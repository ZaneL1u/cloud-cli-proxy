import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/lib/api";

export interface User {
  id: string;
  username: string;
  status: string;
  short_id?: string;
  expires_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface Host {
  id: string;
  user_id: string;
  status: string;
  short_id?: string;
  hostname?: string;
  template_image_ref: string;
  slot_key: string;
  created_at: string;
  updated_at: string;
}

export function useUsers() {
  return useQuery({
    queryKey: ["users"],
    queryFn: () => apiFetch<{ users: User[] }>("/users"),
  });
}

export function useUser(userId: string) {
  return useQuery({
    queryKey: ["users", userId],
    queryFn: () => apiFetch<{ user: User; hosts: Host[] }>(`/users/${userId}`),
  });
}

interface CreateUserResponse {
  user: User;
  password: string;
  short_id: string;
  entry_password: string;
  ssh_public_key: string;
  ssh_private_key: string;
  ssh_key_fingerprint: string;
}

export function useCreateUser() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: { username: string }) =>
      apiFetch<CreateUserResponse>("/users", {
        method: "POST",
        body: JSON.stringify(data),
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["users"] }),
  });
}

export interface RegenerateCredentialsResponse {
  entry_password: string;
  ssh_public_key: string;
  ssh_private_key: string;
  ssh_key_fingerprint: string;
}

export function useRegenerateCredentials() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (userId: string) =>
      apiFetch<RegenerateCredentialsResponse>(
        `/users/${userId}/credentials/regenerate`,
        { method: "POST" },
      ),
    onSuccess: (_data, userId) => {
      qc.invalidateQueries({ queryKey: ["users", userId] });
      qc.invalidateQueries({ queryKey: ["ssh-keys", userId] });
    },
  });
}

export function useUpdateUserStatus() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ userId, status }: { userId: string; status: string }) =>
      apiFetch<{ user: User }>(`/users/${userId}`, {
        method: "PATCH",
        body: JSON.stringify({ status }),
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["users"] }),
  });
}

export function useUpdateUserExpiry() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ userId, expiresAt }: { userId: string; expiresAt: string | null }) =>
      apiFetch<{ user: User }>(`/users/${userId}/expiry`, {
        method: "PUT",
        body: JSON.stringify({ expires_at: expiresAt }),
      }),
    onSuccess: (_data, variables) => {
      qc.invalidateQueries({ queryKey: ["users"] });
      qc.invalidateQueries({ queryKey: ["users", variables.userId] });
    },
  });
}

export function useDeleteUser() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (userId: string) =>
      apiFetch(`/users/${userId}`, { method: "DELETE" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["users"] }),
  });
}

export function useRotatePassword() {
  return useMutation({
    mutationFn: ({
      userId,
      newPassword,
    }: {
      userId: string;
      newPassword?: string;
    }) =>
      apiFetch<{ new_password: string }>(`/users/${userId}/rotate-password`, {
        method: "POST",
        body:
          newPassword !== undefined && newPassword !== ""
            ? JSON.stringify({ new_password: newPassword })
            : undefined,
      }),
  });
}
