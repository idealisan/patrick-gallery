# immich-go 与原版 Immich 差距复盘（Gap Analysis）

> 目标：对当前 `immich-go`（纯 Go 单二进制复刻）与原版 Immich（以仓库内置官方 OpenAPI 契约 `open-api/immich-openapi-specs.json`，tag **v3.1.0**，254 个 operation 为基准）做一次**全面、可复核**的差距复盘。
> 本文所有结论均基于 `internal/app/app.go` 实际路由、各 handler 实现、`internal/video`、`internal/store`、`internal/app/models.go` 与官方契约的逐项 diff，而非凭记忆。
> 最后更新：2026-08-15。配套任务状态见 [STATUS.md](../STATUS.md)；契约回归门禁见 [CONTRACT_TESTING.md](CONTRACT_TESTING.md)；原版并排对比 harness 见 [SIDE_BY_SIDE.md](SIDE_BY_SIDE.md)。

---

## 0. 一句话结论

immich-go 已经把**单人 / 私域场景下的核心闭环**打通了（认证、上传去重、缩略图、时间线、相册、搜索、地图、分享链接、视频进程内转码 + HLS、回收站、实时同步、反向地理编码、Live Photo）。但相对原版 Immich 它仍是一个**功能子集**：

- **API 端点覆盖 102 / 254（约 40%）**，152 个 operation 未实现；
- 缺失高度集中在 **ML/AI（人物/人脸/CLIP/OCR）、多用户管理、OAuth/会话、Memories、Stacks、Workflows、Notifications、Plugins、Queues、Admin 维护/备份** 等原版面向「多用户 + AI + 运维」的能力；
- 即便已实现的端点，**功能深度也不对齐**：人物为空、视频仅单码率、分享链接字段未持久化、资产元数据只能读不能写、`visibility` 不持久化、Partner 关系存在但共享资产未透出等。

下面按「架构 → 端点 → 功能深度 → 数据模型 → 鉴权 → 前端 → 媒体 → 运维 → 版本契约 → 性能」分层复盘。

---

## 1. 架构层差距（根本性，最难补齐）

| 维度 | 原版 Immich | immich-go | 差距 |
|------|-------------|-----------|------|
| 形态 | 微服务：NestJS `immich-server` + Python `immich-machine-learning`（TensorRT/CLIP/OCR）+ Postgres（pgvecto-rs 向量）+ Redis | 单一 Go 二进制 + SQLite（纯 Go `glebarez/sqlite` / `modernc.org/sqlite`） | 完全单体化 |
| 存储 | 支持存储模板、外部库（离线引用）、S3/对象存储 | 仅本地磁盘目录（`resources/`）；无 S3、无存储模板重命名 | 存储形态受限 |
| 向量/AI | Postgres + pgvecto-rs 存 CLIP 向量，机器学习的 Python 服务做推理 | 无向量库、无推理服务；`smartSearch`/`facialRecognition`/`ocr` 全为 `false` | AI 能力缺失 |
| 多实例 | 可水平扩展（无状态 server + 外部 DB/Redis） | 单机单进程；SQLite 单写者串行 | 不可水平扩展 |
| DB 抽象 | — | 已有 `store.Store` 接口（见 `internal/store/store.go`），但**只有 SQLite 一个实现**，Postgres 后端未落地 | 抽象存在、后端缺位 |

> 说明：单体化是 `AGENTS.md` 为「单人 / 私域」主动选择的取舍，并非 bug。但由此带来的多用户、AI、运维能力缺失是结构性的。

---

## 2. 端点覆盖差距（252 个 operation 的精确 diff）

以 `scripts/...` 同款方法：把官方契约 254 个 operation 的 `method + 路径` 归一化（`{id}`→`:id`）后，与从 `internal/app/app.go` 解析出的路由逐条匹配。

- **SPEC 总计：254**
- **方法+路径精确匹配：102（≈40%）**
- **缺失：152**

### 2.1 缺失端点按 tag 分布

