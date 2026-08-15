# immich-go 用户手册（中文版）

> 项目别名 **patrick-gallery**。本手册基于 **最新发布版本 v1.4.0-go**（构建于 2026-08-15）实测整理，
> 覆盖全部配置参数、连接方式与使用场景。英文版见 [USER_MANUAL.md](USER_MANUAL.md)；
> 端点覆盖与差距明细见 [docs/GAP_ANALYSIS.md](docs/GAP_ANALYSIS.md) 与 [docs/API_STATUS.md](docs/API_STATUS.md)。

---

## 0. 文档与版本说明

| 项目 | 值 |
|------|----|
| 当前版本 | `v1.4.0-go` |
| 对外宣称的兼容版本（`IMMICH_COMPAT_VERSION`） | Immich **v3.1.0**（官方手机 App / 网页客户端据此连接） |
| 默认监听 | `0.0.0.0:8081` |
| 默认管理员 | `admin@immich.app` / `password` |
| 运行形态 | 单静态二进制（纯 Go，`CGO_ENABLED=0`），内嵌 Web 前端与离线地理数据集 |
| 数据存储 | 单个 SQLite 文件 + 文件系统目录 |

---

## 1. 它是什么 / 不是什么

`immich-go` 是用纯 Go 重写的一份 **单二进制、自包含** 的 Immich 照片/视频服务端。
它用内置的 SQLite 数据库 + 文件系统取代了原版 Immich 的 Node.js + Postgres + Redis + 机器学习栈，
并且随包内嵌了一个可用 Web 图库，无需 Node.js、Postgres、Redis 或机器学习服务即可运行。

### 已实测可用（v1.4.0-go）

- **认证**：JWT 登录 / 登出、注册（signup）、token 校验、修改密码、API Key 增删查、会话记录
- **用户**：列表、`/users/me`、更新、偏好（preferences）读写、头像、删除
- **多用户 / 账户管理后台**：`/admin/users/*` 完整 CRUD + 恢复 + 偏好 + 会话列表 + 统计 + 日历热力图
- **资产管理（核心）**：多部件上传（图片 + 视频元数据）；图片自动生成 256px JPEG 缩略图（**纯 Go，无需 FFmpeg**）；
  列表 / 检索 / 随机 / 计数 / 统计；查重（duplicates）；单资产获取 / 更新 / 删除（批量删）；
  原图下载、缩略图（含带 `ts` 的缓存变体）；视频原文件直出 + **进程内转码为 H.264/MP4**（经 purego 调用 FFmpeg 共享库）
- **相册**：完整 CRUD + 成员增删改 + 封面 + 统计 + `GET /albums/:id/assets`（按序返回相册内资产）
- **标签**：CRUD + 资产绑定 / 解绑
- **伙伴（partners）**：列表 / 创建 / 删除
- **回收站**：列表 / 恢复 / 清空 / 定时清理（按 `IMMICH_TRASH_DAYS`）
- **动态（activities）**：按资产 / 相册的评论列表与增删
- **分享链接**：创建 / 更新 / 删除 / 列表；以及**免登录公开访问**（`/share/:key` 页面 + JSON + 缩略图/原图流式）
- **时间线**：年 / 月分桶、桶内资产
- **搜索**：文件名 + EXIF 文本、metadata（相机 / 地点）、explore、suggestions、cities、places、smart（语义搜索受限）
- **地图**：`/map/markers`（按 GPS 聚类）+ 离线反向地理编码（`/map/reverse-geocode`）+ 城市/地点搜索
- **库（libraries）**：CRUD + 统计 + **真实磁盘扫描**（`/libraries/:id/scan`，遍历 `ImportPaths`、跳过 `ExcludedPaths`、按内容 sha1 去重）
- **任务（jobs）**：`thumbnailGeneration` / `metadataExtraction` / `videoConversion` / `duplicateDetection` / `trashCleanup` 为**真实执行**；ML 类任务（object/facial/smartSearch 等）返回诚实的 `unsupported` 跳过
- **系统**：config / features / about / version / ping / health、system-config 读写、jobs 列表与状态、下载归档（zip，已实现）
- **实时同步**：WebSocket `/events` + Socket.IO（Engine.IO v4，供官方客户端）+ `/sync/ack` + `/sync/stream`（真实增量喂送）
- **人物（people）**：列表 / 获取 / 资产 / 合并 / 重指派为**真实逻辑**；**人脸检测（faces）因需要 ML 后端而明确返回 `501`**（非假成功）
- **内置 Web UI**：登录 / 时间线图库 / 相册 / 搜索 / 收藏 / 归档 / 回收站 / 管理 / 灯箱查看器（图片预览 + 视频转码播放）/ 拖拽·选择上传 / 多选批量操作 / 地图标签页 / 分享查看页

