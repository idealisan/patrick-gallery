#!/usr/bin/env bash
# build-release.sh — cross-compile immich-go into the 6 release packages and
# produce checksums. Pure Go, CGO_ENABLED=0 (fully static). Run from repo root:
#   scripts/build-release.sh [version]
# Output lands in dist/ (which is intentionally committed per .gitignore).
set -euo pipefail

VERSION="${1:-1.2.0-go}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
OUT="$ROOT/dist"
mkdir -p "$OUT"

TARGETS="linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64"

for t in $TARGETS; do
  os="${t%/*}"; arch="${t#*/}"
  ext=""; [ "$os" = "windows" ] && ext=".exe"
  dir="$OUT/immich-go-$VERSION-$os-$arch"
  rm -rf "$dir"; mkdir -p "$dir"
  echo ">> building $os/$arch"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
    go build -trimpath -ldflags="-s -w" -o "$dir/immich-go$ext" .
  if [ "$os" = "windows" ]; then
    printf '@echo off\r\nimmich-go.exe\r\npause\r\n' > "$dir/start.bat"
  else
    printf '#!/usr/bin/env sh\n./immich-go\n' > "$dir/start.sh"
    chmod +x "$dir/start.sh"
  fi
  cat > "$dir/README.txt" <<EOF
immich-go $VERSION ($os/$arch)

Run:  ./immich-go
Serve: http://localhost:8081   (default admin: admin@immich.app / password)
Data:  immich.db + resources/  (override via IMMICH_DB / IMMICH_RESOURCE)
EOF
  ( cd "$OUT" && tar czf "immich-go-$VERSION-$os-$arch.tar.gz" "immich-go-$VERSION-$os-$arch" )
done

: > "$OUT/checksums.txt"
for t in $TARGETS; do
  os="${t%/*}"; arch="${t#*/}"; ext=""; [ "$os" = "windows" ] && ext=".exe"
  dir="$OUT/immich-go-$VERSION-$os-$arch"
  sha="$(sha256sum "$dir/immich-go$ext" | awk '{print $1}')"
  echo "$sha  $dir/immich-go$ext" >> "$OUT/checksums.txt"
done

echo "release $VERSION built in $OUT"
