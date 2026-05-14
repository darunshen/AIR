#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  scripts/prepare-openclaude-ubuntu-rootfs.sh [output_rootfs] [openclaude_repo] [ubuntu_rootfs_ext4] [bun_bin]

Description:
  Build a Firecracker guest rootfs for OpenClaude from the Ubuntu guest image
  used by Firecracker's official getting-started flow. This guest includes bash,
  ripgrep, curl, git, and CA certificates so OpenClaude's built-in tools can
  run inside the guest without requiring ad-hoc package installation.

Default paths:
  output_rootfs: assets/firecracker/openclaude-ubuntu-rootfs.ext4
  openclaude_repo: ~/Documents/code/openclaude
  ubuntu_rootfs_ext4: assets/firecracker/ubuntu-rootfs.ext4
  bun_bin: auto-download latest glibc Bun for host arch
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

OUTPUT_ROOTFS="${1:-assets/firecracker/openclaude-ubuntu-rootfs.ext4}"
OPENCLAUDE_REPO="${2:-$HOME/Documents/code/openclaude}"
UBUNTU_ROOTFS="${3:-assets/firecracker/ubuntu-rootfs.ext4}"
BUN_BIN="${4:-}"
DEFAULT_PORT=10789
DEFAULT_PROXY_LISTEN="127.0.0.1:18080"
DEFAULT_PROXY_VSOCK_PORT=18080
GUEST_HOST_BINARIES=(
  /usr/bin/bash
  /usr/bin/curl
  /usr/bin/git
  /usr/bin/node
  /usr/bin/rg
)
GUEST_HOST_PATHS=(
  /etc/bash.bashrc
  /etc/ca-certificates.conf
  /etc/ssl/certs
  /usr/lib/git-core
  /usr/share/ca-certificates
  /usr/share/git-core
)

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

is_glibc_bun() {
  local bun_path="$1"
  if [[ ! -x "${bun_path}" ]]; then
    return 1
  fi
  if ! readelf -l "${bun_path}" 2>/dev/null | grep -q "Requesting program interpreter:"; then
    return 1
  fi
  readelf -l "${bun_path}" 2>/dev/null | grep -q "/ld-linux-"
}

resolve_build_bun() {
  if command -v bun >/dev/null 2>&1; then
    command -v bun
    return 0
  fi
  if [[ -n "${BUN_BIN:-}" && -x "${BUN_BIN}" ]]; then
    printf '%s\n' "${BUN_BIN}"
    return 0
  fi
  echo "bun is required on the host to build OpenClaude dist/cli.mjs" >&2
  exit 1
}

prepare_openclaude_tree() {
  local source_repo="$1"
  local output_dir="$2"
  local build_bun="$3"

  echo "Preparing OpenClaude runtime tree..."
  cp -a "${source_repo}" "${output_dir}"
  rm -rf "${output_dir}/.git" "${output_dir}/.air"
  find "${output_dir}" -name '.DS_Store' -delete

  (
    cd "${output_dir}"
    "${build_bun}" run build
  )

  if [[ ! -f "${output_dir}/dist/cli.mjs" ]]; then
    echo "openclaude build did not produce dist/cli.mjs: ${output_dir}" >&2
    exit 1
  fi
}

require_cmd cp
require_cmd dpkg-deb
require_cmd debugfs
require_cmd find
require_cmd go
require_cmd install
require_cmd mkdir
require_cmd mkfs.ext4
require_cmd mktemp
require_cmd readelf
require_cmd readlink
require_cmd rm
require_cmd truncate
require_cmd unzip
require_cmd wc
require_cmd ldd

copy_path_into_rootfs() {
  local dest_root="$1"
  local path
  local rel
  local resolved

  path="$2"
  if [[ ! -e "${path}" && ! -L "${path}" ]]; then
    echo "required host path not found: ${path}" >&2
    return 1
  fi
  rel="${path#/}"
  mkdir -p "${dest_root}/$(dirname "${rel}")"
  (
    cd / && cp -a --parents "${rel}" "${dest_root}"
  )

  if [[ -L "${path}" ]]; then
    resolved="$(readlink -f "${path}")"
    if [[ -n "${resolved}" && "${resolved}" != "${path}" ]]; then
      copy_path_into_rootfs "${dest_root}" "${resolved}"
    fi
  fi
}

