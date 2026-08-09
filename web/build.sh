#!/usr/bin/env bash
#
# Build the official Immich web app and stage it for embedding by immich-go.
#
# Guards:
#   - No-ops if `node` or `npm` are not available.
#   - Never performs the (multi-GB) clone unless IMMICH_WEB_DO_CLONE=1 is set.
#
# When run successfully it produces: ../internal/webroot/dist  (the built SPA)
# which webroot.go's embed directive can then pick up.
#
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SRC="$ROOT/web/immich-src"
WEB="$SRC/web"
OUT="$ROOT/internal/webroot/dist"
API_URL="${IMMICH_API_URL:-http://localhost:3000}"

need() { command -v "$1" >/dev/null 2>&1 || { echo "skip: '$1' not found"; exit 0; }; }

need node
need npm

echo "node: $(node --version)  npm: $(npm --version)"

if [ "${IMMICH_WEB_DO_CLONE:-0}" != "1" ]; then
  echo "skip: set IMMICH_WEB_DO_CLONE=1 to clone + build the official Immich web app."
  echo "      (clone is multi-GB and is intentionally not run automatically.)"
  exit 0
fi

echo "cloning immich (shallow)..."
rm -rf "$SRC"
git clone --depth 1 --filter=blob:none --sparse \
  https://github.com/immich-app/immich.git "$SRC"
git -C "$SRC" sparse-checkout set web

echo "configuring API base -> $API_URL"
echo "$API_URL" > "$WEB/public/immich-web-config"

cd "$WEB"
npm install
IMMICH_SERVER_URL="$API_URL" npm run build

echo "staging build into $OUT"
rm -rf "$OUT"
mkdir -p "$(dirname "$OUT")"
cp -r dist "$OUT"

echo "done. now rebuild immich-go (webroot.go must embed ./dist)."
