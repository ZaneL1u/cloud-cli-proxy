# API Reference

All APIs use `http://YOUR_HOST:8080` as base URL.

## Authentication

### Login

```bash
curl -s -X POST http://YOUR_HOST:8080/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"your-password"}'
```

**Response:**
```json
{"token":"eyJhbGci...","role":"admin"}
```

All Admin API calls require `Authorization: Bearer $TOKEN` header.

Quick token extraction:

```bash
TOKEN=$(curl -s -X POST http://YOUR_HOST:8080/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"your-password"}' | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
```

::: tip
`/v1/admin/login` is a legacy-compatible path, functionally identical to `/v1/auth/login`.
:::

## Health Check

```bash
curl -s http://YOUR_HOST:8080/healthz
```

**Response:**
```json
{"status":"ok","checks":{"database":"ok","agent":"ok"}}
```

## User Management

### Create User

```bash
curl -s -X POST http://YOUR_HOST:8080/v1/admin/users \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "username": "newuser",
    "password": "min-8-chars",
    "expires_at": "2026-12-31T23:59:59Z"
  }'
```

Username: 3-50 chars. Password: 8+ chars. `expires_at` is optional (no expiry if omitted).

| Status | Description |
|--------|-------------|
| `201` | Created successfully |
| `409` | Username already exists |

### List Users

```bash
curl -s http://YOUR_HOST:8080/v1/admin/users \
  -H "Authorization: Bearer $TOKEN"
```

### Get User Details

```bash
curl -s http://YOUR_HOST:8080/v1/admin/users/{userID} \
  -H "Authorization: Bearer $TOKEN"
```

### Disable / Enable User

```bash
curl -s -X PATCH http://YOUR_HOST:8080/v1/admin/users/{userID} \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"status":"disabled"}'
```

Values: `active`, `disabled`.

### Delete User

```bash
curl -s -X DELETE http://YOUR_HOST:8080/v1/admin/users/{userID} \
  -H "Authorization: Bearer $TOKEN"
```

Returns `204`. Associated hosts are cascade-deleted.

### Rotate Password

```bash
curl -s -X POST http://YOUR_HOST:8080/v1/admin/users/{userID}/rotate-password \
  -H "Authorization: Bearer $TOKEN"
```

Returns a new auto-generated 20-char password in plaintext.

### Set Expiration

```bash
curl -s -X PUT http://YOUR_HOST:8080/v1/admin/users/{userID}/expiry \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"expires_at":"2026-12-31T23:59:59Z"}'
```

Clear expiration (never expires):

```bash
curl -s -X PUT http://YOUR_HOST:8080/v1/admin/users/{userID}/expiry \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"expires_at":null}'
```

## SSH Key Management

### Admin-managed User SSH Keys

```bash
# List user's SSH keys
curl -s http://YOUR_HOST:8080/v1/admin/users/{userID}/ssh-keys \
  -H "Authorization: Bearer $TOKEN"

# Add SSH public key
curl -s -X POST http://YOUR_HOST:8080/v1/admin/users/{userID}/ssh-keys \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"public_key":"ssh-ed25519 AAAA... user@host"}'

# Auto-generate key pair
curl -s -X POST http://YOUR_HOST:8080/v1/admin/users/{userID}/ssh-keys/generate \
  -H "Authorization: Bearer $TOKEN"

# Delete SSH key
curl -s -X DELETE http://YOUR_HOST:8080/v1/admin/users/{userID}/ssh-keys/{keyID} \
  -H "Authorization: Bearer $TOKEN"
```

### User Self-managed SSH Keys

Users use their own JWT token:

```bash
# List own SSH keys
curl -s http://YOUR_HOST:8080/v1/user/ssh-keys \
  -H "Authorization: Bearer $TOKEN"

# Add SSH public key
curl -s -X POST http://YOUR_HOST:8080/v1/user/ssh-keys \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"public_key":"ssh-ed25519 AAAA... user@host"}'

# Auto-generate key pair
curl -s -X POST http://YOUR_HOST:8080/v1/user/ssh-keys/generate \
  -H "Authorization: Bearer $TOKEN"
```

## Egress IP Management

### Create Egress IP

`proxy_config` follows [sing-box outbound](https://sing-box.sagernet.org/configuration/outbound/) format.

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
      "password": "your-password"
    }
  }'

# VMess
curl -s -X POST http://YOUR_HOST:8080/v1/admin/egress-ips \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "label": "us-vmess-01",
    "ip_address": "203.0.113.20",
    "tunnel_type": "proxy",
    "provider": "manual",
    "proxy_config": {
      "type": "vmess",
      "server": "203.0.113.20",
      "server_port": 443,
      "uuid": "your-uuid",
      "security": "auto",
      "alter_id": 0
    }
  }'