### 明确不做 / 暂未实现（诚实清单，不假装成功）

- **机器学习相关**：人脸识别/聚类、`/people` 的自动归组、CLIP 语义搜索、OCR —— 纯 Go 内无成熟推理模型，需外接推理或纯 Go 移植（工程量巨大）。相关端点要么返回真实的空结果，要么返回诚实的 `501`。
- **OAuth / SSO、邮件/外部通知、回忆（memories）、工作流/插件、同步流的高级特性** —— 受 `AGENTS.md` 硬规则约束，属延后项。
- **水平多租户扩展**：受 SQLite 单实例约束，需 Postgres 后端（`store.Store` 已留接口）落地后才对等。当前多用户仅限**单实例内**的资源隔离。
- **配额（quota）**：`QuotaSizeInBytes` 可设置并在管理后台显示使用量（`quotaUsageInBytes`），但**目前并未在上传时硬性强制**。

---

## 2. 最新版本功能检查结果（v1.4.0-go 实测）

本手册配套功能检查直接运行了 `dist/immich-go-1.4.0-go-linux-amd64/immich-go`（v1.4.0-go）并对全部核心端点做了端到端 curl 实测，结论如下：

| 功能 | 测试结果 |
|------|----------|
| 健康检查 / about / version / config / features | ✅ 正常（version 返回 `3.1.0`） |
| 登录获取 JWT、刷新 `/users/me` | ✅ 正常 |
| 图片上传（multipart）+ 缩略图生成 | ✅ 上传返回 `created`，纯 Go 生成 64×64 JPEG 缩略图（**无 FFmpeg 也成功**） |
| 资产计数 / 列表 / 单资产详情（完整 DTO 含 exif/people/tags/owner） | ✅ 正常 |
| 相册创建 / 加资产 / 相册内资产列表 / 自动封面 | ✅ 正常 |
| 时间线分桶 | ✅ 正常 |
| 文件名/EXIF 搜索 | ✅ 正常 |
| 地图标记 / 离线反向地理编码（巴黎 → France） | ✅ 正常；GeoNames 离线数据集生效 |
| 城市/地点搜索 | ✅ 正常 |
| 分享链接创建 + 免登录公开查看 | ✅ 正常 |
| 管理员用户列表 / 新建用户（多用户） | ✅ 正常 |
| 外部库创建 + 磁盘扫描（去重 imported/skipped） | ✅ 正常 |
| 任务列表 / `thumbnailGeneration` 触发 | ✅ 正常（真实执行 / 无任务时诚实返回 `nothing to process`） |
| 人脸检测 `POST /faces` | ✅ 诚实返回 `501`（非假成功） |

> 构建与校验：本仓库 `go build` / `go vet` / `go test ./...` 在最新提交保持全绿；CI 在每次 Release 构建 6 个平台静态二进制
> （linux/darwin/windows × amd64/arm64）并跑 Schemathesis 契约一致性回归（详见 [docs/CONTRACT_TESTING.md](docs/CONTRACT_TESTING.md)）。

---

## 3. 下载与安装

`dist/` 目录下已为 6 个平台预构建好产物（由 `scripts/build-release.sh` 以 `CGO_ENABLED=0` 构建）：

| 平台 | 包名 | 备注 |
|------|------|------|
| Linux ARM64（树莓派 / 开发板） | `immich-go-1.4.0-go-linux-arm64.tar.gz` | |
| Linux x86-64 | `immich-go-1.4.0-go-linux-amd64.tar.gz` | |
| macOS Apple Silicon | `immich-go-1.4.0-go-darwin-arm64.tar.gz` | |
| macOS Intel | `immich-go-1.4.0-go-darwin-amd64.tar.gz` | |
| Windows x86-64 | `immich-go-1.4.0-go-windows-amd64.tar.gz` | **内嵌 FFmpeg 7.1 共享 DLL，视频开箱即用** |
| Windows ARM64 | `immich-go-1.4.0-go-windows-arm64.tar.gz` | 视频回退到占位后端（BtbN 未发布 arm64 共享构建） |

