# Configuration

## Commands



### doctor — Five-Domain Self-Check

Diagnoses issues across five domains. Can target a single domain or auto-repair.

| Domain | What it checks |
|--------|---------------|
| network | Container network connectivity, egress IP reachability |
| auth | Credential validity, token availability |
| ssh | SSH connection stability, key configuration |
| mount | sshfs mount state, FUSE compatibility |
| disk | Disk space, inode usage |

```bash
cloud-claude doctor                  # run all five checks
cloud-claude doctor network          # check network only
cloud-claude doctor mount --fix      # check mount and auto-repair
```

### env check — Remote Environment Check

```bash
cloud-claude env check
```

Outputs the remote container's timezone, locale, egress IP, FUSE status, toolchain versions, and more.

### explain — Error Code Lookup

```bash
cloud-claude explain MOUNT_SSHFS_DISCONNECTED
```

Outputs the error code's meaning, possible causes, and suggested fixes.



## Configuration Reference

### Environment Variables

Create `/etc/cloud-cli-proxy/env` (systemd deployment) or `.env` (Docker Compose deployment). Use `setup-env.sh` for interactive generation.

#### Control Plane

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | — | PostgreSQL connection string |
| `CONTROL_PLANE_ADDR` | No | `:8080` | HTTP API listen address |
| `ADMIN_USERNAME` | No | `admin` | Admin username |
| `ADMIN_PASSWORD` | Yes | — | Admin password (seed on first startup) |
| `ADMIN_JWT_SECRET` | Yes | — | JWT signing key (32+ characters) |
| `HOST_AGENT_MODE` | No | `socket` | `socket` standalone process / `embedded` inside control plane |
| `HOST_AGENT_SOCKET` | No | `/run/cloud-cli-proxy/host-agent.sock` | Agent socket path |
| `DATA_DIR` | No | `/var/lib/cloud-cli-proxy` | Data directory |
| `SSH_PROXY_ADDR` | No | `:2222` | SSH proxy listen address |
| `LOG_FORMAT` | No | `json` | Log format: `json` / `text` |
| `LOG_LEVEL` | No | `info` | Log level: `debug` / `info` / `warn` / `error` |

#### Database (Docker Compose built-in PostgreSQL)

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DB_MODE` | No | `docker` | `docker` built-in / `external` |
| `POSTGRES_DB` | No | `cloudproxy` | Database name |
| `POSTGRES_USER` | No | `cloudproxy` | Database user |
| `POSTGRES_PASSWORD` | Yes (docker) | — | Database password |

#### Admin Dashboard & Service Ports

| Variable | Default | Description |
|----------|---------|-------------|
| `ADMIN_PORT` | `3000` | Admin dashboard port |
| `SSH_PROXY_PORT` | `2222` | SSH proxy port |

### cloud-claude Config

`~/.cloud-claude/config.yaml`:

```yaml
gateway: https://gw.example.com
short_id: abc123
proxy_commands:
  - git
hot_sync_max_file_mb: 50
```

| Key | Description | Default |
|-----|-------------|---------|
| `gateway` | Control plane HTTPS address | — |
| `short_id` | Host short ID | — |
| `proxy_commands` | Commands to run on the host | `["git"]` |
| `hot_sync_max_file_mb` | Per-file throttling threshold | `50` |

Environment variables:

- `CLOUD_CLAUDE_GATEWAY` — same as `gateway`
- `CLOUD_CLAUDE_SHORT_ID` — same as `short_id`
- `CLOUD_CLAUDE_PASSWORD` — login password
- `CLOUD_CLAUDE_NO_PROMOTION=1` — disable cold-file promotion

## Proxy Protocols

For `proxy`-type egress IPs, fill in `proxy_config` following the [sing-box outbound](https://sing-box.sagernet.org/configuration/outbound/) format.

Supports six protocols: SOCKS5, Shadowsocks, VMess, VLESS, Trojan, HTTP.

### SOCKS5

```json
{
  "type": "socks",
  "server": "192.0.2.50",
  "server_port": 1080,
  "username": "user",
  "password": "pass"
}
```

### Shadowsocks

```json
{
  "type": "shadowsocks",
  "server": "198.51.100.5",
  "server_port": 8388,
  "method": "aes-256-gcm",
  "password": "your-password"
}
```

Supported methods: `aes-128-gcm`, `aes-256-gcm`, `chacha20-ietf-poly1305`.

### VMess

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

### Trojan

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

### HTTP

```json
{
  "type": "http",
  "server": "192.0.2.100",
  "server_port": 8080,
  "username": "user",
  "password": "pass"
}
```

The egress IP form in the admin dashboard provides a protocol selector with corresponding fields, plus a JSON editor mode.

## Firewall

### Container Level

The host agent uses nftables to set default-deny policies for each container's netns. Rules are managed automatically; no manual configuration is needed.

### Host Level

A basic host firewall is recommended:

```bash
nft add table inet filter
nft add chain inet filter input '{ type filter hook input priority 0; policy drop; }'
nft add rule inet filter input ct state established,related accept
nft add rule inet filter input iif lo accept
nft add rule inet filter input tcp dport 22 accept
nft add rule inet filter input tcp dport 8080 accept
nft add rule inet filter input tcp dport 3000 accept
nft add rule inet filter input tcp dport 2222 accept
```

## Docker Images

All images are built via GitHub Actions for `linux/amd64` and `linux/arm64`.

| Image | Registry |
|-------|----------|
| control-plane | `ghcr.io/zanel1u/cloud-cli-proxy/control-plane` |
| admin | `ghcr.io/zanel1u/cloud-cli-proxy/admin` |
| managed-user | `ghcr.io/zanel1u/cloud-cli-proxy/managed-user` |

Tag convention:

| Tag | Description |
|-----|-------------|
| `latest` | Latest from main |
| `1.2.3` | Release version |
| `1.2` | Follows latest patch |
| `1` | Follows latest minor |

Pin to a specific version in production.

### User Container Pre-installed Software

| Software | Description |
|----------|-------------|
| OpenSSH Server | SSH access |
| Claude Code | AI coding assistant |
| KasmVNC + Chromium | Remote desktop |
| sing-box | Tunnel client |
| Git, tmux, zsh | Dev tools |
| Node.js | JavaScript runtime |
