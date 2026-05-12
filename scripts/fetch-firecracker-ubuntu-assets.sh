#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  scripts/fetch-firecracker-ubuntu-assets.sh [output_dir]

Description:
  Download the latest official Firecracker release binary and the Ubuntu guest
  assets used by Firecracker's current getting-started guide. The upstream CI
  rootfs is published as squashfs, so this script converts it into an ext4 image
  that AIR can boot directly.

Output layout:
  <output_dir>/
    firecracker
    vmlinux.bin
    ubuntu-rootfs.ext4
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

require_cmd basename
require_cmd curl
require_cmd grep
require_cmd mkfs.ext4
require_cmd sort
require_cmd tail
require_cmd tar
require_cmd truncate
require_cmd uname
require_cmd unsquashfs

mkdir -p "${OUTPUT_DIR}"
OUTPUT_DIR="$(cd "${OUTPUT_DIR}" && pwd)"

RELEASE_URL="https://github.com/firecracker-microvm/firecracker/releases"
LATEST_VERSION="$(basename "$(curl -fsSLI -o /dev/null -w '%{url_effective}' "${RELEASE_URL}/latest")")"
CI_VERSION="${LATEST_VERSION%.*}"
TARBALL_NAME="firecracker-${LATEST_VERSION}-${ARCH}.tgz"
KERNEL_OUTPUT="${OUTPUT_DIR}/vmlinux.bin"
ROOTFS_OUTPUT="${OUTPUT_DIR}/ubuntu-rootfs.ext4"

tmpdir="$(mktemp -d)"
cleanup() {
  rm -rf "${tmpdir}"
}
trap cleanup EXIT

echo "Downloading Firecracker ${LATEST_VERSION} for ${ARCH}..."
curl -fsSL "${RELEASE_URL}/download/${LATEST_VERSION}/${TARBALL_NAME}" -o "${tmpdir}/${TARBALL_NAME}"
tar -xzf "${tmpdir}/${TARBALL_NAME}" -C "${tmpdir}"
cp "${tmpdir}/release-${LATEST_VERSION}-${ARCH}/firecracker-${LATEST_VERSION}-${ARCH}" "${OUTPUT_DIR}/firecracker"
chmod +x "${OUTPUT_DIR}/firecracker"

KERNEL_KEY="$(curl -fsSL "http://spec.ccfc.min.s3.amazonaws.com/?prefix=firecracker-ci/${CI_VERSION}/${ARCH}/vmlinux-&list-type=2" \
  | grep -oP "(firecracker-ci/${CI_VERSION}/${ARCH}/vmlinux-[0-9]+\.[0-9]+\.[0-9]{1,3})" \
  | sort -V | tail -1)"
if [[ -z "${KERNEL_KEY}" ]]; then
  echo "failed to resolve latest Firecracker CI kernel for ${CI_VERSION}/${ARCH}" >&2
  exit 1
fi
echo "Downloading kernel ${KERNEL_KEY}..."
curl -fsSL "https://s3.amazonaws.com/spec.ccfc.min/${KERNEL_KEY}" -o "${KERNEL_OUTPUT}"

UBUNTU_KEY="$(curl -fsSL "http://spec.ccfc.min.s3.amazonaws.com/?prefix=firecracker-ci/${CI_VERSION}/${ARCH}/ubuntu-&list-type=2" \
  | grep -oP "(firecracker-ci/${CI_VERSION}/${ARCH}/ubuntu-[0-9]+\.[0-9]+\.squashfs)" \
  | sort -V | tail -1)"
if [[ -z "${UBUNTU_KEY}" ]]; then
  echo "failed to resolve latest Firecracker CI Ubuntu rootfs for ${CI_VERSION}/${ARCH}" >&2
  exit 1
fi

UBUNTU_SQUASHFS="${tmpdir}/ubuntu-rootfs.squashfs"
STAGE_ROOT="${tmpdir}/ubuntu-rootfs"
echo "Downloading rootfs ${UBUNTU_KEY}..."
curl -fsSL "https://s3.amazonaws.com/spec.ccfc.min/${UBUNTU_KEY}" -o "${UBUNTU_SQUASHFS}"

echo "Expanding squashfs rootfs..."
unsquashfs -f -d "${STAGE_ROOT}" "${UBUNTU_SQUASHFS}" >/dev/null

echo "Rebuilding ext4 rootfs..."
rm -f "${ROOTFS_OUTPUT}"
truncate -s 1G "${ROOTFS_OUTPUT}"
mkfs.ext4 -q -F -d "${STAGE_ROOT}" "${ROOTFS_OUTPUT}"

cat <<EOF

Downloaded and prepared:
  Firecracker: ${OUTPUT_DIR}/firecracker
  Kernel:      ${KERNEL_OUTPUT}
  Rootfs:      ${ROOTFS_OUTPUT}

Suggested environment:
  export AIR_VM_RUNTIME=firecracker
  export AIR_FIRECRACKER_BIN=${OUTPUT_DIR}/firecracker
  export AIR_FIRECRACKER_KERNEL=${KERNEL_OUTPUT}
  export AIR_FIRECRACKER_ROOTFS=${ROOTFS_OUTPUT}
  export AIR_KVM_DEVICE=/dev/kvm
EOF
