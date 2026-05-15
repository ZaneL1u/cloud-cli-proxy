#!/usr/bin/env bash
set -euo pipefail

LOG_DIR=/workspace/.vnc
XVNC_LOG="${LOG_DIR}/xvnc.log"
FLUXBOX_LOG="${LOG_DIR}/fluxbox.log"
DESKTOP_LOG="${LOG_DIR}/desktop.log"
CHROMIUM_LOG="${LOG_DIR}/chromium.log"
DESKTOP_DIR=/workspace/Desktop
PCMANFM_PROFILE_DIR=/workspace/.config/pcmanfm/default

write_desktop_config() {
  mkdir -p "${DESKTOP_DIR}" "${PCMANFM_PROFILE_DIR}" /workspace/.chrome-data

  cat > "${DESKTOP_DIR}/Chrome.desktop" <<'DESKTOP'
[Desktop Entry]
Version=1.0
Type=Application
Name=Chrome
Comment=Open the browser
Exec=/usr/local/bin/launch-chromium.sh
Icon=chromium
Terminal=false
StartupNotify=true
Categories=Network;WebBrowser;
DESKTOP

  cat > "${PCMANFM_PROFILE_DIR}/desktop-items-0.conf" <<'CONF'
[*]
desktop_bg=#17324d
desktop_fg=#f5f7ff
desktop_shadow=#1b1f2a
show_wm_menu=0
wallpaper_mode=color
sort=name;ascending;
show_documents=0
show_trash=0
show_mounts=0
CONF

  chmod 0755 "${DESKTOP_DIR}/Chrome.desktop"
  chown -R "${RUN_USER}:${RUN_USER}" "${DESKTOP_DIR}" /workspace/.config /workspace/.chrome-data
}

wait_for_x_display() {
  local display="${1:-:99}"
  local timeout_seconds="${2:-30}"
  local deadline=$((SECONDS + timeout_seconds))

  while (( SECONDS < deadline )); do
    if DISPLAY="${display}" xdpyinfo >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done

  return 1
}

# ===== v3.0 stages — D-09 / PITFALLS M4 串行快速失败 =====

prepare_v3_dirs() {
  echo "[entrypoint] v3: chown /home/claude /workspace-hot /workspace-cold /var/lib/claude-persist"
  chown -R 1000:1000 \
    /home/claude \
    /workspace-hot \
    /workspace-cold \
    /var/lib/claude-persist 2>/dev/null || true
}

prepare_persistent_state() {
  local root=/var/lib/claude-persist
  mkdir -p "$root/.claude" "$root/.cache/claude"

  if [ -d /home/claude/.claude ] && [ -z "$(ls -A "$root/.claude" 2>/dev/null)" ]; then
    cp -an /home/claude/.claude/. "$root/.claude/" 2>/dev/null || true
  fi
  if [ -d /home/claude/.cache/claude ] && [ -z "$(ls -A "$root/.cache/claude" 2>/dev/null)" ]; then
    cp -an /home/claude/.cache/claude/. "$root/.cache/claude/" 2>/dev/null || true
  fi

  chown -R 1000:1000 "$root"

  rm -rf /home/claude/.claude /home/claude/.cache/claude
  ln -sfn "$root/.claude" /home/claude/.claude
  mkdir -p /home/claude/.cache
  ln -sfn "$root/.cache/claude" /home/claude/.cache/claude

  chown -h 1000:1000 /home/claude/.claude /home/claude/.cache/claude

  echo "[entrypoint] v3: persistent state ready (volume=/var/lib/claude-persist)"
}

prepare_container_disguise() {
  # Per-container unique machine-id（基于 hostname + uptime，保证唯一性）
  local h; h="$(hostname)"
  local t; t="$(cat /proc/uptime 2>/dev/null | tr -d ' .')"
  local mid; mid="$(echo -n "${h}-${t}" | sha256sum | cut -c1-32)"
  echo "$mid" > /etc/machine-id
  echo "$mid" > /var/lib/dbus/machine-id 2>/dev/null || true
  chmod 444 /etc/machine-id

  # 删除容器检测标志
  rm -f /.dockerenv

  # 遥测阻断环境变量（写入 /etc/environment 供所有用户 session 继承）
  cat >> /etc/environment <<'ENVTELEM'
DISABLE_TELEMETRY=1
CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1
CLAUDE_CODE_ENHANCED_TELEMETRY_BETA=
DO_NOT_TRACK=1
OTEL_SDK_DISABLED=true
OTEL_TRACES_EXPORTER=none
OTEL_METRICS_EXPORTER=none
OTEL_LOGS_EXPORTER=none
SENTRY_DSN=
DISABLE_ERROR_REPORTING=1
TELEMETRY_DISABLED=1
ENVTELEM

  echo "[entrypoint] container disguise: machine-id generated, /.dockerenv removed, telemetry blocked"
}

