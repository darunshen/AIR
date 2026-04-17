#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  scripts/fetch-firecracker-demo-assets.sh [output_dir]

Description:
  Download the latest official Firecracker release binary and the official
  Firecracker demo kernel/rootfs assets.

Notes:
  - This script follows the official Firecracker getting-started flow.
  - It intentionally skips SSH key patching because AIR does not use SSH yet.
  - The demo rootfs is downloaded directly as ext4, so no local image build is needed.

Output layout:
  <output_dir>/
    firecracker
    hello-vmlinux.bin
    hello-rootfs.ext4
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

OUTPUT_DIR="${1:-assets/firecracker}"
ARCH="${AIR_FIRECRACKER_ARCH:-$(uname -m)}"

case "${ARCH}" in
  x86_64|aarch64)
    ;;
  *)
    echo "unsupported architecture: ${ARCH}" >&2
    exit 1
    ;;
esac

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

require_cmd curl
require_cmd tar
require_cmd grep
require_cmd sort
require_cmd tail
require_cmd basename
require_cmd uname

mkdir -p "${OUTPUT_DIR}"
OUTPUT_DIR="$(cd "${OUTPUT_DIR}" && pwd)"

RELEASE_URL="https://github.com/firecracker-microvm/firecracker/releases"
LATEST_VERSION="$(basename "$(curl -fsSLI -o /dev/null -w '%{url_effective}' "${RELEASE_URL}/latest")")"
tarball_name="firecracker-${LATEST_VERSION}-${ARCH}.tgz"
kernel_name="hello-vmlinux.bin"
rootfs_name="hello-rootfs.ext4"
kernel_url="https://s3.amazonaws.com/spec.ccfc.min/img/hello/kernel/${kernel_name}"
rootfs_url="https://s3.amazonaws.com/spec.ccfc.min/img/hello/fsfiles/${rootfs_name}"

tmpdir="$(mktemp -d)"
cleanup() {
  rm -rf "${tmpdir}"
}
trap cleanup EXIT

echo "Downloading Firecracker ${LATEST_VERSION} for ${ARCH}..."
curl -fsSL "${RELEASE_URL}/download/${LATEST_VERSION}/${tarball_name}" -o "${tmpdir}/${tarball_name}"
tar -xzf "${tmpdir}/${tarball_name}" -C "${tmpdir}"
cp "${tmpdir}/release-${LATEST_VERSION}-${ARCH}/firecracker-${LATEST_VERSION}-${ARCH}" "${OUTPUT_DIR}/firecracker"
chmod +x "${OUTPUT_DIR}/firecracker"

echo "Downloading demo kernel asset ${kernel_name}..."
curl -fsSL "${kernel_url}" -o "${OUTPUT_DIR}/${kernel_name}"

echo "Downloading demo rootfs asset ${rootfs_name}..."
curl -fsSL "${rootfs_url}" -o "${OUTPUT_DIR}/${rootfs_name}"

cat <<EOF

Downloaded and prepared:
  Firecracker: ${OUTPUT_DIR}/firecracker
  Kernel:      ${OUTPUT_DIR}/${kernel_name}
  Rootfs:      ${OUTPUT_DIR}/${rootfs_name}

Suggested environment:
  export AIR_VM_RUNTIME=firecracker
  export AIR_FIRECRACKER_BIN=${OUTPUT_DIR}/firecracker
  export AIR_FIRECRACKER_KERNEL=${OUTPUT_DIR}/${kernel_name}
  export AIR_FIRECRACKER_ROOTFS=${OUTPUT_DIR}/${rootfs_name}
  export AIR_KVM_DEVICE=/dev/kvm
EOF
