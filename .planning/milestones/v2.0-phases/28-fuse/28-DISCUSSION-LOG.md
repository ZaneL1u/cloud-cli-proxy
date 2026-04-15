# Phase 28: 生产环境 FUSE 兼容性验证 - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-15
**Phase:** 28-生产环境 FUSE 兼容性验证
**Areas discussed:** 验证环境与方法, AppArmor/seccomp 处理策略, FUSE 与网络策略共存验证, 验证产出物形式
**Mode:** --auto (all choices auto-selected)

---

## 验证环境与方法

| Option | Description | Selected |
|--------|-------------|----------|
| 自动化验证脚本 | 编写可复用的 shell 脚本，自动检测宿主机安全模块状态并执行 FUSE 挂载/读写测试 | ✓ |
| 手工验证步骤 | 编写 checklist 由运维人员手工执行 | |
| CI 集成测试 | 在 CI 环境中自动运行（需要特权容器支持） | |

**User's choice:** [auto] 自动化验证脚本（推荐默认）
**Notes:** 脚本应支持在不同 Linux 发行版上重复执行，输出结构化 PASS/FAIL 结果

---

## AppArmor/seccomp 处理策略

| Option | Description | Selected |
|--------|-------------|----------|
| 检测优先，必要时最小权限修复 | 先检测默认 profile 是否兼容，不兼容时添加最小权限 --security-opt | ✓ |
| 直接 apparmor=unconfined | 跳过检测，直接禁用 AppArmor（简单但权限过宽） | |
| 自定义 AppArmor profile | 编写专用 profile 精细控制权限（精确但维护成本高） | |

**User's choice:** [auto] 检测优先，必要时最小权限修复（推荐默认）
**Notes:** SYS_ADMIN 已附加，大多数情况下 FUSE 在 docker-default profile 下可正常工作

---

## FUSE 与网络策略共存验证

| Option | Description | Selected |
|--------|-------------|----------|
| 确认无需额外规则 | sshfs slave 走 SSH channel pipe，不经过网络栈，验证在全隧道状态下成立 | ✓ |
| 添加 FUSE 专用防火墙规则 | 在 nftables 中为 FUSE 相关流量添加例外（实际不需要） | |

**User's choice:** [auto] 确认无需额外规则（推荐默认）
**Notes:** sshfs slave 的 SFTP 数据走进程内 pipe（SSH session channel），与网络栈完全无关

---

## 验证产出物形式

| Option | Description | Selected |
|--------|-------------|----------|
| 验证脚本 + 文档更新 + 代码修复 | 三件套：可执行脚本、部署文档补充、必要的 worker.go 修改 | ✓ |
| 仅文档记录 | 只更新部署文档说明兼容性要求 | |
| 仅代码修复 | 只修改 worker.go 添加 security-opt | |

**User's choice:** [auto] 验证脚本 + 文档更新 + 代码修复（推荐默认）
**Notes:** 验证脚本支持 CI 集成和运维手工复查

## Claude's Discretion

- 验证脚本的具体实现结构和诊断信息格式
- 是否需要编写自定义 AppArmor profile 文件或仅使用 --security-opt 开关
- 部署文档的更新范围和详细程度
- 验证脚本中是否包含性能基准测试

## Deferred Ideas

- Mutagen 备选目录映射路径 — v2.x ENH-01
- 大目录 ignore 策略 — v2.x ENH-04
- sshfs 性能调优 — 当性能成为主诉时再评估
