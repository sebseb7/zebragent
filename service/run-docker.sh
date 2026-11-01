#!/usr/bin/env bash
# Runs zebragent in a distroless container with access to host USB (usbfs).
# Builds the static binary via build-docker.sh if missing (REBUILD=1 forces).
# Extra args are passed to zebragent, e.g.: ./run-docker.sh -apiurl https://...
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUTPUT="${OUTPUT:-zebragent}"
IMAGE="${IMAGE:-gcr.io/distroless/static-debian12:nonroot}"
# Reuse the host config (pairing token, API URL, printer) so the container is already paired.
CONFIG_DIR="${CONFIG_DIR:-${XDG_CONFIG_HOME:-$HOME/.config}/zebragent}"

if [[ ! -x "${SCRIPT_DIR}/${OUTPUT}" || "${REBUILD:-0}" == "1" ]]; then
  OUTPUT="${OUTPUT}" "${SCRIPT_DIR}/build-docker.sh"
fi

mkdir -p "${CONFIG_DIR}"

echo "Running ${OUTPUT} in ${IMAGE} with USB access..."

# --network host: local monitor binds 127.0.0.1 and printer IP is reachable.
# /dev/bus/usb is passed through, with a cgroup rule allowing USB char devices (major 189).
# Runs as root so usbfs nodes are accessible regardless of host permissions.
# Default DETACH=1 (set DETACH=0 for foreground): run in background, named, and restart automatically (incl. at boot via the docker daemon).
RUN_OPTS=(--rm)
if [[ "${DETACH:-1}" == "1" ]]; then
  docker rm -f zebragent >/dev/null 2>&1 || true
  RUN_OPTS=(-d --name zebragent --restart unless-stopped)
fi

exec docker run "${RUN_OPTS[@]}" \
  --user 0:0 \
  --network host \
  --device /dev/bus/usb \
  --device-cgroup-rule='c 189:* rmw' \
  -v /dev/bus/usb:/dev/bus/usb \
  -v "${SCRIPT_DIR}/${OUTPUT}:/zebragent:ro" \
  -v "${CONFIG_DIR}:/state/zebragent" \
  -e HOME=/state \
  -e XDG_CONFIG_HOME=/state \
  "${IMAGE}" \
  /zebragent "$@"
