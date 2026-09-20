# 原版 Immich v3.1.0 API 实测报告（Live Probe）

> 本报告基于对**真实运行的原版 Immich v3.1.0 服务端**的逐端点探测，而非 OpenAPI
> 规范文件或源码推断。全部 285 次真实请求的证据（请求 + 状态码 + 响应体）见
> [immich-v3.1.0-live-probe-data/](./immich-v3.1.0-live-probe-data/)。
>
> 探测时间：2026-09-20。环境与方法见 §1，复现步骤见 §6。

---

## 0. 一句话结论

原版 v3.1.0 共 254 个 operation，本次实测覆盖 **253/254**（唯一未覆盖
`GET /oauth/mobile-redirect` 因其 302 指向不可达的外部 OAuth 而无法完成）。
实测得到了大量 OpenAPI 规范**没有写**的行为契约：

1. **`POST /assets` 的 `duration` 字段是数字（秒）**，不是 `HH:MM:SS.mmm`
   字符串；重复上传返回 **200 + `{status:"duplicate", id}`**（按内容 checksum 判定）。
2. **`AssetResponseDto` 的形状与 immich-go 差异很大**：官方用
   `visibility: "timeline" | "archive" | "hidden"`（不存在 `isArchived` 字段），
   顶层带 `owner`（完整 user 对象）、`resized`、`width`、`height`、`isEdited`、
   `duplicateId`、`hasMetadata`、`stack`；**不含** `thumbnailPath`/`previewPath`/`webpPath`。
3. **`GET /timeline/buckets` 返回 `[{"timeBucket":"2021-06-01","count":1}]`** —
   `timeBucket` 是 `YYYY-MM-DD` 日期字符串（按月分桶时是月首日），不是 ISO datetime。
4. **`POST /auth/change-password` 返回 200 + `UserAdminResponseDto`**（不是 204），
   且**会使该用户全部会话失效**；`POST /auth/logout` 同样使会话失效。
5. **官方对缺失/非法输入也返回 500（11 处）**，例如
   `POST /assets/bulk-upload-check`、`PUT /assets/metadata`、`POST /partners`、
   `POST /users/profile-image`、`DELETE /sync/ack` 等在 body 缺字段时直接
   `500 {"message":"Failed to …"}` —— 与本项目 AGENTS.md 规则 7「诚实 4xx」的
   假设不同：**原版自己就不诚实**。
6. **`POST /admin/maintenance {action:"start"}` 会立刻把服务器切进维护模式**
   （普通 API 全部 404），恢复必须持有 start 响应 `Set-Cookie` 下发的
   `immich_maintenance_token`（状态持久化在 `system_metadata.maintenance-mode`，
   含随机 secret，重启不消失）。这是探测/自动化时最大的坑。
7. **`GET /server/config`、`GET /server/features`、`GET /server/version`、
   `GET /server/media-types` 是公开的（无需登录）**；而 `GET /server/about`、
   `GET /server/statistics` 需要认证。`/server/theme` 在 v3.1.0 **不存在**（404）。

---

## 1. 目的与方法

### 1.1 为什么不用 OpenAPI 文件

`open-api/immich-openapi-specs.json` 是官方的**参考**，但本项目已多次发现
「规范没写 = 实际没做」不成立（例如 401/400 错误状态、DTO 里的额外字段、
`duration` 的类型）。要看清原版 API 的真实样子，唯一可靠的办法是把原版
服务端跑起来，用真实客户端行为逐个端点打请求、看真实响应。

### 1.2 环境

| 项 | 值 |
|---|---|
| 原版版本 | **Immich v3.1.0**（`/api/server/version` → `{major:3, minor:1, patch:0}`） |
| 运行方式 | podman（WSL 后端）容器：`immich-server:v3.1.0` + `immich-app/postgres:14-vectorchord0.4.3-pgvectors0.2.0` + `valkey-io/valkey:9` |
| 机器学习 | `IMMICH_MACHINE_LEARNING_ENABLED=false`（真实特征：`/server/features` 返回 `smartSearch:false, facialRecognition:false, duplicateDetection:false, ocr:false`） |
| 探测器 | Node 24（`probe_server` 容器内运行，原生 `fetch` + `FormData`），脚本见 §6 |
| 覆盖 | 285 次真实请求，**253/254** operation（全部覆盖，除 1 个不可完成的 302） |

### 1.3 方法

1. 从规范枚举全部 254 个 `method + path`（仅作为**清单**，不作为行为依据）。
2. 先以真实场景建立状态：管理员注册/登录 → 上传 3 张 PNG（multipart）→
   建相册/标签/回忆/堆叠/共享链接/库/伙伴/Second user 等，把真实 id 记进上下文。
3. 逐端点用**真实 id**（缺失时用规范 example 值）发请求，记录
   `method、path、query、body、状态码、响应体`。
