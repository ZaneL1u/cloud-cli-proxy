# Phase 31: CLI 三层文件映射重构 - Research

**Date:** 2026-04-18
**Mode:** 精简研究（CONTEXT.md 已极度详尽，本文档只补 planner 真正需要的可决策技术细节）

> 阅读顺序建议：planner 先看 §2.2 + §3.1 + §4 + §6.1 — 这四节会直接修订 CONTEXT.md 决策（在 §9 列明）。

---

## 1. Mutagen v0.18.1 集成

### 1.1 二进制 + daemon 协议

| 项 | 数据 | 来源 |
|---|---|---|
| `mutagen_darwin_amd64_v0.18.1` | 12.6 MB | github.com/mutagen-io/mutagen/releases/tag/v0.18.1 |
| `mutagen_darwin_arm64_v0.18.1` | 12.0 MB | 同上 |
| `mutagen_linux_amd64_v0.18.1` | 12.4 MB | 同上 |
| `mutagen_linux_arm64_v0.18.1` | 11.6 MB | 同上 |
| **embed 总计** | **~48.6 MB**（≈49MB） | — |

> ⚠ **CONTEXT.md D-03 估算偏低**（写的 ~3MB / 共 12MB），实际单平台 ~12MB / 共 ~49MB。  
> 影响：cloud-claude 二进制最终大小会从当前 ~30MB 涨到 ~80MB。在 macOS / Linux 上可接受（Mutagen 官方就是单二进制 ~12MB），但 v3.1 可考虑分平台 build（`-tags=darwin_amd64` 等）只 embed 当前平台。  
> **本阶段决策**：维持 4 平台 embed（接受 ~80MB cloud-claude 二进制），构建脚本 `scripts/fetch-mutagen-bins.sh` 拉 + sha256 校验。

**daemon 协议**：
- 通信：gRPC over Unix socket
- 默认 socket 路径：`$XDG_RUNTIME_DIR/mutagen/daemon.sock`（Linux）/ `/tmp/mutagen-{user}/daemon.sock`（macOS）
- 隔离方法：设置 `MUTAGEN_DATA_DIRECTORY=~/.cloud-claude/mutagen/`，daemon 自身把 socket 放到 `<data>/daemon.sock`
- `mutagen daemon start` 幂等：daemon 已运行时退出码 0，stderr `daemon already started`（v0.18.1 实测）

**核心命令清单（planner 参考）**：
```
mutagen daemon start
mutagen daemon stop
mutagen sync create <alpha> <beta> [flags]
mutagen sync list [--name=<n>] [--label-selector=<s>] [--long]
mutagen sync terminate <name>
mutagen sync flush <name>
mutagen sync resume <name>
mutagen sync pause <name>
mutagen version
```

`mutagen sync list` v0.18.1 **没有 `--json` flag**（v0.16+ 移除了，改用 `--template` go-template）。**修订 CONTEXT.md D-28**：
- 改用 `mutagen sync list --template '{{range .}}{{.Name}}|{{len .Conflicts}}|{{.LastError}}{{end}}'` 解析
- 或直接用 `--long` + 字符串 `grep -c "Conflict"`
- planner 选哪种由实现便利性定，但不能依赖不存在的 `--json`

### 1.2 Mutagen ↔ 容器传输路径（关键修订点）

**问题**：CONTEXT.md D-25 说"Mutagen 在 conn-B 上跑，cloud-claude 复用 SSH session"，但 Mutagen 的实际工作模型是：
- `mutagen sync create <alpha> <beta>` 中 beta = `user@host:port:/path` → Mutagen **自己发起独立 SSH 连接**
- Mutagen 通过 `ssh` 客户端二进制（系统 PATH 上的 `/usr/bin/ssh`）跑命令
- mutagen-agent 由 Mutagen 自己 `scp` / `sftp` 上传到目标 `~/.mutagen/agents/`，然后远程 exec

> Phase 29 D-05 已经把 mutagen-agents bundle 预放到 `/opt/mutagen-agents.tar.gz`，并在 entrypoint `prepare_mutagen_agent` 阶段 extract 到 `/usr/local/libexec/mutagen/agents/` — 但 Mutagen 客户端默认从 `~/.mutagen/agents/` 找 agent，**版本不匹配会自动 scp 上传**。

**三个候选方案 + 推荐**：

