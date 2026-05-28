# 故障排查与恢复

## 常见故障

### 控制面无法启动

**症状：** `systemctl status cloud-cli-proxy-control-plane` 显示 `failed`，服务反复重启。

**排查步骤：**

1. 查看日志：`journalctl -u cloud-cli-proxy-control-plane --no-pager -n 50`
2. 检查 `DATABASE_URL`：`grep DATABASE_URL /etc/cloud-cli-proxy/env`
3. 检查 PostgreSQL：`systemctl status postgresql`
4. 检查端口占用：`ss -tlnp | grep 8080`
5. 检查 JWT 密钥：`grep ADMIN_JWT_SECRET /etc/cloud-cli-proxy/env`

**恢复：**
- 数据库连接失败 → 修复 `DATABASE_URL` 或启动 PostgreSQL
- 端口占用 → 停止占用进程或修改 `CONTROL_PLANE_ADDR`
- 权限不足 → 确认 `cloudproxy` 用户有数据库访问权限

### 用户无法登录

**症状：** bootstrap 脚本提示"连接控制面失败"或"认证失败"。

**排查步骤：**

1. 确认控制面运行：`curl -s http://127.0.0.1:8080/healthz`
2. 检查用户状态是否为 `active`
3. 检查用户是否已过期
4. 检查网络连通性（客户端到宿主机 8080 端口）

**恢复：**
- 控制面未运行 → `systemctl start cloud-cli-proxy-control-plane`
- 用户被禁用 → 通过 Admin API 重新启用
- 用户已过期 → 更新到期时间并改回 `active`

### 主机启动失败

**症状：** bootstrap 脚本显示"启动失败"，任务状态为 `failed`。

**排查步骤：**

1. 查看任务详情：通过 Admin API 获取 tasks
2. 检查 Docker daemon：`docker info`
3. 检查受管镜像：`docker images | grep managed-user`
4. 检查磁盘空间：`df -h /var/lib/docker`
5. 检查 host-agent 日志：`journalctl -u cloud-cli-proxy-host-agent --no-pager -n 50`

**恢复：**
- Docker 未运行 → `systemctl start docker`
- 受管镜像丢失 → `bash deploy/docker/managed-user/build-managed-image.sh`
- 磁盘空间不足 → `docker system prune`

### sing-box 代理隧道故障

**症状：** 使用 proxy 类型出口 IP 的主机无法上网或出口 IP 不匹配。

**排查步骤：**

1. 检查容器内 sing-box 进程：`docker exec {container} ps aux | grep sing-box`
2. 检查 sing-box 日志：`docker exec {container} cat /var/log/sing-box.log`
3. 检查 tun 设备：`nsenter --net=/var/run/netns/cloudproxy-{hostID} ip link show`
4. 检查路由表：`nsenter --net=/var/run/netns/cloudproxy-{hostID} ip route show`
5. 测试代理服务器可达性：确认代理服务器端口从宿主机可访问

**恢复：**
- sing-box 未运行 → 重建主机
- 代理服务器不可达 → 检查代理服务器状态和防火墙规则
- 配置错误 → 通过 Admin API 更新出口 IP 的 `proxy_config`，然后重建主机
- tun 设备丢失 → 重启 host-agent 并重建主机

### 代理测试失败

**症状：** 管理后台中出口 IP 测试显示失败。

**排查步骤：**

1. 检查测试结果详情（连通性 / 出口 IP 匹配 / DNS 泄漏具体哪一项失败）
2. 连通性失败 → 代理服务器可能宕机或端口不可达
3. 出口 IP 不匹配 → 代理服务器的出口 IP 与声明的 `ip_address` 不一致
4. DNS 泄漏 → 代理配置可能未正确覆盖 DNS 请求

**恢复：**
- 确认代理服务器运行正常
- 核实 `ip_address` 字段与代理服务器实际出口 IP 一致
- 检查 `proxy_config` 中的服务器地址和端口

### 到期扫描未触发

**症状：** 用户已过期但状态仍为 `active`。

**恢复：**
- 控制面未运行 → 启动控制面
- 需要立即生效 → `systemctl restart cloud-cli-proxy-control-plane`
- 手动处理 → 通过 Admin API 手动禁用用户并停止主机

### 数据库连接耗尽

**症状：** 日志出现 `too many connections` 错误。

**排查：**

```bash
sudo -u postgres psql -c "SELECT count(*) FROM pg_stat_activity WHERE datname='cloudproxy'"
sudo -u postgres psql -c "SHOW max_connections"
```

