# immich-go — User Manual

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

**Not included / stubbed** (require the upstream ML stack or a server
cluster, and are intentionally out of scope):
- Facial recognition / `people` recall, smart (semantic) search, CLIP.
  (`/api/people`, `/api/search/person`, smart search) return minimal data.
- Video transcoding. `encoded-video` serves the original file.
- OAuth / SSO, admin & maintenance dashboards, map/reverse-geocoding,
  stacks, memories, workflows, plugins, sync streaming, notifications.
- `POST /api/assets/bulk-upload-check` and a few admin endpoints are
  absent. The full upstream API surface is ~250 routes; `immich-go`
  implements the core ~90 used by the library experience.

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

---

## 4. Configuration (environment variables)

All are optional; defaults shown.

| Variable | Default | Purpose |
|----------|---------|---------|
| `IMMICH_PORT` | `8081` | Listen port |
| `IMMICH_HOST` | `0.0.0.0` | Listen address |
| `IMMICH_DB` | `immich.db` | SQLite database file path |
| `IMMICH_RESOURCE` | `resources` | Media/thumbnail storage directory |
| `IMMICH_JWT_SECRET` | random per run | JWT signing key |
| `IMMICH_API_KEY_SALT` | random per run | API-key hashing salt |
| `IMMICH_LOGIN_REQUIRED` | `true` | If `false`, anonymous access as admin |
| `IMMICH_ADMIN_EMAIL` | `admin@immich.app` | Admin email (first run) |
| `IMMICH_ADMIN_PASSWORD` | `password` | Admin password (first run) |

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
