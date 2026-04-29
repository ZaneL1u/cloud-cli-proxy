---
mode: quick
plan: 260422
type: research
tags: [security, anti-detection, fingerprint, telemetry]
---

# Quick 260422: cac (Claude Code 小雨衣) 对比分析与增强建议

## 一、cac 技术方案概述

cac 是一个 Claude Code 环境管理器，核心目标是让 Claude Code 的运行环境看起来像一台独立的物理电脑，从而规避环境指纹追踪和遥测上报。其方案分为三个主要层次：

### 1.1 三层指纹伪装

| 层次 | 机制 | 覆盖范围 |
|------|------|----------|
| **Layer 1 — Shell 代理脚本** | 在 `~/.cac/shim-bin/` 放置同名可执行文件（如 `hostname`、`ifconfig`），PATH 优先级高于系统路径 | 所有 shell 命令、子进程 |
| **Layer 2 — Node.js monkey-patch** | 通过 `NODE_OPTIONS --require` 注入补丁脚本，拦截 `os.hostname()`、`os.networkInterfaces()`、`os.userInfo()`、`fs.readFileSync` 等 | Claude Code Node.js 进程内 |
| **Layer 3 — 环境变量** | `CAC_HOSTNAME`、`CAC_MAC`、`CAC_USERNAME`、`CAC_MACHINE_ID` 等全局传递 | 所有子进程继承 |

### 1.2 遥测阻断（12 层）

cac 实施了极其深入的遥测阻断策略：

1. **DNS 级别拦截**：`cac-dns-guard.js` 补丁 Node.js DNS 解析器和 `fetch()`，拦截 Statsig、Sentry、Segment 等遥测域名
2. **环境变量**：`DO_NOT_TRACK=1`、`SENTRY_DSN=""` 等 12 个变量
3. **HOSTALIASES**：将遥测域名映射到 `0.0.0.0`
4. **网络层代理**：`HTTPS_PROXY`/`HTTP_PROXY`/`ALL_PROXY` 强制流量走代理
5. **mTLS 客户端证书**：每个环境独立证书

### 1.3 网络层

- 强制 OAuth 认证（清除 `ANTHROPIC_API_KEY`）
- TCP 预检验证代理可达
- 心跳检测自动重连

---

## 二、逐项对比表

### 2.1 指纹伪装对比

| 对比项 | cac 方案 | 我们的实现 (`spoof-fingerprint.js`) | 差距评估 |
|--------|----------|--------------------------------------|----------|
| **os.hostname()** | Layer 2 Node.js patch + Layer 1 shell shim | 已实现（Node.js 层） | 缺 Shell 层 |
| **os.userInfo()** | Layer 2 patch | 已实现 | 一致 |
| **os.networkInterfaces()** | Layer 2 patch | 已实现（固定 eth0 + lo） | 一致 |
| **os.platform/type/release/arch** | Layer 2 patch | 已实现 | 一致 |
| **os.cpus() / totalmem() / freemem()** | Layer 2 patch | 已实现 | 一致 |
| **os.homedir() / uptime()** | Layer 2 patch | 已实现 | 一致 |
| **process.platform / arch** | Layer 2 patch | 已实现（defineProperty） | 一致 |
| **MAC 地址伪造** | Layer 1 + Layer 2 | 已实现（SHA256 派生） | 一致 |
| **machine-id 伪造** | Layer 1 shell shim + Layer 2 fs patch | Node.js child_process 拦截 | 缺真实文件写入 |
| **Shell 层 hostname 命令** | `~/.cac/shim-bin/hostname` | **未实现** | **关键差距** |
| **Shell 层 ifconfig 命令** | `~/.cac/shim-bin/ifconfig` | **未实现** | **关键差距** |
| **Shell 层 ioreg 等 macOS 命令** | `~/.cac/shim-bin/` 包裹 | 仅在 child_process 内拦截 | **关键差距** |
| **CPU model 伪造** | 通过 sysctl 拦截 | 已实现（os.cpus patch） | 一致 |
| **内存信息伪造** | 通过 sysctl 拦截 | 已实现（os.totalmem/freemem） | 一致 |

### 2.2 遥测阻断对比

