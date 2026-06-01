<div align="center">

<img src="docs/public/logo.svg" width="88" height="88" alt="Cloud CLI Proxy" />

# Cloud CLI Proxy

**A smarter Claude Code Wrapper. Containerize Claude Code so you look like a regular American developer — and Anthropic's risk system leaves you alone.**

[![CI](https://github.com/ZaneL1u/cloud-cli-proxy/actions/workflows/ci.yml/badge.svg)](https://github.com/ZaneL1u/cloud-cli-proxy/actions/workflows/ci.yml)
[![Images](https://github.com/ZaneL1u/cloud-cli-proxy/actions/workflows/build-images.yml/badge.svg)](https://github.com/ZaneL1u/cloud-cli-proxy/actions/workflows/build-images.yml)
[![Release](https://img.shields.io/github/v/release/ZaneL1u/cloud-cli-proxy)](https://github.com/ZaneL1u/cloud-cli-proxy/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

[中文](README.md) | [Docs](https://zanel1u.github.io/cloud-cli-proxy/en/)

</div>

---

## The Problem

Claude Code is great. The risk system is not. Wrong IP, VPS-looking environment fingerprints, telemetry ratting you out — your account's on borrowed time.

Cloud CLI Proxy does one thing: **it makes Claude Code think you're a regular person on a Windows laptop in Los Angeles.** Every layer is spoofed — IP, system fingerprint, TLS handshake. To Anthropic's servers, you're just another residential user in the US, not some cloud instance in a data center.

Deploy it on your own machine, SSH in, start coding. Your local project directory mounts into the container at the exact same path — using Claude Code feels identical to running it natively.

---

## How It Works

### Identity Spoofing

This isn't just slapping a proxy on and calling it done. Claude Code checks multiple signals to figure out where you are and what you're running on:

- **System fingerprint override** — CPU model shows as AMD EPYC. MAC address and machine-id are rewritten. Output from `ioreg`, `system_profiler`, `sysctl` is intercepted and replaced with what we want Claude Code to see
- **Windows-style hostnames** — Auto-generated `DESKTOP-XXXXXXX` or `LAPTOP-XXXXXXX`. Reads like someone's home PC
- **Container detection scrubbed** — `/.dockerenv` is gone. Docker/containerd strings filtered from cgroup. Standard container checks come up empty
- **Timezone & locale** — Defaults to Pacific Time + `en_US.UTF-8`. Configurable per container
- **TLS fingerprint** — Outbound connections use uTLS with a Chrome fingerprint. The TLS handshake looks exactly like someone browsing from Chrome
- **Telemetry blocked** — DNS-level blocking stops Claude Code from phoning home to `statsig.anthropic.com`, `sentry.io`, `cdn.growthbook.io`

### Strict IP Isolation

Every container gets its own exit IP. Every last packet — HTTP, DNS, WebRTC — goes through a sing-box tun tunnel and exits from that IP. nftables default-deny means nothing leaks.

Six proxy protocols supported: SOCKS5, VMess, VLESS, Shadowsocks, Trojan, HTTP.

**You need a residential IP for this to work.** Data center IP ranges are flagged everywhere. AT&T residential IPs are clean and pass risk checks reliably. This is what I use:

👉 [https://www.vircs.com/welcome?vcd=70685425](https://www.vircs.com/welcome?vcd=70685425)

### Code Mapping — Feels Like Local

This is what separates it from a regular VPS. The container mounts your local project directory via sshfs at the **exact same path**.

Working in `~/my-project` locally? It's `~/my-project` inside the container too. Claude Code sees paths that match your machine perfectly — just like running natively. Three mount modes: full sync, smart hot-sync, and plain mount. Disconnects auto-recover within 30 seconds, input buffer survives. tmux sessions keep your workspace alive across disconnects.

### Zero Friction

Admin creates the container from the dashboard. You get a `curl` command. Paste it, enter a password, wait a few seconds for the container to boot, and you're SSHed in. Claude Code is pre-installed. Just type `claude`.

---

## Quick Start

```bash
git clone https://github.com/ZaneL1u/cloud-cli-proxy.git
cd cloud-cli-proxy

bash deploy/scripts/setup-env.sh

docker compose pull
docker compose up -d

curl http://127.0.0.1:8080/healthz
# {"status":"ok"}
```

After startup:

- Admin dashboard: `http://YOUR_HOST:8080`
- API: `http://YOUR_HOST:8080`
- SSH proxy: `YOUR_HOST:2222`

First-time setup: log into admin dashboard → add egress IPs → create users → create hosts → share the access command.

---

## Deployment

### Requirements

- Docker Engine 28.x+
- Docker Compose v2
- PostgreSQL 18.x (or the built-in Docker PostgreSQL)

### Docker Compose (recommended)

```bash
bash deploy/scripts/setup-env.sh  # Interactive setup
docker compose pull               # Pull prebuilt images
docker compose up -d              # Start
```

### Bare-metal

```bash
sudo bash deploy/scripts/deploy.sh
```

Creates a `cloudproxy` system user, builds binaries and images, installs systemd units, starts services.

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `DATABASE_URL` | PostgreSQL connection string | Required |
| `ADMIN_USERNAME` | Admin username | `admin` |
| `ADMIN_PASSWORD` | Admin password (bcrypt) | Required |
| `ADMIN_JWT_SECRET` | JWT signing secret | Required |

| `SSH_PROXY_PORT` | SSH proxy port | `2222` |
| `LOG_FORMAT` | Log format `json` / `text` | `json` |
| `LOG_LEVEL` | Log level | `info` |

---

---

## Admin Dashboard

Beyond spoofing and IP isolation, the platform includes full governance:

- User lifecycle — create, suspend, auto-expire, password rotation
- Host lifecycle — create, start, stop, rebuild (keep or wipe /workspace), delete
- Egress IP management — CRUD, connectivity testing
- Bypass firewall — domain, CIDR, port whitelisting, snapshot versioning, preview → apply → rollback
- Full audit trail — every operation written to the events table
- SSE real-time push for task progress and host status
- Built-in KasmVNC + Chromium — one click from dashboard to remote desktop

---

## Architecture

```
                                                    ┌───────────────────────────────────┐
User ──curl──> Control Plane (:8080) ──Docker──>    │ User Container                    │
                    │                                │  SSH + Claude Code + VNC          │
               PostgreSQL                            │  sshfs ← same path as local cwd  │
                    │                                │  sing-box tun tunnel              │
              Admin UI (embed)                      │       ↓                           │
                    │                                │  Designated Exit IP               │
              SSH Proxy (:2222)                      └───────────────────────────────────┘
```

| Component | Description |
|-----------|-------------|
| **Control Plane** | Go API — auth, user management, task orchestration, SSH proxy |
| **Host Agent** | Privileged agent — manages Docker containers, network namespaces, and tunnels |
| **User Container** | Ubuntu 24.04 — OpenSSH + Claude Code + sshfs + KasmVNC + Chromium |
| **PostgreSQL** | Persists users, hosts, egress IPs, tasks, events, audit logs |
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
make dev      # Backend + frontend hot-reload (API :8080, frontend localhost:2568)
make test     # Run tests
```

See `make help` for all commands.

---

## Documentation

Full docs on [GitHub Pages](https://zanel1u.github.io/cloud-cli-proxy/en/): quick start, deployment, configuration, architecture, API reference, troubleshooting.

---

## License

[MIT](LICENSE)
