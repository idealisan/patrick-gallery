# immich-go — 功能与缺陷清单 (STATUS)

> 本文件基于 `internal/app/` 的实际路由与处理函数核对整理（非凭记忆）。
> 项目别名：**patrick-gallery**。最后更新：2026-08-09。

## Release 2 — in progress

> 目标：视频在进程内完全可用（FFmpeg 共享库经 purego 加载、随包分发）、图像 API 补全、数据库抽象、官方前端移植。最后更新：2026-08-09。

| 工作流 | 状态 | 说明 |
|--------|------|------|
| 数据库抽象层 (`internal/store`) | `[done]` | 接口 + SQLite WAL 调优已完成 |
| 视频后端抽象 (`internal/video`) + 纯软件 FFmpeg (purego) 后端 | `[done]` | 抽象接口就绪；**纯 Go / purego FFmpeg 后端完整可用**：Probe + 抽帧缩略图 + 进程内转码（无 CLI、无 CGO），经 `go test` 与 HTTP 端到端验证（输出经 ffprobe 确认为合法 h264 mp4）。硬件加速后端仍为 stub。 |
| 视频 API 接线（上传缩略图、`/preview`、转码 `/encoded-video`） | `[done]` | 上传即抽帧生成视频封面；`/encoded-video` 走真实进程内转码（libx264 软编），失败时回退原文件 |
| 图像：EXIF 提取 + 更宽解码（WebP） | `[in progress]` | EXIF 与 WebP 解码推进中 |
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
- **地图（已基本补齐 ✅）**：已实现 `/api/map/markers`（按 GPS 聚类、返回标记 + city/country + 代表 assetId）与 SPA「Map」标签页（离线等距投影世界网格 + 标记点 + 缩略图列表，点击打开灯箱）。**反向地理编码（经纬度→地名）尚未做**，标记地名依赖已写入 EXIF 的 city/country。

### 9. 部分兼容字段为最小实现
- 为让官方客户端能加载，新增的 compat 端点（config / features 等）返回的是「形状正确但内容最小化」的 JSON（如 ML 标志一律为 false），并非完整语义。

## 三、优先级建议（按性价比）
1. **分享链接公开访问**（#5）——补全端点即可对外分享。
2. **视频缩略图抽帧**（#2）——提升图库观感。
3. 其余（ML、转码、扫描、多用户并发）属架构级投入，暂不在个人场景范围内。
