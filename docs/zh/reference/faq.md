# 故障排查

## 常见故障

### 控制面无法启动

`systemctl status cloud-cli-proxy-control-plane` 显示 `failed`。

**排查：**

1. `journalctl -u cloud-cli-proxy-control-plane --no-pager -n 50`
2. `grep DATABASE_URL /etc/cloud-cli-proxy/env`
3. `systemctl status postgresql`
4. `ss -tlnp | grep 8080`

**原因与修复：** 数据库连不上则检查 PostgreSQL 状态并修正连接串；端口被占用则停掉冲突进程或改 `CONTROL_PLANE_ADDR`；权限问题则确认 `cloudproxy` 用户有数据库权限。

### 用户无法登录

bootstrap 脚本提示"认证失败"。

**排查：** 确认控制面运行（`curl healthz`）→ 检查用户状态是否为 `active` → 检查是否过期 → 检查客户端到宿主机 8080 端口的连通性。

**修复：** 控制面没启动就 `systemctl start`，用户被禁用就通过 Admin API 重新启用，过期就更新 `expires_at`。

### 主机启动失败

任务状态为 `failed`。

**排查：** 查任务详情（Admin API）→ `docker info` → `docker images | grep managed-user` → `df -h /var/lib/docker` → `journalctl -u cloud-cli-proxy-host-agent`。

**修复：** Docker 没跑就启动它，镜像丢了就重新构建，磁盘满了就 `docker system prune`。

### sing-box 隧道故障

主机无法上网或出口 IP 不匹配。

**排查：** `docker exec {container} ps aux | grep sing-box` → 查看 sing-box 日志 → `nsenter --net=/var/run/netns/cloudproxy-{hostID} ip link show` → 确认代理服务器从宿主机可达。

**修复：** sing-box 未运行则重建主机，代理服务器不可达则检查代理状态和防火墙，配置错误则更新 `proxy_config` 后重建。

### 出口 IP 测试失败

管理后台测试显示失败，检查具体是哪一项没过：连通性、出口 IP 匹配还是 DNS 泄漏。确认代理服务器正常、`ip_address` 与实际出口一致、`proxy_config` 地址和端口正确。

### 到期扫描未触发

用户已过期但状态仍为 `active`。重启控制面触发扫描，或通过 Admin API 手动禁用用户并停止主机。

### 数据库连接耗尽

日志出现 `too many connections`。

```bash
sudo -u postgres psql -c "SELECT count(*) FROM pg_stat_activity WHERE datname='cloudproxy'"
sudo -u postgres psql -c "SHOW max_connections"
```

临时释放重启控制面，长期调大 `max_connections`。

### Host Agent 无法启动

`journalctl -u cloud-cli-proxy-host-agent --no-pager -n 50` → `sudo bash deploy/scripts/host-preflight.sh` → `docker info` → `which sing-box` → `ls -la /run/cloud-cli-proxy/`。

### SSH 代理连接失败

用户 entry 短链接接入后 `:2222` 连不上。

**排查：** `ss -tlnp | grep 2222` → `docker ps | grep {container}` → `docker exec {container} ss -tlnp | grep 22`。

**修复：** SSH 代理没监听就重启控制面，容器没跑就启动主机，容器内 SSH 没起来就重建。

### KasmVNC 桌面无法访问

**排查：** `docker exec {container} ps aux | grep kasmvnc` → `docker exec {container} ss -tlnp | grep 6901`。

**修复：** KasmVNC 没启动就进容器手动拉起或重建主机。

## 灾难恢复

### 完全恢复

宿主机不可用时，在新机器上恢复：

1. 准备新宿主机，满足前置条件
2. 部署服务：
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
5. 验证：`curl -s http://127.0.0.1:8080/healthz`

::: warning
恢复后用户容器需要重新创建和启动，但数据库中的用户、出口 IP 和绑定关系会保留。
:::

### 仅恢复数据库

```bash
systemctl stop cloud-cli-proxy-control-plane
sudo -u postgres psql -c "DROP DATABASE cloudproxy"
sudo -u postgres psql -c "CREATE DATABASE cloudproxy OWNER cloudproxy"
pg_restore -d cloudproxy /var/backups/cloud-cli-proxy/latest.dump
systemctl start cloud-cli-proxy-control-plane
```

## 查看日志

```bash
# 控制面
journalctl -u cloud-cli-proxy-control-plane -f

# Host Agent
journalctl -u cloud-cli-proxy-host-agent -f

# 最近 100 行
journalctl -u cloud-cli-proxy-control-plane --no-pager -n 100

# Docker Compose
docker compose logs -f control-plane
```

## 常见问题

### 支持哪些代理协议？

SOCKS5、VMess、VLESS、Shadowsocks、Trojan、HTTP，共 6 种。配置格式遵循 sing-box outbound 规范。

### 用户容器数据会丢失吗？

重建主机保留 home 目录数据。删除主机会销毁所有数据。建议重要数据用 Git 备份。

### 可以在 macOS / Windows 上开发吗？

可以。`make dev` 启动开发环境时 host-agent 以 `embedded` 模式运行。

### 如何更新用户容器镜像？

重建受管镜像（`make user-image`），然后对需要更新的主机执行重建。重建不影响 home 目录数据。
