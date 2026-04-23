#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  scripts/prepare-firecracker-rootfs.sh [source_rootfs] [output_rootfs]

Description:
  Build the guest-side `air-agent`, unpack the source ext4 rootfs into a
  staging directory, wire the agent into the Firecracker guest boot chain
  through OpenRC `local.d`, and rebuild a fresh ext4 image.

Default paths:
  source_rootfs: assets/firecracker/hello-rootfs.ext4
  output_rootfs: assets/firecracker/hello-rootfs-air.ext4

Result:
  - /usr/bin/air-agent
  - /etc/local.d/air-agent.start
  - /etc/runlevels/default/local -> /etc/init.d/local
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

SOURCE_ROOTFS="${1:-assets/firecracker/hello-rootfs.ext4}"
OUTPUT_ROOTFS="${2:-assets/firecracker/hello-rootfs-air.ext4}"
DEFAULT_PORT=10789

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

require_cmd go
require_cmd install
require_cmd cp
require_cmd ln
require_cmd mktemp
require_cmd debugfs
require_cmd mkfs.ext4
require_cmd truncate

if [[ ! -f "${SOURCE_ROOTFS}" ]]; then
  echo "source rootfs not found: ${SOURCE_ROOTFS}" >&2
  exit 1
fi

mkdir -p "$(dirname "${OUTPUT_ROOTFS}")"
SOURCE_ROOTFS="$(cd "$(dirname "${SOURCE_ROOTFS}")" && pwd)/$(basename "${SOURCE_ROOTFS}")"
OUTPUT_ROOTFS="$(cd "$(dirname "${OUTPUT_ROOTFS}")" && pwd)/$(basename "${OUTPUT_ROOTFS}")"

tmpdir="$(mktemp -d)"
cleanup() {
  rm -rf "${tmpdir}"
}
trap cleanup EXIT

: "${GOCACHE:=/tmp/go-build}"
: "${GOMODCACHE:=/tmp/gomodcache}"
export GOCACHE GOMODCACHE

echo "Building air-agent..."
CGO_ENABLED=0 GOOS=linux GOARCH="${GOARCH:-$(go env GOARCH)}" \
  go build -trimpath -o "${tmpdir}/air-agent" ./cmd/air-agent

cat >"${tmpdir}/air-agent.start" <<EOF
#!/bin/sh
LOG_FILE=/run/air-agent.log
echo "[air-agent] boot hook start" >>"\${LOG_FILE}"
echo "[air-agent] boot hook start" >>/dev/console 2>&1 || true
if [ -b /dev/vdb ] && [ -b /dev/vdc ]; then
  mount -o ro /dev/vdb /mnt/workspace-ro >>"\${LOG_FILE}" 2>&1 || true
  mount /dev/vdc /mnt/workspace-rw >>"\${LOG_FILE}" 2>&1 || true
  mkdir -p /mnt/workspace-rw/upper /mnt/workspace-rw/work
  mount -t overlay overlay -o lowerdir=/mnt/workspace-ro,upperdir=/mnt/workspace-rw/upper,workdir=/mnt/workspace-rw/work /workspace >>"\${LOG_FILE}" 2>&1 || true
fi
(
  /usr/bin/air-agent --network vsock --port ${DEFAULT_PORT} >>"\${LOG_FILE}" 2>&1
  code=\$?
  echo "[air-agent] exited code=\${code}" >>"\${LOG_FILE}"
  echo "[air-agent] exited code=\${code}" >>/dev/console 2>&1 || true
) &
pid=\$!
echo "[air-agent] launched pid=\${pid}" >>"\${LOG_FILE}"
echo "[air-agent] launched pid=\${pid}" >>/dev/console 2>&1 || true
sleep 1
if ! kill -0 "\${pid}" 2>/dev/null; then
  echo "[air-agent] died during startup" >>/dev/console 2>&1 || true
  cat "\${LOG_FILE}" >>/dev/console 2>&1 || true
fi
EOF
chmod 0755 "${tmpdir}/air-agent.start"

mkdir -p "${tmpdir}/rootfs"
echo "Extracting source rootfs..."
debugfs -R "rdump / ${tmpdir}/rootfs" "${SOURCE_ROOTFS}" >/dev/null 2>&1

echo "Injecting air-agent into staging rootfs..."
mkdir -p "${tmpdir}/rootfs/mnt/workspace-ro" "${tmpdir}/rootfs/mnt/workspace-rw" "${tmpdir}/rootfs/workspace"
install -D -m 0755 "${tmpdir}/air-agent" "${tmpdir}/rootfs/usr/bin/air-agent"
install -D -m 0755 "${tmpdir}/air-agent.start" "${tmpdir}/rootfs/etc/local.d/air-agent.start"
rm -f "${tmpdir}/rootfs/etc/runlevels/default/local"
ln -s /etc/init.d/local "${tmpdir}/rootfs/etc/runlevels/default/local"

image_size="$(stat -c '%s' "${SOURCE_ROOTFS}")"
if [[ "${image_size}" -lt 67108864 ]]; then
  image_size=67108864
fi

echo "Rebuilding rootfs image at ${OUTPUT_ROOTFS}..."
rm -f "${OUTPUT_ROOTFS}"
truncate -s "${image_size}" "${OUTPUT_ROOTFS}"
mkfs.ext4 -q -F -d "${tmpdir}/rootfs" "${OUTPUT_ROOTFS}"

cat <<EOF

Prepared Firecracker rootfs:
  Source: ${SOURCE_ROOTFS}
  Output: ${OUTPUT_ROOTFS}

AIR will auto-discover ${OUTPUT_ROOTFS} when it lives under assets/firecracker.
You can also export:
  export AIR_FIRECRACKER_ROOTFS=${OUTPUT_ROOTFS}
EOF