4. 未覆盖端点再以「example 值 + 合成 body」做一轮**通用覆盖**，确保 254 全打。
5. 会话失效（change-password/logout）自动重登；`POST /admin/maintenance`
   强制固定为 `{action:"end"}`（见 K4）。

---

## 2. 服务器事实（无需推断，全部实测）

### 2.1 公开端点（无认证可用）

| 端点 | 实测 | 备注 |
|---|---|---|
| `GET /api/server/ping` | 200 `{"res":"pong"}` | |
| `GET /api/server/version` | 200 `{major,minor,patch,prerelease}` | 客户端版本协商 |
| `GET /api/server/features` | 200 | 开关：`smartSearch/facialRecognition/duplicateDetection/ocr/map/reverseGeocoding/sidecar/search/trash/oauth/passwordLogin/configFile/importFaces` |
| `GET /api/server/config` | 200 | 含 `loginPageMessage、trashDays:30、userDeleteDelay:7、oauthButtonText、isInitialized、isOnboarded、externalDomain、publicUsers、map*StyleUrl、maintenanceMode、minFaces` |
| `GET /api/server/media-types` | 200 | `{video:[…扩展名], image:[…]}` |
| `GET /api/admin/maintenance/status` | 200 | 维护模式状态（公开） |
| `GET /api/share/:key` 及公开分享页 | 200/404 | 无认证 |

### 2.2 需要认证才有的（与直觉不同）

| 端点 | 实测 |
|---|---|
| `GET /api/server/about` | **401**（需要认证） |
| `GET /api/server/statistics` | **401**（需要认证） |
| `GET /api/server/theme` | **404**（v3.1.0 不存在该路由） |
| `GET /api/users/me/statistics` | **404**（v3.1.0 不存在） |

---

## 3. 关键发现（K 系列）

### K1. 认证

- 登录：`POST /api/auth/login` → **201**，
  `{accessToken, userToken, userId, userEmail, name, isAdmin, profileImagePath, shouldChangePassword, isOnboarded, …}`。
  同时 `Set-Cookie: immich_access_token=…`（httpOnly）。
- `Authorization: Bearer <accessToken>` 与 cookie 等价，multipart 上传同样可用。
- **`POST /auth/change-password`（200）与 `POST /auth/logout`（200）都会让该用户
  的所有会话失效**：失效后的请求统一返回
  `401 {"message":"Invalid user token"}`。
- 管理员操作（`/admin/*`）在失去管理员身份后返回
  `403 {"error":"admin privileges required","statusCode":403}`
  （immich-go 对应返回 401，**不一致**）。

### K2. 资产上传与 AssetResponseDto

- `POST /api/assets`（multipart）：`assetData`(文件) + `deviceAssetId`、
  `deviceId`、`fileCreatedAt`、`fileModifiedAt`、**`duration`（数字秒）**。
  `duration` 传字符串 `"0:00:00.000000"` 会得到
  `400 {"message":"Validation failed","errors":[{"expected":"number","received":"NaN","path":["duration"]}]}`。
- 重复上传（同 checksum）→ **200** + `{status:"duplicate", id:"…"}`；
  首次 → **201** + `{status:"created", id:"…"}`。
- `GET /api/assets/:id` 实测字段（完整清单）：
  `id, createdAt, ownerId, owner(嵌套 User 对象), libraryId, type, originalPath,
  originalFileName, originalMimeType, thumbhash, fileCreatedAt, fileModifiedAt,
  localDateTime, updatedAt, isFavorite, isArchived, isTrashed, visibility,
  duration, exifInfo, livePhotoVideoId, tags, people, checksum, stack, isOffline,
  hasMetadata, duplicateId, resized, width, height, isEdited`
  - **`visibility` 取代 `isArchived`**（`"timeline" | "archive" | "hidden"`），
    `isArchived` 仅作兼容保留。
  - `owner` 是嵌套对象；`resized/width/height` 在顶层；**没有**
    `thumbnailPath/previewPath/webpPath`（这些是内部路径，不下发）。

### K3. 时间线

- `GET /api/timeline/buckets` → `[{ "timeBucket": "2021-06-01", "count": 1 }]`，
  `timeBucket` 为 `YYYY-MM-DD`（月桶=当月 1 日）。
- `GET /api/timeline/bucket?timeBucket=…` 返回资产数组（非包络对象）。

### K4. 维护模式（最大陷阱）

- `POST /api/admin/maintenance` body `SetMaintenanceModeDto
  {action:"start"|"end"|"select_database_restore"|"restore_database",
  restoreBackupFilename?}`。
  - `action:"start"` → **服务器立即重启进维护模式**：普通 API 全部 404
    （`{"message":"Cannot GET /api/…"}`），只挂载 `/api/admin/maintenance/*`。
  - 响应 `Set-Cookie: immich_maintenance_token=<jwt>` 是**唯一**的恢复凭据；
    状态持久化于 `system_metadata` key=`maintenance-mode`
    （实测值含 `secret`，重启不消失）。
  - `POST /admin/maintenance/login {token:<维护 jwt>}` → 取得维护态操作权。
  - **在非维护态调用 `action:"end"` 是空操作**（控制器 `if (action === End) return;`）。
