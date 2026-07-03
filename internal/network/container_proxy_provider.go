package network

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// singboxGroupGID 与 deploy/docker/managed-user/Dockerfile `groupadd --gid 9000
// singbox` 严格对齐（D-54-2）。写死 numeric gid 避免依赖 host 上是否安装了
// `singbox` 系统组（控制面所在宿主机不一定有该组）。entrypoint
// start_singbox_or_die 用 `stat -c '%G'` 读 group 名，要求结果 == "singbox"，
// 故 host 端 9000 这个 GID 必须真的属于名为 singbox 的组 —— 这里是设计意图的
// 安全失败：如果 host 上 9000 是其他组，chown 成功但容器内 entrypoint 立即
// fail-closed (exit 1)。
const singboxGroupGID = 9000

type ContainerProxyProvider struct {
	logger   *slog.Logger
	verifier NetworkVerifier
}

// NewContainerProxyProvider 创建一个 container-proxy 风格的 Provider。
// verifier 负责 PrepareHost 阶段的网络完整性验证；
// 通过注入不同实现来适应不同平台 / 部署形态。
func NewContainerProxyProvider(logger *slog.Logger, verifier NetworkVerifier) *ContainerProxyProvider {
	return &ContainerProxyProvider{logger: logger, verifier: verifier}
}

// PrepareGateway 在 worker 容器 docker create 之前把 sing-box config 写到 host 端
// SingBoxConfigDir(hostID)/config.json（D-54-2 / Plan 54-02），容器随后通过 ro bind
// mount 把该文件挂到 /etc/sing-box/config.json，entrypoint start_singbox_or_die 读取
// 后从 fs 删除（D-V4-2）。
//
// v4.0 (Phase 54) 改造（54-01）：
//   - 不再创建 cloudproxy-net-* 自定义 bridge（删除 dockerNetworkCreate 调用）
//   - 不再启动 sidecar gateway 容器（删除 dockerRunGateway / waitGatewayHealthy）
//   - 不再写容器 DNS 入口锁占位（resolv.conf / nsswitch.conf 由容器内 sing-box
//     接管，删除 WriteContainerDNSConfig 调用）
//   - 不再写 v4 sing-box 路径下的 cidrs / domains placeholder（由 sing-box config
//     的 route.rule_set 直接拉取，54-02 决定具体格式）
//
// user 容器自带 sing-box（Phase 53 镜像），entrypoint 内 start_singbox_or_die 在
// 容器自身 netns 里建 tun0 并应用全局策略路由；host-agent 只做「config 注入 + verify」。
func (p *ContainerProxyProvider) PrepareGateway(ctx context.Context, spec HostNetworkSpec) error {
	_ = ctx
	if spec.Egress == nil {
		p.logger.Info("container-proxy: no egress config, skipping config inject", "host_id", spec.HostID)
		return nil
	}
	if spec.Egress.Proxy == nil {
		p.logger.Warn("container-proxy: no proxy config, skipping config inject", "host_id", spec.HostID)
		return nil
	}

	dir := SingBoxConfigDir(spec.HostID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("sing-box: mkdir config dir: %w", err)
	}
	if err := writeContainerSingBoxConfig(spec.HostID, spec.Egress); err != nil {
		// darwin / 非 root 开发环境 chown root:singbox 必失败，但 config 文件本身
		// 已经写到 fs（writeContainerSingBoxConfig 顺序：write → chown → chmod，
		// chown 失败时文件已存在但 owner 仍是 dev user）。降级为 Warn 让 PrepareGateway
		// 继续返回 nil；下游链路上：
		//   - darwin docker desktop 由于无 /dev/net/tun，host create 本来就跑不通，
		//     verifier.Verify 会更早失败；
		//   - Linux 非 root 控制面（不是生产形态）会被容器内 entrypoint
		//     start_singbox_or_die 的 hard-assert (perm/owner/group) 兜底 exit 1，
		//     fail-closed 语义不漏。
		// 真正的安全保证由 entrypoint 兜底，host-agent 不需要在写盘失败时阻断。
		if isChownPermissionError(err) {
			p.logger.Warn("container-proxy: chown root:singbox failed (likely non-root dev env); entrypoint hard-assert will fail-close on Linux prod",
				"host_id", spec.HostID, "error", err)
			return nil
		}
		return fmt.Errorf("sing-box: write config: %w", err)
	}
	p.logger.Info("container-proxy: sing-box config injected", "host_id", spec.HostID, "dir", dir)
	return nil
}

