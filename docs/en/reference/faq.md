# FAQ & Troubleshooting

## Common Issues

### Control Plane Won't Start

**Symptoms:** `systemctl status cloud-cli-proxy-control-plane` shows `failed`, service keeps restarting.

**Check:**

1. View logs: `journalctl -u cloud-cli-proxy-control-plane --no-pager -n 50`
2. Verify `DATABASE_URL`: `grep DATABASE_URL /etc/cloud-cli-proxy/env`
3. Check PostgreSQL: `systemctl status postgresql`
4. Check port conflict: `ss -tlnp | grep 8080`
5. Check JWT secret: `grep ADMIN_JWT_SECRET /etc/cloud-cli-proxy/env`

**Fix:**
- DB connection failure → Fix `DATABASE_URL` or start PostgreSQL
- Port conflict → Stop conflicting process or change `CONTROL_PLANE_ADDR`
- Permissions → Verify `cloudproxy` user has DB access

### User Can't Log In

**Symptoms:** bootstrap script shows "connection failed" or "auth failed".

**Check:**

1. Control plane running: `curl -s http://127.0.0.1:8080/healthz`
2. User status is `active`
3. User not expired
4. Network connectivity (client to host port 8080)

**Fix:**
- Control plane down → `systemctl start cloud-cli-proxy-control-plane`
- User disabled → Re-enable via Admin API
- User expired → Update expiry and set status to `active`

### Host Startup Failure

**Symptoms:** bootstrap shows "startup failed", task status is `failed`.

**Check:**

1. Task details via Admin API
2. Docker daemon: `docker info`
3. Managed image exists: `docker images | grep managed-user`
4. Disk space: `df -h /var/lib/docker`
5. Host-agent logs: `journalctl -u cloud-cli-proxy-host-agent --no-pager -n 50`

**Fix:**
- Docker not running → `systemctl start docker`
- Image missing → `bash deploy/docker/managed-user/build-managed-image.sh`
- Disk full → `docker system prune`

### sing-box Proxy Tunnel Failure

**Symptoms:** Hosts using proxy-type egress IPs can't access the internet or exit IP doesn't match.

**Check:**

1. sing-box process in container: `docker exec {container} ps aux | grep sing-box`
2. sing-box logs: `docker exec {container} cat /var/log/sing-box.log`
3. tun device: `nsenter --net=/var/run/netns/cloudproxy-{hostID} ip link show`
4. Routing table: `nsenter --net=/var/run/netns/cloudproxy-{hostID} ip route show`
5. Proxy server reachability from host

**Fix:**
- sing-box not running → Rebuild host
- Proxy server unreachable → Check proxy server status and firewall
- Config error → Update `proxy_config` via Admin API, then rebuild host
- tun device missing → Restart host-agent and rebuild host

### Proxy Test Failure

**Symptoms:** Egress IP test in admin dashboard shows failure.

**Check:**

1. Review test result details (which check failed: connectivity / IP match / DNS leak)
2. Connectivity failure → Proxy server may be down or port unreachable
3. IP mismatch → Proxy server's actual exit IP differs from declared `ip_address`
4. DNS leak → Proxy config may not properly cover DNS requests

**Fix:**
- Verify proxy server is running
- Ensure `ip_address` matches the proxy server's actual exit IP
- Check server address and port in `proxy_config`

### Expiry Scanner Not Running

**Fix:**
- Control plane down → Start it
- Immediate effect needed → `systemctl restart cloud-cli-proxy-control-plane`
- Manual override → Disable user and stop host via Admin API

### Database Connection Exhaustion

**Symptoms:** Logs show `too many connections`.

**Check:**

```bash
sudo -u postgres psql -c "SELECT count(*) FROM pg_stat_activity WHERE datname='cloudproxy'"
sudo -u postgres psql -c "SHOW max_connections"
```

**Fix:**
- Temporary → Restart control plane
- Permanent → Increase `max_connections` in `postgresql.conf`

