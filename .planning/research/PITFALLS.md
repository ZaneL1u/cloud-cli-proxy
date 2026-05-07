# Pitfalls Research: Adding Remote SSH Forwarding + Dev Containers Support

**Domain:** Containerized SSH cloud host platform (Cloud CLI Proxy)
**Researched:** 2026-05-08
**Confidence:** HIGH (based on official VS Code docs, gliderlabs/ssh source code, sing-box issue tracker, and real-world community issues)

---

## Critical Pitfalls

### Pitfall 1: SSH Proxy Rejects `direct-tcpip` — VS Code Remote SSH Completely Broken

**What goes wrong:**
The current SSH proxy at `internal/sshproxy/proxy.go:206-210` explicitly rejects ALL non-session channels with `"only session channels supported"`. VS Code Remote SSH requires `direct-tcpip` channels for port forwarding. The extension will fail immediately with errors like `open failed: administratively prohibited: open failed` or `ERROR unexpected channel type type=direct-tcpip`.

**Why it happens:**
The current proxy was designed for interactive shell sessions only. The rejection logic is a hardcoded `if newChan.ChannelType() != "session"` check. Developers often assume SSH "just needs session channels" without understanding that modern SSH clients (especially VS Code) rely heavily on `direct-tcpip` forwarding for their architecture.

**How to avoid:**
Implement `direct-tcpip` channel handling alongside `session`. The handler must:
1. Parse the `direct-tcpip` extra data (RFC 4254: dest_addr, dest_port, originator_addr, originator_port)
2. Validate the destination against an allowlist (see Pitfall 2)
3. Open a TCP connection to the destination
4. Bidirectionally copy data between the SSH channel and the TCP connection

Reference implementation pattern (from gliderlabs/ssh `DirectTCPIPHandler`):
```go
var d struct {
    DestAddr string
    DestPort uint32
    // ... originator fields
}
gossh.Unmarshal(newChan.ExtraData(), &d)
// validate d.DestAddr/d.DestPort against allowlist
conn, err := net.Dial("tcp", net.JoinHostPort(d.DestAddr, strconv.Itoa(int(d.DestPort))))
// ... io.Copy bidirectionally
```

**Warning signs:**
- VS Code Remote SSH connection fails at "Setting up SSH tunnel" stage
- SSH debug logs show `channel X: open failed: administratively prohibited`
- `ssh -L` manual test also fails through the proxy

**Phase to address:**
Phase 1 (SSH Proxy `direct-tcpip` support) — this is a hard blocker for all VS Code Remote SSH functionality.

---

### Pitfall 2: Unrestricted `direct-tcpip` Forwarding Becomes a Security Bypass

**What goes wrong:**
Simply accepting all `direct-tcpip` channels without destination validation allows users to tunnel to ANY address reachable from the proxy host — including the Docker daemon socket (if mounted), internal management APIs, the PostgreSQL instance, other users' containers via management veth IPs, or cloud metadata endpoints.

**Why it happens:**
The gliderlabs/ssh `DirectTCPIPHandler` has NO built-in destination validation — it delegates entirely to the optional `LocalPortForwardingCallback`. If this callback is nil or returns `true` unconditionally, every destination is permitted. Developers implementing forwarding often focus on "making it work" and skip the access control layer.

**How to avoid:**
Implement strict destination validation BEFORE dialing:

```go
func validateForwardDestination(destAddr string, destPort uint32) bool {
    // 1. Only allow forwarding to the user's own container
    // 2. Block private ranges that could reach host services
    // 3. Block Docker socket path (unix://) if any
    // 4. Block management network (10.99.x.x)
    // 5. Block cloud metadata endpoints (169.254.169.254)
}
```

Specific rules for this platform:
- Allow: the container's own management veth IP (from `DeriveManagementSSHAccess`)
- Allow: localhost/127.0.0.1 within the container (for VS Code server internal communication)
- Block: 10.99.0.0/16 (management veth network — prevents cross-container access)
- Block: 169.254.169.254 (cloud metadata)
- Block: Docker bridge networks (172.17.0.0/12)
- Block: host loopback via container namespace trickery

