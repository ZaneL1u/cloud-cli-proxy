export interface SSEMessage {
  topic: string;
  action: string;
  id?: string;
  payload?: unknown;
}

export type SSEHandler = (msg: SSEMessage) => void;

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
