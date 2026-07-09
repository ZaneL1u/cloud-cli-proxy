---
status: complete
phase: quick-260709-kqq
subsystem: sqlite, repository, bypass, runtime
tags: [sqlite, json, bypass, egress, host-lifecycle]
---

# Quick Task 260709-kqq: 修复 bypass preset rules SQLite 扫描失败 Summary

## 结果

- 修复 SQLite `TEXT` JSON 列直接扫描到 `json.RawMessage` 失败的问题。
- 新增 `jsonText` 扫描 helper，支持 `string`、`[]byte`、`NULL` 三种 SQLite 返回形态。
- `host_bypass_presets.rules`、bypass snapshot JSON、bypass audit log JSON 改为先扫描到 `jsonText`。
- `hosts.host_mounts` 与 `egress_ips.proxy_config` 的读取路径也改为 `jsonText`，避免主机详情、VNC/SSH 控制、`prepare_host` 读取出口配置时踩同类问题。
- 新增真实 SQLite migration 回归测试，覆盖 bypass preset、host mounts、egress proxy config 与 `GetEgressIPByHost`。

## 根因

`database/sql` 不能把驱动返回的 `string` 直接写入 `*json.RawMessage`，线上错误：

`unsupported Scan, storing driver.Value type string into type *json.RawMessage`

对应字段是 SQLite `TEXT` JSON 列，不是 Go 原生 JSON 类型。

## 验证

- `go test ./internal/store/repository -run 'Test(BypassRepository_ListPresetsScansSQLiteTextRules|Repository_ScansSQLiteTextJSONForHostAndEgress)' -count=1` 通过。
- `go test ./internal/store/repository -count=1` 通过。
- `go test ./internal/runtime/tasks -count=1` 通过。
- `go test ./internal/controlplane/http -count=1` 通过。
- `go test ./... -count=1` 通过。

## 说明

这会停止 `/v1/admin/bypass/presets` 反复 500 与日志刷屏。由于 `prepare_host` 会读取 `egress_ips.proxy_config`，同类扫描失败也可能导致主机准备失败，进而表现为 VNC 和 SSH 都连不上。修复部署后，已有失败任务需要重新触发主机 prepare/start/rebuild。