**恢复：**
- 临时释放 → 重启控制面
- 调整上限 → 编辑 `postgresql.conf` 增大 `max_connections`

### host-agent 无法启动

**排查步骤：**

1. 查看日志：`journalctl -u cloud-cli-proxy-host-agent --no-pager -n 50`
2. 运行依赖检查：`sudo bash deploy/scripts/host-preflight.sh`
3. 检查 Docker：`docker info`
4. 检查 sing-box 二进制：`which sing-box`
5. 检查 socket 目录权限：`ls -la /run/cloud-cli-proxy/`

### SSH 代理连接失败

**症状：** 用户通过 entry 短链接接入后，SSH 到 `:2222` 失败。

**排查步骤：**

1. 确认 SSH 代理端口监听：`ss -tlnp | grep 2222`
2. 确认容器运行中：`docker ps | grep {container_name}`
3. 确认容器内 SSH 服务启动：`docker exec {container} ss -tlnp | grep 22`
4. 检查控制面日志中的 SSH 代理错误

**恢复：**
- SSH 代理未监听 → 重启控制面
- 容器未运行 → 通过 Admin API 启动主机
- 容器内 SSH 未启动 → 重建主机

### KasmVNC 桌面无法访问

**症状：** 管理后台或用户面板中点击 VNC 链接无响应。

**排查步骤：**

1. 检查容器内 KasmVNC 进程：`docker exec {container} ps aux | grep kasmvnc`
2. 检查容器内 6901 端口：`docker exec {container} ss -tlnp | grep 6901`
3. 检查控制面 VNC 反向代理日志

**恢复：**
- KasmVNC 未启动 → 进入容器手动启动或重建主机
- 网络问题 → 确认控制面可以访问容器的管理网段 IP

## 灾难恢复

### 完全恢复流程

适用于宿主机完全不可用、需要在新机器上恢复的场景。

1. 准备新宿主机，确保满足前置条件
2. 运行自动化部署：
   ```bash
   git clone https://github.com/ZaneL1u/cloud-cli-proxy.git /opt/cloud-cli-proxy
   cd /opt/cloud-cli-proxy
   sudo bash deploy/scripts/deploy.sh
   ```
3. 恢复数据库：
   ```bash
   systemctl stop cloud-cli-proxy-control-plane
   pg_restore --clean -d cloudproxy /path/to/backup.dump
   systemctl start cloud-cli-proxy-control-plane
   ```
4. 重建受管镜像：`bash deploy/docker/managed-user/build-managed-image.sh`
5. 验证服务：`curl -s http://127.0.0.1:8080/healthz`

::: warning
恢复后所有用户容器需要重新创建和启动，但数据库中的用户、出口 IP 和绑定关系会保留。
:::

### 仅数据库恢复

```bash
systemctl stop cloud-cli-proxy-control-plane
sudo -u postgres psql -c "DROP DATABASE cloudproxy"
sudo -u postgres psql -c "CREATE DATABASE cloudproxy OWNER cloudproxy"
pg_restore -d cloudproxy /var/backups/cloud-cli-proxy/最新备份.dump
systemctl start cloud-cli-proxy-control-plane
```

## 日志查看

```bash
# 控制面日志（实时）
journalctl -u cloud-cli-proxy-control-plane -f

# host-agent 日志（实时）
journalctl -u cloud-cli-proxy-host-agent -f

# 最近 N 行
journalctl -u cloud-cli-proxy-control-plane --no-pager -n 100

# 按时间范围
journalctl -u cloud-cli-proxy-control-plane --since "2026-03-27 00:00:00" --until "2026-03-27 23:59:59"

# Docker Compose 部署查看日志
docker compose logs -f control-plane
docker compose logs -f admin
```

## 常见问题

### Q: 支持哪些代理协议？

A: 支持 6 种协议：SOCKS5、VMess、VLESS、Shadowsocks、Trojan 和 HTTP Proxy。配置格式遵循 sing-box outbound 规范。

### Q: 用户容器数据会丢失吗？

A: 重建主机会保留 home 目录数据。删除主机则会销毁所有数据。建议重要数据通过 Git 等方式备份。

### Q: 可以在 macOS / Windows 上开发吗？

A: 可以。使用 `make dev` 启动开发环境时，host-agent 以 `embedded` 模式运行，无需额外构建步骤。

### Q: 如何更新用户容器镜像？

A: 重新构建受管镜像（`make user-image`），然后对需要更新的主机执行重建操作。重建不影响 home 目录数据。