prepare_mergerfs_check() {
  if ! command -v mergerfs >/dev/null 2>&1; then
    echo "[entrypoint] v3: FATAL mergerfs binary missing" >&2
    exit 1
  fi
  local ver
  ver="$(mergerfs --version 2>&1 | head -n1 || true)"
  echo "[entrypoint] v3: mergerfs available ($ver) — mount deferred to cloud-claude (Phase 31)"
  # SC1 / C1 / C2 — documented params for Phase 31 (static traceability):
  #   func.readdir=cor:4,cache.attr=30,cache.entry=30,cache.readdir=true,cache.files=off
  #   category.create=ff, inodecalc=path-hash
  #   2-way: /workspace-hot=RW:/workspace-cold=NC,RO
  # Q10: CLOUD_CLAUDE_MERGERFS_BRANCHES reserved for Phase 31
  echo "[entrypoint] v3: mergerfs params (Phase 31): func.readdir=cor:4 category.create=ff inodecalc=path-hash"
}

assert_tmux_version() {
  local tmux_ver
  tmux_ver="$(tmux -V 2>/dev/null | awk '{print $2}' || true)"
  case "$tmux_ver" in
    3.4*|3.5*|3.6*|3.7*|3.8*|3.9*|[4-9].*)
      echo "[entrypoint] v3: tmux ${tmux_ver} >= 3.4 ok"
      echo "$tmux_ver" >/etc/cloud-claude/tmux.version
      ;;
    *)
      echo "[entrypoint] v3: FATAL tmux ${tmux_ver} < 3.4" >&2
      exit 1
      ;;
  esac
}

# ===== v4.0 (Phase 53): sing-box 同容器化启动序列 — fail-closed =====
# D-V4-1..4 + D-53-2/3/4/5/6 集中实现：
# - start_singbox_or_die：runuser → uid=9000 + 文件 cap + tun0 waitFor
# - lock_resolv_conf：DNS 强制走 sing-box stub (127.0.0.1:53)
# - apply_nft_or_die：容器内 nft default-deny ruleset
# - remove_singbox_config：sing-box load 后 shred config 从 fs
# - monitor_singbox_fail_closed：sing-box 死 → kill PID 1 → 容器死

SING_BOX_CONFIG="/etc/sing-box/config.json"
SING_BOX_USER="singbox"
NFT_RULESET="/etc/cloud-claude/default-deny.nft"
TUN_READY_TIMEOUT_S=30
SING_BOX_PID=""

start_singbox_or_die() {
  if [ ! -f "$SING_BOX_CONFIG" ]; then
    echo "[entrypoint] FATAL: sing-box config 不存在: $SING_BOX_CONFIG" >&2
    exit 1
  fi

  # 验证 config 文件权限（D-V4-2 / D-53-3 前置）
  # CR-01 (53-REVIEW): sing-box 跑在 uid=9000，root:0600 在 DAC 下 read 必失败。
  # 改为 root:singbox 0640 — owner=root 写、group=singbox 读，最小权限暴露。
  local perm owner group
  perm="$(stat -c '%a' "$SING_BOX_CONFIG")"
  owner="$(stat -c '%U' "$SING_BOX_CONFIG")"
  group="$(stat -c '%G' "$SING_BOX_CONFIG")"
  if [ "$perm" != "640" ] || [ "$owner" != "root" ]; then
    echo "[entrypoint] FATAL: config 权限不对（want root:singbox 0640，got ${owner}:${group}:${perm}）" >&2
    exit 1
  fi
  if [ "$group" != "singbox" ]; then
    echo "[entrypoint] FATAL: config group 不对（want singbox，got ${group}）" >&2
    exit 1
  fi

  # 启动 sing-box（runuser → singbox uid=9000，binary 文件 cap 提供 NET_ADMIN）
  echo "[entrypoint] starting sing-box as uid=9000 (file-cap based)"
  runuser -u "$SING_BOX_USER" -- /usr/local/bin/sing-box run -c "$SING_BOX_CONFIG" &
  SING_BOX_PID=$!

  # WaitFor tun0 ready（替代裸 sleep）
  echo "[entrypoint] waiting for tun0 (timeout=${TUN_READY_TIMEOUT_S}s)"
  local deadline=$((SECONDS + TUN_READY_TIMEOUT_S))
  while (( SECONDS < deadline )); do
    if ip link show tun0 >/dev/null 2>&1; then
      if kill -0 "$SING_BOX_PID" 2>/dev/null; then
        local sb_uid
        sb_uid="$(ps -o uid= -p "$SING_BOX_PID" 2>/dev/null | tr -d ' ' || echo 'unknown')"
        echo "[entrypoint] tun0 ready (sing-box pid=$SING_BOX_PID, uid=$sb_uid)"
        return 0
      fi
    fi
    sleep 0.5
  done

  echo "[entrypoint] FATAL: tun0 未在 ${TUN_READY_TIMEOUT_S}s 内就绪" >&2
  if [ -n "$SING_BOX_PID" ]; then
    kill "$SING_BOX_PID" 2>/dev/null || true
  fi
  exit 1
}

