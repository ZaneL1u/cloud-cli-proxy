// Package errcodes — Phase 34 D-02 / D-18：
// 为 cloud-claude explain 子命令提供每个非 informational Code 的长说明。
// ExplainExempt 登记 informational 类（不需要长说明）。
//
//nolint:lll // 长说明字面量不折行
package errcodes

import (
	"fmt"
	"sync"
)

var (
	explainMu            sync.RWMutex
	ExtendedExplanations = map[Code]string{}

	// ExplainExempt：informational 类豁免长说明（Severity==SeverityInfo 的降级提示 / APFS 识别 / *_BACKOFF / *_NOTIFIED 等）。
	// 注意：MOUNT_AUTO_DOWNGRADED 虽属"降级提示"，但 Severity=Warn，已登记长说明而不是放进豁免（Rule 1：和 TestExplainExemptOnlyInformational 协同）。
	ExplainExempt = map[Code]struct{}{
		MOUNT_APFS_CASE_INSENSITIVE:   {},
		SESSION_TAKEOVER_NOTIFIED:     {},
		NET_RECONNECT_BACKOFF:         {},
		STATE_LAST_SESSION_MISSING:    {},
		SSH_VSCODE_SERVER_NOT_RUNNING: {}, // Phase 41: Info 级，用户可能未使用 VS Code
		SSH_FORWARDING_SOCKET_MISSING: {}, // Phase 41: Info 级，forwarding 未建立属正常
	}
)

// registerExplanation 与 MustRegister 同语义防御重复注册。
// 由 init() 调用，问题在进程启动时即暴露（与 MustRegister 对齐）。
func registerExplanation(c Code, text string) {
	if text == "" {
		panic(fmt.Sprintf("errcodes: code %q ExtendedExplanations 不能为空", c))
	}
	explainMu.Lock()
	defer explainMu.Unlock()
	if _, exists := ExtendedExplanations[c]; exists {
		panic(fmt.Sprintf("errcodes: 重复注册 ExtendedExplanations %q", c))
	}
	ExtendedExplanations[c] = text
}