校验完整性：

```sh
sha256sum -c dist/checksums.txt
```

解压后进入包目录：

```sh
tar xzf immich-go-1.4.0-go-linux-arm64.tar.gz
cd immich-go-1.4.0-go-linux-arm64
```

每个包含：`immich-go`（或 `immich-go.exe`）、`README.txt`、启动脚本 `start.sh` / `start.bat`。
Windows/amd64 包额外含 `libs/`（FFmpeg 共享 DLL）。Linux/macOS 若需视频缩略图/转码，请参见 §7 让 FFmpeg 共享库可被 purego 加载。

---

## 4. 快速开始

### 4.1 直接运行二进制

```sh
./immich-go            # 监听 http://0.0.0.0:8081
```

浏览器打开 `http://<主机>:8081`，用默认管理员 `admin@immich.app` / `password` 登录。
**请务必在网页「设置 → 修改密码」或通过 API 立即修改默认密码。**

### 4.2 Docker（GHCR / CNB 镜像，视频开箱即用）

镜像在运行时安装了 `ffmpeg` 并建立无版本号 `libav*/libsw*` 符号链接，purego 视频加载器可在容器内直接 `dlopen`，
**视频缩略图/转码开箱即用**。

```sh
docker run -d --name immich-go \
  -p 8081:8081 \
  -v "$(pwd)/data:/data" \
  registry.cnb.cool/finalappstore/immich-go:latest
# 或指定版本标签 :v1.4.0-go
# -> http://localhost:8081  (admin@immich.app / password)
```

- 挂载 `/data` 以持久化 SQLite 库（`immich.db`）与媒体（`resources/`）。
- 构建参数：`IMMICH_PORT=8081`、`IMMICH_DB=/data/immich.db`、`IMMICH_RESOURCE=/data/resources` 已在镜像内预设。
- 镜像基于 Debian bookworm（glibc），因为二进制经 `modernc.org/sqlite` 链接 glibc；**请勿改用 Alpine/musl**，否则无法执行。

---

## 5. 配置（全部环境变量 / 参数）

所有变量均为**可选**，下表列出默认值与含义。变量名统一以 `IMMICH_` 前缀开头。

| 环境变量 | 默认值 | 说明 |
|----------|--------|------|
| `IMMICH_PORT` | `8081` | HTTP 监听端口 |
| `IMMICH_HOST` | `0.0.0.0` | HTTP 监听地址（`127.0.0.1` 仅本机；`0.0.0.0` 全网） |
| `IMMICH_DB` | `immich.db` | SQLite 数据库文件路径（相对工作目录，或绝对路径） |
| `IMMICH_RESOURCE` | `resources` | 媒体/缩略图/转码文件存储根目录（启动时自动建子目录 `upload` `thumbnail` `encoded-video` `profile` `library`） |
| `IMMICH_JWT_SECRET` | `immich-dev-secret-change-me` | JWT 签名密钥。**生产务必改成固定随机串**，否则每次重启已签发的 token 失效 |
| `IMMICH_API_KEY_SALT` | `immich-dev-api-salt` | API Key 哈希盐。**生产务必改成固定随机串**，同上 |
| `IMMICH_LOGIN_REQUIRED` | `true` | 是否要求登录。设为 `false` 时进入**匿名模式**：未带凭据的请求自动以首个管理员身份执行（仅适合受信任的私网） |
| `IMMICH_ADMIN_EMAIL` | `admin@immich.app` | 首次启动（库为空）时引导创建的管理员邮箱 |
| `IMMICH_ADMIN_PASSWORD` | `password` | 首次启动（库为空）时引导创建的管理员密码 |
| `IMMICH_EXTERNAL_DOMAIN` | `""`（空） | 对外可访问域名，用于拼接待分享链接等URL；`/api/server/about` 的 `externalDomain` 字段 |
| `IMMICH_COMPAT_VERSION` | `3.1.0` | 向官方客户端宣称的 Immich 服务端版本。必须与你要连接的客户端版本**匹配**，否则 App 拒绝连接 |
| `IMMICH_TRASH_DAYS` | `30` | 回收站中资产保留天数；超过后由 `trashCleanup` 任务（或 `POST /trash/cleanup`）永久删除 |