| Tag | 缺失 | 性质归类 |
|-----|------|----------|
| Authentication | 12 | OAuth/SSO + Pin + 会话锁 + admin-signup（多用户/企业） |
| Users (admin) | 11 | 多用户管理（超出单人范围） |
| Assets | 11 | **元数据编辑 / 复制 / edits / OCR / 资产级 job（单人也缺）** |
| Users | 10 | 资料图、license、onboarding、calendar-heatmap 等 |
| Maintenance (admin) | 9 | 完整性报告 / 维护模式（PG 专属） |
| Memories | 8 | 「On this day」回忆（锦上添花） |
| Stacks | 7 | 连拍/相似堆叠（单人也实用，但缺） |
| Workflows | 7 | 自动化工作流（企业） |
| Notifications | 6 | 站内/邮件通知（email 缺失） |
| People | 6 | 人物**写入**（聚类 ML） |
| Sessions | 6 | 设备会话管理（多用户） |
| Database Backups (admin) | 5 | PG 备份恢复（SQLite 不适用） |
| Albums | 5 | **相册内用户共享 / 批量（单人也实用）** |
| Queues | 5 | 任务队列可视化（内部 runner 替代） |
| Search | 5 | large-assets/random/statistics（单人）/smart(ML)/person(ML) |
| Shared links | 5 | **查看/登录/资产增删（单人也实用）** |
| Plugins | 4 | 插件系统（企业） |
| System metadata | 4 | onboarding/version-check/reverse-geocoding 状态端点 |
| Tags | 4 | 批量标签操作（单人也实用，小缺） |
| Notifications (admin) | 3 | 测试邮件 / 模板（email 缺失） |
| API keys | 3 | 单 key 读取/`me`/更新（小缺） |
| Faces | 3（注） | 人脸 CRUD（ML） |
| Jobs | 2 | `POST /jobs`、`PUT /jobs/:name`（方法差异） |
| Partners | 2 | 伙伴共享变体（单人也实用） |
| Views | 2 | 官方 Web 文件夹视图（SPA 未用） |
| Activities | 1 | `/activities/statistics`（小缺） |
| Authentication (admin) | 1 | `/admin/auth/unlink-all` |
| Download | 1 | `POST /download/archive`（immich-go 用 GET 变体） |
| Libraries | 1 | `/libraries/:id/validate`（小缺） |
| Server | 1 | `DELETE /server/license`（小缺） |
| Sync | 1 | `/sync/stream`（为 stub） |
| System config | 1 | `/system-config/storage-template-options`（存储模板） |

> 注：`Faces` tag 4 个 operation 在 spec 中，其中 `POST /faces`、`PUT /faces/:id`、`DELETE /faces/:id` 缺失，`GET /faces`、`GET /faces/:id` 为优雅 stub。

### 2.2 缺失清单（节选，按「是否单人范围」标注）

**A. 单人 / 私域也应补的「真实缺口」（优先级高）**
- 资产元数据写入：`PUT /assets/:id/metadata`、`PUT /assets/metadata`、`GET/PUT/DELETE /assets/:id/metadata/:key`、`DELETE /assets/metadata`
- 资产复制 / 编辑历史：`PUT /assets/copy`、`GET/PUT/DELETE /assets/:id/edits`
- 资产 OCR：`GET /assets/:id/ocr`
- 资产级 job：`POST /assets/jobs`
- 相册内用户共享：`PUT /albums/:id/user/:userId`、`DELETE /albums/:id/user/:userId`、`PUT /albums/:id/users`、`PUT /albums/assets`
- 相册地图标记：`GET /albums/:id/map-markers`
- 搜索：`POST /search/large-assets`、`POST /search/random`、`POST /search/statistics`、`GET /search/person`
- 分享链接：`GET /shared-links/:id`、`GET /shared-links/me`、`POST /shared-links/login`、`PUT/DELETE /shared-links/:id/assets`
- 标签批量：`PUT /tags`、`PUT /tags/assets`、`PUT /tags/:id/assets`
- 伙伴共享变体：`POST /partners/:id`、`PUT /partners/:id`
- API key：`GET /api-keys/:id`、`GET /api-keys/me`、`PUT /api-keys/:id`
- 存储模板：`GET /system-config/storage-template-options`
- 同步深化：`GET /sync/stream`（当前为 stub，无完整双向增量同步）

**B. 多用户 / 企业 / 运维（按 `AGENTS.md` 主动超出范围）**
- OAuth/SSO 全系：`/oauth/*`（authorize/callback/link/unlink/mobile-redirect/backchannel-logout）、`auth/admin-sign-up`
- 设备会话：`/sessions/*`（含 lock）、`auth/session/lock|unlock`、`auth/pin-code`（PIN 锁）
- 多用户管理：`/admin/users/*`、`/admin/auth/unlink-all`
- 维护/备份：`/admin/maintenance/*`、`/admin/integrity/*`、`/admin/database-backups/*`
- 通知/邮件：`/notifications/*`、`/admin/notifications/*`
- 队列：`/queues/*`
- 插件：`/plugins/*`
- 工作流：`/workflows/*`
- 系统元数据状态：`/system-metadata/*`

