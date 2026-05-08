# Changelog

All notable changes to this project are documented in this file.

<!-- release-entries -->

## v3.4.0 - 2026-05-08
## What's Changed

### Backend (Go / API)
- fix(local): remove platform-gated SSH port mapping (95773ba)
- feat(44-01): 注册 checkSSHDForwarding 到 doctor ssh 维度 (484e623)
- feat(44-01): 新增 3 个 sshd 转发检查错误码 + parseSSHDForwarding + checkSSHDForwarding + 13 个单元测试 (bc00c7f)
- test(41-02): remote-ssh dimension unit tests + error code coverage (45a73a5)
- feat(41-01): add remote-ssh doctor dimension with 5 checks (a4de2eb)
- feat(41-01): register remote-ssh error codes and explain coverage (72d3b6a)
- fix(40): Docker error matching + SSH key injection for VS Code Remote-SSH (1f5afd6)
- feat(39-02): egress config injection + protocol detection (bdb6904)
- feat(39-01): local CLI subcommand + internal/local package + entrypoint MODE=local (c41735a)
- feat(038-02): pre-dial shared targetClient in handleConnection (2d53dc5)
- feat(038-02): implement handleGlobalRequests + proxyForwardedChannels (2b3efb8)
- test(038-02): add failing tests for handleGlobalRequests and proxyForwardedChannels (474a377)
- feat(038-01): add direct-tcpip channel dispatch and refactor dialContainer (d70c53c)
- test(038-01): add failing tests for direct-tcpip channel dispatch in proxy (d559cd7)
- feat(038-01): implement direct-tcpip forwarding with security validation (8ca26e9)
- test(038-01): add failing tests for direct-tcpip payload parsing and security validation (1346714)

### Runtime & Deployment
- fix(40): Docker error matching + SSH key injection for VS Code Remote-SSH (1f5afd6)
- feat(39-03): devcontainer.json MODE=local + sing-box entrypoint startup (295d558)
- feat(39-01): local CLI subcommand + internal/local package + entrypoint MODE=local (c41735a)
- feat(v3.2): sshd_config forwarding + devcontainer template (a218dcc)
- docs(quick-260508): 基于当前代码全面更新 README、docs 与 deploy 文档 (c71e048)

### Docs
- fix(docs): 修复 VitePress 构建失败 (58a28ce)
- docs(quick-260508): 基于当前代码全面更新 README、docs 与 deploy 文档 (c71e048)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v3.3.7...v3.4.0


## v3.3.7 - 2026-05-07
## What's Changed

### Backend (Go / API)
- fix(app): embedded 模式下也启用 Reconciler 自动恢复 — 新增 dockerInspector 直接调用 docker inspect (16fd02f)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v3.3.6...v3.3.7


## v3.3.6 - 2026-05-07
## What's Changed

### Backend (Go / API)
- fix(quick-260507): user / gateway 容器 restart 策略改为 no，防止 Docker 重启后自动重建错误路由导致 IP 泄漏 (67e8d21)
- feat(quick-260507): Reconciler 对账新增自动恢复逻辑 — DB running 但 docker 缺失时自动排队启动 (03e7dcb)

### Runtime & Deployment
- fix(quick-260507): user / gateway 容器 restart 策略改为 no (67e8d21)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v3.3.5...v3.3.6


## v3.3.5 - 2026-05-06
## What's Changed

### Backend (Go / API)
- fix(runtime): 镜像刷新时从 label 读取实际版本号 (c22d3bc)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v3.3.4...v3.3.5


## v3.3.4 - 2026-05-06
## What's Changed

### Backend (Go / API)
- fix(network): 移除创建主机时的 egress IP smoke check (dc61287)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v3.3.3...v3.3.4


## v3.3.3 - 2026-05-06
## What's Changed

### Backend (Go / API)
- fix(quick-260506-ty7): seed admin generates entry_password + ed25519 keys + ssh_keys row, idempotent backfill (7bfc626)
- refactor(quick-260506-ty7): extract credential generators to internal/controlplane/credgen (5b0105b)
- feat(quick-260506): 容器日志查看 + egress IP 修复 + X11 lock 清理 (1fbf88f)
- fix(quick-260506): 修复状态机分裂 — list 去 DockerStatus + worker 失败路径补 stop + 前端统一 DB status (c4a164c)
- fix(quick-260506): 修复 IP 泄漏 — 统一 disconnect bridge + 反竞态 verify+retry (23a4a02)
- fix: 修复若干问题 (bd147c6)

### Frontend (Admin Web)
- feat(quick-260506): 容器日志查看 + egress IP 修复 + X11 lock 清理 (1fbf88f)
- fix(quick-260506): 修复状态机分裂 — list 去 DockerStatus + worker 失败路径补 stop + 前端统一 DB status (c4a164c)
- fix: 修复若干问题 (bd147c6)
- fix(ssh-keys): create/delete 后立即 refetch 同步状态 (f061126)

### Runtime & Deployment
- fix(quick-260506-urq): install.sh 路径改用 cp -fL 复制真实 claude 二进制 (28d18e7)
- feat(quick-260506): 容器日志查看 + egress IP 修复 + X11 lock 清理 (1fbf88f)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v3.3.2...v3.3.3


