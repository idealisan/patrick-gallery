# immich-go 与原版 Immich 差距复盘（Gap Analysis）

> **目标口径（2026-08-15 明确）**：本项目的目标是为**手机 APP** 提供与原版 Immich **一致的 API 与功能**，并为 **Web UI** 提供与原版**一致的界面与功能**。即**功能对等**，不只是「单人核心闭环」。
>
> 因此，本复盘不再把「多用户 / ML / 管理后台 / 通知 / OAuth」等标为「超出范围」——在「对等」目标下，它们都是**必须补齐的差距**，区别只在于实现成本与受 `AGENTS.md` 硬规则的约束程度。
>
> 所有结论均基于 `internal/app/app.go` 实际路由、各 handler 实现、`internal/video`、`internal/store`、`internal/app/models.go`，并**以官方客户端与网页版实际代码**（immich 仓库 `server/`、`web/`、`packages/sdk/`，tag **v3.1.0**）为权威逐项核对；`open-api/immich-openapi-specs.json`（254 operation）仅作路径/方法清单参考，不作为"为准"的最终依据。
> 配套状态见 [STATUS.md](../STATUS.md)；契约门禁见 [CONTRACT_TESTING.md](CONTRACT_TESTING.md)；原版并排对比 harness 见 [SIDE_BY_SIDE.md](SIDE_BY_SIDE.md)。

---

## 0. 一句话结论

immich-go 目前只覆盖了原版的**单人核心闭环**。要对等原版的**手机 APP + Web UI**，差距远大于此前「核心闭环」视角所见：

- **API 端点覆盖 117 / 254（≈46%）**，137 个 operation 缺失；
- 缺失项按「客户端面」归类后，**手机 APP 对等就差一大块**（人物聚类、回忆、堆叠、PIN/会话锁、通知、语义搜索、完整 sync），**Web UI 对等还额外差一整块管理后台**（用户管理、维护、备份恢复、系统元数据、OAuth 配置、插件、工作流、队列、通知管理）；
- 即便已实现的端点，**功能深度也不对等**（人物为空、视频仅单码率、分享链接字段未持久化、`visibility` 不持久化、Partner 共享资产未透出等）；
- 完全对等意味着要在 `AGENTS.md` 约束（纯 Go / 无 CGO / SQLite 单用户 / 进程内视频）内或附加约束下，重做 ML 推理、多用户、管理运维、通知、OAuth 等——这是量级很大的工程，文档第 12 节显式标注每块的**约束内可行性**。

### 0.1 本次具体目标聚焦（单人个人管理 + 跨端同步）

用户明确：本项目的**即时目标**是——**单个用户用自己的手机 APP 和网页，管理与同步自己的相册，必须可用**。这比「全功能对等」更小、更具体：

- ✅ **已满足**（详见 §14 核验）：照片/视频备份上传、去重、时间线/相册/搜索/地图浏览、收藏/归档/回收站、分享链接、库扫描、作业、实时事件推送（websocket + Socket.IO）。核心媒体闭环对单人个人场景**已真正可用且 v3.1.0 契约兼容**。
- ⚠️ **唯一开箱 blocker（已修）**：`config.go` 默认 `IMMICH_COMPAT_VERSION=1.130.0` 与仓库内置契约 v3.1.0 不一致，会导致客户端报「版本不匹配」。已改为默认 `3.1.0`，开箱即与已验证契约对齐。
- 🔍 **需用真实客户端验证的一项**（2026-09-23 修订）：`/sync/stream` 已是**真实增量快照**（`internal/app/compat_v3.go:handleSyncStream`，按用户emit AuthUserV1/UserV1/AssetV2/AlbumV2/AlbumUserV1/AlbumToAssetV1 + SyncCompleteV1，jsonlines 格式，`SyncState` 持久化 ack，`compat_v3_test.go`/`android_contract_test.go` 覆盖），不再是 stub；主流程靠 REST + 实时事件即可刷新，首次全量校正走该端点，仍建议用真实 APP 实测确认。
- ❌ **不在本即时目标内（归入 P3 / 显式延后）**：ML（人物/CLIP/OCR）、OAuth/SSO、通知、插件/工作流、Memories、水平扩展。这些不影响「单人个人管理+同步」。（注：PIN/设备会话锁、`/sessions`、手动 Stacks、资产 edits、资料图、伙伴共享写操作等手机 APP 必需端点已在 v3.1.0 契约下补齐并实现。）
- ✅ **多用户已转入活动阶段**：用户/账户管理后台 `/admin/users/*` 已实现（见 §13 P2-15）；伙伴/相册内用户共享的关系模型已存在。下一阶段把共享资产透出到 timeline/search、补相册内共享前端入口与按用户资源隔离（见 §13 多用户路线），均在纯 Go / SQLite 单实例约束内推进。

> 一句话：达成「单人个人管理+同步可用」所需的代码，基本已经写好并通过核验；剩下的只是把默认版本对齐、并用真机走一遍同步。

---

## 1. 覆盖范围与数字（精确 diff）

把**官方客户端/网页版实际代码**（以及 `open-api/immich-openapi-specs.json` 作为路径/方法清单参考）的 254 个 operation 的 `method + 路径` 归一化（`{id}`→`:id`）后，与从 `internal/app/app.go` 解析出的路由逐条匹配：

- **SPEC 总计：254**
- **方法+路径精确匹配：117（≈46%）**
- **缺失：137**

