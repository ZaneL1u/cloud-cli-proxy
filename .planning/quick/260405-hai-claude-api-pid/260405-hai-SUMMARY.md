# Quick Task 260405-hai: Summary

## 变更概要

增强了 `GET /v1/admin/hosts/{hostID}/claude/status` API 和前端 Claude 状态卡片：

### 后端
- `GetClaudeStatus` 从使用 `pgrep -c` 计数改为使用 `ps + readlink /proc/PID/cwd` 获取每个进程详情
- 返回新增 `processes` 数组字段，每项包含 `pid`、`work_dir`、`elapsed_seconds`
- `running_instances` 改为从 processes 数组长度计算

### 前端
- `use-hosts.ts` 新增 `ClaudeProcess` 和 `ClaudeStatusResponse` 类型
- `claude-status-card.tsx` 新增 `ProcessTable` 组件，以表格形式展示：
  - PID
  - 工作目录（`/workspace/xxx` 缩写为 `~/xxx`）
  - 运行时间（自动格式化为秒/分/小时）

## 修改文件
| 文件 | 变更 |
|------|------|
| `internal/controlplane/http/admin_hosts.go` | 重写 GetClaudeStatus，新增 claudeProcess struct |
| `web/admin/src/hooks/use-hosts.ts` | 新增 ClaudeProcess/ClaudeStatusResponse 类型 |
| `web/admin/src/components/hosts/claude-status-card.tsx` | 新增 ProcessTable + 时间/路径格式化 |