**C. AI / ML（结构性缺失，需外部后端）**
- 人物写入/聚类：`POST/PUT/DELETE /people`、`PUT /people/:id`、`DELETE /people/:id`、`GET /people/:id/thumbnail`
- 人脸：`POST/PUT/DELETE /faces`、`GET /faces/:id`
- 语义搜索：`POST /search/smart`（CLIP 向量）
- OCR：`GET /assets/:id/ocr`

**D. 锦上添花 / 官方 Web 专属**
- Memories（8）、Stacks（7）、Views（2）、Download 的 POST 变体（immich-go 用 GET）

---

## 3. 已实现端点的功能深度差距（stub vs 真实）

即便端点「存在」，多数也只是**形状对齐**，语义并不完整：

### 3.1 人物 / 人脸（People/Faces）
- `GET /people` 返回**空列表**（无聚类，`facialRecognition:false`）。`Person` 表仅有手动行，没有自动创建路径。
- `GET /people/:id/assets` 返回空形状；`/search/person` 返回 0 结果。
- 所有人脸写操作（`POST/PUT/DELETE /faces`、`POST/PUT/DELETE /people`、缩略图）缺失。
- **结论**：官方 App 的「人物」页为空，不影响连接但无实用价值。

### 3.2 视频（HLS / 转码）
- HLS 为**单变体、单分辨率**：把一次转码的 MP4 包装成 `master → variant → 单 segment`（`internal/app/hls.go`）。原版 v3.x 提供按质量（1080p/720p/480p）的自适应码率。
- **OS 原生后端全是 stub**：`internal/video/native_stub.go` 中 `newVideoToolbox` / `newMediaFoundation` / `newMediaCodec` 一律返回 `errBackendUnavailable`。`AGENTS.md` 要求的「macOS VideoToolbox / Windows Media Foundation / Linux MediaCodec」硬件加速**实际未实现**，硬件加速只靠 FFmpeg 库内编码器名探测（`h264_videotoolbox`/`nvenc`/`qsv`/`amf`，见 `ffmpeg.go` 的 `selectEncoder`）。
- 无**按需按 quality 实时转码**：`encoded_video_path` 是上传/任务时预生成的单一 h264/mp4。
- 无 HEVC 封装（除非 FFmpeg 共享库含且客户端支持）、无多音轨/音频码率选项、无 animated/webp 动图。
- 抽帧缩略图仅取单帧。

### 3.3 分享链接（Shared links）
- `SharedLink` 模型（`internal/app/models.go`）**缺少** `allowDownload` / `allowUpload` / `description` / `password` / `showMetadata` / `slug` / `assets` 关联列。
- 创建响应从请求体**回显**这些字段（STATUS §K 已知限制），但重读（`GET /shared-links/:id`）时丢失。密码保护未生效。

### 3.4 资产元数据
- 仅 `GET /assets/:id/metadata` 可读；**无写入端点**（见 §2.2-A）。
- `visibility` 枚举（archive/timeline/hidden/locked）是**由 `isArchived` 推导**的（`visibilityOf()`，见 `asset.go`），`hidden`/`locked` 不会被持久化。
- `duplicateId` / `isEdited` / `isOffline` 在 DTO 中返回，但库表无对应列（非持久化）。

### 3.5 实时同步
- `GET /api/events`（内存总线）+ `Socket.IO`（Engine.IO v4）协议层已覆盖主要变更（资产/相册增删改、回收站、相册成员）。
- 但 `GET /sync/stream` 仍是 stub，**没有原版完整的双向增量同步协议**（客户端侧的增量拉取/确认）。官方客户端主流程靠 REST + 实时事件即可，sync 协议主要用于跨设备全量校正。

### 3.6 搜索
- 文本 / EXIF 搜索（`/search`、`/search/metadata`、`/search/explore`、`/search/suggestions`）可用；facets 最小化。
- 无语义（CLIP）搜索（`smartSearch:false`）、无人物/地点召回（人物为空）。