> ⚠️ **计数已过时（2026-09-23 标注）**：117/137 是 §13 P2-15（`/admin/users/*` 11 端点）与 §14.3（system-config 持久化、queues、maintenance/integrity、SQLite 备份、`POST /assets/jobs`、伙伴透出/相册共享鉴权、memories、notifications、stacks 手动端点、PIN/会话锁、`/albums/:id/map-markers`、`POST /download/archive`）落地**之前**的数字。下表 §1.1 中 `Users (admin)`、`Memories`、`Stacks`、`Notifications`、`Sessions` 等行的「缺失」数，以及 §2.1/§2.2/§4–§6/§10/§14.1 中对应的「全缺 / 仍是 stub」表述，均以 §13/§14.3 为准（已实现）。精确新计数待重跑 §15 复现脚本后更新。

### 1.1 缺失端点按 tag 分布

| Tag | 缺失 | 客户端面（对等所需） |
|-----|------|----------------------|
| Authentication | 12 | 手机 PIN/会话锁 + Web OAuth 配置（两端都要） |
| Users (admin) | 0（原 11，§13 P2-15 已全部实现） | ✅ 已实现（`/admin/users/*` 全部 real，见 §13 P2-15） |
| Assets | 11 | 两端：元数据编辑 / 复制 / edits / OCR / 资产级 job |
| Users | 10 | 两端：资料图、license、onboarding、calendar-heatmap |
| Maintenance (admin) | 9 | Web 管理后台维护/完整性 |
| Memories | 8 | **手机 + Web 都有「回忆/On this day」** |
| Stacks | 7 | **手机 + Web 都有连拍/相似堆叠** |
| Workflows | 7 | Web（较新功能） |
| Notifications | 6 | **手机接收通知 + Web 通知管理** |
| People | 6 | **手机 + Web 都有「人物」页（聚类 ML）** |
| Sessions | 6 | 手机 PIN/会话锁要用 |
| Database Backups (admin) | 5 | Web 管理后台备份恢复（SQLite 不适用 PG 方式） |
| Albums | 5 | 两端：相册内用户共享 / 批量 |
| Queues | 5 | Web 任务队列可视化 |
| Search | 5 | 两端：large-assets/random/statistics + smart(ML)/person(ML) |
| Shared links | 5 | 两端：查看/登录/资产增删 |
| Plugins | 4 | Web（较新功能） |
| System metadata | 4 | Web onboarding/状态端点 |
| Tags | 4 | 两端：批量标签操作 |
| Notifications (admin) | 3 | Web 测试邮件 / 模板 |
| API keys | 3 | 两端：单 key 读取/`me`/更新 |
| Faces | 3* | **手机 + Web 人脸（ML）** |
| Jobs | 2 | 方法差异（两端 job 状态） |
| Partners | 2 | 两端：伙伴共享变体 |
| Views | 2 | **Web 文件夹视图** |
| Activities | 1 | 两端：统计 |
| Authentication (admin) | 1 | Web 解绑 OAuth |
| Download | 1 | `POST /download/archive`（immich-go 用 GET 变体） |
| Libraries | 1 | 两端：`/libraries/:id/validate` |
| Server | 1 | `DELETE /server/license` |
| Sync | 1 | **手机完整双向同步（当前 stub）** |
| System config | 1 | 存储模板选项 |

> *`Faces` 4 个 operation 中 `GET /faces`、`GET /faces/:id` 为优雅 stub，写操作缺失。

---

## 2. 差距按「客户端面」归类（对等视角）

### 2.1 手机 APP 对等所需（当前缺失或弱化）

手机 Immich 实际会用到、且现状不达对的项：

| 功能 | 原版手机行为 | immich-go 现状 | 差距 |
|------|--------------|----------------|------|
| 人物 / 人脸 | 「人物」Tab，自动聚类、可改名、合并、隐藏 | `GET /people` 返回空；人脸写操作全缺；`facialRecognition:false` | **完全缺失（ML）** |
| 回忆 Memories | 「On this day」时间线 | `GET /api/memories` 已实现（按月/日分组，见 §14.3）；其余 7 个 memories 端点仍缺 | ⚠️ 基础可用，完整套件缺 |
| 堆叠 Stacks | 连拍/相似自动堆叠、可展开 | ✅ 手动堆叠已实现（`POST /stacks`、`GET /stacks/:id`、`DELETE /stacks`，真实建表 + 资产顺序透出）；缺列表/合并与 ML 自动聚类 | ⚠️ 手动堆叠可用，自动堆叠缺（ML） |
| 语义搜索 | 「人物/地点/事物」facets（CLIP 向量） | `smartSearch` 已用 OpenAI 兼容 Chat Completions 视觉接口为 image 生成文本描述（`POST /jobs smartSearch` 后台异步写入 `asset_mls.description`/`labels_json`）；`/search/smart` 以子串匹配 描述+文件名+OCR 文本 返回真实结果，未配 LLM 时降级为纯文件名/OCR 文本搜索（不再 501）。 | ✅ 已实现（LLM 文本描述路线，非 CLIP 向量；需 `IMMICH_LLM_*` 配置） |
| 搜索人物 | `/search/person` | ✅ 按 personId/名称检索其资产（`asset.person_id`，见 `NO_STUBS.md` A#14）；人物自动聚类仍缺（ML） | ⚠️ 检索可用，聚类缺 |
| 设备锁 / PIN | `auth/pin-code`、`auth/session/lock|unlock`、`/sessions/*` | ✅ 已实现（real：PIN 用 bcrypt 存储并校验、`/sessions` 创建子会话带 token、lock 清 `pin_expires_at`、unlock 校验 PIN/密码后置 `pin_expires_at+15min`） | ✅ 已实现（手机必备，v3.1.0 契约） |
| 通知 | 站内通知、`/notifications/*` | 设备令牌 `POST/DELETE /api/notifications` + `POST /admin/notifications`（含 `test-email` 真实发信）已实现（见 §14.3）；无外部推送服务 | ⚠️ 站内可用，推送缺（设计内） |
| 完整同步 | `/sync/stream` 双向增量 | ✅ 真实快照（`handleSyncStream` + `SyncState` 持久化 ack，jsonlines；仅 partners/stacks/memories/people/faces/ocr 等无数据类不 emit，客户端容忍缺席） | ✅ 可用（建议真机复核） |
| 相册内用户共享 | 把相册共享给指定用户 | ✅ `PUT /albums/:id/users`、`PUT/DELETE /albums/:id/user/:userId` 已实现并按 owner/editor/viewer 鉴权（`app.go:372-374`，见 §13 P0-5） | ✅ 可用 |
| 伙伴共享资产 | 伙伴的照片出现在自己时间线 | ✅ 时间线 `withPartners` + 搜索 join 伙伴已实现（`timeline.go`，见 §13 P0-4） | ✅ 可用 |
| 地图瓦片 | 真实地图（样式 URL） | 离线网格，无瓦片 | **弱化** |
| 视频自适应码率 | 多质量 HLS | 单变体单分辨率 | **弱化** |
| OAuth 登录 | 若服务端启用 OAuth，手机走 `/oauth/*` | 全缺 | 取决于部署 |

