#!/usr/bin/env bash
# bundle-deps.sh — download pinned FFmpeg 7.1 shared libraries and package
# them next to each immich-go binary so video works out of the box (no user
# install). The Go binary loads these via purego (internal/video/ffmpeg.go)
# by searching <exe dir> and <exe dir>/libs.
#
# For every built binary found in DIR (named immich-go-<os>-<arch>[.exe], or
# immich-go[.exe]) AND every existing immich-go-*-go-* package directory, this
# script produces a sibling archive `<name>-deps.tar.gz` (linux/darwin) or
# `<name>-deps.zip` (windows) containing: the binary, the FFmpeg shared libs
# under libs/, a README.txt and a start script. The release workflow's
# `files: immich-go-*` glob then uploads these archives automatically.
#
# Usage: scripts/bundle-deps.sh [dir]   (default: current dir)
set -uo pipefail

FFVER="${FFVER:-7.1}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIR="${1:-.}"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

echo "[bundle-deps] FFmpeg version: $FFVER"
echo "[bundle-deps] scanning: $DIR"

# ---- Windows: BtbN shared builds (contains .dll files) ----
fetch_btbn() {
  local arch="$1" libsdir="$2"
  mkdir -p "$libsdir"
  local winarch="$arch" pattern
  case "$arch" in
    amd64) winarch=win64 ;;
    arm64) winarch=win-arm64 ;;
    386)   winarch=win32 ;;
  esac
  local asset_pattern="ffmpeg-n${FFVER}-[^\"]*-${winarch}-gpl-shared[^\"]*\.(zip|7z)"
  echo "[bundle-deps] windows/${arch}: BtbN asset '$asset_pattern'"
  local url
  url="$(curl -sL "https://api.github.com/repos/BtbN/FFmpeg-Builds/releases/tags/${BTBN_TAG:-latest}" \
    | grep -oE "https://github.com/BtbN/FFmpeg-Builds/releases/download/${BTBN_TAG:-latest}/${asset_pattern}" \
    | head -n1)"
  if [ -z "$url" ]; then
    echo "[bundle-deps] WARNING: no BtbN asset for windows/${arch}; package falls back to placeholder video backend" >&2
    return 0
  fi
  local zip="$WORK/btbn-${arch}.zip"
  echo "[bundle-deps]   -> $url"
  curl -sL -o "$zip" "$url" || { echo "[bundle-deps] download failed" >&2; return 0; }
  ( cd "$WORK" && rm -rf "btbn-${arch}" && mkdir -p "btbn-${arch}" && unzip -o -q "$zip" -d "btbn-${arch}" )
  find "$WORK/btbn-${arch}" -name '*.dll' -exec cp -t "$libsdir" {} + 2>/dev/null || true
  echo "[bundle-deps]   copied $(ls -1 "$libsdir" 2>/dev/null | wc -l) dlls"
}

