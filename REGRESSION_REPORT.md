# Regression Report — immich-go v1.1.0-go

**Scope:** Full regression pass over the released tag `v1.1.0-go`
(plus the `ci.yml` Lark-notify workflow added afterward). Goal: classify
every major feature as **fully available**, **partially available**, or
**not done**.

**How tested**
- A new integration suite `internal/app/regression_test.go` drives the real
  HTTP handlers (auth guard + every route) through `httptest` against a real
  temp SQLite store. It covers server meta, auth, asset upload + serve,
  asset queries, albums, library scan, map markers, public sharing, timeline,
  tags, search, people, jobs, trash, and activities. **All 13 areas pass.**
- The same suite runs on **GitHub Actions** (`go test ./...` in
  `.github/workflows/ci.yml`) on every push to `main` and every PR, so the
  checks execute on GitHub's runners, not just the limited local box.
- Local machine is headless Linux with no GPU, so hardware video encoders
  are absent (the ffmpeg backend correctly falls back to software `libx264`).

---

## ✅ Fully available
| Area | Evidence |
|------|----------|
| Server meta (`/ping`,`/health`,`/about`,`/version`,`/config`,`/features`) | 200 on all |
| Auth | login (wrong→401, right→201), `validateToken` (204), `users/me` (200) |
| Asset upload (multipart) | 201, id returned |
| Asset serve | `original`/`thumbnail` (image/*)/`preview` all 200 |
| Asset queries | `search`, `count`, `random`, `statistics`, `duplicates` all 200 and return data |
| Albums | create (201), add assets (200), list/get, set cover, statistics |
| Libraries | create (201), **scan crawls disk, dedups by sha1, imports** (verified imported=2 for 2 distinct files, 0 on re-scan), statistics |
| Map markers | `GET /api/map/markers` returns clustered GPS markers (verified Tokyo marker) |
| Sharing (public) | `POST /api/shared-links` (201) → `GET /api/share/:key` (200, no auth) + thumbnail (200, no auth) |
| Timeline | `buckets` + `assets` return data |
| Tags | create/list |
| Trash / Activities | list endpoints 200 |

## 🟡 Partially available (works, but limited)
| Area | Limitation |
|------|------------|
| **Search** | `metadata`/`suggestions`/`person`/`explore` return 200 but there is **no semantic/CLIP search** — only basic metadata/field matching. `explore` is a GET route. |
| **People / face recognition** | `GET /api/people` returns 200 but an **empty** list — ML clustering is not implemented (STATUS #1). |
| **Jobs** | `GET /api/jobs` (200) and `POST /api/jobs/:id` (202 "queued") work, but are **no-op stubs** — thumbnail generation / metadata extraction are not executed by a job worker (STATUS #3). Thumbnails are generated synchronously at upload/scan time instead. |
| **Map reverse-geocoding** | Markers + SPA view work, but lat/lon→city/country is **not** derived; the name only shows when EXIF already carries `city`/`country`. |
| **Video** | Transcode + thumbnail work **when FFmpeg shared libs are present at runtime** (they are here). Hardware-accel auto-fallback (VideoToolbox→NVENC→QSV→AMF→libx264) is implemented and selects `high`/`main` profile by CPU/RAM, but on a GPU-less host it uses software `libx264`. HEIC/AVIF/HEIF/TIFF/BMP ingest is limited by the pure-Go decoders available. |
| **OAuth / SSO, memories, sync streaming, notifications/email, storage migration, trash auto-cleanup, admin maintenance panels** | Endpoints may exist (return 200) but richer behaviour is not implemented. |

## ❌ Not done
- Real ML: face clustering / person identification, smart (CLIP) search.
- Reverse geocoding service.
- OS-native hardware backends as separate modules (VideoToolbox / MediaFoundation / MediaCodec) — covered in practice by ffmpeg encoder-name probing.
- OAuth/SSO, admin maintenance dashboard, memories, websocket sync, email/notifications, storage migration, scheduled trash cleanup.

---

## Outcome
- All tested endpoints respond correctly; no 5xx regressions found in the
  covered surface. The only 404/401/202 responses are by-design
  (wrong creds → 401, `search/explore` GET vs POST, jobs "queued" → 202).
- The feature gaps above are **pre-existing design limitations**, not
  regressions introduced by v1.1.0-go.
- Automated coverage now lives in CI: pushing this report (and the suite)
  to `main` re-runs the whole regression on GitHub Actions.