### 2.2 Web UI 对等所需（在手机之上额外缺）

Web 管理后台 + 高级界面缺失：

| 功能 | 原版 Web | immich-go 现状 | 差距 |
|------|----------|----------------|------|
| 用户管理后台 | `/admin/users/*` | ✅ 11 端点全部已实现（real，admin 角色守护，见 §13 P2-15） | ✅ 无差距 |
| 维护 / 完整性 | `/admin/maintenance/*`、`/admin/integrity/*` | 全缺 | **完全缺失** |
| 数据库备份恢复 | `/admin/database-backups/*` | 全缺（SQLite 只能拷文件） | **完全缺失（架构）** |
| 系统元数据状态 | `/system-metadata/*` | 全缺 | **缺失** |
| OAuth/SSO 配置 UI | `/oauth/*` + admin | 全缺 | **缺失** |
| 插件 Plugins | `/plugins/*` | 4 端点全缺 | **缺失** |
| 工作流 Workflows | `/workflows/*` | 7 端点全缺 | **缺失** |
| 任务队列可视化 | `/queues/*` | 全缺 | **缺失** |
| 通知管理 | `/admin/notifications/*` | 全缺 | **缺失** |
| 文件夹视图 | `/view/folder`、`/view/folder/unique-paths` | 全缺 | **缺失（Web 专属）** |
| 设置 / 账户 / License | users/license/onboarding/calendar-heatmap | 部分缺 | **部分缺失** |
| 管理面板整体 | 作业、系统配置、存储、主题、关于 | 极简 | **大幅弱化** |

### 2.3 两端共有（已覆盖但深度不对等）

见第 4 节（功能深度）。

---

## 3. 架构层差距（对等的根本阻碍）

| 维度 | 原版 Immich | immich-go | 对等影响 |
|------|-------------|-----------|----------|
| 形态 | 微服务：NestJS server + Python ML（TensorRT/CLIP/OCR）+ Postgres(pgvecto-rs)+ Redis | 单 Go 二进制 + SQLite | ML/多用户/缓存需另寻纯 Go 路径 |
| 存储 | 存储模板、外部库离线、S3/对象存储 | 仅本地磁盘；无 S3、无存储模板 | Web 存储管理页无法对等 |
| 向量/AI | Postgres+pgvecto-rs 存 CLIP 向量；Python 推理 | 无向量库、无推理；`smartSearch/facialRecognition/ocr=false` | 语义搜索/人物/ OCR 全缺 |
| 多实例 | 无状态 server + 外部 DB/Redis 可水平扩展 | 单机单进程；SQLite 单写者串行 | 多用户并发写受限 |
| DB 抽象 | — | `store.Store` 接口有，但**仅 SQLite 实现** | Postgres 后端未落地 |

> 注：单体化本身不违反对等目标，但「ML/多用户/运维」能力必须在这套约束内以纯 Go 方式重建，或显式引入受控的外部组件——见第 12 节。

---

## 4. 已实现端点的功能深度差距（stub vs 真实）

### 4.1 人物 / 人脸（People/Faces）
- `GET /people` 返回空（无聚类，`facialRecognition:false`）。`Person` 表无自动创建路径。
- `GET /people/:id/assets` 空形状；`/search/person` 返回 0。
- 人脸写操作（`POST/PUT/DELETE /faces`、`POST/PUT/DELETE /people`、缩略图）全缺。
- 影响：官方 App/Web 的「人物」页为空。

### 4.2 视频（HLS / 转码）
- HLS 为**单变体单分辨率**（一次转码 MP4 包装成 master→variant→单 segment，`internal/app/hls.go`）。原版提供按质量（1080p/720p/480p）自适应码率。
- **OS 原生后端全是 stub**：`internal/video/native_stub.go` 中 `newVideoToolbox/newMediaFoundation/newMediaCodec` 一律返回 `errBackendUnavailable`。`AGENTS.md` 要求的 macOS/Windows/Linux 原生硬件加速**实际未实现**，仅 FFmpeg 库内编码器名探测可用（`ffmpeg.go` `selectEncoder`）。
- 无**按需按 quality 实时转码**（预生成单一 `encoded_video_path`）。
- 无 HEVC 封装、多音轨、animated/webp 动图；抽帧缩略图单帧。

