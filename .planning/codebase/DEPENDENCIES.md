# Dependencies

**Analysis Date:** 2026-06-01

## Go Dependencies

### Core Runtime

| Package | Version | Purpose |
|---------|---------|---------|
| `modernc.org/sqlite` | latest | SQLite 驱动（纯 Go，WAL 模式） |
| `github.com/google/uuid` | latest | UUID 生成 |
| `github.com/golang-jwt/jwt/v5` | v5.3.1 | JWT 认证（HS256） |
| `github.com/spf13/cobra` | v1.10.2 | cloud-claude CLI 命令框架 |
| `golang.org/x/crypto` | v0.41.0 | SSH 客户端/服务器实现 |
| `golang.org/x/net` | v0.42.0 | 网络工具 |
| `golang.org/x/term` | v0.42.0 | 终端密码输入（隐藏回显） |

### Networking & System

| Package | Version | Purpose |
|---------|---------|---------|
| `github.com/google/nftables` | v0.3.0 | Linux nftables 防火墙规则管理 |
| `github.com/vishvananda/netlink` | v1.3.1 | Linux 网络接口/路由/namespace 操作 |
| `github.com/vishvananda/netns` | v0.0.5 | Linux 网络 namespace 切换 |
| `github.com/pkg/sftp` | v1.13.10 | SFTP 文件传输（用于 hot sync） |

### Utilities

| Package | Version | Purpose |
|---------|---------|---------|
| `al.essio.dev/pkg/shellescape` | v1.6.0 | Shell 参数转义安全 |
| `gopkg.in/yaml.v3` | v3.0.1 | YAML 配置解析 |

### Indirect Dependencies

| Package | Version | Purpose |
|---------|---------|---------|
| `github.com/mdlayher/netlink` | v1.7.3 | netlink 通信（nftables 依赖） |
| `github.com/mdlayher/socket` | v0.5.0 | 套接字抽象 |
| `golang.org/x/sync` | v0.16.0 | 同步原语 |
| `golang.org/x/sys` | v0.43.0 | 系统调用封装 |
| `golang.org/x/text` | v0.28.0 | 文本处理 |

## Frontend Dependencies (web/admin/)

### Core Framework

| Package | Version | Purpose |
|---------|---------|---------|
| `react` | ^19.2.4 | UI 框架 |
| `react-dom` | ^19.2.4 | React DOM 渲染 |
| `vite` | ^8.0.1 | 构建工具 |
| `@vitejs/plugin-react` | ^6.0.1 | Vite React 插件 |

### Routing & State

| Package | Version | Purpose |
|---------|---------|---------|
| `@tanstack/react-router` | ^1.120.0 | 文件系统路由 |
| `@tanstack/router-plugin` | ^1.120.0 | 路由代码生成 |
| `@tanstack/react-query` | ^5.75.0 | 服务端状态管理 / 数据获取 |

### UI & Styling

| Package | Version | Purpose |
|---------|---------|---------|
| `tailwindcss` | ^4.1.0 | CSS 工具类框架 |
| `@tailwindcss/vite` | ^4.1.0 | Tailwind Vite 集成 |
| `tailwind-merge` | ^3.0.0 | Tailwind 类名合并 |
| `tailwindcss-animate` | ^1.0.7 | Tailwind 动画工具 |
| `class-variance-authority` | ^0.7.0 | 组件变体管理 |
| `clsx` | ^2.1.0 | 条件类名组合 |
| `radix-ui` | ^1.4.3 | 无头 UI 组件基座 |
| `lucide-react` | ^0.510.0 | 图标库 |

### Forms & Validation

| Package | Version | Purpose |
|---------|---------|---------|
| `react-hook-form` | ^7.56.0 | 表单状态管理 |
| `@hookform/resolvers` | ^5.0.0 | 表单校验集成 |
| `zod` | ^3.24.0 | Schema 校验 |

### Notifications

| Package | Version | Purpose |
|---------|---------|---------|
| `sonner` | ^2.0.0 | Toast 通知 |

### Dev Dependencies

| Package | Version | Purpose |
|---------|---------|---------|
| `typescript` | ~5.9.3 | 类型系统 |
| `@types/react` | ^19.2.14 | React 类型定义 |
| `@types/react-dom` | ^19.2.3 | React DOM 类型定义 |
| `@types/node` | ^24.12.0 | Node.js 类型定义 |

## Documentation Dependencies

| Package | Version | Purpose |
|---------|---------|---------|
| `vitepress` | ^1.6.4 | 文档站点生成（docs/ 目录） |

## External System Dependencies

| System | Version | Purpose |
|--------|---------|---------|
| Go | 1.26.1 | 后端运行时 |
| SQLite | modernc.org/sqlite (纯 Go, WAL 模式) | 持久化数据库 |
| Docker Engine | 28.x | 容器运行时 |
| OpenSSH | 10.2p1 | 容器内 SSH Server |
| sing-box | 协议稳定 | 全隧道代理网关 |
| Node.js | 24 LTS | 前端构建工具链 |
| systemd | 宿主机稳定版 | 进程监管 |
| nftables / iptables | 宿主机稳定版 | 防火墙规则 |

---

*Dependencies analysis: 2026-06-01*
