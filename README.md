<div align="center">

<img src="docs/public/logo.svg" width="88" height="88" alt="Cloud CLI Proxy" />

# Cloud CLI Proxy

**一个更聪明的 Claude Code Wrapper。把 Claude Code 装进容器，让你看起来像个地地道道的美国开发者。**

[![CI](https://github.com/ZaneL1u/cloud-cli-proxy/actions/workflows/ci.yml/badge.svg)](https://github.com/ZaneL1u/cloud-cli-proxy/actions/workflows/ci.yml)
[![Images](https://github.com/ZaneL1u/cloud-cli-proxy/actions/workflows/build-images.yml/badge.svg)](https://github.com/ZaneL1u/cloud-cli-proxy/actions/workflows/build-images.yml)
[![Release](https://img.shields.io/github/v/release/ZaneL1u/cloud-cli-proxy)](https://github.com/ZaneL1u/cloud-cli-proxy/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

[English](README.en.md) | [文档](https://zanel1u.github.io/cloud-cli-proxy/)

</div>

---

## 解决什么问题？

Claude Code 好用，但风控越来越严。IP 不对、环境特征像 VPS、遥测数据出卖你——封号只是时间问题。

Cloud CLI Proxy 做的事情很简单：**把你的 Claude Code 包装成一个坐在洛杉矶家里用 Windows 电脑的普通美国人。** 从 IP 到系统指纹到 TLS 握手，每一层都做了伪装。Anthropic 看到的就是一个正常的美国住宅用户，不是什么奇奇怪怪的云服务。

部署在你自己的机器上，SSH 进去就直接写代码。本地的项目目录会自动挂载到容器里，路径一模一样——你用 Claude Code 的感觉跟本地跑没有任何区别。

---

## 它怎么做到的？

### 身份伪装

这套伪装不是装个代理就完事的。Claude Code 会从多个维度判断你在哪、用什么机器：

- **系统指纹替换** — CPU 型号伪装成 AMD EPYC，MAC 地址、machine-id 全部重写。`ioreg`、`system_profiler`、`sysctl` 这些命令的输出也被拦截篡改过，Claude Code 读到的是我们想让它看到的
- **Windows 主机名** — 自动生成 `DESKTOP-XXXXXXX` 或 `LAPTOP-XXXXXXX`，就是普通人家里的电脑命名
- **容器痕迹擦除** — `/.dockerenv` 删了，cgroup 里的 docker/containerd 字符串过滤掉，常规的容器检测手段扫不出来
- **时区与语言** — 默认太平洋时区 + `en_US.UTF-8`，创建容器时可以改成别的
- **TLS 指纹** — 出口流量走 uTLS，指纹设成 Chrome 浏览器，跟普通人上网的 TLS 握手特征没区别
- **遥测拦截** — DNS 级别屏蔽 `statsig.anthropic.com`、`sentry.io`、`cdn.growthbook.io`，Claude Code 没法偷偷上报

### IP 严格隔离

每个容器绑定独立的出口 IP，所有流量——HTTP、DNS、WebRTC，全部走 sing-box tun 隧道从指定 IP 出去。nftables 默认拒绝直连，不存在漏网之鱼。

支持 SOCKS5、VMess、VLESS、Shadowsocks、Trojan、HTTP 六种协议。

**关键是你得有自己的家宽 IP。** 机房的 IP 段早就被标记烂了。推荐 AT&T 家宽，干净，风控通过率高。我一直在用这家：

👉 [VIRCS 房产公司自有公寓 提供真实住宅宽带 - 加州合法注册的房产公司 **所有 IP 均为真实住宅宽带**。](https://www.vircs.com/welcome?vcd=70685425)

### 代码映射——跟本地一模一样

这是跟普通 VPS 最大的区别。容器通过 sshfs 把你本地的项目目录挂载进去，**路径完全一致**。

什么意思？你在本地 `~/my-project` 里写代码，容器里也是 `~/my-project`。Claude Code 读到的路径跟你本地没有丝毫差别，就像在本机跑一样。挂载支持三种模式：全量同步、智能热同步、纯挂载。断线了自动重连，30 秒内恢复，输入不丢。tmux 多端会话，断开也不丢工作区。

### 开箱即用

管理员在后台把容器建好，用户拿到 `curl` 命令，终端里跑一下，输密码，等容器启动，自动 SSH 进去。里面 Claude Code 已经装好，直接 `claude` 就能用。

---

## 快速开始

```bash
git clone https://github.com/ZaneL1u/cloud-cli-proxy.git
cd cloud-cli-proxy

bash deploy/scripts/setup-env.sh

docker compose pull
docker compose up -d

curl http://127.0.0.1:8080/healthz
# {"status":"ok"}
```

启动后：

- 管理后台：`http://YOUR_HOST:8080`
- API：`http://YOUR_HOST:8080`
- SSH 代理：`YOUR_HOST:2222`

首次使用：登录管理后台 → 添加出口 IP → 创建用户 → 创建主机 → 把接入命令发给用户。

---

## 安装

### 环境要求

- Docker Engine 28.x+
- Docker Compose v2


### Docker Compose（推荐）

```bash
bash deploy/scripts/setup-env.sh  # 交互式生成密码和密钥
docker compose pull               # 拉取预构建镜像
docker compose up -d              # 启动
```

### 宿主机直接部署

```bash
sudo bash deploy/scripts/deploy.sh
```

创建 `cloudproxy` 系统用户，构建二进制和镜像，安装 systemd 单元并启动。

### 环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `DATABASE_URL` | SQLite 数据库路径 | `file:/data/cloud-cli-proxy.db` |
| `ADMIN_USERNAME` | 管理员用户名 | `admin` |
| `ADMIN_PASSWORD` | 管理员密码（bcrypt） | 必填 |
| `ADMIN_JWT_SECRET` | JWT 签名密钥 | 必填 |
| `SSH_PROXY_PORT` | SSH 代理端口 | `2222` |
| `LOG_FORMAT` | 日志格式 `json` / `text` | `json` |
| `LOG_LEVEL` | 日志级别 | `info` |

---

---

## 管理后台

除了伪装和 IP 隔离，平台还提供完整的管理能力：

- 用户创建、暂停、过期自动停机、密码轮换
- 主机创建、启动、停止、重建（保留或清空 /workspace）、删除
- 出口 IP 增删改查、连通性测试
- Bypass 防火墙——按域名、CIDR、端口白名单直连，快照版本管理，预览→应用→回滚
- 所有操作写入审计事件，可追溯
- SSE 实时推送任务进度和主机状态
- 容器内置 KasmVNC + Chromium，管理后台一键打开远程桌面

---

## 架构

```
                                                    ┌───────────────────────────────────┐
用户 ──curl──> Control Plane (:8080) ──Docker──>    │ 用户容器                          │
                    │                                │  SSH + Claude Code + VNC          │
                   SQLite                             │  sshfs ← 本地 CWD 同名路径映射    │
                    │                                │  sing-box tun 隧道                │
              Admin UI (embed)                   │       ↓                           │
                    │                                │  指定出口 IP                      │
              SSH Proxy (:2222)                      └───────────────────────────────────┘
```

| 组件 | 说明 |
|------|------|
| **Control Plane** | Go API，认证、用户管理、任务编排、SSH 代理 |
| **Host Agent** | 特权代理，管理 Docker 容器、网络命名空间和隧道 |
| **用户容器** | Ubuntu 24.04，预装 OpenSSH + Claude Code + sshfs + KasmVNC + Chromium |
| **SQLite** | 持久化用户、主机、出口 IP、任务、事件、审计日志（WAL 模式） |
| **Admin SPA** | React 19 + TypeScript + Vite + Tailwind CSS |

---

## 参与贡献

Bug 报告和功能建议请提交 [Issue](https://github.com/ZaneL1u/cloud-cli-proxy/issues)。

Pull Request 流程：

1. Fork 仓库，从 `main` 分支创建 feature 分支
2. 修改代码，确保 `make test` 通过
3. 提交 PR，描述改了什么、为什么改

本地开发环境搭建：

```bash
make setup    # 安装依赖
make dev      # 后端 + 前端热重载（API :8080，前端 localhost:2568）
make test     # 运行测试
```

更多命令见 `make help`。

---

## 文档

完整文档见 [GitHub Pages](https://zanel1u.github.io/cloud-cli-proxy/)：快速开始、部署指南、配置参考、架构说明、API 参考、故障排查。

---

## 许可证

[MIT](LICENSE)