// writeContainerSingBoxConfig 把生成的 sing-box config 写到 host 端
// SingBoxConfigDir(hostID)/config.json，权限严格 root:singbox 0640（D-54-2）。
//
// 严格权限对齐 Phase 53 entrypoint start_singbox_or_die hard-assert
// （deploy/docker/managed-user/entrypoint.sh L178-189）：
//   - perm == 0o640（owner read+write，group read，other 0）
//   - owner uid == 0（root）
//   - group gid == 9000（singbox，与 Dockerfile groupadd --gid 9000 singbox 对齐）
//
// 任一不匹配 entrypoint 立即 fail-closed (exit 1)，容器永远起不来。
//
// 写盘三步顺序（重要）：
//  1. os.WriteFile(path, data, 0o600) —— 先写最严格 owner-only，避免任何窗口期
//     被同 group 用户提前读到（一致性窗口缩到 chmod 0640 之前）；
//  2. os.Chown(path, 0, 9000) —— 切到 root:singbox 所有权；
//  3. os.Chmod(path, 0o640) —— 放开 group read。
//
// chown 失败处理：调用方 PrepareGateway 通过 isChownPermissionError 判断；darwin
// 非 root 开发环境降级为 Warn，prod Linux root 必成功。
func writeContainerSingBoxConfig(hostID string, egress *EgressConfig) error {
	if egress == nil || egress.Proxy == nil {
		return fmt.Errorf("writeContainerSingBoxConfig: nil egress / proxy")
	}
	serverIP, _, err := extractProxyServer(egress.Proxy.OutboundConfig)
	if err != nil {
		return fmt.Errorf("resolve proxy server: %w", err)
	}
	cfg, err := buildContainerSingBoxConfig(egress.Proxy.OutboundConfig, egress.Proxy.DNSServer, serverIP)
	if err != nil {
		return fmt.Errorf("build container sing-box config: %w", err)
	}
	dir := SingBoxConfigDir(hostID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir config dir: %w", err)
	}
	cfgPath := filepath.Join(dir, "config.json")

	// 防御：当用户直接 docker run -v <源>:... 且源路径尚不存在时，Docker 会
	// 自动把源路径创建为目录（而非文件）。后续 os.WriteFile 遇到 "is a directory"
	// 错误会直接失败，且 PrepareGateway 的调用链中 worker 只报 "write config"，
	// 排查体验极差。这里显式检测并清除该异常状态。
	if info, statErr := os.Stat(cfgPath); statErr == nil {
		if info.IsDir() {
			if rmErr := os.RemoveAll(cfgPath); rmErr != nil {
				return fmt.Errorf("remove stale config.json directory (Docker auto-created it because source path did not exist): %w", rmErr)
			}
		}
	}

	if err := os.WriteFile(cfgPath, cfg, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := os.Chown(cfgPath, 0, singboxGroupGID); err != nil {
		// macOS Docker Desktop / 非 root 开发环境 chown 必失败。
		// 降级为 0644：singbox 用户通过 other:r 位读取 config，
		// entrypoint hard-assert 对 dev 环境放宽为 root:root:644 放行。
		if err2 := os.Chmod(cfgPath, 0o644); err2 != nil {
			return fmt.Errorf("chown root:singbox: %w (chmod fallback also failed: %v)", err, err2)
		}
		return fmt.Errorf("chown root:singbox: %w", err)
	}
	if err := os.Chmod(cfgPath, 0o640); err != nil {
		return fmt.Errorf("chmod 0640: %w", err)
	}
	return nil
}

// isChownPermissionError 判定错误是否为 chown EPERM（darwin 非 root 跑测试或本地
// 开发跑控制面时常见）。真实安全保证由 Phase 53 entrypoint hard-assert 兜底，
// darwin 开发环境放过去即可（详见 PrepareGateway 内的降级注释）。
//
// 用字符串子串匹配而非 errors.Is(err, fs.ErrPermission)：writeContainerSingBoxConfig
// 把 chown 错误用 fmt.Errorf("chown root:singbox: %w", err) 包了一层，需要靠
// "chown" 前缀辨认是哪一步出错（mkdir / write 的 EPERM 不该走降级路径，应该真失败）。
func isChownPermissionError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	if !strings.Contains(s, "chown") {
		return false
	}
	return strings.Contains(s, "operation not permitted") ||
		strings.Contains(s, "permission denied")
}