- 探测器必须把该端点的 body 固定为 `{action:"end"}`，否则一次误发就让服务不可用。

### K5. 官方自身的 500（11 处实测）

对缺失字段/非法 multipart，原版直接返回 **500**（非 4xx）：

```
POST /admin/database-backups/upload   Failed to upload database backup
POST /admin/notifications             Failed to create notification
POST /assets/bulk-upload-check        Failed to check bulk upload
PUT  /assets/metadata                 Failed to update bulk asset metadata
POST /download/archive                Failed to download archive
POST /oauth/link                      Error in OAuth discovery: TypeError: Invalid URL
POST /partners                        Failed to create partner
DELETE /stacks                        Failed to delete stacks
DELETE /sync/ack                      Failed to delete sync ack
POST /sync/ack                        Failed to send sync ack
POST /users/profile-image             Failed to create profile image
```

含义：本项目规则 7「官方不诚实 → 我们要诚实 4xx」在与官方**对比**时要注意——
这些端点原版行为就是 500，immich-go 若返回 400 反而与原版不一致（取舍见 §5）。

### K6. 错误响应形状（两种）

- 大多数业务校验失败：`{"message": "…", "statusCode": 400}`（部分还有 `"error"`）。
- DTO 校验失败（zod 风格）：
  ```json
  {"message":"Validation failed","errors":[
    {"expected":"number","code":"invalid_type","received":"NaN","path":["duration"],"message":"…"},
    {"origin":"string","code":"invalid_format","format":"uuid","pattern":"/^([0-9a-fA-F]{8}…)$/","path":["id"],"message":"Invalid UUID"}]}
  ```
  （`errors[].path` 为字段路径数组；`code` 有 `invalid_type` / `invalid_format` / `unrecognized_keys` 等。）

### K7. 其它实测契约点

- 标签颜色必须是 hex（`#RGB/#RGBA/#RRGGBB/#RRGGBBAA`），传 `"red"` → 400。
- API Key 创建**必须带 `permissions: […]`** 数组（如 `["asset.read"]`）。
- 共享链接创建返回含 `key`（base64url，非 UUID）、`slug:null`、`showMetadata`；
  公开访问 `GET /api/share/:key`（无认证）。
- `GET /api/api-keys/me` → `403 {"message":"Not authenticated with an API Key"}`
  （普通 JWT 调用是 403 而非 404）。
- `GET /api/admin/maintenance/detect-install` 返回各存储目录的
  `readable/writable/files` 清单。
- 会话列表每项：`{id, createdAt, updatedAt, current, appVersion, deviceOS, deviceType, isPendingSyncReset}`。

---

## 4. 端点实测总表（253/254）

> 状态码为实测值；同一端点多次请求时列出全部（如 `200×2`）。
> 「—」表示该端点仅以异常路径命中（详见证据 JSON）。

| 方法 | 路径 | 实测状态 | 响应/备注 |
|---|---|---|---|

#### Activities

| 方法 | 路径 | 实测状态 | 响应/备注 |
|---|---|---|---|
| GET | `/activities` | 200×1 | [0 items] |
| POST | `/activities` | 400×1 | message, errors |
| GET | `/activities/statistics` | 200×1 | comments, likes |
| DELETE | `/activities/{id}` | 400×1 | message |

#### Authentication (admin)

| 方法 | 路径 | 实测状态 | 响应/备注 |
|---|---|---|---|
| POST | `/admin/auth/unlink-all` | 204×1 |  |

#### Database Backups (admin)

| 方法 | 路径 | 实测状态 | 响应/备注 |
|---|---|---|---|
| DELETE | `/admin/database-backups` | 200×1 |  |
| GET | `/admin/database-backups` | 200×1 | backups |
| POST | `/admin/database-backups/start-restore` | 400×1 | message |
| POST | `/admin/database-backups/upload` | 500×1 | message |
| GET | `/admin/database-backups/{filename}` | 404×1 | message |

#### Maintenance (admin)

| 方法 | 路径 | 实测状态 | 响应/备注 |
|---|---|---|---|
| GET | `/admin/integrity/report` | 400×1 | message, errors |
| DELETE | `/admin/integrity/report/{id}` | 400×1 | message, errors |
| GET | `/admin/integrity/report/{id}/file` | 400×1 | message, errors |
| GET | `/admin/integrity/report/{type}/csv` | 400×1 | message, errors |
| GET | `/admin/integrity/summary` | 200×1 | untracked_file, missing_file, checksum_mismatch |
| POST | `/admin/maintenance` | 201×1 |  |
| GET | `/admin/maintenance/detect-install` | 200×1 | storage |
| POST | `/admin/maintenance/login` | 400×1 | message |
| GET | `/admin/maintenance/status` | 200×1 | active, action |

