# Phase 40: VS Code Remote-SSH E2E 验证报告

**验证日期:** 2026-05-08
**测试环境:** macOS (Docker Desktop)
**容器名:** cloud-claude-local-b594e6e9
**egress 配置:** 无（纯本地模式，无 sing-box 隧道）

---

## UAT 脚本结果

> 本次验证以手动测试为主，UAT 脚本在有 egress 配置的环境下运行。

| 场景 | 状态 | 详情 |
|------|------|------|
| sshd 进程检测 | PASS | sshd 正常监听容器内 22 端口 |
| SSH 连接 | PASS | `ssh workspace@127.0.0.1 -p 32822` key 认证成功 |

---

## 手动测试结果

| 步骤 | 场景 | 状态 | 实际结果 |
|------|------|------|----------|
| 1 | VS Code Remote-SSH 连接 | PASS | VS Code 通过 SSH 成功连接到容器 |
| 2 | 本地 SSH 连接 | PASS | 终端 SSH 连接正常（key 认证） |

---

## 发现的问题

### 问题 1：Docker inspect 错误信息不匹配
- **现象：** `cloud-claude local up` 报错 `no such object`
- **原因：** Docker 新版本返回 `no such object`，代码只匹配 `No such container`
- **修复：** `internal/local/container.go` 增加 `no such object` 匹配

### 问题 2：SSH 公钥未注入容器
- **现象：** VS Code Remote-SSH 无法连接（密码认证不支持）
- **原因：** `local up` 未将用户 `~/.ssh/*.pub` 注入容器 authorized_keys
- **修复：**
  - `entrypoint.sh` 增加 `CONTAINER_SSH_AUTHORIZED_KEY` 环境变量支持
  - `internal/local/local.go` 增加 `findSSHPublicKey()` 自动检测并注入公钥

---

## 修复记录

| 文件 | 修改内容 |
|------|----------|
| `internal/local/container.go` | `containerExists` 和 `inspectContainerStatus` 增加 `no such object` 错误匹配 |
| `deploy/docker/managed-user/entrypoint.sh` | 增加 `CONTAINER_SSH_AUTHORIZED_KEY` 环境变量，写入 `/workspace/.ssh/authorized_keys` |
| `internal/local/local.go` | 增加 `findSSHPublicKey()` 函数，`buildCreateArgs` 自动注入用户 SSH 公钥 |

---

## 结论

**全部通过。** VS Code Remote-SSH 和本地 SSH 连接均正常工作。发现并修复了 2 个问题（Docker 错误匹配、SSH 公钥注入），修复已提交。

---

*Phase: 40-vs-code-remote-ssh-e2e*
*Report created: 2026-05-08*
*Report updated: 2026-05-08*
