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
  duplicates / original / thumbnail / encoded-video
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
- Video transcoding (the original file is served instead)
- OAuth / SSO, admin & maintenance dashboards, map / reverse-geocoding,
  stacks, memories, workflows, plugins, sync streaming, notifications

The official React web/mobile apps can still point at this server (set
their API base URL to it); the built-in UI at `/` is always available.

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

## Distribution / release

Prebuilt artifacts for Linux/macOS/Windows × x64/arm64 are in `dist/`:

- `immich-go-1.0.0-go-<os>-<arch>.{tar.gz,zip}` — per-platform packages
- `immich-go-1.0.0-go-release.zip` — combined release bundle (all 6 + docs + checksums)
- `checksums.txt` — SHA-256 of every binary

Verify: `sha256sum -c dist/checksums.txt`.

## Docker image (GHCR)

On every published GitHub Release the CI also builds and pushes a
multi-arch image to GitHub Container Registry:

```
ghcr.io/idealisan/patrick-gallery:latest
ghcr.io/idealisan/patrick-gallery:<release-tag>
```

```sh
docker run -d --name immich-go \
  -p 8081:8081 \
  -v "$(pwd)/data:/data" \
  ghcr.io/idealisan/patrick-gallery:latest
# -> http://localhost:8081  (admin@immich.app / password)
```

The image is multi-arch (`linux/amd64`, `linux/arm64`), built with
`CGO_ENABLED=0` so the binary is fully static. Mount `/data` to persist
the SQLite DB (`immich.db`) and media (`resources/`). The `Dockerfile`
and `.dockerignore` live at the repo root of this module.

## Performance

Single static binary, ~3 MB idle RAM, ~1.5 s startup, hundreds of
req/s on modest hardware. See `PERFORMANCE_REPORT.md`.

## License

This is an independent reimplementation for learning/self-hosting; it is
not affiliated with the Immich project.