| 对比项 | cac 方案 | 我们的实现 | 差距评估 |
|--------|----------|------------|----------|
| **DNS 级别遥测拦截** | `cac-dns-guard.js` 补丁 `dns.lookup` + `fetch` | **完全未实现** | **关键差距** |
| **HOSTALIASES 环境变量** | 映射遥测域名到 0.0.0.0 | **未实现** | **关键差距** |
| **DO_NOT_TRACK 环境变量** | 12 个变量全覆盖 | **未实现** | **中等差距** |
| **Sentry DSN 清除** | `SENTRY_DSN=""` | **未实现** | **中等差距** |
| **Statsig/Segment 拦截** | DNS + fetch 双层拦截 | **未实现** | **关键差距** |
| **网络层代理强制** | HTTPS_PROXY / HTTP_PROXY / ALL_PROXY | 依赖外部 sing-box 代理配置 | 部分覆盖 |

### 2.3 容器反检测对比

| 对比项 | cac 方案 | 我们的实现 | 差距评估 |
|--------|----------|------------|----------|
| **/.dockerenv 文件处理** | 容器内环境感知 | Phase 21 占位符，**未实现** | **中等差距** |
| **cgroup 掩码** | 无（非容器场景） | Phase 21 占位符，**未实现** | **中等差距** |
| **CLAUDE_CONFIG_DIR 隔离** | 支持 | **未实现** | **低差距** |
| **mTLS 客户端证书** | 每个环境独立证书 | **未实现** | **低差距**（我们的场景不需要） |

### 2.4 网络层对比

| 对比项 | cac 方案 | 我们的实现 | 差距评估 |
|--------|----------|------------|----------|
| **TLS 指纹伪装** | 未明确 | sing-box uTLS (chrome fingerprint) | 我们领先 |
| **IPv6 泄漏防护** | 未明确 | 已实现 | 我们领先 |
| **出口 IP 绑定** | 未明确 | iptables NAT masquerade | 我们领先 |
| **全局隧道** | 依赖用户自行配置 | sing-box tun + netns 全隧道 | 我们领先 |

---

## 三、差距分析

### 3.1 关键差距（必须修复）

**GAP-1：Shell 层命令代理完全缺失**

我们的 `spoof-fingerprint.js` 只在 Node.js child_process 内拦截命令。如果 Claude Code 通过其他方式执行 shell 命令（如 `os.system()` 的 C 绑定、直接 `/bin/sh -c hostname`），真实信息会暴露。

cac 的方案是在 `~/.cac/shim-bin/` 中放置同名脚本，通过 PATH 优先级拦截。这是目前最可靠的方案。

**GAP-2：遥测阻断为零**

这是最大的安全风险。Claude Code 内置了 Statsig、Sentry、Segment 等遥测上报，我们的实现中没有任何阻断措施。即使指纹伪装做得再好，遥测数据仍然会泄露真实环境信息。

**GAP-3：/etc/machine-id 真实文件未写入**

我们只在 Node.js 层拦截了 `cat /etc/machine-id`，但如果通过 shell 直接读取（如 `ssh` 进容器后执行），真实文件不存在或为空，这本身就是一个异常信号。

### 3.2 中等差距（建议修复）

**GAP-4：DO_NOT_TRACK 等环境变量未设置**

简单但有效的阻断手段，通过环境变量告诉 Claude Code 不要上报遥测。实现成本极低。

**GAP-5：HOSTALIASES 未配置**

可以将遥测域名映射到 `0.0.0.0`，在 DNS 解析层面就阻断遥测，比应用层拦截更底层、更可靠。

**GAP-6：CLAUDE_CONFIG_DIR 未隔离**

多个用户或环境可能共享同一个配置目录，导致状态污染。

### 3.3 低差距（可选增强）

**GAP-7：mTLS 客户端证书**

cac 为每个环境签发独立客户端证书。我们的场景是单宿主机 + Docker 容器，网络层已经通过 sing-box 隧道控制，mTLS 的额外价值有限。

**GAP-8：容器反检测（/.dockerenv、cgroup）**

我们的用户容器本身就知道自己是容器，主要风险在于 Claude Code 检测到容器环境后改变行为。占位符已存在，需要决定是否实施。

---

## 四、增强建议（按优先级排序）

### P0 — 必须实施（安全关键）

#### P0-1：添加 Shell 层命令代理

**目标：** 拦截所有 shell 命令调用，确保 hostname、ifconfig、cat /etc/machine-id 等命令返回伪装结果。

**具体方案：**

在 `tools/` 目录下新增 `cac-shim/` 目录，为每个需要拦截的命令创建同名脚本：

