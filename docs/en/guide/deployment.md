# Deployment Guide

> For system administrators with Linux experience, deploying on a single host from scratch.

## Prerequisites

- Ubuntu 22.04+ / Debian 12+ (or equivalent systemd-based Linux)
- Root or sudo access
- Public IP (for bootstrap endpoint and user SSH access)
- At least one WireGuard peer config for an exit IP (from VPN provider)

## 1. Environment Setup

### 1.1 Dependency Check

```bash
sudo bash deploy/scripts/host-preflight.sh
```

| Dependency | Min Version | Purpose |
|-----------|-------------|---------|
| Docker Engine | 28.x+ | Container runtime |
| WireGuard | kernel module | Full-tunnel egress |
| nftables (`nft`) | -- | Container firewall rules |
| `nsenter` | -- | Network namespace verification |
| `curl` | -- | Egress IP verification and health checks |
| `ip` | -- | Network configuration |
| `systemctl` | -- | Service management |
| Go | 1.26+ | Build control-plane and host-agent |
| PostgreSQL | 18.x | Persistent storage |
| Node.js | 24 LTS | Frontend build (optional) |

### 1.2 Install Missing Dependencies

**Docker Engine:**

```bash
curl -fsSL https://get.docker.com | sh
systemctl enable --now docker
```

**WireGuard:**

```bash
apt-get update && apt-get install -y wireguard-tools
modprobe wireguard
```

**Go 1.26:**

```bash
wget https://go.dev/dl/go1.26.1.linux-amd64.tar.gz
rm -rf /usr/local/go && tar -C /usr/local -xzf go1.26.1.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile.d/go.sh
source /etc/profile.d/go.sh
```

## 2. PostgreSQL Configuration

```bash
sudo -u postgres psql <<'SQL'
CREATE DATABASE cloudproxy;
CREATE USER cloudproxy WITH PASSWORD 'replace-with-strong-password';
GRANT ALL PRIVILEGES ON DATABASE cloudproxy TO cloudproxy;
ALTER DATABASE cloudproxy OWNER TO cloudproxy;
\c cloudproxy
GRANT ALL ON SCHEMA public TO cloudproxy;
SQL
```

## 3. Build

```bash
git clone https://github.com/ZaneL1u/cloud-cli-proxy.git /opt/cloud-cli-proxy
cd /opt/cloud-cli-proxy

# Control plane
go build -o /opt/cloud-cli-proxy/bin/control-plane ./cmd/control-plane

# Host agent
go build -o /opt/cloud-cli-proxy/bin/host-agent ./cmd/host-agent

# Managed user image
bash deploy/docker/managed-user/build-managed-image.sh
```

## 4. Configuration

### System User

```bash
useradd --system --no-create-home --shell /usr/sbin/nologin cloudproxy
usermod -aG docker cloudproxy
```

### Directories

```bash
mkdir -p /var/lib/cloud-cli-proxy /run/cloud-cli-proxy /etc/cloud-cli-proxy
chown cloudproxy:cloudproxy /var/lib/cloud-cli-proxy /run/cloud-cli-proxy /etc/cloud-cli-proxy
```

### Environment Variables

Create `/etc/cloud-cli-proxy/env`. See [Configuration](./configuration) for the full reference.

## 5. Install systemd Services

```bash
cp deploy/systemd/cloud-cli-proxy-control-plane.service /etc/systemd/system/
cp deploy/systemd/cloud-cli-proxy-host-agent.service /etc/systemd/system/

systemctl daemon-reload
systemctl enable --now cloud-cli-proxy-control-plane
systemctl enable --now cloud-cli-proxy-host-agent
```

Or use the automated deploy script:

```bash
sudo bash deploy/scripts/deploy.sh
```

## 6. Verify

```bash
systemctl status cloud-cli-proxy-control-plane
systemctl status cloud-cli-proxy-host-agent
curl -s http://127.0.0.1:8080/healthz
# {"status":"ok"}
```

## 7. Batch Refresh Worker Runners After Upgrade

Upgrading control-plane does not automatically rebuild user host containers. After upgrade, run a batch rebuild so both running and non-running hosts move to the latest runner image:

```bash
source /etc/cloud-cli-proxy/env
ADMIN_PASSWORD="$ADMIN_PASSWORD" \
bash deploy/scripts/rebuild-host-runners.sh \
  --base-url "http://127.0.0.1:8080" \
  --scope all
```

You can roll out by scope:

```bash
# Running containers only
bash deploy/scripts/rebuild-host-runners.sh --scope running

# Non-running containers only (including exited / not found)
bash deploy/scripts/rebuild-host-runners.sh --scope non-running
```

To make `deploy.sh` do this automatically, add:

```bash
REBUILD_ALL_HOSTS_AFTER_DEPLOY=1
```

Then run:

```bash
sudo bash deploy/scripts/deploy.sh
```

## Post-deploy Layout

```
/opt/cloud-cli-proxy/bin/     # control-plane, host-agent binaries
/etc/cloud-cli-proxy/env      # Environment variables (chmod 600)
/var/lib/cloud-cli-proxy/     # Data directory
/run/cloud-cli-proxy/         # Runtime Unix socket
```
