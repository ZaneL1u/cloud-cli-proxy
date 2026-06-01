---
phase: 61
name: 部署精简与配置统一
status: completed
execution: inline
commit: 41cd188
date: 2026-06-01
requirements_completed:
  - DEP-01
  - DEP-02
  - DEP-03
  - DEP-04
  - DEP-05
---

# Phase 61 Summary — 部署精简与配置统一

## 概述

将 Docker Compose 从 5 服务精简至 2 服务，统一端口和环境变量，移除所有 PostgreSQL 相关配置和脚本引用。

## 完成内容

### DEP-01: docker-compose.yml 精简
- 移除 postgres、admin、sing-box 三个服务定义
- 移除 `cloudproxy-postgres` volume
- 最终保留 2 个服务：control-plane + managed-user

### DEP-02: control-plane SQLite 数据卷
- `DATABASE_URL` 改为 `file:/data/cloud-cli-proxy.db`
- 新增 `./data:/data` volume 映射确保 SQLite 数据持久化

### DEP-03: 环境变量清理
- `.env` 和 `.env.example` 移除所有 `POSTGRES_*` 变量
- `DATABASE_URL` 改为 SQLite 文件路径
- `CONTROL_PLANE_ADDR` 统一为 `:8080`

### DEP-04: Makefile 清理
- 移除 `db`、`db-stop`、`db-reset` 目标
- `dev` 目标不再检测 PostgreSQL 运行状态

### DEP-05: deploy 脚本清理
- `deploy/scripts/setup-env.sh` 移除 PostgreSQL 交互式配置
- `deploy/scripts/deploy.sh` 移除 psql 依赖和数据库检查步骤

## 验证

- `docker compose up` 只启动 2 个服务
- control-plane 启动后 SQLite 数据库文件在 `./data/` 目录正确创建
- 前端和管理 API 均通过 `:8080` 访问
- Makefile `make dev` 不再检查 PostgreSQL

## 偏差

无 — inline 执行，与设计预期完全一致。
