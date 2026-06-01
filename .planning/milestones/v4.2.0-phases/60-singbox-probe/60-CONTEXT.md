---
phase: 60
name: Sing-box 探针内嵌
status: completed
---

# Phase 60 Context — Sing-box 探针内嵌

## 范围

将 sing-box 二进制内嵌至 control-plane Docker 镜像，使探针功能不再依赖独立 sing-box 容器或预拉取镜像。

## 实施决策

- **D-01**: sing-box v1.13.3 二进制通过 Dockerfile 下载并安装到 `/usr/local/bin/sing-box`
- **D-02**: 探针启动优先级：`startSingBoxNative`（宿主机二进制）→ Docker 容器回退
- **D-03**: 不在 control-plane 二进制内 go:embed sing-box（sing-box 不是 Go 库，无法嵌入）

## 相关需求

- PRB-01: Dockerfile 下载 sing-box v1.13.3 到 /usr/local/bin/
- PRB-02: 探针优先级 — 优先 native，回退 Docker

## 产出

- [60-SUMMARY.md](60-SUMMARY.md) — 执行总结