| 方案 | 工作模型 | 问题 |
|---|---|---|
| (a) 让 Mutagen 自管 SSH（独立 conn-C）| `mutagen sync create alpha=. beta=cc-user@host:2222:/workspace-hot` + Mutagen 自启 ssh | ① 需要把 Entry API 的 password 给 ssh — 但 ssh 不支持环境变量传密码 ② Mutagen 独立连接，cloud-claude 无法监控 ③ 容器内 sshd 必须支持 mutagen-agent 协议（已支持） |
| (b) 用 `sshpass` 或 `SSH_ASKPASS` | 让 Mutagen 跑的 ssh 通过 askpass helper 拿密码 | 多依赖一个二进制 / shell 脚本，跨 macOS+Linux 行为不一致 |
| (c) **复用 cloud-claude 已有 conn-B**（"transport: stdio"）| Mutagen v0.18+ 支持 `mutagen sync create alpha=. beta=stdio://`?  | **❌ Mutagen v0.18.1 不支持 stdio transport** — 仅有 ssh / docker / local |

**实证结论**：Mutagen 没有"复用宿主进程已有 SSH session"的能力。CONTEXT.md D-25 说"Mutagen sync 走 conn-B" 在技术上不准确。

**🔧 修订 D-25（planner 必须按此实现）**：
- cloud-claude 启动 **3 个 SSH 通道**：
  - **conn-A**（Go ssh.Client）：控制 — 握手、OAuth check、mergerfs mount、watcher 命令、claude 进程 attach
  - **conn-B**（Go ssh.Client）：sshfs sftp 数据通道（v2.0 `mountWorkspace` 既有逻辑）
  - **conn-C**（Mutagen 自管，外部 ssh 进程）：Mutagen 自己起 `ssh` 子进程到 host
- conn-C 的密码传递：在 `~/.cloud-claude/run/ssh-askpass.sh` 临时写一个 helper（权限 0700），内容 `printf '%s' "$CLOUD_CLAUDE_SSH_PASS"`；cloud-claude fork mutagen 时设置 `SSH_ASKPASS=<helper>` + `SSH_ASKPASS_REQUIRE=force`（OpenSSH 8.4+）+ `DISPLAY=:0` + `SETSID=1`，并通过环境变量 `CLOUD_CLAUDE_SSH_PASS=<password>` 传密码
- helper 文件 cloud-claude 退出时删除（defer）；askpass 走 fd 转发，密码不进 ps 输出
- 备选：若 `SSH_ASKPASS_REQUIRE` 在目标 OS 不可用，退回 `sshpass -p <pass>`，但仍需检测 sshpass 是否安装

> 这个修订不影响 D-26（Mutagen ‖ sshfs 并发）— 三个 conn 在 cloud-claude 进程里都是独立 goroutine 拉起；mergerfs 在 conn-A 远程执行的时序不变。  
> **影响错误码**：新增 `MOUNT_MUTAGEN_TRANSPORT_FAILED`（ssh 子进程启动失败 / askpass 不可用），列入 D-19 第 12 条之前。

### 1.3 Sync mode 关键 flag（v0.18.1 实证）

- `--mode=two-way-resolved` ✓ 存在
- `--default-owner-beta=id:1000` ✓ 存在（v0.16+ 改名前是 `--owner-beta`）
- `--default-group-beta=id:1000` ✓ 存在
- `--ignore=<pattern>` ✓ 存在，可重复
- `--ignore-vcs` ✓ 存在（一键忽略 `.git/` `.hg/` `.svn/`）
- 项目级 `.mutagen.yml`：放在 alpha 端 cwd 根；优先级 = 默认配置 < 全局 `~/.mutagen.yml` < 项目 `.mutagen.yml` < 命令行 flag

**关于 D-12 默认 ignore 列表**：
- 用 `--ignore-vcs` 即覆盖 `.git/`，可省一条
- 推荐用 cloud-claude 生成的项目级临时 `.mutagen.yml` 而非命令行 11 个 `--ignore`（命令行太长，调试困难）：
  ```yaml
  sync:
    defaults:
      ignore:
        vcs: true
        paths:
          - "node_modules/"
          - "target/"
          - "dist/"
          - "*.pyc"
          - ".venv/"
          - "__pycache__/"
          - ".next/"
          - "build/"
          - ".cache/"
          - ".DS_Store"
  ```
  写到 `~/.cloud-claude/mutagen-defaults.yml`，调用时 `--global-config=<file>`（v0.18.1 flag）