copy_binary_and_runtime_libs() {
  local dest_root="$1"
  local binary="$2"
  local lib

  copy_path_into_rootfs "${dest_root}" "${binary}"
  while IFS= read -r lib; do
    [[ -n "${lib}" ]] || continue
    copy_path_into_rootfs "${dest_root}" "${lib}"
  done < <(
    ldd "${binary}" 2>/dev/null | awk '
      /=> \// {print $3}
      /^[[:space:]]*\/[^[:space:]]+/ {print $1}
    ' | sort -u
  )
}

inject_guest_host_tooling() {
  local dest_root="$1"
  local path

  echo "Injecting guest host tooling..."
  for path in "${GUEST_HOST_BINARIES[@]}"; do
    copy_binary_and_runtime_libs "${dest_root}" "${path}"
  done
  for path in "${GUEST_HOST_PATHS[@]}"; do
    copy_path_into_rootfs "${dest_root}" "${path}"
  done

  if [[ -e "${dest_root}/usr/bin/bash" && ! -e "${dest_root}/bin/bash" ]]; then
    mkdir -p "${dest_root}/bin"
    ln -sf ../usr/bin/bash "${dest_root}/bin/bash"
  fi
}

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
if [[ ! -f "${UBUNTU_ROOTFS}" ]]; then
  echo "ubuntu rootfs not found: ${UBUNTU_ROOTFS}" >&2
  echo "hint: run scripts/fetch-firecracker-ubuntu-assets.sh first" >&2
  exit 1
fi

mkdir -p "$(dirname "${OUTPUT_ROOTFS}")"
OUTPUT_ROOTFS="$(cd "$(dirname "${OUTPUT_ROOTFS}")" && pwd)/$(basename "${OUTPUT_ROOTFS}")"
OPENCLAUDE_REPO="$(cd "${OPENCLAUDE_REPO}" && pwd)"
UBUNTU_ROOTFS="$(cd "$(dirname "${UBUNTU_ROOTFS}")" && pwd)/$(basename "${UBUNTU_ROOTFS}")"

tmpdir="$(mktemp -d)"
cleanup() {
  rm -rf "${tmpdir}"
}
trap cleanup EXIT

arch="$(resolve_arch)"

if [[ -z "${BUN_BIN}" ]] && command -v bun >/dev/null 2>&1; then
  host_bun="$(command -v bun)"
  if is_glibc_bun "${host_bun}"; then
    BUN_BIN="${host_bun}"
    echo "Using host glibc Bun: ${BUN_BIN}"
  else
    echo "Host bun is not glibc-compatible for Ubuntu guest, falling back to official glibc release: ${host_bun}"
  fi
fi

if [[ -z "${BUN_BIN}" ]]; then
  case "${arch}" in
    x86_64)
      bun_asset="bun-linux-x64.zip"
      ;;
    aarch64)
      bun_asset="bun-linux-aarch64.zip"
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

BUILD_BUN="$(resolve_build_bun)"
PREPARED_OPENCLAUDE="${tmpdir}/openclaude"
prepare_openclaude_tree "${OPENCLAUDE_REPO}" "${PREPARED_OPENCLAUDE}" "${BUILD_BUN}"

stage_root="${tmpdir}/rootfs"
mkdir -p "${stage_root}"

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
  code=\$?
  echo "[air-agent] exited code=\${code}" >>"\${LOG_FILE}"
  echo "[air-agent] exited code=\${code}" >>/dev/console 2>&1 || true
) &
echo \$! >/run/air-agent.pid
if command -v getty >/dev/null 2>&1; then
  getty -L ttyS0 115200 vt100 >/dev/ttyS0 2>&1 &
fi
while true; do
  wait || true
  sleep 1
done
EOF
chmod 0755 "${tmpdir}/init"

cat >"${tmpdir}/openclaude-grpc" <<'EOF'
#!/bin/sh
set -eu
cd /opt/openclaude
exec /usr/local/bin/bun run scripts/start-grpc.ts
EOF
chmod 0755 "${tmpdir}/openclaude-grpc"

echo "Extracting Ubuntu rootfs..."
debugfs -R "rdump / ${stage_root}" "${UBUNTU_ROOTFS}" >/dev/null 2>&1

inject_guest_host_tooling "${stage_root}"

echo "Injecting AIR guest agent and OpenClaude runtime..."
mkdir -p "${stage_root}/opt" "${stage_root}/usr/local/bin" "${stage_root}/var/log" "${stage_root}/run" "${stage_root}/mnt/workspace-ro" "${stage_root}/mnt/workspace-rw" "${stage_root}/workspace"
cp -a "${PREPARED_OPENCLAUDE}" "${stage_root}/opt/openclaude"
rm -rf "${stage_root}/opt/openclaude/.git" "${stage_root}/opt/openclaude/.air"
find "${stage_root}/opt/openclaude" -name '.DS_Store' -delete

