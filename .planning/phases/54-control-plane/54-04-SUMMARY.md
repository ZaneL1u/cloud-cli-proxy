---
phase: 54-control-plane
plan: "04"
requirements-completed: [CTRL-05]
subsystem: infra
tags: [cleanup, sing-box-gateway, build, ci, docker-compose, single-container, v4.0]

# Dependency graph
requires:
  - plan: 54-01
    provides: 删除 GatewayImage() 调用路径 + dockerRunGateway 路径 + cloudproxy-net-* bridge
provides:
  - deploy/docker/sing-box-gateway/ 目录整目录删除
  - Makefile gateway-image target / GATEWAY_IMAGE 变量 / dev target 非 Linux 检查清理
  - CI build-images.yml matrix sing-box-gateway 条目删除
  - docker-compose 文件 sing-box-gateway service + CLOUD_CLI_PROXY_GATEWAY_IMAGE env 删除
  - uat-bypass-fixture-up.sh GATEWAY_IMAGE/GATEWAY_CONTAINER 引用清理
affects: [55, 56]

# Tech tracking
language: go / makefile / shell / yaml
files_changed:
  - source: deploy/docker/sing-box-gateway/Dockerfile
    state: deleted
  - source: deploy/docker/sing-box-gateway/entrypoint.sh
    state: deleted
  - source: Makefile
    state: modified
    changes:
      - .PHONY 行删除 gateway-image
      - dev target 删除非 Linux gateway 镜像存在性检查（5 行）
      - 删除 GATEWAY_IMAGE 变量
      - 删除 gateway-image: target（3 行）
      - 删除 help 文本 gateway-image 行
  - source: .github/workflows/build-images.yml
    state: modified
    changes:
      - matrix.include 删除 sing-box-gateway 条目（3 行）
  - source: docker-compose.yml
    state: modified
    changes:
      - control-plane 删除 CLOUD_CLI_PROXY_GATEWAY_IMAGE 环境变量
      - 删除 sing-box-gateway service（5 行）
  - source: docker-compose.build.yaml
    state: modified
    changes:
      - 删除 sing-box-gateway service（8 行）
  - source: scripts/uat-bypass-fixture-up.sh
    state: modified
    changes:
      - Step 10 标题改为 "mock worker 容器"（删 gateway 部分）
      - 删除 GATEWAY_IMAGE / GATEWAY_CONTAINER 变量
      - 删除 gateway 容器创建逻辑（18 行）
      - fixture JSON 删除 gateway_container 字段
      - .uat-fixture.env 删除 UAT_BYPASS_GATEWAY_CONTAINER 导出
      - GITHUB_OUTPUT 删除 gateway_container 行
      - 总结输出删除 gateway_container 行

# Completed tasks
- [x] T1 — Makefile 清理
- [x] T2 — deploy/docker/sing-box-gateway/ 整目录删除
- [x] T3 — CI build-images.yml matrix 清理
- [x] T4 — docker-compose.yml + docker-compose.build.yaml 清理
- [x] T5 — uat-bypass-fixture-up.sh gateway 引用收敛
- [x] T6 — 全仓库 grep 兜底验证（0 命中）

# Dev notes
- go build ./... 通过
- go test ./internal/network/ 全部 PASS
- 全仓库 grep sing-box-gateway/GATEWAY_IMAGE/gateway-image/GatewayImage 0 命中

# Self-Check: PASSED
- [x] deploy/docker/sing-box-gateway/ 目录不存在
- [x] Makefile 不含 gateway-image / GATEWAY_IMAGE
- [x] CI workflows 不含 sing-box-gateway
- [x] docker-compose*.yml* 不含 sing-box-gateway
- [x] container_proxy_provider.go 不含 GatewayImage（54-01 已删）
- [x] uat-bypass-fixture-up.sh 不含 GATEWAY_CONTAINER/GATEWAY_IMAGE
- [x] 构建通过，测试通过
