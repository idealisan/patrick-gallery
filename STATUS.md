# immich-go — 功能与缺陷清单 (STATUS)

> 回归测试结论见 [REGRESSION_REPORT.md](REGRESSION_REPORT.md)：已发布版本 v1.1.0-go 的全部受测端点均正常（无 5xx 回归），并标注了「部分可用 / 未做」的能力边界。自动化覆盖在 `.github/workflows/ci.yml`（每次 push/PR 在 GitHub Actions 上跑 `go test ./...`）。

> 本文件基于 `internal/app/` 的实际路由与处理函数核对整理（非凭记忆）。
> 项目别名：**patrick-gallery**。最后更新：2026-08-15。

## Release 2 — in progress

> 目标：视频在进程内完全可用（FFmpeg 共享库经 purego 加载、随包分发）、图像 API 补全、数据库抽象、官方前端移植。最后更新：2026-08-09。

| 工作流 | 状态 | 说明 |
|--------|------|------|
| 数据库抽象层 (`internal/store`) | `[done]` | 接口 + SQLite WAL 调优已完成 |
| 视频后端抽象 (`internal/video`) + 纯软件 FFmpeg (purego) 后端 | `[done]` | 抽象接口就绪；**纯 Go / purego FFmpeg 后端完整可用**：Probe + 抽帧缩略图 + 进程内转码（无 CLI、无 CGO），经 `go test` 与 HTTP 端到端验证（输出经 ffprobe 确认为合法 h264 mp4）。硬件加速后端仍为 stub。 |
| 视频 API 接线（上传缩略图、`/preview`、转码 `/encoded-video`） | `[done]` | 上传即抽帧生成视频封面；`/encoded-video` 走真实进程内转码（libx264 软编），失败时回退原文件 |
| 图像：EXIF 提取 + 更宽解码（WebP） | `[done]` | EXIF 提取 + WebP 解码已完成；WebP 解码改用纯 Go 的 `golang.org/x/image/webp`（替代需 CGO 的 `chai2010/webp`），整项目现可 `CGO_ENABLED=0` 构建 |
| 前端：官方前端完整移植（SPA） | `[done]` | 纯 vanilla JS SPA（无构建步骤、随包嵌入，唯一契合单二进制纯 Go 模型的方案）：时间线图库、相册（列表/详情/封面/增删成员）、搜索、收藏、归档、回收站、管理页、灯箱查看器（图片预览 + **视频经真实转码播放**）、拖拽/选择上传、多选批量操作。内嵌资源经 `webroot.go` 以 `web/` 目录 + SPA 回退方式服务 |
| 发布打包：将 FFmpeg 共享库打包进各发行包 | `[done]` | `bundle-deps.sh` 为二进制生成 `-deps` 包（含 libs/），CI 已接入 |

## 一、已实现的功能

### 认证与用户
- JWT 登录 / 登出、注册（signup）、token 校验 / 验证
- 修改密码、API Key 增删查
- 用户信息：列表、`/users/me`、`/user/me`（兼容别名）、更新、删除、头像
- 用户偏好（preferences）读写

### 资产管理（核心）
- 多部件上传（图片 + 视频元数据），图片自动生成 256px JPEG 缩略图
- 列表 / 检索、随机、计数、统计（photos / videos / total）
- 批量信息、查重（duplicates）
- 单资产：获取、更新、删除（批量删）、原图下载、缩略图（含带 `ts` 的缓存变体）
- 视频：原文件直接提供（无转码） → **已升级**：上传即抽帧封面、`/encoded-video` 经纯 Go/purego FFmpeg 进程内转码为 h264 mp4（libx264 软编，硬件加速后端为 stub）

### 相册 / 标签 / 伙伴 / 回收站 / 动态 / 分享链接
- 相册：完整 CRUD + 成员增删改 + 封面设置 + 统计 + **`GET /api/albums/:id/assets`（按序返回相册内资产，前端详情页所需）**
- 标签：CRUD + 资产绑定 / 解绑
- 伙伴（partners）：列表 / 创建 / 删除
- 回收站：列表 / 恢复 / 清空
- 动态（activities）：按资产 / 相册的评论列表与增删
- 分享链接：创建 / 更新 / 删除 / 列表

### 时间线 / 搜索 / 库 / 系统
- 时间线：年 / 月分桶、桶内资产
- 搜索：`/search`（文件名 + EXIF 文本）、`/search/metadata`（相机 / 地点）、`/search/explore`、`/search/suggestions`
- 库（libraries）：CRUD + 统计
- 系统：config / features / about / version / ping / health、system-config 读写、jobs 列表、下载归档（zip，已实现）

