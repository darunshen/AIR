#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  scripts/prepare-openclaude-firecracker-rootfs.sh [source_rootfs] [output_rootfs] [openclaude_repo] [bun_bin]

Description:
  Inject the host-side OpenClaude runtime into a Firecracker rootfs so the guest
  can start `bun run scripts/start-grpc.ts` from a fixed guest path.

Important:
  This script keeps the existing guest userspace baseline and is mainly useful
  when that rootfs is already Bun-compatible. The official Firecracker demo
  rootfs is too old for current Bun/OpenClaude builds. For a reproducible
  OpenClaude guest, prefer `scripts/prepare-openclaude-alpine-rootfs.sh`.

Default paths:
  source_rootfs: assets/firecracker/hello-rootfs-air.ext4
  output_rootfs: assets/firecracker/hello-rootfs-openclaude.ext4
  openclaude_repo: ~/Documents/code/openclaude
  bun_bin: command -v bun

Result:
  - /opt/openclaude
  - /usr/local/bin/bun
  - /usr/local/bin/openclaude-grpc

Recommended guest launch path:
  air agent openclaude start --provider firecracker --guest-repo /opt/openclaude
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

SOURCE_ROOTFS="${1:-assets/firecracker/hello-rootfs-air.ext4}"
OUTPUT_ROOTFS="${2:-assets/firecracker/hello-rootfs-openclaude.ext4}"
OPENCLAUDE_REPO="${3:-$HOME/Documents/code/openclaude}"
BUN_BIN="${4:-$(command -v bun || true)}"

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

require_cmd cp
require_cmd debugfs
require_cmd find
require_cmd install
require_cmd ln
require_cmd mkfs.ext4
require_cmd mktemp
require_cmd truncate
require_cmd du
require_cmd readelf
require_cmd wc

if [[ ! -f "${SOURCE_ROOTFS}" ]]; then
  echo "source rootfs not found: ${SOURCE_ROOTFS}" >&2
  exit 1
fi
if [[ ! -d "${OPENCLAUDE_REPO}" ]]; then
  echo "openclaude repo not found: ${OPENCLAUDE_REPO}" >&2
  exit 1
fi
if [[ ! -f "${OPENCLAUDE_REPO}/package.json" ]]; then
  echo "openclaude repo missing package.json: ${OPENCLAUDE_REPO}" >&2
  exit 1
fi
if [[ ! -d "${OPENCLAUDE_REPO}/node_modules" ]]; then
  echo "openclaude repo missing node_modules; run bun install first: ${OPENCLAUDE_REPO}" >&2
  exit 1
fi
if [[ -z "${BUN_BIN}" || ! -x "${BUN_BIN}" ]]; then
  echo "bun binary not found or not executable: ${BUN_BIN:-<empty>}" >&2
  exit 1
fi

bun_interpreter="$(readelf -l "${BUN_BIN}" 2>/dev/null | sed -n 's/.*Requesting program interpreter: \(.*\)\]/\1/p' | head -n1)"
rootfs_lib_listing="$(debugfs -R 'ls -p /lib' "${SOURCE_ROOTFS}" 2>/dev/null || true)"
if printf '%s\n' "${rootfs_lib_listing}" | grep -q 'ld-musl' && [[ "${bun_interpreter}" == *"ld-linux"* ]]; then
  cat >&2 <<EOF
bun binary is glibc-linked (${bun_interpreter}), but the target rootfs is musl-based.
Use a musl Bun binary instead, for example:
  https://github.com/oven-sh/bun/releases/latest/download/bun-linux-x64-musl.zip
Then rerun:
  $0 "${SOURCE_ROOTFS}" "${OUTPUT_ROOTFS}" "${OPENCLAUDE_REPO}" /path/to/musl/bun
EOF
  exit 1
fi

alpine_release="$(debugfs -R 'cat /etc/alpine-release' "${SOURCE_ROOTFS}" 2>/dev/null | tr -d '\r' | head -n1 || true)"
case "${alpine_release}" in
  3.8.*|3.9.*|3.10.*|3.11.*|3.12.*|3.13.*|3.14.*|3.15.*|3.16.*|3.17.*|3.18.*)
    cat >&2 <<EOF
