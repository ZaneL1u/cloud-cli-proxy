#!/usr/bin/env bash
set -euo pipefail

log() {
  printf "[rebuild-host-runners] %s\n" "$*"
}

err() {
  printf "[rebuild-host-runners] ERROR: %s\n" "$*" >&2
}

usage() {
  cat <<'USAGE'
Usage:
  bash deploy/scripts/rebuild-host-runners.sh [options]

Options:
  --base-url URL      Control plane base URL (default: derive from BASE_URL/CONTROL_PLANE_ADDR, fallback http://127.0.0.1:8080)
  --username USER     Admin username (default: $ADMIN_USERNAME or admin)
  --password PASS     Admin password (default: $ADMIN_PASSWORD)
  --token TOKEN       Use an existing JWT token (skip login)
  --scope SCOPE       Host selection scope: all | running | non-running (default: all)
  --dry-run           Show targets only, do not queue rebuild tasks
  -h, --help          Show this help

Examples:
  ADMIN_PASSWORD='***' bash deploy/scripts/rebuild-host-runners.sh --scope all
  bash deploy/scripts/rebuild-host-runners.sh --token "$TOKEN" --scope non-running
USAGE
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    err "missing command: $1"
    exit 1
  fi
}

BASE_URL="${BASE_URL:-}"
if [[ -z "$BASE_URL" ]]; then
  CONTROL_PLANE_ADDR_VALUE="${CONTROL_PLANE_ADDR:-}"
  if [[ -z "$CONTROL_PLANE_ADDR_VALUE" ]]; then
    BASE_URL="http://127.0.0.1:8080"
  elif [[ "$CONTROL_PLANE_ADDR_VALUE" == http://* || "$CONTROL_PLANE_ADDR_VALUE" == https://* ]]; then
    BASE_URL="$CONTROL_PLANE_ADDR_VALUE"
  elif [[ "$CONTROL_PLANE_ADDR_VALUE" == :* ]]; then
    BASE_URL="http://127.0.0.1${CONTROL_PLANE_ADDR_VALUE}"
  else
    BASE_URL="http://${CONTROL_PLANE_ADDR_VALUE}"
  fi
fi
USERNAME="${ADMIN_USERNAME:-admin}"
PASSWORD="${ADMIN_PASSWORD:-}"
TOKEN="${TOKEN:-}"
SCOPE="all"
DRY_RUN=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --base-url)
      BASE_URL="$2"
      shift 2
      ;;
    --username)
      USERNAME="$2"
      shift 2
      ;;
    --password)
      PASSWORD="$2"
      shift 2
      ;;
    --token)
      TOKEN="$2"
      shift 2
      ;;
    --scope)
      SCOPE="$2"
      shift 2
      ;;
    --dry-run)
      DRY_RUN=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      err "unknown option: $1"
      usage
      exit 1
      ;;
  esac
done

case "$SCOPE" in
  all|running|non-running)
    ;;
  *)
    err "invalid --scope: $SCOPE (expected all|running|non-running)"
    exit 1
    ;;
esac

require_cmd curl
require_cmd jq

BASE_URL="${BASE_URL%/}"

if [[ -z "$TOKEN" ]]; then
  if [[ -z "$PASSWORD" ]]; then
    err "missing admin password: set ADMIN_PASSWORD or pass --password"
    exit 1
  fi

  log "logging in as ${USERNAME}..."
  login_payload=$(jq -n --arg username "$USERNAME" --arg password "$PASSWORD" '{username: $username, password: $password}')
  login_resp=$(curl -fsS -X POST "${BASE_URL}/v1/auth/login" \
    -H 'Content-Type: application/json' \
    -d "$login_payload")
  TOKEN=$(printf '%s' "$login_resp" | jq -r '.token // empty')

  if [[ -z "$TOKEN" ]]; then
    err "login succeeded without token in response"
    exit 1
  fi
fi

log "loading hosts from ${BASE_URL}/v1/admin/hosts ..."
hosts_resp=$(curl -fsS "${BASE_URL}/v1/admin/hosts" \
  -H "Authorization: Bearer ${TOKEN}")

host_selector='.hosts[]?'
case "$SCOPE" in
  running)
    host_selector='.hosts[]? | select((.docker_status // "") == "running")'
    ;;
  non-running)
    host_selector='.hosts[]? | select((.docker_status // "") != "running")'
    ;;
esac

host_lines=$(printf '%s' "$hosts_resp" | jq -r "${host_selector} | [.id, .status, (.docker_status // \"unknown\"), .username] | @tsv")

if [[ -z "${host_lines}" ]]; then
  log "no hosts matched scope='${SCOPE}'"
  exit 0
fi

log "matched hosts (scope=${SCOPE}):"
while IFS=$'\t' read -r host_id db_status docker_status username; do
  printf "  - %s (user=%s, db=%s, docker=%s)\n" "$host_id" "$username" "$db_status" "$docker_status"
done <<<"$host_lines"

if [[ "$DRY_RUN" -eq 1 ]]; then
  log "dry-run enabled, no rebuild tasks were queued"
  exit 0
fi

queued=0
failed=0
while IFS=$'\t' read -r host_id _db_status _docker_status _username; do
  resp_file=$(mktemp)
  status_code=$(curl -sS -o "$resp_file" -w '%{http_code}' -X POST \
    "${BASE_URL}/v1/admin/hosts/${host_id}/rebuild" \
    -H "Authorization: Bearer ${TOKEN}")

  if [[ "$status_code" == "202" ]]; then
    task_id=$(jq -r '.task_id // empty' "$resp_file")
    log "queued rebuild host=${host_id} task_id=${task_id:-unknown}"
    queued=$((queued + 1))
  else
    message=$(jq -r '.error // .message // empty' "$resp_file" 2>/dev/null || true)
    err "failed host=${host_id} status=${status_code} message=${message:-unknown}"
    failed=$((failed + 1))
  fi
  rm -f "$resp_file"
done <<<"$host_lines"

log "summary: queued=${queued}, failed=${failed}"
if [[ "$failed" -gt 0 ]]; then
  exit 1
fi
