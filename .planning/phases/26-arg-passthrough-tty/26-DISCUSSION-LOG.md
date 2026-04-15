# Phase 26: 参数透传与终端体验 - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-15
**Phase:** 26-参数透传与终端体验
**Areas discussed:** 参数解析与透传, 信号转发策略, 退出码与TTY恢复, 远程命令构建
**Mode:** --auto (all areas auto-selected, recommended defaults chosen)

---

## 参数解析与透传

| Option | Description | Selected |
|--------|-------------|----------|
| cobra DisableFlags + os.Args 切分 | 禁用根命令 flag 解析，claude 参数完整传入 | ✓ |
| 自定义 arg parser | 手动解析 os.Args，完全绕过 cobra | |
| cobra hook pre-run 截取 | 在 PersistentPreRun 中截取剩余参数 | |

**User's choice:** [auto] cobra DisableFlags + os.Args 切分（推荐默认）
**Notes:** 保持 cobra 子命令结构不变，仅对根命令禁用 flag 解析

---

## 信号转发策略

| Option | Description | Selected |
|--------|-------------|----------|
| raw mode 自然转发 | raw mode 下 Ctrl+C/Ctrl+\ 作为字节序列经 SSH stdin 发送 | ✓ |
| Go signal handler 主动转发 | 捕获 SIGINT/SIGQUIT 后通过 session.Signal() 转发 | |

**User's choice:** [auto] raw mode 自然转发（推荐默认）
**Notes:** raw mode 下 OS 不会生成 SIGINT，字节直接到达 SSH channel

---

## 退出码与 TTY 恢复

| Option | Description | Selected |
|--------|-------------|----------|
| 返回值传递 + main 统一退出 | ConnectAndRunClaude 返回退出码，main 在 defer Restore 后 os.Exit | ✓ |
| 函数内 Restore 后 Exit | 在 Exit 前显式调用 Restore | |

**User's choice:** [auto] 返回值传递 + main 统一退出（推荐默认）
**Notes:** 修复 HI-01：避免 os.Exit 跳过 defer

---

## 远程命令构建

| Option | Description | Selected |
|--------|-------------|----------|
| shell-escaped 单行命令 | claude + args 拼接为 shell-safe 字符串，session.Start() 发送 | ✓ |
| exec 直接传参 | 通过 SSH exec 直接传递参数数组 | |

**User's choice:** [auto] shell-escaped 单行命令（推荐默认）
**Notes:** SSH exec 协议将整个命令字符串发送给远程 shell

---

## Claude's Discretion

- shellescape 方案
- claude --help 本地短路
- 远程 claude 路径

## Deferred Ideas

- sshfs 目录映射（Phase 27）
- SSH 主机密钥钉扎