### 3.7 地图（Map）
- `GET /map/markers` 按 GPS 聚类 + 离线等距投影网格 + 标记点（SPA「Map」标签页）。`reverseGeocoding:true`。
- 但**无真实地图瓦片**（原版用 Mapbox/maplibre 样式 URL），`mapDarkStyleUrl`/`mapLightStyleUrl` 为空。反向地理编码是 1°×1° 最近城市近似（GeoNames `cities15000`），非精确行政边界。
- 缺 `GET /albums/:id/map-markers`。

---

## 4. 数据模型 / 持久化差距（schema 深度）

`internal/app/models.go` 的表远少于原版 Immich 的 GORM 模型：

**完全缺失的表/实体**
- `Stack` / `AssetStack`（连拍/相似堆叠）
- `Face` / `AssetFace`（人脸框与资产关联）
- `Session`（设备会话）
- `Memory`（回忆）
- `Notification`
- `Workflow` / `Plugin` / `PluginJob` / `PluginJobAsset`
- `UserPreferences`（独立表；当前偏好内联处理、字段有限）
- 存储模板相关列

**`Asset` 缺列**：`stackParentId`/`stackId`、`isEdited`、`isOffline`、`visibility` 枚举持久化、`duplicateId`、`originalMimeType`（仅 DTO 计算）、`places`、`fileCreatedAt/fileModifiedAt` 已存但 `exifInfo` 关联弱。

**`SharedLink` 缺列**：`allowDownload` / `allowUpload` / `description` / `password`（哈希）/ `showMetadata` / `slug` / `assets` 关联。

**`Partner`**：关系行存在，但**共享资产未在任何查询中透出**（timeline / search 不 join partner 资产），即伙伴共享是「空关系」。

---

## 5. 鉴权 / 账户差距

- **仅 JWT**：无刷新令牌轮换、无 device session、无 PIN 锁、无 OAuth/SSO、无 LDAP。
- **默认 JWT secret 硬编码**：`IMMICH_JWT_SECRET` 默认 `immich-dev-secret-change-me`（`config.go`），生产必须覆盖，否则可伪造令牌。
- 无账户锁定 / 爆破防护、无密码重置邮件（`email:false`）、无注册审批 / 邀请。
- 无 admin 用户管理端点（11 个）+ `auth/admin-sign-up`。
- API key 仅支持 list/create/delete，缺单 key 读取、更新、`/me` 别名。

---

## 6. 前端（SPA）差距

`internal/webroot/assets/app.js`（vanilla JS，无构建步骤，~976 行）覆盖：时间线图库、相册、搜索、地图、收藏、归档、回收站、上传、多选批量、管理、灯箱（图片 + 视频进程内转码播放）、分享。

**明显缺失的官方前端能力**
- 人物 / 人脸 UI（因 ML 缺失，自然没有）
- Memories（「On this day」）
- 伙伴共享界面（Partner 共享在前端无入口）
- OAuth 登录按钮（仅密码登录）
- 设置 / 账户 / 用户管理 / 作业管理 / 维护面板
- 文件夹视图（`/view/folder`，官方 Web 用，SPA 未实现）
- 高级搜索 facets（人物/地点/相机细化）
- 真实地图瓦片（仅离线网格）

**维护负担**：原版是 React + TypeScript + Vite 持续演进；immich-go 的无构建 vanilla SPA 需手动追赶，随官方 UI 迭代成本递增。

---

## 7. 媒体 / 转码差距（汇总 §3.2）

- 仅 h264/mp4 输出路径；无多分辨率自适应码率（ABR）。
- 无 HEVC 封装、无音频单独处理、无动图（animated webp/gif）。
- 无**按客户端请求的 quality 实时转码**（预生成单一 encoded-video）。
- 抽帧缩略图单帧。
- OS 原生硬件加速后端为 stub（仅 FFmpeg 库内编码器名探测可用）。

---

## 8. 运维 / 可观测 / 备份差距

- **无 admin database-backups**（PG 备份/恢复）——SQLite 只能停服拷文件。
- 无 integrity report / maintenance mode 端点（`/admin/integrity/*`、`/admin/maintenance/*`）。
- 无 notifications / email（站内信、测试邮件、模板）。
- 无 queues 可视化（`/queues/*`）。
- 无 metrics / Prometheus 端点（原版有 `/api/server/health` 简化版，immich-go 有 `ping`/`health` 但无指标）。
- 日志/追踪为 `log.Printf` 级别，无结构化日志、无分布式追踪。