lock_resolv_conf() {
  echo "[entrypoint] locking /etc/resolv.conf to sing-box stub"
  cat > /etc/resolv.conf <<'EOF'
# v4.0: DNS 强制走 sing-box stub resolver (D-V4-3)
nameserver 127.0.0.1
options edns0 trust-ad
EOF
  chmod 0644 /etc/resolv.conf
  # chattr +i 让用户无法修改（即便 user 是 root，没 cap_linux_immutable 也改不了）
  # T5: overlay2 storage driver 可能不支持 immutable bit，仅 WARN，依赖 nft 兜底
  if ! chattr +i /etc/resolv.conf 2>/dev/null; then
    echo "[entrypoint] WARN: chattr +i /etc/resolv.conf 失败（可能是 overlayfs），依赖 nft drop 兜底"
  fi
}

apply_nft_or_die() {
  echo "[entrypoint] applying nft default-deny ruleset: $NFT_RULESET"
  if [ ! -f "$NFT_RULESET" ]; then
    echo "[entrypoint] FATAL: nft ruleset 不存在: $NFT_RULESET" >&2
    if [ -n "$SING_BOX_PID" ]; then kill "$SING_BOX_PID" 2>/dev/null || true; fi
    exit 1
  fi
  if ! nft -f "$NFT_RULESET"; then
    echo "[entrypoint] FATAL: nft 应用失败" >&2
    if [ -n "$SING_BOX_PID" ]; then kill "$SING_BOX_PID" 2>/dev/null || true; fi
    exit 1
  fi
  # 二次 verify
  if ! nft list table inet cloud_proxy_v4 >/dev/null 2>&1; then
    echo "[entrypoint] FATAL: nft table cloud_proxy_v4 未生效" >&2
    if [ -n "$SING_BOX_PID" ]; then kill "$SING_BOX_PID" 2>/dev/null || true; fi
    exit 1
  fi
  echo "[entrypoint] nft default-deny applied"
}

remove_singbox_config() {
  # D-V4-2: sing-box 已加载到内存，从 fs 删除 config
  echo "[entrypoint] removing sing-box config from fs (D-V4-2)"
  shred -u "$SING_BOX_CONFIG" 2>/dev/null || rm -f "$SING_BOX_CONFIG"
  if [ -f "$SING_BOX_CONFIG" ]; then
    echo "[entrypoint] FATAL: config rm 失败" >&2
    if [ -n "$SING_BOX_PID" ]; then kill "$SING_BOX_PID" 2>/dev/null || true; fi
    exit 1
  fi
}

monitor_singbox_fail_closed() {
  # D-V4-4 / EP-04 / NET-03: sing-box 死 → 整个容器死
  # 注意：用 kill -0 polling 而不是 wait —— wait 只能等当前 shell 的子进程，
  # 子 shell `(...)` 看不到父 shell 的 background process，wait 会立即返回。
  (
    while kill -0 "$SING_BOX_PID" 2>/dev/null; do
      sleep 1
    done
    echo "[entrypoint] FATAL: sing-box 退出，触发容器死 (sing-box pid=$SING_BOX_PID)" >&2
    # 给 tini (PID 1) 发 TERM，让它清理子进程并退出
    kill -TERM 1 2>/dev/null || true
    # 兜底：如果 kill 1 没生效，硬退出
    sleep 2
    kill -KILL 1 2>/dev/null || true
  ) &
  echo "[entrypoint] sing-box fail-closed monitor armed (monitor pid=$!, sing-box pid=$SING_BOX_PID)"
}