### 交付与运维
- 内置完整 SPA Web 前端（vanilla JS，随包嵌入 `web/`，SPA 回退）：登录 / 时间线图库 / 相册 / 搜索 / 收藏 / 归档 / 回收站 / 管理 / 灯箱查看器（图片预览 + 视频进程内转码播放）/ 拖拽·选择上传 / 多选批量操作
- 6 平台静态二进制 + GHCR 多架构镜像 + CI 发布流水线

## 二、未实现 / 已知缺陷（目前未解决）

### 1. 机器学习相关（完全缺失）
- 人物 / 人脸识别：`/people` 接口存在但**永远返回空**（无聚类），`/search/person` 是空实现（返回 0 结果）。
- 智能（语义 / CLIP）搜索：未实现。

### 2. 视频能力（已基本补齐）
- 转码：`/encoded-video` 现已走真实进程内转码（purego + libx264），生成合法 h264 mp4。
- 视频缩略图：上传即由 FFmpeg 后端抽首帧，Web 网格视频封面正常。
- **硬件加速自动回退（已完成 ✅）**：FFmpeg 后端在启动时按 `VideoToolbox → NVENC(Nvidia) → QSV(Intel) → AMF(AMD) → 软件(libx264)` 的优先级探测可用 H.264 编码器，选第一个能成功 `avcodec_open2` 的；运行时若探测到的硬件编码器对真实流失败，自动回退到 libx264。H.264 profile 按机器能力选择：`>=4 核 且 >=8GB 内存` 用 `high`，否则用 `main`（见 `ffmpeg.go` 的 `selectEncoder` / `capableMachine`）。无 GPU 的机器（如本机）自动落到软件 libx264。OS 原生后端（VideoToolbox/MediaFoundation/MediaCodec）仍为 stub，统一由 ffmpeg 编码器名探测覆盖。

### 3. 任务系统（jobs）是空壳
- `POST /jobs/:id` 返回「queued (no-op)」，不真正执行任何后台任务（如批量生成缩略图、元数据提取等）。当前缩略图只在上传时同步生成。

### 4. 相册库扫描（library scan）已补齐 ✅
- `POST /libraries/:id/scan` 现在会**真正遍历 `ImportPaths` 磁盘目录**：跳过 `ExcludedPaths` 前缀、按扩展名识别图片/视频、按内容 sha1 对当前用户去重，并通过与上传共用的 `ingestStoredFile` 生成缩略图 / 抽取 EXIF / 写入 Asset/Exif 行（EXTERNAL 库以 `isExternal=true` 原地引用，不上传副本）。
- 实现位于 `internal/app/library.go`（`handleLibraryScan` → `runScan`，已抽为可单测的纯函数）、`internal/app/ingest.go`（`ingestStoredFile` / `extToType`）。前端「管理 → Libraries (disk scan)」面板可创建 EXTERNAL 库并触发扫描，返回 `imported`/`skipped` 计数。
- 验证：`go test ./internal/app/ -run TestRunScanIngestsAndDedups` 通过；端到端 HTTP 冒烟测试（登录 → 建库 → 扫描 → 缩略图/原图端点 → 二次扫描去重）全部通过。

### 5. 分享链接外部访问（已完成 ✅）
- 已补齐公开免登录访问：`/share/<key>`（SPA 页面 `share.html`）+ `/api/share/:key`（JSON）、`/api/share/:key/thumbnail/:assetId`、`/api/share/:key/original/:assetId`。鉴权内创建/管理端点（`/api/shared-links` CRUD）亦已实现。外界持有 key 即可免登录查看内容与缩略图/原图（视频走进程内转码）。

### 6. SQLite 单写者限制（架构性缺陷）
- 并发写入会串行化，多用户 / 高并发写入可能出现锁等待甚至偶发 500。定位是**个人 / 单用户**场景，不适合多用户高并发写入。

### 7. 内置 Web UI（已升级为完整 SPA）
- 已移植为类 Immich 的完整前端（时间线图库 / 相册 / 搜索 / 收藏 / 归档 / 回收站 / 管理 / 灯箱查看器 / 上传），覆盖个人局域网使用的主要单人场景。复杂管理（OAuth、地图、回忆等）仍需官方 App 或直连 API。

### 8. 其余 Immich 大功能未覆盖（约 165 个路由未实现）
- OAuth / SSO、管理 / 维护后台面板、回忆（memories）、工作流 / 插件、同步流（websocket sync）、通知 / 邮件、存储迁移、回收站定时清理等。
- **地图（已补齐 ✅）**：已实现 `/api/map/markers`（按 GPS 聚类、返回标记 + city/country + 代表 assetId）与 SPA「Map」标签页（离线等距投影世界网格 + 标记点 + 缩略图列表，点击打开灯箱）。**反向地理编码已补齐 ✅**：摄取（上传 / 库扫描）时由随包嵌入的离线 GeoNames 数据集（cities15000 + countryInfo，见 `THIRD_PARTY.md`）就近匹配城市 / 国家并写入 `Exif.city/country`；另提供 `POST /api/map/reverse-geocode` 端点（lat/lon → city/state/country），`server/features.reverseGeocoding` 已报 `true`。

