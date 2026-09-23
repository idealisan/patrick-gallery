# Official Immich Web Frontend (embedded)

The product web UI is the **official Immich web frontend** (pinned release
`v3.1.0`), built from the immich monorepo and embedded into the Go binary —
per `AGENTS.md` hard rule 6. The hand-written vanilla SPA that previously
lived in `internal/webroot/assets/` / `internal/webroot/index.html` has been
**removed** and is not the product UI.

## Current state

- Embedded assets: `internal/webroot/webui/` (official build output),
  served by `internal/webroot/webroot.go` via `//go:embed all:webui`
  with SPA history-fallback to `index.html` for every non-`/api` route.
- Source / license / pinned tag: see `THIRD_PARTY.md`
  ("Official Immich web UI" section, AGPL-3.0, tag `v3.1.0`).
- Advertised server version (`Config.CompatVersion`, default `3.1.0`)
  matches the packaged web release so the official client accepts
  the connection.

## Rebuilding the web UI (canonical)

Use `scripts/build-web.sh` (pnpm-based, SvelteKit + Vite):

```bash
scripts/build-web.sh                 # uses IMMICH_WEB_TAG=v3.1.0 by default
go build ./...                       # re-embed internal/webroot/webui
```

## Legacy note

`web/build.sh` is a legacy npm-based variant kept for reference; it targets
`internal/webroot/webui` as well. Prefer `scripts/build-web.sh`, which is
the build referenced by `THIRD_PARTY.md`.
