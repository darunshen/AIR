#!/usr/bin/env bash
set -euo pipefail

OUT_DIR="${1:-dist}"
VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
COMMIT="${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo unknown)}"
BUILD_DATE="${BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
VERSION_CLEAN="${VERSION#v}"

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR_ABS="$ROOT_DIR/$OUT_DIR"
rm -rf "$OUT_DIR_ABS"
mkdir -p "$OUT_DIR_ABS"

LDFLAGS="-s -w -X github.com/darunshen/AIR/internal/buildinfo.Version=${VERSION} -X github.com/darunshen/AIR/internal/buildinfo.Commit=${COMMIT} -X github.com/darunshen/AIR/internal/buildinfo.Date=${BUILD_DATE}"

build_target() {
  local goos="$1"
  local goarch="$2"
  local ext=""
  if [[ "$goos" == "windows" ]]; then
    ext=".exe"
  fi

  local stage="$OUT_DIR_ABS/stage/${goos}_${goarch}"
  mkdir -p "$stage"

  GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 go build -trimpath -ldflags="$LDFLAGS" -o "$stage/air${ext}" ./cmd/air
  GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 go build -trimpath -ldflags="$LDFLAGS" -o "$stage/air-agent${ext}" ./cmd/air-agent
  cp "$ROOT_DIR/README.md" "$ROOT_DIR/LICENSE" "$stage/"

  local base="air_${VERSION}_${goos}_${goarch}"
  if [[ "$goos" == "windows" ]]; then
    (cd "$stage" && zip -q -r "$OUT_DIR_ABS/${base}.zip" .)
  else
    tar -C "$stage" -czf "$OUT_DIR_ABS/${base}.tar.gz" .
  fi
}

for target in \
  "linux amd64" \
  "linux arm64" \
  "darwin amd64" \
  "darwin arm64" \
  "windows amd64"
do
  build_target ${target}
done

declare -a debs=()
for arch in amd64 arm64; do
  stage="$OUT_DIR_ABS/stage/linux_${arch}"
  deb="$OUT_DIR_ABS/air_${VERSION_CLEAN}_${arch}.deb"
  "$ROOT_DIR/scripts/build-deb.sh" \
    "$VERSION_CLEAN" \
    "$arch" \
    "$stage/air" \
    "$stage/air-agent" \
    "$ROOT_DIR/README.md" \
    "$ROOT_DIR/LICENSE" \
    "$deb"
  debs+=("$deb")
done

"$ROOT_DIR/scripts/build-apt-repo.sh" "$OUT_DIR_ABS/apt-repo" "$VERSION_CLEAN" "${debs[@]}"
tar -C "$OUT_DIR_ABS" -czf "$OUT_DIR_ABS/air_${VERSION}_apt-repo.tar.gz" apt-repo

find "$OUT_DIR_ABS" -maxdepth 1 -type f ! -name 'checksums.txt' -print0 | sort -z | xargs -0 sha256sum > "$OUT_DIR_ABS/checksums.txt"

cat > "$OUT_DIR_ABS/BUILD_INFO.txt" <<EOF
version=${VERSION}
commit=${COMMIT}
date=${BUILD_DATE}
EOF