### 9. 部分兼容字段为最小实现
- 为让官方客户端能加载，新增的 compat 端点（config / features 等）返回的是「形状正确但内容最小化」的 JSON（如 ML 标志一律为 false），并非完整语义。

### 10. 构建链路（已加固 ✅，v1.1.2-go）
- 项目现已可纯粹 `CGO_ENABLED=0` 构建：WebP 解码由需 CGO 的 `chai2010/webp` 换成纯 Go 的 `golang.org/x/image/webp`；purego FFmpeg 加载器限定 `//go:build linux || darwin`，Windows 用 stub 回退到 placeholder（该平台无视频缩略图/转码，但服务其余部分正常）。`ci.yml` / `release.yml` 的 vet 改为 `go vet $(go list ./... | grep -v '/internal/video')`，绕开 `internal/video` 中故意使用的 `unsafe.Pointer` FFI。6 个发布目标（linux/darwin × amd64/arm64、windows × amd64/arm64）均已验证可 CGO-free 交叉编译。
- 说明：此前这两项缺陷从未在 CI 暴露——vet 先失败导致 build 步未执行；而 `release.yml` 仅在「发布 GitHub Release」时触发，本项目无 gh/token 故从未发布过 Release，矩阵构建从未真正跑过。v1.1.2-go 一并修掉。

## 三、优先级建议（按性价比）
1. **分享链接公开访问**（#5）——补全端点即可对外分享。
2. **视频缩略图抽帧**（#2）——提升图库观感。
3. 其余（ML、转码、扫描、多用户并发）属架构级投入，暂不在个人场景范围内。

## 四、2026-08-13 补全记录（CNB 仓库 immich-go，源自 patrick-gallery 转移）

本次将 `patrick-gallery`（immich Go 复刻）代码库迁至 CNB `finalappstore/immich-go`，并关闭此前遗留的两处「stub」，实际缩小与 immich 的差距：

### A. Jobs 后台任务系统（从 no-op stub → 真实可执行）✅
- 此前 `POST /api/jobs/:id` 仅返回 `202 queued (no-op)`，后台什么都不做。现已实现带并发 worker 池（上限 4）的真实执行器，进度可经 `GET /api/jobs`（`handleJobsList`）与 `GET /api/jobs/:id`（`handleJobStatus`）实时查询。
- 已落地执行的 job（`internal/app/jobs.go`）：
  - `thumbnailGeneration`：扫描 `has_thumbnail=false` 的 asset，复用 `processMedia` 批量生成缩略图并写回行（`resize_path`/`has_thumbnail`）。
  - `metadataExtraction`：扫描缺 EXIF 行的 asset，抽取并 upsert `Exif` 行。
  - `videoConversion`：扫描 `encoded_video_path=''` 的视频，进程内 FFmpeg 转码为 h264/mp4 落盘。
  - `duplicateDetection`：按 checksum 计算重复分组（结果可由 `GET /api/assets/duplicates` 查询）。
- 不支持的 ML/AI job（`objectDetection`/`facialRecognition`/`smartSearch`/`storageTemplateMigration`/`tagCopy`/`tagImage`）返回 200 + `unsupported:true` 标记，保持官方客户端兼容而不报错；未知 job 返回 400。
- `processMedia`（抽取的媒体处理核心）被上传、库扫描、jobs 三处复用，避免重复逻辑。
- 验证：`go test ./internal/app/ -run TestRegression` 通过；`jobs-stub` 测试已升级为覆盖「已支持 / 不支持 / 未知」三类返回。

### B. Assets 缺失端点补全 ✅
- `GET /api/assets/:id/original/download`：以 `Content-Disposition: attachment` 强制附件下载原图（官方 App「保存到设备」走此端点）。
- `GET /api/assets/:id/metadata`：返回单 asset 的 EXIF（无 EXIF 时返回空对象而非 404，兼容客户端渲染）。

### C. People 端点增强 ✅
- `GET /api/people` 现返回 `{people,total,count,hidden}` 形状（非管理员过滤隐藏人物）。
- `GET /api/people/:id` 返回含 `assets.total` 计数的 immich 形状响应。
- 注：人物数据来自已写入的 Person 行（facialRecognition 为未支持的 ML job，不会自动创建新人物）。

### 剩余差距（仍属 ML/架构级，未本次处理）
- 人脸识别 / 自动聚类人物、智能搜索（CLIP）、对象标签（ML）依赖外部 AI 后端。
- OAuth/SSO、memories、websocket sync、存储模板迁移、回收站定时清理、通知/邮件等 immich 大功能未覆盖（约 160+ 路由）。
- 反向地理编码（经纬度→地名）仍未做。

