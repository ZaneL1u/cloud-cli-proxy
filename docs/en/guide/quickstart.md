# Quick Start

## Docker Compose Deployment

### Prerequisites

- Linux host (Ubuntu 22.04+ / Debian 12+)
- Docker Engine 28+, Docker Compose v2
- At least one egress IP (proxy server)

### 1. Clone

```bash
git clone https://github.com/ZaneL1u/cloud-cli-proxy.git
cd cloud-cli-proxy
```

### 2. Generate Environment Config

```bash
bash deploy/scripts/setup-env.sh
```

The script supports two database modes:

- **Built-in Docker PostgreSQL**: auto-generates database password, managed by Docker Compose.
- **External PostgreSQL**: interactively enter host, port, username, password, with SSL support.

Both modes auto-generate an admin password (20 chars) and JWT secret (48 chars).

::: warning Important
The script displays the admin password once. Save it immediately.
:::

### 3. Start Services

```bash
# Built-in PostgreSQL
docker compose pull
docker compose up -d

# External PostgreSQL (skip built-in database)
docker compose pull control-plane admin
docker compose up -d control-plane admin
```

If prebuilt images are unavailable, build from source:

```bash
docker compose -f docker-compose.yml -f docker-compose.build.yaml --profile build-only build --no-cache
docker compose -f docker-compose.yml -f docker-compose.build.yaml up -d --force-recreate
```

### 4. Verify

```bash
curl http://127.0.0.1:8080/healthz
# {"status":"ok","checks":{"database":"ok","agent":"ok"}}
```

Service endpoints:

- **API**: `http://YOUR_HOST:8080`
- **Admin dashboard (embedded)**: `http://YOUR_HOST:8080`
- **SSH proxy**: `YOUR_HOST:2222`

## Provisioning Users

Full workflow: **login → add egress IP → create user → create host & bind → send connection command**.

### 1. Get Admin Token

Log in via the admin dashboard or use the API:

```bash
TOKEN=$(curl -s -X POST http://YOUR_HOST:8080/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"your-admin-password"}' | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
```

### 2. Add Egress IP

Egress IPs use sing-box tun full-tunnel mode. Set `tunnel_type` to `proxy` and configure the upstream in `proxy_config`.

Supports 6 protocols: SOCKS5, VMess, VLESS, Shadowsocks, Trojan, HTTP.

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
      "password": "your-ss-password"
    }
  }'
```

```bash
# SOCKS5
curl -s -X POST http://YOUR_HOST:8080/v1/admin/egress-ips \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "label": "us-socks-01",
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
```

Test egress IP connectivity:

```bash
curl -s -X POST http://YOUR_HOST:8080/v1/admin/egress-ips/{ipID}/test \
  -H "Authorization: Bearer $TOKEN"
```

The test result covers connectivity, exit IP match, and DNS leak detection.

### 3. Create User

```bash
curl -s -X POST http://YOUR_HOST:8080/v1/admin/users \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "username": "zhangsan",
    "password": "initial-password-for-user",
    "expires_at": "2026-12-31T23:59:59Z"
  }'
```

### 4. Create Host & Bind Egress IP

```bash
# Create host
curl -s -X POST http://YOUR_HOST:8080/v1/admin/hosts \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"user_id": "user-uuid"}'

# Bind egress IP
curl -s -X POST http://YOUR_HOST:8080/v1/admin/bindings \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"host_id": "host-uuid", "egress_ip_id": "egress-ip-uuid"}'
```

::: tip
A host requires at least one bound egress IP to start.
:::

### 5. Send Connection Info

After the host is created and the task shows the container is ready, copy the connection command from the host detail page in the admin dashboard.

**Option A: curl + SSH**

Send this command to the user (replace `YOUR_HOST` and `SHORT_ID`):

```bash
curl -sSf http://YOUR_HOST/entry/SHORT_ID | bash
```

Or use the bootstrap flow (user enters their username):

```bash
curl -sSf http://YOUR_HOST:8080/v1/bootstrap/script | bash
```

**Option B: cloud-claude CLI (recommended)**

In addition to the `curl` command above, share these three values:

| Field | Description |
|-------|-------------|
| **Gateway URL** | Public HTTPS address of the control plane, e.g. `https://gw.example.com` |
| **Short ID** | Host short ID from the host detail page |
| **Password** | The user's password set in the admin dashboard |

After installing `cloud-claude` and running `init` with those values, the user runs `cloud-claude` from their project directory. The current directory is mounted at the same path inside the container via sshfs.