> ⚠️ 旧版手册提到的 `IMMICH_VIDEO_BACKEND` 在当前代码中**并未实现/读取**，无需设置；视频后端在 FFmpeg 共享库存在时自动启用，缺失时自动降级（见 §7）。

示例（生产建议）：

```sh
export IMMICH_JWT_SECRET="$(openssl rand -hex 32)"
export IMMICH_API_KEY_SALT="$(openssl rand -hex 32)"
export IMMICH_PORT=8080
export IMMICH_DB=/var/lib/immich/immich.db
export IMMICH_RESOURCE=/var/lib/immich/library
export IMMICH_COMPAT_VERSION=3.1.0
./immich-go
```

> **务必设置固定的 `IMMICH_JWT_SECRET` 和 `IMMICH_API_KEY_SALT`**：如果运行多个实例或频繁重启，
> 否则已签发的 JWT 与 API Key 会在每次启动时轮换失效。

---

## 6. 数据存储与目录结构

全部状态位于两处：

- **SQLite 文件**（`immich.db`）：用户、资产元数据、相册、标签、伙伴、库、分享链接、系统配置等。
- **资源目录**（`resources/`，由 `IMMICH_RESOURCE` 指定），子目录：
  - `upload/` —— 原图/原视频（上传副本；EXTERNAL 库则原地引用不复制）
  - `thumbnail/` —— 256px（视频 320px）JPEG 缩略图，文件名即资产 UUID
  - `encoded-video/` —— 进程内转码后的 H.264 MP4
  - `profile/` —— 用户头像
  - `library/` —— 外部库相关中间文件

备份：停服后复制 `immich.db` 与整个 `resources/` 目录即可；迁移时两者一起拷贝，无需迁移步骤。

---

## 7. 视频处理与 FFmpeg（进程内 / purego）

`immich-go` 在**进程内**处理视频：通过 `github.com/ebitengine/purego` 在运行时 `dlopen` 系统/打包的
**FFmpeg 共享库**（无 CGO、无 CLI 子进程），调用 `libavformat/libavcodec/libavutil/libswscale/libswresample/libavfilter`。

- **封面抽帧**：上传视频即由 FFmpeg 后端抽首帧生成封面（最大边 320px JPEG）。
- **转码播放**：`/encoded-video` 与 HLS 端点把视频进程内转码为 H.264/MP4（libx264 软编）；硬件加速路径（VideoToolbox / NVENC / QSV / AMF）目前为占位、自动回退到软件 libx264。
- **自动降级**：若 FFmpeg 共享库缺失，服务端仍能正常启动并上传/播放原文件，但无缩略图、转码端点回退为直出原文件（不报错）。
- **共享库搜索顺序**：进程先搜索自身所在目录，再搜索 `<可执行文件目录>/libs` 下的无版本号 so/dylib/dll。

不同平台的视频可用性：

| 平台 | 视频缩略图 / 转码 |
|------|-------------------|
| Docker 镜像（Debian + ffmpeg + 符号链接） | ✅ 开箱即用 |
| Windows amd64 发布包（内嵌 FFmpeg DLL） | ✅ 开箱即用 |
| Linux（裸机） | 需安装 `ffmpeg` *并* 提供无版本号 `libav*`/`libsw*` 软链接（如 Docker 镜像做法，或 `bundle-deps.sh` 打包的 `libs/`） |
| macOS | 由 Homebrew `ffmpeg` 的 `libav*.dylib`/`libsw*.dylib` 提供 |
| Windows arm64 | ⚠️ BtbN 未发布 arm64 共享构建 → 回退占位后端 |

FFmpeg 版本固定为 **7.1**，下载 URL 与校验已记录在 [THIRD_PARTY.md](THIRD_PARTY.md)。

视频相关端点：

