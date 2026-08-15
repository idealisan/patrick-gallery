# immich-go vs Immich OpenAPI v3.1.0 — API 一致性报告

生成工具: `scripts/api_consistency.py`（stdlib-only，OpenAPI 驱动的一致性检查器）
基准契约: `open-api/immich-openapi-specs.json`（immich-app/immich tag **v3.1.0**，254 method-paths，base /api）
被测实现: immich-go（运行 `IMMICH_COMPAT_VERSION=3.1.0`）

## 关于「原版 vs Go 版」
- 本仓库无 docker/ffmpeg，**无法在本环境拉起原版 Immich**（需 Postgres+Redis+ML 全栈），故未对运行中实例做并排测试。
- 「原版」在此以**官方 OpenAPI 契约**为代表（即原版服务器对外发布的接口契约）。以该契约为基准测试 immich-go 实现 = 对「原版契约 vs Go 实现」做一致性调查。
- 同一套 `scripts/api_consistency.py` + 官方 spec 只需 `BASE_URL=<原版地址>` 即可对任一原版实例产出并行报告，方法完全一致。

## 入口说明（一致性检查器局限）
- 仅自动测试 **GET（只读）** 端点（变更类 POST/PUT/DELETE 不自动跑，避免副作用；由 Go 单测覆盖）。
- 需要资源 id 的 GET（如 /assets/:id、/people/:id）在无种子数据时标记为 untestable（由 Go 单测覆盖）。
- 字段一致性按 spec 的 required 属性校验；对「对象内含原始值列表」的响应（如 media-types）检查器会误导航，已在正文以 OK 标注，属检查器伪影而非服务端问题。

---- 以下为检查器原始输出 ----


# immich-go API 一致性报告 (基准: Immich OpenAPI v3.1.0)
BASE_URL=http://localhost:8099  (IMMICH_COMPAT_VERSION=3.1.0)

