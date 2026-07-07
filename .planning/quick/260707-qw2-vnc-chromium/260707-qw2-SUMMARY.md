---
status: complete
phase: quick-260707-qw2
subsystem: vnc, controlplane, web-admin, managed-user-image
tags: [vnc, kasmvnc, chromium, auto-recovery, admin-ui]
---

# Quick Task 260707-qw2: VNC 状态展示、启动重启控制、自动拉起与 Chromium 窗口适配 Summary

## 结果

- 后端新增 VNC 状态与控制接口：
  - `GET /v1/admin/hosts/{hostID}/vnc/status`
  - `POST /v1/admin/hosts/{hostID}/vnc/start`
  - `POST /v1/admin/hosts/{hostID}/vnc/restart`
  - `GET /v1/user/hosts/{hostID}/vnc/status`
  - `POST /v1/user/hosts/{hostID}/vnc/start`
  - `POST /v1/user/hosts/{hostID}/vnc/restart`
- 容器镜像新增 `vnc-status`，`restart-vnc` 支持 `start|restart`。
- `entrypoint.sh` 增加 VNC watchdog：VNC 关闭后自动拉起；30s 窗口内最多自动拉起 3 次；稳定运行后重置预算；手动启动/重启会重置暂停状态。
- Chromium 默认窗口改为 `1880,1000`，保留 `--start-maximized`，并允许用 `CHROMIUM_WINDOW_SIZE` 覆盖。
- 管理员主机详情页和用户门户主机详情页都显示 VNC 状态、自动拉起暂停提示、启动/重启操作。
- 中英文 API 文档更新 VNC 状态与控制端点。

## 验证

- `go test ./internal/controlplane/http` 通过。
- `bash -n deploy/docker/managed-user/entrypoint.sh && bash -n deploy/docker/managed-user/restart-vnc.sh && bash -n deploy/docker/managed-user/vnc-status.sh && bash -n deploy/docker/managed-user/launch-chromium.sh` 通过。
- `pnpm --dir web/admin build` 通过。
- `git diff --check` 通过。
- `pnpm --dir web/admin typecheck` 仍失败；失败项为既有 TypeScript 债务，第二次运行已确认没有本次新增文件的错误。

## 关键文件

- `internal/controlplane/http/vnc_control.go`
- `internal/controlplane/http/admin_hosts.go`
- `internal/controlplane/http/user_hosts.go`
- `internal/controlplane/http/router.go`
- `deploy/docker/managed-user/entrypoint.sh`
- `deploy/docker/managed-user/restart-vnc.sh`
- `deploy/docker/managed-user/vnc-status.sh`
- `deploy/docker/managed-user/launch-chromium.sh`
- `web/admin/src/hooks/use-hosts.ts`
- `web/admin/src/hooks/use-portal-hosts.ts`
- `web/admin/src/routes/_dashboard/hosts/$hostId.tsx`
- `web/admin/src/routes/_portal/portal/hosts/$hostId.tsx`
