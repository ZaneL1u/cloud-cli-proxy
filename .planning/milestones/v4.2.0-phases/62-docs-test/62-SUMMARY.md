---
phase: 62
name: 文档同步与测试适配
status: completed
execution: inline
commit: 94626f2
date: 2026-06-01
requirements_completed:
  - DOC-01
  - DOC-02
  - DOC-03
  - TEST-01
  - TEST-02
---

# Phase 62 Summary — 文档同步与测试适配

## 概述

将项目文档全面更新以反映 SQLite 迁移和服务精简，同时将单元测试从 testcontainers PostgreSQL 切换到内存 SQLite。

## 完成内容

### DOC-01: README 更新
- `README.md` / `README.en.md` 更新架构图（移除 PostgreSQL/admin/sing-box）
- 环境变量表更新（移除 POSTGRES_*，DATABASE_URL 改为 SQLite 路径）
- 访问地址更新为统一端口 :8080

### DOC-02: docs 目录更新
- `docs/zh/guide/` 和 `docs/en/guide/` 更新 architecture、deployment、quickstart、configuration
- 架构描述从 5 服务改为 2 服务
- 部署步骤移除 PostgreSQL 安装要求

### DOC-03: FAQ 更新
- `docs/zh/reference/faq.md` 和 `docs/en/reference/faq.md`
- PG 相关排障条目（如连接失败、迁移失败）替换为 SQLite 对应条目

### TEST-01: repository 测试适配
- `internal/store/repository/*_test.go` 从 testcontainers PG 切换到内存 SQLite
- 测试启动时自动创建 SQLite 内存数据库并运行迁移

### TEST-02: app 测试适配
- `internal/controlplane/app/app_test.go` 更新配置常量
- 确认编译和 `go test ./internal/...` 全部通过

## 验证

- `go test ./internal/...` — ALL PASS (16 packages)
- `go vet ./internal/...` — PASS
- 文档站点 `make docs-dev` 本地预览正常

## 偏差

无 — inline 执行，与设计预期完全一致。
