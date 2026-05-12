export interface SSEMessage {
  topic: string;
  action: string;
  id?: string;
  payload?: unknown;
}

export type SSEHandler = (msg: SSEMessage) => void;

/**
 * buildSSEUrl —— 拼接带鉴权 token 的 SSE 订阅地址（CR-07）。
 *
 * 后端 /v1/admin/sse 与 /v1/user/sse 都已套 AuthMiddleware；EventSource 不能
 * 附 Authorization header，所以把 token 作为 query param 传入。无 token 时
 * 返回空串，让调用方的 useSSE 自动跳过订阅，避免一直 401 重连。
 *
 * @param path 形如 `/v1/admin/sse` 或 `/v1/user/sse`
 * @param topics 形如 `tasks` / `hosts,events` / `image-status`
 * @param token 当前会话 token；null/undefined/"" 一律返回空串
 */
export function buildSSEUrl(
  path: string,
  topics: string,
  token: string | null | undefined,
): string {
  if (!token) return "";
  const params = new URLSearchParams();
  if (topics) params.set("topics", topics);
  params.set("token", token);
  return `${window.location.origin}${path}?${params.toString()}`;
}

interface ConnectionState {
  es: EventSource | null;
  listeners: Set<SSEHandler>;
  reconnectAttempts: number;
  fallbackMode: boolean;
  reconnectTimer: number | null;
}

class SSEManager {
  private connections = new Map<string, ConnectionState>();

  subscribe(url: string, handler: SSEHandler): () => void {
    let state = this.connections.get(url);
    if (!state) {
      state = {
        es: null,
        listeners: new Set(),
        reconnectAttempts: 0,
        fallbackMode: false,
        reconnectTimer: null,
      };
      this.connections.set(url, state);
    }

    state.listeners.add(handler);

    if (!state.es && !state.fallbackMode) {
      this.connect(url, state);
    }

    return () => {
      state!.listeners.delete(handler);
      if (state!.listeners.size === 0) {
        this.disconnect(url);
      }
    };
  }

  private connect(url: string, state: ConnectionState) {
    const es = new EventSource(url, { withCredentials: true });
    state.es = es;

    es.onmessage = (event) => {
      state.reconnectAttempts = 0;
      try {
        const data: SSEMessage = JSON.parse(event.data);
        state.listeners.forEach((h) => h(data));
      } catch {
        // 忽略解析错误
      }
    };

    es.onerror = () => {
      es.close();
      state.es = null;
      state.reconnectAttempts++;

      // 连续失败 5 次后进入降级模式，不再重连
      if (state.reconnectAttempts >= 5) {
        state.fallbackMode = true;
        return;
      }

      const delay = Math.min(1000 * Math.pow(2, state.reconnectAttempts), 30000);
      state.reconnectTimer = window.setTimeout(() => {
        if (!state.fallbackMode) {
          this.connect(url, state);
        }
      }, delay);
    };
  }

  private disconnect(url: string) {
    const state = this.connections.get(url);
    if (!state) return;

    if (state.es) {
      state.es.close();
      state.es = null;
    }
    if (state.reconnectTimer) {
      clearTimeout(state.reconnectTimer);
      state.reconnectTimer = null;
    }
    this.connections.delete(url);
  }
}

const globalSSE = new SSEManager();

export function subscribeSSE(url: string, handler: SSEHandler): () => void {
  return globalSSE.subscribe(url, handler);
}