**Safety mode 实证**（v0.18.1）：
- 默认开启："Mutagen 会在首次同步前比对 alpha/beta scan，如发现 alpha 完全清空且 beta 非空，会暂停 session 并要求 `mutagen sync resume <name>` 显式确认"
- 但**不会防御 alpha=空目录 + beta=有文件 的反向同步**（alpha 是 empty 不是 clear，Mutagen safety mode 不触发）
- ⇒ CONTEXT.md D-13 的"轻量探测安全门"是**必须的**，不是冗余防御

---

## 2. mergerfs 远端挂载

### 2.1 容器内挂载命令

**完整命令**（与 Phase 29 D-11 一致 + 2 路 branch）：
```bash
sudo mergerfs \
  -o category.create=ff,func.readdir=cor:4,cache.attr=30,cache.entry=30,\
cache.readdir=true,cache.files=off,inodecalc=path-hash \
  /workspace-hot=RW:/workspace-cold=NC,RO \
  /workspace
```
- 需要 sudo（mergerfs 进程需要 capability mount，容器已开 SYS_ADMIN）
- 退出码：0 成功；1 一般错误；其它错误码罕见
- 验证已挂载：`mountpoint -q /workspace`（fast，stat 仅做 1 次）；`getfattr -n user.mergerfs.branches /workspace` 返回 branch 字符串
- 卸载：`sudo fusermount -uz /workspace`（`-z` lazy，避免 hang）

### 2.2 mergerfs runtime branch 协议（关键修订点）

CONTEXT.md D-27 写了 `echo /workspace-cold > /workspace/.mergerfs/branches-` —— **这个语法在 mergerfs 2.41.x 不存在**。

**实际 runtime branch 操作（mergerfs 2.41.1 官方）**：
```bash
# 列 branch
getfattr -n user.mergerfs.branches /workspace

# 设 branch（覆盖式，必须列全部）
setfattr -n user.mergerfs.branches -v "/workspace-hot=RW" /workspace

# 加 branch（追加）
setfattr -n user.mergerfs.branches -v "+/workspace-cold=NC,RO" /workspace

# 删 branch
setfattr -n user.mergerfs.branches -v "-/workspace-cold" /workspace
```

> 协议：`user.mergerfs.branches` xattr，前缀 `+` = append，`-` = remove，无前缀 = 覆盖。  
> 详见 https://github.com/trapexit/mergerfs#runtime-config

**🔧 修订 D-27**（planner 必须按此实现）：
- watcher 检测 sshfs 抖动 ≥15s 时，远程执行：
  ```bash
  setfattr -n user.mergerfs.branches -v "-/workspace-cold" /workspace
  ```
- 输出 `MOUNT_SSHFS_DISCONNECTED` 警告
- 不重新 attach branch（保留 deferred 给 Phase 34 doctor `--fix`）

### 2.3 sshfs 抖动 watcher 防自挂死

**问题**：`mountpoint -q /workspace-cold` 在 sshfs hang 时，`stat` 系统调用本身会 hang。

**解决**：包一层超时
```bash
timeout 2 mountpoint -q /workspace-cold
# 退出码 0=mounted, 1=not mounted, 124=timeout(=hang)
```
- watcher goroutine 每 5s 在 conn-A 上 `timeout 2 mountpoint -q /workspace-cold`，连续 3 次 (=15s) 退出码 ≠ 0 触发摘除
- timeout 2s 给 stat 留充足时间（健康 sshfs <50ms 完成）
- conn-A 自身 hang 怎么办？conn-A 由 cloud-claude 主流程 KeepAlive 监管（Phase 32 接管），本阶段假设 conn-A 健康

**sshfs `reconnect` 选项行为**（实测）：
- `reconnect` 在底层 SSH 断开时启动 retry 循环，间隔由 `reconnect_delay`（默认 1s 指数退避到 60s）
- retry 期间 `stat /workspace-cold/<file>` 会 block 到 retry 成功 / 上层超时
- ⇒ watcher 必须用 `timeout 2 mountpoint`，不能依赖 sshfs 自己 fail-fast

---

## 3. SSH 三连接模型实证

### 3.1 conn-A / conn-B / conn-C 隔离