| 端点 | 用途 |
|------|------|
| `GET /api/assets/:id/thumbnail` | 视频封面帧（库存在时抽帧；否则占位） |
| `GET /api/assets/:id/encoded-video/:ts` | 转码后的 H.264/MP4 播放流（库缺失时直出原文件） |
| `GET /api/assets/:id/preview` | 缩放预览变体 |
| `GET /api/assets/:id/video/playback` | 直接可播放视频字节 |
| `GET /api/assets/:id/video/stream/main.m3u8` 等 | HLS 兼容流（供官方 Android/iOS 播放器） |

---

## 8. 内置 Web 界面使用

打开 `http://<主机>:8081` 即可使用内置 SPA（vanilla JS，随包内嵌，无需构建）：

1. **登录**：用管理员凭据登录。
2. **时间线图库**：仪表盘显示照片/视频总数；网格展示缩略图。
3. **上传**：拖拽或选择图片/视频；上传后自动生成缩略图（图片纯 Go；视频抽帧）。
4. **灯箱查看器**：点击资产查看大图；视频经进程内转码播放。
5. **相册**：新建/重命名/删除相册，添加/移除资产，设置封面，邀请其他用户（多用户）。
6. **搜索**：按文件名/EXIF 文本、相机型号、地点检索；浏览 cities/places。
7. **地图**：按 GPS 在地图上查看聚类标记，点击打开灯箱（地点名由离线 GeoNames 提供）。
8. **收藏 / 归档 / 回收站**：标记收藏、归档，回收站可恢复或清空。
9. **管理**：账户/用户管理（`/admin/users/*`）、库（磁盘扫描）、系统配置、任务触发。
10. **分享**：生成分享链接；他人持 key 访问 `/share/:key` 免登录查看（支持缩略图/原图）。

---

## 9. 多用户与账户管理

`immich-go` 支持**单实例内多用户**（资源按 `owner_id` 隔离），通过管理后台 `/admin/users/*`：

- 管理员在「管理 → 用户」可创建/编辑/删除用户，设置是否管理员、头像色、`PinCode`（6 位）、配额（仅显示用量，未强制）。
- 用户删除为**软删除**，可恢复；最后一个管理员不可被删除。
- 跨用户共享：相册可通过 `PUT /albums/:id/users` 把相册共享给其他用户；伙伴（partners）建立后可互看。
- 每个用户的数据（资产、相册、库、回收站）彼此隔离；分享链接与公开查看不受此限。

> 多用户能力在纯 Go / SQLite 单实例约束内推进；**水平多租户扩展仍属延后项**（需 Postgres 后端）。

---

## 10. 通过 API 使用（curl 示例）

```sh
# 健康检查 / 关于 / 配置
curl http://localhost:8081/api/server/health
curl http://localhost:8081/api/server/about
curl http://localhost:8081/api/server/config

# 登录 -> 取 accessToken
TOKEN=$(curl -s -X POST http://localhost:8081/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@immich.app","password":"password"}' \
  | python3 -c 'import sys,json;print(json.load(sys.stdin)["accessToken"])')

# 计数 + 列表
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/assets/count
curl -H "Authorization: Bearer $TOKEN" "http://localhost:8081/api/assets?take=50"

# 上传图片
curl -X POST http://localhost:8081/api/assets \
  -H "Authorization: Bearer $TOKEN" \
  -F 'asset={"deviceAssetId":"x-1","deviceId":"cli","fileCreatedAt":"2024-01-01T00:00:00Z","fileModifiedAt":"2024-01-01T00:00:00Z","localDateTime":"2024-01-01T00:00:00Z","fileExtension":".png","type":"IMAGE"}' \
  -F 'assetData=@photo.png'

# 取生成的缩略图（需鉴权）
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/assets/<ASSET_ID>/thumbnail -o thumb.jpg

# 相册
curl -X POST http://localhost:8081/api/albums -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' -d '{"albumName":"我的相册"}'
curl -X POST http://localhost:8081/api/albums/<ALBUM_ID>/assets -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' -d '{"ids":["<ASSET_ID>"]}'

# 搜索（文件名/EXIF 文本）
curl -X POST http://localhost:8081/api/search -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' -d '{"query":"海滩","take":20}'

# 反向地理编码
curl -X POST http://localhost:8081/api/map/reverse-geocode -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' -d '{"lat":48.8566,"lon":2.3522}'

# 创建分享链接（个人资产）
curl -X POST http://localhost:8081/api/shared-links -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' -d '{"type":"INDIVIDUAL","assetId":"<ASSET_ID>"}'
# 持 key 免登录公开查看
curl http://localhost:8081/api/share/<KEY>

# 外部库扫描
curl -X POST http://localhost:8081/api/libraries -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' -d '{"name":"照片盘","type":"EXTERNAL","importPaths":"/mnt/photos"}'
curl -X POST http://localhost:8081/api/libraries/<LIB_ID>/scan -H "Authorization: Bearer $TOKEN"

# 触发缩略图生成任务
curl -X POST http://localhost:8081/api/jobs/thumbnailGeneration -H "Authorization: Bearer $TOKEN"
```

