# Backend alignment — immich-go vs. upstream Immich

This document explains **how `immich-go` maps onto the official Immich
backend** (`immich-app/immich` → `server/`, a NestJS/TypeScript service) and
where the two diverge. It exists so contributors can find the equivalent code
on either side and understand the compatibility boundary.

> Reference source: a shallow clone of the upstream `server/` tree
> (`immich-app/immich`, `main` branch) was used to write this document
> (kept locally for reference, **not** committed to this repo — this project
> is an independent, pure-Go reimplementation per `AGENTS.md`, not a fork of
> the Node code).

## 1. Architectural mapping

| Upstream Immich (`server/src`) | immich-go (`internal/app/*.go`) | Notes |
|---|---|---|
| `main.ts` + `app.module.ts` + `database.ts` | `app.go` + `db.go` | Bootstrap, DI wiring, DB connection. Upstream uses **TypeORM**; immich-go uses `store.Store` (GORM + pure-Go SQLite, `glebarez/sqlite`). |
| `controllers/server.controller.ts` (`config`/`features`/`version`/`about`/`ping`/`health`) | `server.go` + `compat.go` + `config.go` + `misc.go` | The "handshake" trio (`/api/server/{config,features,version,about}`) is aligned field-for-field with the latest upstream spec. |
| `controllers/auth.controller.ts` + `auth.service.ts` (+ `api-key`, `session`) | `auth.go` | JWT login/signup/logout/validate, change-password, API-key CRUD. OAuth/SSO routes are **not** implemented. |
| `controllers/user.controller.ts` (+ `user-admin`) | `user.go` | `/users`, `/users/me`, update, preferences, avatar. Admin dashboards omitted. |
| `controllers/asset.controller.ts` + `asset-media.controller.ts` (+ `services`) | `asset.go` | Upload (multipart), list/count/search/random, get/update/delete, original, thumbnail, `bulk-upload-check`, `encoded-video`. |
| `services/media.service.ts`, `storage.service.ts`, `metadata.service.ts` | `ingest.go` + `internal/image` + `internal/video` | Thumbnail generation, EXIF extraction, video probe/transcode. Upstream shells out to `ffmpeg`/`exiftool`; immich-go is **in-process** (purego-loaded FFmpeg, pure-Go EXIF/WebP). |
| `controllers/album.controller.ts` | `album.go` | Full CRUD + membership + cover + `GET /:id/assets`. |
| `controllers/tag.controller.ts` | `tag.go` | CRUD + asset (un)bind. |
| `controllers/library.controller.ts` | `library.go` | CRUD + statistics + real on-disk `scan` (EXTERNAL libraries, dedup by content hash). |
| `controllers/timeline.controller.ts` | `timeline.go` | Year/month buckets + bucket assets. |
| `controllers/search.controller.ts` | `search.go` | Filename/EXIF text, metadata, explore, suggestions. Semantic/CLIP search omitted. |
| `controllers/map.controller.ts` | `map.go` | `/map/markers` (GPS-clustered). Reverse-geocoding omitted. |
| `controllers/shared-link.controller.ts` + public `view`/`share` | `share.go` | CRUD + public keyless `/share/:key` access. |
| `controllers/job.controller.ts` + `job.service.ts` | `jobs.go` | Real worker-pool executor for `thumbnailGeneration`/`metadataExtraction`/`videoConversion`/`duplicateDetection`; ML jobs return `unsupported`. |
| `controllers/activity.controller.ts`, `partner`, `trash`, `duplicate` | `misc.go` | Activities, partners, trash, duplicates. |
| `controllers/download.controller.ts` | `misc.go` | Zip archive download. |
| `dtos/*` + `entities/*` | `models.go` | Response DTOs shaped to match the upstream contract (e.g. `AssetResponseDto`). |

## 2. Compatibility contract (what the mobile/desktop apps expect)

Aligned to the **latest upstream OpenAPI** (target `IMMICH_COMPAT_VERSION`
default `1.130.0`, configurable via env). Key endpoints verified:

- `GET /api/server/version` (`prerelease`), `/api/server/features` (all 16
  required keys), `/api/server/config`, `/api/server/about`.
- `POST /api/assets` (latest multipart contract: `filename` +
  `fileCreatedAt`/`fileModifiedAt` ISO8601, `duration` int, `visibility`
  enum, `livePhotoVideoId`; type sniffed from magic bytes, no `assetType`).
- `POST /api/assets/bulk-upload-check` (checksum dedup →
  `{results:[{id,action:accept|reject,reason,assetId,isTrashed}]}`).
- `AssetResponseDto` fields: `exifInfo`, `isTrashed`, `duration` (int),
  `visibility`, `resized`, `thumbhash`, `originalMimeType`, `owner`,
  `people`, `tags`, etc.
- Gallery fidelity: `width`/`height`, `thumbhash` (byte-compatible with the
  upstream `thumbhash` reference), `owner`.

Set `IMMICH_COMPAT_VERSION` to match the version of the client app you
connect; a mismatch triggers the "server version not supported" error in the
app.

## 3. Upstream modules intentionally NOT implemented

These exist as controllers/services upstream but are out of scope for the
single-user / private-LAN target (see `AGENTS.md`):

- **ML stack**: `person`/`face` (facial recognition), `smart-info` (CLIP),
  `ocr`, object detection — `people` returns stored rows only; no clustering.
- **Auth/identity**: `oauth`, `auth-admin`, `session`, `user-admin`.
- **Operations**: `maintenance`, `database-backup`, `integrity-admin`,
  `queue`, `system-metadata`, `notification(-admin)`.
- **Product features**: `memory`, `plugin`, `workflow`, `stack`, `sync`
  (websocket), `trash` auto-prune, storage-template migration, email.
- **Video streaming**: upstream `video-stream.controller.ts` (HLS); immich-go
  serves a transcoded MP4 via `/api/assets/:id/encoded-video` instead.

This is ~160 of upstream's ~220 routes. The library experience — upload,
dedup, browse, albums, search, timeline, sharing, admin jobs, in-process
video — is covered.

## 4. Where the engines differ (by design)

| Concern | Upstream | immich-go |
|---|---|---|
| DB | Postgres (TypeORM) | SQLite WAL via `store.Store` (single-user) |
| Image thumb | `sharp` (libvips) | pure-Go decode + `golang.org/x/image` |
| EXIF | `exiftool` | pure-Go EXIF parse |
| Video | CLI `ffmpeg` + `libvips` | **in-process** purego `libav*` (no CLI, no CGO) |
| ML | Python microservice | none (stub endpoints) |
| Web UI | React SPA (build step) | vanilla-JS SPA embedded in the binary |
| Build | Node + tsc | single static Go binary (`CGO_ENABLED=0`) |

## 5. How to extend

To add an upstream feature, find its controller in the table above, mirror
the route + DTO shape in the matching `internal/app/*.go` file, and add a
regression test under `internal/app/*_test.go`. Keep the binary pure-Go and
CGO-free; any native work (video) goes through `internal/video.Processor`.