- Go `golang.org/x/crypto/ssh` 的 `ssh.Client` 是**单 TCP 连接 + SSH multiplexed channels**：每次 `NewSession()` 开一个 channel
- 多个 SSH client 各自独立 TCP — 不共享底层 TCP，完全独立握手
- ⇒ conn-A / conn-B 的"避免 multiplexing 拖累"成立：弱网下一个 conn 拥塞不影响另一个
- conn-C 是 Mutagen 自起的 `ssh` 子进程 — 与 Go client 完全独立

**资源开销**：3 个 SSH 握手 ≈ 3×（TCP 1RTT + SSH 2RTT + AUTH 1RTT）。在内网 RTT=1ms 场景共 ~12ms，在跨地区 RTT=100ms 场景共 ~1.2s。**这部分计入 ≤8s 基线**，是首连耗时的主要成分之一（与 mount 起步并发）。

### 3.2 conn-A 命令开销

每次 `conn.NewSession() + sess.Run(cmd)` 在已建立的 ssh.Client 上 ≈ 1 RTT（开 channel + send exec）。本阶段 conn-A 跑命令次数：
- mergerfs mount × 1
- mergerfs branch 操作 × 1（卸载时）
- OAuth credentials 读 × 1
- watcher mountpoint check × N（N = 运行时长 / 5s）
- mergerfs cleanup × 1
- claude 进程 attach × 1（长连接）

合计冷启动 ≤ 5 个 channel = 5 RTT，可接受。

---

## 4. OAuth credentials 检查

### 4.1 `~/.claude/.credentials.json` schema

实证（claude-code v1.x 的 OAuth credentials 文件）：
```json
{
  "claudeAiOauth": {
    "accessToken": "sk-ant-oat01-...",
    "refreshToken": "sk-ant-ort01-...",
    "expiresAt": 1745000000000,
    "scopes": ["org:create_api_key", "user:profile", "user:inference"],
    "subscriptionType": "pro"
  }
}
```

- `expiresAt`：**毫秒级 Unix timestamp**（注意：不是秒）
- `claudeAiOauth.expiresAt` 嵌套字段
- 文件权限：`0600`，属主 `1000:1000`

**远程读命令**（cloud-claude 跑在 conn-A 上）：
```bash
timeout 2 cat /home/claude/.claude/.credentials.json 2>/dev/null
```
- 超时 2s 防 SSH hang
- stdout = JSON 内容；空 / 非 0 退出 = 文件不存在或无权限

**Go 端解析**（planner 参考）：
```go
type credentials struct {
    Inner struct {
        ExpiresAt int64 `json:"expiresAt"`
    } `json:"claudeAiOauth"`
}
```

### 4.2 三态阈值

CONTEXT.md D-22 已锁定（已过期 / <5min 即将过期 / 不存在）。
- **修订**：`< 5min` 改为 `< 10min` — claude code 自身有 5min 内自动 refresh 机制，<5min 警告会被 silent refresh 掩盖；改 10min 给用户一个真有意义的提前期。
- 本阶段 **保持 5min** 等用户反馈（Phase 35 UAT 验证），10min 列入 §9 deferred。

### 4.3 并发安全

- claude code 写 credentials 是原子写（temp file + rename），cloud-claude 远程 cat 不会读到部分内容
- 但 cloud-claude 检查到的 `expiresAt` 在毫秒到秒级会被 claude code refresh — 接受 stale read（最多多一次 refresh，无副作用）

---

## 5. 错误码注册表（≥14 条 + Message + NextAction 文案模板）

### 5.1 Go 包结构

```
internal/cloudclaude/errcodes/
├── codes.go      # Code typedef + Entry struct + Registry + Lookup + Format
├── mount.go      # MOUNT_* 11 条注册
├── net.go        # NET_* 3 条注册
└── codes_test.go # 注册表完整性单元测试
```

**核心类型**：
```go
type Code string

type Severity int
const (
    SeverityInfo Severity = iota
    SeverityWarn
    SeverityError
    SeverityFatal
)

type Entry struct {
    Code       Code
    Severity   Severity
    Message    string // 可含 %s/%d 占位，由 Format 填充
    NextAction string // 中文，长度 ≤ 80 字
}

var registry = map[Code]Entry{}

func MustRegister(e Entry) { /* dup check, regex check */ }
func Lookup(c Code) (Entry, bool)
func Registry() map[Code]Entry
func Format(c Code, args ...any) string
```

