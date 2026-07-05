#!/usr/bin/env bash
set -euo pipefail

# 平台检测：macOS Docker Desktop vs Linux 原生 Docker
detect_platform() {
  # macOS Docker Desktop / OrbStack 的特征：overlayfs 不支持 chattr +i、
  # APFS 不支持 chown root:singbox、/dev/net/tun 行为不同
  local testfile="/tmp/.dev_mode_test_$$"
  if ! touch "$testfile" 2>/dev/null || ! chattr +i "$testfile" 2>/dev/null; then
    DEV_MODE=true
    rm -f "$testfile"
  else
    chattr -i "$testfile" 2>/dev/null || true
    rm -f "$testfile"
    DEV_MODE=false
  fi
  readonly DEV_MODE
  echo "[entrypoint] platform: DEV_MODE=$DEV_MODE"
}
detect_platform

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


prepare_workspace_dirs() {
  echo "[entrypoint] chown workspace dirs"
  chown -R "${RUN_USER}:${RUN_USER}" \
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

  chown -R "${RUN_USER}:${RUN_USER}" "$root" 2>/dev/null || true

  rm -rf /home/claude/.claude /home/claude/.cache/claude
  ln -sfn "$root/.claude" /home/claude/.claude
  mkdir -p /home/claude/.cache
  ln -sfn "$root/.cache/claude" /home/claude/.cache/claude

  chown -h "${RUN_USER}:${RUN_USER}" /home/claude/.claude /home/claude/.cache/claude 2>/dev/null || true

  echo "[entrypoint] persistent state ready (volume=/var/lib/claude-persist)"
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

  # 遥测阻断环境变量。覆盖写入（非追加）防止容器 restart 重复追加。
  cat > /etc/environment <<'ENVTELEM'
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
    echo "[entrypoint] FATAL mergerfs binary missing" >&2
    exit 1
  fi
  local ver
  ver="$(mergerfs --version 2>&1 | head -n1 || true)"
  echo "[entrypoint] mergerfs available ($ver) — mount deferred to cloud-claude"
  # mergerfs params (static traceability):
  #   func.readdir=cor:4,cache.attr=30,cache.entry=30,cache.readdir=true,cache.files=off
  #   category.create=ff, inodecalc=path-hash
  #   2-way: /workspace-hot=RW:/workspace-cold=NC,RO
  echo "[entrypoint] mergerfs params: func.readdir=cor:4 category.create=ff inodecalc=path-hash"
}

assert_tmux_version() {
  local tmux_ver
  tmux_ver="$(tmux -V 2>/dev/null | awk '{print $2}' || true)"
  case "$tmux_ver" in
    3.4*|3.5*|3.6*|3.7*|3.8*|3.9*|[4-9].*)
      echo "[entrypoint] tmux ${tmux_ver} >= 3.4 ok"
      echo "$tmux_ver" >/etc/cloud-claude/tmux.version
      ;;
    *)
      echo "[entrypoint] FATAL tmux ${tmux_ver} < 3.4" >&2
      exit 1
      ;;
  esac
}

# ===== v4.0 (Phase 53): sing-box 同容器化启动序列 — fail-closed =====
# D-V4-1..4 + D-53-2/3/4/5/6 集中实现：
# - start_singbox_or_die：runuser → uid=9000 + 文件 cap + tun0 waitFor
# - lock_resolv_conf：DNS 强制走 sing-box DNS inbound (127.0.0.1:53)
# - apply_nft_or_die：容器内 nft default-deny ruleset
# - remove_singbox_config：sing-box load 后 shred config 从 fs
# - monitor_singbox_fail_closed：sing-box 死 → kill PID 1 → 容器死

SING_BOX_CONFIG="/etc/sing-box/config.json"
SING_BOX_USER="singbox"
NFT_RULESET="/etc/cloud-claude/default-deny.nft"
BYPASS_DIR="/etc/cloud-claude/bypass"
TUN_READY_TIMEOUT_S=30
SING_BOX_PID=""
SSHD_PID=""

