# API 参考

所有 API 基于 `http://YOUR_HOST:8080`。

## 认证

### 登录

```bash
curl -s -X POST http://YOUR_HOST:8080/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"你的密码"}'
```

响应：

```json
{"token":"eyJhbGci...","role":"admin"}
```

后续请求携带 `Authorization: Bearer $TOKEN`。

提取 Token：

```bash
TOKEN=$(curl -s -X POST http://YOUR_HOST:8080/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"你的密码"}' | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
```

::: tip
`/v1/admin/login` 兼容旧路径，功能与 `/v1/auth/login` 相同。
:::

## 健康检查

```bash
curl -s http://YOUR_HOST:8080/healthz
```

响应：

```json
{"status":"ok","checks":{"database":"ok","agent":"ok"}}
```

## 用户管理

### 创建用户

```bash
curl -s -X POST http://YOUR_HOST:8080/v1/admin/users \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "username": "newuser",
    "password": "初始密码至少8位",
    "expires_at": "2026-12-31T23:59:59Z"
  }'
```

用户名 3-50 字符，密码至少 8 字符。`expires_at` 可选，不设则永不过期。

| 状态码 | 说明 |
|--------|------|
| `201` | 创建成功 |
| `409` | 用户名已存在 |

### 查看用户列表

```bash
curl -s http://YOUR_HOST:8080/v1/admin/users \
  -H "Authorization: Bearer $TOKEN"
```

### 查看单个用户详情

```bash
curl -s http://YOUR_HOST:8080/v1/admin/users/{userID} \
  -H "Authorization: Bearer $TOKEN"
```

### 禁用 / 启用用户

```bash
curl -s -X PATCH http://YOUR_HOST:8080/v1/admin/users/{userID} \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"status":"disabled"}'
```

可选值：`active`、`disabled`。

### 删除用户

```bash
curl -s -X DELETE http://YOUR_HOST:8080/v1/admin/users/{userID} \
  -H "Authorization: Bearer $TOKEN"
```

返回 `204`。关联的主机会因外键级联删除。

### 密码轮换

```bash
curl -s -X POST http://YOUR_HOST:8080/v1/admin/users/{userID}/rotate-password \
  -H "Authorization: Bearer $TOKEN"
```

系统自动生成 20 字符随机强密码，返回新密码明文。

### 设置到期时间

```bash
curl -s -X PUT http://YOUR_HOST:8080/v1/admin/users/{userID}/expiry \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"expires_at":"2026-12-31T23:59:59Z"}'
```

清除到期时间（永不过期）：

```bash
curl -s -X PUT http://YOUR_HOST:8080/v1/admin/users/{userID}/expiry \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"expires_at":null}'
```

## SSH 密钥管理

### 管理员代管用户 SSH 密钥

```bash
# 查看用户的 SSH 公钥
curl -s http://YOUR_HOST:8080/v1/admin/users/{userID}/ssh-keys \
  -H "Authorization: Bearer $TOKEN"

# 添加 SSH 公钥
curl -s -X POST http://YOUR_HOST:8080/v1/admin/users/{userID}/ssh-keys \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"public_key":"ssh-ed25519 AAAA... user@host"}'

# 自动生成密钥对
curl -s -X POST http://YOUR_HOST:8080/v1/admin/users/{userID}/ssh-keys/generate \
  -H "Authorization: Bearer $TOKEN"

# 删除 SSH 公钥
curl -s -X DELETE http://YOUR_HOST:8080/v1/admin/users/{userID}/ssh-keys/{keyID} \
  -H "Authorization: Bearer $TOKEN"
```

### 用户自管 SSH 密钥

用户使用自己的 JWT Token 操作：

```bash
# 查看自己的 SSH 公钥
curl -s http://YOUR_HOST:8080/v1/user/ssh-keys \
  -H "Authorization: Bearer $TOKEN"

# 添加 SSH 公钥
curl -s -X POST http://YOUR_HOST:8080/v1/user/ssh-keys \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"public_key":"ssh-ed25519 AAAA... user@host"}'

# 自动生成密钥对
curl -s -X POST http://YOUR_HOST:8080/v1/user/ssh-keys/generate \
  -H "Authorization: Bearer $TOKEN"
```

