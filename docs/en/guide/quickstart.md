# Quick Start

## Docker Compose Deployment (Recommended)

### Prerequisites

- Linux host (Ubuntu 22.04+ / Debian 12+)
- Docker Engine 28+, Docker Compose v2
- At least one egress IP (proxy server)

### UI Preview

> The screenshots below are from repository `imgs/`, so you can see the admin and user experience before deploying.

#### Dashboard Overview

![Dashboard Overview](/imgs/1.png)

#### Host Management List

![Host Management List](/imgs/2.png)

#### Host Detail and Access Entry

![Host Detail and Access Entry](/imgs/4.png)

#### Lifecycle and Network Operations

![Lifecycle and Network Operations](/imgs/5.png)

#### Browser Remote Desktop (KasmVNC)

![Browser Remote Desktop](/imgs/3.png)

### Step 1: Clone

```bash
git clone https://github.com/ZaneL1u/cloud-cli-proxy.git
cd cloud-cli-proxy
```

### Step 2: Generate Environment Config

Run the setup script to auto-generate all passwords and secrets:

```bash
bash deploy/scripts/setup-env.sh
```

Choose a database mode:

- **Built-in Docker PostgreSQL (recommended)**: auto-generates DB password, managed by Docker Compose, zero config.
- **External PostgreSQL**: interactively enter your DB host, port, credentials, with SSL support.

Both options auto-generate an admin password (20 chars) and JWT secret (48 chars).

::: warning Important
The script displays the admin password once. Save it immediately!
:::

### Step 3: Start Services

Default recommendation: **prefer prebuilt images** (`latest`) for faster and consistent CI-aligned deployment.

```bash
# Built-in Docker PostgreSQL
docker compose pull --policy always
docker compose up -d

# External PostgreSQL (skip built-in DB)
docker compose pull --policy always control-plane admin
docker compose up -d control-plane admin
```

Optional local source build:

```bash
docker compose -f docker-compose.yml -f docker-compose.build.yaml --profile build-only build --no-cache
docker compose -f docker-compose.yml -f docker-compose.build.yaml up -d --force-recreate
```

### Step 4: Verify

```bash
curl http://127.0.0.1:8080/healthz
# {"status":"ok","checks":{"database":"ok","agent":"ok"}}
```

Service endpoints:
- **API**: `http://YOUR_HOST:8080`
- **Admin dashboard**: `http://YOUR_HOST:3000`
- **SSH proxy**: `YOUR_HOST:2222`

## Provisioning Users

Five steps: **login → add egress IP → create user → create host & bind → send connection command**.

### 1. Get Admin Token

Log in via the admin dashboard, or use the API:

```bash
TOKEN=$(curl -s -X POST http://YOUR_HOST:8080/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"your-admin-password"}' | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
```

### 2. Add Egress IP

Egress IPs use sing-box tun full-tunnel with `tunnel_type` set to `proxy`; configure the upstream in `proxy_config` (sing-box outbound).

Supports 6 protocols — SOCKS5, VMess, VLESS, Shadowsocks, Trojan, HTTP.

```bash
# Shadowsocks example
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
# SOCKS5 example
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

**Test egress IP connectivity:**

```bash
curl -s -X POST http://YOUR_HOST:8080/v1/admin/egress-ips/{ipID}/test \
  -H "Authorization: Bearer $TOKEN"
```

Tests connectivity, exit IP match, and DNS leak detection.

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

**Create host:**

```bash
curl -s -X POST http://YOUR_HOST:8080/v1/admin/hosts \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"user_id": "user-uuid"}'
```

**Bind egress IP:**

```bash
curl -s -X POST http://YOUR_HOST:8080/v1/admin/bindings \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"host_id": "host-uuid", "egress_ip_id": "egress-ip-uuid"}'
```

::: tip
A host requires at least one bound egress IP to start.
:::

### 5. Send to User

Copy the access command from the host detail page in the admin dashboard, or send this directly (replace `SHORT_ID` with the host's short ID):

```bash
curl -sSf http://YOUR_HOST/entry/SHORT_ID | bash
```

Or use the bootstrap method (requires username input):

```bash
curl -sSf http://YOUR_HOST:8080/v1/bootstrap/script | bash
```

## User Access

> Share this section directly with your users.

### Option 1: cloud-claude Local CLI (Recommended)

Install `cloud-claude` locally to transparently use Claude Code on your remote cloud host. Your current directory is automatically mapped into the container.

**Install:**

```bash
curl -fsSL https://raw.githubusercontent.com/ZaneL1u/cloud-cli-proxy/main/scripts/install.sh | bash
```

**First-time setup:**

```bash
cloud-claude init
```

You'll be prompted for:
- **Gateway URL**: Server address from your admin (e.g. `https://gw.example.com`)
- **Short ID**: Host short ID assigned by admin
- **Password**: Your login password (hidden input)

