---
phase: quick
plan: "260420"
plan_id: 260420-claude-chrome
subsystem: admin-panel
tags: [export, import, config, claude, chrome, backup]
dependency_graph:
  requires: []
  provides: ["admin.host.config_exported", "admin.host.config_imported"]
  affects: ["web/admin/src/routes/_dashboard/hosts/$hostId.tsx"]
tech-stack:
  added: []
  patterns: [docker-exec-pipe, multipart-upload, blob-download, hidden-file-input]
key-files:
  created: []
  modified:
    - internal/controlplane/http/admin_hosts.go
    - internal/controlplane/http/router.go
    - web/admin/src/hooks/use-hosts.ts
    - web/admin/src/routes/_dashboard/hosts/$hostId.tsx
decisions: []
metrics:
  duration: "3m 20s"
  completed_date: "2026-04-28"
---

# Quick Task 260420: 主机 Claude/Chrome 配置导出导入功能 Summary

**One-liner:** 为后台管理主机详情页添加容器内 Claude 配置和 Chrome 数据的 tar.gz 导出/导入功能，支持流式下载和 multipart 上传恢复。

## 执行结果

| 任务 | 名称 | Commit | 文件 |
|------|------|--------|------|
| 1 | 后端实现导出和导入 handler | 103331b | internal/controlplane/http/admin_hosts.go |
| 2 | 注册路由并添加前端 hooks | a892af4 | internal/controlplane/http/router.go, web/admin/src/hooks/use-hosts.ts |
| 3 | 前端主机详情页添加导出导入按钮 | 2038c4b | web/admin/src/routes/_dashboard/hosts/$hostId.tsx |

## 变更详情

### 后端 (Go)

**`internal/controlplane/http/admin_hosts.go`**
- 新增 `ExportConfig() nethttp.Handler`：
  - 验证主机存在且 `status == "running"`，否则返回 409
  - 容器名：`cloudproxy-{hostID}`
  - 执行 `docker exec -i containerName tar czf - -C /workspace .claude .claude.json .chrome-data -C /var/lib/claude-persist . .cache`
  - 通过 `cmd.StdoutPipe()` + `io.Copy(w, stdout)` 流式输出，避免内存缓冲
  - 响应头：`Content-Type: application/gzip`，`Content-Disposition: attachment; filename="host-{hostID}-config.tar.gz"`
  - 超时：30 秒
  - 记录审计事件 `admin.host.config_exported`

- 新增 `ImportConfig() nethttp.Handler`：
  - 验证主机存在且 `status == "running"`，否则返回 409
  - 限制上传大小：`r.ParseMultipartForm(100 << 20)`（100MB）
  - 从 form 中取文件字段名 `file`
  - 执行 `docker exec -i containerName tar xzf - -C /`，将上传文件直接 pipe 到 `cmd.Stdin`
  - 成功返回 `{"status": "ok"}`
  - 超时：30 秒
  - 记录审计事件 `admin.host.config_imported`

**`internal/controlplane/http/router.go`**
- 在 `deps.AdminHosts != nil` 块中新增两条路由：
  - `GET /v1/admin/hosts/{hostID}/config/export`
  - `POST /v1/admin/hosts/{hostID}/config/import`

### 前端 (React/TypeScript)

**`web/admin/src/hooks/use-hosts.ts`**
- 新增 `useExportHostConfig()` hook：
  - 使用 `fetch` 直接请求（非 `apiFetch`，因为需要处理 blob）
  - 从 `localStorage` 读取 token 添加 `Authorization: Bearer` header
  - 解析 `Content-Disposition` 获取文件名，创建临时 `<a>` 标签触发下载

- 新增 `useImportHostConfig()` hook：
  - 接受 `{ hostId: string; file: File }`
  - 构造 `FormData`，字段名 `file`
  - 使用 `fetch` POST，同样从 `localStorage` 读取 token
  - `onSuccess` 时 `toast.success("配置导入成功")`

**`web/admin/src/routes/_dashboard/hosts/$hostId.tsx`**
- 导入 `Download`、`Upload` 图标和新增 hooks
- 在生命周期卡片的「编辑 Claude 配置」和「打开 VNC 桌面」按钮之间添加：
  - **导出配置**按钮：`variant="secondary"`，`Download` 图标，点击触发下载，非 running 时 disabled
  - **导入配置**按钮：`variant="secondary"`，`Upload` 图标，点击触发隐藏 `<input type="file">`，选择文件后上传，非 running 时 disabled
- 导入使用 `useRef<HTMLInputElement>` 管理隐藏 file input
- 导出/导入失败均通过 `toast.error` 提示

## 验证结果

- `go build ./...` PASS
- `cd web/admin && npx tsc --noEmit` PASS (exit code 0)

## Deviations from Plan

**无偏差** — 计划按预期执行，所有任务完成，无 Rule 1-4 触发。

## Auth Gates

无。

## Known Stubs

无。所有功能均直接调用 docker exec 和真实 API，无占位数据或硬编码空值。

## Self-Check: PASSED

- [x] `internal/controlplane/http/admin_hosts.go` 包含 `ExportConfig()` 和 `ImportConfig()`
- [x] `internal/controlplane/http/router.go` 包含两条新路由
- [x] `web/admin/src/hooks/use-hosts.ts` 包含 `useExportHostConfig` 和 `useImportHostConfig`
- [x] `web/admin/src/routes/_dashboard/hosts/$hostId.tsx` 包含导出/导入按钮
- [x] Commit `103331b` 存在
- [x] Commit `a892af4` 存在
- [x] Commit `2038c4b` 存在