## 出口 IP 管理

### 创建出口 IP

`proxy_config` 字段遵循 [sing-box outbound](https://sing-box.sagernet.org/configuration/outbound/) 格式。

```bash
# Shadowsocks
curl -s -X POST http://YOUR_HOST:8080/v1/admin/egress-ips \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "label": "jp-ss-01",
    "ip_address": "198.51.100.5",
    "tunnel_type": "proxy",
    "provider": "manual",
    "proxy_config": {
      "type": "shadowsocks",
      "server": "198.51.100.5",
      "server_port": 8388,
      "method": "aes-256-gcm",
      "password": "your-password"
    }
  }'

# VMess
curl -s -X POST http://YOUR_HOST:8080/v1/admin/egress-ips \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "label": "us-vmess-01",
    "ip_address": "203.0.113.20",
    "tunnel_type": "proxy",
    "provider": "manual",
    "proxy_config": {
      "type": "vmess",
      "server": "203.0.113.20",
      "server_port": 443,
      "uuid": "your-uuid",
      "security": "auto",
      "alter_id": 0
    }
  }'

# SOCKS5
curl -s -X POST http://YOUR_HOST:8080/v1/admin/egress-ips \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "label": "eu-socks-01",
    "ip_address": "192.0.2.50",
    "tunnel_type": "proxy",
    "provider": "manual",
    "proxy_config": {
      "type": "socks",
      "server": "192.0.2.50",
      "server_port": 1080,
      "username": "user",
      "password": "pass"
    }
  }'

# Trojan
curl -s -X POST http://YOUR_HOST:8080/v1/admin/egress-ips \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "label": "sg-trojan-01",
    "ip_address": "203.0.113.30",
    "tunnel_type": "proxy",
    "provider": "manual",
    "proxy_config": {
      "type": "trojan",
      "server": "203.0.113.30",
      "server_port": 443,
      "password": "your-password",
      "tls": {"enabled": true, "server_name": "your-domain.com"}
    }
  }'

# HTTP Proxy
curl -s -X POST http://YOUR_HOST:8080/v1/admin/egress-ips \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "label": "proxy-http-01",
    "ip_address": "192.0.2.100",
    "tunnel_type": "proxy",
    "provider": "manual",
    "proxy_config": {
      "type": "http",
      "server": "192.0.2.100",
      "server_port": 8080,
      "username": "user",
      "password": "pass"
    }
  }'
```

### 查看出口 IP 列表

```bash
curl -s http://YOUR_HOST:8080/v1/admin/egress-ips \
  -H "Authorization: Bearer $TOKEN"
```

### 查看出口 IP 详情

```bash
curl -s http://YOUR_HOST:8080/v1/admin/egress-ips/{ipID} \
  -H "Authorization: Bearer $TOKEN"
```

### 更新出口 IP

```bash
curl -s -X PUT http://YOUR_HOST:8080/v1/admin/egress-ips/{ipID} \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"label": "jp-ss-01-updated", "proxy_config": {"type": "shadowsocks", "server": "198.51.100.5", "server_port": 8388, "method": "aes-256-gcm", "password": "new-password"}}'
```

### 删除出口 IP

```bash
curl -s -X DELETE http://YOUR_HOST:8080/v1/admin/egress-ips/{ipID} \
  -H "Authorization: Bearer $TOKEN"
```

如果该 IP 仍被主机绑定，删除会被拒绝（`409`）。需先解除绑定再删除。

### 测试出口 IP

```bash
curl -s -X POST http://YOUR_HOST:8080/v1/admin/egress-ips/{ipID}/test \
  -H "Authorization: Bearer $TOKEN"
```

测试包含三项检测（30 秒超时）：

| 检测项 | 说明 |
|--------|------|
| 连通性 | 通过隧道访问外部 HTTP 端点 |
| 出口 IP 匹配 | 实际出口 IP 是否与声明的 `ip_address` 一致 |
| DNS 泄漏 | DNS 请求是否走隧道 |

## 主机管理

### 创建主机

```bash
curl -s -X POST http://YOUR_HOST:8080/v1/admin/hosts \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"user_id": "用户UUID"}'
```

### 查看主机列表

```bash
curl -s http://YOUR_HOST:8080/v1/admin/hosts \
  -H "Authorization: Bearer $TOKEN"
```

### 查看主机详情

```bash
curl -s http://YOUR_HOST:8080/v1/admin/hosts/{hostID} \
  -H "Authorization: Bearer $TOKEN"
```

返回主机信息，包括用户接入命令、短 ID 等。

### 启动 / 停止主机

```bash
# 启动
curl -s -X POST http://YOUR_HOST:8080/v1/admin/hosts/{hostID}/start \
  -H "Authorization: Bearer $TOKEN"

# 停止
curl -s -X POST http://YOUR_HOST:8080/v1/admin/hosts/{hostID}/stop \
  -H "Authorization: Bearer $TOKEN"
```

### 重建主机

```bash
curl -s -X POST http://YOUR_HOST:8080/v1/admin/hosts/{hostID}/rebuild \
  -H "Authorization: Bearer $TOKEN"
```

重建会销毁当前容器并基于受管镜像重新创建，用户 home volume 会保留。

### 重新生成用户 SSH 凭据

```bash
curl -s -X POST http://YOUR_HOST:8080/v1/admin/users/{userID}/credentials/regenerate \
  -H "Authorization: Bearer $TOKEN"
```

SSH 密码属于用户级 SSH 凭据，主机创建成功时会返回当前用户的 SSH 接入信息。丢失后请重新生成用户 SSH 凭据。

### 重启 VNC 服务（不中断主机）

```bash
curl -s -X POST http://YOUR_HOST:8080/v1/admin/hosts/{hostID}/vnc/restart \
  -H "Authorization: Bearer $TOKEN"
```

### 删除主机

```bash
curl -s -X DELETE http://YOUR_HOST:8080/v1/admin/hosts/{hostID} \
  -H "Authorization: Bearer $TOKEN"
```

### VNC 反向代理

管理后台通过以下路径反向代理到容器内的 KasmVNC：

```
/v1/admin/hosts/{hostID}/vnc/{path...}
```

## 主机与出口 IP 绑定

### 创建绑定

```bash
curl -s -X POST http://YOUR_HOST:8080/v1/admin/bindings \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"host_id":"主机UUID","egress_ip_id":"出口IP的UUID"}'
```

### 解除绑定

```bash
curl -s -X DELETE http://YOUR_HOST:8080/v1/admin/bindings/{bindingID} \
  -H "Authorization: Bearer $TOKEN"
```

## 用户面板 API

以下接口供普通用户使用（使用用户角色的 JWT Token）。

### 修改密码

```bash
curl -s -X POST http://YOUR_HOST:8080/v1/user/change-password \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"old_password":"旧密码","new_password":"新密码"}'
```

### 查看自己的主机

```bash
curl -s http://YOUR_HOST:8080/v1/user/hosts \
  -H "Authorization: Bearer $TOKEN"
```

### 查看主机详情

```bash
curl -s http://YOUR_HOST:8080/v1/user/hosts/{hostID} \
  -H "Authorization: Bearer $TOKEN"
```

### 重建自己的主机

```bash
curl -s -X POST http://YOUR_HOST:8080/v1/user/hosts/{hostID}/rebuild \
  -H "Authorization: Bearer $TOKEN"
```

### 重启自己的 VNC 服务

```bash
curl -s -X POST http://YOUR_HOST:8080/v1/user/hosts/{hostID}/vnc/restart \
  -H "Authorization: Bearer $TOKEN"
```

### 用户 VNC 访问

```
/v1/user/hosts/{hostID}/vnc/{path...}
```

## Bootstrap 接入

### 获取引导脚本

```bash
curl -sSf http://YOUR_HOST:8080/v1/bootstrap/script | bash
```

### 创建 Bootstrap 会话

```bash
curl -s -X POST http://YOUR_HOST:8080/v1/bootstrap/sessions \
  -H "Content-Type: application/json" \
  -d '{"username":"用户名","password":"密码"}'
```

认证成功后返回 `task_id` 和 `status_url`。

### 查询任务状态

```bash
curl -s http://YOUR_HOST:8080/v1/bootstrap/tasks/{taskID}
```

### 获取 SSH 接入参数

```bash
curl -s http://YOUR_HOST:8080/v1/bootstrap/tasks/{taskID}/handoff
```

任务成功后返回 SSH 连接所需的 `host`、`port`、`user` 参数。

## Entry 短链接接入

### 获取入口脚本

```bash
curl -sSf http://YOUR_HOST/entry/{shortId} | bash
```

### 认证

```bash
curl -s -X POST http://YOUR_HOST:8080/v1/entry/{shortId}/auth \
  -H "Content-Type: application/json" \
  -d '{"password":"密码"}'
```

成功后返回 SSH 连接参数（`ssh_user`、`ssh_pass`、`ssh_host`、`ssh_port`），用户通过 SSH 代理（`:2222`）接入。

## 任务列表

```bash
curl -s http://YOUR_HOST:8080/v1/admin/tasks \
  -H "Authorization: Bearer $TOKEN"
```

## 事件查看

```bash
# 最近事件
curl -s "http://YOUR_HOST:8080/v1/admin/events?limit=50" \
  -H "Authorization: Bearer $TOKEN"

# 按用户筛选
curl -s "http://YOUR_HOST:8080/v1/admin/events?user_id={userID}" \
  -H "Authorization: Bearer $TOKEN"

# 按主机筛选
curl -s "http://YOUR_HOST:8080/v1/admin/events?host_id={hostID}" \
  -H "Authorization: Bearer $TOKEN"

# 按时间范围
curl -s "http://YOUR_HOST:8080/v1/admin/events?since=2026-03-01T00:00:00Z&until=2026-03-31T23:59:59Z" \
  -H "Authorization: Bearer $TOKEN"
```

## 实时推送（SSE）

管理后台和用户面板可通过 SSE（Server-Sent Events）订阅实时事件，无需轮询。

### 管理员 SSE

```bash
curl -N -H "Authorization: Bearer $TOKEN" \
  "http://YOUR_HOST:8080/v1/admin/sse?topics=hosts,tasks,image-status"
```

### 用户 SSE

```bash
curl -N -H "Authorization: Bearer $TOKEN" \
  "http://YOUR_HOST:8080/v1/user/sse?topics=hosts,tasks"
```

### 查询参数

| 参数 | 说明 |
|------|------|
| `topics` | 逗号分隔的主题列表，可选值：`hosts`、`tasks`、`image-status` |

### 事件格式

每条事件为 JSON 行（newline-delimited JSON）：

```json
event: message
data: {"topic":"tasks","action":"update","id":"task-uuid"}
```

| 字段 | 说明 |
|------|------|
| `topic` | 事件主题：`hosts`、`tasks`、`image-status` |
| `action` | 操作类型：`update`、`create`、`delete` |
| `id` | 关联资源 ID（可选） |
| `payload` | 额外载荷（可选） |

前端收到事件后，建议使用 `queryClient.invalidateQueries()` 刷新对应缓存。

## 仪表盘

```bash
curl -s http://YOUR_HOST:8080/v1/admin/dashboard/stats \
  -H "Authorization: Bearer $TOKEN"
```

## 备份与恢复

### 数据库备份

```bash
sudo bash deploy/scripts/backup.sh
```

默认配置：备份目录 `/var/backups/cloud-cli-proxy`，保留 7 天。

通过环境变量自定义：

```bash
BACKUP_DIR=/data/backups RETENTION_DAYS=30 bash deploy/scripts/backup.sh
```

建议通过 cron 设置定期备份：

```bash
echo "0 2 * * * root /opt/cloud-cli-proxy/deploy/scripts/backup.sh" > /etc/cron.d/cloud-cli-proxy-backup
```

## 密钥轮换

### JWT Secret 轮换

1. 生成新密钥：`NEW_SECRET=$(head -c 48 /dev/urandom | base64 | tr -d '=+/' | head -c 48)`
2. 更新 `/etc/cloud-cli-proxy/env` 中的 `ADMIN_JWT_SECRET`
3. 重启控制面：`systemctl restart cloud-cli-proxy-control-plane`

轮换后所有已颁发的 JWT Token 立即失效。
