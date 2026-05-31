# Architecture

## Overview

Cloud CLI Proxy consists of four core components: Control Plane, Host Agent, user containers, and the cloud-claude CLI. The control plane handles API, authentication, and task orchestration. The host agent executes Docker and network operations. The two communicate over a Unix socket.

```
User ──curl──> Control Plane (:8080) ──Docker──> │ User Container           │
                    │                              │  SSH + Claude + VNC      │
               PostgreSQL                          │  sing-box tun tunnel     │
                    │                              │       ↓                  │
              Admin SPA (:3000)                    │  Designated egress IP    │
                    │                              └──────────────────────────┘
              SSH Proxy (:2222)
```

## Core Components

### Control Plane

A Go API server that acts as the central orchestrator:

- **HTTP API** — RESTful interface for admin and user panels
- **Authentication** — JWT token issuance with admin and user roles
- **Task orchestration** — Host create, start, stop, rebuild via async task queue
- **Expiry scanner** — Periodically checks user expiration, auto-stops and disables
- **SSH proxy** — Listens on `:2222`, proxies SSH sessions to target containers
- **Reconciler** — Syncs running container state with database records

### Host Agent

Executes privileged host operations, communicates with the control plane over a Unix socket:

- **Docker management** — Create, start, stop, delete user containers
- **Network configuration** — Create netns, configure sing-box tun
- **Firewall management** — Set nftables default-deny rules per container
- **Network verification** — Triple check: connectivity, exit IP match, DNS leak detection

Two run modes: `socket` (standalone process, recommended for production) and `embedded` (inside control plane, for development).

### User Container

A managed image based on Ubuntu 24.04, created with `--network=none` for complete network isolation. Pre-installed with OpenSSH, Claude Code, KasmVNC + Chromium, sing-box, and dev tools (Git, tmux, zsh, Node.js).

### cloud-claude CLI

A Go CLI installed on the user's laptop, bridging the local terminal and the remote container:

- Local directory mounted at the same path inside the container via sshfs
- Three mount modes: Auto / Full / SSHFS-Only
- tmux multi-client sessions with auto-reconnect on disconnect
- `doctor` five-domain self-check + `--fix` auto-repair
- `explain` error code lookup
- `proxy_commands` to run commands like `git` on the local machine

### PostgreSQL

Persists all system state: users, hosts, egress IP configs, bindings, async tasks, and audit events.

## Network Model

### Container Network Isolation

Each container is created with `--network=none`. Only the loopback interface exists after creation. No direct external network access.

### sing-box tun Tunnel

```
User container namespace
├── lo (loopback)
├── tun0 (sing-box tun device)
│   └── Route: 0.0.0.0/0 → tun0
└── nftables: default deny, only proxy server connections allowed
```

sing-box runs in tun mode, capturing all outbound traffic and forwarding through the designated proxy protocol.

### Triple Network Verification

After every host start, three checks are performed:

1. **Connectivity** — HTTP request to external endpoint from container netns
2. **Exit IP match** — Verify actual exit IP matches the configured one
3. **DNS leak detection** — Ensure DNS requests also go through the tunnel

If any check fails, the host is marked unavailable.

## Security Boundaries

### Privilege Separation

The control plane does not touch Docker or networking directly. All privileged operations are centralized in the host agent and exposed to the control plane through a Unix socket.

### User Isolation

- Each user gets an independent container, created with `--network=none`
- No inter-container networking
- JWT tokens distinguish roles; users can only access their own resources

### Credential Management

- User passwords stored as bcrypt hashes
- JWT token authentication with rotatable keys
- Container SSH passwords are independent from user login passwords

## Data Flows

### User Access (curl)

```
User → curl /entry/{shortId} → Get entry script
     → Enter password
     → POST /v1/entry/{shortId}/auth → Authenticate
     → SSH params returned → SSH proxy connects to container
```

### cloud-claude CLI

```
User → cloud-claude init → Write config
     → cd project dir → cloud-claude
     → Authenticate + wait for container ready
     → sshfs mount local directory to same path in container
     → Attach or create tmux session
     → Start Claude Code remotely
```

### Host Startup Task Flow

```
Control plane creates task → Host Agent receives
  → Pull managed image
  → Create container (--network=none)
  → Configure netns + sing-box tun
  → Configure nftables
  → Start container
  → Triple network verification
  → Mark task succeeded
```

## Architecture Principles

- **Single-host first** — No multi-node scheduling in v1
- **Network enforcement** — All traffic must go through designated egress IPs; no bypasses
- **Least privilege** — Strict separation between API layer and privileged operations

## Project Structure

```
cloud-cli-proxy/
├── cmd/
│   ├── cloud-claude/           # cloud-claude CLI
│   ├── control-plane/          # Control plane API
│   └── host-agent/             # Host agent
├── internal/
│   ├── controlplane/           # HTTP routes, business logic, scheduling
│   │   ├── http/               # Routes and middleware
│   │   ├── app/                # App lifecycle
│   │   ├── scheduler/          # Expiry scanner and scheduled tasks
│   │   └── credgen/            # Credential generation
│   ├── agent/                  # Host agent server
│   ├── agentapi/               # Agent API client
│   ├── broadcast/              # SSE real-time broadcast
│   ├── cloudclaude/            # cloud-claude CLI library
│   ├── local/                  # Local Dev Containers
│   ├── network/                # nftables / sing-box configuration
│   ├── runtime/                # Task runtime, container lifecycle
│   ├── sshproxy/               # SSH proxy
│   └── store/                  # Database migrations and queries (pgx)
├── web/admin/                  # React admin dashboard
├── deploy/                     # Dockerfiles, Compose, scripts
├── docs/                       # VitePress documentation
├── docker-compose.yml          # Production Compose
└── Makefile
```

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Backend | Go 1.26, net/http, pgx v5 |
| Frontend | React 19, TypeScript, Vite, Tailwind CSS |
| Database | PostgreSQL 18 |
| Containers | Docker Engine 28, Ubuntu 24.04 |
| Networking | sing-box tun + Linux netns, nftables |
| Desktop | KasmVNC + Chromium |