```
tools/cac-shim/
├── hostname
├── ifconfig
├── cat          # 拦截 cat /etc/machine-id
├── sysctl       # 拦截 sysctl kern.uuid
├── ioreg        # macOS
├── system_profiler  # macOS
└── machine-id   # 写入假的 /etc/machine-id
```

**`tools/cac-shim/hostname` 示例：**
```bash
#!/bin/sh
echo "${SPOOF_HOSTNAME:-cloud-vm-default}"
```

**`tools/cac-shim/cat` 示例：**
```bash
#!/bin/sh
# 只拦截 /etc/machine-id，其他参数透传给真 cat
if [ "$1" = "/etc/machine-id" ]; then
  echo "${SPOOF_MACHINE_ID:-$(echo -n "${SPOOF_HOSTNAME}" | sha256sum | cut -c1-32)}"
else
  /bin/cat "$@"
fi
```

**`tools/claude-spoofed.sh` 修改：** 在启动前将 shim 目录加入 PATH 最前面：
```bash
SHIM_DIR="$SCRIPT_DIR/cac-shim"
export PATH="$SHIM_DIR:$PATH"
```

**验证方法：** 启动伪装环境后，在 shell 中直接执行 `hostname`、`cat /etc/machine-id`，确认返回伪装值。

---

#### P0-2：实现遥测阻断（核心层）

**目标：** 阻断 Claude Code 向 Statsig、Sentry、Segment 等服务发送遥测数据。

**具体方案 A — HOSTALIASES + 环境变量（推荐起步）：**

在 `tools/claude-spoofed.sh` 中追加：

```bash
# 遥测域名黑名单 → 全部指向 0.0.0.0
export HOSTALIASES="$SCRIPT_DIR/cac-telemetry-hosts"

# 创建 hosts 文件
cat > "$SCRIPT_DIR/cac-telemetry-hosts" << 'EOF'
0.0.0.0 statsig.anthropic.com
0.0.0.0 api.statsig.com
0.0.0.0 sentry.io
0.0.0.0 sentry-next.wbient.net
0.0.0.0 browser.sentry-cdn.com
0.0.0.0 segment.io
0.0.0.0 api.segment.io
0.0.0.0 cdn.segment.com
0.0.0.0 cdn-settings.com
0.0.0.0 api.mixpanel.com
0.0.0.0 events.mapbox.com
0.0.0.0 client.io
0.0.0.0 api.client.io
EOF

# 遥测阻断环境变量
export DO_NOT_TRACK=1
export SENTRY_DSN=""
export TELEMETRY_DISABLED=1
export STATSIC_ENABLED=0
export ANALYTICS_DISABLED=1
```

**具体方案 B — DNS 级别拦截（更彻底）：**

新建 `tools/cac-dns-guard.js`，通过 monkey-patch `dns.lookup()` 和全局 `fetch()`，拦截遥测域名：

```javascript
"use strict";
const dns = require("dns");
const BLOCKED_DOMAINS = [
  "statsig.anthropic.com",
  "api.statsig.com",
  "sentry.io",
  "segment.io",
  "api.segment.io",
];
const _lookup = dns.lookup;
dns.lookup = function (hostname, options, callback) {
  if (typeof options === "function") { callback = options; options = {}; }
  if (BLOCKED_DOMAINS.some(d => hostname === d || hostname.endsWith("." + d))) {
    const err = new Error(`getaddrinfo ENOTFOUND ${hostname}`);
    err.code = "ENOTFOUND";
    if (callback) process.nextTick(() => callback(err));
    return;
  }
  return _lookup.call(this, hostname, options, callback);
};
```

然后在 `NODE_OPTIONS` 中同时注入：
```bash
export NODE_OPTIONS="--require $SPOOF_SCRIPT --require $SCRIPT_DIR/cac-dns-guard.js ${NODE_OPTIONS:-}"
```

**推荐：先实施 A（HOSTALIASES），再追加 B（DNS guard）作为深度防御。**

---

#### P0-3：写入真实 /etc/machine-id 文件

**目标：** 在容器内写入伪装的 machine-id 文件，使 shell 层 `cat /etc/machine-id` 无需 shim 也能返回正确值。

**具体方案：**

在 `tools/claude-spoofed.sh` 启动前检测并写入：

