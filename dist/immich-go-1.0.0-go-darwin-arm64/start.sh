#!/bin/sh
export IMMICH_PORT="${IMMICH_PORT:-8081}"
exec "$(dirname "$0")/immich-go" "$@"