---

## 9. 版本 / 契约维护风险（重要）

- **兼容性版本漂移**：`config.go` 默认 `IMMICH_COMPAT_VERSION=1.130.0`，而仓库内置契约是 **v3.1.0**（`STATUS.md` §H 实测需显式设 `3.1.0` 才能过 v3.1.0 客户端校验）。默认广告版本与代码内契约不一致，易引发「服务器版本不匹配」。
- **客户端快速迭代**：官方 App / Web 随版本升级可能引入新必填字段或新端点，immich-go 需持续追赶（每次契约 diff 都要补）。这是长期维护成本，非一次性工作。
- **契约测试覆盖有限**：Schemathesis 仅跑 `examples` 阶段，实际只覆盖 30/254 个 operation；`scripts/schemathesis-allowlist.txt` 豁免 25 个已知未实现端点 + 3 个良性边界。其余 152 缺失端点不在门禁视野内（只靠本 diff 暴露）。
- DTO 形状回归靠 `response_schema_conformance` 等检查，但「内容最小化」的桩端点（ML/sync）即使形状正确也无真实语义。

---

## 10. 性能 / 规模差距

- **SQLite 单写者串行化**：并发写会串行，多用户/高并发写可能出现锁等待甚至偶发 500（个人场景可接受，多用户不行）。
- 无缓存层（Redis）；缩略图有 `resize_path` 落盘缓存，但原图解码/转码每次按需。
- 无水平扩展 / 多副本 / 读写分离。

---

## 11. 已实现亮点（差距对照中也应肯定）

为公平起见，以下是对齐甚至优于原版「开箱即用」的部分：
- **纯 Go / `CGO_ENABLED=0` / 单静态二进制**，跨 linux/darwin/windows × amd64/arm64（6 平台），Docker 多架构镜像。
- **进程内 purego 视频**（无 ffmpeg CLI、无 CGO），随包分发 FFmpeg 共享库（Windows 开箱即用；Linux 推荐 Docker 镜像内含 ffmpeg）。
- 离线反向地理编码（GeoNames 嵌入，无需外部服务）。
- HLS 兼容（v3.1.0 客户端视频播放）。
- Socket.IO（Engine.IO v4）+ 裸 websocket 双通道实时同步。
- 回收站定时清理、Live Photo 配对、分享链接免登录访问、库磁盘扫描去重。
- DTO 形状经 Schemathesis 契约回归门禁守护。

---

## 12. 建议优先级（big → small）

**P0 — 单人私域真实缺口（值得补，性价比高）**
1. 资产元数据写入（`PUT /assets/:id/metadata` 等）+ `visibility` 持久化
2. 分享链接字段持久化（allowDownload/upload/description/password/slug）+ 重读正确
3. 相册内用户共享（`/albums/:id/user/:userId`、`/albums/:id/users`）+ 前端入口
4. 伙伴共享资产透出（timeline/search join partner 资产）
5. 修复兼容版本漂移：默认 `IMMICH_COMPAT_VERSION` 与内置契约版本对齐（统一到 v3.1.0 或调整文档）

**P1 — 增强单人体验**
6. Stacks（连拍/相似堆叠）
7. 搜索 large-assets / random / statistics
8. 存储模板（`/system-config/storage-template-options` + 落盘重命名）
9. 视频多分辨率 ABR（至少 1080p/720p 两档）
10. OS 原生硬件加速后端落地（VideoToolbox / Media Foundation / MediaCodec via purego）

**P2 — 契约/测试加固**
11. 把 Schemathesis 门禁从 `examples` 扩到更广 phase，或补充读端点自动覆盖
12. 把 152 缺失端点纳入「已知差距清单」持续跟踪（即本文）

**P3 — 架构扩展（超出当前单人范围，按 AGENTS.md 暂不做）**
13. Postgres 后端（`store.Store` 已有接口，落地第二个实现）
14. ML/AI 后端（CLIP/OCR/人脸聚类）外接
15. OAuth/SSO、Sessions、Memories、Workflows、Notifications、Plugins、Admin 维护/备份

---

## 13. 复现方法

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

*本文为审计文档，不含代码改动。所有「未实现」项均为有意记录，便于后续规划；其中 P3 项按 `AGENTS.md` 当前范围暂不实施。*