### 4.3 分享链接（Shared links）
- `SharedLink` 模型缺 `allowDownload/allowUpload/description/password/showMetadata/slug/assets` 列。
- 创建响应从请求体回显（STATUS §K 已知限制），**重读时丢失**；密码保护未生效。

### 4.4 资产元数据
- 仅 `GET /assets/:id/metadata` 可读；**无写入端点**。
- `visibility` 枚举（archive/timeline/hidden/locked）由 `isArchived` 推导（`visibilityOf()`，`asset.go`），`hidden`/`locked` 不持久化。
- `duplicateId/isEdited/isOffline` 仅 DTO 返回，库表无列。

### 4.5 实时同步（2026-09-23 已修订：`/sync/stream` 不再是 stub）
- `GET /api/events`（内存总线）+ `Socket.IO`（Engine.IO v4）覆盖主要变更。
- ✅ `/sync/stream`（GET/POST）为**真实增量快照**：`handleSyncStream`（`compat_v3.go:228`）按当前用户 emit AuthUserV1/UserV1/AssetV2/AlbumV2/AlbumUserV1/AlbumToAssetV1，以 SyncCompleteV1 收尾，`Content-Type: application/jsonlines+json`；`/sync/ack`（GET 返回 `SyncAckDto[]`，POST/DELETE 返回 204）持久化 `sync_state`。无数据类（partners/stacks/memories/people/faces/ocr）有意不 emit，客户端容忍缺席。残留：多类型全量双向校正的完备性建议真机复核。

### 4.6 搜索
- 文本/EXIF 搜索可用，facets 最小化；无 CLIP 语义搜索（`smartSearch:false`）、无人物/地点召回（人物为空）。

### 4.7 地图（Map）
- `GET /map/markers` + 离线等距投影网格 + 标记点；`reverseGeocoding:true`。
- **无真实地图瓦片**（原版用 Mapbox/maplibre 样式）；`mapDarkStyleUrl/mapLightStyleUrl` 为空。
- 反向地理编码为 1°×1° 最近城市近似（GeoNames `cities15000`）。
- ✅ `GET /albums/:id/map-markers` 已实现（`album.go` 按 GPS EXIF 过滤，`regression_test.go:TestAlbumMapMarkers` 覆盖）。

---

## 5. 数据模型 / 持久化差距

`internal/app/models.go` 表远少于原版：

**完全缺失的表/实体**（2026-09-23 已按 `models.go`/`db.go` 核对修订）：`Face`/`AssetFace`（人脸检测无 ML 后端，端点诚实 501）、`Workflow`/`Plugin`/`PluginJob`、存储模板相关列。**以下已存在，不再缺失**：`Stack`（`models.go:377`，手动堆叠）、`Session`（`:328`，登录落表）、`Memory`（`:395`）、`Notification`（`:159`，设备令牌）、`UserPreferences` 独立表（`:346`，JSON blob，已纳入 `AutoMigrate`）。

**`Asset` 缺列**：`stackParentId`/`stackId`、`isEdited`、`isOffline`、`visibility` 枚举持久化、`duplicateId`、`originalMimeType`（仅 DTO 计算）、`places`。

**`SharedLink` 缺列**：`allowDownload/allowUpload/description/password(哈希)/showMetadata/slug/assets`。

**`Partner`**：关系行存在，但共享资产未在任何查询透出（timeline/search 不 join partner 资产）——伙伴共享是「空关系」。

---

## 6. 鉴权 / 账户差距

- **仅 JWT**：无刷新令牌轮换、无 OAuth/SSO、无 LDAP。
- 手机 PIN/会话锁已实现：PIN 用 bcrypt 存储校验、`/sessions` 创建子会话带 token、`auth/session/lock|unlock` 控制 `pin_expires_at`（第 2.1 节）——**对等手机体验已打通**；OAuth 登录仍需外部 IdP（延后）。
- **默认 JWT secret 硬编码**：`IMMICH_JWT_SECRET` 默认 `immich-dev-secret-change-me`（`config.go`），生产必须覆盖。
- 无账户锁定/爆破防护、无密码重置邮件（`email:false`）、无注册审批。
- ✅ admin 用户管理 11 端点已实现（`app.go:308-318`，见 §13 P2-15）；仍缺 `auth/admin-sign-up`。
- API key 仅 list/create/delete，缺单 key 读取/更新/`/me`。

---

## 7. 前端（Web UI）差距

> ⚠️ **本节已过时（2026-09-23 标注）**：`internal/webroot/assets/app.js` 手写 vanilla SPA 已不存在（`internal/webroot/` 下仅剩 `webroot.go` + 官方构建产物 `webui/`）。产品 UI 现为**官方 Immich Web 前端 v3.1.0 构建嵌入**（`//go:embed all:webui`，见 `THIRD_PARTY.md` 与 `AGENTS.md` 硬规则 6），下述「相对原版 Web 的缺失」多为手写 SPA 时代的描述，不再适用；Web 差异以官方 Web 实际行为 + 契约测试为准。
>
> 原文归档：手写 SPA（vanilla JS，无构建，~976 行）曾覆盖：时间线、相册、搜索、地图、收藏、归档、回收站、上传、多选批量、管理、灯箱（图片+视频转码播放）、分享。

**相对原版 Web 的缺失**
- 人物 / 人脸 UI（因 ML 缺失，自然没有）
- 回忆 Memories（「On this day」）
- 堆叠 Stack 视图
- 伙伴共享界面（前端无入口）
- OAuth 登录按钮（仅密码登录）
- **完整管理后台**：用户管理、作业、系统配置、存储、OAuth、日志、主题、关于、数据库备份/恢复、维护/完整性、License
- 文件夹视图（`/view/folder`）
- 高级搜索 facets（人物/地点/相机细化）
- 真实地图瓦片（仅离线网格）
- 通知中心 UI

