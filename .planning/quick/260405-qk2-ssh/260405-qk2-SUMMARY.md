# Quick Task 260405-qk2: SSH 密钥体系改造 — Summary

**Completed:** 2026-04-05

## 变更概要

### 问题
原有 SSH 密钥系统每用户仅支持一对密钥，存储在 users 表上，无法区分入站（免密登录）和出站（外部鉴权）用途，也不支持写入 authorized_keys。

### 解决方案

#### 数据库
- 新建 `ssh_keys` 表，支持多密钥、`purpose` 字段（inbound/outbound）、label、fingerprint
- 自动从 users 表迁移已有密钥数据

#### 后端
- `contracts.go`: 新增 `SSHKeyEntry` 类型，`HostActionRequest` 新增 `SSHKeys` 数组字段
- `ssh_keys.go`: 完全重写为 List/Create/Delete 三端点，Create 支持自动生成密钥对（outbound）或直接导入公钥（inbound）
- `runtime_service.go`: 从新表查询用户所有密钥并传递到 HostActionRequest
- `worker.go`: 入站密钥追加到 `~/.ssh/authorized_keys`，出站密钥注入为身份文件（第一个用默认名，后续用 label 命名）
- `router.go`: 更新 admin 和 user 路由

#### 前端
- `use-ssh-keys.ts`: 重写为多密钥 CRUD hooks
- `ssh-key-manager.tsx`: 重新设计为入站/出站两个独立区域，入站仅需粘贴公钥，出站支持生成和导入
- 用户详情页和门户页同步更新
- 引导组件更新描述文字

## 修改的文件（13 个）
- `internal/store/migrations/0012_ssh_keys_table.sql` — 新建
- `internal/store/repository/models.go` — SSHKey 模型
- `internal/store/repository/queries.go` — 5 个 CRUD 方法
- `internal/agentapi/contracts.go` — SSHKeyEntry + SSHKeys 字段
- `internal/controlplane/http/ssh_keys.go` — 完全重写
- `internal/controlplane/http/router.go` — 路由更新
- `internal/runtime/runtime_service.go` — 密钥查询和传递
- `internal/runtime/tasks/worker.go` — injectSSHKeys 重写
- `web/admin/src/hooks/use-ssh-keys.ts` — 完全重写
- `web/admin/src/components/ssh-keys/ssh-key-manager.tsx` — 完全重写
- `web/admin/src/routes/_dashboard/users/$userId.tsx` — 适配新接口
- `web/admin/src/routes/_portal/portal/hosts/$hostId.tsx` — 适配新接口
- `web/admin/src/components/onboarding-guide.tsx` — 更新引导文字