#### Notifications (admin)

| 方法 | 路径 | 实测状态 | 响应/备注 |
|---|---|---|---|
| POST | `/admin/notifications` | 500×1 | message |
| POST | `/admin/notifications/templates/{name}` | 200×1 | name, html |
| POST | `/admin/notifications/test-email` | 400×1 | message |

#### Users (admin)

| 方法 | 路径 | 实测状态 | 响应/备注 |
|---|---|---|---|
| GET | `/admin/users` | 200×1 | [2 items] |
| POST | `/admin/users` | 400×1 | message |
| DELETE | `/admin/users/{id}` | 403×1 | message |
| GET | `/admin/users/{id}` | 200×1 | id, email, name, profileImagePath, avatarColor, profileChangedAt |
| PUT | `/admin/users/{id}` | 400×1 | message, errors |
| GET | `/admin/users/{id}/calendar-heatmap` | 200×1 | from, to, series, totalCount |
| GET | `/admin/users/{id}/preferences` | 200×1 | albums, folders, memories, people, sharedLinks, ratings |
| PUT | `/admin/users/{id}/preferences` | 200×1 | albums, folders, memories, people, sharedLinks, ratings |
| POST | `/admin/users/{id}/restore` | 200×1 | id, email, name, profileImagePath, avatarColor, profileChangedAt |
| GET | `/admin/users/{id}/sessions` | 200×1 | [2 items] |
| GET | `/admin/users/{id}/statistics` | 200×1 | images, videos, total |

#### Albums

| 方法 | 路径 | 实测状态 | 响应/备注 |
|---|---|---|---|
| GET | `/albums` | 200×1 | [7 items] |
| POST | `/albums` | 201×1 | albumName, description, albumThumbnailAssetId, createdAt, updatedAt, i |
| PUT | `/albums/assets` | 200×1 | success |
| GET | `/albums/statistics` | 200×1 | owned, shared, notShared |
| DELETE | `/albums/{id}` | 400×1 | message |
| GET | `/albums/{id}` | 200×2 | owned, shared, notShared |
| PATCH | `/albums/{id}` | 200×1 | albumName, description, albumThumbnailAssetId, createdAt, updatedAt, i |
| DELETE | `/albums/{id}/assets` | 400×1 | message |
| PUT | `/albums/{id}/assets` | 200×1 | [2 items] |
| GET | `/albums/{id}/map-markers` | 400×1 | message |
| DELETE | `/albums/{id}/user/{userId}` | 400×1 | message |
| PUT | `/albums/{id}/user/{userId}` | 400×1 | message |
| PUT | `/albums/{id}/users` | 400×1 | message, errors |

#### API keys

| 方法 | 路径 | 实测状态 | 响应/备注 |
|---|---|---|---|
| GET | `/api-keys` | 200×1 | [2 items] |
| POST | `/api-keys` | 201×1 | secret, apiKey |
| GET | `/api-keys/me` | 403×1 | message |
| DELETE | `/api-keys/{id}` | 400×1 | message |
| GET | `/api-keys/{id}` | 400×1 403×1 | message |
| PUT | `/api-keys/{id}` | 400×1 | message, errors |

#### Assets

| 方法 | 路径 | 实测状态 | 响应/备注 |
|---|---|---|---|
| DELETE | `/assets` | 204×1 |  |
| POST | `/assets` | 200×2 201×2 | status, id |
| PUT | `/assets` | 204×2 |  |
| POST | `/assets/bulk-upload-check` | 500×1 | message |
| PUT | `/assets/copy` | 400×1 | message |
| POST | `/assets/jobs` | 204×1 |  |
| DELETE | `/assets/metadata` | 204×1 |  |
| PUT | `/assets/metadata` | 500×1 | message |
| GET | `/assets/statistics` | 200×1 | images, videos, total |
| GET | `/assets/{id}` | 200×3 400×2 | message, errors |
| PUT | `/assets/{id}` | 400×2 500×1 | message |
| DELETE | `/assets/{id}/edits` | 400×1 | message |
| GET | `/assets/{id}/edits` | 400×1 | message |
| PUT | `/assets/{id}/edits` | 400×1 | message, errors |
| GET | `/assets/{id}/metadata` | 400×1 | message |
| PUT | `/assets/{id}/metadata` | 400×1 | message |
| DELETE | `/assets/{id}/metadata/{key}` | 400×1 | message |
| GET | `/assets/{id}/metadata/{key}` | 400×1 | message |
| GET | `/assets/{id}/ocr` | 400×1 | message |
| GET | `/assets/{id}/original` | 200×1 | �PNG