**维护负担**：原版是 React+TS+Vite 持续演进；immich-go 无构建 vanilla SPA 需手动追赶。

---

## 8. 媒体 / 转码差距（汇总 §4.2）
- 仅 h264/mp4；无多分辨率自适应码率（ABR）。
- 无 HEVC 封装、音频单独处理、动图。
- 无按客户端请求 quality 实时转码；抽帧缩略图单帧。
- OS 原生硬件加速后端为 stub。

---

## 9. 运维 / 可观测 / 备份差距
- 无 admin database-backups（PG 备份/恢复）——SQLite 只能停服拷文件。
- 无 integrity report / maintenance mode（`/admin/integrity/*`、`/admin/maintenance/*`）。
- 无 notifications/email（站内信、测试邮件、模板）。
- 无 queues 可视化（`/queues/*`）。
- 无 metrics/Prometheus 端点（仅有 `ping`/`health`）。
- 日志为 `log.Printf` 级别，无结构化日志/分布式追踪。

---

## 10. 版本 / 契约维护风险
- ~~兼容性版本漂移~~ ✅ **已修复（2026-09-23 核对）**：`config.go:106` 默认已为 `IMMICH_COMPAT_VERSION=3.1.0`，与内置 v3.1.0 契约对齐，开箱即过版本门控。
- **客户端快速迭代**：官方 App/Web 升级可能引入新必填字段/端点，需持续追赶——长期维护成本。
- **契约测试覆盖有限**：Schemathesis 仅 `examples` 阶段，实际只覆盖 30/254 operation；`schemathesis-allowlist.txt` 豁免 25 已知未实现 + 3 良性边界。其余 152 缺失端点不在门禁视野内（仅靠本 diff 暴露）。

---

## 11. 性能 / 规模差距
- SQLite 单写者串行化；多用户/高并发写锁等待甚至偶发 500。
- 无缓存层（Redis）；缩略图有 `resize_path` 落盘缓存。
- 无水平扩展 / 多副本 / 读写分离。

---

## 12. 约束张力与可行性评估（关键）

在「对等」目标下，按 `AGENTS.md` 硬规则（纯 Go / 无 CGO / SQLite 单用户 / 进程内视频）评估每块差距的可行性：

| 差距块 | 约束内可行性 | 说明 |
|--------|--------------|------|
| 文本/EXIF 搜索、反向地理编码、时间线、相册、分享、回收站 | ✅ 纯 Go 可做（已做） | 无额外约束 |
| 视频转码/HLS（单码率） | ✅ 纯 Go purego（已做） | 多码率 ABR 也纯 Go 可做，工作量中 |
| 资产元数据写入、`visibility` 持久化、分享链接字段持久化 | ✅ 纯 Go + SQLite 列扩展 | 工作量小–中 |
| 相册内用户共享、伙伴共享资产透出 | ⚠️ SQLite 可做但需多用户模型 | 单人内「家庭共享」可行；严格多租户难 |
| 堆叠 Stacks | ✅ 纯 Go（相似度用感知哈希/尺寸） | 工作量中 |
| 回忆 Memories | ✅ 纯 Go（按日期聚合） | 工作量小 |
| 设备 PIN/会话锁/Sessions | ✅ 纯 Go + SQLite | 工作量中；手机必备 |
| 通知 Notifications（站内） | ✅ 纯 Go + SQLite | email 通知需外部 SMTP（突破纯本地约束） |
| 人脸聚类 / 人物 | ❌ 难（ML 推理） | 纯 Go 无成熟人脸/聚类模型；需外接推理或纯 Go 移植，工程量巨大 |
| CLIP 语义搜索 | ⚠️ 已用 LLM 文本描述路线实现 | 用 OpenAI 兼容视觉接口生成描述 + 子串匹配，无需向量库；CLIP 向量语义召回为更高阶目标（可选） |
| OCR | ❌ 难 | 纯 Go OCR 弱；需模型或外部服务 |
| 多用户管理后台 / 维护 / 备份 / 完整性 | ⚠️ 部分可行 | SQLite 可做用户管理/维护；PG 式备份恢复不适用，需 SQLite 专属方案 |
| OAuth/SSO | ⚠️ 需外部 IdP | 协议纯 Go 可做，但依赖外部身份提供方 |
| 插件 / 工作流 | ❌ 架构级 | 原版插件/工作流是独立子系统，对等成本极高 |
| 水平扩展 / 高并发 | ❌ 与 SQLite 单用户冲突 | 需 Postgres 后端（`store.Store` 已留接口） |

**结论张力**：
- 纯 Go/无 CGO/SQLite 约束下，**「数据类、UI 类、单机运维类」功能基本可对等**；
- **「AI/ML 类」（人物、CLIP、OCR）与「插件/工作流」类**在纯 Go 约束下极难对等，必须决定：① 接受不对等，或 ② 引入纯 Go 可加载的推理库（仍受 AGENTS.md 进程内/无 CLI 约束，可做但工程量巨大），或 ③ 放宽约束引入外部 AI 服务；
- **多用户 / 高并发** 与 SQLite 单用户约束冲突，需落地 Postgres 后端才能对等；
- 这些抉择应回到 `AGENTS.md` 的「Release 目标」与硬规则层面确认——当前 `AGENTS.md` 写的是「Single-user / private-LAN priority，多用户 scaling 显式 out of scope」，与本次明确的「对等」目标存在冲突，建议在文档外另作决策（见第 13 节）。