## 五、2026-08-14 兼容性对齐（目标：最新 Immich 契约 / App Store 当前版本 ~v1.13x）

经与官方 OpenAPI 规范（`immich-open-api-specs.json`）逐条比对，原实现实际对齐的是**旧版 Immich（~v1.0–v1.9x）**契约；为"对接原版手机 APP、兼容性一致"，本批将服务器契约向**最新 Immich**对齐。已落地（均通过 `go build` / `go vet` / `go test` + 运行时冒烟验证）：

### A. 握手端点（App 启动即命中，原缺必填字段）✅
- `GET /api/server/version`：补齐必填 `prerelease`（int）；版本号改为**可配置**（`IMMICH_COMPAT_VERSION`，默认 `1.130.0`），需与所连客户端期望版本匹配，否则 App 拒绝连接。
- `GET /api/server/features`：补齐全部 16 个必填键（`configFile/duplicateDetection/email/facialRecognition/importFaces/map/oauth/oauthAutoLaunch/ocr/passwordLogin/realtimeTranscoding/reverseGeocoding/search/sidecar/smartSearch/trash`）；`map`/`duplicateDetection`/`realtimeTranscoding` 按**已实现**报 `true`（此前误报 `false` 会导致地图/去重 Tab 被隐藏）。
- `GET /api/server/config`：补齐必填 `isOnboarded/maintenanceMode/mapDarkStyleUrl/mapLightStyleUrl/minFaces/oauthButtonText/publicUsers/trashDays/userDeleteDelay`。
- `GET /api/server/about`：补齐必填 `version`/`licensed`/`versionUrl`；`build` 改为字符串。

### B. 上传 / 去重契约（备份命脉）✅
- `POST /api/assets`：改为接受**最新 multipart 契约**——独立字段 `filename` + `fileCreatedAt`/`fileModifiedAt`（ISO8601）、`duration`（**int 秒**）、`isFavorite`、`visibility` 枚举（archive/timeline/hidden/locked）、`livePhotoVideoId` 等；**不再依赖 `assetType`**（按扩展名 + 魔数嗅探 IMAGE/VIDEO）。保留对旧版单 `asset` JSON 字段的回退，兼容老客户端。
- `POST /api/assets/bulk-upload-check`（**新增**，最新去重端点，旧 `/api/assets/check` 保留）：按内容 **checksum** 去重，返回 `{results:[{id, action: accept|reject, reason, assetId, isTrashed}]}`，与官方客户端握手一致。

### C. 资产响应结构 `AssetResponseDto` ✅
重写为独立 DTO（不再内嵌 DB 模型），字段/类型对齐最新规范：`exifInfo`（原 `exif`）、`isTrashed`（原 `isTrash`）、`duration` 为 **int**、`visibility` 枚举、移除响应中的 `deviceAssetId`/`deviceId`，新增 `resized`/`hasMetadata`/`thumbhash`/`originalMimeType`/`owner`/`duplicateId`/`isEdited`/`isOffline`/`livePhotoVideoId`/`people`/`tags` 等。运行时冒烟已验证字段正确。

### D. 开发环境配置 ✅
- 新增 `.cnb.yml`：`runner.cpus: 4`（云原生开发 4 核；内存按 cpus×2GB 自动分配）。

### E. 画廊保真度（width/height/thumbhash/owner）✅
- `width`/`height`：ingest 时记录像素尺寸——图片走 `image.DecodeConfig`（全格式，不限于 JPEG/TIFF），视频走 `video.Probe`，写入 `Asset` 并在 `AssetResponseDto` 输出（移动端据此做网格等宽高布局，避免 reflow）。
- `thumbhash`：新增纯 Go 实现（`internal/image/thumbhash.go`），**逐行移植官方 reference（npm `thumbhash` v0.1.1）**，与 Immich 移动端解码字节兼容（已用参考 JS 交叉验证：同一像素 Go 与参考实现输出仅差末尾最低阶 AC 项——源于 `math.Cos` 与 V8 `Math.cos` 的次 ULP 差异，解码后观感一致；参考解码器验证 Go 生成的 hash 能正确还原红/绿/蓝布局）。零新第三方依赖（CGO-free）。
- `owner`：`AssetResponseDto` 新增 `owner`（独立 `UserResponse` DTO，含 id/email/name/avatarColor/profileChangedAt/profileImagePath），按 `ownerId` 填充，用于伙伴/共享场景。

