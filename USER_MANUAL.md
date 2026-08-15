# immich-go — User Manual

> 🇨🇳 A comprehensive **Chinese** user manual is available at
> [USER_MANUAL_ZH.md](USER_MANUAL_ZH.md) — it covers every config variable,
> how to configure, connect, and use the server (current version **v1.4.0-go**).

`immich-go` is a self-contained, single-binary reimplementation of the
Immich photo/video server API, written in Go. It stores everything in a
local SQLite database and the filesystem, serves a built-in web gallery,
and requires no Node.js, Postgres, Redis, or machine-learning services.

---

## 1. What it is (and isn't)

**Included (core library):** authentication (JWT + API keys), users,
asset upload (images + video metadata), thumbnail generation for images,
asset listing / count / search / random / duplicates, albums (CRUD +
membership + cover), libraries (CRUD + statistics), timeline buckets,
search (filename/EXIF text + metadata + explore + suggestions), tags,
partners, trash, activities, shared-links, system-config, and a small
web UI.

**Not included / deferred** (require the upstream ML stack or a clustered
server, and are intentionally out of scope for now):
- Facial recognition / auto person clustering, smart (semantic) search, CLIP,
  OCR. `/api/people` *list/get/merge/reassign* are real, but **face
  detection** (`/faces`) returns an honest `501` (no ML backend bundled).
- OAuth / SSO, email / external notifications, memories, workflows, plugins,
  horizontal multi-tenant scaling.
- The full upstream API surface is ~254 routes; `immich-go` implements the
  core ~100+ used by the library experience and is contract-tested against
  the v3.1.0 OpenAPI spec.

**Now implemented (correcting older docs):** in-process video transcoding
(`/encoded-video` + HLS via purego-loaded FFmpeg 7.1), map markers + offline
reverse-geocoding, admin user-management dashboard (`/admin/users/*`),
shared-link public access, on-disk library scan, and real background jobs
(`thumbnailGeneration` / `metadataExtraction` / `videoConversion` /
`duplicateDetection` / `trashCleanup`).

The web UI is a lightweight built-in gallery (login, upload, thumbnail
grid, counts), **not** the full React Immich web app. To use the official
React UI, point its `IMMICH_API_URL` at this server (see §7).

---

## 2. Download & install

Get the package for your platform from the distribution:

| Platform | Archive |
|----------|---------|
| Linux ARM64 (e.g. Raspberry Pi, SBC) | `immich-go-1.0.0-go-linux-arm64.tar.gz` |
| Linux x86-64 | `immich-go-1.0.0-go-linux-amd64.tar.gz` |
| macOS Apple Silicon | `immich-go-1.0.0-go-darwin-arm64.tar.gz` |
| macOS Intel | `immich-go-1.0.0-go-darwin-amd64.tar.gz` |
| Windows x86-64 | `immich-go-1.0.0-go-windows-amd64.zip` |

Verify integrity:

```sh
sha256sum -c checksums.txt
```

Extract and enter the package:

```sh
tar xzf immich-go-1.0.0-go-linux-arm64.tar.gz
cd immich-go-1.0.0-go-linux-arm64
```

Each package contains `immich-go` (or `immich-go.exe`), `README.txt`,
and a launcher (`start.sh` / `start.bat`).

---

## 3. Run

### Linux / macOS

```sh
./start.sh
# or simply:
./immich-go
```

### Windows

```bat
start.bat
:: or double-click immich-go.exe
```

The server listens on `http://0.0.0.0:8081` by default. Open
`http://<host>:8081` in a browser.

### Default account

On first run a default admin is created:

```
email:    admin@immich.app
password: password
```

Change it immediately via the web UI (Settings → change password) or the
API (`PUT /api/auth/change-password`).

### With Docker (GHCR multi-arch image)

On every published release the CI pushes a multi-arch image
(`linux/amd64`, `linux/arm64`) to GitHub Container Registry:

```sh
docker run -d --name immich-go \
  -p 8081:8081 \
  -v "$(pwd)/data:/data" \
  ghcr.io/idealisan/patrick-gallery:latest
```

