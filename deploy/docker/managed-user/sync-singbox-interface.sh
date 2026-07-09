#!/usr/bin/env bash
set -euo pipefail

SOURCE_CONFIG="${SING_BOX_SOURCE_CONFIG:-/etc/sing-box/config.json}"
RUNTIME_CONFIG="${SING_BOX_RUNTIME_CONFIG:-/run/sing-box/config.json}"
STATE_FILE="${SING_BOX_EGRESS_IF_FILE:-/run/cloud-cli-proxy/egress-interface}"
SING_BOX_UID="${SING_BOX_UID:-9000}"
IF_PRESENT=false

usage() {
  cat >&2 <<'EOF'
usage: cloud-cli-proxy-sync-singbox-interface [--source PATH] [--config PATH] [--if-present]

Copies the sing-box source config to a writable runtime config when needed,
detects the real container egress interface for proxy-out, and writes
bind_interface/default_interface to that runtime config.

--if-present skips quietly when the runtime config is absent. This is used
after sing-box has already loaded and removed the runtime config from disk.
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --source)
      SOURCE_CONFIG="${2:-}"
      shift 2
      ;;
    --config)
      RUNTIME_CONFIG="${2:-}"
      shift 2
      ;;
    --if-present)
      IF_PRESENT=true
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "[singbox-iface] unknown argument: $1" >&2
      usage
      exit 2
      ;;
  esac
done

if [ -z "$SOURCE_CONFIG" ] || [ -z "$RUNTIME_CONFIG" ]; then
  echo "[singbox-iface] source/config path must not be empty" >&2
  exit 2
fi

if [ ! -f "$RUNTIME_CONFIG" ]; then
  if [ "$IF_PRESENT" = true ]; then
    echo "[singbox-iface] runtime config absent, skip"
    exit 0
  fi
  if [ ! -f "$SOURCE_CONFIG" ]; then
    echo "[singbox-iface] source config missing: $SOURCE_CONFIG" >&2
    exit 1
  fi
  mkdir -p "$(dirname "$RUNTIME_CONFIG")"
  cp "$SOURCE_CONFIG" "$RUNTIME_CONFIG"
fi

proxy_server="$(jq -r '.outbounds[]? | select(.tag == "proxy-out") | .server // empty' "$RUNTIME_CONFIG" | head -n1)"
proxy_port="$(jq -r '.outbounds[]? | select(.tag == "proxy-out") | .server_port // empty' "$RUNTIME_CONFIG" | head -n1)"

iface=""
if printf '%s' "$proxy_server" | grep -Eq '^[0-9]+(\.[0-9]+){3}$'; then
  iface="$(ip -4 route get "$proxy_server" uid "$SING_BOX_UID" 2>/dev/null | awk '
    {
      for (i = 1; i <= NF; i++) {
        if ($i == "dev") {
          print $(i + 1)
          exit
        }
      }
    }'
  )"
fi

if [ -z "$iface" ]; then
  iface="$(ip -4 route show default 2>/dev/null | awk '{print $5; exit}')"
fi

case "$iface" in
  ""|lo|tun*)
    echo "[singbox-iface] invalid detected egress interface: '${iface}'" >&2
    exit 1
    ;;
esac

tmp="$(mktemp "${RUNTIME_CONFIG}.tmp.XXXXXX")"
jq --arg iface "$iface" '
  .route.default_interface = $iface
  | .outbounds = (
      .outbounds | map(
        if .tag == "proxy-out" or .tag == "direct" then
          .bind_interface = $iface
        else
          .
        end
      )
    )
' "$RUNTIME_CONFIG" > "$tmp"
mv "$tmp" "$RUNTIME_CONFIG"

mkdir -p "$(dirname "$STATE_FILE")"
printf '%s\n' "$iface" > "$STATE_FILE"
chmod 0640 "$RUNTIME_CONFIG" 2>/dev/null || true

if [ -n "$proxy_server" ] && [ -n "$proxy_port" ]; then
  echo "[singbox-iface] proxy-out ${proxy_server}:${proxy_port} bound to ${iface}"
else
  echo "[singbox-iface] proxy-out bound to ${iface}"
fi
