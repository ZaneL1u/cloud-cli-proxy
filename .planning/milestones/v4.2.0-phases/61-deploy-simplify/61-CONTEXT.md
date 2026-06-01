---
phase: 61
name: 部署精简与配置统一
status: completed
---

# Phase 61 Context — 部署精简与配置统一

## 范围

将 Docker Compose 从 5 服务精简至 2 服务，统一端口为 :8080，移除所有 PostgreSQL 相关配置。

## 实施决策

- **D-01**: docker-compose.yml 只保留 control-plane + managed-user，移除 postgres/admin/sing-box
- **D-02**: DATABASE_URL 改为 `file:/data/cloud-cli-proxy.db`，SQLite 数据通过 volume 持久化
- **D-03**: 所有环境变量引用统一为 SQLite 路径，CONTROL_PLANE_ADDR=:8080

## 相关需求

- DEP-01 ~ DEP-05

## 产出

- [61-SUMMARY.md](61-SUMMARY.md) — 执行总结
