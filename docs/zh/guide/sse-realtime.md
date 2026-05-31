# 实时推送（SSE）

管理后台和用户面板通过 SSE（Server-Sent Events）订阅事件，无需轮询。

## 端点

### 管理员

```bash
curl -N -H "Authorization: Bearer $TOKEN" \
  "http://YOUR_HOST:8080/v1/admin/sse?topics=hosts,tasks,image-status"
```

### 用户

```bash
curl -N -H "Authorization: Bearer $TOKEN" \
  "http://YOUR_HOST:8080/v1/user/sse?topics=hosts,tasks"
```

## 参数

| 参数 | 说明 |
|------|------|
| `topics` | 逗号分隔的主题列表：`hosts`、`tasks`、`image-status` |

## 事件格式

每条事件为一行 JSON：

```json
event: message
data: {"topic":"tasks","action":"update","id":"task-uuid"}
```

| 字段 | 说明 |
|------|------|
| `topic` | 事件主题 |
| `action` | `update`、`create` 或 `delete` |
| `id` | 关联资源 ID（可选） |
| `payload` | 额外载荷（可选） |

## 前端集成

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

## 实现

SSE 广播系统位于 `internal/broadcast/sse.go`，采用 topic-based pub/sub 模型。控制面在资源变更时向对应 topic 发布事件，每个连接维护独立订阅通道，断开时自动清理。

## 与轮询对比

| 方式 | 延迟 | 负载 | 适用 |
|------|------|------|------|
| SSE | 实时 | 低（事件驱动） | 主机状态、任务进度 |
| 轮询 | 秒级 | 高（固定频率） | 仪表盘统计 |

推荐管理后台列表页使用 SSE，仪表盘统计用按需刷新。
