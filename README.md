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

- Facial recognition / `people` recall, smart (semantic) search, CLIP
- Hardware-accelerated / OS-native video backends (VideoToolbox, Media
  Foundation, MediaCodec) — the **software libx264 backend works in-process**;
  hardware paths are stubs that fall back to software.
- OAuth / SSO, admin & maintenance dashboards, reverse-geocoding (markers
  exist; city/country come from EXIF), stacks, memories, workflows, plugins,
  sync streaming, notifications

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

> Compatibility: the server speaks the latest Immich contract and reports
> `IMMICH_COMPAT_VERSION` (default `1.130.0`). Point the official mobile/desktop
> app at it and set that env var to match your app's expected version if needed.

## Docker image (CNB Container Registry)

The image is built from the repo `Dockerfile` (`golang:1.23` multi-stage,
`CGO_ENABLED=0` → no-cgo build; the resulting binary is glibc-linked via the
modernc.org/sqlite stack, so the runtime base is **Debian bookworm (glibc)**
— Alpine/musl would fail to exec it). The image installs `ffmpeg` and adds
unversioned `libav*`/`libsw*` symlinks next to the binary so the purego video
loader finds them; **in-container video thumbnails/transcode work out of the
box** (verified: server logs `using backend: ffmpeg-software`).
It is published to the CNB registry:

```
registry.cnb.cool/finalappstore/immich-go:latest
registry.cnb.cool/finalappstore/immich-go:v1.2.0-go
```

```sh
docker run -d --name immich-go \
  -p 8081:8081 \
  -v "$(pwd)/data:/data" \
  registry.cnb.cool/finalappstore/immich-go:latest
# -> http://localhost:8081  (admin@immich.app / password)
```

Currently built for `linux/amd64` (multi-arch `arm64` needs `docker buildx` +
QEMU). Mount `/data` to persist the SQLite DB (`immich.db`) and media
(`resources/`). The `Dockerfile` and `.dockerignore` live at the repo root.

## Performance

Single static binary, ~3 MB idle RAM, ~1.5 s startup, hundreds of
req/s on modest hardware. See `PERFORMANCE_REPORT.md`.

## License

This is an independent reimplementation for learning/self-hosting; it is
not affiliated with the Immich project.