---

## 13. 建议优先级（对准「手机 + Web 对等」重排）

**P0 — 手机 APP 对等硬缺口（不做则手机体验断档）**
1. ~~设备 PIN / 会话锁 / Sessions（`auth/pin-code`、`auth/session/lock|unlock`、`/sessions/*`）~~ ✅ 已实现（v3.1.0 契约，real）
2. 资产元数据写入（`PUT /assets/:id/metadata` 等）+ `visibility` 持久化
3. 分享链接字段持久化（allowDownload/upload/description/password/slug）+ 重读正确
4. ✅ **伙伴共享资产透出**：时间线 `withPartners`（默认纳入伙伴资产）+ 搜索已 join 伙伴；相册内共享已按角色鉴权。详见 `docs/WEB_ADMIN_UI_ANALYSIS.md` 第 10 节与 `internal/app/timeline.go`/`album.go`。
5. ✅ **相册内用户共享**：`/albums/:id/user/:userId`、`/albums/:id/users` 端点已实现，并按 `albumRole`（owner/editor/viewer）做增删资产与共享管理鉴权，共享相册已出现在对方相册列表。
6. ~~修复兼容版本漂移~~ ✅ 默认 `IMMICH_COMPAT_VERSION=3.1.0` 已与内置契约对齐

**P1 — 手机/Web 共有体验补齐**
7. 回忆 Memories（8 端点）
8. 堆叠 Stacks（7 端点）
9. 站内通知 Notifications（6 端点）+ 前端通知中心
10. 完整同步 `/sync/stream`（双向增量）
11. 搜索 large-assets / random / statistics
12. 存储模板（`/system-config/storage-template-options` + 落盘重命名）
13. 视频多分辨率 ABR（至少 1080p/720p）
14. OS 原生硬件加速后端落地（VideoToolbox/MediaFoundation/MediaCodec via purego）

**P2 — Web 管理后台对等**
15. 用户管理后台（`/admin/users/*`）—— ✅ **已实现（real，非 stub）**：11 个端点全部落地（列出/创建/详情/更新/软删/恢复/偏好读写在 `/admin/users/:id` 下，外加 `calendar-heatmap`/`sessions`/`statistics`）。端点需 admin 角色；`sessions` 在登录/签发 token 时记录；偏好以 JSON blob 持久化。属单人/私域范围内的多用户管理（伙伴、相册内共享用户）。
16. 维护 / 完整性（`/admin/maintenance/*`、`/admin/integrity/*`）
17. 系统元数据状态（`/system-metadata/*`）
18. 任务队列可视化（`/queues/*`）
19. 文件夹视图（`/view/folder`）
20. SQLite 专属备份/恢复方案（替代 PG 式 `/admin/database-backups/*`）

### 多用户功能路线（活动阶段）

账户与权限基座已落地（P2-15 `/admin/users/*`），多用户从「显式延后」转为**当前活动工作流**。在纯 Go / 无 CGO / SQLite 单实例约束内逐步推进；水平多租户扩展仍属 P3（需 Postgres 后端，见 P3-26）。

**已完成（基座）**
- 用户账户管理：`/admin/users/*` 11 端点（CRUD、软删/恢复、偏好、会话、统计、日历热力），受 admin 角色守护。
- 关系模型已存在：`Partner`（伙伴）、`AlbumUser`（相册内用户共享）、`SharedLink`（分享链接）、`ApiKey`（设备/API 令牌）。
- 登录/注册时已记录 `Session`（登录/签发 token 时落 `sessions` 表）。
- `User` 已含 `QuotaSizeInBytes` / `StorageLabel` / `PinCode` 等字段（配额/存储标签的数据层已备）。

**下一阶段（按优先级）**
1. **伙伴共享资产透出**（P0-4）：`Partner` 关系存在，但 timeline/search 未 join 伙伴资产——把伙伴资产按「只读」并入当前用户时间线/搜索，且按 `owner_id` 正确隔离。
2. **相册内用户共享可用化**（P0-5）：`/albums/:id/user/:userId`、`/albums/:id/users` 端点已存在，需补前端入口 + 按共享角色（editor/viewer）做资产/编辑权限校验；共享给指定用户后其时间线能看到该相册。
3. **按用户资源隔离与配额**：确认所有 asset/album/library 查询严格 `owner_id` 作用域（已大部分做到）；实现 `quotaSizeInBytes` 的写入配额校验（当前仅存储、未强制）。
4. **认证加固（可选，按需在 P3 推进）**：刷新令牌轮换、device session 管理（表已备）、PIN 锁——在纯 Go 内可做，不依赖外部组件。

**约束与边界（与 `AGENTS.md` 一致）**
- 不得假设水平分片存储：多用户代码须先在 SQLite 单实例跑通；Postgres 仅用于解除「水平扩展」这一项约束。
- OAuth/SSO（P3-24）、邮件/外部通知（P3-27）依赖外部 IdP/SMTP，纯 Go 协议可做但需外部依赖；在依赖就绪前以诚实 4xx/501 暴露，不得假成功（见 `AGENTS.md` 硬规则 7 与 `NO_STUBS.md`）。
- 所有新增多用户端点必须契约兼容（`schemathesis_check.py` 守护），不得引入 fake-success stub。

