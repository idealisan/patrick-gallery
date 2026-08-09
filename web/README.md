# Official Immich Web Frontend Port

This directory documents the plan to replace the inline single-file SPA
(`internal/webroot/index.html`) with a build of the **official Immich web
application** so that `immich-go` ships the real UI instead of the minimal
gallery.

## Why

`internal/webroot/index.html` is a self-contained, no-build demo SPA. It is
good for getting started, but it lacks the full Immich experience (albums,
sharing, map, multi-select, etc.). The official web app is a Vite + React +
TypeScript project and is the canonical frontend for the Immich API.

## Plan

1. **Obtain the official web app source**

   ```bash
   git clone --depth 1 https://github.com/immich-app/immich.git immich-src
   ```

   (The full clone is multi-GB; `build.sh` uses a shallow clone of just the
   `web/` subfolder where possible. This is intentionally NOT run automatically
   in constrained environments — see `build.sh`.)

2. **Point the API base at this server**

   The Immich web app reads its server URL from a runtime config value
   (`IMMICH_API_URL` / the value returned by `GET /api/server/config`
   normally `/_app/immich-web-config`, or at build time via the
   `IMMICH_SERVER_URL` env var used by the build). Set it to the host/port
   where `immich-go` is listening, e.g. `http://localhost:3000`.

3. **Build**

   ```bash
   cd immich-src/web
   npm install
   npm run build        # outputs to immich-src/web/dist
   ```

4. **Embed the built assets with Go**

   Today `internal/webroot/webroot.go` does:

   ```go
   //go:embed index.html
   var indexFS embed.FS
   ```

   To ship the real UI, change the embed to the built `dist/` directory and
   keep `Index()` / `Serve()` intact, e.g.:

   ```go
   //go:embed all:dist
   var indexFS embed.FS
   func Index() ([]byte, error) { return indexFS.ReadFile("dist/index.html") }
   ```

   `Serve()` and `Index()` behaviour (always `200` for non-API routes) must be
   preserved so the binary still builds and SPA routing keeps working. The SPA
   must be served at `/` and the prerendered `index.html` returned for unknown
   client-side routes.

5. **Wire it up**

   `build.sh` produces `immich-src/web/dist`, which you copy into
   `internal/webroot/dist` (or adjust the embed path) before `go build`.

## Automation

`build.sh` performs steps 1–4. It is **guarded**: if `node`/`npm` are missing
it is a no-op, and the clone only happens when `IMMICH_WEB_DO_CLONE=1` is set,
so a multi-GB clone is never triggered by accident.
