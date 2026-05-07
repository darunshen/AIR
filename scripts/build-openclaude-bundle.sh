#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  scripts/build-openclaude-bundle.sh <output_dir> <repo_dir> [<bun_bin>]

Description:
  Build a host-side OpenClaude runtime bundle for AIR release distribution.

Output:
  <output_dir>/air_openclaude_<os>_<arch>.tar.gz
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

if [[ $# -lt 2 || $# -gt 3 ]]; then
  usage >&2
  exit 1
fi

OUTPUT_DIR="$1"
OPENCLAUDE_REPO="$2"
BUN_BIN="${3:-$(command -v bun || true)}"

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

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUTPUT_DIR_ABS="$(mkdir -p "${OUTPUT_DIR}" && cd "${OUTPUT_DIR}" && pwd)"
STAGE_DIR="$(mktemp -d)"
trap 'rm -rf "${STAGE_DIR}"' EXIT

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "${ARCH}" in
  x86_64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "unsupported architecture: ${ARCH}" >&2; exit 1 ;;
esac

mkdir -p "${STAGE_DIR}/openclaude"
cp -a "${OPENCLAUDE_REPO}/." "${STAGE_DIR}/openclaude/"
rm -rf "${STAGE_DIR}/openclaude/.git" "${STAGE_DIR}/openclaude/.air"
find "${STAGE_DIR}/openclaude" -name '.DS_Store' -delete

mkdir -p "${STAGE_DIR}/bin"
install -m 0755 "${BUN_BIN}" "${STAGE_DIR}/bin/bun"

cat > "${STAGE_DIR}/AIR_OPENCLAUDE_BUNDLE.txt" <<EOF
repo_dir=openclaude
bun_path=bin/bun
grpc_command=bin/bun run scripts/start-grpc.ts
EOF

ARCHIVE_NAME="air_openclaude_${OS}_${ARCH}.tar.gz"
tar -C "${STAGE_DIR}" -czf "${OUTPUT_DIR_ABS}/${ARCHIVE_NAME}" .

echo "Built ${OUTPUT_DIR_ABS}/${ARCHIVE_NAME}"