# SSH setup
mkdir -p /var/run/sshd
if ! ls /etc/ssh/ssh_host_*_key >/dev/null 2>&1; then
  ssh-keygen -A
fi

CONTAINER_USER="${CONTAINER_USER:-workspace}"
CONTAINER_PASSWORD="${CONTAINER_SSH_PASSWORD:-workspace}"

# 清除 Ubuntu 镜像自带的 ubuntu 用户（UID 1000 冲突会导致 whoami 返回 ubuntu）
if [ "$CONTAINER_USER" != "ubuntu" ] && id ubuntu >/dev/null 2>&1; then
  userdel -f ubuntu 2>/dev/null || true
  groupdel ubuntu 2>/dev/null || true
fi

if [ "$CONTAINER_USER" != "workspace" ] && id workspace >/dev/null 2>&1; then
  usermod -l "$CONTAINER_USER" workspace
  groupmod -n "$CONTAINER_USER" workspace 2>/dev/null || true
  # v4.0 (D-53-4): 不再 sed sudoers — v4.0 移除了用户 sudo NOPASSWD 路径。
fi

RUN_USER="${CONTAINER_USER:-workspace}"

# v4.0 (D-53-4): 删除 v3.x 的 sudoers NOPASSWD 写入。
# 用户拿到 shell 后不再有任何 sudo / root 提权路径。
# 兜底清理（防御历史镜像残留 / volume 挂载的 sudoers.d）：
rm -f /etc/sudoers.d/workspace 2>/dev/null || true
if [ "${RUN_USER}" != "workspace" ]; then
  rm -f "/etc/sudoers.d/${RUN_USER}" 2>/dev/null || true
fi

mkdir -p /workspace/.ssh
chown -R "${RUN_USER}:${RUN_USER}" /workspace
chmod 0700 /workspace/.ssh

# 注入 SSH 公钥（local 模式或显式传入时）
if [ -n "${CONTAINER_SSH_AUTHORIZED_KEY:-}" ]; then
  echo "${CONTAINER_SSH_AUTHORIZED_KEY}" > /workspace/.ssh/authorized_keys
  chmod 0600 /workspace/.ssh/authorized_keys
  chown "${RUN_USER}:${RUN_USER}" /workspace/.ssh/authorized_keys
fi

echo "${RUN_USER}:${CONTAINER_PASSWORD}" | chpasswd

# Phase 29.1: 验证密码已被成功设置，避免"密码退化为 workspace"的静默失败复现。
# passwd -S 第 2 列语义：P / PS = 已设置密码；L / LK = 已锁定；NP = 无密码；UNSET = 读取失败。
status="$(passwd -S "${RUN_USER}" 2>/dev/null | awk '{print $2}' || echo UNSET)"
case "$status" in
  P|PS)
    ;;
  *)
    echo "[entrypoint] FATAL: password status for ${RUN_USER} is '${status}', refusing to start" >&2
    exit 1
    ;;
esac

# 禁用 IPv6（防止 IPv6 泄漏真实 IP）
sysctl -w net.ipv6.conf.all.disable_ipv6=1 >/dev/null 2>&1 || true
sysctl -w net.ipv6.conf.default.disable_ipv6=1 >/dev/null 2>&1 || true

if [ -c /dev/fuse ]; then
  chmod 666 /dev/fuse
fi

# ===== v4.0: sing-box 启动序列（所有 MODE 都跑，fail-fast）=====
# 顺序固定，任一步失败 entrypoint 非 0 退出 → tini 关停容器（EP-01 / D-V4-4）
start_singbox_or_die
lock_resolv_conf
apply_nft_or_die
remove_singbox_config
monitor_singbox_fail_closed

# MODE 检测：remote（默认）= 完整桌面栈，local = 仅 sshd
MODE="${MODE:-remote}"

