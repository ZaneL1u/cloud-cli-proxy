# Quick Task 260709-kqq: 修复 bypass preset rules SQLite 扫描失败

## Goal

修复生产环境 `list bypass presets failed` 反复报错，原因是 SQLite `TEXT` JSON 列不能直接扫描到 `json.RawMessage`。

## Tasks

1. 用真实 SQLite migration 写回归测试，复现 `host_bypass_presets.rules` 扫描失败。
2. 梳理同类 JSON TEXT 字段，避免只修 `rules` 单点。
3. 增加通用 JSON 文本扫描 helper，并更新 bypass preset / snapshot / audit log 扫描路径。
4. 跑 repository 相关测试和格式检查。
5. 说明这类错误与 managed CPU、VNC/SSH 不可连接之间的可能关系与部署动作。