- Mount `/data` to persist the SQLite DB and media. Inside the container
  `IMMICH_DB=/data/immich.db` and `IMMICH_RESOURCE=/data/resources`.
- Tags: `:latest` and `:<release-tag>` (e.g. `:v1.0.0-go`).
- The binary is fully static (`CGO_ENABLED=0`), so the image runs on any
  compatible arch with no libc dependency.
- To build the image locally instead: `docker build -t immich-go .`
  (Dockerfile + .dockerignore are in this directory).

### Bundled FFmpeg

Each platform package ships with a `libs/` directory containing the
pinned **FFmpeg 7.1 shared libraries** (`.so` / `.dylib` / `.dll`). The
Go binary loads them at runtime via purego by searching its own
directory and `<exe dir>/libs`, so **video thumbnail generation and
transcoding work out of the box** — no separate FFmpeg install required.

Differences in packaging:

- **Linux / macOS**: the shared objects (`libav*` / `libsw*`) are placed
  in `libs/`.
- **Windows**: the BtbN FFmpeg 7.1 shared build DLLs are placed in
  `libs/`.

If `libs/` is absent (e.g. you only built the binary yourself), the
server falls back to a **placeholder video backend**: video still
uploads and plays its original file, but no thumbnail is extracted and
`/encoded-video` serves the original without transcoding.

> Note: there is **no** `IMMICH_VIDEO_BACKEND` env var — the video backend
> is selected automatically at runtime (purego-loaded FFmpeg 7.1 when the
> shared libs are found, otherwise the placeholder backend).

Video endpoints:

| Endpoint | Purpose |
|----------|---------|
| `GET /api/assets/:id/thumbnail` | Poster / poster frame (extracted from video when libs present). |
| `GET /api/assets/:id/encoded-video/:ts` | Transcoded playback stream (original served if no transcode). |
| `GET /api/assets/:id/preview` | *(planned)* downscaled preview variant. |

---

## 4. Configuration (environment variables)

All are optional; defaults shown.

| Variable | Default | Purpose |
|----------|---------|---------|
| `IMMICH_PORT` | `8081` | Listen port |
| `IMMICH_HOST` | `0.0.0.0` | Listen address |
| `IMMICH_DB` | `immich.db` | SQLite database file path |
| `IMMICH_RESOURCE` | `resources` | Media/thumbnail storage directory |
| `IMMICH_JWT_SECRET` | `immich-dev-secret-change-me` | JWT signing key (**set a fixed value in prod**) |
| `IMMICH_API_KEY_SALT` | `immich-dev-api-salt` | API-key hashing salt (**set a fixed value in prod**) |
| `IMMICH_LOGIN_REQUIRED` | `true` | If `false`, anonymous access as admin |
| `IMMICH_ADMIN_EMAIL` | `admin@immich.app` | Admin email (first run / empty DB) |
| `IMMICH_ADMIN_PASSWORD` | `password` | Admin password (first run / empty DB) |
| `IMMICH_EXTERNAL_DOMAIN` | `` (empty) | External domain for share-link URLs |
| `IMMICH_COMPAT_VERSION` | `3.1.0` | Immich server version advertised to clients |
| `IMMICH_TRASH_DAYS` | `30` | Days before trashed assets are permanently deleted |

Examples:

```sh
# persistent tokens across restarts:
export IMMICH_JWT_SECRET="a-long-random-string"
export IMMICH_API_KEY_SALT="another-long-random-string"
# custom port + storage location:
export IMMICH_PORT=8080
export IMMICH_DB=/var/lib/immich/immich.db
export IMMICH_RESOURCE=/var/lib/immich/library
./immich-go
```

> **Set `IMMICH_JWT_SECRET` and `IMMICH_API_KEY_SALT` to fixed values**
> if you run more than one instance or restart often — otherwise issued
> JWTs and API keys rotate on every start.

---

## 5. Using the web UI

Open `http://<host>:8081`:

1. **Sign in** with the admin credentials.
2. The dashboard shows total / photos / videos counts.
3. **Upload asset** — pick an image or video; it is stored and a 256px
   JPEG thumbnail is generated for images.
