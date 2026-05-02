# Changelog

All notable changes to this project are documented in this file.

<!-- release-entries -->

## v3.2.4 - 2026-05-02
## What's Changed


**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v3.2.4...v3.2.4


## v3.2.5 - 2026-05-02
## What's Changed

### Backend (Go / API)
- feat: 镜像缓存状态检测与手动刷新 (2acb198)

### Frontend (Admin Web)
- feat: 镜像缓存状态检测与手动刷新 (2acb198)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v3.2.4...v3.2.5


## v3.1 - 2026-04-30
## What's Changed

### Backend (Go / API)
- fix(hot-sync): 退出不删文件 + 远程删除不再同步到本地 (64cfcc9)
- feat(quick-260424): 为 cloud-claude 添加外层会话信息面板 (77aee4d)
- feat(ui): 紧凑现代 CLI 输出 + 连接阶段状态刷新 (01b1a57)
- feat(ui): 极客风进度条 + hot-sync 安全修复 (ed8d93e)

### Runtime & Deployment
- feat(security): 容器反检测 + per-container machine-id + 遥测环境变量 (08f0a02)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v3.2.2...v3.1


## v3.2.3 - 2026-04-30
## What's Changed

### Backend (Go / API)
- feat(ui): 紧凑现代 CLI 输出 + 连接阶段状态刷新 (01b1a57)
- feat(ui): 极客风进度条 + hot-sync 安全修复 (ed8d93e)

### Runtime & Deployment
- feat(security): 容器反检测 + per-container machine-id + 遥测环境变量 (08f0a02)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v3.2.2...v3.2.3


## v3.2.2 - 2026-04-29
## What's Changed

### Runtime & Deployment
- fix(image): 修复 Claude Code 二进制路径查找 (045f50d)
- perf(image): 镜像瘦身 — 去 nodejs/npm，用官方二进制安装 Claude Code，清理 apt 缓存 (9739fe0)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v3.2.1...v3.2.2


## v2.1.2 - 2026-04-16
## What's Changed

### Frontend (Admin Web)
- fix(admin): 修复新建主机弹窗中 taskStatus 在声明前被引用 (1213659)

### Docs
- docs: Homebrew 安装与 cloud-claude 建机后连接说明 (3e2a7a5)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v2.1.1...v2.1.2


## v2.1.1 - 2026-04-16
## What's Changed

### Backend (Go / API)
- fix(cloudclaude): return remote mount path in timeout error (85502d8)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v2.1.0...v2.1.1


## v2.0 - 2026-04-15
## What's Changed

### Backend (Go / API)
- feat(28-01): add --security-opt apparmor=unconfined to createHost() (fa90fbe)
- feat(27-02): pass current working directory to ConnectAndRunClaude (6614104)
- refactor(27-02): split ConnectAndRunClaude into sshConnect/mountWorkspace/runClaude three-phase architecture (5bcc546)
- test(27-01): waitForMount 单元测试 (aa29d87)
- feat(27-01): 添加 pkg/sftp 依赖并创建 mount.go (30d691a)
- feat(26-01): cobra 根命令透传与退出码统一 (125e773)
- feat(26-01): SSH 模块重构——参数接收、安全命令构建、非 TTY 分支与退出码上浮 (3a7f666)
- feat(25-01): SSH 会话增加连接超时与根命令退出码完善 (8550bfe)
- feat(25-01): 完善 Entry 认证响应处理与就绪轮询错误分类 (5d18243)
- feat(25-01): cloud-claude CLI 骨架——cobra 入口、配置模块、init 子命令 (6fd35c3)
- feat(24-01): Worker 创建容器附加 FUSE 设备和 SYS_ADMIN 能力 (07a7b06)

### Runtime & Deployment
- feat(28-02): add FUSE kernel module detection to host-preflight.sh (bf22560)
- feat(24-01): 受管镜像预装 sshfs/fuse3 并配置 FUSE 权限 (d853b50)

### Docs
- docs(28-02): add FUSE prerequisites and AppArmor compatibility to deployment guides (fca403e)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.6.10...v2.0


## v1.6.10 - 2026-04-10
## What's Changed

### Backend (Go / API)
- fix(ssh): generate valid outbound keys (0daecbd)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.6.9...v1.6.10


## v1.6.9 - 2026-04-08
## What's Changed

