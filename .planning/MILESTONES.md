# Milestones

## v2.0 cloud-claude 透明远程 CLI (Shipped: 2026-04-15)

**Phases completed:** 5 phases, 7 plans, 16 tasks

**Key accomplishments:**

- 受管镜像预装 sshfs + fuse3 并配置 FUSE 权限，Worker 附加 --device /dev/fuse 和 --cap-add SYS_ADMIN，SSH Proxy 确认零改造支持多 session channel
- cobra 入口 + init 配置持久化 + Entry API 认证轮询 + SSH PTY 远程 claude 会话的完整 CLI 闭环
- shellescape 安全命令构建 + cobra 透传用户 claude 参数 + 非 TTY 管道模式 + 退出码返回值上浮修复 HI-01
- pkg/sftp 嵌入式 SFTP server + sshfs passive 模式启动 + mountpoint 轮询检测 + fusermount 防御性清理
- 重构 ConnectAndRunClaude 为 sshConnect→mountWorkspace→runClaude 三阶段架构，main.go 传递 os.Getwd() 实现端到端目录映射
- worker.go 添加 apparmor=unconfined 解除 FUSE 阻断，238 行验证脚本覆盖 sshfs 真实挂载 + 网络策略共存 + E2E 流程
- host-preflight.sh 添加 FUSE 内核模块双重检测，中英文部署文档补充 FUSE/AppArmor 兼容性章节和已知限制表

---

## v1.2 用户自助面板与 Bootstrap 重设计 (Partial: 2026-03-29, remaining deferred)

**Phases completed:** 2 of 6 phases (Phase 11-12), remaining (Phase 13-16) deferred to future milestone
**Plans completed:** 5 plans

**Key accomplishments:**

- 用户登录认证体系（区别于管理员 JWT），统一登录页按角色自动跳转，用户 API 资源隔离（403）
- claude_accounts 数据模型（一个用户多个 Claude 账号，每个账号关联一台主机）
- 用户自助面板骨架：TanStack Router 角色路由守卫，用户面板与管理员面板共存于同一 React 应用
- 用户自助 API（UserHostsHandler + 主机列表/详情/重建 + 出口 IP 查看）
- auth_middleware.go（AuthMiddleware / RequireRole / UserIDFromContext / RoleFromContext）

**Deferred to future:**

- Phase 13: 账号管理与用户资源视图（账号 CRUD、有效期、售后换号）
- Phase 14: KasmVNC 用户面（浏览器远程桌面）
- Phase 15: Bootstrap 重设计（短 URL 入口与实时状态推送）
- Phase 16: 级联禁用与到期治理（用户/账号/主机到期联动）

---

## v1.1 支持代理协议出网 (Shipped: 2026-03-28)

**Phases completed:** 4 phases, 11 plans, 21 tasks

**Key accomplishments:**

- egress_ips 表新增 tunnel_type（wireguard/proxy CHECK 约束）和 proxy_config JSONB 列，Go 模型和全部 6 个 SQL 查询同步扩展
- EgressConfig 扩展为 wireguard/proxy 双模式，ValidateEgressBinding 按 TunnelType 分支校验，新增 ProxySpec 和 3 个 proxy 测试用例
- Admin API 完整支持 tunnel_type/proxy_config 字段的创建、更新、白名单校验和响应脱敏，repoValidator 正确映射新字段
- sing-box 配置结构体 + JSON 生成函数（tun inbound / proxy outbound / DNS hijack）及受管镜像 v1.13.3 二进制预装
- proxy 模式 nftables 防火墙规则（tun0/proxy server 白名单）和宿主机 IP 转发 + masquerade
- SingBoxProvider 15 步 PrepareHost 流水线（tun 模式全流量代理）和 RoutingProvider 工厂按 TunnelType 自动路由到 WireGuard/sing-box
- 代理测试 API 支持 SOCKS5/HTTP/vmess/ss/trojan 五种协议，返回连通性、出口 IP 匹配、DNS 泄漏三项检测结果，前端 TestResult 类型和 mutation hook 就绪
- 动态隧道类型表单切换 + 5 种代理协议字段渲染 + 表单/JSON 双向编辑 + 后端密码合并逻辑
- 出口 IP 列表页增加隧道类型 / 测试状态两列并集成 TestResultDialog 展示连通性、出口 IP 匹配和 DNS 泄漏三项检测详情
- stopHost 追加 CleanupHost 消除 mgmt veth 残留 + vmess/ss/trojan 代理测试添加 sing-box LookPath 预检返回中文提示
- localStorage 持久化代理测试结果跨刷新恢复 + WireGuard 类型出口 IP 测试按钮禁用并显示 toast 提示

---

## v1.0 MVP (Shipped: 2026-03-28)

**Phases completed:** 6 phases, 19 plans, 42 tasks

**Key accomplishments:**

- 基于 Go 标准库的 control-plane 启动骨架、PostgreSQL 核心 schema 与单宿主机开发编排
- 固定镜像锁、SSH 工作环境和 `claude code` 预装的受管用户模板容器
- 基于 Unix socket 的 host-agent、真实 Docker 生命周期 worker 与 systemd 特权边界
- WireGuard 隧道类型建模、6 类网络错误体系、启动前绑定校验门禁和 --network=none 容器隔离
- WireGuard birthplace-namespace 隧道注入、nftables 默认拒绝防火墙、管理 veth 和隧道 DNS 配置，TunnelProvider 替换 NoopProvider
- Triple network verification (egress IP match, DNS path, leak blocking) integrated as PrepareHost pipeline gate with typed event recording and extended host preflight checks
- bcrypt 密码认证 + 异步 start_host 任务入队 + 受管 bootstrap 脚本（密码不回显 + 稳定退出码）
- SSH readiness gate 阻止假就绪接入 + GET /v1/bootstrap/tasks/{taskID} 阶段化进度轮询（D-06 固定映射）
- host-agent ssh.handoff.ready 元数据 + GET handoff API + 稳定 error_code/exit_code 映射 + bootstrap 脚本 poll→handoff→exec ssh 完整闭环
- Go 端 JWT 登录 API + 认证中间件 + 仪表板统计 API，React 19 SPA 脚手架含登录页、5 项侧边栏导航和 3 卡片仪表板概览
- 用户 CRUD API（Go bcrypt + crypto/rand 密码轮换）+ React 前端用户管理全页面（列表/详情/创建/删除确认/密码轮换）
- 出口 IP CRUD + 绑定管理（含运行中主机保护）+ 主机启停重建 + 任务列表的完整前后端实现
- DB migration with expires_at/user_id fields, generic ticker-based scheduler, expiry scanner with auto-stop, and admin expiry API endpoints
- 为所有管理 handler 注入事件记录、新增事件查询 API、实现 host-agent 容器 inspect 端点和 DB/Docker 运行时对账定时器
- 用户列表/详情页展示和管理到期时间，事件日志页面支持筛选分页和 metadata 展开，仪表板集成最近事件摘要卡片
- ExpiryScanner/Reconciler 11 个 mock 单元测试 + bootstrap 脚本 7 个 BATS 错误码契约测试，修复脚本 set -eo pipefail 下两个退出码 bug
- 部署指南、运维手册、故障排查手册和自动化部署/备份脚本，覆盖从零部署到日常运维的完整文档体系
- 结构化日志 + healthz 分组检查 + pgxpool 显式配置 + bootstrap 错误码解析 + EgressIP 敏感字段清除 + 前端表单格式校验

---