prepare_bypass_rule_sets() {
  # v4.1: sing-box rule_set 引用白名单文件，必须预先存在。
  # 每次启动都覆盖写入空规则，防止旧磁盘残留导致 bypass 状态不干净。
  mkdir -p "$BYPASS_DIR"
  echo '{"version":3,"rules":[]}' > "$BYPASS_DIR/whitelist-cidrs.json"
  echo '{"version":3,"rules":[]}' > "$BYPASS_DIR/whitelist-domains.json"
  chown -R root:root "$BYPASS_DIR" 2>/dev/null || true
  echo "[entrypoint] bypass rule_set files ready"
}

start_singbox_or_die() {
  if [ ! -f "$SING_BOX_CONFIG" ]; then
    echo "[entrypoint] FATAL: sing-box config 不存在: $SING_BOX_CONFIG" >&2
    exit 1
  fi

  # 验证 config 文件权限（D-V4-2 / D-53-3 前置）
  # CR-01 (53-REVIEW): sing-box 跑在 uid=9000，root:0600 在 DAC 下 read 必失败。
  # 改为 root:singbox 0640 — owner=root 写、group=singbox 读，最小权限暴露。
  #
  # macOS Docker Desktop 开发环境：host-agent chown root:singbox 必失败（APFS 不支持），
  # config 文件保持 host user 权限。entrypoint 以 root 身份在容器内修复权限后
  # 继续 hard-assert，prod Linux 上修复操作是 no-op。
  if ! chown root:singbox "$SING_BOX_CONFIG" 2>/dev/null; then
    if [ "$DEV_MODE" = true ]; then
      echo "[entrypoint] DEV: chown failed, falling back to 644"
      chmod 0644 "$SING_BOX_CONFIG" 2>/dev/null || true
    else
      echo "[entrypoint] FATAL: chown root:singbox failed" >&2
      exit 1
    fi
  else
    chmod 0640 "$SING_BOX_CONFIG" 2>/dev/null || true
  fi

  local perm owner group
  perm="$(stat -c '%a' "$SING_BOX_CONFIG")"
  owner="$(stat -c '%U' "$SING_BOX_CONFIG")"
  group="$(stat -c '%G' "$SING_BOX_CONFIG")"
  if [ "$perm" = "644" ] && [ "$owner" = "root" ] && [ "$group" = "root" ]; then
    # macOS Docker Desktop dev 环境：host-agent chown 失败，降级为 644。
    # singbox 用户通过 other:r 位读取 config，安全由 :ro bind mount + 生产环境 hard-assert 兜底。
    echo "[entrypoint] WARN: config 权限为 dev 降级模式（root:root:644），仅限 macOS dev 环境" >&2
  elif [ "$perm" != "640" ] || [ "$owner" != "root" ]; then
    echo "[entrypoint] FATAL: config 权限不对（want root:singbox 0640，got ${owner}:${group}:${perm}）" >&2
    exit 1
  elif [ "$group" != "singbox" ]; then
    echo "[entrypoint] FATAL: config group 不对（want singbox，got ${group}）" >&2
    exit 1
  fi

  # 启动 sing-box（setpriv → singbox uid=9000，binary 文件 cap 提供 NET_ADMIN）
  # HI-03 (53-REVIEW): 原先用 runuser 会走 PAM session fork，$! 抓到的是 wrapper PID（uid=0）
  # 而非真正的 sing-box 进程，导致 ps 输出谎报 uid=0。setpriv 来自 util-linux 不走 PAM，
  # 不 fork —— $! 直接是 sing-box 主进程 PID，uid=9000 可观测。
  echo "[entrypoint] starting sing-box as uid=9000 (setpriv, no PAM fork)"
  setpriv --reuid="$SING_BOX_USER" --regid="$SING_BOX_USER" --init-groups \
    /usr/local/bin/sing-box run -c "$SING_BOX_CONFIG" &
  SING_BOX_PID=$!

  # WaitFor tun0 ready（替代裸 sleep）
  echo "[entrypoint] waiting for tun0 (timeout=${TUN_READY_TIMEOUT_S}s)"
  local deadline=$((SECONDS + TUN_READY_TIMEOUT_S))
  while (( SECONDS < deadline )); do
    if ip link show tun0 >/dev/null 2>&1; then
      if kill -0 "$SING_BOX_PID" 2>/dev/null; then
        local sb_uid
        sb_uid="$(ps -o uid= -p "$SING_BOX_PID" 2>/dev/null | tr -d ' ' || echo 'unknown')"
        # HI-03 hard-assert：必须 uid=9000，否则 fail-closed（防 setpriv 静默 fallback）
        if [ "$sb_uid" != "9000" ]; then
          echo "[entrypoint] FATAL: sing-box uid=$sb_uid != 9000（fail-closed）" >&2
          kill "$SING_BOX_PID" 2>/dev/null || true
          exit 1
        fi
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
  # v4.0: DNS 指向 127.0.0.1，由 sing-box DNS inbound 接管。
  # 不能用 tun0 IP (172.19.0.1) —— 内核本地处理该地址的包，tun 设备收不到。
  echo "[entrypoint] locking /etc/resolv.conf to sing-box DNS inbound (127.0.0.1)"
  cat > /etc/resolv.conf <<'EOF'
# v4.0: DNS 强制走 sing-box DNS inbound (D-V4-3)
nameserver 127.0.0.1
options edns0 trust-ad
EOF
  chmod 0644 /etc/resolv.conf
  # chattr +i 让用户无法修改（即便 user 是 root，没 cap_linux_immutable 也改不了）
  # T5: overlay2 storage driver 可能不支持 immutable bit，仅 WARN，依赖 nft 兜底
  if ! chattr +i /etc/resolv.conf 2>/dev/null; then
    if [ "$DEV_MODE" = true ]; then
      echo "[entrypoint] DEV: chattr +i not supported (overlayfs), relying on nft"
    else
      echo "[entrypoint] WARN: chattr +i failed unexpectedly, checking nft fallback"
    fi
  fi
}