### Host Agent Won't Start

**Check:**

1. View logs: `journalctl -u cloud-cli-proxy-host-agent --no-pager -n 50`
2. Run preflight: `sudo bash deploy/scripts/host-preflight.sh`
3. Check Docker: `docker info`
4. Check sing-box binary: `which sing-box`
5. Check socket directory: `ls -la /run/cloud-cli-proxy/`

### SSH Proxy Connection Failure

**Symptoms:** User connects via entry short-link but SSH to `:2222` fails.

**Check:**

1. SSH proxy port listening: `ss -tlnp | grep 2222`
2. Container running: `docker ps | grep {container_name}`
3. SSH inside container: `docker exec {container} ss -tlnp | grep 22`
4. Check control plane logs for SSH proxy errors

**Fix:**
- SSH proxy not listening → Restart control plane
- Container not running → Start host via Admin API
- SSH not started in container → Rebuild host

### KasmVNC Desktop Not Accessible

**Symptoms:** VNC link in admin or user panel doesn't respond.

**Check:**

1. KasmVNC process: `docker exec {container} ps aux | grep kasmvnc`
2. Port 6901 in container: `docker exec {container} ss -tlnp | grep 6901`
3. Control plane VNC reverse proxy logs

**Fix:**
- KasmVNC not started → Enter container and start manually, or rebuild host
- Network issue → Verify control plane can reach container's management network IP

## Disaster Recovery

### Full Recovery

For when the host is completely unavailable and you need to restore on a new machine:

1. Prepare new host meeting prerequisites
2. Deploy:
   ```bash
   git clone https://github.com/ZaneL1u/cloud-cli-proxy.git /opt/cloud-cli-proxy
   cd /opt/cloud-cli-proxy
   sudo bash deploy/scripts/deploy.sh
   ```
3. Restore database:
   ```bash
   systemctl stop cloud-cli-proxy-control-plane
   pg_restore --clean -d cloudproxy /path/to/backup.dump
   systemctl start cloud-cli-proxy-control-plane
   ```
4. Rebuild image: `bash deploy/docker/managed-user/build-managed-image.sh`
5. Verify: `curl -s http://127.0.0.1:8080/healthz`

::: warning
After recovery, all user containers need to be recreated and started. User accounts, egress IPs, and bindings from the database are preserved.
:::

### Database-only Recovery

```bash
systemctl stop cloud-cli-proxy-control-plane
sudo -u postgres psql -c "DROP DATABASE cloudproxy"
sudo -u postgres psql -c "CREATE DATABASE cloudproxy OWNER cloudproxy"
pg_restore -d cloudproxy /var/backups/cloud-cli-proxy/latest-backup.dump
systemctl start cloud-cli-proxy-control-plane
```

## Logs

```bash
# Control plane (follow)
journalctl -u cloud-cli-proxy-control-plane -f

# Host-agent (follow)
journalctl -u cloud-cli-proxy-host-agent -f

# Last N lines
journalctl -u cloud-cli-proxy-control-plane --no-pager -n 100

# Time range
journalctl -u cloud-cli-proxy-control-plane --since "2026-03-27 00:00:00" --until "2026-03-27 23:59:59"

# Docker Compose deployment
docker compose logs -f control-plane
docker compose logs -f admin
```

## FAQ

### Q: What proxy protocols are supported?

A: Six protocols: SOCKS5, VMess, VLESS, Shadowsocks, Trojan, and HTTP Proxy. Configuration follows the sing-box outbound format.

### Q: Will user container data be lost?

A: Rebuilding a host preserves the home directory. Deleting a host destroys all data. Back up important data using Git or similar tools.

### Q: Can I develop on macOS / Windows?

A: Yes. When using `make dev`, host-agent runs in `embedded` mode. No additional build steps required.

### Q: How do I update the user container image?

A: Rebuild the managed image (`make user-image`), then rebuild the hosts that need updating. Rebuilding preserves home directory data.
