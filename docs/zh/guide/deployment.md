# 部署指南

面向有 Linux 运维经验的技术人员，从零完成单宿主机部署。

## 前置条件

- Ubuntu 22.04+ / Debian 12+（或等效 systemd-based 发行版）
- Root 或 sudo 权限
- 公网 IP（用于 bootstrap 入口和用户 SSH 接入）
- 至少一个出口 IP 的代理配置

## 1. 环境准备

### 依赖检查

```bash
sudo bash deploy/scripts/host-preflight.sh
```

检查项：Docker Engine 28+、FUSE 内核模块、nftables、nsenter、curl、ip、systemctl、Go 1.26+、PostgreSQL 18.x、Node.js 24 LTS（可选）。

### 安装缺失依赖

**Docker Engine：**

```bash
curl -fsSL https://get.docker.com | sh
systemctl enable --now docker
```

**nftables / nsenter / curl：**

```bash
apt-get install -y nftables util-linux curl
```

**FUSE 内核模块：**

```bash
modprobe fuse
echo fuse >> /etc/modules-load.d/fuse.conf
```

验证：`ls -la /dev/fuse` 应输出 `crw-rw-rw-` 权限的字符设备。

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

### FUSE 与 AppArmor 兼容性

容器内的 sshfs 依赖 FUSE。Docker 默认 AppArmor profile 包含 `deny mount` 规则，系统已在创建容器时自动添加 `--security-opt apparmor=unconfined`。

| 宿主机 OS | 影响 | 处理 |
|-----------|------|------|
| Ubuntu 24.04 | 默认 AppArmor 阻断 FUSE mount | 系统自动处理 |
| Ubuntu 25.04+ | fusermount3 额外 profile 可能阻断 | `aa-disable /usr/bin/fusermount3` |
| Debian 12+ | 无 AppArmor | 无需处理 |

验证 FUSE 兼容性：

```bash
sudo bash scripts/verify-fuse-compat.sh
```

## 2. PostgreSQL 配置

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

验证连接：

```bash
psql "postgresql://cloudproxy:密码@127.0.0.1:5432/cloudproxy" -c "SELECT 1"
```

## 3. 构建

```bash
git clone https://github.com/ZaneL1u/cloud-cli-proxy.git /opt/cloud-cli-proxy
cd /opt/cloud-cli-proxy

go build -o /opt/cloud-cli-proxy/bin/control-plane ./cmd/control-plane
go build -o /opt/cloud-cli-proxy/bin/host-agent ./cmd/host-agent
bash deploy/docker/managed-user/build-managed-image.sh

# 前端（可选）
cd web/admin && pnpm install && pnpm build && cd /opt/cloud-cli-proxy
```

## 4. 配置

```bash
useradd --system --no-create-home --shell /usr/sbin/nologin cloudproxy
usermod -aG docker cloudproxy

mkdir -p /var/lib/cloud-cli-proxy /run/cloud-cli-proxy /etc/cloud-cli-proxy
chown cloudproxy:cloudproxy /var/lib/cloud-cli-proxy /run/cloud-cli-proxy /etc/cloud-cli-proxy
```

创建 `/etc/cloud-cli-proxy/env`，完整变量列表见 [配置参考](./configuration)。

## 5. 安装 systemd 服务

```bash
cp deploy/systemd/cloud-cli-proxy-control-plane.service /etc/systemd/system/
cp deploy/systemd/cloud-cli-proxy-host-agent.service /etc/systemd/system/

systemctl daemon-reload
systemctl enable --now cloud-cli-proxy-control-plane
systemctl enable --now cloud-cli-proxy-host-agent
```

也可以使用自动化脚本一键完成：

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

## 部署后文件布局

```
/opt/cloud-cli-proxy/bin/     # 二进制文件
/etc/cloud-cli-proxy/env      # 环境变量（chmod 600）
/var/lib/cloud-cli-proxy/     # 数据目录
/run/cloud-cli-proxy/         # Unix socket
```