**P3 — AI/ML 与架构扩展（受约束，需决策）**
21. 人脸聚类 / 人物（`/people`、`/faces` 写操作）——需纯 Go 推理或外部 AI
22. CLIP 向量语义搜索（`/search/smart` 的高阶召回）——已用 LLM 文本描述 + 子串匹配实现基础版；CLIP 向量语义召回为可选增强
23. OCR（`/assets/:id/ocr`）
24. OAuth/SSO 配置 UI（`/oauth/*` + admin）
25. 插件 / 工作流（`/plugins/*`、`/workflows/*`）
26. Postgres 后端（`store.Store` 已有接口，落地第二实现以支持多用户/高并发）
27. 邮件/外部通知（SMTP）

---

## 14. 核心媒体闭环验证（非 ML / 非多用户功能）

> 用户核心问题：除 ML 相关、以及个性化/多用户功能外，**照片/视频的上传、同步、管理是否真正做好了、真正能用、且与原版完全兼容？**
> 结论：**是——核心媒体闭环已真正可用且与官方 v3.1.0 契约兼容**；残留项均为增强/边缘，且已列入 P0/P1，不构成阻断。下为逐项核验（基于实读 handler + 已跑通的构建/测试）。

### 14.1 核验表（核心媒体功能）

| 功能 | 端点 / 实现 | 契约兼容 | 真正可用 | 残留差距 |
|------|-------------|----------|----------|----------|
| 照片/视频上传 | `POST /assets`（v3.1.0 multipart：`filename`/`fileCreatedAt`/`duration`/`visibility`/`livePhotoVideoId` + 旧 JSON `asset` 回退） | ✅ | ✅ 单测+端到端 | 类型由扩展名+魔数嗅探（与原版一致，不依赖 `assetType`） |
| 去重握手 | `/assets/bulk-upload-check`（checksum 按用户去重）、`/assets/check` | ✅ | ✅ | — |
| 原图 / 缩略图 / 预览 | `/original`、`/thumbnail`、`/thumbnail/:ts`、`/preview`、`/original/download`（强制附件） | ✅ | ✅ | — |
| 视频转码 / HLS | `/encoded-video`（进程内 purego libx264）、`/video/stream/*`（单变体 HLS） | ✅ | ✅ | 仅单分辨率（P1-13） |
| Live Photo | `/live-photo`（优先 encoded-video 否则原文件） | ✅ | ✅ | — |
| 资产元数据读取 | `/metadata`（无 EXIF 返回空对象而非 404） | ✅ | ✅ | 仅读，写入缺失（P0-2） |
| 资产更新 / 批量 / 删除 | `PUT /assets`、`PUT /assets`(bulk)、`DELETE /assets`（`isTrash`/强制删+清文件） | ✅ | ✅ | favorite/archive/trash/exif 位置可改 |
| 收藏 / 归档 | 经 asset update（`isFavorite`/`isArchived`） | ✅ | ✅ | `visibility` 仅 `archive`/`timeline` 持久化（hidden/locked 不持久化） |
| 时间线 | `/timeline/buckets`、`/timeline/bucket`、`/timeline/assets` | ✅ | ✅ | — |
| 相册 | CRUD + 成员增删 + 封面 + 统计 + `/albums/:id/assets` 顺序 | ✅ | ✅ | 相册内用户共享缺（P0-5） |
| 标签 | CRUD + 资产绑定/解绑 | ✅ | ✅ | 批量标签操作小缺（P2） |
| 回收站 | list/restore/empty + 定时清理（`TrashedAt` + 24h 调度） | ✅ | ✅ | — |
| 搜索 | `/search`、`/search/metadata`、`/search/explore`、`/search/suggestions`（文件名+EXIF 文本） | ✅（文本/EXIF） | ✅ | `smart`/`person` 缺（ML） |
| 地图 | `/map/markers`（裸 `array[MapMarkerResponseDto]`）、`/reverse-geocode` | ✅ | ✅ | 无真实瓦片；`/albums/:id/map-markers` 缺 |
| 库磁盘扫描 | `/libraries/:id/scan`（walk `ImportPaths`、`ExcludedPaths` 跳过、扩展名识别、sha1 去重、复用 `ingestStoredFile`）+ `/libraries/:id/validate`（逐个 `os.Stat` 导入路径校验 `isValid`） | ✅ | ✅ | — |
| 作业 | `/jobs`（thumbnailGeneration/metadataExtraction/videoConversion/duplicateDetection 真实执行 + 进度查询；ML job 返回 `unsupported:true`） | ✅ | ✅ | — |
| 分享链接（免登录） | `/share/:key` + `/api/share/:key/{thumbnail,original}/:assetId` | ✅ | ✅ | 字段未持久化（P0-3） |
| 伙伴 | list/create/delete | ✅ 形状 | ⚠️ 共享资产未透出 | P0-4 |
| 活动（评论） | `/activities`（asset/album） | ✅ | ✅ | — |
| 实时同步（事件推送） | `/api/events`（websocket 内存总线）+ `/socket.io`（Engine.IO v4 + Socket.IO，`onAssetUpload/Update/Trash/Delete/Album*` 事件名） | ✅ 协议层 | ✅ | `/sync/stream` 已是真实快照（见 §4.5），残留仅完备性真机复核 |
| 反向地理编码 | `/map/reverse-geocode` + 摄取写入 `Exif.city/country`（GeoNames 离线） | ✅ | ✅ | 1°×1° 最近城市近似 |
| 下载归档 | `/download/archive`（GET + POST 双动词，`app.go:572-573`） | ✅ | ✅ | 无差异（原 GET-only 描述已过时） |

### 14.2 总体结论