### Runtime & Deployment
- fix(runtime): remove tmux wrapper from claude (da09ba9)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.6.8...v1.6.9


## v1.6.8 - 2026-04-08
## What's Changed

### Backend (Go / API)
- fix(ssh): 代理连容器认证顺序、create 后同步凭据与稳定解析容器 IP (05f763b)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.6.7...v1.6.8


## v1.6.7 - 2026-04-05
## What's Changed

### Backend (Go / API)
- fix: curl入口变量遮蔽bug + 增强日志 + SSH快捷复制按钮 (ef07560)

### Frontend (Admin Web)
- fix: curl入口变量遮蔽bug + 增强日志 + SSH快捷复制按钮 (ef07560)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.6.6...v1.6.7


## v1.6.6 - 2026-04-05
## What's Changed

### Backend (Go / API)
- fix: curl 入口改用主机 short_id 替代用户 short_id (82a9ef6)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.6.5...v1.6.6


## v1.6.5 - 2026-04-05
## What's Changed

### Backend (Go / API)
- feat: SSH代理用自己的密钥连容器，不再依赖密码 (069f2ac)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.6.4...v1.6.5


## v1.6.4 - 2026-04-05
## What's Changed

### Backend (Go / API)
- feat: SSH代理支持公钥认证（入站密钥免密登录） (4169f85)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.6.3...v1.6.4


## v1.6.3 - 2026-04-05
## What's Changed

### Backend (Go / API)
- feat: SSH密钥与容器双向实时同步 (5eb1477)

### Frontend (Admin Web)
- feat: SSH密钥与容器双向实时同步 (5eb1477)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.6.2...v1.6.3


## v1.6.2 - 2026-04-05
## What's Changed

### Backend (Go / API)
- fix: SSH密钥API响应格式与前端对齐 (195aefe)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.6.1...v1.6.2


## v1.6.1 - 2026-04-05
## What's Changed

### Runtime & Deployment
- fix(ci): 镜像构建仅由打 tag 触发，移除 push to main 触发 (b3353d0)
- fix: control-plane 镜像缺少 migrations 目录导致数据库迁移不执行 (4f33cd5)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.6.0...v1.6.1


## v1.6.0 - 2026-04-05
## What's Changed

### Backend (Go / API)
- feat(260405-qk2): SSH密钥体系改造：拆分为入站免密登录和出站外部鉴权 (8b994dc)

### Frontend (Admin Web)
- feat(260405-qk2): SSH密钥体系改造：拆分为入站免密登录和出站外部鉴权 (8b994dc)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.5.1...v1.6.0


## v1.5.1 - 2026-04-05
## What's Changed

### Backend (Go / API)
- feat(260405-jji): 镜像版本管理：自动拉取最新镜像 + 版本展示 + 一键升级 (507b288)

### Frontend (Admin Web)
- feat(260405-jji): 镜像版本管理：自动拉取最新镜像 + 版本展示 + 一键升级 (507b288)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.5.0...v1.5.1


## v1.5.0 - 2026-04-05
## What's Changed

### Backend (Go / API)
- feat(260405-hio): structured Claude Code settings panel and system fingerprint (bea7886)
- feat(260405-hai): enhance Claude status API with per-process details (2ae9592)
- feat(260405-h13): add 5 container management API endpoints (e8de4a8)

### Frontend (Admin Web)
- feat(260405-hio): structured Claude Code settings panel and system fingerprint (bea7886)
- feat(260405-hai): enhance Claude status API with per-process details (2ae9592)
- feat(260405-h13): add frontend hooks, dialogs and Claude status card (6affd42)

### Docs
- docs: add UI screenshots and local development setup guide (bae69a5)
- docs: refine hero glow to vitest-like large halo (6720691)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.4.6...v1.5.0


## v1.4.6 - 2026-04-02
## What's Changed

### Runtime & Deployment
- fix(ci): lowercase ghcr image prefix for registry cache (6f420d8)

### Docs
- docs: add hero logo and glow effect on homepage (cd689f5)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.4.5...v1.4.6


## v1.4.5 - 2026-04-02
## What's Changed

### Runtime & Deployment
- perf(ci): improve docker layer cache hit rate (6e410d4)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.4.4...v1.4.5


## v1.4.4 - 2026-04-02
## What's Changed

### Runtime & Deployment
- fix(ci): source pnpm version from root package manager (6cce3f8)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.4.3...v1.4.4