**Warning signs:**
- Security audit finds unexpected outbound connections from proxy host
- Users report they can `curl` internal services through forwarded ports
- Container escape via forwarded Docker socket

**Phase to address:**
Phase 1 (same as Pitfall 1) — the validation must be implemented alongside the handler, not as an afterthought.

---

### Pitfall 3: Missing `tcpip-forward` + `forwarded-tcpip` Breaks Remote Port Forwarding

**What goes wrong:**
VS Code Remote SSH uses not just `direct-tcpip` (local forwarding) but also `tcpip-forward` global requests and `forwarded-tcpip` channels (remote forwarding) for certain features. If these are not implemented, some VS Code functionality (like the "Ports" panel auto-forwarding) will silently fail or behave inconsistently.

**Why it happens:**
Developers often implement `direct-tcpip` and stop there, thinking "port forwarding works." But VS Code's architecture uses remote forwarding for:
- Auto-detecting and forwarding ports from the remote container
- Extension host communication in some configurations
- GitHub Codespaces-style port visibility features

**How to avoid:**
Implement the full forwarding triad:

| Channel/Request | Handler | Purpose |
|-----------------|---------|---------|
| `direct-tcpip` | `DirectTCPIPHandler` | Local forwarding (client → server → dest) |
| `tcpip-forward` | `ForwardedTCPHandler.HandleSSHRequest` | Request server to listen on a port |
| `forwarded-tcpip` | Opened by server when connection arrives | Server → client for remote forwards |
| `cancel-tcpip-forward` | `ForwardedTCPHandler.HandleSSHRequest` | Cancel a previous forward request |

The gliderlabs/ssh library provides `ForwardedTCPHandler` for the remote forwarding side. Use it, but apply the same destination validation logic.

**Warning signs:**
- VS Code "Ports" panel shows no forwarded ports
- `ssh -R` manual test fails
- Some extensions that rely on port auto-discovery don't work

**Phase to address:**
Phase 1 or Phase 2 (Remote forwarding can be deferred if `direct-tcpip` is the immediate blocker, but should be in the same milestone).

---

### Pitfall 4: Network Namespace Isolation Breaks VS Code Server Internal Communication

**What goes wrong:**
Containers are created with `--network=none` and only have a management veth + sing-box tun. VS Code Server running inside the container needs to bind to `127.0.0.1` for internal services and expects those services to be reachable via `direct-tcpip` forwarding from the SSH proxy. If the container's loopback interface is misconfigured or the tun device interferes with localhost routing, VS Code Server fails to start or extensions cannot connect.

**Why it happens:**
`sing-box` TUN with `auto_route: true` and `strict_route: true` can hijack ALL traffic including loopback-bound connections. The sing-box issue tracker shows multiple reports of TUN breaking localhost connectivity and Docker bridge networking. In a `--network=none` container, the ONLY network interfaces are `lo` and the sing-box `tun`. If sing-box routes `127.0.0.1` into the tunnel, VS Code's internal HTTP server becomes unreachable.

**How to avoid:**
1. Ensure sing-box configuration explicitly excludes `127.0.0.0/8` from TUN routing:
```json
{
  "route": {
    "rules": [
      {
        "ip_cidr": ["127.0.0.0/8", "::1/128"],
        "outbound": "direct"
      }
    ]
  }
}
```
2. Ensure the container's `lo` interface is UP and properly configured (it should be by default even with `--network=none`)
3. Test that `curl http://127.0.0.1:PORT` works inside the container before VS Code Server starts
4. Consider sing-box's `route_exclude_address` for the management veth subnet (10.99.x.x)

**Warning signs:**
- VS Code Server starts but extensions fail to activate
- "Failed to connect to extension host" errors
- `netstat -tlnp` inside container shows services listening, but connections timeout
- sing-box logs show unexpected routing of 127.0.0.1 traffic

**Phase to address:**
Phase 2 (Cloud版 VS Code Remote SSH 验证) — requires integration testing with actual VS Code client.

---

### Pitfall 5: Dev Containers `forwardPorts` Conflicts with Docker Port Mapping

