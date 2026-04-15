# Phase 28: 生产环境 FUSE 兼容性验证 - Pattern Map

**Mapped:** 2026-04-15
**Files analyzed:** 5（含 1 项可选产物）
**Analogs found:** 4 / 5（可选自定义安全 profile 无现成类比）

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `scripts/verify-fuse-compat.sh` | utility / script | batch + 子进程检查（docker exec、宿主机命令） | `deploy/scripts/host-preflight.sh` + `scripts/verify-managed-image.sh` | composite |
| `internal/runtime/tasks/worker.go`（`createHost`） | service / runtime | request-response（Docker CLI 创建容器） | 同文件现有 `createHost` | exact |
| `docs/zh/guide/deployment.md` | documentation | — | 同文件现有「前置条件 / 依赖检查」结构 | self |
| `docs/en/guide/deployment.md` | documentation | — | 同文件英文版对应章节 | self |
| （可选）自定义 `seccomp` JSON / AppArmor profile 文件 | config | — | 仓库内无同类已提交文件 | none |

## Pattern Assignments

### `scripts/verify-fuse-compat.sh`（utility，batch + 子进程检查）

**Analog 1:** `deploy/scripts/host-preflight.sh` — 宿主机前置依赖检查、`require_cmd`、`set -euo pipefail`。

**Shebang 与严格模式**（lines 1-2）：

```bash
#!/usr/bin/env bash
set -euo pipefail
```

**依赖探测与失败即退出**（lines 4-18）：

```bash
require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

require_cmd docker
require_cmd ip
require_cmd systemctl

if ! command -v nft >/dev/null 2>&1 && ! command -v iptables >/dev/null 2>&1; then
  echo "missing required firewall tool: nft or iptables" >&2
  exit 1
fi
```

**内核能力探测（可类比 FUSE：`modprobe fuse` / `/dev/fuse`）**（lines 20-28）：

```bash
# Phase 2: WireGuard kernel module must be loadable
if ! modprobe -n wireguard 2>/dev/null; then
  if ip link add wg-test type wireguard 2>/dev/null; then
    ip link del wg-test 2>/dev/null || true
  else
    echo "missing wireguard kernel module" >&2
    exit 1
  fi
fi
```

**Analog 2:** `scripts/verify-managed-image.sh` — 从仓库配置读镜像名、`docker` 子命令验证。

**镜像名与 Docker 调用**（lines 1-12）：

```bash
#!/usr/bin/env bash
set -euo pipefail

IMAGE_NAME="$(awk -F': ' '$1 == "local_dev_image_name" { print $2 }' deploy/docker/managed-user/image.lock)"

if [[ -z "${IMAGE_NAME}" ]]; then
  echo "failed to read local_dev_image_name from deploy/docker/managed-user/image.lock" >&2
  exit 1
fi

docker image inspect "${IMAGE_NAME}" >/dev/null
docker run --rm --entrypoint sh "${IMAGE_NAME}" -lc 'sshd -T >/dev/null && command -v claude && command -v chromium && command -v xdpyinfo && command -v xterm && command -v pcmanfm && getent passwd workspace && test -d /workspace'
```

**语义对齐（非 shell，供验证逻辑复用）：** `internal/cloudclaude/mount.go` 已用 `mountpoint -q /workspace` 作为挂载就绪判据；验证脚本在容器内应优先使用相同判据。

**挂载就绪轮询与 mountpoint**（lines 82-89）：

```go
	check := func() error {
		sess, err := conn.NewSession()
		if err != nil {
			return err
		}
		defer sess.Close()
		return sess.Run("mountpoint -q /workspace")
	}
```

**sshfs passive 启动命令（端到端对照）**（lines 62-66）：

```go
	if err := sshfsSession.Start("sshfs : /workspace -o passive -f"); err != nil {
```

**结构化「多项检查」心智模型（Go，可对齐 RESEARCH 中 [PASS]/[FAIL] 输出）：** `internal/network/verify.go` 中独立子检查 + 汇总通过条件。