Config is saved to `~/.cloud-claude/config.yaml` and auto-loaded on subsequent runs.

You can also use flags:

```bash
cloud-claude init --gateway https://gw.example.com --short-id abc123 --password your-password
```

Or environment variables:

```bash
export CLOUD_CLAUDE_GATEWAY=https://gw.example.com
export CLOUD_CLAUDE_SHORT_ID=abc123
export CLOUD_CLAUDE_PASSWORD=your-password
cloud-claude init
```

**Daily usage:**

```bash
# Set alias for seamless experience
alias claude=cloud-claude

# Use exactly like local claude
claude

# All claude arguments are passed through
claude -p "refactor this function"
claude --model sonnet
```

When you run `cloud-claude`, it automatically:
1. Authenticates with the gateway
2. Waits for your container to be ready
3. Maps your **current directory** to `/workspace` via sshfs
4. Launches Claude Code in the container with `/workspace` as the working directory

Terminal resizing (SIGWINCH), Ctrl+C signals, and exit codes are all properly forwarded — the experience is identical to local `claude`.

**Error codes:**

| Exit Code | Meaning | Action |
|-----------|---------|--------|
| 1 | Auth failed | Check Short ID and password |
| 2 | Network error | Check gateway URL is reachable |
| 3 | Timeout | Container startup timeout, contact admin |
| 4 | Config error | Run `cloud-claude init` to reconfigure |

### Option 2: curl + SSH Access

Run the command your admin provided:

```bash
curl -sSf http://YOUR_HOST/entry/abc123 | bash
```

Enter your password and you'll be in your cloud host within seconds.

### Pre-installed Tools

| Tool | Description |
|------|-------------|
| **Claude Code** | AI coding assistant — just run `claude` in terminal |
| **KasmVNC + Chromium** | Browser remote desktop, accessible via admin or user panel |
| **Git** | Version control |
| **tmux** | Terminal multiplexer, sessions survive disconnects |
| **zsh** | Enhanced shell experience |
| **Node.js** | JavaScript runtime |

### Using Claude Code (via SSH)

Once inside your cloud host, just run:

```bash
claude
```

All Claude API requests are automatically routed through the admin-designated exit IP. No proxy configuration needed.

### Reconnecting

If your SSH connection drops, re-run the same `curl` command to reconnect. Your container keeps running.

### Rebuilding

If you need to reset your environment, click "Rebuild" in the user panel. This recreates the container but preserves your home directory data.

## Local Source Development (From Clone)

If you want to contribute or customize behavior, use this local development flow.

### 1. Install Dependencies

- Git
- Go `1.25.7+`
- Node.js `20+` (recommended with `corepack` enabled)
- pnpm `10+`
- Docker Engine + Docker Compose v2
- GNU Make

### 2. Clone Repository

```bash
git clone https://github.com/ZaneL1u/cloud-cli-proxy.git
cd cloud-cli-proxy
```

### 3. Initialize Dependencies and Environment

```bash
make setup
```

This installs frontend dependencies and auto-creates `.env` from `.env.example` when missing.

### 4. Start Local Database

```bash
make db
```

The default local PostgreSQL endpoint is `127.0.0.1:5433`.

### 5. Start Dev Mode

```bash
make dev
```

After startup:

- Admin frontend: `http://localhost:5173`
- Control Plane API: `http://127.0.0.1:8090`

### 6. Verify and Run Tests

```bash
curl http://127.0.0.1:8090/healthz
make test
```

### Common Commands

```bash
make dev-api   # backend only
make dev-web   # frontend only
make db-stop   # stop local database
make db-reset  # recreate local database
make help      # list all commands
```

## Next Steps

- [Deployment Guide](./deployment) — systemd native deployment
- [Configuration](./configuration) — Environment variables and networking
- [API Reference](../reference/api) — Full Admin API docs
- [FAQ & Recovery](../reference/faq) — Troubleshooting and disaster recovery