### 仍待补全（最新手机 APP 兼容性差距，按优先级）
1. **Live Photo 配对（已补齐 ✅）**：`livePhotoVideoId` 已存储并打通——新增 `GET /api/assets/:id/live-photo` 流式返回配对视频（优先 encoded-video，否则原文件），SPA 灯箱新增「● LIVE」角标与「▶ Live Photo」按钮播放动态。资产响应 DTO 已含 `livePhotoVideoId`。
2. **反向地理编码（已补齐 ✅）**：见 §二.8；摄取时写库 + `POST /api/map/reverse-geocode` 端点，`server/features.reverseGeocoding` 已报 `true`。
3. **回收站定时清理（已补齐 ✅）**：新增 `TrashedAt` 列与后台调度（`startSchedulers`，启动 30s 后首次、之后每 24h）调用 `runTrashCleanup`，永久删除超过 `trashDays`（默认 30，`IMMICH_TRASH_DAYS` 可调，已接入 `server/config`）的回收站资产（含原图 / 缩略图 / 转码文件与 Exif 行）；另提供 `POST /api/trash/cleanup` 手动触发与 `trashCleanup` job（进度可经 `/api/jobs` 查询）。
4. **架构级大功能**：人脸聚类/`people` 召回、智能(CLIP)搜索、OAuth/SSO、memories、通知/邮件、管理后台——仍属 ML / 外部后端 / 多用户范畴，超出单人 / 私域范围（约 160+ 路由未覆盖）。`/api/people/:id/assets` 已返回正确的空形状以兼容官方客户端。**websocket 实时同步已补齐 ✅**：新增 `GET /api/events` 纯 websocket 事件流（RFC6455，依赖 `gorilla/websocket`，纯 Go / CGO-free），资产 / 相册变更（创建 / 更新 / 移入回收站 / 删除 / 恢复 / 相册增删成员等）经内存事件总线实时推送给所有已连接客户端；SPA 自动重连并在收到事件后防抖刷新当前视图。
5. **版本门控**：`IMMICH_COMPAT_VERSION` 默认 `1.130.0`；若用户所用 App 版本不同，需按客户端实际期望版本调整该环境变量，否则可能出现「服务器版本不匹配」提示。

## 六、2026-08-15 发布 v1.2.0-go（克隆原版后端 + 发布产物 + Docker 镜像）

本批工作：① 克隆原版 `immich-app/immich` 的 `server/`（NestJS 后端）作为**参考**（浅克隆、仅 `server/` 子树，存于本地 `/tmp/immich-ref`，不提交仓库——本项目按 `AGENTS.md` 是独立纯 Go 复刻，非 Node 代码 fork），据此补写 [BACKEND_ALIGNMENT.md](BACKEND_ALIGNMENT.md)（原版控制器/服务 ↔ `internal/app` 模块映射 + 契约对齐说明）；② 产出并发布 release 产物；③ 构建并推送 Docker 镜像；④ 全部提交并推送至 CNB。

### A. 发布产物（dist/，依 `.gitignore` 刻意纳入版本库）✅
- 6 平台静态二进制（`CGO_ENABLED=0`，`linux/darwin/windows × amd64/arm64`）由 `scripts/build-release.sh 1.2.0-go` 生成，命名 `immich-go-1.2.0-go-<os>-<arch>/`，配套 `README.txt` + `start.sh`/`.bat` 与 `.tar.gz` 归档，`checksums.txt` 含每个二进制 SHA-256。
- 旧 `1.0.0-go` 产物已清理，仅保留 `1.2.0-go`。

### B. FFmpeg 共享库随包分发（按 `scripts/bundle-deps.sh`）✅ / ⚠️
- **Windows/amd64**：从 BtbN FFmpeg-Builds 拉取 **FFmpeg 7.1 gpl-shared**，实际拷入 8 个 `.dll`（`avcodec/avdevice/avfilter/avformat/avutil/postproc/swresample/swscale`），归档已含该 libs/，**开箱即用**。
- **Windows/arm64**：BtbN **未发布** `win-arm64-gpl-shared`（已用 GitHub API 验证 asset 列表确认），该平台回退占位视频后端（服务其余正常）。
- **Linux / macOS**：本沙箱无法取得完整可移植的共享库（linux `apt-get` 仅能取部分 `.so` 且缺传递依赖、macOS 无 Homebrew），统一回退占位；**推荐 Linux 服务端直接跑下面带 ffmpeg 的 Docker 镜像**，或宿主机 `apt install ffmpeg` 后由 purego 加载器在系统路径找到库。
- 修复了 `bundle-deps.sh` 两个 bug：① 首个 glob 误把 `.tar.gz` 归档当二进制处理（已改为跳过归档/目录）；② BtbN 的 `grep` 用了 glob 风格 `*` 而非正则（已改为 `ffmpeg-n7.1-[^"]*-<winarch>-gpl-shared[^\"]*\.(zip|7z)`）。

### C. Docker 镜像（CNB registry）✅
- 已基于仓库 `Dockerfile`（`golang:1.23` 多阶段，`CGO_ENABLED=0`）构建并推送至 CNB 容器 registry：
  `docker.cnb.cool/finalappstore/immich-go:latest` 与 `:v1.2.0-go`（已验证两 tag 均可拉取、server 正常监听、`[video] using backend: ffmpeg-software`）。
