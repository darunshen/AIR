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
DEFAULT_PROXY_LISTEN="127.0.0.1:18080"
DEFAULT_PROXY_VSOCK_PORT=18080
ALPINE_RUNTIME_PACKAGES="libgcc libstdc++"

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

resolve_alpine_branch() {
  local source_path="$1"
  local version
  version="$(basename "${source_path}" | sed -n 's/^alpine-minirootfs-\([0-9]\+\.[0-9]\+\)\.[0-9]\+-.*$/v\1/p')"
  if [[ -z "${version}" ]]; then
    echo "failed to resolve Alpine branch from ${source_path}" >&2
    exit 1
  fi
  echo "${version}"
}

resolve_apk_filename() {
  local index_file="$1"
  local package_name="$2"
  awk -v pkg="${package_name}" '
    BEGIN { RS=""; FS="\n" }
    {
      name=""
      version=""
      for (i = 1; i <= NF; i++) {
        if ($i ~ /^P:/) {
          name = substr($i, 3)
        }
        if ($i ~ /^V:/) {
          version = substr($i, 3)
        }
      }
      if (name == pkg && version != "") {
        print name "-" version ".apk"
        exit
      }
    }
  ' "${index_file}"
}

install_alpine_runtime_package() {
  local repo_url="$1"
  local index_file="$2"
  local package_name="$3"
  local apk_name
  local apk_path

  apk_name="$(resolve_apk_filename "${index_file}" "${package_name}")"
  if [[ -z "${apk_name}" ]]; then
    echo "failed to resolve package ${package_name} from ${index_file}" >&2
    exit 1
  fi

  apk_path="${tmpdir}/${apk_name}"
  echo "Downloading Alpine package ${apk_name}..."
  curl -fsSL "${repo_url}/${apk_name}" -o "${apk_path}"
  tar -xzf "${apk_path}" \
    --exclude='.PKGINFO' \
    --exclude='.SIGN.*' \
    --exclude='.INSTALL' \
    --exclude='.post-install' \
    -C "${stage_root}"
}

is_musl_bun() {
  local bun_path="$1"
  if [[ ! -x "${bun_path}" ]]; then
    return 1
  fi
  if ! readelf -l "${bun_path}" 2>/dev/null | grep -q "Requesting program interpreter:"; then
    return 0
  fi
  readelf -l "${bun_path}" 2>/dev/null | grep -q "/ld-musl-"
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
require_cmd readelf
require_cmd rm
require_cmd sed
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

alpine_branch="$(resolve_alpine_branch "${ALPINE_MINIROOTFS}")"
alpine_repo_url="https://dl-cdn.alpinelinux.org/alpine/${alpine_branch}/main/${arch}"
alpine_apk_index="${tmpdir}/APKINDEX"
echo "Downloading Alpine package index for ${alpine_branch}/${arch}..."
curl -fsSL "${alpine_repo_url}/APKINDEX.tar.gz" -o "${tmpdir}/APKINDEX.tar.gz"
tar -xOf "${tmpdir}/APKINDEX.tar.gz" APKINDEX > "${alpine_apk_index}"

if [[ -z "${BUN_BIN}" ]] && command -v bun >/dev/null 2>&1; then
  host_bun="$(command -v bun)"
  if is_musl_bun "${host_bun}"; then
    BUN_BIN="${host_bun}"
    echo "Using host musl Bun: ${BUN_BIN}"
  else
    echo "Host bun is not musl-compatible for Alpine guest, falling back to official musl release: ${host_bun}"
  fi
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
mount -t tmpfs tmpfs /run 2>/dev/null || true
mount -t tmpfs tmpfs /tmp 2>/dev/null || true
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
pid=\$!
echo "\${pid}" >/run/air-agent.pid
echo "[air-agent] launched pid=\${pid}" >>"\${LOG_FILE}"
echo "[air-agent] launched pid=\${pid}" >>/dev/console 2>&1 || true
sleep 1
if ! kill -0 "\${pid}" 2>/dev/null; then
  echo "[air-agent] died during startup" >>/dev/console 2>&1 || true
  cat "\${LOG_FILE}" >>/dev/console 2>&1 || true
fi
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
mkdir -p "${stage_root}/opt" "${stage_root}/usr/local/bin" "${stage_root}/var/log" "${stage_root}/run" "${stage_root}/mnt/workspace-ro" "${stage_root}/mnt/workspace-rw" "${stage_root}/workspace"
cp -a "${OPENCLAUDE_REPO}" "${stage_root}/opt/openclaude"
rm -rf "${stage_root}/opt/openclaude/.git" "${stage_root}/opt/openclaude/.air"
find "${stage_root}/opt/openclaude" -name '.DS_Store' -delete

for pkg in ${ALPINE_RUNTIME_PACKAGES}; do
  install_alpine_runtime_package "${alpine_repo_url}" "${alpine_apk_index}" "${pkg}"
done

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
