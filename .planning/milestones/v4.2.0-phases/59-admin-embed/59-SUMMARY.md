---
phase: 59
name: Admin 前端嵌入
status: completed
execution: inline
commit: b979d6c
date: 2026-06-01
requirements_completed:
  - UI-01
  - UI-02
  - UI-03
  - UI-04
---

# Phase 59 Summary — Admin 前端嵌入

## 概述

将 Admin 前端 `web/admin/dist/*` 通过 `//go:embed` 嵌入到 control-plane Go 二进制中，实现单一二进制同时提供 API 和静态文件服务，去掉独立的前端容器或 nginx。

## 完成内容

### UI-01: //go:embed 嵌入
- 在 `internal/controlplane/http/` 新增 `spa.go`，使用 `//go:embed` 嵌入 `web/admin/dist/*`
- 构建时前端 dist 目录内容自动打包进 Go 二进制

### UI-02: SPA fallback
- 实现 `NewSPAHandler` — 非 `/v1/` 前缀路径先尝试匹配静态文件，未命中返回 `index.html`
- 自定义 `spaFileSystem` wrapper 将 `index.html` 作为 SPA fallback 兜底

### UI-03: router.go 注册
- router.go 在 API 路由之后注册静态文件 handler
- API 路由优先级高于静态文件，确保 `/v1/*` 不会被 SPA fallback 拦截

### UI-04: vite.config.ts
- 前端 dev 代理 target 从 `127.0.0.1:8090` 改为 `127.0.0.1:8080`
- 统一所有流量走单一端口

## 验证

- `go build ./cmd/control-plane` 通过，二进制包含 admin UI
- 启动 control-plane 后访问 `http://localhost:8080` 直接加载 Admin 界面
- API 路由 (`/v1/*`) 正常响应，不受静态文件 handler 影响

## 偏差

无 — inline 执行，与设计预期完全一致。