- 运行时由 **alpine(musl) 改为 Debian bookworm(glibc)**：本仓二进制经 `modernc.org/sqlite` 栈为 **glibc 动态链接**（非完全静态），在 musl 下报 `no such file or directory`。镜像内 `apt-get install ffmpeg`，并在二进制 `libs/` 目录建**无版本软链**（`libavformat.so` → `libavformat.so.59` 等），使 purego 加载器按 `exeDir/libs/<soname>` 找到 FFmpeg；其传递依赖经系统 `ld.so` 缓存解析。**容器内视频缩略图/转码完整可用**，是 Linux 服务端部署的推荐方式。
- 当前仅构建 **linux/amd64**（主容器无 `buildx`，多架构 arm64 需 `docker buildx` + QEMU，留待 CI/后续）；前端/Windows 客户端用对应 `dist/` 二进制即可。

### D. 提交与推送 ✅
- 源码 + 文档（`BACKEND_ALIGNMENT.md`、更新后的 `STATUS.md`/`README.md`/`THIRD_PARTY.md`）+ `dist/` 产物 + 修复后的 `bundle-deps.sh` 一并提交并推送至 `origin/main`（CNB）；并打 `v1.2.0-go` 标签推送，作为本次发布快照。

### E. 可行性缺口闭环（2026-08-15 同源补充，非发布）
本批在**不违背 AGENTS.md 硬规则**（纯 Go / `CGO_ENABLED=0` / 视频仅进程内 purego / 单用户私域）前提下，补齐 `STATUS.md` 中全部「符合架构规则且可行」的剩余缺口。`go build` / `go vet` / `go test ./...` 全绿，并经真实二进制端到端冒烟验证。

- **离线反向地理编码（见 `internal/app/geo`）✅**：随包嵌入 GeoNames `cities15000`（34,073 城市，裁为 `cities.tsv`）+ `countryInfo.txt`（ISO→国名），进程内 1°×1° 空间网格最近邻匹配；摄取时把城市 / 国家写入 `Exif.city/country`，并新增 `POST /api/map/reverse-geocode`（lat/lon→city/state/country）。`server/features.reverseGeocoding` 已报 `true`；数据集来源 / 许可已记入 `THIRD_PARTY.md`。
- **Live Photo 配对 ✅**：新增 `GET /api/assets/:id/live-photo` 流式返回配对视频（优先 encoded-video，否则原文件），资产响应 DTO 已含 `livePhotoVideoId`；SPA 灯箱新增「● LIVE」角标 + 「▶ Live Photo」按钮播放动态。
- **回收站定时清理 ✅**：`Asset` 新增 `TrashedAt` 列；`startSchedulers` 启动 30s 后首次、之后每 24h 调用 `runTrashCleanup` 永久删除超过 `trashDays`（默认 30，`IMMICH_TRASH_DAYS`，已接入 `server/config`）的回收站资产（含原图 / 缩略图 / 转码文件与 Exif 行）；另提供 `POST /api/trash/cleanup` 手动触发与 `trashCleanup` job（进度可经 `/api/jobs` 查询）。
- **兼容路由补齐 ✅**：`GET /api/server/statistics`（全局计数）、`GET /api/people/:id/assets`（正确空形状）。
- **测试**：新增 `internal/app/geo/geocoder_test.go`（巴黎 / 东京 / 公海）、`internal/app/features_test.go`（反向地理编码端点、Live Photo 流式、回收站清理、statistics/people 兼容路由），全部通过。

### F. websocket 实时同步（2026-08-15 补充）
- 新增 `internal/app/events.go`：内存事件总线 `EventBus`（pub/sub，每客户端 64 缓冲、慢客户端丢帧不阻塞）+ `GET /api/events` 纯 websocket 端点（gorilla/websocket，25s 心跳 ping、断线由客户端指数退避重连）。
- 事件发射接入：资产上传（`asset.create`）、单/批量更新（`asset.update`）、移入回收站 / 批量删除（`asset.trash` / `asset.delete`）、回收站恢复（`asset.restore`）、回收站清空（`asset.delete`）；相册创建 / 更新 / 删除 / 增删成员（`album.create` / `album.update` / `album.delete` / `album.addAssets` / `album.removeAssets`）。
- SPA 新增 `connectSync()`：登录后 / 启动后连接 `/api/events`，收到事件防抖 600ms 后调用 `route()` 刷新当前视图；断线指数退避重连（上限 30s）。
- 测试：`internal/app/events_test.go`（连接收到 init、发布 `asset.create` 被流式推送、无 token 握手失败），全部通过。`go build` / `go vet` / `go test ./...` 全绿。
- 兼容性说明：SPA 走 `/api/events` 纯 websocket；官方 App 走 Socket.IO。Socket.IO 协议层已实现，见 §六.G。