```bash
# 如果当前用户有权限写入 /etc/machine-id，写入伪装值
MACHINE_ID="${SPOOF_MACHINE_ID:-$(echo -n "${SPOOF_HOSTNAME}" | sha256sum | cut -c1-32)}"
if [ -w /etc/machine-id ] 2>/dev/null; then
  echo "$MACHINE_ID" > /etc/machine-id
elif [ -w /var/lib/dbus/machine-id ] 2>/dev/null; then
  echo "$MACHINE_ID" > /var/lib/dbus/machine-id
fi
```

同时导出环境变量供 Node.js 层使用：
```bash
export SPOOF_MACHINE_ID="$MACHINE_ID"
```

---

### P1 — 建议实施（显著提升）

#### P1-1：添加 /etc/hostname 和 /etc/hosts 同步

**目标：** 确保 `/etc/hostname` 文件内容与伪装 hostname 一致。

**具体方案：**

```bash
# claude-spoofed.sh 中追加
if [ -w /etc/hostname ] 2>/dev/null; then
  echo "$SPOOF_HOSTNAME" > /etc/hostname
fi

# 更新 /etc/hosts 中的 127.0.1.1 映射
if [ -w /etc/hosts ] 2>/dev/null; then
  sed -i "s/^127\.0\.1\.1.*/127.0.1.1\t$SPOOF_HOSTNAME/" /etc/hosts 2>/dev/null || true
fi
```

#### P1-2：容器启动时集成指纹设置

**目标：** 将指纹伪装集成到 Docker 容器的 entrypoint 中，而非依赖用户手动运行。

**具体方案：**

在 `entrypoint.sh` 的 `prepare_v3_dirs` 之后追加 `prepare_fingerprint` 阶段：

```bash
prepare_fingerprint() {
  local fake_hostname="${SPOOF_HOSTNAME:-cloud-vm-$(echo -n "$CONTAINER_ID" | sha256sum | cut -c1-6)}"
  local fake_machine_id=$(echo -n "$fake_hostname" | sha256sum | cut -c1-32)

  # 写入 /etc/hostname
  echo "$fake_hostname" > /etc/hostname

  # 写入 /etc/machine-id
  echo "$fake_machine_id" > /etc/machine-id

  # 更新 /etc/hosts
  sed -i "s/^127\.0\.1\.1.*/127.0.1.1\t$fake_hostname/" /etc/hosts 2>/dev/null || true

  # 导出环境变量供 Node.js 进程使用
  export SPOOF_HOSTNAME="$fake_hostname"
  export SPOOF_MACHINE_ID="$fake_machine_id"
  export NODE_OPTIONS="--require /workspace/.cloud-claude/spoof-fingerprint.js ${NODE_OPTIONS:-}"
}
```

#### P1-3：DNS guard 覆盖更多遥测端点

**目标：** 扩展 `cac-dns-guard.js` 的拦截列表，覆盖所有已知的 Claude Code 遥测端点。

**具体端点列表（需持续更新）：**

```javascript
const BLOCKED_DOMAINS = [
  // Statsig
  "statsig.anthropic.com",
  "api.statsig.com",
  "statsigapi.net",
  // Sentry
  "sentry.io",
  "sentry-next.wbient.net",
  "sentry-cdn.com",
  "browser.sentry-cdn.com",
  // Segment
  "segment.io",
  "api.segment.io",
  "cdn.segment.com",
  "cdn-settings.com",
  // 其他
  "api.mixpanel.com",
  "events.mapbox.com",
  "api.client.io",
  // Anthropic 内部分析
  "api.anthropic.com",  // 注意：此域名同时用于 API 调用，需精确路径过滤
];
```

**注意：** `api.anthropic.com` 同时承载 API 请求和遥测，不能简单 DNS 拦截，需要在 fetch 层做路径级别过滤（只阻断 `/v1/telemetry` 等路径）。

---

### P2 — 可选增强（锦上添花）

#### P2-1：CLAUDE_CONFIG_DIR 隔离

**目标：** 每个用户环境使用独立的配置目录，防止状态污染。

**具体方案：**

```bash
# claude-spoofed.sh 中追加
export CLAUDE_CONFIG_DIR="${CAC_CONFIG_DIR:-$HOME/.claude-$SPOOF_HOSTNAME}"
mkdir -p "$CLAUDE_CONFIG_DIR"
```

#### P2-2：/proc/sys/kernel 指纹伪装（容器内）