// init 注册所有非 informational 码的长说明。每条 ≥ 200 中文字符，五段模板：
// 触发场景 / 根本原因 / 复现方式（可选）/ 修复路径 / 关联文档。
func init() {
	// ────────────────────────────────────────────────────────────────────
	// MOUNT_* 域（Phase 31）
	// ────────────────────────────────────────────────────────────────────

	registerExplanation(MOUNT_HOT_SYNC_FAILED, `触发场景：自研热同步层在初始全量上传或后续双向轮询同步过程中失败，未能把代码层变更推到远端 hot staging 或从远端拉回本地。
根本原因：常见原因包括当前目录中存在不可读文件、远端 hot staging 目录权限不正确、SFTP channel 被提前关闭、轮询过程中某个文件被并发删除、或隐藏 staging 路径与现有挂载状态冲突。
复现方式：chmod 000 某个源码文件后启动 cloud-claude，或在同步过程中手工删除远端 hot staging 根目录。
修复路径：先确认本地目录与远端 staging 路径均可读写；若是会话残留，重启 cloud-claude 让 staging 目录重建；若仍失败，可临时回退 sshfs-only，随后查看具体 stderr reason 定位。
关联文档：同路径 hot+cold 方案设计`)

	registerExplanation(MOUNT_SSHFS_FAILED, `触发场景：sshfs 命令挂载远端 /workspace 失败（macOS macFUSE 未授权 / Linux fuse 模块未加载 / fusermount3 缺失）。
根本原因：sshfs 是 cloud-claude 的兜底文件映射方案，依赖宿主机上有 macFUSE / fuse3。macOS 升级后 macFUSE 内核扩展常需重新允许；Linux 容器或最小化镜像可能未装 fuse3。
复现方式：sudo kextunload com.osxfuse → cloud-claude → sshfs failed。
修复路径：macOS 到"系统设置 → 隐私与安全性"允许 macFUSE；Linux 安装 fuse3 + sshfs 包；最后运行 cloud-claude doctor mount --fix 重试。
关联文档：Phase 31 RESEARCH §4.4 / .planning/research/PITFALLS.md C6`)

	registerExplanation(MOUNT_SSHFS_DISCONNECTED, `触发场景：sshfs 已挂载但底层 SSH 连接断开 ≥15 秒，mergerfs 已自动把 /workspace-cold 摘除以避免 stale handle。
根本原因：sshfs 的 reconnect 选项不能完美恢复所有场景，断网时间过长就会触发 IO 错误冻结目录树。cloud-claude 检测到后摘除分支保证其它层正常工作。
复现方式：人为 kill -9 <sshd_pid> → 等 15s → mergerfs 自动摘 cold 层。
修复路径：网络恢复后运行 cloud-claude doctor mount --fix 自动 fusermount3 -u + 重新 mount + mergerfs add 回去；或直接重启 cloud-claude 全栈重连。
关联文档：Phase 31 RESEARCH §4.4 / Phase 32 D-04`)

	registerExplanation(MOUNT_MERGERFS_FAILED, `触发场景：mergerfs 挂载 /workspace 失败，无法把 hot（HotSync）+ cold（sshfs）合并成统一视图。
根本原因：常见原因为容器未启用 SYS_ADMIN capability 或 /dev/fuse 设备节点不可用、mergerfs 二进制缺失（很老的镜像）、mergerfs 选项不支持当前内核版本。
复现方式：移除容器 SYS_ADMIN cap → cloud-claude → mergerfs cannot allocate fuse device。
修复路径：升级容器镜像到 v3.0.0+（已默认带 mergerfs 2.41.x + fuse3）；运行 cloud-claude doctor mount 查看具体子项；管理员侧确认 docker run --cap-add SYS_ADMIN --device /dev/fuse 已配置。
关联文档：Phase 31 RESEARCH §4.5`)

	registerExplanation(MOUNT_AUTO_DOWNGRADED, `触发场景：自动 mount 模式下某一层（HotSync / sshfs / mergerfs）启动失败，cloud-claude 降级到下一档（如 HotSync → sshfs-only）。
根本原因：cloud-claude 文件映射是三层栈：HotSync 双向高速 + sshfs 容量层 + mergerfs 合并视图。任一层失败时不阻塞用户使用，自动降级保证基本可用，但会牺牲性能或功能（hot 同步失败后 IDE 反向写需手动 rsync）。
复现方式：docker exec <ctr> mv /usr/local/bin/mergerfs /tmp → cloud-claude → 自动降级到 sshfs-only。
修复路径：观察 stderr 提示的下游错误码（[CODE] 后面的部分）针对修复；运行 cloud-claude doctor mount 看完整健康度；如要锁定模式可用 --mount-mode flag 强制。
关联文档：Phase 31 RESEARCH §6 / Phase 31 PITFALLS M13（禁止静默降级）`)

	registerExplanation(MOUNT_FORCE_MODE_FAILED, `触发场景：用户通过 --mount-mode={full,hot-only,sshfs-only} 强制指定模式，但该模式启动失败。强制模式下 cloud-claude 不会自动降级。
根本原因：用户显式指定模式意味着接受失败即报错的语义（M13 防御静默降级）。常见触发为 hot-only 模式下热同步初始化失败、sshfs-only 模式下 fuse 不可用。
复现方式：cloud-claude --mount-mode=hot-only 但远端同路径目录不可写 → SeverityFatal 阻断。
修复路径：移除 --mount-mode flag 让自动降级生效，或针对错误信息中的具体下游 code 修复；需要锁定模式时建议先运行 cloud-claude doctor mount 看可用层。
关联文档：Phase 31 PLAN <errcode_registry> / Phase 31 PITFALLS M13`)

	registerExplanation(MOUNT_REQUIRE_GIT_REPO, `触发场景：用户在非 git 仓库目录（例如 /tmp、下载目录、家目录下的临时文件夹）直接运行 cloud-claude，启动前的 git rev-parse --show-toplevel 检查返回非零，系统立即拒绝进入挂载流程。
根本原因：cloud-claude 的热同步语义是"同步一个项目"，而不是"同步你当前目录里的所有东西"。如果允许在非仓库目录启动，最危险的情况是把整个家目录、下载目录或临时目录误当成工程同步到远端，既会显著拉长首次扫描时间，也可能把与当前任务无关的大量文件带进容器工作区，重演 Phase 31 讨论过的 cwd 误同步风险。git 仓库边界是当前版本最稳定、最可解释的项目边界信号。
复现方式：执行 cd /tmp && cloud-claude，或 mkdir /tmp/demo && cd /tmp/demo && cloud-claude；由于目录内没有 .git 元数据，命令会直接输出对应错误码并以 exitConfigError 退出，不会继续发起 SSH、SFTP、sshfs 或 mergerfs 相关操作。
修复路径：进入已有 git 仓库目录后重新运行 cloud-claude；如果当前目录本来就是一个新项目，请先执行 git init 建立仓库边界，再启动 cloud-claude；注意这是一道全局前置闸门，任何 --mount-mode 都不能绕过它，因为它保护的是"同步范围正确性"而不是某一种挂载实现细节。
关联文档：.planning/REQUIREMENTS.md REQ-MOUNT-V31-01；.planning/milestones/v3.0-phases/31-cli/31-CONTEXT.md §D-11（目录级熔断）`)

	registerExplanation(MOUNT_OVERSIZED_FILE_SKIPPED, `触发场景：热同步初始化扫描发现某个未被 ignore 规则命中的文件大小已经达到或超过 hot_sync_max_file_mb 阈值（默认 50MB），系统不会把它推入 hot 分支，而是让它继续留在 cold sshfs 分支中提供访问。
根本原因：大文件通常不是需要高频双向实时同步的源码，而更像模型、视频、安装包、构建产物或导出的数据集。把这类文件纳入热同步会明显拖慢首次连接时间、增大本地与远端的传输负担，并放大同步冲突或重试成本。Phase 36 的设计目标不是"禁止读取大文件"，而是"避免为了一个大文件拖垮整个工作区的热同步体验"，因此选择跳过热同步、保留 cold sshfs 兜底这一更稳妥的策略。
复现方式：在一个 git 仓库里放入 60MB 的 model.bin 或 archive.tar，确保它既不在 .gitignore 也不在默认二进制黑名单里，然后启动 cloud-claude。stderr 会出现一次性的跳过提示，last-session.json 中也会记录 oversized_files 列表；文件依然可读，只是不会进入 hot tree。
修复路径：如果你确实希望它参与热同步，可以手动编辑 ~/.cloud-claude/config.yaml 调高 hot_sync_max_file_mb；如果它本就不需要同步，建议把路径加入 .gitignore 或 cloud-claude 的忽略列表，减少启动期提示噪音；如果只是偶尔读取，保持默认配置即可，因为 cold sshfs 仍会提供完整访问，且结合 page cache 后重复读取成本会明显下降。
关联文档：.planning/REQUIREMENTS.md REQ-MOUNT-V31-02 / REQ-MOUNT-V31-03；.planning/milestones/v3.0-phases/31-cli/31-CONTEXT.md §D-11`)

	registerExplanation(MOUNT_GIT_PROXY_DISABLED, `触发场景：cloud-claude doctor mount 在体检本地配置时发现 ~/.cloud-claude/config.yaml 中 proxy_commands 字段没有包含 "git" 字面量，意味着用户后续在 cloud-claude 会话内执行 git 子命令时不会自动走本地侧的命令代理转发，远端容器内的 git 调用将直接命中容器原生网络出口，可能拿不到本地 ssh-agent 凭证或本地 GPG 配置。
根本原因：cloud-claude 默认通过 proxy_commands 列表把若干高频本地交互命令（典型为 git）通过宿主机侧的 socket 反向暴露给远端容器，让远端 git 命令复用本地 SSH 私钥与 GPG 签名能力。如果用户在 init 流程或后续手动编辑 config.yaml 时把 git 从该列表中删除，就只是禁用了这条便利通路，并不是配置文件本身损坏，所以 doctor 只给 Warn 级提示而不是 Fatal。
复现方式：编辑 ~/.cloud-claude/config.yaml，把 proxy_commands 从 ["git"] 改成 ["curl"] 或 [] 后保存；运行 cloud-claude doctor mount 即可看到本条 Warn 出现在 mount 维度的输出中，且其 NextAction 直接指向编辑 proxy_commands 字段。
修复路径：用任意编辑器打开 ~/.cloud-claude/config.yaml，把 proxy_commands 字段改回包含 "git" 的列表（例如 [git] 或 [git, curl]），保存后重启 cloud-claude，再跑一次 cloud-claude doctor mount 确认本条 check 变为 Pass；如果你刻意不想启用 git 代理，可以忽略本条 Warn，但要明白远端 git 子命令需要自带凭证。
关联文档：.planning/phases/36-sshfs/36-REVIEW.md WR-01；internal/cloudclaude/config.go EffectiveProxyCommands；internal/cloudclaude/doctor/mount.go checkGitProxyEnabled`)

	registerExplanation(MOUNT_DEFAULT_IGNORE_DISABLED, `触发场景：cloud-claude doctor mount 检查环境变量 CLOUD_CLAUDE_NO_DEFAULT_IGNORE 时发现其值为 "1"，意味着用户主动关闭了 cloud-claude 内置的默认二进制黑名单。该黑名单负责把常见的二进制文件（编译产物、模型权重、视频、压缩包、字体等）从热同步路径中剔除，关闭后这些文件会重新参与扫描和上传判定。
根本原因：默认二进制黑名单是 Phase 36 引入的体验防御层，用于避免初次同步在大型仓库下被一堆显然不需要双向同步的二进制文件拖慢。少数排查场景下用户可能临时设置 CLOUD_CLAUDE_NO_DEFAULT_IGNORE=1 用来观察「如果不过滤会怎样」，但生产环境长期开启会显著拉升首次扫描时间、放大单文件熔断 oversized 提示数量，并让无意义的二进制变更触发热同步轮询。doctor 给 Warn 级提示提醒用户记得回滚。
复现方式：在 shell 中执行 export CLOUD_CLAUDE_NO_DEFAULT_IGNORE=1，再运行 cloud-claude doctor mount，本条 Warn 会立即出现在 mount 维度输出；同 shell 启动 cloud-claude 时也会观察到原本被默认黑名单滤掉的二进制文件出现在热同步扫描或 oversized 列表中。
修复路径：在 shell 中执行 unset CLOUD_CLAUDE_NO_DEFAULT_IGNORE 或在 ~/.zshrc / ~/.bashrc 里删除对应的 export 行后重启终端；如果确实需要长期保留某些被默认黑名单覆盖但又想同步的路径，建议改用 .gitignore + mount-ignore 的组合而不是关闭整个默认黑名单；最后再跑一次 cloud-claude doctor mount 确认本条 check 变回 Pass。
关联文档：.planning/phases/36-sshfs/36-REVIEW.md WR-01；internal/cloudclaude/doctor/mount.go checkDefaultIgnoreLoaded；CLOUD_CLAUDE_NO_DEFAULT_IGNORE 默认黑名单设计`)

	registerExplanation(MOUNT_PROMOTER_FAILED, `触发场景：远程 cold-promoter 通过 SSH 在容器内启动 shell 脚本失败。cloud-claude Full 模式 mount 就绪后尝试在容器内启动 inotifywait/find 监控冷文件访问并晋升到热层时，SSH session 创建或脚本执行失败，系统降级为无读取晋升模式继续运行（写入仍通过 HotSyncEngine 自动晋升）。
	根本原因：冷文件晋升引擎通过 SSH session 在远程容器内运行 shell 脚本，优先使用 inotifywait 捕获读取事件，不可用时降级为 find 轮询捕获修改事件。启动失败通常由 SSH session 不可用（connA 已断开或达到 MaxSessions 上限）导致。若 inotifywait 运行时失败（如 max_user_watches 不足），脚本会自动降级到 find 轮询，不会触发此错误。
	复现方式：在容器内移除 /bin/sh 或限制 sshd MaxSessions=0 后启动 cloud-claude --mount-mode=full，stderr 出现 [MOUNT_PROMOTER_FAILED] 输出。
	修复路径：确认 connA SSH 连接正常且未达到 MaxSessions 限制（默认 10）。此错误仅影响读取触发晋升功能，写入晋升（cold=RW + HotSyncEngine 轮询）不受影响。如需彻底关闭晋升，设 CLOUD_CLAUDE_NO_PROMOTION=1。
	关联文档：internal/cloudclaude/mount_strategy.go startRemotePromoter；CLOUD_CLAUDE_NO_PROMOTION 环境变量定义`)

	// ────────────────────────────────────────────────────────────────────
	// NET_OAUTH_* 域 + NET_RECONNECT_* + NET_TCP_KEEPALIVE_*（Phase 31/32）
	// ────────────────────────────────────────────────────────────────────

	registerExplanation(NET_OAUTH_EXPIRED, `触发场景：cloud-claude 在 mount 完成后做 OAuth 三态检查时发现容器内 ~/.claude/.credentials.json 中的 access_token 已过期。
根本原因：Claude OAuth token 默认 1 小时过期，refresh_token 30 天过期。如果用户超过 30 天没用，refresh 也会失效。Phase 33 引入 claude-state 持久化 volume 后，credentials 跨容器重建保留，但仍会因时间过期。
复现方式：手动把 expires_at 改成过去时间 → cloud-claude 启动 → 立即 SeverityFatal 阻断。
修复路径：在容器内运行 cloud-claude exec claude login 完成 OAuth 重新登录流程；登录态会持久化到 claude-state 命名 volume。
关联文档：Phase 31 RESEARCH §7 / Phase 33 SUMMARY`)

	registerExplanation(NET_OAUTH_EXPIRING_SOON, `触发场景：OAuth 三态检查发现 access_token 还有 < 30 分钟过期，仅警告不阻塞。
根本原因：访问令牌即将到期，长会话期间可能在用户操作中突然失效，提前提示让用户在合适时机主动刷新避免被打断。这是 Phase 31 三态语义的中间态。
复现方式：手动把 expires_at 改成 20 分钟后 → 启动时 stderr 出现 WARN 提示。
修复路径：建议在当前任务完成后运行 cloud-claude exec claude login 主动刷新；或继续使用直到过期再刷新（不影响当前会话）。
关联文档：Phase 31 RESEARCH §7 / Plan 03 三态实现`)

	registerExplanation(NET_OAUTH_NOT_FOUND, `触发场景：容器内 ~/.claude/.credentials.json 文件不存在，cloud-claude 拒绝启动 claude code 进程。
根本原因：用户从未在该 claude_account 完成首次登录，或持久化 volume 被清理掉。Phase 33 之前每次容器重建都丢登录态，Phase 33 后绑定 claude-state-{account_id} volume 解决此问题。
复现方式：docker volume rm claude-state-test → cloud-claude → 立即 SeverityFatal。
修复路径：在容器内运行 cloud-claude exec claude login 完成首次 OAuth 登录；如果反复丢失，检查 claude-state-{account_id} volume 是否绑定（管理员侧）。
关联文档：Phase 33 SUMMARY / Phase 31 RESEARCH §7`)

	registerExplanation(NET_RECONNECT_GAVE_UP, `触发场景：SSH 连接断开后，Reconnector 按 1/2/4/8/30s 退避策略重试若干次仍未成功，最终放弃。
根本原因：网络长时间不可达（Wifi 切换异常 / VPN 断开 / 远端宿主机宕机），自动重连兜底机制有上限避免无限循环消耗资源。Phase 32 设计为 fastRetry 60s 5 次封顶。
复现方式：vpn 关闭 30 分钟 → reconnect_count 达到上限 → SeverityFatal 退出。
修复路径：检查本地网络（ping gateway / 测试 SSH 直连远端）；运行 cloud-claude doctor 全栈诊断；网络恢复后重新运行 cloud-claude 即可，远端 tmux 会话不丢失（Phase 32 D-10）。
关联文档：Phase 32 RESEARCH §3 reconnect`)

	registerExplanation(NET_TCP_KEEPALIVE_UNSUPPORTED, `触发场景：cloud-claude 启动时尝试设置 TCP_KEEPIDLE/TCP_KEEPINTVL/TCP_KEEPCNT 三个 socket option 但内核不支持（极少数 BSD / 旧 Linux 内核）。
根本原因：TCP keepalive 用于在 NAT 网关上保活长连接，避免被 idle timeout 强制断开。失败仅是性能优化损失，SSH 应用层 keepalive（KeepAliveInterval=15s）仍生效兜底。
复现方式：在 OpenBSD 上运行 cloud-claude → 平台特化失败但功能正常。
修复路径：无需操作；如果观察到弱网下重连频繁可手动调高 KeepAliveInterval（但 < 15s 会被 SESSION_KEEPALIVE_TOO_AGGRESSIVE 阻断）。
关联文档：Phase 32 RESEARCH §3 keepalive / D-04`)

	// ────────────────────────────────────────────────────────────────────
	// SESSION_* 域（Phase 32）
	// ────────────────────────────────────────────────────────────────────

	registerExplanation(SESSION_KEEPALIVE_TOO_AGGRESSIVE, `触发场景：用户通过 config.yaml 或环境变量把 keepalive_interval 配成 < 15s，cloud-claude 在启动期校验时拒绝。
根本原因：过短的 keepalive 会让弱网（移动网络 / VPN）频繁触发假重连，Phase 32 实测 < 15s 时重连率显著上升。15s 是 PITFALLS M11 给出的最低安全线（含 4 次失败后的 60s 容忍窗口）。
复现方式：echo "keepalive_interval: 5s" >> ~/.cloud-claude/config.yaml → cloud-claude → exit 4。
修复路径：把 keepalive_interval 调整到 ≥ 15s，或干脆删除该字段使用默认 15s；如果是为了快速断网检测，建议改用 cloud-claude doctor network 主动诊断。
关联文档：Phase 32 PITFALLS M11 / REQ-F3-A`)

	registerExplanation(SESSION_TMUX_UNAVAILABLE, `触发场景：cloud-claude 启动后探测容器内 tmux 二进制存在性失败，自动 fallback 到 plain SSH 模式（不带会话恢复）。
根本原因：tmux 是 Phase 32 引入的会话可靠性核心组件，让 30s 抖动 / 多端 attach 等场景无感。容器镜像 v3.0.0 起默认包含 tmux 3.4，旧镜像或自定义镜像可能缺失。
复现方式：docker exec <ctr> apt remove -y tmux → cloud-claude → 退到 plain ssh。
修复路径：升级容器镜像到 v3.0.0+；如果是自定义镜像，在 Dockerfile 加 RUN apt-get install -y tmux；运行 cloud-claude doctor mount 看容器健康度。
关联文档：Phase 32 D-06 / RESEARCH §1`)

	registerExplanation(SESSION_NOT_FOUND, `触发场景：用户运行 cloud-claude sessions attach <name> 但远端 tmux 中不存在该会话名。
根本原因：tmux 会话名按 claude_account_id 派生（防多账号串扰），用户输入的名字可能拼写错误，或会话已被人工 kill / 因容器重启丢失。
复现方式：cloud-claude sessions attach typo-name → exit code 非 0。
修复路径：先运行 cloud-claude sessions ls 查看实际可用会话名；如果是首次连接，让 cloud-claude 自动创建（直接 cloud-claude 而不是 sessions attach）。
关联文档：Phase 32 D-10 / Plan 02 sessions.go`)

	registerExplanation(SESSION_TAKEOVER_FAILED, `触发场景：用户运行 cloud-claude --take-over 试图把其它客户端踢掉独占 attach，但远端 tmux detach-client 命令返回非零。
根本原因：常见为 tmux server 状态异常、目标会话已被外部 kill、或 tmux 版本过旧不支持 detach-client -a。这种情况下 take-over 静默失败，多个客户端可能仍并存导致输入串扰。
复现方式：在 cloud-claude --take-over 执行的瞬间外部 tmux kill-session → detach 命令找不到目标。
修复路径：运行 cloud-claude sessions ls 检查会话状态；最坏情况删除会话重建：cloud-claude exec tmux kill-session -t <name>；运行 cloud-claude doctor 全栈诊断。
关联文档：Phase 32 D-11 / RESEARCH §2 take-over`)

	registerExplanation(SESSION_SYNC_LOCKED, `触发场景：同一 claude_account 已经有另一个 cloud-claude 实例占用热同步写锁（flock /tmp/cloud-claude/locks/sync-{id}.lock），本端只能拿 sshfs 只读视图。
根本原因：双向热同步同时写两端会冲突，Phase 32 D-17 引入 flock 互斥保证一时刻只一端做 hot 写。secondary 客户端用 sshfs 看到 primary 写入的内容（read-only 视角）。
复现方式：开两个终端同时跑 cloud-claude → 第二个 stderr 出现 SESSION_SYNC_LOCKED 提示。
修复路径：无需操作；如果需要独占同步，先关闭 primary 端 cloud-claude 进程，secondary 会在 1 秒内拿到锁升级为双向热同步。
关联文档：Phase 32 D-17/D-18/D-19 sync_lock`)

	registerExplanation(SESSION_BUFFER_OVERFLOW, `触发场景：网络断开期间 BufferedStdin 4KB ringBuf 已满，再输入会丢弃最早的字符。
根本原因：Phase 32 引入本地输入缓冲让用户在断网期间继续敲键，恢复后 Flush 回放。但缓冲不能无限大（避免内存爆炸 / 重连后回放卡顿），4KB 是均衡选择。
复现方式：人为断网后粘贴 10KB 文本 → 出现 SESSION_BUFFER_OVERFLOW 警告。
修复路径：等待网络恢复后重新输入丢失部分；规避策略是断网期间避免大段粘贴；长期断网建议 Ctrl+C 退出 cloud-claude 等恢复后重连（远端 tmux 会话不丢）。
关联文档：Phase 32 RESEARCH §4 input_buffer / WR-04`)

	// ────────────────────────────────────────────────────────────────────
	// STATE_* 域（Phase 34，新增；STATE_LAST_SESSION_MISSING 已豁免）
	// ────────────────────────────────────────────────────────────────────

	registerExplanation(STATE_VOLUME_IN_USE_001, `触发场景：管理员对某 claude_account 调用 DELETE /v1/admin/claude-accounts/{id}，但其持久化 volume 仍被某容器持有。
根本原因：Docker volume rm 在 volume 被任何容器（运行中或停止）持有时会失败。Phase 33 admin handler 走 BeginTx + LockClaudeAccount + GetHostWithClaudeAccount 三步原子事务防止 orphan。
复现方式：先 cloud-claude 启动占用 volume，再调 admin DELETE → 强一致路径返回 409 + STATE_VOLUME_IN_USE_001。
修复路径：先停止容器 cloud-claude admin hosts stop <id>，再调 DELETE；或带 ?force=true 走宽松路径（DB 先 commit + 事后 rm，可能留 orphan）。
关联文档：Phase 33 SUMMARY / docs/runbooks/v3-claude-state-volumes.md`)

	registerExplanation(STATE_CONTAINER_NOT_RUNNING, `触发场景：cloud-claude doctor 检查发现远端宿主机上对应 claude_account 的容器不在 running 状态（可能是 exited / created / paused）。
根本原因：远端容器异常退出（OOM / 镜像 pull 失败 / volume 挂载失败 / sing-box tun 启动失败等），doctor 无法 ssh 进去做后续检查，跳过远端检查项。
复现方式：docker stop <container> → cloud-claude doctor → SeverityWarn 提示。
修复路径：运行 cloud-claude admin hosts start <id> 启动容器；如果反复退出，cloud-claude admin hosts logs <id> 查看启动日志；联系管理员排查宿主机健康（disk / 内存 / 镜像）。
关联文档：Phase 33 D-13 / .planning/phases/34/34-CONTEXT §D-21`)

	// ────────────────────────────────────────────────────────────────────
	// SYSTEM_* 域（Phase 34 新增）
	// ────────────────────────────────────────────────────────────────────

	registerExplanation(SYSTEM_APPARMOR_FUSERMOUNT3_MISSING, `触发场景：宿主机 AppArmor profile 中缺少 fusermount3 的 capability dac_override 行，容器内 sshfs / mergerfs 在挂载时被内核拒绝。
根本原因：Ubuntu 22.04+ 默认 AppArmor docker-default 收紧了 mount 操作权限，需要在 fusermount3 profile（不是 docker-default！）显式 allow。Launchpad bug #2111105 + moby#50013 + sysbox#947 + stargz-snapshotter#2144 多源证据一致。
复现方式：apparmor_status | grep fusermount3 → 缺失 → 容器内 mount 系列调用 EPERM。
修复路径：以宿主机 root 运行 deploy/scripts/host-preflight.sh 自动写入 capability dac_override 行；之后 systemctl reload apparmor && docker restart <ctr>。
关联文档：Phase 29 D-23 / .planning/research/PITFALLS.md C6`)

	registerExplanation(SYSTEM_FUSE_RESIDUAL_MOUNT, `触发场景：cloud-claude doctor 扫描宿主机 /workspace 类目录，发现上次崩溃留下的残留 FUSE 挂载点（mount | grep fuse）。
根本原因：cloud-claude 进程被 kill -9 时来不及做 fusermount3 -u 清理，下次启动会被 mount table 中的旧条目阻塞或导致 stale handle。
复现方式：kill -9 cloud-claude 进程 → 留下若干 fuse.sshfs / fuse.mergerfs 挂载条目。
修复路径：运行 cloud-claude doctor mount --fix 自动 fusermount3 -u 所有残留；手动方式 mount | grep fuse 列出后逐个 fusermount3 -u <path>；最后重启 cloud-claude 验证。
关联文档：Phase 34 RESEARCH §3.4 fuse_residual / Phase 31 PITFALLS M14`)

	registerExplanation(SYSTEM_DNS_RESOLVE_FAILED, `触发场景：cloud-claude doctor network 解析 gateway 域名 / Anthropic API 域名失败（gethostbyname 返回 NXDOMAIN 或超时）。
根本原因：常见为本机 /etc/resolv.conf 配错、企业 VPN 改写 DNS 后未生效、macOS scutil DNS cache 陈旧、Linux systemd-resolved 未刷新。
复现方式：sudo dscacheutil -flushcache（macOS）后立刻反复测试。
修复路径：运行 cloud-claude doctor network --fix 自动调用平台对应的 flush 命令（macOS：dscacheutil -flushcache；Linux：systemd-resolve --flush-caches）；检查 /etc/resolv.conf；尝试切换到 8.8.8.8 / 1.1.1.1 排除 DNS server 问题。
关联文档：Phase 34 RESEARCH §4.5 DNS flush`)

	registerExplanation(SYSTEM_CHECK_TIMEOUT, `触发场景：单个 doctor 检查项执行时间超过默认超时（如 SSH 连通性 5s / mount 检查 10s），cloud-claude 中止该项继续下一项。
根本原因：弱网 / 远端容器负载高 / 检查项设计本身慢都可能触发。doctor 严格的超时保证整体 walltime 可控（≤ 30s 出报告）。
复现方式：iptables -A OUTPUT -p tcp --dport 22 -j DROP（模拟丢包）→ doctor ssh 连通性超时。
修复路径：加 --verbose flag 把超时放宽到 30s 重试；如果仍超时，运行 cloud-claude doctor network 看底层网络；检查远端容器 docker stats 看是否 CPU/IO 饱和。
关联文档：Phase 34 D-21 / RESEARCH §5 doctor 超时设计`)

	// ────────────────────────────────────────────────────────────────────
	// SSH_* 域（Phase 34 新增）
	// ────────────────────────────────────────────────────────────────────

	registerExplanation(SSH_KNOWN_HOSTS_CONFLICT, `触发场景：cloud-claude doctor ssh 检查发现 ~/.ssh/known_hosts 中记录的远端 fingerprint 与本次握手实际收到的不一致。
根本原因：通常是远端容器被销毁重建导致 host key 重新生成（Phase 33 之前每次 admin recreate 都换 key），或 IP 复用到不同主机。SSH 客户端会强 reject 防中间人攻击。
复现方式：admin 重建容器 → 直接 ssh 报 WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED。
修复路径：运行 cloud-claude doctor ssh --fix 自动 ssh-keygen -R <host> 删除旧条目；下次连接会重新写入正确 fingerprint；如果担心中间人，先比对 admin 后台显示的 host key fingerprint。
关联文档：Phase 34 D-21 / Phase 33 SUMMARY 容器重建语义`)

	registerExplanation(SSH_SSHD_KEEPALIVE_DRIFT, `触发场景：cloud-claude doctor ssh 检查远端容器 /etc/ssh/sshd_config 中 ClientAliveInterval / ClientAliveCountMax 与基线（15 / 8）不一致。
根本原因：基线 15 / 8 是 Phase 32 PITFALLS M11 给出的最优值（与客户端 KeepAliveInterval=15s 协同保证 30s 抖动可恢复）。漂移可能是用户/管理员手动修改 sshd_config 或镜像版本不对。
复现方式：docker exec <ctr> sed -i 's/ClientAliveInterval 15/ClientAliveInterval 60/' /etc/ssh/sshd_config && systemctl reload sshd → doctor 检测到漂移。
修复路径：重建容器以恢复基线（参考 deploy/docker/managed-user/sshd_config）；如果是定制场景，需保证客户端 keepalive_interval ≤ 服务端 ClientAliveInterval；运行 cloud-claude doctor 全栈诊断。
关联文档：Phase 32 PITFALLS M11 / Phase 34 D-21`)

	// ────────────────────────────────────────────────────────────────────
	// AUTH_* 域（Phase 34 新增）
	// ────────────────────────────────────────────────────────────────────

	registerExplanation(AUTH_CONFIG_MISSING, `触发场景：cloud-claude 启动期读取 ~/.cloud-claude/config.yaml 失败（文件不存在 / YAML 解析错误 / 必填字段缺失）。
根本原因：首次使用未运行 cloud-claude init；或文件被外部工具误删 / 改坏；或权限问题（chmod 000）。这是 Fatal 级错误，cloud-claude 无法继续。
复现方式：rm ~/.cloud-claude/config.yaml && cloud-claude → 立即 exit 4。
修复路径：运行 cloud-claude init 交互式重新配置网关地址 + username + 密码；或手动恢复 config.yaml（参考 docs/runbooks/v3-claude-state-volumes.md 配置示例）；权限问题 chmod 600 ~/.cloud-claude/config.yaml。
关联文档：Phase 31 init 流程 / Phase 34 D-21`)

	registerExplanation(AUTH_GATEWAY_UNREACHABLE, `触发场景：cloud-claude 向配置的 gateway URL 发起 /v1/cli/authenticate 请求失败（连接超时 / TLS 握手失败 / DNS 解析失败 / HTTP 5xx）。
根本原因：常见为本机网络问题、gateway 地址配置错误（typo / 协议错 / port 错）、企业代理拦截、gateway 服务端真实宕机。
复现方式：把 ~/.cloud-claude/config.yaml 中 gateway 改成不存在的域名 → 立即 SeverityError。
修复路径：运行 cloud-claude doctor network 检查 DNS + TCP 连通性；ping / curl gateway 直测；检查 config.yaml 中 gateway 是否带正确 scheme（https://）和端口；如果是企业代理，确认 HTTPS_PROXY 环境变量；最后联系管理员确认 gateway 健康。
关联文档：Phase 34 D-21 / Phase 31 RESEARCH §3 entry`)

	registerExplanation(AUTH_TOKEN_EXPIRED, `触发场景：cloud-claude 调 Entry API 时收到 401 Unauthorized 或服务端明确返回 token 过期错误。
根本原因：Entry API 颁发的 access token 有过期时间（默认 24h），长时间未使用 cloud-claude 后再启动会触发；或服务端轮换密钥强制失效旧 token。
复现方式：手动把 ~/.cloud-claude/cache/token.json 中的 expiry 改成过去 → cloud-claude → 401 → 提示。
修复路径：运行 cloud-claude doctor auth --fix 自动重新走 username/password 拿新 token；或手动 rm ~/.cloud-claude/cache/token.json 触发下次启动重新认证；如果反复 401 检查 username / password 是否正确。
关联文档：Phase 34 D-21 / Phase 31 RESEARCH §3`)

	registerExplanation(AUTH_OAUTH_REFRESH_FAILED, `触发场景：cloud-claude 在容器内尝试用 refresh_token 换新的 access_token 失败（Anthropic API 5xx / refresh_token 也已过期 / 网络问题）。
根本原因：refresh_token 默认 30 天有效，超期后必须重新走完整 OAuth 登录流程；少数情况是 Anthropic 服务端临时故障。这与 NET_OAUTH_EXPIRED（access_token 过期但 refresh 还有效）的区别在于 refresh 也已挂掉。
复现方式：把 .credentials.json 中 refresh_token 改坏 → claude api 调用 → 刷新失败。
修复路径：在容器内运行 cloud-claude exec claude login 重新登录，与 NET_OAUTH_EXPIRED 处理路径一致；如果反复失败先 cloud-claude doctor network 排除网络。
关联文档：Phase 31 RESEARCH §7 / Phase 33 SUMMARY`)

	registerExplanation(NET_EGRESS_IP_DRIFT, `触发场景：cloud-claude doctor network 通过远端容器 curl ifconfig.me 拿到的出口 IP 与 Entry API 期望（admin 配置的 binding）不一致。
根本原因：sing-box tun 全隧道路由表错乱、iptables 规则被外部修改、出口 IP binding 在 admin 侧被改但容器未重建、DNS over HTTPS 绕过 tun 等都可能触发。这是网络强约束（核心价值）的关键监控项。
复现方式：admin 后台修改 claude_account 的 egress_ip binding 但不重建容器 → doctor 立即检测到漂移。
修复路径：运行 cloud-claude doctor network 看完整网络栈；让管理员在 admin 后台重建容器（确保 sing-box 用最新 binding）；检查容器内 iptables -L -t mangle 看路由规则是否被破坏。
关联文档：CLAUDE.md 核心价值 / Phase 34 D-21`)

	// ────────────────────────────────────────────────────────────────────
	// DISK_* 域（Phase 34 新增）
	// ────────────────────────────────────────────────────────────────────

	registerExplanation(DISK_LOCAL_LOW, `触发场景：cloud-claude doctor disk 发现本地 ~/.cloud-claude/ 所在分区可用空间 < 500MB 警戒线。
根本原因：~/.cloud-claude/ 存放 HotSync 数据目录、token 缓存、日志等，分区满会导致热同步无法 staging 文件、token 无法刷新、日志切割失败。
复现方式：dd if=/dev/zero of=~/big bs=1M count=1000（填到剩 < 500MB）→ doctor warn。
修复路径：清理 ~/.cloud-claude/hotsync/sessions/ 旧 session 数据（下次启动 cloud-claude 自动重建）；删除 ~/.cloud-claude/cache/ 下旧日志；如果是系统盘满，找出大文件 du -sh ~/* | sort -h；考虑把 ~/.cloud-claude 软链到大分区。
关联文档：Phase 34 D-21 / RESEARCH §6 disk thresholds`)

	registerExplanation(DISK_CONTAINER_LOW, `触发场景：cloud-claude doctor disk 通过远端 df 命令发现容器内 /workspace 可用空间 < 100MB 警戒线。
根本原因：/workspace 是用户主工作目录，挂的是 docker named volume，volume 占用宿主机分区，宿主机磁盘满会导致写失败。容器内大量临时文件（npm/cargo build / docker layer）也会消耗。
复现方式：在容器内 dd if=/dev/zero of=/workspace/big bs=1M count=10000 → doctor warn。
修复路径：在容器内清理大文件 du -sh /workspace/* | sort -h；删除 build artifacts（node_modules / target / dist）；联系管理员扩容 docker volume 或在宿主机加盘；最坏情况 cloud-claude admin hosts recreate（保留 claude_account 持久化登录态）。
关联文档：Phase 33 SUMMARY / Phase 34 D-21`)

	registerExplanation(DISK_HOTSYNC_DATA_BLOAT, `触发场景：cloud-claude doctor disk 发现本地 ~/.cloud-claude/hotsync/ 目录已经超过 1GB 警戒线。
根本原因：HotSync 在每个 session 下保存 staging snapshot + 冲突历史 + 元数据，长期使用会逐渐膨胀；session 异常退出时可能留 orphan 数据；大仓库初次同步也会瞬间放大。
复现方式：连续启动 cloud-claude 在多个大仓库下数十次 → hotsync 数据目录显著膨胀。
修复路径：运行 rm -rf ~/.cloud-claude/hotsync/sessions/ 强制清理（下次启动 cloud-claude 自动重建 session）；这不会丢失代码（实际代码在仓库 + 远端 /workspace 中），只丢同步历史；定期运行 cloud-claude doctor disk 监控趋势。
关联文档：Phase 34 D-21 / RESEARCH §6`)

	// ────────────────────────────────────────────────────────────────────
	// Phase 41: remote-ssh doctor 维度（Warn/Error 级 explain）
	// ────────────────────────────────────────────────────────────────────

	registerExplanation(SSH_VSCODE_PORT_NOT_LISTENING, `触发场景：cloud-claude doctor remote-ssh 检测到容器内 VS Code Server 进程存在（pgrep 返回成功），但 ss -tlnp 未发现其监听任何 TCP 端口。
根本原因：VS Code Server 启动分为两个阶段——进程创建和端口绑定。进程启动后需要下载扩展、初始化工作区，然后才开始监听端口。如果在初始化完成前执行 doctor 检查，或扩展安装过程中出错导致服务卡在中间状态，就会出现"进程在但端口未开"的情况。此外，容器资源不足（CPU 满载、磁盘空间耗尽）也会显著延长启动时间。
复现方式：通过 VS Code Remote-SSH 连接容器，连接建立后立即运行 cloud-claude doctor remote-ssh；或在容器内执行 pkill -STOP -f vscode-server 暂停进程后检查。
修复路径：等待 30 秒后重新运行 cloud-claude doctor remote-ssh 确认是否为启动延迟；如果持续无端口，在 VS Code 中断开并重新连接 Remote-SSH；检查容器资源使用（docker stats）排除 CPU/内存瓶颈；最坏情况在容器内 rm -rf ~/.vscode-server 让 VS Code 重新下载。
关联文档：Phase 41 CONTEXT §VS Code Server 进程检测；VS Code Remote-SSH 官方文档 Troubleshooting`)

	registerExplanation(SSH_FORWARDING_BLOCKED, `触发场景：cloud-claude doctor remote-ssh 检测到容器内 OUTPUT 链存在 DROP 规则，可能拦截 VS Code 端口转发（语言服务器、调试器）使用的 SSH forwarding 流量。
根本原因：cloud-claude 的网络安全模型通过 sing-box tun 全隧道 + iptables 默认拒绝策略实现出口 IP 强约束。如果 iptables OUTPUT 链的 DROP 规则过于宽泛（例如拒绝了容器内部的 loopback 转发流量），VS Code 的端口转发也会被误拦截。正常情况下 sing-box 只拦截出站 TCP/UDP，不应影响 SSH forwarding（走 unix socket）。
复现方式：在容器内手动添加 iptables -A OUTPUT -p tcp -j DROP → 运行 cloud-claude doctor remote-ssh → 出现 SSH_FORWARDING_BLOCKED 警告。
修复路径：检查容器内 iptables -L OUTPUT -v -n 确认 DROP 规则的具体匹配条件；如果规则由 sing-box 自动添加，重启容器让 sing-box 重新初始化路由表；如果规则是手动添加的，移除过于宽泛的 DROP 规则；确认 VS Code 端口转发在 doctor 报告通过后可正常使用。
关联文档：Phase 41 CONTEXT §Forwarding Channel 检测；CLAUDE.md 网络安全约束`)

	registerExplanation(DISK_VSCODE_SERVER_WARN, `触发场景：cloud-claude doctor remote-ssh 发现容器内 ~/.vscode-server/ 目录占用超过 500MB 警戒线。
根本原因：VS Code Remote-SSH 会在容器内 ~/.vscode-server/ 下缓存扩展（extensions/）、扩展宿主进程（bin/）、以及各类临时数据。长时间使用或多版本 VS Code 客户端连接后，旧版本的 Server 和扩展会残留，目录逐渐膨胀。extensions-cache/ 子目录是扩展下载缓存，删除不影响已安装扩展。
复现方式：在容器内安装 10+ 个 VS Code 扩展（如 Python、ESLint、Prettier 等），连续使用数周后运行 cloud-claude doctor remote-ssh → 出现 DISK_VSCODE_SERVER_WARN。
修复路径：轻量清理：rm -rf ~/.vscode-server/extensions-cache/（仅删除下载缓存，不影响已安装扩展）；中量清理：rm -rf ~/.vscode-server/*/extensions/*/（删除所有扩展，VS Code 重连时会重新安装）；如果磁盘紧张，完整清理 rm -rf ~/.vscode-server/（VS Code 重连时会自动重建整个目录结构）。
关联文档：Phase 41 CONTEXT §~/.vscode-server/ 磁盘占用；Phase 41 RESEARCH §6 Check Design`)

	registerExplanation(DISK_VSCODE_SERVER_BLOAT, `触发场景：cloud-claude doctor remote-ssh 发现容器内 ~/.vscode-server/ 目录占用超过 2GB 严重警戒线，容器可用磁盘空间可能已受到显著影响。
根本原因：~/.vscode-server/ 超过 2GB 通常是多个 VS Code Server 版本残留叠加大量已安装扩展共同导致。每次 VS Code 客户端升级后重新连接时会下载新版本 Server，但旧版本不会自动清理。扩展的 native 依赖（如 Python 的 Pylance、C/C++ 的 clangd）单个就可能占用数百 MB。在容器磁盘空间有限的场景下（通常 10-20GB volume），2GB 的 .vscode-server 会严重挤压 /workspace 可用空间。
复现方式：连续数月使用 VS Code Remote-SSH 连接同一容器，期间 VS Code 客户端升级 3-4 次，安装 20+ 个扩展 → ~/.vscode-server/ 可达 2GB+。
修复路径：完整清理 rm -rf ~/.vscode-server/（这是最直接有效的方案，VS Code 下次重连时会自动下载当前版本 Server 和已注册扩展）；清理后重新运行 cloud-claude doctor remote-ssh 确认磁盘恢复正常；如果空间仍然紧张，联系管理员检查容器 volume 容量是否充足。
关联文档：Phase 41 CONTEXT §~/.vscode-server/ 磁盘占用（完整清理路径）；Phase 41 RESEARCH §GOTCHA 3 du 耗时`)
}