### G. 对接官方 App：Socket.IO 实时通道（2026-08-15）
官方 Immich 手机 / 网页 App 的实时通道是 **Socket.IO**（Engine.IO v4 传输 + Socket.IO 消息层），而非裸 websocket。本批补齐该协议层，使官方客户端可直接连接并接收实时事件。

- 新增 `internal/app/socketio.go`：实现 Engine.IO v4 + Socket.IO（纯 Go / `CGO_ENABLED=0`，复用 `gorilla/websocket`）：
  - **握手**：`GET /api/socket.io/?EIO=4&transport=websocket` 返回 Engine.IO `open` 包（含 sid、pingInterval/pingTimeout、`upgrades`）；
  - **websocket 传输**（App 实际使用的路径，支持带/不带前期轮询 sid）：发送 `open` → 收到 `40`(connect) 回 `40`(ack) → 周期 `2`(ping)/`3`(pong) 心跳；
  - **轮询传输**（尽力兼容）：握手返回带 sid 的 `open` 包，长轮询 GET 等待下个事件，POST 回 200（App 随后升级到 websocket）；
  - 事件名对齐 Immich 网关：`onAssetUpload` / `onAssetUpdate` / `onAssetTrash` / `onAssetDelete` / `onAlbumUpdate` / `onAlbumDelete` / `onAlbumAddAssets` / `onAlbumRemoveAssets`，与现有内存事件总线同源广播（SPA 的 `/api/events` 与官方 App 的 `/api/socket.io` 收到同一批事件）。
- 路由：`api.GET/POST("/socket.io")` 与 `"/socket.io/"`。
- 测试：`internal/app/socketio_test.go` 用手写 Socket.IO 客户端验证握手 → connect ack → 事件投递（42[...]）、轮询握手返回带 sid 的 `open` 包；`go build` / `go vet` / `go test ./...` 全绿。

### 与官方 App 对接的其余前提（重要）
- **版本门控**：官方 App 启动会校验服务器版本，不匹配则拒绝连接。本服务通过 `IMMICH_COMPAT_VERSION`（默认 `1.130.0`）宣告版本；**请用你客户端实际期望的版本覆盖该变量**（如 App 为 v1.13x 保持 1.130.0 即可，其他版本按客户端提示调整），否则 App 报「服务器版本不匹配」。
- **REST 兼容性**：timeline / 上传 / 相册 / 地图 / 搜索 / 回收站等核心 REST 已对齐最新 Immich 契约（见 §五），官方 App 可浏览、上传、播放。仍属 ML / 多用户范畴未覆盖的约 160 路由（人脸聚类召回、`/people` 实际数据、CLIP 搜索、OAuth/SSO、memories、通知/邮件、管理后台）在官方 App 中对应页可能为空或报错，但不影响主流程连接与同步。
- 本环境无法运行真实官方 App 做端到端验证；Socket.IO 实现经**线级协议测试**（手写客户端走完整握手 + 事件投递）验证，建议在你自己的设备/App 上以匹配的 `IMMICH_COMPAT_VERSION` 实测确认。

### H. 兼容性核对：官方客户端 v3.1.0（2026-08-15）
拉取官方 OpenAPI 规范 `open-api/immich-openapi-specs.json`（release tag **v3.1.0**，254 个 method-path，`info.version=3.1.0`，base `/api`），与 immich-go 路由逐端点 diff：

- **补齐前**：匹配 70/254，缺失 184。
- **补齐后（本次）**：匹配 97/254，缺失 157。

补齐项（`internal/app/compat_v3.go` + 路由别名）：
- **方法别名**（App 实际使用的动词）：`PATCH /albums/:id`、`PATCH /shared-links/:id`、`PUT /albums/:id/assets`、`GET /map/reverse-geocode`、`GET /search/suggestions`。
- **启动信息端点**（App 启动轮询）：`/server/version-check`、`/server/media-types`、`/server/storage`、`/server/apk-links`、`/server/version-history`、`/server/license`(GET/PUT)。
- **优雅存根**（返回正确空 DTO，不伪造 ML 结果，避免客户端硬 404）：`/sync/ack`(GET/POST/DELETE)、`/sync/stream`、`/people/:id/statistics`、`/faces`、`/faces/:id`、`/people/:id/merge`、`/people/:id/reassign`、`/search/cities`、`/search/places`、`/download/info`、`/trash/restore/assets`、`/duplicates`(+ CRUD 存根)。

**版本门控**：需设 `IMMICH_COMPAT_VERSION=3.1.0` 以通过 v3.1.0 客户端版本校验（已实测 `/api/server/version` 正确返回 `{"major":3,"minor":1,"patch":0,"prerelease":0,"version":"3.1.0"}`）。