warning:
  source rootfs appears to be Alpine ${alpine_release}, which is typically too old
  for current Bun/OpenClaude userspace requirements.
  Prefer:
    scripts/prepare-openclaude-alpine-rootfs.sh
EOF
    ;;
esac

mkdir -p "$(dirname "${OUTPUT_ROOTFS}")"
SOURCE_ROOTFS="$(cd "$(dirname "${SOURCE_ROOTFS}")" && pwd)/$(basename "${SOURCE_ROOTFS}")"
OUTPUT_ROOTFS="$(cd "$(dirname "${OUTPUT_ROOTFS}")" && pwd)/$(basename "${OUTPUT_ROOTFS}")"
OPENCLAUDE_REPO="$(cd "${OPENCLAUDE_REPO}" && pwd)"
BUN_BIN="$(cd "$(dirname "${BUN_BIN}")" && pwd)/$(basename "${BUN_BIN}")"

tmpdir="$(mktemp -d)"
cleanup() {
  rm -rf "${tmpdir}"
}
trap cleanup EXIT

stage_root="${tmpdir}/rootfs"
guest_repo_dir="${stage_root}/opt/openclaude"

cat >"${tmpdir}/openclaude-grpc" <<'EOF'
#!/bin/sh
set -eu
cd /opt/openclaude
exec /usr/local/bin/bun run scripts/start-grpc.ts
EOF
chmod 0755 "${tmpdir}/openclaude-grpc"

echo "Extracting source rootfs..."
mkdir -p "${stage_root}"
debugfs -R "rdump / ${stage_root}" "${SOURCE_ROOTFS}" >/dev/null 2>&1

echo "Injecting Bun and OpenClaude into staging rootfs..."
mkdir -p "${stage_root}/opt" "${stage_root}/usr/local/bin"
rm -rf "${guest_repo_dir}"
cp -a "${OPENCLAUDE_REPO}" "${guest_repo_dir}"
rm -rf "${guest_repo_dir}/.git" "${guest_repo_dir}/.air"
find "${guest_repo_dir}" -name '.DS_Store' -delete

install -D -m 0755 "${BUN_BIN}" "${stage_root}/usr/local/bin/bun"
install -D -m 0755 "${tmpdir}/openclaude-grpc" "${stage_root}/usr/local/bin/openclaude-grpc"

source_size="$(stat -c '%s' "${SOURCE_ROOTFS}")"
stage_size="$(du -sb "${stage_root}" | awk '{print $1}')"
inode_count="$(find "${stage_root}" | wc -l | awk '{print $1}')"
target_size=$((stage_size + 134217728))
if [[ "${target_size}" -lt "${source_size}" ]]; then
  target_size="${source_size}"
fi
align=$((64 * 1024 * 1024))
target_size=$((((target_size + align - 1) / align) * align))
inode_target=$((inode_count + inode_count / 2 + 16384))

echo "Rebuilding rootfs image at ${OUTPUT_ROOTFS}..."
rm -f "${OUTPUT_ROOTFS}"
truncate -s "${target_size}" "${OUTPUT_ROOTFS}"
mkfs.ext4 -q -F -N "${inode_target}" -d "${stage_root}" "${OUTPUT_ROOTFS}"

cat <<EOF

Prepared Firecracker rootfs with OpenClaude:
  Source rootfs: ${SOURCE_ROOTFS}
  Output rootfs: ${OUTPUT_ROOTFS}
  Estimated inode count: ${inode_target}
  Guest OpenClaude repo: /opt/openclaude
  Guest Bun binary: /usr/local/bin/bun
  Guest launcher: /usr/local/bin/openclaude-grpc

Recommended environment:
  export AIR_FIRECRACKER_ROOTFS=${OUTPUT_ROOTFS}

Recommended startup:
  air agent openclaude start --provider firecracker --guest-repo /opt/openclaude
EOF
