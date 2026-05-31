# Real-time Push (SSE)

The admin dashboard and user portal subscribe to events via SSE (Server-Sent Events) without polling.

## Endpoints

### Admin

```bash
curl -N -H "Authorization: Bearer $TOKEN" \
  "http://YOUR_HOST:8080/v1/admin/sse?topics=hosts,tasks,image-status"
```

### User

```bash
curl -N -H "Authorization: Bearer $TOKEN" \
  "http://YOUR_HOST:8080/v1/user/sse?topics=hosts,tasks"
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `topics` | Comma-separated topic list: `hosts`, `tasks`, `image-status` |

## Event Format

Each event is a line of JSON:

```json
event: message
data: {"topic":"tasks","action":"update","id":"task-uuid"}
```

| Field | Description |
|-------|-------------|
| `topic` | Event topic |
| `action` | `update`, `create`, or `delete` |
| `id` | Associated resource ID (optional) |
| `payload` | Additional payload (optional) |

## Frontend Integration

```typescript
const eventSource = new EventSource(
  '/v1/admin/sse?topics=hosts,tasks',
  { headers: { Authorization: `Bearer ${token}` } }
);

eventSource.onmessage = (event) => {
  const data = JSON.parse(event.data);
  queryClient.invalidateQueries({ queryKey: [data.topic] });
};
```

## Implementation

The SSE broadcast system lives in `internal/broadcast/sse.go` and uses a topic-based pub/sub model. The control plane publishes events to topics when resources change. Each connection maintains an independent subscription channel and is cleaned up automatically on disconnect.

## SSE vs Polling

| Approach | Latency | Load | Best For |
|----------|---------|------|----------|
| SSE | Real-time | Low (event-driven) | Host status, task progress |
| Polling | Seconds | High (fixed frequency) | Dashboard stats |

Use SSE for admin list pages and on-demand refresh for dashboard statistics.