guest_grpc_server="${stage_root}/opt/openclaude/src/grpc/server.ts"
if [[ -f "${guest_grpc_server}" && "${AIR_DEBUG_OPENCLAUDE:-0}" == "1" ]]; then
  perl -0pi -e "s/call\\.on\\('data', async \\(clientMessage\\) => \\{/call.on('data', async (clientMessage) => {\\n      console.error('[air-debug] grpc data keys=' + Object.keys(clientMessage).join(','))\\n      if (clientMessage.request) {\\n        console.error('[air-debug] grpc request session=' + (clientMessage.request.session_id || '') + ' cwd=' + (clientMessage.request.working_directory || '') + ' msg_len=' + String((clientMessage.request.message || '').length))\\n      }/s" "${guest_grpc_server}"
  perl -0pi -e "s/const generator = engine\\.submitMessage\\(req\\.message\\)/console.error('[air-debug] queryengine submit start')\\n          const generator = engine.submitMessage(req.message)/s" "${guest_grpc_server}"
  perl -0pi -e "s/for await \\(const msg of generator\\) \\{/for await (const msg of generator) {\\n            console.error('[air-debug] queryengine msg type=' + msg.type + ((msg.type === 'result' \\&\\& 'subtype' in msg) ? ' subtype=' + String(msg.subtype) : ''))/s" "${guest_grpc_server}"
  perl -0pi -e "s/call\\.write\\(\\{\\n              done: \\{/console.error('[air-debug] grpc done full_text_len=' + String(fullText.length) + ' prompt_tokens=' + String(promptTokens) + ' completion_tokens=' + String(completionTokens))\\n            call.write({\\n              done: {/s" "${guest_grpc_server}"
  perl -0pi -e "s/console\\.error\\('Error processing stream'\\)/console.error('[air-debug] error processing stream', err)\\n        console.error('Error processing stream')/s" "${guest_grpc_server}"
fi

install -D -m 0755 "${tmpdir}/air-agent" "${stage_root}/usr/bin/air-agent"
if [[ -e "${stage_root}/sbin/init" && ! -e "${stage_root}/sbin/init.air-orig" ]]; then
  cp -a "${stage_root}/sbin/init" "${stage_root}/sbin/init.air-orig"
fi
install -D -m 0755 "${tmpdir}/init" "${stage_root}/sbin/init"
install -D -m 0755 "${BUN_BIN}" "${stage_root}/usr/local/bin/bun"
install -D -m 0755 "${tmpdir}/openclaude-grpc" "${stage_root}/usr/local/bin/openclaude-grpc"

inode_count="$(find "${stage_root}" | wc -l | awk '{print $1}')"
stage_size="$(du -sb "${stage_root}" | awk '{print $1}')"
target_size=$((stage_size + 536870912))
align=$((64 * 1024 * 1024))
target_size=$((((target_size + align - 1) / align) * align))
if [[ "${target_size}" -lt 1073741824 ]]; then
  target_size=1073741824
fi
inode_target=$((inode_count + inode_count / 2 + 32768))

echo "Rebuilding ext4 rootfs at ${OUTPUT_ROOTFS}..."
rm -f "${OUTPUT_ROOTFS}"
truncate -s "${target_size}" "${OUTPUT_ROOTFS}"
mkfs.ext4 -q -F -N "${inode_target}" -d "${stage_root}" "${OUTPUT_ROOTFS}"

cat <<EOF

Prepared Ubuntu-based Firecracker rootfs with OpenClaude:
  Base Ubuntu rootfs: ${UBUNTU_ROOTFS}
  Output rootfs: ${OUTPUT_ROOTFS}
  Guest OpenClaude repo: /opt/openclaude
  Guest Bun binary: /usr/local/bin/bun
  Guest launcher: /usr/local/bin/openclaude-grpc
  Guest air-agent: /usr/bin/air-agent
  Guest extra tooling: ${GUEST_HOST_BINARIES[*]}

Recommended environment:
  export AIR_FIRECRACKER_ROOTFS=${OUTPUT_ROOTFS}
  export AIR_FIRECRACKER_BOOT_ARGS="console=ttyS0 reboot=k panic=1 pci=off init=/sbin/init"
EOF