IHDR��=�2	pHYs��sR |
| GET | `/assets/{id}/thumbnail` | 200×1 | RIFFVWEBPVP8X
0��ICCP��lcms mntrRGB XY |
| GET | `/assets/{id}/video/playback` | 404×1 | message |
| GET | `/assets/{id}/video/stream/main.m3u8` | 400×1 | message |
| DELETE | `/assets/{id}/video/stream/{sessionId}` | 400×1 | message |
| GET | `/assets/{id}/video/stream/{sessionId}/{variantIndex}/playlist.m3u8` | 400×1 | message, errors |
| GET | `/assets/{id}/video/stream/{sessionId}/{variantIndex}/{filename}` | 400×2 | message, errors |

#### Authentication

| 方法 | 路径 | 实测状态 | 响应/备注 |
|---|---|---|---|
| POST | `/auth/admin-sign-up` | 400×2 | message |
| POST | `/auth/change-password` | 200×1 | id, email, name, profileImagePath, avatarColor, profileChangedAt |
| POST | `/auth/login` | 201×3 | accessToken, userId, userEmail, name, isAdmin, profileImagePath |
| POST | `/auth/logout` | 200×1 | successful, redirectUri |
| DELETE | `/auth/pin-code` | 204×1 401×1 |  |
| POST | `/auth/pin-code` | 204×1 |  |
| PUT | `/auth/pin-code` | 400×1 | message, errors |
| POST | `/auth/session/lock` | 204×1 |  |
| POST | `/auth/session/unlock` | 204×1 |  |
| GET | `/auth/status` | 200×1 | pinCode, password, isElevated, pinExpiresAt |
| POST | `/auth/validateToken` | 200×1 | authStatus |
| POST | `/oauth/authorize` | 400×1 | message |
| POST | `/oauth/backchannel-logout` | 400×1 | message, errors |
| POST | `/oauth/callback` | 400×1 | message |
| POST | `/oauth/link` | 500×1 | message |
| GET | `/oauth/mobile-redirect` | 0×1 | ERR fetch failed |
| POST | `/oauth/unlink` | 200×1 | id, email, name, profileImagePath, avatarColor, profileChangedAt |

#### Download

| 方法 | 路径 | 实测状态 | 响应/备注 |
|---|---|---|---|
| POST | `/download/archive` | 500×1 | message |
| POST | `/download/info` | 201×1 | totalSize, archives |

#### Duplicates

| 方法 | 路径 | 实测状态 | 响应/备注 |
|---|---|---|---|
| DELETE | `/duplicates` | 204×1 |  |
| GET | `/duplicates` | 200×1 | [0 items] |
| POST | `/duplicates/resolve` | 400×1 | message, errors |
| DELETE | `/duplicates/{id}` | 400×1 | message |

#### Faces

| 方法 | 路径 | 实测状态 | 响应/备注 |
|---|---|---|---|
| GET | `/faces` | 400×1 | message, errors |
| POST | `/faces` | 400×1 | message |
| DELETE | `/faces/{id}` | 400×1 | message |
| PUT | `/faces/{id}` | 400×1 | message |

#### Jobs

| 方法 | 路径 | 实测状态 | 响应/备注 |
|---|---|---|---|
| GET | `/jobs` | 200×1 | thumbnailGeneration, metadataExtraction, videoConversion, faceDetectio |
| POST | `/jobs` | 204×1 |  |
| PUT | `/jobs/{name}` | 200×1 | queueStatus, jobCounts |

#### Libraries

| 方法 | 路径 | 实测状态 | 响应/备注 |
|---|---|---|---|
| GET | `/libraries` | 200×1 | [0 items] |
| POST | `/libraries` | 400×1 | message, errors |
| DELETE | `/libraries/{id}` | 400×1 | message |
| GET | `/libraries/{id}` | 400×1 | message |
| PUT | `/libraries/{id}` | 400×1 | message |
| POST | `/libraries/{id}/scan` | 400×1 | message |
| GET | `/libraries/{id}/statistics` | 400×1 | message |
| POST | `/libraries/{id}/validate` | 200×1 | importPaths |

#### Map

| 方法 | 路径 | 实测状态 | 响应/备注 |
|---|---|---|---|
| GET | `/map/markers` | 200×1 | [0 items] |
| GET | `/map/reverse-geocode` | 200×1 | [1 items] |

#### Memories

| 方法 | 路径 | 实测状态 | 响应/备注 |
|---|---|---|---|
| GET | `/memories` | 200×1 | [1 items] |
| POST | `/memories` | 201×1 | id, createdAt, updatedAt, memoryAt, ownerId, type |
| GET | `/memories/statistics` | 200×1 | total |
| DELETE | `/memories/{id}` | 400×1 | message |
| GET | `/memories/{id}` | 200×2 | total |
| PUT | `/memories/{id}` | 200×1 | id, createdAt, updatedAt, memoryAt, ownerId, type |
| DELETE | `/memories/{id}/assets` | 400×1 | message |
| PUT | `/memories/{id}/assets` | 200×1 | [1 items] |

