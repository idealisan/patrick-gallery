#!/bin/sh
# Builds the OFFICIAL Immich web UI (pinned release) and copies the static
# output into internal/webroot/webui so the Go binary can embed it.
#
# This is the product UI (see AGENTS.md hard rule 6). The hand-written SPA
# that previously lived in internal/webroot/assets has been replaced.
#
# Usage:
#   scripts/build-web.sh                 # uses defaults
#   IMMICH_WEB_TAG=v3.1.0 scripts/build-web.sh
set -eu

IMMICH_WEB_TAG="${IMMICH_WEB_TAG:-v3.1.0}"
IMMICH_WEB_SRC="${IMMICH_WEB_SRC:-/root/immich-src}"
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT_DIR="$REPO_ROOT/internal/webroot/webui"

echo "==> Building official Immich web @ $IMMICH_WEB_TAG"

if [ ! -d "$IMMICH_WEB_SRC/web" ]; then
  echo "==> Cloning immich $IMMICH_WEB_TAG into $IMMICH_WEB_SRC"
  rm -rf "$IMMICH_WEB_SRC"
  git clone --depth 1 --branch "$IMMICH_WEB_TAG" https://github.com/immich-app/immich.git "$IMMICH_WEB_SRC"
fi

cd "$IMMICH_WEB_SRC"
git checkout "$IMMICH_WEB_TAG" 2>/dev/null || true

# pnpm is required (immich monorepo uses pnpm workspaces).
export COREPACK_ENABLE_DOWNLOAD_PROMPT=0
corepack enable >/dev/null 2>&1 || true
corepack prepare pnpm@11.13.1 --activate >/dev/null 2>&1 || true

echo "==> Installing web + sdk dependency subgraph"
pnpm install --filter "immich-web..."

echo "==> Building @immich/sdk (tsc)"
pnpm --filter @immich/sdk build

echo "==> Building immich-web (vite -> static)"
pnpm --filter immich-web build

echo "==> Copying build output into $OUT_DIR"
rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR"
cp -r web/build/. "$OUT_DIR/"

echo "==> Done. The Go binary will now embed the official Immich web UI."
echo "    Rebuild with: go build -o dist/immich-go-<ver>-linux-amd64/immich-go ."
