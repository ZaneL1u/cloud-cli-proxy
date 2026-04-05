---
phase: quick-260405-hai
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/controlplane/http/admin_hosts.go
  - web/admin/src/hooks/use-hosts.ts
  - web/admin/src/components/hosts/claude-status-card.tsx
autonomous: true
---

<objective>
增强 Claude 状态 API，从仅返回进程总数变为返回每个进程的 PID、工作目录和运行时间，
前端同步升级为进程详情表格。
</objective>

<tasks>
<task type="auto">
  <name>Task 1: 后端 + 前端增强 Claude 进程详情</name>
  <files>internal/controlplane/http/admin_hosts.go, web/admin/src/hooks/use-hosts.ts, web/admin/src/components/hosts/claude-status-card.tsx</files>
  <action>
  1. 后端 GetClaudeStatus 改用 `ps -eo pid=,etimes=,args=` + `readlink /proc/$pid/cwd` 获取每个 claude-real 进程的 PID、运行时间和工作目录
  2. 返回 `processes` 数组，每项包含 pid/work_dir/elapsed_seconds
  3. 前端 hooks 新增 ClaudeProcess/ClaudeStatusResponse 类型
  4. ClaudeStatusCard 新增 ProcessTable 组件展示进程列表
  </action>
  <done>后端返回进程详情数组，前端以表格形式展示每个 Claude 的 PID、工作目录和运行时间</done>
</task>
</tasks>