# ---- Linux: distro shared libs (.so) ----
fetch_linux() {
  local arch="$1" libsdir="$2"
  mkdir -p "$libsdir"
  if command -v apt-get >/dev/null 2>&1; then
    local tmpd="$WORK/apt-${arch}"; mkdir -p "$tmpd"
    ( cd "$tmpd" && apt-get download libavformat-dev libavcodec-dev libavutil-dev libswscale-dev libswresample-dev libavfilter-dev 2>/dev/null ) || true
    for d in "$tmpd"/*.deb; do [ -e "$d" ] || continue; dpkg-deb -x "$d" "$tmpd/extract" 2>/dev/null || true; done
    find "$tmpd/extract" \( -name 'libav*.so*' -o -name 'libsw*.so*' \) 2>/dev/null -exec cp -t "$libsdir" {} + 2>/dev/null || true
  fi
  if [ -z "$(ls -A "$libsdir" 2>/dev/null)" ]; then
    echo "[bundle-deps] WARNING: linux/${arch}: no shared libs obtained (install ffmpeg-dev or set them manually)" >&2
  else
    echo "[bundle-deps]   copied $(ls -1 "$libsdir" | wc -l) sos"
  fi
}

# ---- macOS: Homebrew dylibs ----
fetch_darwin() {
  local arch="$1" libsdir="$2"
  mkdir -p "$libsdir"
  if command -v brew >/dev/null 2>&1; then
    local prefix; prefix="$(brew --prefix ffmpeg 2>/dev/null || true)"
    if [ -n "$prefix" ]; then
      find "$prefix/lib" \( -name 'libav*.dylib' -o -name 'libsw*.dylib' \) 2>/dev/null -exec cp -t "$libsdir" {} + 2>/dev/null || true
      echo "[bundle-deps]   copied $(ls -1 "$libsdir" | wc -l) dylibs from $prefix"
      return
    fi
  fi
  echo "[bundle-deps] WARNING: darwin/${arch}: Homebrew ffmpeg not found; package falls back to placeholder video backend" >&2
}

# Assemble a -deps archive for one binary.
package_binary() {
  local bin="$1" os="$2" arch="$3"
  local base; base="$(basename "$bin")"; base="${base%.exe}"
  local pkg="$WORK/pkg-$base"; rm -rf "$pkg"; mkdir -p "$pkg/libs"
  cp "$bin" "$pkg/${base##*-}" 2>/dev/null || cp "$bin" "$pkg/immich-go"  # place as immich-go[.exe]
  case "$os" in
    windows) fetch_btbn "$arch" "$pkg/libs" ;;
    linux)   fetch_linux "$arch" "$pkg/libs" ;;
    darwin)  fetch_darwin "$arch" "$pkg/libs" ;;
  esac
  cat > "$pkg/README.txt" <<EOF
immich-go ($base) — release package (FFmpeg 7.1 shared libs bundled in libs/)

Run ./immich-go (or immich-go.exe) — video thumbnail/transcode work out of the box.
If libs/ is empty on this platform, the server still runs and uses a placeholder
video backend (video uploads/plays the original file).
EOF
  if [ "$os" = "windows" ]; then
    printf '@echo off\r\n"%%~dp0immich-go" %%*\r\n' > "$pkg/start.bat"
    ( cd "$pkg" && zip -q -r "$DIR/${base}-deps.zip" . )
    echo "[bundle-deps] wrote $DIR/${base}-deps.zip"
  else
    printf '#!/bin/sh\nexec "$(dirname "$0")/immich-go" "$@"\n' > "$pkg/start.sh"
    chmod +x "$pkg/start.sh" "$pkg/immich-go" 2>/dev/null
    ( cd "$pkg" && tar -czf "$DIR/${base}-deps.tar.gz" . )
    echo "[bundle-deps] wrote $DIR/${base}-deps.tar.gz"
  fi
}

# Collect bare binaries: immich-go-<os>-<arch>[.exe] (skip dirs and archives)
for bin in "$DIR"/immich-go-*; do
  [ -f "$bin" ] || continue
  case "$bin" in *.tar.gz|*.zip) continue;; esac
  base="$(basename "$bin")"; base="${base%.exe}"
  name="${base#immich-go-}"            # e.g. linux-arm64
  os="${name%-*}"; arch="${name##*-}"
  package_binary "$bin" "$os" "$arch"
done

# Also handle committed package directories immich-go-*-go-<os>-<arch>
for pkgdir in "$DIR"/immich-go-*-go-*; do
  [ -d "$pkgdir" ] || continue
  base="$(basename "$pkgdir")"          # immich-go-1.0.0-go-linux-arm64
  name="${base#immich-go-*-go-}"        # linux-arm64
  os="${name%-*}"; arch="${name##*-}"
  # put libs inside the existing package dir (idempotent)
  case "$os" in
    windows) fetch_btbn "$arch" "$pkgdir/libs" ;;
    linux)   fetch_linux "$arch" "$pkgdir/libs" ;;
    darwin)  fetch_darwin "$arch" "$pkgdir/libs" ;;
  esac
  echo "[bundle-deps] bundled libs into existing package $base"
done

echo "[bundle-deps] done."