---

## 11. API Key 鉴权

除 JWT 外，支持 API Key（更适脚本/长期调用）：

```sh
# 创建 key
KEY=$(curl -X POST http://localhost:8081/api/api-keys \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"cli"}' | python3 -c 'import sys,json;print(json.load(sys.stdin)["key"])')

# 用 key 代替 JWT
curl -H "x-api-key: $KEY" http://localhost:8081/api/assets/count
```

API Key 经 `IMMICH_API_KEY_SALT` 加盐哈希存储；设置固定 salt 以保证重启后仍有效。

---

## 12. 连接官方 Immich 手机 / 网页客户端

官方 React 网页 App 与手机 App 均说 Immich REST API。`immich-go` 实现了它们浏览/上传/整理所需的核心路由，
并对外宣称 **v3.1.0**。连接步骤：

1. 确认服务端 `IMMICH_COMPAT_VERSION` 与你客户端版本一致（默认 `3.1.0`；不一致则改环境变量或升级客户端）。
2. 手机 App：在「服务器地址」填入 `http://<主机>:8081`，用 `admin@immich.app` / `password`（或你自建的账户）登录。
3. 网页端（官方 React）：将其 `IMMICH_API_URL` 指向本服务（例如通过反向代理把官方 `web/dist` 置于 `/`、
   `/api` 代理到 `immich-go`），或直接复用本服务内嵌的 Web UI（`/` 始终可用）。
4. 实时同步走 Socket.IO（Engine.IO v4）+ WebSocket `/events`；首次全量对账由 `/sync/stream` 提供。

---

## 13. 反向代理与 HTTPS

`immich-go` 仅提供明文 HTTP。要用 HTTPS，请置于 Nginx / Caddy 之后：

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

若通过反向代理对外暴露分享链接，请设置 `IMMICH_EXTERNAL_DOMAIN` 为对外域名，便于生成正确 URL。

---

## 14. systemd 守护进程（Linux）

将 `dist/immich-go.service` 复制为 `/etc/systemd/system/immich-go.service`，按需修改
`WorkingDirectory` / `ExecStart` 路径与密钥，然后：

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now immich-go
```

示例单元（节选自仓库 `dist/immich-go.service`）：

```ini
[Unit]
Description=immich-go (Immich server, Go port)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=immich
WorkingDirectory=/opt/immich-go
ExecStart=/opt/immich-go/immich-go
Restart=on-failure
RestartSec=5
Environment=IMMICH_PORT=8081
Environment=IMMICH_DB=/opt/immich-go/immich.db
Environment=IMMICH_RESOURCE=/opt/immich-go/resources
Environment=IMMICH_JWT_SECRET=change-me
Environment=IMMICH_API_KEY_SALT=change-me

[Install]
WantedBy=multi-user.target
```

---

## 15. Docker / docker-compose 部署

多架构镜像（linux/amd64、linux/arm64）发布到 CNB 制品库：

```
registry.cnb.cool/finalappstore/immich-go:latest
registry.cnb.cool/finalappstore/immich-go:v1.4.0-go
```

```sh
docker run -d --name immich-go \
  -p 8081:8081 \
  -v "$(pwd)/data:/data" \
  registry.cnb.cool/finalappstore/immich-go:latest
