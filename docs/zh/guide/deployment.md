# 部署指南

> 本文档面向有 Linux 运维经验的技术人员，指导从零完成单宿主机部署。

## 前置条件

- Ubuntu 22.04+ / Debian 12+（或等效 systemd-based Linux 发行版）
- Root 或 sudo 权限
- 公网 IP（用于 bootstrap 入口和用户 SSH 接入）
- 至少一个出口 IP 的 WireGuard peer 配置（从 VPN 提供商获取）

## 1. 环境准备

### 1.1 依赖检查

运行内置的依赖检查脚本：

```bash
sudo bash deploy/scripts/host-preflight.sh
```

该脚本会检查以下依赖是否就绪：

| 依赖 | 最低版本 | 用途 |
|------|----------|------|
| Docker Engine | 28.x+ | 容器运行时 |
| WireGuard | 内核模块 | 全隧道出网 |
| nftables (`nft`) | -- | 容器防火墙规则 |
| `nsenter` | -- | 容器网络命名空间校验 |
| `curl` | -- | 出口 IP 校验和健康检查 |
| `ip` | -- | 网络配置 |
| `systemctl` | -- | 服务管理 |
| Go | 1.26+ | 构建控制面和 host-agent |
| PostgreSQL | 18.x | 持久化存储 |
| Node.js | 24 LTS | 前端构建（可选） |

### 1.2 安装缺失依赖

**Docker Engine：**

```bash
curl -fsSL https://get.docker.com | sh
systemctl enable --now docker
```

**WireGuard：**

```bash
apt-get update && apt-get install -y wireguard-tools
modprobe wireguard
```

**nftables / nsenter / curl：**

```bash
apt-get install -y nftables util-linux curl
```

**Go 1.26：**

```bash
wget https://go.dev/dl/go1.26.1.linux-amd64.tar.gz
rm -rf /usr/local/go && tar -C /usr/local -xzf go1.26.1.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile.d/go.sh
source /etc/profile.d/go.sh
```

**PostgreSQL 18：**

```bash
apt-get install -y postgresql-18
systemctl enable --now postgresql
```

## 2. PostgreSQL 配置

### 2.1 初始化数据库

```bash
sudo -u postgres psql <<'SQL'
CREATE DATABASE cloudproxy;
CREATE USER cloudproxy WITH PASSWORD '替换为强密码';
GRANT ALL PRIVILEGES ON DATABASE cloudproxy TO cloudproxy;
ALTER DATABASE cloudproxy OWNER TO cloudproxy;
\c cloudproxy
GRANT ALL ON SCHEMA public TO cloudproxy;
SQL
```

### 2.2 验证连接

```bash
psql "postgresql://cloudproxy:密码@127.0.0.1:5432/cloudproxy" -c "SELECT 1"
```

## 3. 构建

```bash
git clone https://github.com/ZaneL1u/cloud-cli-proxy.git /opt/cloud-cli-proxy
cd /opt/cloud-cli-proxy

# 控制面
go build -o /opt/cloud-cli-proxy/bin/control-plane ./cmd/control-plane

# host-agent
go build -o /opt/cloud-cli-proxy/bin/host-agent ./cmd/host-agent

# 受管用户镜像
bash deploy/docker/managed-user/build-managed-image.sh

# 管理后台前端（可选）
cd web/admin && npm install && npm run build && cd /opt/cloud-cli-proxy
```

## 4. 配置

### 4.1 创建系统用户

```bash
useradd --system --no-create-home --shell /usr/sbin/nologin cloudproxy
usermod -aG docker cloudproxy
```

### 4.2 创建必要目录

```bash
mkdir -p /var/lib/cloud-cli-proxy /run/cloud-cli-proxy /etc/cloud-cli-proxy
chown cloudproxy:cloudproxy /var/lib/cloud-cli-proxy /run/cloud-cli-proxy /etc/cloud-cli-proxy
```

### 4.3 环境变量

创建 `/etc/cloud-cli-proxy/env` 文件，完整变量列表见 [配置参考](./configuration)。

## 5. 安装 systemd 服务

```bash
cp deploy/systemd/cloud-cli-proxy-control-plane.service /etc/systemd/system/
cp deploy/systemd/cloud-cli-proxy-host-agent.service /etc/systemd/system/

systemctl daemon-reload
systemctl enable --now cloud-cli-proxy-control-plane
systemctl enable --now cloud-cli-proxy-host-agent
```

### 自动化部署

也可以使用自动化部署脚本一键完成上述步骤：

```bash
sudo bash deploy/scripts/deploy.sh
```

## 6. 验证

```bash
systemctl status cloud-cli-proxy-control-plane
systemctl status cloud-cli-proxy-host-agent
curl -s http://127.0.0.1:8080/healthz
# {"status":"ok"}
```

## 7. 升级后批量刷新 Worker Runner

控制面升级不会自动重建用户主机容器。升级后建议执行一次批量重建，让所有主机（含运行中与未运行）切到最新 runner 镜像：

```bash
source /etc/cloud-cli-proxy/env
ADMIN_PASSWORD="$ADMIN_PASSWORD" \
bash deploy/scripts/rebuild-host-runners.sh \
  --base-url "http://127.0.0.1:8080" \
  --scope all
```

也可按范围灰度：

```bash
# 仅运行中容器
bash deploy/scripts/rebuild-host-runners.sh --scope running

# 仅未运行容器（包含 exited / not found）
bash deploy/scripts/rebuild-host-runners.sh --scope non-running
```

如需在 `deploy.sh` 中自动执行，给环境文件增加：

```bash
REBUILD_ALL_HOSTS_AFTER_DEPLOY=1
```

然后再次执行：

```bash
sudo bash deploy/scripts/deploy.sh
```

## 部署后的文件布局

```
/opt/cloud-cli-proxy/bin/     # control-plane、host-agent 二进制
/etc/cloud-cli-proxy/env      # 环境变量（chmod 600）
/var/lib/cloud-cli-proxy/     # 数据目录
/run/cloud-cli-proxy/         # 运行时 Unix socket
```
