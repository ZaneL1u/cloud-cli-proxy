# Deployment Guide

For system administrators with Linux experience, deploying on a single host from scratch.

## Prerequisites

- Ubuntu 22.04+ / Debian 12+ (or equivalent systemd-based distro)
- Root or sudo access
- Public IP (for bootstrap endpoint and user SSH access)
- At least one proxy config for an egress IP

## 1. Environment Setup

### Dependency Check

```bash
sudo bash deploy/scripts/host-preflight.sh
```

Checks: Docker Engine 28+, FUSE kernel module, nftables, nsenter, curl, ip, systemctl, Go 1.26+, PostgreSQL 18.x, Node.js 24 LTS (optional).

### Install Missing Dependencies

**Docker Engine:**

```bash
curl -fsSL https://get.docker.com | sh
systemctl enable --now docker
```

**nftables / nsenter / curl:**

```bash
apt-get install -y nftables util-linux curl
```

**FUSE kernel module:**

```bash
modprobe fuse
echo fuse >> /etc/modules-load.d/fuse.conf
```

Verify: `ls -la /dev/fuse` should show a character device with `crw-rw-rw-` permissions.

**Go 1.26:**

```bash
wget https://go.dev/dl/go1.26.1.linux-amd64.tar.gz
rm -rf /usr/local/go && tar -C /usr/local -xzf go1.26.1.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile.d/go.sh
source /etc/profile.d/go.sh
```

**PostgreSQL 18:**

```bash
apt-get install -y postgresql-18
systemctl enable --now postgresql
```

### FUSE & AppArmor Compatibility

sshfs inside containers requires FUSE. Docker's default AppArmor profile includes a `deny mount` rule. The system automatically adds `--security-opt apparmor=unconfined` when creating containers.

| Host OS | Impact | Handling |
|---------|--------|----------|
| Ubuntu 24.04 | Default AppArmor blocks FUSE mount | Handled automatically |
| Ubuntu 25.04+ | Additional fusermount3 profile may block | `aa-disable /usr/bin/fusermount3` |
| Debian 12+ | No AppArmor | No action needed |

Verify FUSE compatibility:

```bash
sudo bash scripts/verify-fuse-compat.sh
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

Verify connection:

```bash
psql "postgresql://cloudproxy:password@127.0.0.1:5432/cloudproxy" -c "SELECT 1"
```

## 3. Build

```bash
git clone https://github.com/ZaneL1u/cloud-cli-proxy.git /opt/cloud-cli-proxy
cd /opt/cloud-cli-proxy

go build -o /opt/cloud-cli-proxy/bin/control-plane ./cmd/control-plane
go build -o /opt/cloud-cli-proxy/bin/host-agent ./cmd/host-agent
bash deploy/docker/managed-user/build-managed-image.sh

# Frontend (optional)
cd web/admin && pnpm install && pnpm build && cd /opt/cloud-cli-proxy
```

## 4. Configuration

```bash
useradd --system --no-create-home --shell /usr/sbin/nologin cloudproxy
usermod -aG docker cloudproxy

mkdir -p /var/lib/cloud-cli-proxy /run/cloud-cli-proxy /etc/cloud-cli-proxy
chown cloudproxy:cloudproxy /var/lib/cloud-cli-proxy /run/cloud-cli-proxy /etc/cloud-cli-proxy
```

Create `/etc/cloud-cli-proxy/env`. See [Configuration](./configuration) for the full variable reference.

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

## Post-deploy Layout

```
/opt/cloud-cli-proxy/bin/     # binaries
/etc/cloud-cli-proxy/env      # environment variables (chmod 600)
/var/lib/cloud-cli-proxy/     # data directory
/run/cloud-cli-proxy/         # Unix socket
```
