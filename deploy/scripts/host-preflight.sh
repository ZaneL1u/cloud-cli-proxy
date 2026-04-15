#!/usr/bin/env bash
set -euo pipefail

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

# FUSE kernel module must be loadable (required for sshfs directory mapping)
if ! modprobe fuse 2>/dev/null; then
  if [ ! -c /dev/fuse ]; then
    echo "missing fuse kernel module (required for sshfs directory mapping)" >&2
    echo "try: modprobe fuse" >&2
    exit 1
  fi
fi

if [ ! -c /dev/fuse ]; then
  echo "missing /dev/fuse device (required for container FUSE mount)" >&2
  exit 1
fi

# Phase 2: nsenter required for container namespace verification
require_cmd nsenter

# Phase 2: nft required for container firewall rules
require_cmd nft

# Phase 2: curl required for egress IP verification
require_cmd curl

mkdir -p /var/lib/cloud-cli-proxy
mkdir -p /run/cloud-cli-proxy