# SOCKS5
curl -s -X POST http://YOUR_HOST:8080/v1/admin/egress-ips \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "label": "eu-socks-01",
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

# Trojan
curl -s -X POST http://YOUR_HOST:8080/v1/admin/egress-ips \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "label": "sg-trojan-01",
    "ip_address": "203.0.113.30",
    "tunnel_type": "proxy",
    "provider": "manual",
    "proxy_config": {
      "type": "trojan",
      "server": "203.0.113.30",
      "server_port": 443,
      "password": "your-password",
      "tls": {"enabled": true, "server_name": "your-domain.com"}
    }
  }'

# HTTP Proxy
curl -s -X POST http://YOUR_HOST:8080/v1/admin/egress-ips \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "label": "proxy-http-01",
    "ip_address": "192.0.2.100",
    "tunnel_type": "proxy",
    "provider": "manual",
    "proxy_config": {
      "type": "http",
      "server": "192.0.2.100",
      "server_port": 8080,
      "username": "user",
      "password": "pass"
    }
  }'
```

### List Egress IPs

```bash
curl -s http://YOUR_HOST:8080/v1/admin/egress-ips \
  -H "Authorization: Bearer $TOKEN"
```

### Get Egress IP Details

```bash
curl -s http://YOUR_HOST:8080/v1/admin/egress-ips/{ipID} \
  -H "Authorization: Bearer $TOKEN"
```

### Update Egress IP

```bash
curl -s -X PUT http://YOUR_HOST:8080/v1/admin/egress-ips/{ipID} \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"label": "jp-ss-01-updated", "proxy_config": {"type": "shadowsocks", "server": "198.51.100.5", "server_port": 8388, "method": "aes-256-gcm", "password": "new-password"}}'
```

### Delete Egress IP

```bash
curl -s -X DELETE http://YOUR_HOST:8080/v1/admin/egress-ips/{ipID} \
  -H "Authorization: Bearer $TOKEN"
```

Rejects deletion (`409`) if IP is still bound to hosts. Unbind first.

### Test Egress IP

```bash
curl -s -X POST http://YOUR_HOST:8080/v1/admin/egress-ips/{ipID}/test \
  -H "Authorization: Bearer $TOKEN"
```

Runs three checks (30-second timeout):

| Check | Description |
|-------|-------------|
| Connectivity | HTTP request through tunnel to external endpoint |
| Exit IP match | Actual exit IP matches declared `ip_address` |
| DNS leak | DNS requests go through tunnel |

## Host Management

### Create Host

```bash
curl -s -X POST http://YOUR_HOST:8080/v1/admin/hosts \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"user_id": "user-uuid"}'
```

### List Hosts

```bash
curl -s http://YOUR_HOST:8080/v1/admin/hosts \
  -H "Authorization: Bearer $TOKEN"
```

### Get Host Details

```bash
curl -s http://YOUR_HOST:8080/v1/admin/hosts/{hostID} \
  -H "Authorization: Bearer $TOKEN"
```

Returns host info including user access command, short ID, etc.

### Start / Stop Host

```bash
# Start
curl -s -X POST http://YOUR_HOST:8080/v1/admin/hosts/{hostID}/start \
  -H "Authorization: Bearer $TOKEN"

# Stop
curl -s -X POST http://YOUR_HOST:8080/v1/admin/hosts/{hostID}/stop \
  -H "Authorization: Bearer $TOKEN"
```

### Rebuild Host

```bash
curl -s -X POST http://YOUR_HOST:8080/v1/admin/hosts/{hostID}/rebuild \
  -H "Authorization: Bearer $TOKEN"
```

Destroys and recreates the container from the managed image. Home volume is preserved.

### Rotate SSH Password

```bash
curl -s -X POST http://YOUR_HOST:8080/v1/admin/hosts/{hostID}/rotate-ssh-password \
  -H "Authorization: Bearer $TOKEN"
```

### Restart VNC Service (without rebuilding host)

```bash
curl -s -X POST http://YOUR_HOST:8080/v1/admin/hosts/{hostID}/vnc/restart \
  -H "Authorization: Bearer $TOKEN"
```

### Delete Host

```bash
curl -s -X DELETE http://YOUR_HOST:8080/v1/admin/hosts/{hostID} \
  -H "Authorization: Bearer $TOKEN"