// PrepareHost 在 worker 容器 docker start 之后调用。
//
// v4.0 (Phase 54) 改造（54-01）：user 容器自带 sing-box（Phase 53），entrypoint
// apply_nft_or_die 在容器内部应用 fail-closed firewall + sing-box 自起 tun0；
// host-agent 不再：
//   - dockerNetworkConnect / disconnect bridge（容器走默认 docker bridge）
//   - configureWorkerEgress（容器内 sing-box 自己配 tun 路由）
//   - applyWorkerFirewall（容器内 entrypoint 自己 apply）
//   - join 控制面到隔离网络（无隔离网络存在）
//
// 创建 / 启动路径不再把出口 IP 探测作为硬门槛：Phase 53 entrypoint 已经在容器内
// fail-closed（sing-box / nft 任一步失败即退出），这里仅记录状态并放行，避免代理
// 端点短暂不可达或出口回显服务抖动导致主机创建失败。
func (p *ContainerProxyProvider) PrepareHost(ctx context.Context, spec HostNetworkSpec) error {
	_ = ctx
	if spec.Egress == nil {
		if p.logger != nil {
			p.logger.Info("container-proxy: no egress config, skipping create-time network verify", "host_id", spec.HostID)
		}
		return nil
	}
	if spec.Egress.Proxy == nil {
		if p.logger != nil {
			p.logger.Warn("container-proxy: no proxy config, skipping create-time network verify", "host_id", spec.HostID)
		}
		return nil
	}

	if p.logger != nil {
		p.logger.Info("container-proxy: create-time egress verification skipped",
			"host_id", spec.HostID,
			"mode", "fail-closed-entrypoint",
		)
	}
	return nil
}

// CleanupHost 在 host 失败或 stop 路径下被调用。
//
// v4.0 (Phase 54) 改造（54-01）：单容器架构下 host-agent 不再持有 gateway 容器
// 或自定义 bridge 网络，CleanupHost 仅需移除 host 端的 sing-box config 目录。
// 容器本身的销毁由 worker.stopHost / rebuildHost 路径上的 docker stop / rm 负责。
//
// 失败仅 Warn，不阻断后续 cleanup 链路（best-effort 幂等）。
func (p *ContainerProxyProvider) CleanupHost(ctx context.Context, spec HostNetworkSpec) error {
	_ = ctx
	dir := SingBoxConfigDir(spec.HostID)
	if err := os.RemoveAll(dir); err != nil {
		p.logger.Warn("container-proxy: cleanup remove sing-box config dir failed",
			"host_id", spec.HostID, "dir", dir, "error", err)
	}
	return nil
}

// SingBoxConfigDir 返回 host 专属的 sing-box config 目录。
// 路径规则：<DATA_DIR>/gateway/<host_id>/。
//
// 路径名 "gateway" 是 v3.x 历史包袱（D-54-10）：为避免跨包（bypass_reload.go /
// admin_hosts.go / app.go / e2e 等）大改动，54-01 保留物理路径不变，语义重定义
// 为「单容器架构下 host-agent 注入到 user 容器内 /etc/sing-box/config.json 的源
// 路径」。下一里程碑（v4.1）再考虑物理迁移到 sing-box/<host_id>/。
func SingBoxConfigDir(hostID string) string {
	base := os.Getenv("DATA_DIR")
	if base == "" {
		base = "/var/lib/cloud-cli-proxy"
	}
	return filepath.Join(base, "gateway", hostID)
}

// GatewayConfigDir 是 SingBoxConfigDir 的 v4.0 兼容 alias（D-54-9），
// 保留一个里程碑（v4.1 删除）。新代码请使用 SingBoxConfigDir。
//
// Deprecated: use SingBoxConfigDir.
func GatewayConfigDir(hostID string) string {
	return SingBoxConfigDir(hostID)
}

// workerContainerName 是 worker 容器的统一命名规则单点。
func workerContainerName(hostID string) string {
	return "cloudproxy-" + hostID
}
