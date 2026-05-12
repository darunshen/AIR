#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  scripts/build-openclaude-firecracker-bundle.sh <output_dir> <openclaude_repo> <arch>

Description:
  Build the AIR official Firecracker guest bundle for OpenClaude.

Output:
  <output_dir>/air_openclaude_firecracker_linux_<arch>.tar.gz
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

if [[ $# -lt 3 ]]; then
  usage >&2
  exit 1
fi

OUTPUT_DIR="$1"
OPENCLAUDE_REPO="$2"
ARCH="$3"

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUTPUT_DIR_ABS="$(mkdir -p "${OUTPUT_DIR}" && cd "${OUTPUT_DIR}" && pwd)"
STAGE_DIR="$(mktemp -d)"
ASSET_DIR="${STAGE_DIR}/assets/firecracker"
trap 'rm -rf "${STAGE_DIR}"' EXIT

mkdir -p "${ASSET_DIR}"
AIR_FIRECRACKER_ARCH="${ARCH}" "${ROOT_DIR}/scripts/fetch-firecracker-ubuntu-assets.sh" "${ASSET_DIR}"
GOARCH="${ARCH}" "${ROOT_DIR}/scripts/prepare-openclaude-ubuntu-rootfs.sh" \
  "${ASSET_DIR}/openclaude-ubuntu-rootfs.ext4" \
  "${OPENCLAUDE_REPO}" \
  "${ASSET_DIR}/ubuntu-rootfs.ext4"

rm -f "${ASSET_DIR}/ubuntu-rootfs.ext4"

ARCHIVE_NAME="air_openclaude_firecracker_linux_${ARCH}.tar.gz"
tar -C "${ASSET_DIR}" -czf "${OUTPUT_DIR_ABS}/${ARCHIVE_NAME}" .

echo "Built ${OUTPUT_DIR_ABS}/${ARCHIVE_NAME}"