#### Notifications

| 方法 | 路径 | 实测状态 | 响应/备注 |
|---|---|---|---|
| DELETE | `/notifications` | 400×1 | message, errors |
| GET | `/notifications` | 200×1 | [0 items] |
| PUT | `/notifications` | 400×1 | message, errors |
| DELETE | `/notifications/{id}` | 400×1 | message |
| GET | `/notifications/{id}` | 400×1 | message |
| PUT | `/notifications/{id}` | 400×1 | message |

#### Partners

| 方法 | 路径 | 实测状态 | 响应/备注 |
|---|---|---|---|
| GET | `/partners` | 400×2 | message, errors |
| POST | `/partners` | 500×1 | message |
| DELETE | `/partners/{id}` | 204×1 |  |
| POST | `/partners/{id}` | 201×1 | id, email, name, profileImagePath, avatarColor, profileChangedAt |
| PUT | `/partners/{id}` | 200×1 | id, email, name, profileImagePath, avatarColor, profileChangedAt |

#### People

| 方法 | 路径 | 实测状态 | 响应/备注 |
|---|---|---|---|
| DELETE | `/people` | 204×1 |  |
| GET | `/people` | 200×1 | people, hasNextPage, total, hidden |
| POST | `/people` | 400×1 | message, errors |
| PUT | `/people` | 200×1 | [0 items] |
| DELETE | `/people/{id}` | 400×1 | message |
| GET | `/people/{id}` | 400×1 | message, errors |
| PUT | `/people/{id}` | 400×1 | message, errors |
| POST | `/people/{id}/merge` | 400×1 | message |
| PUT | `/people/{id}/reassign` | 400×1 | message |
| GET | `/people/{id}/statistics` | 400×1 | message |
| GET | `/people/{id}/thumbnail` | 404×1 | message |

#### Plugins

| 方法 | 路径 | 实测状态 | 响应/备注 |
|---|---|---|---|
| GET | `/plugins` | 200×1 | [1 items] |
| GET | `/plugins/methods` | 200×1 | [12 items] |
| GET | `/plugins/templates` | 200×1 | [3 items] |
| GET | `/plugins/{id}` | 200×2 400×1 | message |

#### Queues

| 方法 | 路径 | 实测状态 | 响应/备注 |
|---|---|---|---|
| GET | `/queues` | 200×1 | [19 items] |
| GET | `/queues/{name}` | 400×1 | message, errors |
| PUT | `/queues/{name}` | 400×1 | message, errors |
| DELETE | `/queues/{name}/jobs` | 400×1 | message, errors |
| GET | `/queues/{name}/jobs` | 400×1 | message, errors |

#### Search

| 方法 | 路径 | 实测状态 | 响应/备注 |
|---|---|---|---|
| GET | `/search/cities` | 200×1 | [1 items] |
| GET | `/search/explore` | 200×1 | [2 items] |
| POST | `/search/large-assets` | 200×1 | [0 items] |
| POST | `/search/metadata` | 200×1 | albums, assets |
| GET | `/search/person` | 400×1 | message, errors |
| GET | `/search/places` | 200×1 | [20 items] |
| POST | `/search/random` | 200×1 | [0 items] |
| POST | `/search/smart` | 400×1 | message |
| POST | `/search/statistics` | 200×1 | total |
| GET | `/search/suggestions` | 400×1 | message, errors |

#### Server

| 方法 | 路径 | 实测状态 | 响应/备注 |
|---|---|---|---|
| GET | `/server/about` | 401×1 | message |
| GET | `/server/apk-links` | 200×1 | arm64v8a, armeabiv7a, universal, x86_64 |
| GET | `/server/config` | 200×1 | loginPageMessage, trashDays, userDeleteDelay, oauthButtonText, isIniti |
| GET | `/server/features` | 200×1 | smartSearch, facialRecognition, duplicateDetection, map, reverseGeocod |
| DELETE | `/server/license` | 204×1 |  |
| GET | `/server/license` | 404×1 | message |
| PUT | `/server/license` | 400×1 | message, errors |
| GET | `/server/media-types` | 200×1 | video, image, sidecar |
| GET | `/server/ping` | 200×1 | res |
| GET | `/server/statistics` | 401×1 | message |
| GET | `/server/storage` | 200×1 | diskAvailable, diskSize, diskUse, diskAvailableRaw, diskSizeRaw, diskU |
| GET | `/server/version` | 200×1 | major, minor, patch, prerelease |
| GET | `/server/version-check` | 200×1 | checkedAt, releaseVersion |
| GET | `/server/version-history` | 200×1 | [1 items] |

#### Sessions

