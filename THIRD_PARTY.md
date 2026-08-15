# Third-party dependencies (runtime native libraries)

This file records **runtime native libraries** that `immich-go` loads *in
process* via `github.com/ebitengine/purego` (no CGO, no CLI). Go module
dependencies are tracked separately in `go.mod` / `go.sum`.

The release packaging step (`scripts/bundle-deps.sh`) downloads the pinned
artifacts below and bundles the shared libraries inside each
`immich-go-<os>-<arch>` package, so video works out of the box.

## Official Immich web UI (embedded SPA source)

The product web UI is the **official Immich web frontend**, built from the
immich monorepo and embedded into the Go binary via `//go:embed`
(`internal/webroot/webui`, produced by `scripts/build-web.sh`). See
`AGENTS.md` hard rule 6 — the hand-written SPA was replaced by this.

- **Pinned version: immich `v3.1.0`** (the web's `package.json` version and our
  `Config.CompatVersion` are aligned to this release).
- **Source (canonical, exact):** https://github.com/immich-app/immich — tag
  `v3.1.0` (web app at `web/`, API client at `packages/sdk`).
- **License:** **AGPL-3.0** (the web UI and `@immich/sdk` are AGPL-3.0). The
  embedded build output under `internal/webroot/webui/` is derived from this
  source; the AGPL-3.0 license/notices ship inside that build output.
- **Build:** SvelteKit + Vite, via pnpm (`pnpm install --filter "immich-web..."`,
  then `pnpm --filter @immich/sdk build` and `pnpm --filter immich-web build`).
  Output (`web/build/`) is copied into `internal/webroot/webui/`.

## FFmpeg (shared libraries)

Used by the software video backend (`internal/video`, purego-loaded). We load
`libavformat`, `libavcodec`, `libavutil`, `libswscale`, `libswresample`,
`libavfilter` from the shared build at runtime.

- **Pinned version: FFmpeg 7.1**
- **Source (canonical, exact):** https://ffmpeg.org/releases/ffmpeg-7.1.tar.xz
- **License:** LGPL 2.1+ (GPL build used for the Windows shared artifacts
  below — compatible with our usage; see license notes in each artifact).

### Per-platform shared builds (what `bundle-deps.sh` fetches)

| Platform | Arch | Source | Exact artifact pattern |
|----------|------|--------|------------------------|
| Windows  | amd64 | BtbN FFmpeg-Builds (GitHub) | `ffmpeg-n7.1-*-win64-gpl-shared.zip` |
| Windows  | arm64 | BtbN FFmpeg-Builds (GitHub) | `ffmpeg-n7.1-*-win-arm64-gpl-shared.zip` — **not published by BtbN** (verified via GitHub API: only `win64` gpl-shared exists); package falls back to placeholder video backend |
| macOS    | amd64 | Homebrew `ffmpeg` | `brew --prefix ffmpeg` → copy `libav*.dylib`, `libsw*.dylib` |
| macOS    | arm64 | Homebrew `ffmpeg` | same as above (Apple Silicon) |
| Linux    | amd64 | distro shared libs (Debian/Ubuntu `libavformat-dev` etc.) or `johnvansickle` is static (not usable) | copy `libav*.so*`, `libsw*.so*` from the system / pinned deb |
| Linux    | arm64 | same as Linux amd64 | same |

- BtbN releases index: https://github.com/BtbN/FFmpeg-Builds/releases
  (the script resolves the exact `ffmpeg-n7.1-*` asset via the GitHub API and
  extracts the `.dll` files into the Windows package).
- Homebrew formula: https://formulae.brew.sh/formula/ffmpeg
- Debian packages: https://packages.debian.org/stable/libavformat-dev
  (the script can `apt-get download` the `.deb` and extract `libav*.so*`).

> Note: FFmpeg official builds do not ship a single cross-platform shared
> archive. `bundle-deps.sh` is the single source of truth for *how* each
> platform's shared libs are obtained and where they land in the package.
> If a platform/arch has no shared build (e.g. Windows arm64), the script
> records it and the package falls back to the placeholder video backend
> (server still runs; video just has no thumbnail/transcode).

## purego (Go library, not a native lib)

- Module: `github.com/ebitengine/purego` (loaded at compile time, no native
  dependency). Enables calling the native FFmpeg/OS libraries from pure Go.
- Repo: https://github.com/ebitengine/purego
- License: BSD-3-Clause.

## GeoNames offline reverse-geocoding dataset (bundled runtime data)

Used by the offline reverse geocoder (`internal/app/geo`, embedded via
`go:embed`). It lets the server attach city / country names to GPS-tagged
assets with **no external service and no internet egress** (keeps immich-go
CGO-free and usable on a private LAN). This is *data*, not a native library,
but it is a runtime dependency the binary loads, so it is pinned here.

- **Source (exact):** GeoNames dumps
  - https://download.geonames.org/export/dump/cities15000.zip
    (processed into `internal/app/geo/cities.tsv`: name, lat, lon, country
    code, admin1 code — 34,073 rows as of the 2026-08-15 snapshot)
  - https://download.geonames.org/export/dump/countryInfo.txt
    (ISO 3166-1 alpha-2 → country name map; embedded as-is)
- **License:** GeoNames data is free for use with attribution under the
  [GeoNames licensing terms](https://www.geonames.org/). The dataset is
  trimmed/minimal (populated places only) and embedded read-only at build
  time; no code from GeoNames is linked.
- **Reproducibility:** the two files are committed under
  `internal/app/geo/`; `geo.NewGeocoder()` parses them once into a 1°×1°
  spatial grid. To refresh, re-download the two URLs and re-run the same
  column extraction used originally (name, lat, lon, country, admin1).
- **Fidelity note:** `state` returned by `/api/map/reverse-geocode` is the
  GeoNames admin1 *code* (e.g. Paris → "11"), not a spelled-out province
  name; the city and resolved country *name* are the primary fields used by
  the Map view.

## Already-tracked Go dependencies (for reference)

`go.mod` pins: `gin`, `gorm.io/gorm`, `github.com/glebarez/sqlite`
(pure-Go SQLite via `modernc.org/sqlite`), `golang-jwt/jwt/v5`,
`golang.org/x/crypto`. These are vendored through the Go module proxy and do
not need separate bundling.