```

### VNC Reverse Proxy

The admin dashboard proxies to KasmVNC inside the container:

```
/v1/admin/hosts/{hostID}/vnc/{path...}
```

## Host & Egress IP Bindings

### Create Binding

```bash
curl -s -X POST http://YOUR_HOST:8080/v1/admin/bindings \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"host_id":"host-uuid","egress_ip_id":"egress-ip-uuid"}'
```

### Remove Binding

```bash
curl -s -X DELETE http://YOUR_HOST:8080/v1/admin/bindings/{bindingID} \
  -H "Authorization: Bearer $TOKEN"
```

## User Portal API

These endpoints are for regular users (using user-role JWT tokens).

### Change Password

```bash
curl -s -X POST http://YOUR_HOST:8080/v1/user/change-password \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"old_password":"old","new_password":"new"}'
```

### List Own Hosts

```bash
curl -s http://YOUR_HOST:8080/v1/user/hosts \
  -H "Authorization: Bearer $TOKEN"
```

### Get Host Details

```bash
curl -s http://YOUR_HOST:8080/v1/user/hosts/{hostID} \
  -H "Authorization: Bearer $TOKEN"
```

### Rebuild Own Host

```bash
curl -s -X POST http://YOUR_HOST:8080/v1/user/hosts/{hostID}/rebuild \
  -H "Authorization: Bearer $TOKEN"
```

### Restart Own VNC Service

```bash
curl -s -X POST http://YOUR_HOST:8080/v1/user/hosts/{hostID}/vnc/restart \
  -H "Authorization: Bearer $TOKEN"
```

### User VNC Access

```
/v1/user/hosts/{hostID}/vnc/{path...}
```

## Bootstrap Access

### Get Bootstrap Script

```bash
curl -sSf http://YOUR_HOST:8080/v1/bootstrap/script | bash
```

### Create Bootstrap Session

```bash
curl -s -X POST http://YOUR_HOST:8080/v1/bootstrap/sessions \
  -H "Content-Type: application/json" \
  -d '{"username":"user","password":"pass"}'
```

Returns `task_id` and `status_url` on success.

### Query Task Status

```bash
curl -s http://YOUR_HOST:8080/v1/bootstrap/tasks/{taskID}
```

### Get SSH Handoff Parameters

```bash
curl -s http://YOUR_HOST:8080/v1/bootstrap/tasks/{taskID}/handoff
```

Returns `host`, `port`, `user` for SSH connection after task succeeds.

## Entry Short-link Access

### Get Entry Script

```bash
curl -sSf http://YOUR_HOST/entry/{shortId} | bash
```

### Authenticate

```bash
curl -s -X POST http://YOUR_HOST:8080/v1/entry/{shortId}/auth \
  -H "Content-Type: application/json" \
  -d '{"password":"pass"}'
```

Returns SSH params (`ssh_user`, `ssh_pass`, `ssh_host`, `ssh_port`). User connects via SSH proxy (`:2222`).

## Tasks

```bash
curl -s http://YOUR_HOST:8080/v1/admin/tasks \
  -H "Authorization: Bearer $TOKEN"
```

## Events

```bash
# Recent events
curl -s "http://YOUR_HOST:8080/v1/admin/events?limit=50" \
  -H "Authorization: Bearer $TOKEN"

# Filter by user
curl -s "http://YOUR_HOST:8080/v1/admin/events?user_id={userID}" \
  -H "Authorization: Bearer $TOKEN"

# Filter by host
curl -s "http://YOUR_HOST:8080/v1/admin/events?host_id={hostID}" \
  -H "Authorization: Bearer $TOKEN"

# Time range
curl -s "http://YOUR_HOST:8080/v1/admin/events?since=2026-03-01T00:00:00Z&until=2026-03-31T23:59:59Z" \
  -H "Authorization: Bearer $TOKEN"
```

## Dashboard

```bash
curl -s http://YOUR_HOST:8080/v1/admin/dashboard/stats \
  -H "Authorization: Bearer $TOKEN"
```

## Backup & Recovery

### Database Backup

```bash
sudo bash deploy/scripts/backup.sh
```

Defaults: directory `/var/backups/cloud-cli-proxy`, retention 7 days.

Customize:

```bash
BACKUP_DIR=/data/backups RETENTION_DAYS=30 bash deploy/scripts/backup.sh
```

### Key Rotation

**JWT Secret:**
1. Generate: `NEW_SECRET=$(head -c 48 /dev/urandom | base64 | tr -d '=+/' | head -c 48)`
2. Update `ADMIN_JWT_SECRET` in env file
3. Restart: `systemctl restart cloud-cli-proxy-control-plane`

All issued JWT tokens are immediately invalidated after rotation.