if [ "$MODE" != "local" ]; then
# KasmVNC 配置（无密码认证——由控制面反代保护）
mkdir -p /workspace/.vnc
cat > /workspace/.vnc/kasmvnc.yaml <<'YAML'
network:
  protocol: http
  websocket_port: 6080
  ssl:
    require_ssl: false
    pem_certificate:
    pem_key:
  udp:
    public_ip: 127.0.0.1
    stun_server:
desktop:
  resolution:
    width: 1920
    height: 1080
  allow_resize: true
  pixel_depth: 24
keyboard:
  remap_keys:
  ignore_numlock: false
  raw_keyboard: false
pointer:
  enabled: true
runtime_configuration:
  allow_client_to_override_kasm_server_settings: true
  allow_override_standard_vnc_server_settings: true
  allow_override_list:
    - pointer.enabled
    - desktop.allow_resize
    - desktop.resolution
encoding:
  max_frame_rate: 60
  rect_encoding_mode:
    min_quality: 7
    max_quality: 10
    consider_lossless_quality: 10
    rectangle_compress_threads: 4
YAML
chown -R "${RUN_USER}:${RUN_USER}" /workspace/.vnc
touch "${XVNC_LOG}" "${FLUXBOX_LOG}" "${DESKTOP_LOG}" "${CHROMIUM_LOG}"
chown "${RUN_USER}:${RUN_USER}" "${XVNC_LOG}" "${FLUXBOX_LOG}" "${DESKTOP_LOG}" "${CHROMIUM_LOG}"

# 创建 KasmVNC 用户（非交互，避免卡在 TUI 提示）
echo -e "kasmpass\nkasmpass\n" | kasmvncpasswd -u "${RUN_USER}" -w /workspace/.vnc/passwd 2>/dev/null || true
chown "${RUN_USER}:${RUN_USER}" /workspace/.vnc/passwd

# 创建 /tmp/.X11-unix（非 root 用户启动 Xvnc 需要）
mkdir -p /tmp/.X11-unix
chmod 1777 /tmp/.X11-unix

# 清理可能残留的 X11 lock 文件（容器 restart 后 /tmp 可能仍保留上次运行的 lock）
rm -f /tmp/.X99-lock /tmp/.X11-unix/X99

# 启动 KasmVNC（Xvnc 直接启动，跳过 vncserver perl 脚本的交互提示）
export DISPLAY=:99
su "${RUN_USER}" -c 'Xvnc :99 \
  -geometry 1920x1080 \
  -depth 24 \
  -websocketPort 6080 \
  -SecurityTypes None \
  -interface 0.0.0.0 \
  -BlacklistThreshold 0 \
  -FreeKeyMappings \
  -disableBasicAuth \
  -publicIP 127.0.0.1 \
  -httpd /usr/share/kasmvnc/www' >>"${XVNC_LOG}" 2>&1 &

if ! wait_for_x_display ":99" 30; then
  echo "Xvnc did not become ready on DISPLAY :99 within 30 seconds" >>"${XVNC_LOG}"
  exit 1
fi

write_desktop_config

# 提前设置根窗口背景，pcmanfm 接管前也不会闪成纯黑。
DISPLAY=:99 xsetroot -solid "#17324d" >/dev/null 2>&1 || true

su "${RUN_USER}" -c 'DISPLAY=:99 fluxbox' >>"${FLUXBOX_LOG}" 2>&1 &
su "${RUN_USER}" -c 'DISPLAY=:99 HOME=/workspace pcmanfm --desktop --profile default' >>"${DESKTOP_LOG}" 2>&1 &

# 预热一遍 Chromium 检测，方便排查图标点击失败。
su "${RUN_USER}" -c 'HOME=/workspace /usr/local/bin/launch-chromium.sh --version' >>"${CHROMIUM_LOG}" 2>&1 || true

# ===== v3.0 stages (serialized fail-fast, D-09 order) =====
prepare_v3_dirs
prepare_persistent_state
prepare_container_disguise
prepare_mergerfs_check
assert_tmux_version

fi # end MODE != "local"

# v4.0 (Phase 53): 旧的 MODE=local sing-box 分支已删除。
# 出网现在由顶部 start_singbox_or_die / apply_nft_or_die 序列统一处理，
# 所有 MODE 都强制走 sing-box tun + nft default-deny，没有 proxy fallback。
# 用户的 ALL_PROXY / HTTP_PROXY 环境变量也不再注入（应用应直接走 tun）。

# Foreground: sshd
exec /usr/sbin/sshd -D -e
