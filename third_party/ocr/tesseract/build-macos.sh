#!/bin/sh
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
OUT=${1:-"$ROOT/../../../dist/ocr/macos/libimmich_ocr_tesseract.dylib"}
mkdir -p "$(dirname "$OUT")"
xcrun clang -dynamiclib -mmacosx-version-min=12.0 \
  -I/opt/homebrew/opt/tesseract/include \
  -I/opt/homebrew/opt/leptonica/include \
  -L/opt/homebrew/opt/tesseract/lib \
  -L/opt/homebrew/opt/leptonica/lib \
  -ltesseract -lleptonica \
  -install_name @rpath/libimmich_ocr_tesseract.dylib \
  "$ROOT/immich_ocr_tesseract.c" -o "$OUT"
echo "$OUT"
