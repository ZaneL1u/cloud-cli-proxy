# Requirements: Cloud CLI Proxy

**Defined:** 2026-04-14
**Core Value:** 给每个用户提供一台开箱即用的 SSH 云主机，并且严格保证其所有出网流量都走受控的指定出口 IP

## v2.0 Requirements

Requirements for cloud-claude 透明远程 CLI 里程碑。Each maps to roadmap phases.

### CLI 基础

- [x] **CLI-01**: 用户可以运行 `cloud-claude`（无参数）透明启动远端容器并进入 Claude Code 交互会话
- [x] **CLI-02**: 用户可以运行 `cloud-claude init` 配置网关地址和凭证，持久化到 `~/.cloud-claude/config.yaml`
- [x] **CLI-03**: 用户传入的所有 claude 参数原样透传到容器内 Claude Code
- [x] **CLI-04**: 网关不可达、认证失败、容器未就绪等场景给出清晰的中文错误提示
- [x] **CLI-05**: 用户可以配置自有网关地址，支持私有部署场景

### 终端体验

- [x] **TTY-01**: 终端窗口 resize 时 SIGWINCH 正确传递到容器内进程
- [x] **TTY-02**: Ctrl+C / Ctrl+\ 等信号正确转发到容器
- [x] **TTY-03**: 容器退出码透传给本地 CLI 进程

### 目录映射

- [ ] **MAP-01**: 用户当前目录自动映射到容器 /workspace，通过 sshfs slave 实现
- [ ] **MAP-02**: 映射为双向实时读写，本地改动容器内即时可见，反之亦然
- [ ] **MAP-03**: 会话结束时自动清理容器内挂载点和相关资源

### 服务端支撑

- [x] **SRV-01**: 容器镜像预装 sshfs + fuse3 并配置 user_allow_other
- [x] **SRV-02**: 容器创建时附加 `--device /dev/fuse` + `--cap-add SYS_ADMIN`
- [x] **SRV-03**: SSH Proxy 保持零改造，利用现有多 session channel + exec 转发能力
- [ ] **SRV-04**: 在 Linux 生产环境验证 FUSE + AppArmor/seccomp 兼容性

## v2.x Requirements

Deferred to future release. Tracked but not in current roadmap.

### 增强功能

- **ENH-01**: Mutagen 备选目录映射路径（当 sshfs 性能不足时切换）
- **ENH-02**: 端口转发支持（本地 ↔ 容器端口映射）
- **ENH-03**: SSH 连接池/复用优化（延迟成为主诉时）
- **ENH-04**: 大目录 ignore 策略（node_modules 等排除）
- **ENH-05**: 异常退出时终端 raw 状态恢复保障

## Out of Scope

| Feature | Reason |
|---------|--------|
| WebSocket 自定义隧道 | 仅在 SSH 不可穿透时才考虑，v2.0 不需要 |
| 多宿主调度/负载均衡 | v2.0 仍为单宿主机形态 |
| GUI / 图形界面 | CLI 工具不需要图形界面 |
| Windows 原生支持 | 优先 macOS/Linux，Windows 通过 WSL |
| 静默降级到本地 claude | 与产品承诺矛盾，本地模式是 v1.3 范围 |
| 容器生命周期管理 CLI | 已有管理后台，不重复 |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| SRV-01 | Phase 24 | Complete |
| SRV-02 | Phase 24 | Complete |
| SRV-03 | Phase 24 | Complete |
| CLI-01 | Phase 25 | Complete |
| CLI-02 | Phase 25 | Complete |
| CLI-04 | Phase 25 | Complete |
| CLI-05 | Phase 25 | Complete |
| CLI-03 | Phase 26 | Complete |
| TTY-01 | Phase 26 | Complete |
| TTY-02 | Phase 26 | Complete |
| TTY-03 | Phase 26 | Complete |
| MAP-01 | Phase 27 | Pending |
| MAP-02 | Phase 27 | Pending |
| MAP-03 | Phase 27 | Pending |
| SRV-04 | Phase 28 | Pending |

**Coverage:**
- v2.0 requirements: 15 total
- Mapped to phases: 15 ✓
- Unmapped: 0

---
*Requirements defined: 2026-04-14*
*Last updated: 2026-04-15 after roadmap creation*
