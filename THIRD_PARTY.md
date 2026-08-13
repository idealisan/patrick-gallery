# Third-party dependencies (runtime native libraries)

This file records **runtime native libraries** that `immich-go` loads *in
process* via `github.com/ebitengine/purego` (no CGO, no CLI). Go module
dependencies are tracked separately in `go.mod` / `go.sum`.

The release packaging step (`scripts/bundle-deps.sh`) downloads the pinned
artifacts below and bundles the shared libraries inside each
`immich-go-<os>-<arch>` package, so video works out of the box.

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
| Windows  | arm64 | BtbN FFmpeg-Builds (GitHub) | `ffmpeg-n7.1-*-win-arm64-gpl-shared.zip` (if published; otherwise unsupported — see note) |
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

## Already-tracked Go dependencies (for reference)

`go.mod` pins: `gin`, `gorm.io/gorm`, `github.com/glebarez/sqlite`
(pure-Go SQLite via `modernc.org/sqlite`), `golang-jwt/jwt/v5`,
`golang.org/x/crypto`. These are vendored through the Go module proxy and do
not need separate bundling.