4. The **gallery grid** renders thumbnails (fetched with your auth token).
5. **Refresh** reloads counts and the grid.
6. **Logout** clears the local token.

---

## 6. Using the API (curl)

```sh
# health / about
curl http://localhost:8081/api/server/health
curl http://localhost:8081/api/server/about
curl http://localhost:8081/api/server/config

# login -> capture accessToken
TOKEN=$(curl -s -X POST http://localhost:8081/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@immich.app","password":"password"}' \
  | sed -n 's/.*"accessToken":"\([^"]*\)".*/\1/p')

# counts + list
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/assets/count
curl -H "Authorization: Bearer $TOKEN" "http://localhost:8081/api/assets?take=50"

# upload an image
curl -X POST http://localhost:8081/api/assets \
  -H "Authorization: Bearer $TOKEN" \
  -F 'asset={"deviceAssetId":"x-1","deviceId":"cli","fileCreatedAt":"2024-01-01T00:00:00Z","fileModifiedAt":"2024-01-01T00:00:00Z","localDateTime":"2024-01-01T00:00:00Z","fileExtension":".png","type":"IMAGE"}' \
  -F 'assetData=@photo.png'

# fetch the generated thumbnail (auth required)
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/assets/<ASSET_ID>/thumbnail -o thumb.jpg
```

### API-key auth

```sh
# create a key
KEY=$(curl -X POST http://localhost:8081/api/api-keys \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"cli"}' | sed -n 's/.*"key":"\([^"]*\)".*/\1/p')
# use it instead of a JWT
curl -H "x-api-key: $KEY" http://localhost:8081/api/assets/count
```

---

## 7. Pointing the official Immich web/mobile apps at immich-go

The official React web app and mobile apps speak the Immich REST API.
`immich-go` implements the core routes they need for browsing, uploading,
and organizing. To use the official web UI:

1. Build/obtain the Immich `web/dist` (or a release `immich-web` image).
2. Serve those static files and set their API base URL to this server
   (e.g. `IMMICH_API_URL=http://host:8081`).
3. Or configure a reverse proxy to serve the official `web/dist` at `/`
   and proxy `/api` to `immich-go`.

> The built-in UI (§5) is always available at `/` regardless.

---

## 8. Running behind a reverse proxy (TLS)

`immich-go` serves plain HTTP. For HTTPS, put it behind Nginx/Caddy:

```nginx
location /api/ {
    proxy_pass http://127.0.0.1:8081;
    proxy_set_header Host $host;
    proxy_set_header Authorization $http_authorization;
}
location / {
    proxy_pass http://127.0.0.1:8081;
}
```

---

## 9. Running as a systemd service (Linux)

Copy `immich-go.service` to `/etc/systemd/system/`, edit the
`WorkingDirectory` / `ExecStart` paths and secrets, then:

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now immich-go
```

---

## 10. Backups & data

All state lives in two places:
- the SQLite file (`immich.db`) — back this up (stop the server or copy
  while idle to avoid a partial write).
- the resource directory (`resources/`) — the original media + thumbnails.

To move the server, copy both together. There is no migration step.

---

## 11. Troubleshooting

| Symptom | Fix |
|---------|-----|
| `address already in use` | Another instance or port conflict; change `IMMICH_PORT`. |
| 401 on a previously-valid token | Server restarted with a new random `IMMICH_JWT_SECRET`; set it to a fixed value. |
| Thumbnail 404 after upload | The uploaded file wasn't a decodable image; re-upload a valid JPEG/PNG. Videos don't get thumbnails (stub). |
| Uploads/DB not where expected | Check `IMMICH_DB` / `IMMICH_RESOURCE`; they're created relative to the working directory. |
| Web UI shows JSON instead of gallery | You requested a bare API path; open `/` in a browser, not `/api/...`. |

Logs are written to stdout; run in the foreground to see them.

---

## 12. Reset / start over

Stop the server and delete its data:

```sh
rm -f immich.db
rm -rf resources
```

The next start recreates the database and the default admin account.