**结果结构与「全部通过」**（lines 12-26）：

```go
// VerifyResult captures the outcome of each verification check performed
// inside a container's network namespace after tunnel wiring completes.
type VerifyResult struct {
	EgressIPMatch  bool
	ActualEgressIP string
	DNSCorrect     bool
	ActualDNS      string
	LeakBlocked    bool
	LeakTarget     string
}

// AllPassed returns true only when all three verification checks passed.
func (r VerifyResult) AllPassed() bool {
	return r.EgressIPMatch && r.DNSCorrect && r.LeakBlocked
}
```

**多项检查均执行、再汇总错误**（lines 37-61）：

```go
func VerifyNetworkIntegrity(ctx context.Context, containerPID uint32, expected EgressConfig) (VerifyResult, error) {
	prefix := []string{"nsenter", "-t", strconv.FormatUint(uint64(containerPID), 10), "-n", "--"}

	var result VerifyResult

	// Check 1: egress IP matches binding
	verifyEgressIP(ctx, prefix, expected.ExpectedIP, &result)

	// Check 2: DNS resolver points to tunnel DNS
	var expectedDNS string
	if expected.Tunnel != nil {
		expectedDNS = expected.Tunnel.DNSServer
	} else if expected.Proxy != nil {
		expectedDNS = expected.Proxy.DNSServer
	}
	verifyDNS(ctx, prefix, expectedDNS, &result)

	// Check 3: direct outbound is blocked by firewall
	verifyLeakBlocked(ctx, prefix, &result)

	if result.AllPassed() {
		return result, nil
	}

	return result, firstNetworkError(expected, result)
}
```

---

### `internal/runtime/tasks/worker.go`（`createHost`，service，request-response）

**Analog:** `internal/runtime/tasks/worker.go` 现有 `createHost`。

**包与依赖**（lines 1-15）：

```go
package tasks

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/zanel1u/cloud-cli-proxy/internal/agentapi"
	"github.com/zanel1u/cloud-cli-proxy/internal/network"
	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)
```

**Docker `create` 参数切片构建（插入 `--security-opt` 的位置）**（lines 153-165）：

```go
	args := []string{
		"create",
		"--name", containerName,
		"--network", "bridge",
		"--cap-add", "NET_ADMIN",
		"--cap-add", "SYS_ADMIN",
		"--device", "/dev/fuse",
		"--label", "cloud-cli-proxy.managed=true",
		"--label", fmt.Sprintf("cloud-cli-proxy.host_id=%s", request.HostID),
		"--hostname", hostname,
		"--shm-size", "1g",
		"--sysctl", "net.ipv6.conf.all.disable_ipv6=1",
	}
```

**后续条件追加与镜像名**（lines 167-188）：

```go
	if request.MemoryLimitMB > 0 {
		args = append(args, "--memory", fmt.Sprintf("%dm", request.MemoryLimitMB))
	}
	if request.CPULimit > 0 {
		args = append(args, "--cpus", fmt.Sprintf("%.1f", request.CPULimit))
	}

	args = append(args,
		"-e", "TZ="+firstNonEmpty(request.Timezone, "America/Los_Angeles"),
		// ... env and -v ...
	)
```

**Planner 提示：** 按 D-04，在 `--device`, `/dev/fuse` 与 `--label` 之间（或 RESEARCH 建议的紧跟 fuse 设备之后）追加 `"--security-opt", "apparmor=unconfined"` 等与现有 `append` 风格一致；避免改用 `--privileged`。

---

### `docs/zh/guide/deployment.md` / `docs/en/guide/deployment.md`（documentation）

**Analog:** 各自现有「前置条件」「依赖检查」章节；新内容应插入 **1.1 依赖检查** 表格或其后独立小节，并与 `deploy/scripts/host-preflight.sh` 的叙述方式一致。