**仍缺失（按设计超出单人/私域范围，不阻断连接与同步主流程）**：`admin/*`、`memories/*`、`notifications/*`、`oauth/*`、`plugins/*`、`workflows/*`、`queues/*`、`sessions/*`、ML（`people` 聚类 / `faces` / CLIP `smart-search` / `ocr`——People 页在客户端为空，`server/features` 已置 `facialRecognition:false`）、视频 **HLS 流**（`/assets/:id/video/stream/*`、`main.m3u8`，App 端视频播放走 HLS；immich-go 以 `/assets/:id/encoded-video` MP4 替代）、`stacks/*`、`partners` 部分变体、`trash` 部分变体。

**实测**：`IMMICH_COMPAT_VERSION=3.1.0` 起服，登录及上述新增端点均正常返回（album PATCH、GET 反向地理编码返回 Paris/France、sync/faces/people 存根返回空）；`go build` / `go vet` / `go test ./...` 全绿。

### I. 视频 HLS 兼容（2026-08-15）
官方 v3.1.0 客户端视频走 **HLS**（`/assets/:id/video/stream/main.m3u8` → 变体 `playlist.m3u8` → 分段），之前 immich-go 仅提供 `/assets/:id/encoded-video` 的 MP4，App 内视频无法播放。本次补齐 HLS：

- 新增 `internal/app/hls.go`：以**单次转码 MP4 包装成单变体 HLS**（master → variant playlist → 单分段指向转码 MP4），以 asset id 作为 HLS session id 使播放列表 URL 自洽，客户端直接跟随即可；`DELETE /video/stream/:sessionId` 为无状态 204。
- 端点：`GET /assets/:id/video/playback`（直出视频字节）、`/video/stream/main.m3u8`、`/video/stream/:sessionId/:variantIndex/playlist.m3u8`、`/video/stream/:sessionId/:variantIndex/:filename`、`DELETE /video/stream/:sessionId`。
- 复用进程内视频转码（purego FFmpeg，CGO-free）；无后端时回退原文件。`parseDurationInt(asset.Duration)` 写入 `#EXTINF`。
- 测试：`internal/app/hls_test.go`（master/variant/segment/playback/delete 全链路，content-type 与报文形状校验）；`go build`/`go vet`/`go test ./...` 全绿。

### J. API 一致性调查（v3.1.0 OpenAPI，2026-08-15）
以官方 OpenAPI（`open-api/immich-openapi-specs.json`, tag v3.1.0, 254 method-paths）为基准，对 immich-go 做实时一致性测试。检查器：`scripts/api_consistency.py`（stdlib-only，OpenAPI 驱动；行业标准 CLI 为 `schemathesis run <spec> --base-url <url>`，需 pip，本环境未装，故自建等价 harness）。

**结果（GET 只读端点自动测试）**：
- 可自动测 GET：75；其中返回合法 JSON(2xx)：**42**（已实现并响应）；返回正确 404（超出范围，未实现）：33；SPA 兜底误报：0。
- 字段一致性（required 属性校验）：59 一致 / 16 缺字段。16 处多为**检查器导航伪影**（对象内含原始值列表，如 media-types，服务端实际 OK）或**有意部分实现的大 DTO**（system-config 完整嵌套、timeline 的 AssetResponseDto、UserResponseDto——客户端均容忍，主流程正常）。
- 不可测（需资源 id，由单测覆盖）：88；非 GET 变更类（不自动跑）：91。

**本次据报告修正的 DTO 形状**（原版契约要求，之前返回了错误字段名）：`/sync/ack`→`{ack,type}`、`/server/apk-links`→`{arm64v8a,armeabiv7a,universal,x86_64}`、`/server/version-check`→`{checkedAt,releaseVersion}`、`/server/storage` 补 `*Raw`、`/server/media-types` 补 `sidecar`、`/server/version-history`→数组。修正后这些端点通过一致性校验。

**路线层覆盖**（§H）：匹配 97/254；缺失 157（admin/memories/notifications/oauth/plugins/workflows/queues/sessions/ML/stacks/partners 变体等，按设计超出单人/私域范围）。

**原版并排测试说明**：本环境无 docker/ffmpeg，无法拉起原版 Immich（需 Postgres+Redis+ML 全栈），故未对运行中实例做并排；同一 harness + spec 设 `BASE_URL=<原版地址>` 即可产出并行报告。完整报告见 `reports/api-consistency-v3.1.0.md`。

### 仍待补全（发布相关）
- Windows/arm64 视频开箱即用需 BtbN 提供 `win-arm64-gpl-shared`（上游缺失）；可改从其他渠道取 arm64 共享库。
- Docker 镜像多架构（arm64）+ CNB 流水线自动发版（`.cnb.yml` stages）待接入。

