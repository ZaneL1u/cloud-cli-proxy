---
status: complete
phase: quick-260708-q1b
subsystem: managed-user-image, vnc, chromium
tags: [vnc, kasmvnc, chromium, resource-limits]
---

# Quick Task 260708-q1b: 降低 VNC 与 Chromium 默认资源占用 Summary

## 结果

- KasmVNC 默认编码压力降低：`max_frame_rate` 从 60 降到 30，`max_quality` 从 10 降到 8，`rectangle_compress_threads` 从 4 降到 2。
- `entrypoint.sh` 与 `restart-vnc.sh` 的 KasmVNC 配置保持一致，避免首次启动与手动重启行为不一致。
- Chromium 增加默认 renderer 进程上限：`CHROMIUM_RENDERER_PROCESS_LIMIT=6`，并保留环境变量覆盖能力。
- 测试锁定轻量 KasmVNC 配置和 Chromium renderer 上限，避免后续改回高资源默认值。

## 验证

- `go test ./internal/controlplane/http -run TestVNCScriptsDeclareWatcherLimitAndSafeChromiumWindow -count=1` 通过。
- `go test ./internal/controlplane/http -count=1` 通过。
- `bash -n deploy/docker/managed-user/entrypoint.sh` 通过。
- `bash -n deploy/docker/managed-user/restart-vnc.sh` 通过。
- `bash -n deploy/docker/managed-user/vnc-status.sh` 通过。
- `bash -n deploy/docker/managed-user/launch-chromium.sh` 通过。
- `git diff --check` 通过。

## 说明

这次调整不能保证 VNC 永不崩溃，但会明显降低高负载下 `Xvnc` 与 Chromium renderer 被 CPU、内存或 PID 压力打掉的概率。既有容器需要重建或升级镜像内脚本后才能获得这些新默认值。
