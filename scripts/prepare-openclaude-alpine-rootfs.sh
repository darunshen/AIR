#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  scripts/prepare-openclaude-alpine-rootfs.sh [output_rootfs] [openclaude_repo] [alpine_minirootfs_tarball] [bun_bin]

Description:
  Build a Firecracker guest rootfs for OpenClaude from a recent Alpine minirootfs
  instead of the older Firecracker demo rootfs. This is the recommended path for
  Bun/OpenClaude because the demo rootfs is too old for the current Bun runtime.

Default paths:
  output_rootfs: assets/firecracker/openclaude-alpine-rootfs.ext4
  openclaude_repo: ~/Documents/code/openclaude
  alpine_minirootfs_tarball: auto-download latest stable minirootfs for host arch
  bun_bin: auto-download latest musl Bun for host arch

Result:
  - /usr/bin/air-agent
  - /usr/local/bin/bun
  - /usr/local/bin/openclaude-grpc
  - /opt/openclaude
  - /etc/inittab boot hook for air-agent on ttyS0

Recommended runtime:
  export AIR_FIRECRACKER_ROOTFS=/absolute/path/to/openclaude-alpine-rootfs.ext4
  export AIR_FIRECRACKER_BOOT_ARGS="console=ttyS0 reboot=k panic=1 pci=off init=/sbin/init"
  air agent openclaude start --provider firecracker --guest-repo /opt/openclaude
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

OUTPUT_ROOTFS="${1:-assets/firecracker/openclaude-alpine-rootfs.ext4}"
OPENCLAUDE_REPO="${2:-$HOME/Documents/code/openclaude}"
ALPINE_MINIROOTFS="${3:-}"
BUN_BIN="${4:-}"
DEFAULT_PORT=10789

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

resolve_arch() {
  local arch="${GOARCH:-}"
  if [[ -z "${arch}" ]]; then
    arch="$(uname -m)"
  fi
  case "${arch}" in
    amd64|x86_64)
      echo "x86_64"
      ;;
    arm64|aarch64)
      echo "aarch64"
      ;;
    *)
      echo "unsupported architecture: ${arch}" >&2
      exit 1
      ;;
  esac
}

require_cmd awk
require_cmd cp
require_cmd curl
require_cmd find
require_cmd go
require_cmd install
require_cmd mkdir
require_cmd mkfs.ext4
require_cmd mktemp
require_cmd rm
require_cmd tar
require_cmd truncate
require_cmd unzip
require_cmd wc

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

mkdir -p "$(dirname "${OUTPUT_ROOTFS}")"
OUTPUT_ROOTFS="$(cd "$(dirname "${OUTPUT_ROOTFS}")" && pwd)/$(basename "${OUTPUT_ROOTFS}")"
OPENCLAUDE_REPO="$(cd "${OPENCLAUDE_REPO}" && pwd)"

tmpdir="$(mktemp -d)"
cleanup() {
  rm -rf "${tmpdir}"
}
trap cleanup EXIT

arch="$(resolve_arch)"
alpine_base_url="https://dl-cdn.alpinelinux.org/alpine/latest-stable/releases/${arch}"

if [[ -z "${ALPINE_MINIROOTFS}" ]]; then
  releases_yaml="${tmpdir}/latest-releases.yaml"
  echo "Downloading Alpine release metadata for ${arch}..."
  curl -fsSL "${alpine_base_url}/latest-releases.yaml" -o "${releases_yaml}"
  alpine_file="$(awk '
    $1 == "flavor:" && $2 == "alpine-minirootfs" { in_mini=1; next }
    in_mini && $1 == "file:" { print $2; exit }
  ' "${releases_yaml}")"
  if [[ -z "${alpine_file}" ]]; then
    echo "failed to resolve alpine minirootfs filename from ${releases_yaml}" >&2
    exit 1
  fi
  ALPINE_MINIROOTFS="${tmpdir}/${alpine_file}"
  echo "Downloading Alpine minirootfs ${alpine_file}..."
  curl -fsSL "${alpine_base_url}/${alpine_file}" -o "${ALPINE_MINIROOTFS}"
fi

if [[ ! -f "${ALPINE_MINIROOTFS}" ]]; then
  echo "alpine minirootfs not found: ${ALPINE_MINIROOTFS}" >&2
  exit 1
fi

if [[ -z "${BUN_BIN}" ]]; then
  case "${arch}" in
    x86_64)
      bun_asset="bun-linux-x64-musl.zip"
      ;;
    aarch64)
      bun_asset="bun-linux-aarch64-musl.zip"
      ;;
    *)
      echo "unsupported bun architecture: ${arch}" >&2
      exit 1
      ;;
  esac
  bun_zip="${tmpdir}/${bun_asset}"
  bun_extract_dir="${tmpdir}/bun"
  echo "Downloading Bun asset ${bun_asset}..."
  curl -fsSL "https://github.com/oven-sh/bun/releases/latest/download/${bun_asset}" -o "${bun_zip}"
  unzip -q "${bun_zip}" -d "${bun_extract_dir}"
  BUN_BIN="$(find "${bun_extract_dir}" -type f -name bun | head -n1)"
