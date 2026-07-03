# Deployment Guide

Docker Compose is the recommended way to deploy. No need to install a database, Go, or compile from source. SQLite is embedded.

## 1. Install Docker

### Linux

```bash
curl -fsSL https://get.docker.com | sh
```

Add your user to the `docker` group:

```bash
sudo usermod -aG docker $USER
# Log out and back in for it to take effect
```

### macOS

Install [Docker Desktop](https://www.docker.com/products/docker-desktop/), or via Homebrew:

```bash
brew install --cask docker
```

### Windows

Install [Docker Desktop](https://www.docker.com/products/docker-desktop/) with WSL 2 backend enabled.

## 2. Start

```bash
git clone https://github.com/ZaneL1u/cloud-cli-proxy.git
cd cloud-cli-proxy

bash deploy/scripts/setup-env.sh
docker compose pull
docker compose up -d
```

`setup-env.sh` interactively generates all passwords and secrets. It supports:

- SQLite single-file database, Docker Compose managed `/data` persistence, zero config

After startup:

- Admin dashboard: `http://YOUR_HOST:8080`
- API: `http://YOUR_HOST:8080`

The admin dashboard is embedded in the control-plane service on current
releases. If you are running an older `v3.4.x` Compose file with a standalone
`admin` service and port `3000` is unavailable on the host, set `ADMIN_PORT` in
`.env` to another free port and recreate the service.

Verify:

```bash
curl http://127.0.0.1:8080/healthz
# {"status":"ok"}
```

## Users in Mainland China

`ghcr.io` may be slow or unreachable from within mainland China. `setup-env.sh` auto-detects connectivity and prompts whether to switch to `ghcr.1ms.run` mirror. Accept and it writes the setting to `.env`. Then just `docker compose pull && docker compose up -d`.

If you already generated `.env`, add this line manually:

```bash
CONTAINER_REGISTRY=ghcr.1ms.run
```

This variable controls both compose image pulls and runtime `docker pull` operations (`managed-user` updates, `sing-box` probes). Every `ghcr.io` reference is replaced.

For bare-metal systemd deployments, add the same line to `/etc/cloud-cli-proxy/env` and restart the control plane.

## Environment Variables

After running `setup-env.sh`, manual changes are usually unnecessary. See [Configuration](./configuration) for the full reference.

## Building from Source

Only needed when prebuilt images are unavailable:

```bash
docker compose -f docker-compose.yml -f docker-compose.build.yaml --profile build-only build --no-cache
docker compose -f docker-compose.yml -f docker-compose.build.yaml up -d --force-recreate
```

## Bare-metal Deployment

For scenarios that require native systemd deployment:

```bash
sudo bash deploy/scripts/deploy.sh
```

This automates: creating the system user → building binaries and images → generating config → installing systemd units → starting services. See `deploy/scripts/deploy.sh` in the repo for details.
