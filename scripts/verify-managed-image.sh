#!/usr/bin/env bash
set -euo pipefail

IMAGE_NAME="$(awk -F': ' '$1 == "local_dev_image_name" { print $2 }' deploy/docker/managed-user/image.lock)"

if [[ -z "${IMAGE_NAME}" ]]; then
  echo "failed to read local_dev_image_name from deploy/docker/managed-user/image.lock" >&2
  exit 1
fi

docker image inspect "${IMAGE_NAME}" >/dev/null
docker run --rm --entrypoint sh "${IMAGE_NAME}" -lc 'sshd -T >/dev/null && command -v claude && command -v chromium && command -v xdpyinfo && command -v xterm && command -v pcmanfm && getent passwd workspace && test -d /workspace'
grep -n "^Port 22$" deploy/docker/managed-user/sshd_config >/dev/null
