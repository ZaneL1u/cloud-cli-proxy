# Changelog

All notable changes to this project are documented in this file.

<!-- release-entries -->

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

