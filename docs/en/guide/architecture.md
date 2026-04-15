# Architecture

## System Architecture

```
┌─────────────┐   curl    ┌────────────────┐    Docker     ┌──────────────────────┐
│  End User   │ ────────> │  Control Plane │ ───────────>  │  User Container      │
│ (SSH Client)│ <──────── │  (Go API :8080)│              │  SSH + Claude + VNC   │
└─────────────┘   SSH     └───────┬────────┘              └────────┬─────────────┘
       │                          │                                │
       │                   ┌──────┴───────┐               ┌───────┴─────────┐
       │                   │  PostgreSQL   │               │  sing-box tun   │
       │                   │  (State store)│               │  (full tunnel)  │
       │                   └──────────────┘               └───────┬─────────┘
       │                          │                                │
       │                   ┌──────┴───────┐               ┌───────┴─────────┐
       │                   │  Admin SPA   │               │  Designated     │
       └──── SSH ────────> │  (:3000)     │               │  Exit IP        │
           proxy :2222     └──────────────┘               └─────────────────┘
```

## Core Components

### Control Plane

Go API server, the system's central coordinator:

- **HTTP API** — RESTful interface for admin and user panels
- **Authentication** — JWT token issuance, supports admin and user roles
- **Task Orchestration** — Host create, start, stop, rebuild via async task queue
- **Expiry Scanner** — Periodically checks user expiration, auto-stops and disables
- **SSH Proxy** — Listens on `:2222`, proxies SSH sessions to target containers
- **Reconciler** — Syncs running container state with database records

### Host Agent

Executes privileged host operations, communicates with control plane via Unix socket:

- **Docker Management** — Create, start, stop, and delete user containers
- **Network Configuration** — Create namespaces, configure sing-box tun
- **Firewall Management** — Set nftables default-deny rules per container
- **Network Verification** — Triple check: connectivity, exit IP match, DNS leak detection

Two run modes:
- `socket` — Standalone process, receives commands via Unix socket (production)
- `embedded` — Runs inside control plane process (development / Docker Compose)

### User Container

Managed image based on Ubuntu 24.04, created with `--network=none` for complete network isolation:

- OpenSSH Server — SSH access
- Claude Code — AI coding assistant
- KasmVNC + Fluxbox + Chromium — Remote desktop
- sing-box — Proxy mode tunnel client
- Common dev tools — Git, tmux, zsh, Node.js, etc.

### PostgreSQL

Persists all system state:

- User accounts, passwords (bcrypt), and expiration
- Host records, short IDs, SSH passwords, and status
- Egress IP config (proxy config JSONB)
- Host-to-egress-IP bindings
- Async task records
- Audit events (13 event types)

## Network Model

### Container Network Isolation

Each user container is created with `--network=none` — no network interfaces except loopback. Cannot reach any external network directly.

### sing-box tun Proxy Tunnel

```
User container namespace
├── lo (loopback)
├── veth (virtual ethernet to host)
├── tun0 (sing-box tun device)
│   └── Route: 0.0.0.0/0 → tun0
└── nftables: default deny, only proxy server connections allowed
```

sing-box runs in tun mode, capturing all outbound traffic and forwarding through the designated proxy protocol.

### Triple Network Verification

After every host start, three checks must pass or the host is marked unavailable:

1. **Connectivity test** — HTTP request to external endpoint from container namespace
2. **Exit IP match** — Verify actual exit IP matches expected
3. **DNS leak detection** — Ensure DNS requests also go through the tunnel

## Security Boundaries

### Privilege Separation

```
┌─────────────────────────────────┐
│  Control Plane (low privilege)   │
│  - HTTP API + business logic    │
│  - No direct Docker/network     │
│  - Delegates via Unix socket    │
└─────────────┬───────────────────┘
              │ Unix socket
┌─────────────┴───────────────────┐
│  Host Agent (high privilege)     │
│  - Docker container management  │
│  - Network namespace ops        │
│  - nftables firewall config     │
│  - sing-box management         │
└─────────────────────────────────┘
```