```

挂载 `/data` 持久化 `immich.db` 与 `resources/`。镜像内已装 ffmpeg 并建立符号链接，视频开箱即用。
本地构建：`docker build -t immich-go .`（仓库根含 `Dockerfile` 与 `.dockerignore`）。

---

## 16. 库扫描（磁盘导入）

除了 App 上传，还可用「外部库」把既有磁盘目录纳入管理：

1. `POST /libraries` 创建 `type:"EXTERNAL"` 库，`importPaths` 填逗号/换行分隔的目录列表，`excludedPaths` 填需跳过的路径。
2. `POST /libraries/:id/scan` 真实遍历目录：识别图片/视频扩展名、按内容 sha1 对当前用户去重、
   通过上传共用的 `ingestStoredFile` 生成缩略图 / 抽取 EXIF / 写 Asset 行。EXTERNAL 库以 `isExternal=true` **原地引用**，不复制副本。
3. 返回 `imported` / `skipped` 计数；前端「管理 → Libraries (disk scan)」面板可创建并触发扫描。

---

## 17. 任务系统（jobs）

`GET /jobs` 列出所有任务及其进度；`POST /jobs/:id` 触发。任务在后端以有界 worker 池（并发 4）真实执行：

| 任务 id | 是否真实执行 | 说明 |
|---------|--------------|------|
| `thumbnailGeneration` | ✅ | 为缺缩略图的资产生成缩略图 |
| `metadataExtraction` | ✅ | 为缺 EXIF 的图片抽取元数据（GPS 同时做离线反查城市/国家） |
| `videoConversion` | ✅ | 把未转码视频进程内转码为 H.264/MP4 |
| `duplicateDetection` | ✅ | 按 checksum 计算重复组（结果经 `/duplicates` 查询） |
| `trashCleanup` | ✅ | 永久删除超过 `IMMICH_TRASH_DAYS` 的回收站资产 |
| `objectDetection` / `facialRecognition` / `smartSearch` / `storageTemplateMigration` / `tagCopy` / `tagImage` | ⚠️ 跳过 | 需要 ML/AI 后端（未随包提供），返回诚实的 `unsupported` 标记，不报错、不假成功 |

`GET /jobs/:id` 返回实时进度（`active`/`completed`/`failed`/`total`/`running`）。

---

## 18. 地图与离线地理编码

- `/map/markers`：按 ~1.1km 网格聚类返回带 GPS 的资产标记（含 city/country）。
- `/map/reverse-geocode`：给定经纬度，用**内嵌离线 GeoNames 数据集**（cities15000 + countryInfo，约 3.4 万城市）返回最近城市与国家，**无外部服务、无外网流量**。`state` 返回 GeoNames admin1 代码（如 Paris → `11`），`city`/`country` 为可显示名称。
- `/search/cities`、`/search/places`：城市/地点联想搜索（同样离线）。

---

## 19. 分享链接

- 鉴权内创建/管理：`/api/shared-links` 的 CRUD（`type` 可为 `INDIVIDUAL` 单资产或 `ALBUM` 相册；可设 `allowDownload` / `allowUpload` / `expiresAt` / `password`）。
- 免登录公开访问（无需登录）：`/share/<key>`（SPA 页面 `share.html`）+ `/api/share/:key`（JSON）、
  `/api/share/:key/thumbnail/:assetId`、`/api/share/:key/original/:assetId`。持 key 即可查看缩略图/原图（视频走进程内转码）。

---

## 20. 备份与数据迁移

全部状态位于两处：SQLite 文件（`immich.db`）+ 资源目录（`resources/`）。
- 备份：停服后复制这两个位置（空闲时复制避免半写）。
- 迁移：把两者一起拷贝到新机器，设置相同的 `IMMICH_DB` / `IMMICH_RESOURCE` 即可，无需迁移步骤。
- 下载归档：`POST /download/archive`（或 `GET`）把所选资产打包为 zip 下载。

---

## 21. 故障排查

| 现象 | 处理 |
|------|------|
| `address already in use` | 端口冲突；改 `IMMICH_PORT` |
| 此前有效的 token 突然 401 | 服务器重启后 `IMMICH_JWT_SECRET` 变化；设为固定值 |
| 上传后缩略图 404（图片） | 上传文件不可解码，重新上传有效 JPEG/PNG；图片缩略图**纯 Go** 必定生成 |
| 视频无缩略图 / 不转码 | 未加载到 FFmpeg 共享库 → 占位后端；Docker 或 Windows/amd64 包开箱即用，Linux 裸机需装 ffmpeg 并提供无版本号软链接（见 §7） |
| 上传/库不在预期位置 | 检查 `IMMICH_DB` / `IMMICH_RESOURCE`，相对工作目录解析 |
| 网页打开显示 JSON 而非图库 | 你访问的是 `/api/...` 路径；请在浏览器打开 `/` |
| `POST /faces` 返回 501 | 正常：人脸检测需要 ML 后端（延后项），已诚实返回 501 而非假成功 |
| 官方 App 拒绝连接 | `IMMICH_COMPAT_VERSION` 与客户端版本不一致；对齐版本或升级客户端 |

日志输出到 stdout，前台运行即可看到。

---

## 22. 重置 / 从头开始

停服后删除数据：

```sh
rm -f immich.db
rm -rf resources
```

下次启动会重建数据库并创建默认管理员账户（`IMMICH_ADMIN_EMAIL` / `IMMICH_ADMIN_PASSWORD`）。

---

## 23. 已知限制与未实现功能（诚实清单）

- **机器学习**：人脸/人物聚类、`/people` 自动归组、CLIP 语义搜索、OCR 未实现（相关端点返回真实空结果或 `501`）。
- **OAuth / SSO、邮件/外部通知、回忆、工作流/插件**：延后项。
- **水平多租户 / 高并发写入**：受 SQLite 单写者约束，写入会串行化；适合个人/单用户/轻量多用户私网，不适合高并发多用户写入。
- **配额（quota）**：可配置并显示用量，但**未在上传时强制**。
- **部分兼容字段为最小实现**：为使官方客户端能加载，config/features 等返回「形状正确、内容最小」的 JSON（如 ML 标志一律 `false`），并非完整语义。

---

## 24. 安全建议

- **立即修改默认管理员密码**（网页设置或 `PUT /api/auth/change-password`）。
- 生产环境务必设置**固定**的 `IMMICH_JWT_SECRET` 与 `IMMICH_API_KEY_SALT`。
- 公网暴露请配合反向代理启用 HTTPS，并仅暴露必要端口。
- `IMMICH_LOGIN_REQUIRED=false` 的匿名模式仅适合受信任私网，切勿在公网开启。
- 通过 `IMMICH_HOST=127.0.0.1` 限制仅本机访问，再用反向代理对外。
- 分享链接为免登录公开访问，生成时注意 `expiresAt` / `password` 设置。

---

## 附录 A：环境变量速查

```sh
IMMICH_PORT=8081
IMMICH_HOST=0.0.0.0
IMMICH_DB=immich.db
IMMICH_RESOURCE=resources
IMMICH_JWT_SECRET=immich-dev-secret-change-me
IMMICH_API_KEY_SALT=immich-dev-api-salt
IMMICH_LOGIN_REQUIRED=true
IMMICH_ADMIN_EMAIL=admin@immich.app
IMMICH_ADMIN_PASSWORD=password
IMMICH_EXTERNAL_DOMAIN=
IMMICH_COMPAT_VERSION=3.1.0
IMMICH_TRASH_DAYS=30
```

## 附录 B：相关文档

- [README.md](README.md) —— 项目总览与构建
- [AGENTS.md](AGENTS.md) —— 项目硬规则与方向（禁止 stub 等）
- [STATUS.md](STATUS.md) —— 功能与缺陷清单（按路由核对）
- [docs/GAP_ANALYSIS.md](docs/GAP_ANALYSIS.md) —— 与原版 Immich 逐端点差距复盘（手机 App + Web UI 功能对等）
- [docs/API_STATUS.md](docs/API_STATUS.md) —— 254 个官方 operation 的实现现状（✅🟡🟠❌）
- [docs/NO_STUBS.md](docs/NO_STUBS.md) —— 禁 stub 整改清单
- [docs/CONTRACT_TESTING.md](docs/CONTRACT_TESTING.md) —— Schemathesis 契约一致性回归
- [THIRD_PARTY.md](THIRD_PARTY.md) —— 运行时原生库（FFmpeg 7.1、GeoNames）固定下载地址
