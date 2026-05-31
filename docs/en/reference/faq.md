# Troubleshooting

## Common Issues

### Control Plane Won't Start

`systemctl status cloud-cli-proxy-control-plane` shows `failed`.

**Check:**

1. `journalctl -u cloud-cli-proxy-control-plane --no-pager -n 50`
2. `grep DATABASE_URL /etc/cloud-cli-proxy/env`
3. `systemctl status postgresql`
4. `ss -tlnp | grep 8080`

**Causes and fixes:** If the database is unreachable, check PostgreSQL status and fix the connection string. If the port is in use, stop the conflicting process or change `CONTROL_PLANE_ADDR`. If there are permission issues, verify the `cloudproxy` user has database access.

### User Can't Log In

The bootstrap script shows "authentication failed".

**Check:** Verify the control plane is running (`curl healthz`) → check user status is `active` → check expiration → check client-to-host connectivity on port 8080.

**Fix:** Start the control plane if it is down (`systemctl start`), re-enable the user via Admin API if disabled, update `expires_at` if expired.

### Host Startup Failure

Task status is `failed`.

**Check:** View task details (Admin API) → `docker info` → `docker images | grep managed-user` → `df -h /var/lib/docker` → `journalctl -u cloud-cli-proxy-host-agent`.

**Fix:** Start Docker if it is not running, rebuild the image if missing, `docker system prune` if disk is full.

### sing-box Tunnel Failure

Host cannot reach the internet or the exit IP does not match.

**Check:** `docker exec {container} ps aux | grep sing-box` → view sing-box logs → `nsenter --net=/var/run/netns/cloudproxy-{hostID} ip link show` → verify the proxy server is reachable from the host.

**Fix:** Rebuild the host if sing-box is not running, check proxy server status and firewall if unreachable, update `proxy_config` and rebuild if configuration is wrong.

### Egress IP Test Failure

The admin dashboard test shows failure. Check which specific check failed: connectivity, exit IP match, or DNS leak. Verify the proxy server is working, `ip_address` matches the actual exit IP, and the `proxy_config` address and port are correct.

### Expiry Scanner Not Triggered

User is expired but status is still `active`. Restart the control plane to trigger a scan, or manually disable the user and stop the host via Admin API.

### Database Connection Exhaustion

Logs show `too many connections`.

```bash
sudo -u postgres psql -c "SELECT count(*) FROM pg_stat_activity WHERE datname='cloudproxy'"
sudo -u postgres psql -c "SHOW max_connections"
```

Restart the control plane for a temporary fix. Increase `max_connections` for a permanent one.

### Host Agent Won't Start

`journalctl -u cloud-cli-proxy-host-agent --no-pager -n 50` → `sudo bash deploy/scripts/host-preflight.sh` → `docker info` → `which sing-box` → `ls -la /run/cloud-cli-proxy/`.

### SSH Proxy Connection Failure

User connects via entry short link but `:2222` is unreachable.

**Check:** `ss -tlnp | grep 2222` → `docker ps | grep {container}` → `docker exec {container} ss -tlnp | grep 22`.

**Fix:** Restart the control plane if the SSH proxy is not listening, start the host if the container is not running, rebuild if SSH is not running inside the container.

### KasmVNC Desktop Not Accessible

**Check:** `docker exec {container} ps aux | grep kasmvnc` → `docker exec {container} ss -tlnp | grep 6901`.

**Fix:** If KasmVNC is not running, enter the container and start it manually, or rebuild the host.

## Disaster Recovery

### Full Recovery

When the host is unavailable and you need to restore on a new machine:

1. Prepare a new host meeting prerequisites
2. Deploy services:
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
4. Rebuild managed image: `bash deploy/docker/managed-user/build-managed-image.sh`
5. Verify: `curl -s http://127.0.0.1:8080/healthz`

::: warning
After recovery, user containers must be recreated and started. User accounts, egress IPs, and bindings in the database are preserved.
:::

### Database-only Recovery

```bash
systemctl stop cloud-cli-proxy-control-plane
sudo -u postgres psql -c "DROP DATABASE cloudproxy"
sudo -u postgres psql -c "CREATE DATABASE cloudproxy OWNER cloudproxy"
pg_restore -d cloudproxy /var/backups/cloud-cli-proxy/latest.dump
systemctl start cloud-cli-proxy-control-plane
```

## Viewing Logs

```bash
# Control plane
journalctl -u cloud-cli-proxy-control-plane -f

# Host agent
journalctl -u cloud-cli-proxy-host-agent -f

# Last 100 lines
journalctl -u cloud-cli-proxy-control-plane --no-pager -n 100

# Docker Compose
docker compose logs -f control-plane
```

## FAQ

### What proxy protocols are supported?

SOCKS5, VMess, VLESS, Shadowsocks, Trojan, HTTP. Six protocols in total. Configuration follows the sing-box outbound format.

### Will user container data be lost?

Rebuilding a host preserves the home directory. Deleting a host destroys all data. Back up important data with Git.

### Can I develop on macOS / Windows?

Yes. When running `make dev`, the host agent operates in `embedded` mode.

### How do I update the user container image?

Rebuild the managed image (`make user-image`), then rebuild the hosts that need updating. Rebuilding preserves home directory data.