- **上传 / 去重 / 管理 / 媒体服务**：已真正可用，且与 v3.1.0 契约在「形状与状态码」层面一致。手机 APP 可完成备份、浏览、播放、收藏、归档、删除、相册、搜索、地图、分享、库扫描等主流程。
- **同步**：实时事件推送（websocket + Socket.IO）已对等，客户端能即时刷新；`/sync/stream` 亦为真实快照（见 §4.5），残留仅完备性真机复核。
- **真正的「硬缺口」不在核心媒体本身，而在账户/共享元数据层**（2026-09-23 修订：其中 PIN/会话锁、伙伴透出、相册内共享、版本对齐均已落地，见 §13 P0-4/P0-5 与 §14.3；剩余为）：资产元数据写入（P0-2）、分享链接字段持久化（P0-3）。

### 14.3 Web 后台（admin）后端实现（2026-08-19 批次）

基于官方 `web/src/routes/admin/*` 与 SDK `packages/sdk/src/fetch-client.ts`（v3.1.0）逐页
梳理 UI 元素→API 工作流，产出清单见 `docs/WEB_ADMIN_UI_ANALYSIS.md`，并真实实现以下端点
（全部非 stub，集成冒烟已通过）：

- **系统配置**：`GET/PUT /system-config` 现持久化完整嵌套 `SystemConfigDto`（JSON blob），
  admin 各设置页可往返保存；`GET /system-config/defaults`、`/system-config/storage-template-options` 已对齐。
- **队列监控**：`GET /queues`、`GET /queues/:name`、`PUT /queues/:name`（isPaused）、
  `GET /queues/:name/jobs`、`DELETE /queues/:name/jobs` —— 19 个命名队列，统计取自真实 job 进度。
- **任务状态**：`GET /jobs` 返回官方 legacy 形状（19 命名队列计数）；`POST /jobs` 触发 manual job；
  `POST /jobs/:name` 支持 `pause/resume/empty/clear-failed` 命令。
- **维护模式 + 完整性**：`POST /admin/maintenance`（enable/disable/restore）、
  `GET /admin/maintenance/status`、`/detect-install`；`GET /admin/integrity/summary` 做真实
  sha1+os.Stat 扫描（checksum_mismatch/missing_file/untracked_file），`/admin/integrity/:type`、
  `/file/:id`、`/csv` 均实现。
- **数据库备份（SQLite 专属）**：`GET /admin/database-backups`、`/start-restore`、
  `DELETE`、`/upload` —— 以 `.db.bak` 快照替代 PG 式备份（已在文档标注为 SQLite 适配，非伪装）。
- **通知**：`POST /admin/notifications` 落库；`POST /admin/notifications/test-email` 用纯 Go
  `net/smtp` 真实发信（STARTTLS/隐式 TLS）。
- **system-metadata**：`/reverse-geocoding-state`、`/version-check-state` 返回契约正确形状。
- **资产级 job**：`POST /assets/jobs` 对指定 assetIds 重跑缩略图/元数据/转码/OCR。
- **多用户共享（P0-4/P0-5）**：时间线纳入伙伴资产（`withPartners`），相册内共享按
  owner/editor/viewer 角色鉴权并透出到对方列表（见上 P0 标注）。

所有新增端点经 `go build`/`go vet`/`go test` 与本地集成冒烟（登录→各 admin 端点 200，
DTO 形状核对）验证通过。

### 14.4 验证证据（2026-08-15 实测）

- `go build ./...` ✅（go1.23.4，CGO 关闭）
- `go vet $(go list ./... | grep -v /internal/video)` ✅（`internal/video` 的 `unsafe.Pointer` FFI 按 AGENTS.md 规则豁免）
- `go test ./...` ✅ —— **修复 2 个过时测试**：`TestMapMarkersReturnsGeoTagged`（`map_test.go`）与 `TestRegression/map-markers`（`regression_test.go`）仍期望旧的 `{markers:[...]}` 包裹，而 `GET /map/markers` 已按 v3.1.0 契约改为返回裸 `array[MapMarkerResponseDto]`（见 STATUS §K.2）。两测试现已改为裸数组解码，**套件转绿**。这说明此前 `STATUS.md` 反复宣称的「`go test ./...` 全绿」并不准确，已在此审计中纠正。
- 契约回归：`STATUS.md` §K 的 Schemathesis（v3.1.0）结果显示 `response_schema_conformance` / `content_type_conformance` / 5xx 零违反（修复的 5 处 DTO 形状：UUID 格式、`/map/markers` 裸数组、`/timeline/bucket`、`/search/*`、`/shared-links`）。

---

## 15. 复现方法

端点 diff 可复现（需 python3 + 仓库契约）：
```bash
python3 - <<'PY'
import json, re
spec = json.load(open("open-api/immich-openapi-specs.json"))
src  = open("internal/app/app.go").read()
impl=set()
for m in ("GET","POST","PUT","PATCH","DELETE"):
    for mm in re.finditer(r'\b(?:r|api)\.'+m+r'\(\s*"([^"]+)"', src):
        p=mm.group(1)
        if p.startswith("/api/"): p=p[4:]
        impl.add((m,p))
spec_ops=set()
for path,item in spec["paths"].items():
    for meth,op in item.items():
        if meth.lower() in ("get","post","put","patch","delete"):
            norm=re.sub(r"\{([^}]+)\}", r":\1", path)
            spec_ops.add((meth.upper(), norm))
missing=[o for o in spec_ops if o not in impl]
print("SPEC:",len(spec_ops)," IMPL:",len(impl)," MISSING:",len(missing))
for o in sorted(missing): print(o)
PY
```

---

*本文为审计文档，不含代码改动。所有「缺失/弱化」项均按「手机 APP + Web UI 功能对等」目标记录，便于后续规划；P3 项受 `AGENTS.md` 硬规则约束，需在项目目标层面决策后实施。*
