import { useEffect, useRef } from "react";
import { subscribeSSE } from "@/lib/sse-manager";
import type { SSEMessage } from "@/lib/sse-manager";

export function useSSE(url: string, onMessage: (msg: SSEMessage) => void) {
  const onMessageRef = useRef(onMessage);
  onMessageRef.current = onMessage;

  useEffect(() => {
    if (!url) return;
    return subscribeSSE(url, (msg) => onMessageRef.current(msg));
  }, [url]);
}
