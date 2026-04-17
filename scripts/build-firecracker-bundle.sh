#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  scripts/build-firecracker-bundle.sh <output_dir> <arch>

Description:
  Build the AIR official Firecracker bundle for release distribution.

Output:
  <output_dir>/air_firecracker_linux_<arch>.tar.gz
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

if [[ $# -lt 2 ]]; then
  usage >&2
  exit 1
fi

OUTPUT_DIR="$1"
ARCH="$2"
case "${ARCH}" in
  amd64)
    FIRECRACKER_ARCH="x86_64"
    ;;
  arm64)
    FIRECRACKER_ARCH="aarch64"
    ;;
  *)
    echo "unsupported arch: ${ARCH}" >&2
    exit 1
    ;;
esac

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUTPUT_DIR_ABS="$(mkdir -p "${OUTPUT_DIR}" && cd "${OUTPUT_DIR}" && pwd)"
STAGE_DIR="$(mktemp -d)"
ASSET_DIR="${STAGE_DIR}/assets/firecracker"
trap 'rm -rf "${STAGE_DIR}"' EXIT

AIR_FIRECRACKER_ARCH="${FIRECRACKER_ARCH}" "${ROOT_DIR}/scripts/fetch-firecracker-demo-assets.sh" "${ASSET_DIR}"
GOARCH="${ARCH}" "${ROOT_DIR}/scripts/prepare-firecracker-rootfs.sh" \
  "${ASSET_DIR}/hello-rootfs.ext4" \
  "${ASSET_DIR}/hello-rootfs-air.ext4"

rm -f "${ASSET_DIR}/hello-rootfs.ext4"

ARCHIVE_NAME="air_firecracker_linux_${ARCH}.tar.gz"
tar -C "${ASSET_DIR}" -czf "${OUTPUT_DIR_ABS}/${ARCHIVE_NAME}" .

echo "Built ${OUTPUT_DIR_ABS}/${ARCHIVE_NAME}"