**中文版：标题与前置条件列表**（lines 1-10）：

```markdown
# 部署指南

> 本文档面向有 Linux 运维经验的技术人员，指导从零完成单宿主机部署。

## 前置条件

- Ubuntu 22.04+ / Debian 12+（或等效 systemd-based Linux 发行版）
- Root 或 sudo 权限
- 公网 IP（用于 bootstrap 入口和用户 SSH 接入）
- 至少一个出口 IP 的 WireGuard peer 配置（从 VPN 提供商获取）
```

**中文版：依赖检查脚本与表格**（lines 12-35）：

```markdown
## 1. 环境准备

### 1.1 依赖检查

运行内置的依赖检查脚本：

```bash
sudo bash deploy/scripts/host-preflight.sh
```

该脚本会检查以下依赖是否就绪：

| 依赖 | 最低版本 | 用途 |
|------|----------|------|
| Docker Engine | 28.x+ | 容器运行时 |
| WireGuard | 内核模块 | 全隧道出网 |
| nftables (`nft`) | -- | 容器防火墙规则 |
| `nsenter` | -- | 容器网络命名空间校验 |
| `curl` | -- | 出口 IP 校验和健康检查 |
| `ip` | -- | 网络配置 |
| `systemctl` | -- | 服务管理 |
| Go | 1.26+ | 构建控制面和 host-agent |
| PostgreSQL | 18.x | 持久化存储 |
| Node.js | 24 LTS | 前端构建（可选） |
```

**英文版：对应结构**（lines 1-31）：

```markdown
# Deployment Guide

> For system administrators with Linux experience, deploying on a single host from scratch.

## Prerequisites

- Ubuntu 22.04+ / Debian 12+ (or equivalent systemd-based Linux)
- Root or sudo access
- Public IP (for bootstrap endpoint and user SSH access)
- At least one WireGuard peer config for an exit IP (from VPN provider)

## 1. Environment Setup

### 1.1 Dependency Check

```bash
sudo bash deploy/scripts/host-preflight.sh
```

| Dependency | Min Version | Purpose |
|-----------|-------------|---------|
| Docker Engine | 28.x+ | Container runtime |
```

---

## Shared Patterns

### 受管镜像内 `/dev/fuse` 权限基线

**Source:** `deploy/docker/managed-user/entrypoint.sh`  
**Apply to:** 验证脚本中「容器内 FUSE 是否可用」的说明、部署文档前置条件

```bash
if [ -c /dev/fuse ]; then
  chmod 666 /dev/fuse
fi
```

### FUSE 卸载兜底（验证脚本 teardown 可参考，非必须同一命令）

**Source:** `internal/cloudclaude/mount.go` — `fusermountCleanup`

```go
	_ = sess.Run("fusermount -u /workspace 2>/dev/null || true")
```

### 容器网络校验哲学（与 RESEARCH「三项 Success Criteria」叙事对齐）

**Source:** `internal/network/verify.go` — 与全隧道 + 防火墙共存的验证思路一致；FUSE 脚本侧用 `[PASS]/[FAIL]` 与计数汇总即可类比，无需调用 Go 代码。

---

## No Analog Found

| File / 产物 | Role | Data Flow | Reason |
|-------------|------|-----------|--------|
| 自定义 seccomp JSON 或 AppArmor profile（若 D-04 走最小权限而非 `apparmor=unconfined`） | config | — | 仓库中无已提交的同类 profile 文件；Planner 应优先采用 `28-RESEARCH.md` 中示例与运维加载步骤 |

---

## Metadata

**Analog search scope:** `scripts/`、`deploy/scripts/`、`internal/runtime/tasks/`、`internal/cloudclaude/`、`internal/network/`、`docs/zh/guide/`、`docs/en/guide/`、`deploy/docker/managed-user/`  
**Files scanned:** 10+（含上述路径代表性文件）  
**Pattern extraction date:** 2026-04-15
