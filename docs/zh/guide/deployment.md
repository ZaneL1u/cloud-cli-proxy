# 部署指南

推荐使用 Docker Compose 一键部署，无需手动安装 PostgreSQL、Go 或编译源码。

## 1. 安装 Docker

### Linux

```bash
curl -fsSL https://get.docker.com | sh
```

安装后确保当前用户在 `docker` 组：

```bash
sudo usermod -aG docker $USER
# 退出重新登录生效
```

### macOS

安装 [Docker Desktop](https://www.docker.com/products/docker-desktop/)，或通过 Homebrew：

```bash
brew install --cask docker
```

### Windows

安装 [Docker Desktop](https://www.docker.com/products/docker-desktop/)，启用 WSL 2 后端。

## 2. 启动

```bash
git clone https://github.com/ZaneL1u/cloud-cli-proxy.git
cd cloud-cli-proxy

bash deploy/scripts/setup-env.sh
docker compose pull
docker compose up -d
```

`setup-env.sh` 交互式生成所有密码和密钥。支持：

- **内置 PostgreSQL（推荐）**：自动创建，Docker Compose 统一管理，零配置
- **外部 PostgreSQL**：填入已有数据库的连接信息

启动后：

  - 管理后台（内嵌）：`http://YOUR_HOST:8080`
- API：`http://YOUR_HOST:8080`

验证：

```bash
curl http://127.0.0.1:8080/healthz
# {"status":"ok"}
```

## 中国大陆用户

中国大陆访问 `ghcr.io` 可能较慢或不可达。`setup-env.sh` 会自动检测连通性，如果不可达会提示你是否切换到 `ghcr.1ms.run`（毫秒镜像），同意后自动写入 `.env`。直接 `docker compose pull && docker compose up -d` 即可。

如果已经生成了 `.env`，手动加上一行：

```bash
CONTAINER_REGISTRY=ghcr.1ms.run
```

这个变量同时控制 compose 镜像拉取和运行时 `docker pull`（`managed-user` 更新、`sing-box` 探针），所有 `ghcr.io` 引用都会自动替换。

systemd 裸机部署同理，在 `/etc/cloud-cli-proxy/env` 中加上这一行后重启控制面。

## 环境变量

使用 `setup-env.sh` 生成后通常不需要手动修改。完整列表见 [配置参考](./configuration)。

## 源码构建

预构建镜像不可用时才需要本地构建：

```bash
docker compose -f docker-compose.yml -f docker-compose.build.yaml --profile build-only build --no-cache
docker compose -f docker-compose.yml -f docker-compose.build.yaml up -d --force-recreate
```

## 宿主机直接部署

对于需要裸机 systemd 部署的场景，提供自动化脚本：

```bash
sudo bash deploy/scripts/deploy.sh
```

会自动完成：创建系统用户 → 构建二进制和镜像 → 生成配置 → 安装 systemd 服务 → 启动。详细步骤见仓库内的 `deploy/scripts/deploy.sh`。
