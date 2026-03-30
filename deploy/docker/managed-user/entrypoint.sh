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

# SSH setup
mkdir -p /var/run/sshd
if ! ls /etc/ssh/ssh_host_*_key >/dev/null 2>&1; then
  ssh-keygen -A
fi

CONTAINER_USER="${CONTAINER_USER:-workspace}"
CONTAINER_PASSWORD="${CONTAINER_SSH_PASSWORD:-workspace}"

if [ "$CONTAINER_USER" != "workspace" ] && id workspace >/dev/null 2>&1; then
  usermod -l "$CONTAINER_USER" workspace
  groupmod -n "$CONTAINER_USER" workspace 2>/dev/null || true
  if [ -f /etc/sudoers.d/workspace ]; then
    sed -i "s/^workspace /${CONTAINER_USER} /" /etc/sudoers.d/workspace
  fi
fi

RUN_USER="${CONTAINER_USER:-workspace}"

mkdir -p /workspace/.ssh
chown -R "${RUN_USER}:${RUN_USER}" /workspace
chmod 0700 /workspace/.ssh

echo "${RUN_USER}:${CONTAINER_PASSWORD}" | chpasswd

# 禁用 IPv6（防止 IPv6 泄漏真实 IP）
sysctl -w net.ipv6.conf.all.disable_ipv6=1 >/dev/null 2>&1 || true
sysctl -w net.ipv6.conf.default.disable_ipv6=1 >/dev/null 2>&1 || true

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

# Foreground: sshd
exec /usr/sbin/sshd -D -e