apply_nft_or_die() {
  echo "[entrypoint] applying nft default-deny ruleset: $NFT_RULESET"
  if [ ! -f "$NFT_RULESET" ]; then
    echo "[entrypoint] FATAL: nft ruleset 不存在: $NFT_RULESET" >&2
    if [ -n "$SING_BOX_PID" ]; then kill "$SING_BOX_PID" 2>/dev/null || true; fi
    exit 1
  fi
  # TODO(v5): 清理 v3.x 残留的 ip cloudproxy 表。所有 v3.x 部署迁移完毕后移除此行。
  nft delete table ip cloudproxy 2>/dev/null || true
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
  # dev 环境：config 以 :ro bind mount 注入，无法删除；跳过即可。
  if touch "$SING_BOX_CONFIG" 2>/dev/null; then
    if ! shred -u "$SING_BOX_CONFIG" 2>/dev/null && ! rm -f "$SING_BOX_CONFIG" 2>/dev/null; then
      echo "[entrypoint] FATAL: config rm failed" >&2
      if [ -n "$SING_BOX_PID" ]; then kill "$SING_BOX_PID" 2>/dev/null || true; fi
      exit 1
    fi
  else
    if [ "$DEV_MODE" = true ]; then
      echo "[entrypoint] DEV: config on ro bind mount, skip shred"
    else
      echo "[entrypoint] WARN: config remove failed, continuing"
    fi
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

start_sshd_or_die() {
  mkdir -p /run/sshd /var/run/sshd
  echo "[entrypoint] starting sshd"
  /usr/sbin/sshd -D -e &
  SSHD_PID=$!

  local deadline=$((SECONDS + 10))
  while (( SECONDS < deadline )); do
    if ! kill -0 "$SSHD_PID" 2>/dev/null; then
      echo "[entrypoint] FATAL: sshd exited during startup" >&2
      exit 1
    fi
    if bash -lc '</dev/tcp/127.0.0.1/22' >/dev/null 2>&1; then
      echo "[entrypoint] sshd ready (pid=$SSHD_PID)"
      return 0
    fi
    sleep 0.5
  done

  echo "[entrypoint] FATAL: sshd not ready within 10s" >&2
  kill "$SSHD_PID" 2>/dev/null || true
  exit 1
}

start_desktop_stack() {
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

  echo -e "kasmpass\nkasmpass\n" | kasmvncpasswd -u "${RUN_USER}" -w /workspace/.vnc/passwd 2>/dev/null || true
  chown "${RUN_USER}:${RUN_USER}" /workspace/.vnc/passwd

  mkdir -p /tmp/.X11-unix
  chmod 1777 /tmp/.X11-unix
  rm -f /tmp/.X99-lock /tmp/.X11-unix/X99

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
    return 1
  fi

  write_desktop_config

  su "${RUN_USER}" -c 'DISPLAY=:99 xsetroot -solid "#17324d"' >/dev/null 2>&1 || true
  su "${RUN_USER}" -c 'DISPLAY=:99 fluxbox' >>"${FLUXBOX_LOG}" 2>&1 &
  su "${RUN_USER}" -c 'DISPLAY=:99 HOME=/workspace pcmanfm --desktop --profile default' >>"${DESKTOP_LOG}" 2>&1 &
  su "${RUN_USER}" -c 'HOME=/workspace /usr/local/bin/launch-chromium.sh --version' >>"${CHROMIUM_LOG}" 2>&1 || true

  prepare_workspace_dirs
  prepare_persistent_state
  prepare_container_disguise
  prepare_mergerfs_check
  assert_tmux_version

  echo "[entrypoint] desktop stack ready (display=:99 websocket=6080)"
}

start_desktop_stack_async() {
  (
    if ! start_desktop_stack; then
      echo "[entrypoint] WARN: desktop stack failed; SSH remains available" >&2
    fi
  ) &
  echo "[entrypoint] desktop stack starting in background (pid=$!)"
}

# SSH setup
mkdir -p /var/run/sshd
if ! ls /etc/ssh/ssh_host_*_key >/dev/null 2>&1; then
  ssh-keygen -A
fi

CONTAINER_USER="${CONTAINER_USER:-workspace}"
CONTAINER_PASSWORD="${CONTAINER_SSH_PASSWORD:-workspace}"

# HI-01 (53-REVIEW): 校验 CONTAINER_USER 合法性，防止路径穿越 / glob 注入。
# 参考 useradd NAME_REGEX：仅允许 [a-z_][a-z0-9_-]{0,30}（POSIX portable username）。
# 任何带 / .. * \0 \n 等非法字符的输入在 sing-box 启动前 fail-closed。
if ! [[ "$CONTAINER_USER" =~ ^[a-z_][a-z0-9_-]{0,30}$ ]]; then
  echo "[entrypoint] FATAL: CONTAINER_USER 非法（仅允许 POSIX portable username [a-z_][a-z0-9_-]{0,30}）: $CONTAINER_USER" >&2
  exit 1
fi

# 清除 Ubuntu 镜像自带的 ubuntu 用户（UID 1000 冲突会导致 whoami 返回 ubuntu）
if [ "$CONTAINER_USER" != "ubuntu" ] && id ubuntu >/dev/null 2>&1; then
  userdel -f ubuntu 2>/dev/null || true
  groupdel ubuntu 2>/dev/null || true
fi

if [ "$CONTAINER_USER" != "workspace" ] && id workspace >/dev/null 2>&1; then
  usermod -l "$CONTAINER_USER" workspace
  groupmod -n "$CONTAINER_USER" workspace 2>/dev/null || true
  # v4.0: 用户无 sudo 权限（安全基线）
fi

RUN_USER="${CONTAINER_USER:-workspace}"

# v4.0: 用户无 sudo 权限。兜底清理历史镜像残留的 sudoers.d：
# TODO(v5): 移除此 v3.x sudoers 兜底清理。
rm -f /etc/sudoers.d/workspace 2>/dev/null || true
if [ "${RUN_USER}" != "workspace" ]; then
  rm -f "/etc/sudoers.d/${RUN_USER}" 2>/dev/null || true
fi

mkdir -p /workspace/.ssh
# -xdev 防止 chown 递归进入 bind mount 点（如 macOS 挂载目录），
# 避免在 virtiofs/FUSE 文件系统上遍历海量文件导致卡死。
find /workspace -xdev -exec chown "${RUN_USER}:${RUN_USER}" {} + 2>/dev/null || true
chmod 0700 /workspace/.ssh

# 注入 SSH 公钥（local 模式或显式传入时）
if [ -n "${CONTAINER_SSH_AUTHORIZED_KEY:-}" ]; then
  echo "${CONTAINER_SSH_AUTHORIZED_KEY}" > /workspace/.ssh/authorized_keys
  chmod 0600 /workspace/.ssh/authorized_keys
  chown "${RUN_USER}:${RUN_USER}" /workspace/.ssh/authorized_keys
fi

echo "${RUN_USER}:${CONTAINER_PASSWORD}" | chpasswd

# 验证密码已被成功设置，避免"密码退化为 workspace"的静默失败复现。
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

fix_singbox_routing() {
  # v4.0 (Phase 54): sing-box auto_route 默认在规则 9001 上设置
  # suppress_prefixlength 0，该选项会压制 table 2022 中的默认路由，
  # 导致用户流量回退到 main 表走 Docker 外联路由 → 被 nft default-deny 丢弃。
  #
  # 修复分三步：
  #   1. 为 sing-box 自身 (uid=9000) 添加优先规则走 main 表，
  #      防止移除 suppress_prefixlength 后 sing-box 代理流量环路。
  #   2. 删除规则 9001 并用无 suppress_prefixlength 的版本替换。
  #   3. 在 table 2022 中添加所有本地子网的直连路由，确保 VNC/SSH
  #      的返回流量不走 tun0 → sing-box（sing-box 无法处理内网流量）。
  #
  # 关键操作失败时输出 ERROR 但不退出。set +e 防止 macOS Docker Desktop
  # 或特定内核版本下 ip 命令的非预期返回码触发 set -e 终止整个 entrypoint。

  set +e
  local failed=0

  ip rule add pref 8999 uidrange 9000-9000 lookup main 2>/dev/null || {
    echo "[entrypoint] ERROR: cannot add ip rule 8999 (sing-box main route)" >&2
    failed=1
  }

  ip rule del pref 9001 2>/dev/null || true
  ip rule add pref 9001 from all lookup 2022 2>/dev/null || {
    echo "[entrypoint] ERROR: cannot add ip rule 9001 (user tun0 route)" >&2
    failed=1
  }

  # 从主路由表复制本地子网直连路由到 table 2022。
  # ip addr show 返回的是主机 IP 如 192.168.107.5/24，不是网络地址，
  # 直接用于 ip route add 会报 Invalid prefix 错误。
  # 改为从 main 表已有的 proto kernel 路由复制，避免手动计算网络地址。
  while read -r r; do
    [ -n "$r" ] && ip route add $r table 2022 2>/dev/null || true
  done < <(ip route show | grep "proto kernel")
  ip route add 127.0.0.0/8 dev lo table 2022 2>/dev/null || true

  if [ "$failed" -eq 1 ]; then
    echo "[entrypoint] WARN: some routing fixes failed — fail-closed by nft" >&2
  else
    echo "[entrypoint] sing-box routing fixed (uid 9000 → main, user → tun0)"
  fi
  set -e
}

# ===== v4.0: sing-box 启动序列（所有 MODE 都跑，fail-fast）=====
# 顺序固定，任一步失败 entrypoint 非 0 退出 → tini 关停容器（EP-01 / D-V4-4）
# H8: nft 尽早应用（start_singbox → apply_nft → fix_routing → lock_resolv），
# 避免 sing-box 启动后 nft 应用前的泄漏窗口。
prepare_bypass_rule_sets
start_singbox_or_die
apply_nft_or_die
fix_singbox_routing
lock_resolv_conf
remove_singbox_config
monitor_singbox_fail_closed
start_sshd_or_die

# MODE 检测：remote（默认）= 完整桌面栈，local = 仅 sshd
MODE="${MODE:-remote}"

if [ "$MODE" != "local" ]; then
  start_desktop_stack_async
fi

wait "$SSHD_PID"
