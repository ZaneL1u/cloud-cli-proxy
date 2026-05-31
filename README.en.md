<div align="center">

<img src="docs/public/logo.svg" width="88" height="88" alt="Cloud CLI Proxy" />

# Cloud CLI Proxy

Give each user an isolated Docker container as a cloud dev environment on a single host. Containers come with Claude Code and common tools pre-installed. All outbound traffic goes through a sing-box full tunnel pinned to your designated exit IP.

[![CI](https://github.com/ZaneL1u/cloud-cli-proxy/actions/workflows/ci.yml/badge.svg)](https://github.com/ZaneL1u/cloud-cli-proxy/actions/workflows/ci.yml)
[![Images](https://github.com/ZaneL1u/cloud-cli-proxy/actions/workflows/build-images.yml/badge.svg)](https://github.com/ZaneL1u/cloud-cli-proxy/actions/workflows/build-images.yml)
[![Release](https://img.shields.io/github/v/release/ZaneL1u/cloud-cli-proxy)](https://github.com/ZaneL1u/cloud-cli-proxy/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

[中文](README.md) | [Docs](https://zanel1u.github.io/cloud-cli-proxy/en/)

</div>

---

## What is this?

Admins create users, assign containers, and bind egress IPs from the dashboard. Users get a `curl` command—paste it in the terminal, enter a password, wait for the container to boot, and SSH straight in. Each container is an isolated Ubuntu 24.04 environment with Claude Code, OpenSSH, and KasmVNC remote desktop already set up. All outbound traffic goes through a sing-box tun tunnel and exits from the designated IP, DNS and WebRTC included.

This exists to solve a few practical problems:

- Teams using Claude Code need a shared exit IP for API calls, without every person configuring their own proxy
- Global teams need specific regional exit IPs—bind one to a container and that's it
- Contractors or temporary staff need isolated dev machines that you can audit and tear down when done
- Compliance requires that all dev traffic exits from a known IP with no possible bypass

---

## Key Capabilities

### Networking & Security

- **Full-tunnel egress enforcement** — sing-box tun + Linux netns captures all container outbound traffic; nftables default-deny prevents DNS/WebRTC leaks
- **6 proxy protocols** — SOCKS5, VMess, VLESS, Shadowsocks, Trojan, HTTP for exit IPs
- **Bypass firewall** — Whitelist by domain, CIDR, or port with preset rule sets (loopback force-enabled, LAN optional), snapshot versioning with preview → apply → rollback, and full audit logging
- **Egress IP auto-correction** — Probes actual egress IP and auto-corrects the database when it diverges from configuration
- **Container hardening** — NET_ADMIN only (no SYS_ADMIN), NET_RAW dropped, IPv6 disabled at kernel level, PID limits, log rotation

### Environment Spoofing

Makes Claude Code inside the container appear to run on a real physical machine rather than a cloud environment. Defaults to mimicking a macOS or Windows desktop:

- **System fingerprint** — Overrides Node.js-readable CPU model (spoofed as AMD EPYC), MAC address, `/etc/machine-id`, and other hardware identifiers; intercepts `ioreg`, `system_profiler`, `sysctl` command output
- **Hostname spoofing** — Auto-generates `DESKTOP-XXXXXXX` or `LAPTOP-XXXXXXX` style hostnames
- **Container detection bypass** — Hides `/.dockerenv`, filters docker/containerd strings from cgroup, preventing container environment detection
- **Timezone & locale** — Configurable timezone and locale per container, defaulting to `America/Los_Angeles` / `en_US.UTF-8`
- **TLS fingerprint** — sing-box outbound connections enable uTLS with Chrome fingerprint by default, making TLS handshakes indistinguishable from regular browser traffic
- **Telemetry blocking** — DNS-level blocking inside the container prevents Claude Code from phoning home to `statsig.anthropic.com`, `sentry.io`, `cdn.growthbook.io`, and other telemetry endpoints

### Containers & Runtime

- **Isolated Docker containers** — One Ubuntu 24.04 container per user with configurable CPU, memory, and disk limits
- **Claude Code pre-installed** — Ready to use immediately; all API requests auto-routed through the designated exit IP
- **KasmVNC remote desktop** — Built-in Chromium browser with VNC proxy; access the desktop from the admin dashboard with one click
- **Persistent storage** — Claude state lives on named volumes; rebuilds preserve your work
- **Host bind mounts** — Mount host directories into containers for shared data access

### Access Experience

- **One-command onboarding** — Users run `curl | bash` to auto-authenticate, create their container, and SSH in. Zero configuration needed
- **cloud-claude local CLI** — Run remote Claude Code transparently from your local terminal; your working directory is sshfs-mounted at the **same path** inside the container. Three mount modes (Auto / Full / SSHFS-Only) with automatic oversized-file throttling
- **tmux multi-client sessions** — Multiple clients attach the same tmux session; disconnects never lose your workspace. `--new-session` for isolated sessions, `--take-over` to detach other clients
- **Network resilience** — Built-in Reconnector auto-recovers within 30s on disconnect; buffered input survives reconnections
- **doctor five-domain diagnostics** — `cloud-claude doctor [network|auth|ssh|mount|disk]` with `--fix` for automatic repair
- **Self-explanatory error codes** — `cloud-claude explain <CODE>` for detailed descriptions and remediation steps

### Admin Panel & Governance

- **React SPA dashboard** — At-a-glance view of active users, running hosts, available egress IPs, and recent events
- **Full user lifecycle** — Create, suspend, auto-expire (auto-stop containers and block login on expiry), rotate passwords
- **Full host lifecycle** — Create, start, stop, rebuild (preserve or wipe /workspace), delete
- **Egress IP management** — CRUD, connectivity testing (streaming output), bind to hosts
- **Event auditing** — All operations recorded to the events table with complete traceability
- **SSE real-time push** — Task progress, host status, and events via Server-Sent Events
- **User self-service portal** — Users can view their hosts, rebuild, restart VNC, and manage SSH keys

---

## Quick Start

```bash
git clone https://github.com/ZaneL1u/cloud-cli-proxy.git
cd cloud-cli-proxy

# Interactive password and secret generation
bash deploy/scripts/setup-env.sh

# Pull prebuilt images and start
docker compose pull
docker compose up -d

# Verify
curl http://127.0.0.1:8080/healthz
# {"status":"ok"}
```

After startup:

- Admin dashboard: `http://YOUR_HOST:3000`
- API: `http://YOUR_HOST:8080`
- SSH proxy: `YOUR_HOST:2222`

First-time setup: Log into the admin dashboard → add egress IPs → create users → create hosts → share the access command with users.

---

## Deployment

### Requirements

- Docker Engine 28.x+
- Docker Compose v2
- PostgreSQL 18.x (or use the built-in Docker PostgreSQL)

### Docker Compose (recommended)

```bash
bash deploy/scripts/setup-env.sh  # Interactive setup
docker compose pull               # Pull prebuilt images
docker compose up -d              # Start
```

`setup-env.sh` auto-generates JWT secrets, admin passwords, etc. Supports built-in Docker PostgreSQL (zero config) or an external database.

Local source build (fallback when prebuilt images are unavailable):

```bash
docker compose -f docker-compose.yml -f docker-compose.build.yaml --profile build-only build --no-cache
docker compose -f docker-compose.yml -f docker-compose.build.yaml up -d --force-recreate
```

### Bare-metal deployment

```bash
sudo bash deploy/scripts/deploy.sh
```

Creates a `cloudproxy` system user, builds Go binaries and container images, installs systemd units, and starts the services.

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `DATABASE_URL` | PostgreSQL connection string | Required |
| `ADMIN_USERNAME` | Admin username | `admin` |
| `ADMIN_PASSWORD` | Admin password (bcrypt) | Required |
| `ADMIN_JWT_SECRET` | JWT signing secret | Required |
| `ADMIN_PORT` | Admin dashboard port | `3000` |
| `SSH_PROXY_PORT` | SSH proxy port | `2222` |
| `LOG_FORMAT` | Log format `json` / `text` | `json` |
| `LOG_LEVEL` | Log level | `info` |

---

## Usage

### Admin Setup

1. **Add egress IPs** — Choose from 6 proxy protocols, test connectivity with one click
2. **Create users** — Set username, password, and expiration
3. **Create hosts** — Create containers for users and bind egress IPs
4. **Share access** — Copy the `curl` command from the host detail page for your users

### User Access (curl)

```bash
curl -sSf http://YOUR_HOST/entry/abc123 | bash
# Enter password → wait for container → auto SSH into cloud host
```

Claude Code is ready to use immediately:

```bash
claude
```

### cloud-claude CLI (recommended)

Install the CLI to run remote Claude Code from your local terminal. Your working directory is automatically mounted at the same path inside the container.

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

Or download from [Releases](https://github.com/ZaneL1u/cloud-cli-proxy/releases), or build from source:

```bash
go build -ldflags "-s -w" -trimpath -o cloud-claude ./cmd/cloud-claude
```

#### Initial Setup

The admin provides three pieces of info: **gateway URL**, **host Short ID**, and **password**.

```bash
cloud-claude init
# Interactive prompts → writes ~/.cloud-claude/config.yaml
```

Or use flags / environment variables:

```bash
cloud-claude init --gateway https://gw.example.com --short-id abc123 --password your-password
```

#### Daily Use

```bash
cd ~/your-project
alias claude=cloud-claude

cloud-claude            # default: attach existing tmux session
cloud-claude --new-session    # force a new isolated session
cloud-claude --take-over      # take over the primary session, detach others
cloud-claude sessions         # list current sessions
```

**Diagnostics:**

```bash
cloud-claude doctor                     # full five-domain check
cloud-claude doctor mount --fix         # mount check with auto-repair
cloud-claude explain MOUNT_SSHFS_DISCONNECTED  # error code details
cloud-claude env check                  # verify timezone, egress IP, FUSE, etc.
```

**Configuration:**

- `proxy_commands` — Commands to run locally (default: `git` only); set to `[]` to disable
- `hot_sync_max_file_mb` — Per-file throttling threshold (default 50MB)
- `CLOUD_CLAUDE_NO_PROMOTION=1` — Disable cold-file read promotion

### KasmVNC Remote Desktop

Access the container's browser desktop directly from the admin dashboard — no local GUI required.

---

## Architecture

```
                                                    ┌───────────────────────────────────┐
User ──curl──> Control Plane (:8080) ──Docker──>    │ User Container                    │
                    │                                │  SSH + Claude Code + VNC          │
               PostgreSQL                            │  sshfs ← same path as local cwd  │
                    │                                │  sing-box tun tunnel              │
              Admin SPA (:3000)                      │       ↓                           │
                    │                                │  Designated Exit IP               │
              SSH Proxy (:2222)                      └───────────────────────────────────┘
                    ↑                                           ↑
                    │                                           │
User ──cloud-claude──> auth + SSH + sshfs ──────────────────────┘
```

| Component | Description |
|-----------|-------------|
| **Control Plane** | Go API — authentication, user management, task orchestration, SSH proxy |
| **Host Agent** | Privileged agent — manages Docker containers, network namespaces, and tunnels |
| **User Container** | Ubuntu 24.04 — OpenSSH + Claude Code + sshfs + KasmVNC + Chromium |
| **cloud-claude** | Go CLI — transparent `claude` replacement; sshfs same-path mount; Auto/Full/SSHFS-Only mount modes, tmux multi-client sessions, auto-reconnect, doctor five-domain diagnostics, and error code explanations |
| **PostgreSQL** | Persists users, hosts, egress IPs, tasks, events, and audit logs |
| **Admin SPA** | React 19 + TypeScript + Vite + Tailwind CSS |

---

## Contributing

Bug reports and feature requests: open an [Issue](https://github.com/ZaneL1u/cloud-cli-proxy/issues).

Pull request process:

1. Fork the repo, create a feature branch from `main`
2. Make your changes, ensure `make test` passes
3. Open a PR describing what you changed and why

Local dev environment:

```bash
make setup    # Install dependencies
make db       # Start PostgreSQL
make dev      # Backend + frontend hot-reload (API :8090, frontend localhost:2568)
make test     # Run tests
```

See `make help` for all commands.

---

## Documentation

Full documentation on [GitHub Pages](https://zanel1u.github.io/cloud-cli-proxy/en/): quick start, deployment, configuration, architecture, API reference, and troubleshooting.

---

## License

[MIT](LICENSE)