**What goes wrong:**
In the local standalone mode, if both `devcontainer.json`'s `forwardPorts` and Docker Compose's `ports` (or `docker run -p`) map the same port, the second attempt fails with `bind: address already in use`. This is a known VS Code issue (#3025).

**Why it happens:**
Dev Containers has TWO port forwarding mechanisms:
- `forwardPorts` in `devcontainer.json`: SSH-based forwarding (works in Codespaces, requires active SSH session)
- `ports` in `docker-compose.yml` / `appPort`: Native Docker port publishing (direct host port binding)

They are NOT the same. Using both for the same port creates a collision.

**How to avoid:**
Choose ONE mechanism per port:

| Scenario | Use |
|----------|-----|
| Local standalone dev (Docker Desktop) | Docker `ports` or `appPort` — faster, direct |
| GitHub Codespaces / cloud | `forwardPorts` — required for cloud environments |
| Mixed local/cloud | Use `forwardPorts` consistently, remove `ports` from compose |

For Cloud CLI Proxy's local mode:
- Since containers already have `--network=none`, Docker port mapping won't work anyway
- Use `forwardPorts` exclusively, relying on the SSH proxy's `direct-tcpip` capability
- Document that local mode requires SSH-based forwarding, not Docker port publishing

**Warning signs:**
- `docker-compose up` fails with port already in use
- VS Code shows forwarded ports but connections are refused
- `forwardPorts` works in Codespaces but not locally

**Phase to address:**
Phase 3 (本地版 Dev Containers 支持) — specifically during `.devcontainer.json` template design.

---

### Pitfall 6: Dev Containers Lifecycle Scripts Run as Wrong User or in Wrong Order

**What goes wrong:**
In Dev Containers, `postCreateCommand` runs once at creation time, `postStartCommand` runs on every start. If `sshd` is started in `postCreateCommand` instead of `postStartCommand`, it won't be running after container restart. If scripts assume `root` but `remoteUser` is set to a non-root user, permission denied errors occur.

**Why it happens:**
The Dev Containers lifecycle has subtle ordering:
```
onCreateCommand → updateContentCommand → postCreateCommand → postStartCommand → postAttachCommand
```

- `postCreateCommand` runs ONCE (first creation only)
- `postStartCommand` runs EVERY time the container starts
- Feature commands run BEFORE user-defined commands
- `waitFor` controls when the IDE connects (default: `updateContentCommand`)

**How to avoid:**
For a container that needs SSH access via Dev Containers:
```json
{
  "postCreateCommand": "apt-get update && apt-get install -y openssh-server && mkdir -p /run/sshd",
  "postStartCommand": "/usr/sbin/sshd -D",
  "remoteUser": "vscode",
  "updateRemoteUserUID": true
}
```

Key rules:
- Install services in `postCreateCommand` (one-time setup)
- START services in `postStartCommand` (every boot)
- Match `remoteUser` to the user that owns the SSH keys and workspace
- Use `updateRemoteUserUID: true` to align container UID with host UID

**Warning signs:**
- SSH works on first container creation but fails after restart
- `Permission denied` when VS Code tries to install its server
- `mkdir: cannot create directory '/root': Permission denied`

**Phase to address:**
Phase 3 (本地版 Dev Containers 支持) — during `.devcontainer.json` template and entrypoint design.

---

### Pitfall 7: SSH Agent Forwarding Fails Due to Socket Path Mismatch

**What goes wrong:**
VS Code automatically creates its own SSH agent socket inside the container (`/tmp/vscode-ssh-auth-*.sock`) and forwards the host's agent. But this auto-forwarding can conflict with manual `mounts` + `remoteEnv` configurations. The agent socket may exist but contain no identities, or the `SSH_AUTH_SOCK` environment variable may point to the wrong path.

**Why it happens:**
Multiple mechanisms compete:
1. VS Code's built-in auto-forwarding (creates proxy socket)
2. Manual `mounts` + `remoteEnv.SSH_AUTH_SOCK` (user-configured)
3. Dev Container Features (e.g., `ghcr.io/devcontainers/features/sshd:1`)

They can override each other. Cursor IDE was specifically reported to create its own relay socket that bypasses user configuration.

**How to avoid:**
For Cloud CLI Proxy's use case (managed containers accessed via SSH proxy):
- The SSH proxy handles authentication — agent forwarding may not be needed for the primary use case
- If agent forwarding IS needed (e.g., for git operations inside the container), explicitly configure:
```json
{
  "mounts": [
    "type=bind,source=${env:SSH_AUTH_SOCK},target=/tmp/ssh-agent.sock"
  ],
  "remoteEnv": {
    "SSH_AUTH_SOCK": "/tmp/ssh-agent.sock"
  }
}
```
- Test with `ssh-add -l` inside the container as the first validation step
- Document that VS Code's auto-forwarding may override manual settings

**Warning signs:**
- `ssh-add -l` shows "The agent has no identities" inside container
- `ssh -v git@github.com` shows it's not trying any keys
- Socket file exists at expected path but is empty/unusable

**Phase to address:**
Phase 3 (本地版 Dev Containers 支持) — during SSH agent integration testing.

---

### Pitfall 8: Cloud/Local Architecture Boundary Blur Leads to Maintenance Nightmare

**What goes wrong:**
Attempting to make Cloud版和本地版 share too much code or too little code. If they share everything, the local mode drags in control-plane + PostgreSQL dependencies. If they share nothing, bug fixes and feature updates must be duplicated.

**Why it happens:**
The two modes have fundamentally different architectures:

| Aspect | Cloud版 | 本地版 |
|--------|---------|--------|
| Auth | Control-plane + PostgreSQL | Local config file or env vars |
| Container lifecycle | Control-plane manages | Local CLI manages |
| Network | Management veth + sing-box tun | Direct Docker networking or host |
| SSH access | Via SSH proxy | Direct container port or SSH proxy |
| Dev Containers | N/A (Remote SSH instead) | `.devcontainer.json` support |

**How to avoid:**
Define clear shared boundaries:

**Shared components (library packages):**
- SSH proxy implementation (with `direct-tcpip`/`tcpip-forward` support)
- Container network setup (namespace, veth, tun injection)
- sing-box configuration generation
- Error code registry and explain system

**Cloud-only components:**
- Control-plane API server
- PostgreSQL models and migrations
- Admin dashboard (React)
- JWT authentication
- Container lifecycle scheduler

**Local-only components:**
- Standalone CLI entrypoint (`cloud-claude local` or new binary)
- Local config file management
- Docker Compose orchestration (if needed)
- `.devcontainer.json` template generation

**Warning signs:**
- Local mode binary size balloons due to imported control-plane packages
- Database migration files appear in local mode codebase
- Feature flags proliferate to switch between modes (`if cloudMode { ... } else { ... }`)
- Testing one mode requires setting up the other's infrastructure

**Phase to address:**
Phase 0 (架构边界分析) — BEFORE any implementation. This is an architectural decision that affects all subsequent phases.

---

## Technical Debt Patterns

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|----------------|-----------------|
| Accept all `direct-tcpip` without validation | VS Code works immediately | Security bypass — users can reach any internal service | NEVER |
| Only implement `direct-tcpip`, skip `tcpip-forward` | Core VS Code functionality works | Remote port forwarding features broken | Only for initial MVP, must follow up |
| Use `gliderlabs/ssh` default handlers without callbacks | Less code to write | No audit trail, no access control | NEVER in production |
| Hardcode container SSH credentials | Simpler auth | Security vulnerability, no rotation | NEVER |
| Share control-plane DB models with local mode | Less code duplication | Local mode requires Postgres, loses standalone value | NEVER |

---

## Integration Gotchas

| Integration | Common Mistake | Correct Approach |
|-------------|----------------|------------------|
| VS Code Remote SSH | Only test with terminal SSH, not VS Code | Test with actual VS Code client; it uses channels differently |
| sing-box TUN + container | Enable `strict_route: true` without exclusions | Exclude 127.0.0.0/8 and management veth subnet from TUN |
| Dev Containers + Docker Compose | Define ports in BOTH compose and devcontainer.json | Choose one: `forwardPorts` for SSH-based, `ports` for direct |
| SSH agent forwarding | Mount socket but set `containerEnv` instead of `remoteEnv` | Use `remoteEnv` so VS Code server AND terminal inherit it |
| gliderlabs/ssh | Use `DirectTCPIPHandler` without `LocalPortForwardingCallback` | Always set callback for destination validation and audit logging |

---

## Performance Traps

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| `direct-tcpip` handler spawns goroutines without limits | Memory exhaustion under many forwarded connections | Add per-connection rate limiting and max concurrent forwards | >50 simultaneous forwarded channels |
| `tcpip-forward` listeners accumulate without cleanup | File descriptor exhaustion, port leaks | Implement listener cleanup on SSH disconnect, timeout stale forwards | Long-running sessions with many port forwards |
| VS Code Server + sing-box TUN = high CPU | TUN intercepts all VS Code internal HTTP traffic | Exclude 127.0.0.0/8 from TUN routing; use `direct` outbound | Any VS Code Remote SSH session |

---

## Security Mistakes

| Mistake | Risk | Prevention |
|---------|------|------------|
| `direct-tcpip` to Docker daemon socket (unix:///var/run/docker.sock) | Container escape, full host compromise | Block all Unix socket paths; validate only TCP destinations |
| `direct-tcpip` to cloud metadata (169.254.169.254) | Credential theft, instance takeover | Hardcode block for 169.254.0.0/16 in validator |
| `direct-tcpip` to management veth network (10.99.x.x) | Cross-container access, lateral movement | Block management subnet; only allow target container's own IP |
| Forwarding to `0.0.0.0` interpreted as host | Accidental host network exposure | Normalize `0.0.0.0` to `127.0.0.1` in container context |
| No audit logging for forwarded connections | Cannot detect abuse or investigate incidents | Log every `direct-tcpip`/`tcpip-forward` request with user, source, destination |
| VS Code auto-exposes `~/.ssh` to container | Private keys leaked to container environment | Document risk; provide `postCreateCommand` to remove auto-mounted keys if desired |

---

## UX Pitfalls

| Pitfall | User Impact | Better Approach |
|---------|-------------|-----------------|
| VS Code Remote SSH fails with generic "connection lost" | User has no idea `direct-tcpip` is the issue | Provide diagnostic command that checks proxy channel support |
| Dev Container fails silently after restart | User thinks configuration is broken | Document that `postStartCommand` is required for service startup |
| Port forwarding works for some ports but not others | Confusing behavior, hard to debug | Document which ports use Docker mapping vs SSH forwarding |
| Local mode requires control-plane setup | Defeats "standalone" purpose | Ensure local mode binary has zero external dependencies |

---

## "Looks Done But Isn't" Checklist

- [ ] **SSH Proxy `direct-tcpip`:** Handler implemented AND destination validator enforced — verify with `ssh -L` and `nc` probe to blocked destination
- [ ] **SSH Proxy `tcpip-forward`:** Global request handler AND `forwarded-tcpip` channel support — verify with `ssh -R` test
- [ ] **Destination validation:** Management subnet blocked, cloud metadata blocked, Docker socket blocked — verify with unit tests for each case
- [ ] **sing-box TUN exclusions:** 127.0.0.0/8 routes to `direct`, management subnet excluded — verify with `ip route` inside container
- [ ] **VS Code Remote SSH:** Full connection, terminal, file explorer, and port forwarding all work — verify with actual VS Code client
- [ ] **Dev Containers lifecycle:** `sshd` starts on every container resume, not just first creation — verify by stopping and restarting container
- [ ] **SSH agent forwarding:** `ssh-add -l` works inside container — verify with GitHub git push test
- [ ] **Cloud/Local boundary:** Local mode binary runs without Postgres or control-plane — verify by running on clean machine

---

## Recovery Strategies

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| Unrestricted forwarding deployed | HIGH (security incident) | Immediately restrict with `LocalPortForwardingCallback`; audit logs for abuse; rotate any exposed credentials |
| VS Code Server broken by TUN routing | LOW | Add 127.0.0.0/8 exclusion to sing-box config; restart container |
| Dev Container service not starting after restart | LOW | Move service start from `postCreateCommand` to `postStartCommand`; rebuild container |
| Cloud/Local code entanglement | MEDIUM | Extract shared packages; introduce interface boundaries; gradual refactoring |

---

## Pitfall-to-Phase Mapping

| Pitfall | Prevention Phase | Verification |
|---------|------------------|------------|
| Pitfall 1: Rejecting `direct-tcpip` | Phase 1: SSH Proxy forwarding support | `ssh -L` manual test; VS Code connection test |
| Pitfall 2: Unrestricted forwarding | Phase 1: SSH Proxy forwarding support | Unit tests for validator; penetration test to internal services |
| Pitfall 3: Missing `tcpip-forward` | Phase 1 or Phase 2 | `ssh -R` test; VS Code Ports panel auto-forward test |
| Pitfall 4: TUN breaks localhost | Phase 2: Cloud版 VS Code Remote SSH 验证 | `curl 127.0.0.1:PORT` inside container; VS Code extension activation |
| Pitfall 5: Port mapping conflict | Phase 3: 本地版 Dev Containers 支持 | `docker-compose up` with both configs; verify no bind errors |
| Pitfall 6: Wrong lifecycle script | Phase 3: 本地版 Dev Containers 支持 | Stop/restart container; verify `sshd` process exists |
| Pitfall 7: Agent forwarding fails | Phase 3: 本地版 Dev Containers 支持 | `ssh-add -l` inside container; git push to GitHub |
| Pitfall 8: Architecture boundary blur | Phase 0: 架构边界分析 | Code review: no control-plane imports in local mode; no DB dependencies |

---

## Sources

- [VS Code Remote Development Troubleshooting](https://code.visualstudio.com/docs/remote/troubleshooting) — Official docs confirming `AllowTcpForwarding yes` requirement
- [VS Code Remote-SSH Issue #92](https://github.com/microsoft/vscode-remote-release/issues/92) — "Don't rely on ssh TCP port forwarding" feature request, confirms dependency
- [gliderlabs/ssh tcpip.go](https://github.com/gliderlabs/ssh/blob/master/tcpip.go) — Source code for `DirectTCPIPHandler` and `ForwardedTCPHandler`
- [Fly.io VS Code Remote SSH Discussion](https://community.fly.io/t/how-to-connect-vscode-remote-development-to-a-fly-machine/23541) — Real-world failure due to missing `direct-tcpip`
- [Tailscale SSH Issue #5295](https://github.com/tailscale/tailscale/issues/5295) — `unsupported channel type` with VS Code
- [Dev Containers Discussion #224](https://github.com/orgs/devcontainers/discussions/224) — SSH agent forwarding comprehensive troubleshooting
- [VS Code Remote-SSH Issue #3025](https://github.com/microsoft/vscode-remote-release/issues/3025) — `forwardPorts` conflicts with Docker Compose port mapping
- [sing-box Issue #2700](https://github.com/SagerNet/sing-box/issues/2700) — TUN mode breaks Docker bridge networking
- [sing-box Issue #1666](https://github.com/SagerNet/sing-box/issues/1666) — `auto_route` hardcodes routing table 2022
- [sing-box Issue #2322](https://github.com/SagerNet/sing-box/issues/2322) — SSH connection drops in TUN mode
- [Dev Containers Lifecycle Scripts](https://code.visualstudio.com/docs/devcontainers/containers#_lifecycle-scripts) — Official execution order documentation
- [VS Code Remote-SSH Issue #7657](https://github.com/microsoft/vscode-remote-release/issues/7657) — Permission denied with `remoteUser`
- [Cursor Forum: SSH Forwarding Broken](https://forum.cursor.com/t/ssh-forwarding-broken-in-new-dev-containers-extension/124369) — IDE-specific agent forwarding issues
- [DevPod SSH-Based Dev Containers](https://fabiorehm.com/blog/2025/11/11/devpod-ssh-devcontainers/) — Alternative architecture for SSH-based containers

---
*Pitfalls research for: Cloud CLI Proxy v3.2 — Remote SSH + Dev Containers milestone*
*Researched: 2026-05-08*