fi

if [[ -z "${BUN_BIN}" || ! -x "${BUN_BIN}" ]]; then
  echo "bun binary not found or not executable: ${BUN_BIN:-<empty>}" >&2
  exit 1
fi

stage_root="${tmpdir}/rootfs"
mkdir -p "${stage_root}"

: "${GOCACHE:=/tmp/go-build}"
: "${GOMODCACHE:=/tmp/gomodcache}"
export GOCACHE GOMODCACHE

echo "Building air-agent..."
CGO_ENABLED=0 GOOS=linux GOARCH="${GOARCH:-$(go env GOARCH)}" \
  go build -trimpath -o "${tmpdir}/air-agent" ./cmd/air-agent

cat >"${tmpdir}/air-init" <<EOF
#!/bin/sh
set -eu
mkdir -p /proc /sys /dev /run /tmp /var/log
mount -t proc proc /proc 2>/dev/null || true
mount -t sysfs sysfs /sys 2>/dev/null || true
mount -t devtmpfs devtmpfs /dev 2>/dev/null || true
echo "[air-agent] boot hook start" >>/air-agent.log
echo "[air-agent] boot hook start" >>/dev/console 2>&1 || true
/usr/bin/air-agent --network vsock --port ${DEFAULT_PORT} >>/air-agent.log 2>&1 &
echo \$! >/run/air-agent.pid
echo "[air-agent] launched pid=\$(cat /run/air-agent.pid)" >>/air-agent.log
echo "[air-agent] launched" >>/dev/console 2>&1 || true
EOF
chmod 0755 "${tmpdir}/air-init"

cat >"${tmpdir}/inittab" <<'EOF'
::sysinit:/usr/local/bin/air-init
ttyS0::respawn:/sbin/getty -L ttyS0 115200 vt100
::ctrlaltdel:/sbin/reboot
::shutdown:/bin/umount -a -r
EOF

cat >"${tmpdir}/openclaude-grpc" <<'EOF'
#!/bin/sh
set -eu
cd /opt/openclaude
exec /usr/local/bin/bun run scripts/start-grpc.ts
EOF
chmod 0755 "${tmpdir}/openclaude-grpc"

echo "Extracting Alpine minirootfs..."
tar -xzf "${ALPINE_MINIROOTFS}" -C "${stage_root}"

echo "Injecting AIR guest agent and OpenClaude runtime..."
mkdir -p "${stage_root}/opt" "${stage_root}/usr/local/bin" "${stage_root}/var/log" "${stage_root}/run"
cp -a "${OPENCLAUDE_REPO}" "${stage_root}/opt/openclaude"
rm -rf "${stage_root}/opt/openclaude/.git" "${stage_root}/opt/openclaude/.air"
find "${stage_root}/opt/openclaude" -name '.DS_Store' -delete

install -D -m 0755 "${tmpdir}/air-agent" "${stage_root}/usr/bin/air-agent"
install -D -m 0755 "${tmpdir}/air-init" "${stage_root}/usr/local/bin/air-init"
install -D -m 0755 "${BUN_BIN}" "${stage_root}/usr/local/bin/bun"
install -D -m 0755 "${tmpdir}/openclaude-grpc" "${stage_root}/usr/local/bin/openclaude-grpc"
install -D -m 0644 "${tmpdir}/inittab" "${stage_root}/etc/inittab"

inode_count="$(find "${stage_root}" | wc -l | awk '{print $1}')"
stage_size="$(du -sb "${stage_root}" | awk '{print $1}')"
target_size=$((stage_size + 268435456))
align=$((64 * 1024 * 1024))
target_size=$((((target_size + align - 1) / align) * align))
inode_target=$((inode_count + inode_count / 2 + 32768))

echo "Rebuilding ext4 rootfs at ${OUTPUT_ROOTFS}..."
rm -f "${OUTPUT_ROOTFS}"
truncate -s "${target_size}" "${OUTPUT_ROOTFS}"
mkfs.ext4 -q -F -N "${inode_target}" -d "${stage_root}" "${OUTPUT_ROOTFS}"

cat <<EOF

Prepared Alpine-based Firecracker rootfs with OpenClaude:
  Alpine minirootfs: ${ALPINE_MINIROOTFS}
  Output rootfs: ${OUTPUT_ROOTFS}
  Guest OpenClaude repo: /opt/openclaude
  Guest Bun binary: /usr/local/bin/bun
  Guest launcher: /usr/local/bin/openclaude-grpc
  Guest air-agent: /usr/bin/air-agent

Recommended environment:
  export AIR_FIRECRACKER_ROOTFS=${OUTPUT_ROOTFS}
  export AIR_FIRECRACKER_BOOT_ARGS="console=ttyS0 reboot=k panic=1 pci=off init=/sbin/init"

Recommended startup:
  air agent openclaude start --provider firecracker --guest-repo /opt/openclaude

Note:
  This image solves the Bun/OpenClaude userspace baseline problem.
  Guest outbound network access for remote LLM providers still depends on AIR's
  Firecracker networking or proxy strategy.
EOF
