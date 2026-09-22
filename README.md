# immich-go (patrick-gallery)

A single-binary, drop-in reimplementation of the **Immich** photo/video
server API, written in Go. It replaces the Node.js + Postgres + Redis +
machine-learning stack with one static binary backed by an embedded
SQLite database and a built-in web UI.

> Project alias: **patrick-gallery**.

## What it does

Implements the core Immich API surface used by the library experience:

- **Auth**: JWT + API keys, login / signup / validate / change-password / logout
- **Users**: list / me / update / preferences / thumb
- **Assets**: multipart upload (images + video metadata), 256px JPEG
  thumbnail generation for images, list / count / search / random /
  duplicates / original / thumbnail / **encoded-video** (in-process FFmpeg
  transcode to H.264/MP4 via purego — no CLI, no CGO)
- **Albums**: full CRUD + asset membership + cover
- **Libraries**: CRUD + statistics
- **Timeline**: year/month buckets + bucket assets
- **Search**: filename/EXIF text + metadata + explore + suggestions
- **Tags, Partners, Trash, Activities, Shared-links, System-config, Jobs**

Plus a small built-in **web gallery** (login, upload, thumbnail grid,
counts) served at `/`.

## What it intentionally does NOT do

These require the upstream ML stack or a clustered server and are out of
scope (stubbed or omitted):

- Facial recognition / auto person clustering, smart (semantic) search, CLIP,
  OCR — `/api/people` *list/get/merge/reassign* are real, but face detection
  (`/faces`) returns an honest `501` (no ML backend bundled).
- Hardware-accelerated / OS-native video backends (VideoToolbox, Media
  Foundation, MediaCodec) — the **software libx264 backend works in-process**;
  hardware paths are stubs that fall back to software.
- OAuth / SSO, email / external notifications, memories, workflows, plugins,
  horizontal multi-tenant scaling. (Admin user-management (`/admin/users/*`),
  on-disk library scan, offline reverse-geocoding, shared links, and real
  background jobs are now implemented — see [USER_MANUAL_ZH.md](USER_MANUAL_ZH.md).)

The official React web/mobile apps can still point at this server (set
their API base URL to it); the built-in UI at `/` is always available.

See [BACKEND_ALIGNMENT.md](BACKEND_ALIGNMENT.md) for the full mapping to the
upstream Immich backend and the exact compatibility contract.

## Build

```sh
go build -o immich-go .
# cross-compile (CGO_ENABLED=0, pure-Go SQLite)
CGO_ENABLED=0 GOOS=linux   GOARCH=arm64 go build -o immich-go-linux-arm64 .
CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -o immich-go.exe      .
```

## Run

```sh
./immich-go          # http://0.0.0.0:8081
```

Default admin: `admin@immich.app` / `password`. State lives in
`immich.db` (SQLite) and `resources/` (media). See `USER_MANUAL.md` for
all environment variables, reverse-proxy and systemd setup.

> In-process video (thumbnails/transcode) needs FFmpeg shared libs loaded by
> the purego backend. The **Docker image bundles them**, so video works there
> out of the box. On a bare Linux host, install `ffmpeg` *with* the unversioned
> `libav*`/`libsw*` symlinks (e.g. the `-dev` packages, or the Docker image) —
> otherwise the server logs `using backend: … placeholder` and serves the
> original video file without thumbnails/transcode. Windows/macOS packages:
> only the Windows/amd64 build bundles FFmpeg DLLs.

## Distribution / release

Prebuilt artifacts for Linux/macOS/Windows × x64/arm64 are in `dist/`
(built by `scripts/build-release.sh` with `CGO_ENABLED=0`):

- `immich-go-1.2.0-go-<os>-<arch>.tar.gz` — per-platform package
  (binary + `README.txt` + `start.sh`/`.bat`; Windows/amd64 also bundles
  FFmpeg 7.1 shared DLLs for out-of-the-box video).
- `checksums.txt` — SHA-256 of every binary

Verify: `sha256sum -c dist/checksums.txt`.

> Compatibility: the server speaks the Immich v3.1.0 contract and reports
> `IMMICH_COMPAT_VERSION` (default `3.1.0`). Point the official mobile/desktop
> app at it and set that env var to match your app's expected version if needed.

## Docker image & Compose

Published by the tag pipeline (push a tag → CI + contract gate → artifacts +
image) as a **multi-arch** manifest for `linux/amd64` and `linux/arm64`:

```
ghcr.io/idealisan/patrick-gallery:latest
ghcr.io/idealisan/patrick-gallery:<tag>      # e.g. v1.6.2-go
```

The image is a **runtime-only** bundle (`debian:trixie-slim`, whose `ffmpeg` is 7.1.x — the ABI `internal/video` is pinned to): it packages the
pre-compiled binary and runtime configuration — the Go build happens on the CI
runner (native cross-compile), so no toolchain ships in the image and no
emulated compile runs during the build. It installs `ffmpeg` and adds
unversioned `libav*`/`libsw*` symlinks next to the binary so the purego video
loader finds them; **in-container video thumbnails/transcode work out of the
box**. The binary is `CGO_ENABLED=0` but glibc-linked (modernc.org/sqlite), so
the Debian base is required — Alpine/musl would fail to exec it. The purego
video backend pins FFmpeg 7.x struct offsets: with a different major (bookworm
5.1, Ubuntu 6.1) it refuses the library at startup and falls back to the
placeholder backend (no thumbnails/transcode) rather than corrupting memory.

### Compose (recommended)

```sh
docker compose -f docker-compose.immich-go.yml up -d
# -> http://localhost:8081   (admin@immich.app / password on first start)
```

All state persists in `./immich-go-data` (SQLite `immich.db` + media under
`resources/`). Override any value via env vars, e.g.
`IMMICH_GO_PORT=9000 IMMICH_GO_DATA=/srv/immich docker compose -f docker-compose.immich-go.yml up -d`.
The file documents the full set (`IMMICH_TRASH_DAYS`, `IMMICH_PREVIEW_SIZE`,
`IMMICH_EXTERNAL_DOMAIN`, …).

### docker run

```sh
docker run -d --name immich-go -p 8081:8081   -v "$(pwd)/immich-go-data:/data"   ghcr.io/idealisan/patrick-gallery:latest
```

> The root `docker-compose.yml` is a **different** stack: it runs the *original*
> Immich (postgres/redis/immich-server) for side-by-side API comparison — see
> `docs/SIDE_BY_SIDE.md`. To run immich-go itself use
> `docker-compose.immich-go.yml`.

### Building the image yourself

The Dockerfile only packages the artifact (no toolchain), so stage the binary
for the target architecture first:

```sh
mkdir -p dockerctx/amd64
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w"   -o dockerctx/amd64/immich-go .
docker build --build-arg TARGETARCH=amd64 -t immich-go:local .
```

## Performance

Single static binary, ~3 MB idle RAM, ~1.5 s startup, hundreds of
req/s on modest hardware. See `PERFORMANCE_REPORT.md`.

## License

This is an independent reimplementation for learning/self-hosting; it is
not affiliated with the Immich project.