| 方法 | 路径 | 实测状态 | 响应/备注 |
|---|---|---|---|
| DELETE | `/sessions` | 204×1 |  |
| GET | `/sessions` | 200×1 | [2 items] |
| POST | `/sessions` | 201×1 | id, createdAt, updatedAt, expiresAt, current, appVersion |
| DELETE | `/sessions/{id}` | 400×1 | message |
| PUT | `/sessions/{id}` | 400×1 | message |
| POST | `/sessions/{id}/lock` | 400×1 | message |

#### Shared links

| 方法 | 路径 | 实测状态 | 响应/备注 |
|---|---|---|---|
| GET | `/shared-links` | 200×1 | [7 items] |
| POST | `/shared-links` | 201×1 | id, description, password, userId, key, type |
| POST | `/shared-links/login` | 403×1 | message |
| GET | `/shared-links/me` | 403×1 | message |
| DELETE | `/shared-links/{id}` | 400×1 | message |
| GET | `/shared-links/{id}` | 200×1 403×1 | id, description, password, userId, key, type |
| PATCH | `/shared-links/{id}` | 200×1 | id, description, password, userId, key, type |
| DELETE | `/shared-links/{id}/assets` | 400×1 | message |
| PUT | `/shared-links/{id}/assets` | 400×1 | message |

#### Stacks

| 方法 | 路径 | 实测状态 | 响应/备注 |
|---|---|---|---|
| DELETE | `/stacks` | 500×1 | message |
| GET | `/stacks` | 200×1 | [1 items] |
| POST | `/stacks` | 201×1 | id, primaryAssetId, assets |
| DELETE | `/stacks/{id}` | 400×1 | message |
| GET | `/stacks/{id}` | 200×1 | id, primaryAssetId, assets |
| PUT | `/stacks/{id}` | 200×1 | id, primaryAssetId, assets |
| DELETE | `/stacks/{id}/assets/{assetId}` | 400×1 | message |

#### Sync

| 方法 | 路径 | 实测状态 | 响应/备注 |
|---|---|---|---|
| DELETE | `/sync/ack` | 500×1 | message |
| GET | `/sync/ack` | 200×1 | [0 items] |
| POST | `/sync/ack` | 500×1 | message |
| POST | `/sync/stream` | 400×1 | message, errors |

#### System config

| 方法 | 路径 | 实测状态 | 响应/备注 |
|---|---|---|---|
| GET | `/system-config` | 200×1 | backup, ffmpeg, logging, machineLearning, map, newVersionCheck |
| PUT | `/system-config` | 400×1 | message, errors |
| GET | `/system-config/defaults` | 200×1 | backup, ffmpeg, integrityChecks, job, logging, machineLearning |
| GET | `/system-config/storage-template-options` | 200×1 | secondOptions, minuteOptions, dayOptions, weekOptions, hourOptions, ye |

#### System metadata

| 方法 | 路径 | 实测状态 | 响应/备注 |
|---|---|---|---|
| GET | `/system-metadata/admin-onboarding` | 200×1 | isOnboarded |
| POST | `/system-metadata/admin-onboarding` | 204×1 |  |
| GET | `/system-metadata/reverse-geocoding-state` | 200×1 | lastUpdate, lastImportFileName |
| GET | `/system-metadata/version-check-state` | 200×1 | checkedAt, releaseVersion |

#### Tags

| 方法 | 路径 | 实测状态 | 响应/备注 |
|---|---|---|---|
| GET | `/tags` | 200×1 | [1 items] |
| POST | `/tags` | 400×1 | message |
| PUT | `/tags` | 200×1 | [0 items] |
| PUT | `/tags/assets` | 200×1 | count |
| DELETE | `/tags/{id}` | 400×1 | message |
| GET | `/tags/{id}` | 400×1 | message |
| PUT | `/tags/{id}` | 200×1 400×1 | message, errors |
| DELETE | `/tags/{id}/assets` | 400×1 | message |
| PUT | `/tags/{id}/assets` | 400×1 | message |

#### Timeline

| 方法 | 路径 | 实测状态 | 响应/备注 |
|---|---|---|---|
| GET | `/timeline/bucket` | 200×2 | duration, id, visibility, isFavorite, isImage, isTrashed |
| GET | `/timeline/buckets` | 200×1 | [3 items] |

#### Trash

| 方法 | 路径 | 实测状态 | 响应/备注 |
|---|---|---|---|
| POST | `/trash/empty` | 200×1 | count |
| POST | `/trash/restore` | 200×1 | count |
| POST | `/trash/restore/assets` | 200×1 | count |

#### Users

