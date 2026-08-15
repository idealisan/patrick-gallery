# No-Stub Policy & Current Stub Inventory

> **Binding rule (AGENTS.md hard rule #7):** This is a real, production-grade
> product — **not** an API mock. No API route or feature may be a stub. Any
> endpoint in the official Immich client contract must be *genuinely
> implemented*, or return an *honest* error (`4xx` / `501`) when the capability
> is genuinely infeasible. Returning an empty `200` "so the client proceeds" is
> **forbidden**. Full definition, disposition rules, and CI gate: see
> `AGENTS.md` → "No-stub policy (operational)".

This document is the working inventory of every current stub. Each row lists
the handler, the contract operation it maps to, the stub *shape*, the required
*disposition*, and the feasibility. Drive the list to zero.

Legend:
- **FAKE-SUCCESS** — returns a constant / empty / zeroed `200` while doing no
  real work. Hard violation. Must implement or convert to honest error.
- **HONEST-EMPTY** — returns the truthful (empty) current state because an
  upstream capability (e.g. ML clustering) is missing. Not "faking", but the
  *feature* is not delivered; must implement the upstream or convert the
  endpoint to an honest error.
- **FALLBACK** — honest runtime degradation to a real backend; **not** a stub
  violation (tracked only as a future enhancement).

---

## A. Fake-success stubs (must fix)

| # | Handler (`file:line`) | Contract op | Stub shape | Disposition | Feasible in pure Go? |
|---|---|---|---|---|---|
| 1 | `internal/app/compat_v3.go:37` `handleServerStorage` | `GET /server/storage` | Constant `0 B` / `0` for disk fields | ✅ **已实现** — 真实磁盘用量（`disk_unix.go`/`disk_windows.go` 跨平台 `diskUsage`） | ✅ trivial |
| 2 | `internal/app/compat_v3.go:49` `handleServerApkLinks` | `GET /server/apk-links` | Empty APK URLs | Honest: return empty is acceptable (no APKs hosted) **or** `501` | ✅ trivial |
| 3 | `internal/app/compat_v3.go:78` `handleSyncAck` | `GET/POST/DELETE /sync/ack` | Returns `{ack, type:"AssetV1"}`, no state change | ✅ **已实现** — 持久化用户已确认序列（`sync_state`） | ✅ |
| 4 | `internal/app/compat_v3.go:86` `handleSyncStream` | `GET /sync/stream` | No-op `[]any{}` delta stream | ✅ **已实现** — 返回用户全部资源/相册真实增量（AssetV1/AlbumV1）；需在真机验证客户端是否据此完成首次同步 | ✅ / ⚠️ needs real-app test |
| 5 | `internal/app/compat_v3.go:92` `handlePersonStatistics` | `GET /people/:id/statistics` | `{assets:0}` constant | Count from `assets.person_id` (needs schema column) or honest `501` | ⚠️ needs schema |
| 6 | `internal/app/compat_v3.go:96` `handleFacesList` | `GET /faces` | `[]any{}` | Implement face list (needs face table + detector) or honest `501` | ❌ ML detector out of scope |
| 7 | `internal/app/compat_v3.go:104` `handlePersonMerge` | `POST /people/:id/merge` | `200` no-op | Implement merge (reassign `person_id`) or honest `501` | ⚠️ needs schema |
| 8 | `internal/app/compat_v3.go:108` `handlePersonReassign` | `PUT /people/:id/reassign` | `200` no-op | Implement reassign or honest `501` | ⚠️ needs schema |
| 9 | `internal/app/compat_v3.go:114` `handleSearchCities` | `GET /search/cities` | `[]any{}` | ✅ **已实现** — 基于内嵌 GeoNames 城市库按名称检索 | ✅ (geo data present) |
| 10 | `internal/app/compat_v3.go:118` `handleSearchPlaces` | `GET /search/places` | Empty `places/recentPlaces/allPlaces` | ✅ **已实现** — 基于内嵌 GeoNames 库返回 places/allPlaces | ✅ (geo data present) |
| 11 | `internal/app/compat_v3.go:124` `handleDownloadInfo` | `POST /download/info` | `{size:0}` constant | ✅ **已实现** — 计算请求资源的真实字节大小 | ✅ |
| 12 | `internal/app/compat_v3.go:128` `handleDuplicatesResolve` | `POST /duplicates/resolve` | `200` no-op | ✅ **已实现** — 记录 keeper/hidden（`duplicate_resolutions`），GET /assets/duplicates 不再重复展示 | ✅ |
| 13 | `internal/app/compat_v3.go:132` `handleDuplicatesUpdate` | `PUT/DELETE /duplicates/:id`, `DELETE /duplicates` | `200` no-op | ✅ **已实现** — PUT 改 keeper / DELETE 移除处理记录（无 :id 清空该用户全部） | ✅ |
| 14 | `internal/app/search.go:135` `handleSearchPerson` | `POST /search/person` | `{total:0, assets:[]}` | Implement person search (needs clustering) or honest `501` | ❌ ML out of scope |

## B. Honest-empty (feature incomplete — must deliver or honest-error)

These handlers return truthful state (often empty) rather than faking, but the
underlying feature is not delivered because an upstream capability (face
clustering, `person_id` linkage) is missing.

| # | Handler (`file:line`) | Contract op | State | Disposition |
|---|---|---|---|---|
| 15 | `internal/app/misc.go:289` `handlePeopleList` | `GET /people` | Real `Person` rows (empty without ML) | Implement clustering, or honest `501` when no ML |
| 16 | `internal/app/misc.go:309` `handlePersonGet` | `GET /people/:id` | Real person, `assets.total:0` (no `person_id`) | Add `person_id` linkage or honest `501` |
| 17 | `internal/app/compat.go:145` `handlePersonAssets` | `GET /people/:id/assets` | `{assets:[], total:0}` (no `person_id`) | Add `person_id` linkage or honest `501` |
| 18 | `internal/app/compat.go:159` `handleSearchSuggestions` | `POST /search/suggestions` | Empty `people/locations/tags` (ML/geo) | Implement geo suggestions; people via clustering or honest `501` |

## C. Acceptable fallback (NOT a stub — enhancement only)

| Item | Location | Notes |
|---|---|---|
| OS-native video backends | `internal/video/native_stub.go` (`newVideoToolbox` / `newMediaFoundation` / `newMediaCodec`) | Return `errBackendUnavailable` and fall through to the **real** FFmpeg backend. Honest degradation, not a fake feature. Implement purego bindings as enhancement. |
| Placeholder video backend | `internal/video/placeholder.go` | Returns the original file / no thumbnail when no engine is available. Real degradation, not a stub. |
| `handleFaceGet` | `internal/app/compat_v3.go:100` | Returns `404 "no ML backend"` — an **honest error**, not a fake success. (Note: method mismatch with spec `/faces/:id` GET/PUT/DELETE — see `docs/API_STATUS.md`.) |

---

## Remediation procedure

For each row above:
1. **Implement for real** when feasible (prefer; many are trivial — see ✅).
2. **Otherwise return an honest error**: `c.JSON(501, ...)` (or `404` where the
   resource genuinely cannot exist). Add the operation to
   `scripts/schemathesis-allowlist.txt` so the contract test treats it as an
   *expected* gap, never as a fake success. Record it in `docs/GAP_ANALYSIS.md`.
3. Remove any "returns the correct, empty shape so the client proceeds" comment
   and the corresponding fake-success body.

**Definition of done:** every route registered in `internal/app/app.go` either
performs its real function or returns an honest `4xx`/`501`. `docs/API_STATUS.md`
should contain **zero** 🟠 rows.