The web/API layer does not hold excessive host privileges. All Docker and network namespace operations are centralized in host-agent, exposed to the control plane through a Unix socket interface.

### User Isolation

- Each user gets an independent container, created with `--network=none`
- No inter-container networking
- Users can only access their own container via SSH (direct or proxy)
- JWT tokens distinguish admin and user roles; users only see their own resources

### Credential Management

- User passwords stored as bcrypt hashes
- Admin dashboard uses JWT token auth, keys are rotatable
- Container SSH passwords are independent from user login passwords

## Data Flows

### Bootstrap Access Flow

```
User → curl /v1/bootstrap/script → Get bootstrap script
     → Script prompts for username and password
     → POST /v1/bootstrap/sessions → Auth + queue start task
     → Poll GET /v1/bootstrap/tasks/{id} → Wait for completion
     → GET /v1/bootstrap/tasks/{id}/handoff → Get SSH params
     → exec ssh → Enter container
```

### Entry Short-link Access Flow

```
User → curl /entry/{shortId} → Get entry script
     → Script prompts for password
     → POST /v1/entry/{shortId}/auth → Authenticate
     → Returns SSH params (host, port, user)
     → ssh -p 2222 → Connect via SSH proxy
```

### Host Startup Task Flow

```
Control plane creates task → host-agent receives
  → Pull/verify managed image
  → Create container (--network=none)
  → Create network namespace
  → Configure sing-box tun
  → Configure nftables firewall rules
  → Start container
  → Triple network verification
  → Mark task succeeded
```

## Architecture Principles

- **Single-host first** — No multi-node scheduling in v1
- **Network enforcement first** — All traffic must go through designated exits
- **Startup experience** — Built on verifiable runtime correctness
- **Least privilege** — Strict separation between API layer and privileged operations

## Project Structure

```
cloud-cli-proxy/
├── cmd/
│   ├── control-plane/          # Control plane API entrypoint
│   └── host-agent/             # Host agent entrypoint
├── internal/
│   ├── controlplane/           # HTTP routes, business logic, expiry scanner, reconciler
│   │   ├── http/               # Route registration and middleware
│   │   ├── app/                # App lifecycle and dependency assembly
│   │   └── admin/              # Admin API handlers
│   ├── agent/                  # Host-agent server
│   ├── network/                # nftables / sing-box networking
│   ├── runtime/                # Task runtime, Docker container lifecycle
│   ├── sshproxy/               # SSH proxy (forwards to container port 22)
│   └── store/                  # Database migrations and queries (pgx)
├── web/admin/                  # React admin dashboard (TanStack Router + Query)
├── deploy/
│   ├── docker/                 # 4 Dockerfiles
│   │   ├── control-plane/      # Control plane image
│   │   ├── admin/              # Admin dashboard image
│   │   ├── managed-user/       # User container image
│   │   └── sing-box-gateway/   # sing-box gateway sidecar
│   ├── compose/                # Dev Compose files
│   ├── bootstrap/              # User curl bootstrap script
│   ├── scripts/                # setup-env.sh, deploy.sh, backup.sh
│   └── systemd/                # systemd service units
├── docs/                       # VitePress docs site
├── docker-compose.yml          # Production Compose
├── Makefile                    # Dev command entrypoint
└── .github/workflows/          # CI/CD
```

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Backend | Go 1.26, net/http stdlib, pgx v5 |
| Frontend | React 19, TypeScript, Vite, Tailwind CSS, TanStack Router/Query |
| Database | PostgreSQL 18 |
| Containers | Docker Engine 28, Ubuntu 24.04 user image |
| Networking | sing-box tun + Linux netns, nftables |
| Desktop | KasmVNC 1.4.0 + Fluxbox + Chromium |
| CI/CD | GitHub Actions, multi-arch builds |