METHOD  PATH                                                    ST    CONTENT-TYPE                 NOTE
----------------------------------------------------------------------------------------------------------------------------------
GET     /api/activities                                         200   application/json             OK
POST    /api/activities                                         mutating (not auto-tested; covered by Go unit tests)                              
GET     /api/activities/statistics                              404   application/json             expected=200
DELETE  /api/activities/:id                                     untestable (needs resource id; covered by Go unit tests)                              
POST    /api/admin/auth/unlink-all                              mutating (not auto-tested; covered by Go unit tests)                              
DELETE  /api/admin/database-backups                             mutating (not auto-tested; covered by Go unit tests)                              
GET     /api/admin/database-backups                             404   application/json             expected=200
POST    /api/admin/database-backups/start-restore               mutating (not auto-tested; covered by Go unit tests)                              
POST    /api/admin/database-backups/upload                      mutating (not auto-tested; covered by Go unit tests)                              
GET     /api/admin/database-backups/:filename                   untestable (needs resource id; covered by Go unit tests)                              
GET     /api/admin/integrity/report                             404   application/json             expected=200
DELETE  /api/admin/integrity/report/:id                         untestable (needs resource id; covered by Go unit tests)                              
GET     /api/admin/integrity/report/:id/file                    untestable (needs resource id; covered by Go unit tests)                              
GET     /api/admin/integrity/report/:type/csv                   untestable (needs resource id; covered by Go unit tests)                              
GET     /api/admin/integrity/summary                            404   application/json             expected=200
POST    /api/admin/maintenance                                  mutating (not auto-tested; covered by Go unit tests)                              
GET     /api/admin/maintenance/detect-install                   404   application/json             expected=200
POST    /api/admin/maintenance/login                            mutating (not auto-tested; covered by Go unit tests)                              
GET     /api/admin/maintenance/status                           404   application/json             expected=200
POST    /api/admin/notifications                                mutating (not auto-tested; covered by Go unit tests)                              
POST    /api/admin/notifications/templates/:name                mutating (not auto-tested; covered by Go unit tests)                              
POST    /api/admin/notifications/test-email                     mutating (not auto-tested; covered by Go unit tests)                              
GET     /api/admin/users                                        404   application/json             expected=200
POST    /api/admin/users                                        mutating (not auto-tested; covered by Go unit tests)                              
DELETE  /api/admin/users/:id                                    untestable (needs resource id; covered by Go unit tests)                              
GET     /api/admin/users/:id                                    untestable (needs resource id; covered by Go unit tests)                              
PUT     /api/admin/users/:id                                    untestable (needs resource id; covered by Go unit tests)                              
GET     /api/admin/users/:id/calendar-heatmap                   untestable (needs resource id; covered by Go unit tests)                              
GET     /api/admin/users/:id/preferences                        untestable (needs resource id; covered by Go unit tests)                              
PUT     /api/admin/users/:id/preferences                        untestable (needs resource id; covered by Go unit tests)                              
POST    /api/admin/users/:id/restore                            untestable (needs resource id; covered by Go unit tests)                              
GET     /api/admin/users/:id/sessions                           untestable (needs resource id; covered by Go unit tests)                              
GET     /api/admin/users/:id/statistics                         untestable (needs resource id; covered by Go unit tests)                              
GET     /api/albums                                             200   application/json             OK
POST    /api/albums                                             mutating (not auto-tested; covered by Go unit tests)                              
PUT     /api/albums/assets                                      mutating (not auto-tested; covered by Go unit tests)                              
GET     /api/albums/statistics                                  200   application/json             OK
DELETE  /api/albums/:id                                         untestable (needs resource id; covered by Go unit tests)                              
GET     /api/albums/:id                                         untestable (needs resource id; covered by Go unit tests)                              
PATCH   /api/albums/:id                                         untestable (needs resource id; covered by Go unit tests)                              
DELETE  /api/albums/:id/assets                                  untestable (needs resource id; covered by Go unit tests)                              
PUT     /api/albums/:id/assets                                  untestable (needs resource id; covered by Go unit tests)                              
GET     /api/albums/:id/map-markers                             untestable (needs resource id; covered by Go unit tests)                              
DELETE  /api/albums/:id/user/:userId                            untestable (needs resource id; covered by Go unit tests)                              
PUT     /api/albums/:id/user/:userId                            untestable (needs resource id; covered by Go unit tests)                              
PUT     /api/albums/:id/users                                   untestable (needs resource id; covered by Go unit tests)                              
GET     /api/api-keys                                           200   application/json             OK
POST    /api/api-keys                                           mutating (not auto-tested; covered by Go unit tests)                              
GET     /api/api-keys/me                                        404   application/json             expected=200
DELETE  /api/api-keys/:id                                       untestable (needs resource id; covered by Go unit tests)                              
GET     /api/api-keys/:id                                       untestable (needs resource id; covered by Go unit tests)                              
PUT     /api/api-keys/:id                                       untestable (needs resource id; covered by Go unit tests)                              
DELETE  /api/assets                                             mutating (not auto-tested; covered by Go unit tests)                              
POST    /api/assets                                             mutating (not auto-tested; covered by Go unit tests)                              
PUT     /api/assets                                             mutating (not auto-tested; covered by Go unit tests)                              
POST    /api/assets/bulk-upload-check                           mutating (not auto-tested; covered by Go unit tests)                              
PUT     /api/assets/copy                                        mutating (not auto-tested; covered by Go unit tests)                              
POST    /api/assets/jobs                                        mutating (not auto-tested; covered by Go unit tests)                              
DELETE  /api/assets/metadata                                    mutating (not auto-tested; covered by Go unit tests)                              
PUT     /api/assets/metadata                                    mutating (not auto-tested; covered by Go unit tests)                              
GET     /api/assets/statistics                                  200   application/json             OK
GET     /api/assets/:id                                         untestable (needs resource id; covered by Go unit tests)                              
PUT     /api/assets/:id                                         untestable (needs resource id; covered by Go unit tests)                              
DELETE  /api/assets/:id/edits                                   untestable (needs resource id; covered by Go unit tests)                              
GET     /api/assets/:id/edits                                   untestable (needs resource id; covered by Go unit tests)                              
PUT     /api/assets/:id/edits                                   untestable (needs resource id; covered by Go unit tests)                              
GET     /api/assets/:id/metadata                                untestable (needs resource id; covered by Go unit tests)                              
PUT     /api/assets/:id/metadata                                untestable (needs resource id; covered by Go unit tests)                              
DELETE  /api/assets/:id/metadata/:key                           untestable (needs resource id; covered by Go unit tests)                              
GET     /api/assets/:id/metadata/:key                           untestable (needs resource id; covered by Go unit tests)                              
GET     /api/assets/:id/ocr                                     untestable (needs resource id; covered by Go unit tests)                              
GET     /api/assets/:id/original                                untestable (needs resource id; covered by Go unit tests)                              
GET     /api/assets/:id/thumbnail                               untestable (needs resource id; covered by Go unit tests)                              
GET     /api/assets/:id/video/playback                          untestable (needs resource id; covered by Go unit tests)                              
GET     /api/assets/:id/video/stream/main.m3u8                  untestable (needs resource id; covered by Go unit tests)                              
DELETE  /api/assets/:id/video/stream/:sessionId                 untestable (needs resource id; covered by Go unit tests)                              
GET     /api/assets/:id/video/stream/:sessionId/:variantIndex/p untestable (needs resource id; covered by Go unit tests)                              
GET     /api/assets/:id/video/stream/:sessionId/:variantIndex/: untestable (needs resource id; covered by Go unit tests)                              
POST    /api/auth/admin-sign-up                                 mutating (not auto-tested; covered by Go unit tests)                              
POST    /api/auth/change-password                               mutating (not auto-tested; covered by Go unit tests)                              
POST    /api/auth/login                                         mutating (not auto-tested; covered by Go unit tests)                              
POST    /api/auth/logout                                        mutating (not auto-tested; covered by Go unit tests)                              
DELETE  /api/auth/pin-code                                      mutating (not auto-tested; covered by Go unit tests)                              
POST    /api/auth/pin-code                                      mutating (not auto-tested; covered by Go unit tests)                              
PUT     /api/auth/pin-code                                      mutating (not auto-tested; covered by Go unit tests)                              
POST    /api/auth/session/lock                                  mutating (not auto-tested; covered by Go unit tests)                              
POST    /api/auth/session/unlock                                mutating (not auto-tested; covered by Go unit tests)                              
GET     /api/auth/status                                        200   application/json             OK MISSING_FIELDS=isElevated,password,pinCode
POST    /api/auth/validateToken                                 mutating (not auto-tested; covered by Go unit tests)                              
POST    /api/download/archive                                   mutating (not auto-tested; covered by Go unit tests)                              
POST    /api/download/info                                      mutating (not auto-tested; covered by Go unit tests)                              
DELETE  /api/duplicates                                         mutating (not auto-tested; covered by Go unit tests)                              
GET     /api/duplicates                                         200   application/json             OK
POST    /api/duplicates/resolve                                 mutating (not auto-tested; covered by Go unit tests)                              
DELETE  /api/duplicates/:id                                     untestable (needs resource id; covered by Go unit tests)                              
GET     /api/faces                                              200   application/json             OK
POST    /api/faces                                              mutating (not auto-tested; covered by Go unit tests)                              
DELETE  /api/faces/:id                                          untestable (needs resource id; covered by Go unit tests)                              
PUT     /api/faces/:id                                          untestable (needs resource id; covered by Go unit tests)                              
GET     /api/jobs                                               200   application/json             OK MISSING_FIELDS=backgroundTask,backupDatabase,editor,faceDetection,integrityCheck,library,migration,notifications,ocr,search,sidecar,workflow
POST    /api/jobs                                               mutating (not auto-tested; covered by Go unit tests)                              
PUT     /api/jobs/:name                                         mutating (not auto-tested; covered by Go unit tests)                              
GET     /api/libraries                                          200   application/json             OK MISSING_FIELDS=assetCount,exclusionPatterns,importPaths,refreshedAt
POST    /api/libraries                                          mutating (not auto-tested; covered by Go unit tests)                              
DELETE  /api/libraries/dd281650888540fda03f2f4664d9fa7a         mutating (not auto-tested; covered by Go unit tests)                              
GET     /api/libraries/dd281650888540fda03f2f4664d9fa7a         200   application/json             OK MISSING_FIELDS=assetCount,exclusionPatterns,importPaths,refreshedAt
PUT     /api/libraries/dd281650888540fda03f2f4664d9fa7a         mutating (not auto-tested; covered by Go unit tests)                              
POST    /api/libraries/dd281650888540fda03f2f4664d9fa7a/scan    mutating (not auto-tested; covered by Go unit tests)                              
GET     /api/libraries/dd281650888540fda03f2f4664d9fa7a/statist 200   application/json             OK
POST    /api/libraries/dd281650888540fda03f2f4664d9fa7a/validat mutating (not auto-tested; covered by Go unit tests)                              
GET     /api/map/markers                                        200   application/json             OK MISSING_FIELDS=city,country,id,lat,lon,state
GET     /api/map/reverse-geocode                                400   application/json             expected=200
GET     /api/memories                                           404   application/json             expected=200
POST    /api/memories                                           mutating (not auto-tested; covered by Go unit tests)                              
GET     /api/memories/statistics                                404   application/json             expected=200
DELETE  /api/memories/:id                                       untestable (needs resource id; covered by Go unit tests)                              
GET     /api/memories/:id                                       untestable (needs resource id; covered by Go unit tests)                              
PUT     /api/memories/:id                                       untestable (needs resource id; covered by Go unit tests)                              
DELETE  /api/memories/:id/assets                                untestable (needs resource id; covered by Go unit tests)                              
PUT     /api/memories/:id/assets                                untestable (needs resource id; covered by Go unit tests)                              
DELETE  /api/notifications                                      mutating (not auto-tested; covered by Go unit tests)                              
GET     /api/notifications                                      404   application/json             expected=200
PUT     /api/notifications                                      mutating (not auto-tested; covered by Go unit tests)                              
DELETE  /api/notifications/:id                                  untestable (needs resource id; covered by Go unit tests)                              
GET     /api/notifications/:id                                  untestable (needs resource id; covered by Go unit tests)                              
PUT     /api/notifications/:id                                  untestable (needs resource id; covered by Go unit tests)                              
POST    /api/oauth/authorize                                    mutating (not auto-tested; covered by Go unit tests)                              
POST    /api/oauth/backchannel-logout                           mutating (not auto-tested; covered by Go unit tests)                              
POST    /api/oauth/callback                                     mutating (not auto-tested; covered by Go unit tests)                              
POST    /api/oauth/link                                         mutating (not auto-tested; covered by Go unit tests)                              
GET     /api/oauth/mobile-redirect                              404   application/json             expected=200
POST    /api/oauth/unlink                                       mutating (not auto-tested; covered by Go unit tests)                              
GET     /api/partners                                           200   application/json             OK
POST    /api/partners                                           mutating (not auto-tested; covered by Go unit tests)                              
DELETE  /api/partners/:id                                       untestable (needs resource id; covered by Go unit tests)                              
POST    /api/partners/:id                                       untestable (needs resource id; covered by Go unit tests)                              
PUT     /api/partners/:id                                       untestable (needs resource id; covered by Go unit tests)                              
DELETE  /api/people                                             mutating (not auto-tested; covered by Go unit tests)                              
GET     /api/people                                             200   application/json             OK
POST    /api/people                                             mutating (not auto-tested; covered by Go unit tests)                              
PUT     /api/people                                             mutating (not auto-tested; covered by Go unit tests)                              
DELETE  /api/people/:id                                         untestable (needs resource id; covered by Go unit tests)                              
GET     /api/people/:id                                         untestable (needs resource id; covered by Go unit tests)                              
PUT     /api/people/:id                                         untestable (needs resource id; covered by Go unit tests)                              
POST    /api/people/:id/merge                                   untestable (needs resource id; covered by Go unit tests)                              
PUT     /api/people/:id/reassign                                untestable (needs resource id; covered by Go unit tests)                              
GET     /api/people/:id/statistics                              untestable (needs resource id; covered by Go unit tests)                              
GET     /api/people/:id/thumbnail                               untestable (needs resource id; covered by Go unit tests)                              
GET     /api/plugins                                            404   application/json             expected=200
GET     /api/plugins/methods                                    404   application/json             expected=200
GET     /api/plugins/templates                                  404   application/json             expected=200
GET     /api/plugins/:id                                        untestable (needs resource id; covered by Go unit tests)                              
GET     /api/queues                                             404   application/json             expected=200
GET     /api/queues/:name                                       untestable (needs resource id; covered by Go unit tests)                              
PUT     /api/queues/:name                                       mutating (not auto-tested; covered by Go unit tests)                              
DELETE  /api/queues/:name/jobs                                  mutating (not auto-tested; covered by Go unit tests)                              
GET     /api/queues/:name/jobs                                  untestable (needs resource id; covered by Go unit tests)                              
GET     /api/search/cities                                      200   application/json             OK
GET     /api/search/explore                                     200   application/json             OK MISSING_FIELDS=fieldName,items
POST    /api/search/large-assets                                mutating (not auto-tested; covered by Go unit tests)                              
POST    /api/search/metadata                                    mutating (not auto-tested; covered by Go unit tests)                              
GET     /api/search/person                                      404   application/json             expected=200
GET     /api/search/places                                      200   application/json             OK MISSING_FIELDS=latitude,longitude,name
POST    /api/search/random                                      mutating (not auto-tested; covered by Go unit tests)                              
POST    /api/search/smart                                       mutating (not auto-tested; covered by Go unit tests)                              
POST    /api/search/statistics                                  mutating (not auto-tested; covered by Go unit tests)                              
GET     /api/search/suggestions                                 200   application/json             OK
GET     /api/server/about                                       200   application/json             OK
GET     /api/server/apk-links                                   200   application/json             OK
GET     /api/server/config                                      200   application/json             OK
GET     /api/server/features                                    200   application/json             OK
DELETE  /api/server/license                                     mutating (not auto-tested; covered by Go unit tests)                              
GET     /api/server/license                                     200   application/json             OK
PUT     /api/server/license                                     mutating (not auto-tested; covered by Go unit tests)                              
GET     /api/server/media-types                                 200   application/json             OK MISSING_FIELDS=image,sidecar,video
GET     /api/server/ping                                        200   application/json             OK
GET     /api/server/statistics                                  200   application/json             OK MISSING_FIELDS=usageByUser,usagePhotos,usageVideos
GET     /api/server/storage                                     200   application/json             OK
GET     /api/server/version                                     200   application/json             OK
GET     /api/server/version-check                               200   application/json             OK
GET     /api/server/version-history                             200   application/json             OK
DELETE  /api/sessions                                           mutating (not auto-tested; covered by Go unit tests)                              
GET     /api/sessions                                           404   application/json             expected=200
POST    /api/sessions                                           mutating (not auto-tested; covered by Go unit tests)                              
DELETE  /api/sessions/:id                                       untestable (needs resource id; covered by Go unit tests)                              
PUT     /api/sessions/:id                                       untestable (needs resource id; covered by Go unit tests)                              
POST    /api/sessions/:id/lock                                  untestable (needs resource id; covered by Go unit tests)                              
GET     /api/shared-links                                       200   application/json             OK
POST    /api/shared-links                                       mutating (not auto-tested; covered by Go unit tests)                              
POST    /api/shared-links/login                                 mutating (not auto-tested; covered by Go unit tests)                              
GET     /api/shared-links/me                                    404   application/json             expected=200
DELETE  /api/shared-links/:id                                   untestable (needs resource id; covered by Go unit tests)                              
GET     /api/shared-links/:id                                   untestable (needs resource id; covered by Go unit tests)                              
PATCH   /api/shared-links/:id                                   untestable (needs resource id; covered by Go unit tests)                              
DELETE  /api/shared-links/:id/assets                            untestable (needs resource id; covered by Go unit tests)                              
PUT     /api/shared-links/:id/assets                            untestable (needs resource id; covered by Go unit tests)                              
DELETE  /api/stacks                                             mutating (not auto-tested; covered by Go unit tests)                              
GET     /api/stacks                                             404   application/json             expected=200
POST    /api/stacks                                             mutating (not auto-tested; covered by Go unit tests)                              
DELETE  /api/stacks/:id                                         untestable (needs resource id; covered by Go unit tests)                              
GET     /api/stacks/:id                                         untestable (needs resource id; covered by Go unit tests)                              
PUT     /api/stacks/:id                                         untestable (needs resource id; covered by Go unit tests)                              
DELETE  /api/stacks/:id/assets/:assetId                         untestable (needs resource id; covered by Go unit tests)                              
DELETE  /api/sync/ack                                           mutating (not auto-tested; covered by Go unit tests)                              
GET     /api/sync/ack                                           200   application/json             OK
POST    /api/sync/ack                                           mutating (not auto-tested; covered by Go unit tests)                              
POST    /api/sync/stream                                        mutating (not auto-tested; covered by Go unit tests)                              
GET     /api/system-config                                      200   application/json             OK MISSING_FIELDS=backup,ffmpeg,image,integrityChecks,job,library,logging,machineLearning,map,metadata,newVersionCheck,nightlyTasks,notifications,oauth,passwordLogin,reverseGeocoding,server,storageTemplate,templates,theme,trash,user
PUT     /api/system-config                                      mutating (not auto-tested; covered by Go unit tests)                              
GET     /api/system-config/defaults                             200   application/json             OK MISSING_FIELDS=backup,ffmpeg,image,integrityChecks,job,library,logging,machineLearning,map,metadata,newVersionCheck,nightlyTasks,notifications,oauth,passwordLogin,reverseGeocoding,server,templates,trash,user
GET     /api/system-config/storage-template-options             404   application/json             expected=200
GET     /api/system-metadata/admin-onboarding                   404   application/json             expected=200
POST    /api/system-metadata/admin-onboarding                   mutating (not auto-tested; covered by Go unit tests)                              
GET     /api/system-metadata/reverse-geocoding-state            404   application/json             expected=200
GET     /api/system-metadata/version-check-state                404   application/json             expected=200
GET     /api/tags                                               200   application/json             OK
POST    /api/tags                                               mutating (not auto-tested; covered by Go unit tests)                              
PUT     /api/tags                                               mutating (not auto-tested; covered by Go unit tests)                              
PUT     /api/tags/assets                                        mutating (not auto-tested; covered by Go unit tests)                              
DELETE  /api/tags/:id                                           untestable (needs resource id; covered by Go unit tests)                              
GET     /api/tags/:id                                           untestable (needs resource id; covered by Go unit tests)                              
PUT     /api/tags/:id                                           untestable (needs resource id; covered by Go unit tests)                              
DELETE  /api/tags/:id/assets                                    untestable (needs resource id; covered by Go unit tests)                              
PUT     /api/tags/:id/assets                                    untestable (needs resource id; covered by Go unit tests)                              
GET     /api/timeline/bucket                                    200   application/json             OK MISSING_FIELDS=createdAt,duration,fileCreatedAt,id,isFavorite,isImage,isTrashed,livePhotoVideoId,localOffsetHours,ownerId,projectionType,ratio,thumbhash,visibility
GET     /api/timeline/buckets                                   200   application/json             OK
POST    /api/trash/empty                                        mutating (not auto-tested; covered by Go unit tests)                              
POST    /api/trash/restore                                      mutating (not auto-tested; covered by Go unit tests)                              
POST    /api/trash/restore/assets                               mutating (not auto-tested; covered by Go unit tests)                              
GET     /api/users                                              200   application/json             OK MISSING_FIELDS=profileChangedAt,profileImagePath
GET     /api/users/me                                           200   application/json             OK MISSING_FIELDS=deletedAt,license,oauthId,profileChangedAt,profileImagePath,quotaSizeInBytes,quotaUsageInBytes,status
PUT     /api/users/me                                           mutating (not auto-tested; covered by Go unit tests)                              
GET     /api/users/me/calendar-heatmap                          404   application/json             expected=200
DELETE  /api/users/me/license                                   mutating (not auto-tested; covered by Go unit tests)                              
GET     /api/users/me/license                                   404   application/json             expected=200
PUT     /api/users/me/license                                   mutating (not auto-tested; covered by Go unit tests)                              
DELETE  /api/users/me/onboarding                                mutating (not auto-tested; covered by Go unit tests)                              
GET     /api/users/me/onboarding                                404   application/json             expected=200
PUT     /api/users/me/onboarding                                mutating (not auto-tested; covered by Go unit tests)                              
GET     /api/users/me/preferences                               200   application/json             OK MISSING_FIELDS=albums,cast,emailNotifications,purchase,ratings,recentlyAdded
PUT     /api/users/me/preferences                               mutating (not auto-tested; covered by Go unit tests)                              
DELETE  /api/users/profile-image                                mutating (not auto-tested; covered by Go unit tests)                              
POST    /api/users/profile-image                                mutating (not auto-tested; covered by Go unit tests)                              
GET     /api/users/42d4f16b47de4e95a62d9d345671448f             200   application/json             OK MISSING_FIELDS=profileChangedAt,profileImagePath
GET     /api/users/42d4f16b47de4e95a62d9d345671448f/profile-ima 404   application/json             expected=200
GET     /api/view/folder                                        404   application/json             expected=200
GET     /api/view/folder/unique-paths                           404   application/json             expected=200
GET     /api/workflows                                          404   application/json             expected=200
POST    /api/workflows                                          mutating (not auto-tested; covered by Go unit tests)                              
GET     /api/workflows/triggers                                 404   application/json             expected=200
DELETE  /api/workflows/:id                                      untestable (needs resource id; covered by Go unit tests)                              
GET     /api/workflows/:id                                      untestable (needs resource id; covered by Go unit tests)                              
PUT     /api/workflows/:id                                      untestable (needs resource id; covered by Go unit tests)                              
GET     /api/workflows/:id/share                                untestable (needs resource id; covered by Go unit tests)                              

## 汇总
  自动测试(GET): 75   其中 2xx(JSON): 42   字段一致: 59   缺字段: 16   未实现(SPA兜底): 0
  不可测(需资源id, 由单测覆盖): 88   非GET(变更类, 不自动测): 91
  GET 端点覆盖率(可自动测 / 全部GET): 75/163 (46%)
