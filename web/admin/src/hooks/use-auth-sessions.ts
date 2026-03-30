import { useSyncExternalStore } from "react";
import {
  getCurrentSession,
  getSessions,
  subscribeAuthChanges,
  type AuthSession,
} from "@/lib/auth";

interface AuthSessionsState {
  currentSession: AuthSession | null;
  sessions: AuthSession[];
}

let cachedSnapshot: AuthSessionsState = buildSnapshot();
let cachedKey = snapshotKey(cachedSnapshot);

function snapshotKey(state: AuthSessionsState): string {
  const sid = state.currentSession?.id ?? "";
  const ids = state.sessions.map((s) => s.id).join(",");
  return `${sid}|${ids}`;
}

function buildSnapshot(): AuthSessionsState {
  return {
    currentSession: getCurrentSession(),
    sessions: getSessions(),
  };
}

function getSnapshot(): AuthSessionsState {
  const fresh = buildSnapshot();
  const freshKey = snapshotKey(fresh);
  if (freshKey !== cachedKey) {
    cachedSnapshot = fresh;
    cachedKey = freshKey;
  }
  return cachedSnapshot;
}

function subscribe(callback: () => void): () => void {
  return subscribeAuthChanges(() => {
    cachedSnapshot = buildSnapshot();
    cachedKey = snapshotKey(cachedSnapshot);
    callback();
  });
}

export function useAuthSessions(): AuthSessionsState {
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
}