**目标：** 防止通过 `/proc/sys/kernel/hostname` 或 `/proc/sys/kernel/osrelease` 泄露真实信息。

**具体方案：** 在容器内通过 `--read-only` 挂载 + tmpfs overlay 掩盖 `/proc/sys/kernel/hostname`（需要 `--privileged` 或 `SYS_ADMIN` capability，对安全性有影响，需权衡）。

**建议：** 仅在确认 Claude Code 会读取 `/proc` 时才实施，否则暂缓。

#### P2-3：mTLS 客户端证书（高级场景）

**目标：** 为每个容器环境签发独立的客户端证书，增强通信层隔离。

**适用场景：** 如果需要在代理层做细粒度的流量审计和身份识别。

**不建议在 v1 实施：** 当前 sing-box 隧道 + NAT masquerade 已经足够，mTLS 会增加证书管理复杂度。

---

## 五、结论

### 5.1 我们已有的优势

在网络层安全方面，我们的实现**显著领先**于 cac：
- sing-box uTLS chrome 指纹伪装（TLS Client Hello 级别）
- IPv6 泄漏防护
- iptables NAT masquerade 出口 IP 控制
- 全隧道 namespace 路由（所有流量强制走指定出口）

这些是 cac 方案中完全缺失的，也是我们产品的核心价值。

### 5.2 cac 的核心优势

cac 在**应用层指纹伪装**和**遥测阻断**方面做得更深入：
- Shell 层命令代理确保所有子进程都能拿到伪装结果
- 12 层遥测阻断覆盖 DNS、环境变量、文件系统等多个层面
- 更完整的机器身份伪装（machine-id、/etc/hostname、/etc/hosts 联动）

### 5.3 核心建议

1. **立即实施 P0-1（Shell shim）+ P0-2（遥测阻断）+ P0-3（machine-id 文件）**，这三个改动工作量不大（预计 1-2 天），但能显著缩小我们在应用层伪装方面的差距。

2. **跟进 P1 级别建议**，将指纹伪装集成到容器 entrypoint，实现开箱即用。

3. **P2 级别暂不实施**，除非有明确的检测风险报告。

4. **保持我们的网络层优势**，这是 cac 不具备的，也是我们产品的护城河。

### 5.4 风险提示

- 遥测阻断可能影响 Claude Code 的正常功能（如错误上报、功能开关等），需要充分测试
- Shell shim 的 PATH 注入可能与其他工具冲突，需要确保只在 Claude Code 环境内生效
- `/etc/machine-id` 写入需要容器有写权限，可能与只读根文件系统冲突

---

## 六、源码级深度对比（基于 cac 最新代码 clone）

> 以下对比基于 `cac` 源码 `/tmp/cac-analysis/` 与本项目 `tools/spoof-fingerprint.js` + `tools/claude-spoofed.sh` 的逐行分析。

### 6.1 Node.js 指纹补丁对比

| API | cac (`fingerprint-hook.js`) | 我们 (`spoof-fingerprint.js`) | 差异 |
|-----|---------------------------|-------------------------------|------|
| `os.hostname()` | 补丁，读 `CAC_HOSTNAME` | 补丁，读 `SPOOF_HOSTNAME` | 等价 |
| `os.userInfo()` | 只覆盖 `username` | 覆盖 `username` + `homedir` + `shell` | **我们更全面** |
| `os.networkInterfaces()` | 修改真实接口的 MAC（XOR 派生 per-interface） | 完全替换为虚构的 `eth0` + `lo` | cac 更隐蔽（保留真实接口名和 IP） |
| `os.platform/type/release/arch` | 未补丁 | 已补丁 | **我们更全面** |
| `os.totalmem/freemem` | 未补丁 | 已补丁 | **我们更全面** |
| `os.cpus()` | 未补丁 | 已补丁 | **我们更全面** |
| `os.homedir()` | 未补丁 | 已补丁 | **我们更全面** |
| `os.uptime()` | 未补丁 | 已补丁（7 天随机偏移） | **我们更全面** |
| `process.platform/arch` | 未补丁 | 已补丁（defineProperty） | **我们更全面** |
| `fs.readFileSync` (machine-id) | 补丁，读 `CAC_MACHINE_ID` | 未补丁（仅 child_process 拦截） | **cac 更深** |
| `fs.readFile` (machine-id) | 补丁（callback 版） | 未补丁 | **cac 更深** |
| `fs.promises.readFile` (machine-id) | 补丁 | 未补丁 | **cac 更深** |
| `fs.existsSync('/.dockerenv')` | 补丁（返回 false） | 未补丁 | **cac 更深** |
| `/proc/1/cgroup` 读取 | 补丁（替换 docker/containerd/kubepods → system.slice） | 未补丁 | **cac 更深** |
| `.git/config` 读取 | 补丁（替换 remote origin URL） | 未补丁 | **cac 更深** |
| Git email 拦截 | 补丁 `git config --get user.email` | 未补丁 | **cac 更深** |
| Git remote 命令拦截 | 补丁 `git remote get-url/-v/--get` | 未补丁 | **cac 更深** |
| MAC 地址派生 | SHA256 + XOR per-interface index | SHA256 直接截取 | cac 更真实 |

