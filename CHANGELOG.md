# Changelog

All notable changes to this project are documented in this file.

<!-- release-entries -->

## v1.6.10 - 2026-04-10
## What's Changed

### Backend (Go / API)
- fix(ssh): generate valid outbound keys (0daecbd)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.6.9...v1.6.10


## v1.6.9 - 2026-04-08
## What's Changed

### Runtime & Deployment
- fix(runtime): remove tmux wrapper from claude (da09ba9)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.6.8...v1.6.9


## v1.6.8 - 2026-04-08
## What's Changed

### Backend (Go / API)
- fix(ssh): 代理连容器认证顺序、create 后同步凭据与稳定解析容器 IP (05f763b)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.6.7...v1.6.8


## v1.6.7 - 2026-04-05
## What's Changed

### Backend (Go / API)
- fix: curl入口变量遮蔽bug + 增强日志 + SSH快捷复制按钮 (ef07560)

### Frontend (Admin Web)
- fix: curl入口变量遮蔽bug + 增强日志 + SSH快捷复制按钮 (ef07560)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.6.6...v1.6.7


## v1.6.6 - 2026-04-05
## What's Changed

### Backend (Go / API)
- fix: curl 入口改用主机 short_id 替代用户 short_id (82a9ef6)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.6.5...v1.6.6


## v1.6.5 - 2026-04-05
## What's Changed

### Backend (Go / API)
- feat: SSH代理用自己的密钥连容器，不再依赖密码 (069f2ac)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.6.4...v1.6.5


## v1.6.4 - 2026-04-05
## What's Changed

### Backend (Go / API)
- feat: SSH代理支持公钥认证（入站密钥免密登录） (4169f85)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.6.3...v1.6.4


## v1.6.3 - 2026-04-05
## What's Changed

### Backend (Go / API)
- feat: SSH密钥与容器双向实时同步 (5eb1477)

### Frontend (Admin Web)
- feat: SSH密钥与容器双向实时同步 (5eb1477)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.6.2...v1.6.3


## v1.6.2 - 2026-04-05
## What's Changed

### Backend (Go / API)
- fix: SSH密钥API响应格式与前端对齐 (195aefe)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.6.1...v1.6.2


## v1.6.1 - 2026-04-05
## What's Changed

### Runtime & Deployment
- fix(ci): 镜像构建仅由打 tag 触发，移除 push to main 触发 (b3353d0)
- fix: control-plane 镜像缺少 migrations 目录导致数据库迁移不执行 (4f33cd5)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.6.0...v1.6.1


## v1.6.0 - 2026-04-05
## What's Changed

### Backend (Go / API)
- feat(260405-qk2): SSH密钥体系改造：拆分为入站免密登录和出站外部鉴权 (8b994dc)

### Frontend (Admin Web)
- feat(260405-qk2): SSH密钥体系改造：拆分为入站免密登录和出站外部鉴权 (8b994dc)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.5.1...v1.6.0


## v1.5.1 - 2026-04-05
## What's Changed

### Backend (Go / API)
- feat(260405-jji): 镜像版本管理：自动拉取最新镜像 + 版本展示 + 一键升级 (507b288)

### Frontend (Admin Web)
- feat(260405-jji): 镜像版本管理：自动拉取最新镜像 + 版本展示 + 一键升级 (507b288)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.5.0...v1.5.1


## v1.5.0 - 2026-04-05
## What's Changed

### Backend (Go / API)
- feat(260405-hio): structured Claude Code settings panel and system fingerprint (bea7886)
- feat(260405-hai): enhance Claude status API with per-process details (2ae9592)
- feat(260405-h13): add 5 container management API endpoints (e8de4a8)

### Frontend (Admin Web)
- feat(260405-hio): structured Claude Code settings panel and system fingerprint (bea7886)
- feat(260405-hai): enhance Claude status API with per-process details (2ae9592)
- feat(260405-h13): add frontend hooks, dialogs and Claude status card (6affd42)

### Docs
- docs: add UI screenshots and local development setup guide (bae69a5)
- docs: refine hero glow to vitest-like large halo (6720691)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.4.6...v1.5.0


## v1.4.6 - 2026-04-02
## What's Changed

### Runtime & Deployment
- fix(ci): lowercase ghcr image prefix for registry cache (6f420d8)

### Docs
- docs: add hero logo and glow effect on homepage (cd689f5)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.4.5...v1.4.6


## v1.4.5 - 2026-04-02
## What's Changed

### Runtime & Deployment
- perf(ci): improve docker layer cache hit rate (6e410d4)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.4.4...v1.4.5


## v1.4.4 - 2026-04-02
## What's Changed

### Runtime & Deployment
- fix(ci): source pnpm version from root package manager (6cce3f8)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.4.3...v1.4.4