| 方法 | 路径 | 实测状态 | 响应/备注 |
|---|---|---|---|
| GET | `/users` | 200×1 | [2 items] |
| GET | `/users/me` | 200×2 | id, email, name, profileImagePath, avatarColor, profileChangedAt |
| PUT | `/users/me` | 400×1 | message, errors |
| GET | `/users/me/calendar-heatmap` | 200×1 | from, to, series, totalCount |
| DELETE | `/users/me/license` | 204×1 |  |
| GET | `/users/me/license` | 404×1 | message |
| PUT | `/users/me/license` | 400×1 | message, errors |
| DELETE | `/users/me/onboarding` | 204×1 |  |
| GET | `/users/me/onboarding` | 200×1 | isOnboarded |
| PUT | `/users/me/onboarding` | 200×1 | isOnboarded |
| GET | `/users/me/preferences` | 200×1 | albums, folders, memories, people, sharedLinks, ratings |
| PUT | `/users/me/preferences` | 200×1 | albums, folders, memories, people, sharedLinks, ratings |
| DELETE | `/users/profile-image` | 400×1 | message |
| POST | `/users/profile-image` | 500×1 | message |
| GET | `/users/{id}` | 200×2 | id, email, name, profileImagePath, avatarColor, profileChangedAt |
| GET | `/users/{id}/profile-image` | 404×1 | message |

#### Views

| 方法 | 路径 | 实测状态 | 响应/备注 |
|---|---|---|---|
| GET | `/view/folder` | 200×1 | [1 items] |
| GET | `/view/folder/unique-paths` | 200×1 | [3 items] |

#### Workflows

| 方法 | 路径 | 实测状态 | 响应/备注 |
|---|---|---|---|
| GET | `/workflows` | 200×1 | [3 items] |
| POST | `/workflows` | 201×1 | id, enabled, trigger, name, description, createdAt |
| GET | `/workflows/triggers` | 200×1 | [2 items] |
| DELETE | `/workflows/{id}` | 400×1 | message |
| GET | `/workflows/{id}` | 200×1 400×1 | message |
| PUT | `/workflows/{id}` | 400×1 | message |
| GET | `/workflows/{id}/share` | 400×1 | message |

---

## 5. 对 immich-go 的行动建议

1. **`AssetResponseDto` 对齐**：补 `owner`（嵌套）、`resized`、`width`、
   `height`、`isEdited`、`hasMetadata`、`duplicateId`；`visibility` 与
   `isArchived` 并存且语义一致；**不要**下发 `thumbnailPath/previewPath`。
2. **`duration` 改为数字秒**（上传与读取一致）。
3. **`GET /timeline/buckets` 的 `timeBucket` 用 `YYYY-MM-DD`**。
4. **`changePassword` 返回 200 + UserAdminResponseDto**，并使会话失效；
   `updateNotifications` 返回 204 且支持 `{ids, readAt}`；admin 用户更新
   按可选字段语义（缺失字段不改）。✅ 本次已同步修复。
5. **管理员失去权限返回 403**（immich-go 目前 401），错误消息
   `{"error":"admin privileges required","statusCode":403}`。
6. `GET /api-keys/me` 对非 API-Key 调用返回 **403**。
7. 评估是否跟随官方的 500 行为（规则 7 vs 对等性）：建议**保持诚实 4xx**，
   但在本报告与 GAP_ANALYSIS 中记录差异，避免误判为回归。
8. `/server/theme`、`/users/me/statistics` 在官方 v3.1.0 不存在——若 immich-go
   实现了，属于超集（保留但记录）。

---

## 6. 复现方法

```bash
# 1) 启动原版栈（ghcr.io 直连可达；docker.io 不可达）
podman network create probe_net
podman run -d --name probe_redis --network probe_net ghcr.io/valkey-io/valkey:9
podman run -d --name probe_db --network probe_net \
  -e POSTGRES_PASSWORD=postgres -e POSTGRES_USER=postgres -e POSTGRES_DB=immich \
  -e POSTGRES_INITDB_ARGS='--data-checksums' --shm-size=128mb \
  -v probe_pgdata:/var/lib/postgresql/data \
  ghcr.io/immich-app/postgres:14-vectorchord0.4.3-pgvectors0.2.0
podman run -d --name probe_server --network probe_net -p 2283:2283 \
  -e DB_HOSTNAME=probe_db -e DB_USERNAME=postgres -e DB_PASSWORD=postgres \
  -e DB_DATABASE_NAME=immich -e REDIS_HOSTNAME=probe_redis \
  -e IMMICH_MACHINE_LEARNING_ENABLED=false -v probe_upload:/data \
  ghcr.io/immich-app/immich-server:v3.1.0

# 2) 探测（在 Windows 侧把 harness/规范/测试图拷进容器后运行；
#    注意 Git Bash 需要 MSYS_NO_PATHCONV=1，否则容器内路径会被改写）
podman cp scripts/probe_original_api.mjs probe_server:/tmp/probe.mjs
podman cp open-api/immich-openapi-specs.json probe_server:/tmp/spec.json
podman exec probe_server node /tmp/probe.mjs
podman cp probe_server:/tmp/probe-out .tmp/probe-final
```

证据文件：`immich-v3.1.0-live-probe-data/{probe-results,dto-samples,coverage}.json`。
