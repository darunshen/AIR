#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 7 ]]; then
  echo "usage: $0 <version> <arch> <air-bin> <air-agent-bin> <readme> <license> <output-deb>" >&2
  exit 1
fi

VERSION="$1"
ARCH="$2"
AIR_BIN="$3"
AIR_AGENT_BIN="$4"
README_FILE="$5"
LICENSE_FILE="$6"
OUTPUT_DEB="$7"

WORKDIR="$(mktemp -d)"
cleanup() {
  rm -rf "$WORKDIR"
}
trap cleanup EXIT

PKGROOT="$WORKDIR/pkg"
mkdir -p \
  "$PKGROOT/DEBIAN" \
  "$PKGROOT/usr/bin" \
  "$PKGROOT/usr/share/doc/air"

install -m 0755 "$AIR_BIN" "$PKGROOT/usr/bin/air"
install -m 0755 "$AIR_AGENT_BIN" "$PKGROOT/usr/bin/air-agent"
install -m 0644 "$README_FILE" "$PKGROOT/usr/share/doc/air/README.md"
install -m 0644 "$LICENSE_FILE" "$PKGROOT/usr/share/doc/air/LICENSE"

cat > "$PKGROOT/DEBIAN/control" <<EOF
Package: air
Version: ${VERSION}
Section: utils
Priority: optional
Architecture: ${ARCH}
Maintainer: AIR Maintainers <opensource@darunshen.dev>
Description: AIR agent isolation runtime
 AIR is an open-source runtime for executing untrusted AI-generated
 code inside isolated lightweight VMs.
EOF

dpkg-deb --build "$PKGROOT" "$OUTPUT_DEB" >/dev/null
