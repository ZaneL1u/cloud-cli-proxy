---
phase: 29-v3-worker
plan: 01-image-base
subsystem: infra
tags: [docker, buildkit, tini, managed-user]

key-files:
  created: []
  modified:
    - deploy/docker/managed-user/Dockerfile

requirements-completed:
  - BASE-04
  - C5
  - C7
  - M17
  - M18

duration: "~25min"
completed: 2026-04-18
---

# Phase 29 Plan 01-image-base Summary

在受管用户镜像 Dockerfile 中落地 BuildKit 语法与 apt 双 cache mount、合并安装 `tini`、预建 v3 挂载点与 `1000:1000` 属主、并将 `ENTRYPOINT` 改为经 `tini` 的 exec 形式，为后续 Plan 02/03 的二进制与 entrypoint 改造打好镜像基线。

## Task Commits

1. **Task 1.1** — `2f6c041` — BuildKit syntax + docker-clean / keep-cache
2. **Task 1.2** — `3dba2e0` — apt cache mount RUN + `tini`
3. **Task 1.3** — `7b63223` — v3 目录预建与 chown
4. **Task 1.4** — `3d9d409` — ENTRYPOINT → `tini` exec form

## Verification

- **静态**：`# syntax=docker/dockerfile:1.7`、`docker-clean` 移除、`--mount=type=cache`（apt + lists）、`tini` 包、`/home/claude` 等 `mkdir`、`chown -R "${WORKSPACE_UID}:${WORKSPACE_GID}"`、以及 `"/usr/bin/tini", "--", "/usr/local/bin/entrypoint.sh"` 均在 `deploy/docker/managed-user/Dockerfile` 中可核对。
- **`docker build`**：已按计划在本地启动 `DOCKER_BUILDKIT=1 docker build -f deploy/docker/managed-user/Dockerfile -t local/managed-user:p01-test .`；构建在拉取 `docker/dockerfile:1.7` 前端镜像阶段耗时过长，会话内未等到完成即中止。**请在网络正常的环境补跑同一命令**以完全闭合计划中的 build 断言。

## Deviations from Plan

- **Docker 完整构建**：未在会话内得到成功/失败结论；与镜像层拉取速度相关，非 Dockerfile 编辑错误。补跑通过后本偏差可视为消除。

## Self-Check: PASSED

实现与计划四段任务及原子提交策略一致；剩余风险仅为上述 **`docker build` 未在本机跑完**，由运维/CI 补验证即可。
