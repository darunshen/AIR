#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 3 ]]; then
  echo "usage: $0 <repo-dir> <version> <deb> [<deb>...]" >&2
  exit 1
fi

REPO_DIR="$1"
VERSION="$2"
shift 2
DEBS=("$@")

POOL_DIR="$REPO_DIR/pool/main/a/air"
mkdir -p "$POOL_DIR"

for deb in "${DEBS[@]}"; do
  cp "$deb" "$POOL_DIR/"
done

for arch in amd64 arm64; do
  BIN_DIR="$REPO_DIR/dists/stable/main/binary-${arch}"
  mkdir -p "$BIN_DIR"
  dpkg-scanpackages -m -a "$arch" "$POOL_DIR" > "$BIN_DIR/Packages"
  gzip -9 -c "$BIN_DIR/Packages" > "$BIN_DIR/Packages.gz"
done

RELEASE_FILE="$REPO_DIR/dists/stable/Release"
mkdir -p "$(dirname "$RELEASE_FILE")"
cat > "$RELEASE_FILE" <<EOF
Origin: AIR
Label: AIR
Suite: stable
Codename: stable
Version: ${VERSION}
Architectures: amd64 arm64
Components: main
Description: AIR apt repository
Date: $(LC_ALL=C date -Ru)
EOF

append_checksums() {
  local algo="$1"
  local cmd="$2"
  echo "${algo}:" >> "$RELEASE_FILE"
  while IFS= read -r -d '' file; do
    local rel
    rel="${file#"$REPO_DIR/"}"
    local size
    size="$(stat -c '%s' "$file")"
    local sum
    sum="$($cmd "$file" | awk '{print $1}')"
    printf " %s %16s %s\n" "$sum" "$size" "$rel" >> "$RELEASE_FILE"
  done < <(find "$REPO_DIR/dists/stable" -type f \( -name 'Packages' -o -name 'Packages.gz' \) -print0 | sort -z)
}

append_checksums "MD5Sum" "md5sum"
append_checksums "SHA256" "sha256sum"
