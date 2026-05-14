#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  scripts/prepare-firecracker-ubuntu-rootfs.sh [source_rootfs] [output_rootfs]

Description:
  Inject AIR's guest agent into an Ubuntu-based Firecracker rootfs and replace
  the guest init with a tiny AIR bootstrap script. This avoids depending on the
  upstream init system layout inside the CI rootfs.

Default paths:
  source_rootfs: assets/firecracker/ubuntu-rootfs.ext4
  output_rootfs: assets/firecracker/ubuntu-rootfs-air.ext4
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

SOURCE_ROOTFS="${1:-assets/firecracker/ubuntu-rootfs.ext4}"
OUTPUT_ROOTFS="${2:-assets/firecracker/ubuntu-rootfs-air.ext4}"
DEFAULT_PORT=10789
DEFAULT_PROXY_LISTEN="127.0.0.1:18080"
DEFAULT_PROXY_VSOCK_PORT=18080

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

require_cmd cp
require_cmd debugfs
require_cmd go
require_cmd install
require_cmd mkdir
require_cmd mkfs.ext4
require_cmd mktemp
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

cat >"${tmpdir}/init" <<EOF
#!/bin/sh
set -eu
mount -t proc proc /proc 2>/dev/null || true
mount -t sysfs sysfs /sys 2>/dev/null || true
mount -t devtmpfs devtmpfs /dev 2>/dev/null || true
mkdir -p /dev/pts
mount -t devpts devpts /dev/pts 2>/dev/null || true
if [ ! -e /dev/ptmx ]; then
  ln -s pts/ptmx /dev/ptmx 2>/dev/null || true
fi
chmod 666 /dev/ptmx /dev/pts/ptmx 2>/dev/null || true
mount -t tmpfs tmpfs /run 2>/dev/null || true
mount -t tmpfs tmpfs /tmp 2>/dev/null || true
mkdir -p /var/log /mnt/workspace-ro /mnt/workspace-rw /workspace
mount -t tmpfs tmpfs /var/log 2>/dev/null || true
ip link set lo up 2>/dev/null || ifconfig lo up 2>/dev/null || true
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
  export HTTP_PROXY=http://${DEFAULT_PROXY_LISTEN}
  export HTTPS_PROXY=http://${DEFAULT_PROXY_LISTEN}
  export ALL_PROXY=http://${DEFAULT_PROXY_LISTEN}
  export NO_PROXY=127.0.0.1,localhost
  /usr/bin/air-agent --network vsock --port ${DEFAULT_PORT} --host-proxy-listen ${DEFAULT_PROXY_LISTEN} --host-proxy-vsock-port ${DEFAULT_PROXY_VSOCK_PORT} >>"\${LOG_FILE}" 2>&1
) &
if command -v getty >/dev/null 2>&1; then
  getty -L ttyS0 115200 vt100 >/dev/ttyS0 2>&1 &
fi
while true; do
  wait || true
  sleep 1
done
EOF
chmod 0755 "${tmpdir}/init"

mkdir -p "${tmpdir}/rootfs"
echo "Extracting source rootfs..."
debugfs -R "rdump / ${tmpdir}/rootfs" "${SOURCE_ROOTFS}" >/dev/null 2>&1

echo "Injecting air-agent into Ubuntu rootfs..."
mkdir -p "${tmpdir}/rootfs/proc" "${tmpdir}/rootfs/sys" "${tmpdir}/rootfs/dev" "${tmpdir}/rootfs/run" "${tmpdir}/rootfs/tmp"
install -D -m 0755 "${tmpdir}/air-agent" "${tmpdir}/rootfs/usr/bin/air-agent"
if [[ -e "${tmpdir}/rootfs/sbin/init" && ! -e "${tmpdir}/rootfs/sbin/init.air-orig" ]]; then
  mv "${tmpdir}/rootfs/sbin/init" "${tmpdir}/rootfs/sbin/init.air-orig"
fi
install -D -m 0755 "${tmpdir}/init" "${tmpdir}/rootfs/sbin/init"

image_size="$(stat -c '%s' "${SOURCE_ROOTFS}")"
if [[ "${image_size}" -lt 1073741824 ]]; then
  image_size=1073741824
fi

echo "Rebuilding rootfs image at ${OUTPUT_ROOTFS}..."
rm -f "${OUTPUT_ROOTFS}"
truncate -s "${image_size}" "${OUTPUT_ROOTFS}"
mkfs.ext4 -q -F -d "${tmpdir}/rootfs" "${OUTPUT_ROOTFS}"

cat <<EOF

Prepared Ubuntu-based Firecracker rootfs:
  Source: ${SOURCE_ROOTFS}
  Output: ${OUTPUT_ROOTFS}

AIR will auto-discover ${OUTPUT_ROOTFS} when it lives under assets/firecracker.
You can also export:
  export AIR_FIRECRACKER_ROOTFS=${OUTPUT_ROOTFS}
  export AIR_FIRECRACKER_BOOT_ARGS="console=ttyS0 reboot=k panic=1 pci=off init=/sbin/init"
EOF
