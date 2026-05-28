# Configuration

## Environment Variables

Create `/etc/cloud-cli-proxy/env` (systemd deployment) or `.env` (Docker Compose deployment).

Use `setup-env.sh` for interactive generation:

```bash
bash deploy/scripts/setup-env.sh
```

### Control Plane

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | — | PostgreSQL connection string, e.g. `postgres://user:pass@host:5432/db?sslmode=disable` |
| `CONTROL_PLANE_ADDR` | No | `:8080` | Control plane HTTP API listen address |
| `ADMIN_USERNAME` | No | `admin` | Admin username |
| `ADMIN_PASSWORD` | Yes | — | Admin password, used as seed on first startup |
| `ADMIN_JWT_SECRET` | Yes | — | JWT signing key (32+ chars), disables admin API if unset |
| `HOST_AGENT_MODE` | No | `socket` | Host-agent mode. `socket` = connect to standalone process via Unix socket, `embedded` = run inside control plane process |
| `HOST_AGENT_SOCKET` | No | `/run/cloud-cli-proxy/host-agent.sock` | Host-agent Unix socket path (socket mode only) |
| `DATA_DIR` | No | `/var/lib/cloud-cli-proxy` | Data directory for runtime files |
| `SSH_PROXY_ADDR` | No | `:2222` | SSH proxy listen address |
| `LOG_FORMAT` | No | `json` | Log format, `json` or `text` |
| `LOG_LEVEL` | No | `info` | Log level: `debug` / `info` / `warn` / `error` |

### Database (Docker Compose built-in PostgreSQL)

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DB_MODE` | No | `docker` | Database mode: `docker` = built-in, `external` = external |
| `POSTGRES_DB` | No | `cloudproxy` | Database name |
| `POSTGRES_USER` | No | `cloudproxy` | Database user |
| `POSTGRES_PASSWORD` | Yes (docker mode) | — | Database password |

### Admin Dashboard

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `ADMIN_PORT` | No | `3000` | Admin frontend port (maps to container port 80) |

### Docker Compose Port Mappings

| Variable | Default | Description |
|----------|---------|-------------|
| `SSH_PROXY_PORT` | `2222` | Host SSH proxy port |
| `ADMIN_PORT` | `3000` | Host admin dashboard port |

## Proxy Protocol Configuration

For `proxy`-type egress IPs, provide a `proxy_config` JSON field following the [sing-box outbound](https://sing-box.sagernet.org/configuration/outbound/) format.

### Supported Protocols

#### SOCKS5

```json
{
  "type": "socks",
  "server": "192.0.2.50",
  "server_port": 1080,
  "username": "user",
  "password": "pass"
}
```

#### Shadowsocks

```json
{
  "type": "shadowsocks",
  "server": "198.51.100.5",
  "server_port": 8388,
  "method": "aes-256-gcm",
  "password": "your-password"
}
```

Supported methods: `aes-128-gcm`, `aes-256-gcm`, `chacha20-ietf-poly1305`, etc.

#### VMess

```json
{
  "type": "vmess",
  "server": "203.0.113.20",
  "server_port": 443,
  "uuid": "your-uuid",
  "security": "auto",
  "alter_id": 0
}
```

#### Trojan

```json
{
  "type": "trojan",
  "server": "203.0.113.30",
  "server_port": 443,
  "password": "your-password",
  "tls": {
    "enabled": true,
    "server_name": "your-domain.com"
  }
}
```

#### HTTP

```json
{
  "type": "http",
  "server": "192.0.2.100",
  "server_port": 8080,
  "username": "user",
  "password": "pass"
}
```

### Admin Dashboard Configuration

The egress IP form provides a protocol selector with corresponding fields, plus a JSON editor mode.

## Firewall Rules

### Container Level

Host-agent uses nftables to set default-deny policy for each container namespace:

- **Proxy mode**: Only allows connections to the proxy server

Rules are managed automatically by host-agent.

### Host Level

Recommended host firewall:

```bash
nft add table inet filter
nft add chain inet filter input '{ type filter hook input priority 0; policy drop; }'
nft add rule inet filter input ct state established,related accept
nft add rule inet filter input iif lo accept
nft add rule inet filter input tcp dport 22 accept     # Host SSH
nft add rule inet filter input tcp dport 8080 accept   # API
nft add rule inet filter input tcp dport 3000 accept   # Admin dashboard
nft add rule inet filter input tcp dport 2222 accept   # SSH proxy
```

## Docker Images

All images are built via GitHub Actions for `linux/amd64` and `linux/arm64`.

| Image | Registry | Description |
|-------|----------|-------------|
| control-plane | `ghcr.io/zanel1u/cloud-cli-proxy/control-plane` | Control plane API server |
| admin | `ghcr.io/zanel1u/cloud-cli-proxy/admin` | Admin dashboard frontend (Nginx) |
| managed-user | `ghcr.io/zanel1u/cloud-cli-proxy/managed-user` | User container image |

**Tag convention:**

| Tag | Description |
|-----|-------------|
| `latest` | Latest build from main |
| `1.2.3` | Release version, corresponds to GitHub Release |
| `1.2` | Auto-follows latest patch |
| `1` | Auto-follows latest minor |
| `a1b2c3d` | Pinned to exact commit |

**Pin versions in production:**

```bash
docker pull ghcr.io/zanel1u/cloud-cli-proxy/control-plane:1.2.3
```

## User Container Pre-installed Software

The managed user image is based on Ubuntu 24.04 with:

| Software | Version | Description |
|----------|---------|-------------|
| OpenSSH Server | 10.2p1 | SSH access |
| Claude Code | Latest | AI coding assistant |
| KasmVNC | 1.4.0 | Remote desktop server |
| Chromium | Latest | Browser (with KasmVNC) |
| Fluxbox | — | Lightweight window manager |
| sing-box | 1.13.3 | Proxy mode tunnel client |
| Git, tmux, zsh | — | Common dev tools |
| Node.js | LTS | JavaScript runtime |