### 6.2 child_process 拦截对比

| 拦截目标 | cac | 我们 | 差异 |
|----------|-----|------|------|
| `ioreg` (macOS) | Shell shim（PATH 优先级） | Node.js child_process 内正则 | cac 更可靠 |
| `system_profiler` (macOS) | 无（macOS 通过 ioreg 已覆盖） | Node.js child_process 内正则 | 我们多余 |
| `sysctl kern.uuid` | 无（macOS 通过 ioreg 已覆盖） | Node.js child_process 内正则 | 我们多余 |
| `cat /etc/machine-id` | Shell shim `cat`（PATH 优先级）+ Node.js fs patch | Node.js child_process 内正则 | cac 双层 |
| `hostname` | Shell shim（PATH 优先级） | Node.js child_process 内正则 | cac 更可靠 |
| `ifconfig` | Shell shim（sed 替换 MAC） | 未拦截 | **cac 更深** |
| `wmic csproduct get uuid` (Windows) | Node.js child_process 内正则 | 未覆盖 | cac 更全 |
| `reg query MachineGuid` (Windows) | Node.js child_process 内正则 | 未覆盖 | cac 更全 |
| `git remote get-url/-v` | Node.js child_process 内正则 | 未拦截 | **cac 更深** |
| `git config --get user.email` | Node.js child_process 内正则 | 未拦截 | **cac 更深** |

### 6.3 遥测阻断对比（cac 独有）

| 阻断层 | cac 实现 | 我们 |
|--------|----------|------|
| `dns.lookup` 拦截 | `cac-dns-guard.js` 补丁 | 无 |
| `dns.resolve/resolve4/resolve6` 拦截 | 同上 | 无 |
| `dns.promises.*` 拦截 | 同上 | 无 |
| `net.connect/net.createConnection` 拦截 | 同上（安全网） | 无 |
| `globalThis.fetch` 拦截 | 同上（undici 路径） | 无 |
| `https.request/https.get` 拦截 | 同上 | 无 |
| HOSTALIASES 文件 | `blocked_hosts` → localhost | 无 |
| 环境变量（DO_NOT_TRACK 等） | 12 个变量 | 无 |
| 健康检查绕过 | 拦截 `api.anthropic.com/api/hello` 返回假 200 | 无 |

**cac 拦截的遥测域名：**
- `statsig.anthropic.com`
- `sentry.io`
- `o1137031.ingest.sentry.io`
- `cdn.growthbook.io`
- `http-intake.logs.us5.datadoghq.com`

### 6.4 网络层对比（cac 独有）

| 功能 | cac 实现 | 我们 |
|------|----------|------|
| mTLS 客户端证书 | `tls.connect` 拦截注入 + `https.globalAgent.options.ca` 注入 | 无 |
| 代理自动检测 | `curl ip-api.com` + 协议自动检测 | 手动配置 |
| 时区自动检测 | 通过代理 IP 检测时区和国家 | 无 |
| 心跳检测 | `relay.js` 内置 | 无 |

### 6.5 环境隔离对比

| 功能 | cac | 我们 |
|------|-----|------|
| `CLAUDE_CONFIG_DIR` 隔离 | 完整支持，每个环境独立 `.claude` 目录 | 无（共享配置） |
| settings.json 合并 | 支持 clone + override + 深度合并 | 无 |
| 环境切换 | `cac <name>` 一键切换 | 无 |
| 版本管理 | `cac claude install/pin/ls` | 无 |
| Persona 预设 | `macos-vscode`, `macos-cursor`, `macos-iterm`, `linux-desktop` | 无 |
| 遥测模式 | `stealth`（默认）, `paranoid`, `transparent` 三档 | 无 |
| 设备 token | 每环境独立 `device_token` | 无 |