## v3.3.2 - 2026-05-05
## What's Changed

### Backend (Go / API)
- refactor(admin): 重构挂载和端口映射输入交互 (ff1a8b4)
- fix(probe): docker pull 超时从 30s 放宽到 3 分钟 (e3ff822)
- fix(probe): docker pull 单独限 30 秒，超时给明确引导 (8a848fa)
- fix(probe): 保留 ghcr.io 镜像，本地存在时跳过 docker pull (6819211)
- fix(probe): 探针改回本地 gateway 镜像，去掉 ghcr.io pull (d33c835)
- fix(probe): SSE 探测超时延长至 5 分钟，增加心跳保活 (1211155)
- feat(quick-260505-gjs-01): 后端 SSE 流式探测 endpoint (4ae4948)
- fix(probe): 探针 sing-box 镜像固定为 v1.13.3 版本 (e82a8d8)
- feat(probe): 探针启动前显式拉取 sing-box 镜像，失败报错更清晰 (b89d44d)
- revert(probe): 换回 ghcr.io/sagernet/sing-box 作为 IP 探测镜像 (a1b625e)
- fix(quick-260504-elo-02): rejoinHostNetworks 加容器存在性探测，避免 macOS 误报 (5596353)
- feat(quick-260504-dtd-01): add resolveProbeNetworking helper for host-binary mode (07c4a36)
- test(quick-260504-dtd-01): add failing tests for resolveProbeNetworking (de904f7)

### Frontend (Admin Web)
- refactor(admin): 重构挂载和端口映射输入交互 (ff1a8b4)
- feat(quick-260505-gjs): 探测阶段移到状态列，去掉检测中自动弹窗 (a75e8eb)
- feat(quick-260505-gjs-02): 前端 SSE hook 与阶段性弹窗 (a1b8654)
- fix(quick-260505-fjq-02): 前端时区选择改为固定标准偏移 (ed0f63d)

### Runtime & Deployment
- fix(quick-260504-elo-01): image.lock 中 managed-user 镜像 tag 改为 latest (d1cbdf9)
- feat(quick-260504-dtd-02): default-pull sing-box-gateway and avoid restart loop (97d59a6)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v3.3.1...v3.3.2


## v3.3.1 - 2026-05-03
## What's Changed

### Backend (Go / API)
- fix(quick-260504-414-01): 修复路径联想——按父目录查询+前缀过滤 (335bead)
- feat(quick-260504-414-01): implement GET /v1/admin/host-files API with security filters (9754c3e)

### Frontend (Admin Web)
- feat(quick-260504-414-02): 新建主机对话框 UX 优化 (02ef622)
- fix(quick-260504-414-01): 修复路径联想——按父目录查询+前缀过滤 (335bead)
- feat(quick-260504-414-03): replace host path inputs with PathAutocomplete in dialogs (4c482a1)
- feat(quick-260504-414-02): add PathAutocomplete component and useHostFiles hook (da36a2b)
- fix(quick-260504-2n4-02): 修复挂载管理目标路径行潜在溢出 (bf1c526)
- fix(quick-260504-2n4-01): 修复创建主机对话框挂载路径溢出 (e208501)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v3.3.0...v3.3.1


## v3.3.0 - 2026-05-04
## What's Changed

### Database / Migrations
- feat(creds): 0018 用户中心化凭据迁移 — 反向回填 users.entry_password、一用户一活跃 host 硬约束、DROP hosts.entry_password (e232473)

### Backend (Go / API)
- feat(creds): A2 用户中心化凭据后端重构 — admin_users.Create 生成 entry_password + ed25519 SSH 密钥对，新增 POST /admin/users/{id}/credentials/regenerate；admin_hosts.Create 加入用户已有活跃主机 → 409 拒绝；删除 GetHostByShortID/GetUserByShortID 死代码 (38fad3c)
- fix(probe): B3 IP 探测改用本地 sing-box gateway 镜像（CLOUD_CLI_PROXY_GATEWAY_IMAGE 默认 cloud-cli-proxy-sing-gateway:local），移除冗余 run -c 参数 (7ab8da7)

### Frontend (Admin Web)
- feat(creds): A3 用户中心化凭据 UI — 创建用户对话框一次性展示 SSH 公私钥 / 密码 / 指纹；用户详情页新增"重新生成 SSH 凭据"按钮 + 二次确认 + 凭据展示双对话框；移除主机创建对话框中的 SSH 凭据块与重置主机 SSH 密码按钮 (2805c51)

### Runtime & Deployment
- feat(runtime): B1 工作容器 + sing-box gateway 容器统一加 --restart=unless-stopped，宿主机重启自动恢复 (94b79b7)
- feat(image): B2 删除 claude-wrapper.sh，managed-user 镜像 binary 直接落到 /usr/local/bin/claude，顺带修复 fallback 路径 CLAUDE_BIN 找不到的潜在问题 (060edae)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v3.2.6...v3.3.0

## v3.2.6 - 2026-05-02
## What's Changed

### Frontend (Admin Web)
- feat: 前端端口映射管理 UI (a54fb0f)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v3.2.5...v3.2.6


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

