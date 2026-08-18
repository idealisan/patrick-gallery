#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
OUT=${1:-"$ROOT/../../../dist/ocr/macos/libimmich_ocr_macos.dylib"}
mkdir -p "$(dirname "$OUT")"
xcrun clang -dynamiclib -fobjc-arc -mmacosx-version-min=12.0 \
  -framework Foundation -framework Vision -framework ImageIO -framework CoreGraphics \
  -install_name @rpath/libimmich_ocr_macos.dylib \
  "$ROOT/immich_ocr_macos.m" -o "$OUT"
echo "$OUT"