### 6.6 关键发现总结

**我们做得更好的地方（os API 层）：**
- `os.platform/type/release/arch/tmpdir` — cac 未补丁
- `os.totalmem/freemem/cpus` — cac 未补丁
- `os.homedir/uptime` — cac 未补丁
- `process.platform/arch` — cac 未补丁
- `os.userInfo()` 补丁更全面（homedir + shell）

**cac 做得更深的地方（文件系统 + 容器检测层）：**
- `fs.readFileSync/readFile/promises.readFile` 三路 machine-id 拦截
- `fs.existsSync('/.dockerenv')` 容器检测绕过
- `/proc/1/cgroup` 内容清洗
- `.git/config` 读取拦截（防 repo hash 关联）
- Git email/remote 命令拦截

**cac 独有的关键能力：**
1. **遥测阻断**（DNS + fetch + net.connect + HOSTALIASES + 环境变量 + 健康检查绕过）— 我们完全缺失
2. **Shell shim**（PATH 优先级命令代理）— 我们完全缺失
3. **mTLS 客户端证书注入** — 我们完全缺失
4. **CLAUDE_CONFIG_DIR 环境隔离** — 我们完全缺失
5. **容器检测绕过**（/.dockerenv + cgroup）— 我们有占位符未实现

### 6.7 修订后的优先级建议

基于源码级分析，修订之前的优先级排序：

#### P0 — 必须实施（安全关键）

| 编号 | 建议 | 理由 | 工作量 |
|------|------|------|--------|
| P0-1 | **实现遥测阻断** | 最大安全风险。DNS guard + fetch + 环境变量三层阻断。直接复用 cac 的 `cac-dns-guard.js` 架构，适配我们的域名列表 | 半天 |
| P0-2 | **添加 Shell shim** | PATH 优先级 `hostname`、`cat`（machine-id）、`ifconfig` 脚本。确保非 Node.js 进程也能拿到伪装结果 | 半天 |
| P0-3 | **fs 层 machine-id 拦截** | 在 `spoof-fingerprint.js` 中补丁 `fs.readFileSync`、`fs.readFile`、`fs.promises.readFile`，拦截 `/etc/machine-id` 和 `/var/lib/dbus/machine-id` | 1 小时 |
| P0-4 | **容器检测绕过** | 补丁 `fs.existsSync('/.dockerenv')` 返回 false + `/proc/1/cgroup` 内容清洗 | 1 小时 |

#### P1 — 建议实施（显著提升）

| 编号 | 建议 | 理由 | 工作量 |
|------|------|------|--------|
| P1-1 | **Git remote/email 拦截** | 防止 Claude Code 通过 repo hash 做跨账户关联 | 2 小时 |
| P1-2 | **健康检查绕过** | 拦截 `api.anthropic.com/api/hello` 返回假 200，避免 Cloudflare 403 | 1 小时 |
| P1-3 | **CLAUDE_CONFIG_DIR 隔离** | 每个用户容器独立配置目录 | 1 小时 |
| P1-4 | **容器 entrypoint 集成** | 将指纹伪装集成到 Docker entrypoint，开箱即用 | 2 小时 |

#### P2 — 可选增强

| 编号 | 建议 | 理由 | 工作量 |
|------|------|------|--------|
| P2-1 | **mTLS 客户端证书** | 当前 sing-box 隧道已足够，额外价值有限 | 半天 |
| P2-2 | **MAC 地址 per-interface 派生** | XOR 派生更真实，但实际影响不大 | 1 小时 |
| P2-3 | **Persona 预设** | macOS/Linux 桌面环境预设，提升开箱体验 | 半天 |

---

## Self-Check: PASSED

- 计划文件 `.planning/quick/260422-cac-claude/260422-PLAN.md` 存在：FOUND
- 输出文件 `.planning/quick/260422-cac-claude/260422-SUMMARY.md` 存在：FOUND（本文件）
- 覆盖所有分析维度：指纹伪装对比、遥测阻断对比、DNS 拦截、容器反检测、mTLS、综合建议
- 包含具体代码层面建议：P0-1/P0-2/P0-3 均有完整代码示例
- 标注优先级：P0（3 项）、P1（3 项）、P2（3 项）
- 全部中文描述：通过