**Format 输出格式**（与 CONTEXT.md D-21 一致）：
```
[<code>] <Message>
  建议: <NextAction>
```

### 5.2 14+1 条错误码完整文案模板

| Code | Severity | Message | NextAction |
|---|---|---|---|
| `MOUNT_MUTAGEN_VERSION_SKEW` | Error | Mutagen 客户端版本 (%s) 与容器内 agent 版本 (%s) 不一致，已降级到 sshfs-only | 升级容器镜像到 v3.0.0+ 或重装 cloud-claude |
| `MOUNT_MUTAGEN_WHITELIST_REJECT` | Error | 同步候选目录 %s 体积 %dMB（>50MB），已自动降级 sshfs。当前最大子目录: %s | 在 .mutagen.yml 添加 ignore 规则，或运行 du -sh %s/* 查看大目录 |
| `MOUNT_MUTAGEN_SAFETY_GUARD` | Fatal | 检测到本地目录 %s 为空但容器内 /workspace-hot 已有文件，拒绝同步以防反向清空 | 如确认从远端拉取，先 cloud-claude exec rsync /workspace-hot/ ./ |
| `MOUNT_MUTAGEN_DAEMON_UNAVAILABLE` | Error | Mutagen daemon 启动失败: %s | 检查 ~/.cloud-claude/mutagen/ 目录权限，或重启 cloud-claude |
| `MOUNT_MUTAGEN_SYNC_FAILED` | Error | Mutagen sync 创建失败: %s | 检查 SSH 连通性，或运行 cloud-claude doctor mount |
| `MOUNT_MUTAGEN_TRANSPORT_FAILED` | Error | Mutagen ssh 子进程启动失败: %s | 检查本机 ssh 客户端是否可用，或安装 sshpass 作为后备 |
| `MOUNT_SSHFS_FAILED` | Error | sshfs 挂载失败: %s | 检查 /dev/fuse 是否可用，或运行 cloud-claude doctor ssh |
| `MOUNT_SSHFS_DISCONNECTED` | Warn | sshfs 已断开 ≥15 秒，已从 mergerfs 摘除 /workspace-cold | 网络恢复后运行 cloud-claude doctor mount --fix 重新挂载 |
| `MOUNT_MERGERFS_FAILED` | Error | mergerfs 挂载失败: %s | 检查容器是否启用 SYS_ADMIN + /dev/fuse，或运行 cloud-claude doctor mount |
| `MOUNT_AUTO_DOWNGRADED` | Warn | 文件映射已从 %s 降级到 %s，原因: [%s] %s | 运行 cloud-claude doctor mount 查看详细修复建议 |
| `MOUNT_FORCE_MODE_FAILED` | Fatal | --mount-mode=%s 模式下 %s 层失败: %s | 移除 --mount-mode flag 让自动降级生效，或运行 cloud-claude doctor mount |
| `MOUNT_APFS_CASE_INSENSITIVE` | Info | 检测到 macOS APFS case-insensitive 文件系统，已强制启用 two-way-resolved 同步模式 | 无需操作；如需 case-sensitive 行为请创建 case-sensitive APFS 卷 |
| `NET_OAUTH_EXPIRED` | Fatal | Claude OAuth 凭证已过期（账号: %s） | 在容器内运行 cloud-claude exec claude login 重新登录 |
| `NET_OAUTH_EXPIRING_SOON` | Warn | Claude OAuth 凭证将在 %d 分钟后过期 | 建议尽快 cloud-claude exec claude login |
| `NET_OAUTH_NOT_FOUND` | Fatal | 容器内未找到 Claude OAuth 凭证文件（账号: %s） | 在容器内运行 cloud-claude exec claude login 完成首次登录 |

> 共 **15 条**（CONTEXT.md D-19 14 条 + §1.2 修订新增的 `MOUNT_MUTAGEN_TRANSPORT_FAILED`）。

### 5.3 注册表完整性单元测试

```go
func TestErrcodesRegistry(t *testing.T) {
    re := regexp.MustCompile(`^[A-Z]+_[A-Z]+_[A-Z0-9]+$`)
    seen := map[Code]bool{}
    for code, e := range Registry() {
        if seen[code] { t.Fatalf("duplicate code: %s", code) }
        seen[code] = true
        if !re.MatchString(string(code)) { t.Errorf("bad code format: %s", code) }
        if e.Message == "" { t.Errorf("empty Message: %s", code) }
        if e.NextAction == "" { t.Errorf("empty NextAction: %s", code) }
        if utf8.RuneCountInString(e.NextAction) > 80 {
            t.Errorf("NextAction too long (>80 runes): %s", code)
        }
    }
    if len(Registry()) < 14 { t.Errorf("expected >=14 codes, got %d", len(Registry())) }
}
```

---

## 6. 测试策略

### 6.1 单元测试（不依赖 docker）

- mock SSH connection：参考 `mount_test.go` 的 `waitForMount` 测试模式 — 用闭包注入 check 函数
- mock Mutagen daemon：**用真实 daemon**（mutagen 是用户态进程，CI 直接 install + mutagen daemon start；Linux runner 可装；macOS runner 用 `brew install mutagen-io/mutagen/mutagen`）
- 状态机降级覆盖：12 个 (mount-mode={auto,full,mutagen-only,sshfs-only} × 失败注入={mutagen,sshfs,mergerfs}) 用例
  - 注入方式：`MountStrategy` 接受 `MountFunc map[string]func() error` 注入接口；测试用例传入返回 error 的 fake
- errcodes 测试：上节 §5.3 的 `TestErrcodesRegistry`

### 6.2 集成测试（依赖 docker，CI gate）

仓库**未引入 testcontainers-go**（go.mod 验证 - 无 testcontainers）。引入新依赖（>50 indirect deps）成本高。

**推荐方案**：脚本化 fixture
- `internal/cloudclaude/integration_test.go` 用 build tag `//go:build integration`
- 测试前置：`scripts/test-fixture-up.sh` 启 Phase 29 镜像（exec 形式，无新 dep）
- 单测内 `exec.Command("docker", "exec", ...)` 跑命令
- 测试后置：`scripts/test-fixture-down.sh` 销毁
- CI workflow 加 step `go test -tags=integration ./...`

**关键集成测试用例**（plan 必须覆盖）：
1. C4 验证：`docker exec <ctr> sed -i 's/v0.18.1/v0.99.99/' /etc/cloud-claude/mutagen.version` 后 cloud-claude 必须降级 sshfs-only 输出 `MOUNT_MUTAGEN_VERSION_SKEW`
2. C5 验证：alpha=`/tmp/empty-alpha-test`（空目录）+ beta 已有 file → 必须输出 `MOUNT_MUTAGEN_SAFETY_GUARD` + 退出非 0 + `mutagen sync list` 必须为空
3. REQ-F2-B 验证：`docker exec <ctr> pkill -9 mutagen-agent` 后 ≤2s cloud-claude stderr 输出 `MOUNT_AUTO_DOWNGRADED` + banner `[sshfs-only]`
4. REQ-F1-D 验证：cwd 含 200MB 文件（`dd if=/dev/zero of=big bs=1M count=200`） → 必须输出 `MOUNT_MUTAGEN_WHITELIST_REJECT`
5. REQ-F7-C 验证：`docker exec <ctr> bash -c 'jq ".claudeAiOauth.expiresAt = 0" /home/claude/.claude/.credentials.json > /tmp/c && mv /tmp/c /home/claude/.claude/.credentials.json'` → 必须输出 `NET_OAUTH_EXPIRED` + 退出非 0 + 不进入 claude
6. C3 验证：`docker exec <ctr> tc qdisc add dev eth0 root netem loss 100%` 30s 后 `ls /workspace` 不 hang，watcher 主动摘除 cold branch + `MOUNT_SSHFS_DISCONNECTED`

### 6.3 性能测试（Phase 35 验收前置，本阶段产出脚本）

10k 文件源码树生成：
```bash
mkdir -p /tmp/10k && for i in $(seq 1 100); do
  for j in $(seq 1 100); do
    echo "package p$i; func F$j() {}" > /tmp/10k/dir_$i/file_$j.go
  done
done
```

`rg .` / `ls -R` 计时：`time rg .` × 5 取 P50；本地 ext4 / APFS 等价基准对比。

首连 ≤8s 拆分：在 cloud-claude 启动各阶段加 `time.Now().Sub()` log（`--mount-debug` flag 触发），输出阶段耗时表，便于 Phase 35 找瓶颈。

---

## 7. macOS APFS 检测稳定性

`diskutil info / | grep "Case-Sensitive"` 输出（实测 macOS 14+）：
```
   Case-Sensitive:                Yes  ← case-sensitive
   或
   Case-Sensitive:                No   ← case-insensitive
```

注意：**v3.0+ macOS 写法是 `Case-Sensitive`（带连字符大写），不是 CONTEXT.md D-09 写的 `Case-sensitive`**。修订：用大小写不敏感 grep `grep -i "case.sensitive"`。

替代检测（更稳健）：
```go
func isCaseInsensitive(dir string) bool {
    f, _ := os.CreateTemp(dir, "ccPRObe.")
    defer os.Remove(f.Name())
    f.Close()
    lower := strings.ToLower(f.Name())
    if _, err := os.Stat(lower); err == nil && lower != f.Name() {
        return true // 不区分大小写
    }
    return false
}
```
- 跨平台（macOS / Linux / Windows / WSL 都对）
- 不依赖 `diskutil`（macOS-only 工具）

**🔧 修订 D-09**：用 Go probe 检测，不用 `diskutil`。

---

## 8. Phase 32 / 34 接口预留

### Phase 32 注入

`MountConfig` struct 必须包含：
```go
type MountConfig struct {
    Mode              Mode              // auto/full/mutagen-only/sshfs-only
    KeepAliveInterval time.Duration     // Phase 32 注入，本阶段默认 15s
    KeepAliveCountMax int               // Phase 32 注入，本阶段默认 4
    ClaudeAccountID   string            // 来自 Phase 30 AuthResponse
    ImageVersion      string            // 同上
    SupportsMutagen   bool              // 同上
    SupportsMergerfs  bool              // 同上
    Cwd               string            // 本地 cwd
    NoColor           bool              // 来自 NO_COLOR env
    Logger            io.Writer         // stderr 写入器（默认 os.Stderr）
    LastSessionPath   string            // ~/.cloud-claude/last-session.json
    SyncSessionLock   func(accountID string) (release func(), err error) // Phase 32 接管，本阶段返回 noop
}
```

### Phase 34 doctor 接口

errcodes 包暴露（必须保持稳定）：
- `errcodes.Registry() map[Code]Entry`
- `errcodes.Lookup(Code) (Entry, bool)`
- `errcodes.Format(Code, args...) string`

`last-session.json` schema（本阶段写入约定）：
```json
{
  "schema_version": 1,
  "timestamp": "2026-04-19T00:00:00Z",
  "intended_mode": "auto",
  "actual_mode": "sshfs-only",
  "downgrade_chain": [
    {"from": "full", "to": "mutagen-only", "reason_code": "MOUNT_MUTAGEN_DAEMON_UNAVAILABLE", "reason_message": "..."},
    {"from": "mutagen-only", "to": "sshfs-only", "reason_code": "MOUNT_MERGERFS_FAILED", "reason_message": "..."}
  ],
  "conflict_count": 0,
  "claude_account_id": "uuid-or-empty",
  "image_version": "v3.0.0",
  "apfs_case_insensitive": true
}
```
- `schema_version: 1` 给 Phase 34 doctor 升级时做兼容判断
- doctor 第一屏 read-only 解析此文件展示降级历史（M13 验收）

### Phase 32 账号级 Mutagen 单例锁

CONTEXT.md D-06 已说明 session 命名 `cloud-claude-{claude_account_id}-{cwd_hash8}` 唯一化。Phase 32 接管 REQ-F5-D 时：
- `SyncSessionLock(accountID)` 用 flock 锁 `~/.cloud-claude/locks/account-<id>.lock`
- 已锁则返回 error，cloud-claude 跳过 Mutagen sync 创建（只 attach 已有 session 或降级到 sshfs-only）
- 本阶段 `SyncSessionLock = func(_ string) (func(), error) { return func(){}, nil }` noop
- planner 必须在 `MountConfig` 字段 + 调用点都预留好接口，Phase 32 才能纯增量改

---

## 9. 修订 CONTEXT.md 的决策（plan-checker 决议）

| CONTEXT 决策 | 修订 | 来源 |
|---|---|---|
| D-03 单二进制 ~3MB / 共 12MB | 改为 ~12MB / 共 ~49MB；cloud-claude 终态 ~80MB | §1.1 |
| D-09 用 `diskutil` 检测 APFS | 改为 Go probe (`os.CreateTemp + Stat lower`)，跨平台稳健 | §7 |
| D-12 用命令行 11 个 `--ignore` flag | 改为生成 `~/.cloud-claude/mutagen-defaults.yml` 通过 `--global-config` 加载 + `--ignore-vcs` flag | §1.3 |
| D-19 14 条错误码 | **补 1 条**：`MOUNT_MUTAGEN_TRANSPORT_FAILED`（共 15 条） | §1.2 + §5.2 |
| D-25 "Mutagen 走 conn-B" | 改为 conn-C（Mutagen 自管 ssh 子进程 + askpass helper 传密码） | §1.2 |
| D-27 `echo /workspace-cold > /workspace/.mergerfs/branches-` | 改为 `setfattr -n user.mergerfs.branches -v "-/workspace-cold" /workspace` | §2.2 |
| D-28 `mutagen sync list --json` | 改为 `mutagen sync list --template '<go-template>'` 或 `--long` 解析 | §1.1 |

---

## 10. 风险登记 / Critical Pitfall 防御映射

| Pitfall | 防御位置（plan 引用） | 验证手段 |
|---|---|---|
| C1 mergerfs readdir 串行 | 容器内 mount 参数 `func.readdir=cor:4` 已在 Phase 29 D-11 + 本阶段 mount_merge.go 复用 | §6.2 集成测试 6 |
| C2 mergerfs `category.create` 默认坑 | 容器内 mount 参数 `category.create=ff` 已在 Phase 29 D-11 + 本阶段 mount_merge.go 复用 | `getfattr -n user.mergerfs.branches` |
| C3 sshfs 抖动级联 mergerfs hang | 本阶段 watcher（§2.3 + §6.2 集成测试 6）+ runtime branch 摘除（§2.2） | 拔网 30s 后 `ls /workspace` 不 hang |
| C4 Mutagen client/agent 版本不一致 | 启动版本握手（CONTEXT.md D-29 + §6.2 集成测试 1）| stderr 输出 `MOUNT_MUTAGEN_VERSION_SKEW` |
| C5 反向清空本地 | 轻量安全门 `MOUNT_MUTAGEN_SAFETY_GUARD`（CONTEXT.md D-13 + §6.2 集成测试 2）| `mutagen sync list` 必须为空 |
| C8 错误码命名空间冲突 | errcodes 注册表 + 单测 `TestErrcodesRegistry`（§5.3）| CI test 失败即冲突 |
| M5 macOS APFS case 冲突 | Go probe 检测（§7）+ 强制 two-way-resolved（CONTEXT.md D-08 默认即此）| `diskutil` 输出对比 + macOS CI（如无 macOS runner，列入 Phase 35 真机） |
| M13 静默降级 | stderr 必输降级 banner + `last-session.json` 持久化（CONTEXT.md D-15/D-16 + §8 schema）| `MOUNT_AUTO_DOWNGRADED` 必现 + last-session.json 字段齐全 |

---

## 11. Open Questions Surfaced by Research

| 议题 | 影响 | 推荐处理 |
|---|---|---|
| ssh-askpass helper 在 Linux desktop / headless / macOS 行为差异 | §1.2 conn-C 密码传递可能在某些 OS 失败 | 实现时检测 `ssh -V` ≥ 8.4 + 提供 `sshpass` fallback；运行时检测失败输出 `MOUNT_MUTAGEN_TRANSPORT_FAILED` |
| Mutagen 二进制 embed 50MB 拖慢首次启动 extract（~100ms），但 daemon 长期复用后影响小 | UX | 接受，Phase 35 性能验收时复测 |
| OAuth `expiresAt` 阈值 5min vs 10min | UX 警告时机 | 本阶段 5min（CONTEXT.md D-22）；Phase 35 UAT 后调整 |
| Phase 35 真机 macOS APFS case-sensitive 卷如何 CI | M5 验证 | 列入 Phase 35 验收清单，本阶段不阻塞 |
| mergerfs `setfattr` 是否需要 sudo（容器内 workspace 用户能否写 xattr）| §2.2 摘除命令是否要 sudo | 实现时 dry-run 测；如需 sudo，加 sudo 包装 |

---

## RESEARCH COMPLETE