## User Access

### cloud-claude CLI (recommended)

#### Install

**Homebrew (macOS / Linux):**

```bash
brew tap ZaneL1u/tap
brew install cloud-claude
```

**One-liner:**

```bash
curl -fsSL https://raw.githubusercontent.com/ZaneL1u/cloud-cli-proxy/main/scripts/install.sh | bash
```

Also available from [Releases](https://github.com/ZaneL1u/cloud-cli-proxy/releases) or `go build ./cmd/cloud-claude`.

#### First-time Setup

```bash
cloud-claude init
```

Follow the prompts to enter gateway URL, Short ID, and password. Writes to `~/.cloud-claude/config.yaml`.

Or use flags or environment variables:

```bash
cloud-claude init --gateway https://gw.example.com --short-id abc123 --password your-password

export CLOUD_CLAUDE_GATEWAY=https://gw.example.com
export CLOUD_CLAUDE_SHORT_ID=abc123
export CLOUD_CLAUDE_PASSWORD=your-password
cloud-claude init
```

#### Daily Use

```bash
cd ~/your-project
alias claude=cloud-claude

cloud-claude
cloud-claude -p "refactor this function"
```

**Session management:**

```bash
cloud-claude                  # default: attach existing session
cloud-claude --new-session    # create a new isolated session
cloud-claude --take-over      # take over primary session, detach others
cloud-claude sessions         # list current sessions
```

**Mount modes:**

```bash
cloud-claude --mount-mode=auto         # default: HotSync preferred, falls back to SSHFS
cloud-claude --mount-mode=full         # HotSync + SSHFS dual-track
cloud-claude --mount-mode=sshfs-only   # SSHFS only
```

**Self-checks and troubleshooting:**

```bash
cloud-claude doctor                     # full five-domain check
cloud-claude doctor mount --fix         # mount check with auto-repair
cloud-claude explain MOUNT_SSHFS_DISCONNECTED  # error code lookup
cloud-claude env check                  # verify remote timezone, egress IP, FUSE, etc.
```

**Common config (`~/.cloud-claude/config.yaml`):**

- `proxy_commands` — commands to run on the host (default `["git"]`); set to `[]` to disable
- `hot_sync_max_file_mb` — per-file throttling threshold (default 50MB)
- `CLOUD_CLAUDE_NO_PROMOTION=1` — disable cold-file read promotion

### curl + SSH Access

Run the command your admin provided:

```bash
curl -sSf http://YOUR_HOST/entry/abc123 | bash
```

Enter your password, wait for the container to be ready, and you will be connected via SSH automatically.

### Pre-installed Tools

| Tool | Description |
|------|-------------|
| **Claude Code** | Run `claude` in terminal |
| **KasmVNC + Chromium** | Browser desktop via admin dashboard |
| **Git / tmux / zsh** | Common dev tools |
| **Node.js** | JavaScript runtime |

### Using Claude Code

Once inside the container:

```bash
claude
```

All Claude API requests are automatically routed through the egress IP. No proxy configuration needed.

### Reconnecting

If SSH disconnects, re-run the same `curl` command to reconnect. The container keeps running.

### Rebuilding

Click "Rebuild" in the admin dashboard to reset the environment. Home directory data is preserved.

## Source Development

For contributing or customizing, set up a local dev environment as follows.

### 1. Install Dependencies

- Go 1.25.7+
- Node.js 20+ (recommend enabling `corepack`)
- pnpm 10+
- Docker Engine + Docker Compose v2
- GNU Make

### 2. Set Up

```bash
git clone https://github.com/ZaneL1u/cloud-cli-proxy.git
cd cloud-cli-proxy

make setup    # install frontend deps, generate .env
make db       # start PostgreSQL
make dev      # backend + frontend hot reload
```

After startup:

- Admin frontend: `http://localhost:2568`
- Control Plane API: `http://127.0.0.1:8080`

### 3. Verify

```bash
curl http://127.0.0.1:8080/healthz
make test
```

### Common Commands

```bash
make dev-api   # backend only
make dev-web   # frontend only
make db-stop   # stop PostgreSQL
make db-reset  # recreate database
make help      # list all commands
```

## Next Steps

- [Deployment Guide](./deployment) — systemd native deployment
- [Configuration](./configuration) — environment variables and networking
- [Architecture](./architecture) — system design and project structure
- [API Reference](../reference/api) — full Admin API docs
- [FAQ & Troubleshooting](../reference/faq) — common issues and recovery
