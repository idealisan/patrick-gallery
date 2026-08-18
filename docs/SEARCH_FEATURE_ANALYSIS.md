# 官方 iPad 客户端搜索专项分析

> 目标：逐步对齐官方 Immich v3.1.0 iPad/移动端搜索 UI、请求契约、服务端查询和客户端结果处理。
>
> 当前阶段只记录分析结果，不代表所有功能已经实现或验证通过。

## 1. 分析范围

- 官方客户端版本：Immich v3.1.0
- 客户端源码：`immich/mobile/lib/`
- 服务端源码：`internal/app/`
- 真实客户端：Apple Silicon Mac 原生运行的 iPad 兼容版
- 真实抓包：本地 TCP 字节代理保存的完整双向流

## 2. 搜索页面入口

官方页面：

```text
mobile/lib/presentation/pages/search/drift_search.page.dart
```

页面由搜索输入框、文本搜索模式菜单、横向筛选 Chip、结果网格和快捷入口组成。

搜索状态初始化为：

- `people = {}`
- `location = {country:null, state:null, city:null}`
- `camera = {make:null, model:null}`
- `date = {takenAfter:null, takenBefore:null}`
- `display = {isNotInAlbum:false, isArchive:false, isFavorite:false}`
- `rating = none`
- `mediaType = other`
- `tagIds = []`

当服务端 `smartSearch=false` 时，客户端默认使用文件名搜索；当 `smartSearch=true` 时，默认使用上下文/语义搜索。

## 3. 主文本搜索模式

右上角菜单提供以下搜索模式：

### 3.1 文件名

- UI：文件名或扩展名搜索
- 客户端字段：`filename`
- API DTO 字段：`originalFileName`
- API：`POST /api/search/metadata`

### 3.2 上下文/语义

- UI：自然语言上下文搜索，例如“海边日落”
- 客户端字段：`context`
- API DTO：`SmartSearchDto.query`
- API：`POST /api/search/smart`
- 显示条件：服务端 `smartSearch=true`
- 当前服务端状态：`smartSearch=false`

### 3.3 描述

- UI：按图片描述搜索
- 客户端字段：`description`
- API DTO：`MetadataSearchDto.description`
- API：`POST /api/search/metadata`

### 3.4 OCR

- UI：按 OCR 文本搜索
- 客户端字段：`ocr`
- API DTO：`MetadataSearchDto.ocr`
- API：`POST /api/search/metadata`
- 显示条件：服务端 `ocr=true`
- 当前服务端状态：`ocr=false`

## 4. 筛选功能

### 4.1 People

- UI：人物选择器
- 客户端状态：`people`
- 请求字段：`personIds`
- 语义搜索路径：`POST /api/search/smart`
- 普通 metadata 路径：`POST /api/search/metadata`
- 依赖：人物/人脸数据

### 4.2 Location

- UI：地点选择器
- 字段：`country`、`state`、`city`
- 普通 metadata 搜索：`POST /api/search/metadata`
- 语义搜索：`POST /api/search/smart`

### 4.3 Tags

- UI：标签选择器
- 字段：`tagIds`
- 普通 metadata 搜索：`POST /api/search/metadata`
- 语义搜索：`POST /api/search/smart`
- 显示条件：用户偏好 `tagsEnabled`

### 4.4 Camera

- UI：相机选择器
- 字段：`make`、`model`
- 普通 metadata 搜索：`POST /api/search/metadata`
- 语义搜索：`POST /api/search/smart`

### 4.5 Date

- UI：快捷日期选择器和完整日期范围选择器
- 字段：`takenAfter`、`takenBefore`
- 普通 metadata 搜索：`POST /api/search/metadata`
- 语义搜索：`POST /api/search/smart`
- 客户端会将日期范围结束时间扩展到当天 `23:59:59`

### 4.6 Media Type

- UI：媒体类型选择器
- 选项：图片、视频
- 字段：`type`
- 值：`IMAGE` 或 `VIDEO`
- 普通 metadata 搜索：`POST /api/search/metadata`
- 语义搜索：`POST /api/search/smart`

### 4.7 Rating

- UI：评分选择器
- 字段：`rating`
- 允许值：未评分或具体评分
- 显示条件：用户偏好 `ratingsEnabled`
- 普通 metadata 搜索：`POST /api/search/metadata`
- 语义搜索：`POST /api/search/smart`

### 4.8 Display Options

- UI：显示选项选择器
- `isNotInAlbum`：只显示不在相册中的资产
- `isArchive`：只显示归档资产
- `isFavorite`：只显示收藏资产
- 普通 metadata 搜索：`POST /api/search/metadata`
- 语义搜索：`POST /api/search/smart`

## 5. 请求分流规则

官方源码：

```text
mobile/lib/infrastructure/repositories/search_api.repository.dart
```

规则如下：

```text
context 非空 OR assetId 非空
    -> POST /api/search/smart

否则
    -> POST /api/search/metadata
```

文件名搜索属于 metadata 路径，发送 `originalFileName`。

metadata 请求还可能包含：

```json
{
  "originalFileName": "IMG",
  "description": "...",
  "ocr": "...",
  "country": "...",
  "state": "...",
  "city": "...",
  "make": "...",
  "model": "...",
  "takenAfter": "...",
  "takenBefore": "...",
  "rating": 4,
  "isFavorite": true,
  "isNotInAlbum": true,
  "type": "IMAGE",
  "personIds": [],
  "tagIds": [],
  "page": 1,
  "size": 1000
}
```

smart 请求还会增加：

```json
{
  "query": "...",
  "queryAssetId": "...",
  "language": "..."
}
```

## 6. 响应模型

官方客户端期望：

```json
{
  "albums": {
    "items": [],
    "count": 0,
    "facets": [],
    "total": 0
  },
  "assets": {
    "items": [],
    "count": 0,
    "facets": [],
    "nextPage": null,
    "total": 0
  }
}
```

客户端从 `assets.items` 解析 `AssetResponseDto`，然后转换为移动端 `RemoteAsset`。

客户端结果处理源码：

```text
mobile/lib/domain/services/search.service.dart
mobile/lib/presentation/pages/search/paginated_search.provider.dart
mobile/lib/extensions/asset_extensions.dart
```

处理流程：

1. API 返回 `SearchResponseDto`。
2. 如果 response 为空或 `assets.items` 为空，返回空结果。
3. 每个 `AssetResponseDto` 调用 `toDto()` 转为 `RemoteAsset`。
4. 结果追加到分页状态 `SearchState.assets`。
5. 使用 `nextPage` 决定是否继续加载。
6. Timeline 组件以 `RemoteAsset` 列表展示结果。

## 7. 客户端结果解析的关键字段

官方 `AssetResponseDto` 强制读取的字段包括：

```text
checksum
createdAt
duration
fileCreatedAt
fileModifiedAt
hasMetadata
height
id
isArchived
isEdited
isFavorite
isOffline
isTrashed
localDateTime
originalFileName
originalPath
ownerId
thumbhash
type
updatedAt
visibility
width
```

可选但会被解析的嵌套字段包括：

- `exifInfo`
- `owner`
- `people`
- `tags`
- `stack`
- `livePhotoVideoId`
- `resized`
- `libraryId`
- `originalMimeType`

其中 `owner` 如果存在，必须包含：

```text
avatarColor
email
id
name
profileChangedAt
profileImagePath
```

任何一条资产的嵌套 DTO 解析失败，都可能导致整个 `SearchService.search()` 捕获异常并返回空结果。

## 8. 快捷搜索入口

没有活动筛选条件时，页面显示快捷入口：

- Recently taken
- Recently added
- Videos
- Favorites

这些入口跳转到独立页面，不等同于主搜索请求。

## 9. 分页和展示

- 初始页码为 `1`。
- 文件名 metadata 搜索默认 `size=1000`。
- 语义搜索默认 `size=100`。
- 结果写入 `PaginatedSearchNotifier`。
- 滚动接近底部时继续请求下一页。
- `nextPage == null` 时停止分页。
- 没有资产且不在加载时显示搜索无结果状态。

## 10. 当前实测记录

真实 iPad 客户端曾发送：

```http
POST /api/search/metadata
```

```json
{
  "albumIds": [],
  "originalFileName": "IMG",
  "page": 1,
  "personIds": [],
  "size": 1000,
  "tagIds": [],
  "visibility": "timeline"
}
```

修复后通过原始流量回放验证：

- HTTP 状态：`200`
- `assets.count`：非零
- `assets.total`：非零
- `assets.items`：包含 `IMG_*.jpeg`、`IMG_*.heic`、`IMG_*.mov` 等结果

但客户端界面仍未显示结果，下一步必须继续核对完整 `AssetResponseDto`、嵌套 DTO 和客户端日志异常，不能只看服务端查询数量。

## 11. 当前待办

1. 完整补齐服务端 `AssetResponse` 与官方 `AssetResponseDto` 的字段和类型。
2. 逐条验证 `owner`、`exifInfo`、`people`、`tags`、`stack` 的解析安全性。
3. 通过真实客户端日志确认 `SearchService.search()` 是否捕获异常。
4. 为每个搜索模式建立请求/响应回放用例。
5. 逐项验证 People、Location、Tags、Camera、Date、Media Type、Rating、Display Options。
6. 语义搜索、人物搜索和 OCR 要么真实实现，要么返回诚实的 `501`，不能伪造结果。

## 12. 文件名搜索回放诊断（2026-08-18）

### 12.1 服务端查询已确认正常

使用 TCP 代理保存的原始请求回放：

```http
POST /api/search/metadata
```

请求：

```json
{
  "albumIds": [],
  "originalFileName": "IMG",
  "page": 1,
  "personIds": [],
  "size": 1000,
  "tagIds": [],
  "visibility": "timeline"
}
```

当前服务端返回：

- HTTP `200`
- `assets.count = 60`
- `assets.total = 60`
- `assets.items` 包含 `IMG_*.jpeg`、`IMG_*.heic`、`IMG_*.mov` 等资产

因此，`originalFileName` 查询条件和数据库匹配逻辑已经生效；搜索无结果不是 SQL 查询为空。

### 12.2 客户端结果处理链

官方客户端源码调用链：

```text
SearchApi.searchAssets()
  -> deserialize SearchResponseDto
  -> deserialize SearchAssetResponseDto
  -> deserialize each AssetResponseDto
  -> deserialize nested owner UserResponseDto
  -> AssetResponseDto.toDto()
  -> SearchService.search()
  -> PaginatedSearchNotifier
  -> Timeline
```

`SearchService.search()` 捕获反序列化异常后返回 `null`，页面随后表现为无搜索结果。因此必须检查所有嵌套 DTO，不能只检查 `assets.count`。

### 12.3 已确认的 DTO 根因

官方 `UserResponseDto` 的必填字段：

```text
avatarColor
email
id
name
profileChangedAt
profileImagePath
```

当前 Go 搜索结果中的 `owner` 实际为：

```json
{
  "id": "...",
  "email": "admin@immich.app",
  "name": "Administrator",
  "avatarColor": "primary",
  "profileChangedAt": "2026-08-17T15:42:53Z"
}
```

缺少：

```json
"profileImagePath": ""
```

官方生成代码会强制读取：

```dart
profileImagePath: mapValueOfType<String>(json, r'profileImagePath')!
```

字段缺失会导致 `UserResponseDto.fromJson()` 抛异常，进而导致 `AssetResponseDto`、`SearchService.search()` 失败，最终客户端显示为空结果。

Go 根因：

```go
ProfileImagePath string `json:"profileImagePath,omitempty"`
```

空字符串因 `omitempty` 被删除。该字段必须始终输出，即使值为空字符串：

```json
"profileImagePath": ""
```

### 12.4 当前修复结论

- `originalFileName` 搜索查询：已修复并通过请求回放。
- 搜索响应外层结构：已返回非空 `assets.items`。
- 搜索客户端仍显示空结果：根因锁定为嵌套 `owner.profileImagePath` 缺失。
- 尚未修改该 DTO 字段；后续修复后必须重新构建、重启，并用真实 iPad 客户端和原始请求回放双重验证。

## 13. 前三种文本搜索的完整调用链

本节不是功能摘要，而是按官方 v3.1.0 客户端实际代码把“用户操作 → 状态 → 请求 → 服务端 → 响应 → 客户端结果”逐步展开。

### 13.1 共同入口与共同状态

页面入口：

```text
mobile/lib/presentation/pages/search/drift_search.page.dart
```

页面初始化一个 `SearchFilter`，包含以下全部状态：

| 状态 | 初始值 | 作用 |
|---|---|---|
| `context` | `null` | 自然语言/语义查询文本 |
| `filename` | `null` | 原始文件名查询文本 |
| `description` | `null` | EXIF 描述文本查询 |
| `ocr` | `null` | OCR 文本查询 |
| `language` | 当前 locale，例如 `zh-CN` | 语义搜索语言 |
| `assetId` | `null` | 以某个资产作为语义搜索参考 |
| `tagIds` | `[]` | 标签筛选 |
| `people` | `{}` | 人物筛选集合 |
| `location.country` | `null` | 国家筛选 |
| `location.state` | `null` | 州/省筛选 |
| `location.city` | `null` | 城市筛选 |
| `camera.make` | `null` | 相机品牌筛选 |
| `camera.model` | `null` | 相机型号筛选 |
| `date.takenAfter` | `null` | 拍摄起始时间 |
| `date.takenBefore` | `null` | 拍摄结束时间 |
| `display.isNotInAlbum` | `false` | 是否限制不在相册 |
| `display.isArchive` | `false` | 是否只看归档 |
| `display.isFavorite` | `false` | 是否只看收藏 |
| `rating` | none | 无评分/具体评分 |
| `mediaType` | `other` | 不限制/图片/视频 |

页面中的 `search(SearchFilter f)` 执行以下步骤：

1. 如果新 filter 与旧 filter 相等，直接返回，不发请求。
2. 将页面 filter 替换为新 filter。
3. 调用 `paginatedSearchProvider.notifier.clear()` 清空旧结果。
4. 如果 `f.isEmpty`，不发搜索请求，页面显示快捷入口。
5. 如果 `f.isEmpty == false`，调用 `PaginatedSearchNotifier.search(f)`。

### 13.2 文件名搜索：UI 到请求

#### UI 入口

页面启动时根据 `serverFeatures.smartSearch` 选择默认模式：

- `smartSearch=true`：默认上下文模式。
- `smartSearch=false`：默认文件名模式。

当前服务端返回 `smartSearch=false`，所以 iPad 客户端看到的是文件名/扩展名输入框。

右上角文本搜索菜单选择文件名时只改变本地 UI 状态：

- `textSearchType = TextSearchType.filename`
- `searchHintText = file_name_or_extension`

菜单选择本身不会发请求。只有输入框提交时才搜索。

输入框由 `SearchField` 提供，提交回调是：

```dart
handleTextSubmitted(String value)
```

文件名分支构造新 filter：

```dart
filter.value.copyWith(
  filename: value,
  context: '',
  description: '',
  ocr: '',
)
```

这里的关键行为是：文件名搜索会清空其他三种文本搜索状态，确保不会同时走语义、描述或 OCR 条件。

#### 文件名 filter 到 API DTO

官方源码：

```text
mobile/lib/infrastructure/repositories/search_api.repository.dart
```

判断逻辑：

```dart
if (context != null && context.isNotEmpty || assetId != null && assetId.isNotEmpty) {
  return _api.searchSmart(...);
}
return _api.searchAssets(...);
```

文件名 filter 的 `context` 和 `assetId` 都为空，因此进入 metadata 路径。

`MetadataSearchDto` 中客户端会设置的字段如下：

| DTO 字段 | 文件名搜索时来源 | 当前简单输入时的值 |
|---|---|---|
| `albumIds` | filter 没有对应 UI 状态 | `[]` 默认值 |
| `checksum` | 没有设置 | absent |
| `city` | 地点筛选 | absent |
| `country` | 地点筛选 | absent |
| `createdAfter` | 当前页面没有设置 | absent |
| `createdBefore` | 当前页面没有设置 | absent |
| `description` | 已清空 | absent |
| `encodedVideoPath` | 没有设置 | absent |
| `id` | 没有设置 | absent |
| `isEncoded` | 没有设置 | absent |
| `isFavorite` | Display Options | absent/true |
| `isMotion` | 页面普通筛选没有设置 | absent |
| `isNotInAlbum` | Display Options | absent/true |
| `isOffline` | 页面普通筛选没有设置 | absent |
| `lensModel` | 当前 Camera UI 只返回 make/model | absent |
| `libraryId` | 页面没有设置 | absent |
| `make` | Camera Picker | absent |
| `model` | Camera Picker | absent |
| `ocr` | 已清空 | absent |
| `order` | 页面没有设置 | absent |
| `originalFileName` | `filter.filename` | 用户输入，例如 `IMG` |
| `originalPath` | 页面没有设置 | absent |
| `page` | `PaginatedSearchNotifier` | `1` |
| `personIds` | People Picker | `[]` |
| `previewPath` | 页面没有设置 | absent |
| `rating` | Rating Picker | absent/具体值 |
| `size` | metadata 搜索固定 | `1000` |
| `state` | Location Picker | absent |
| `tagIds` | Tag Picker | `[]` |
| `takenAfter` | Date Picker | absent |
| `takenBefore` | Date Picker | absent |
| `thumbnailPath` | 页面没有设置 | absent |
| `trashedAfter` | 页面没有设置 | absent |
| `trashedBefore` | 页面没有设置 | absent |
| `type` | Media Type Picker | absent/IMAGE/VIDEO |
| `updatedAfter` | 页面没有设置 | absent |
| `updatedBefore` | 页面没有设置 | absent |
| `visibility` | `display.isArchive` | timeline 或 archive |
| `withDeleted` | 页面没有设置 | absent |
| `withExif` | 页面没有设置 | absent |
| `withPeople` | 页面没有设置 | absent |
| `withStacked` | 页面没有设置 | absent |

客户端真实抓包中的简单文件名请求是：

```json
{
  "albumIds": [],
  "originalFileName": "IMG",
  "page": 1,
  "personIds": [],
  "size": 1000,
  "tagIds": [],
  "visibility": "timeline"
}
```

#### 文件名请求到 Go 服务端

Go 路由：

```text
POST /api/search/metadata -> handleSearchMetadata
```

当前 handler 对请求解析后的 `searchRequest` 处理：

1. 从上下文读取当前用户 `uid`。
2. 读取 `originalFileName`。
3. 构造用户隔离条件：
   ```sql
   owner_id = uid AND is_trash = false
   ```
4. 如果文件名非空，追加：
   ```sql
   original_file_name LIKE '%IMG%'
   ```
5. 查询匹配资产。
6. 如果同时有 make/model/city/country，再查询 EXIF 资产并与文件名结果合并去重。
7. 将每个 Asset 转成 `AssetResponse`。
8. 返回 `SearchResponse`。

当前回放已证明文件名 SQL 命中：

```text
IMG -> assets.count=60, assets.total=60, items=60
```

#### 文件名响应的完整外层结构

官方 `SearchResponseDto` 要求：

```json
{
  "albums": {
    "count": 0,
    "facets": [],
    "items": [],
    "total": 0
  },
  "assets": {
    "count": 60,
    "facets": [],
    "items": [],
    "nextPage": null,
    "total": 60
  }
}
```

客户端解析顺序是：

1. `SearchResponseDto.fromJson()` 强制解析 `albums` 和 `assets`。
2. `SearchAlbumResponseDto.fromJson()` 读取 `count/facets/items/total`。
3. `SearchAssetResponseDto.fromJson()` 读取 `count/facets/items/nextPage/total`。
4. `AssetResponseDto.listFromJson()` 逐项解析所有资产。
5. `SearchService.search()` 将每项转换成 `RemoteAsset`。
6. `PaginatedSearchNotifier` 将资产追加到状态。
7. 页面通过 `Timeline` 展示状态里的资产。

#### 文件名响应的每个资产字段

官方 `AssetResponseDto` 完整字段如下：

| 字段 | 官方类型/要求 | Go 当前响应 | 客户端用途 |
|---|---|---|---|
| `checksum` | 必需 String | 有 | RemoteAsset checksum |
| `createdAt` | 必需 DateTime | 有 | 上传时间 |
| `duplicateId` | 可选 String? | 未返回 | 可选重复组 |
| `duration` | 必需 int? | 有/null | RemoteAsset durationMs |
| `exifInfo` | 可选 Exif DTO | 有时有 | 详情/EXIF |
| `fileCreatedAt` | 必需 DateTime | 有 | RemoteAsset createdAt |
| `fileModifiedAt` | 必需 DateTime | 有 | 资产更新时间来源 |
| `hasMetadata` | 必需 bool | 有 | 是否有 EXIF |
| `height` | 必需 int? | 有 | RemoteAsset height |
| `id` | 必需 String | 有 | 资产主键 |
| `isArchived` | 必需 bool | 有 | 显示/筛选 |
| `isEdited` | 必需 bool | 有 | 编辑状态 |
| `isFavorite` | 必需 bool | 有 | 收藏状态 |
| `isOffline` | 必需 bool | 有 | 离线状态 |
| `isTrashed` | 必需 bool | 有 | 回收站状态 |
| `libraryId` | 可选 String? | 有/空值时可能省略 | 资产库 |
| `livePhotoVideoId` | 可选 String? | 有/空值时省略 | Live Photo |
| `localDateTime` | 必需 DateTime | 有 | 时间线排序 |
| `originalFileName` | 必需 String | 有 | 搜索结果标题 |
| `originalMimeType` | 可选 String? | 有 | 媒体类型 |
| `originalPath` | 必需 String | 有 | 原始路径 |
| `owner` | 可选 User DTO | 有 | owner 解析 |
| `ownerId` | 必需 String | 有 | 权限/RemoteAsset |
| `people` | 可选 Person[] | 有空数组 | 人物信息 |
| `resized` | 可选 bool? | 有 | 缩略图状态 |
| `stack` | 可选 Stack DTO | 未返回 | 堆叠 ID |
| `tags` | 可选 Tag[] | 有空数组 | 标签 |
| `thumbhash` | nullable String | 有 | 缩略图占位 |
| `type` | 必需枚举 | 有 | IMAGE/VIDEO |
| `updatedAt` | 必需 DateTime | 有 | 本地缓存更新 |
| `visibility` | 必需枚举 | 有 | timeline/archive/etc |
| `width` | 必需 int? | 有 | RemoteAsset width |

#### 文件名搜索当前已确认的客户端失败点

回放结果中的每个 `owner` 都缺少：

```json
"profileImagePath": ""
```

官方 `UserResponseDto` 需要：

```text
avatarColor
email
id
name
profileChangedAt
profileImagePath
```

官方解析使用非空断言：

```dart
profileImagePath: mapValueOfType<String>(json, r'profileImagePath')!
```

所以实际流程是：

1. 服务端查询成功。
2. 服务端返回 60 条。
3. 客户端开始解析第一条。
4. `owner.profileImagePath` 缺失。
5. `UserResponseDto.fromJson()` 抛异常。
6. `SearchService.search()` catch 异常并返回 null。
7. `PaginatedSearchNotifier` 不写入结果。
8. UI 显示无结果。

这解释了“服务端回放有结果，但 iPad 搜索页面空白”。

### 13.3 上下文/语义搜索：完整调用链

#### UI 入口

页面只有在服务端 feature 中：

```json
"smartSearch": true
```

时才在右上角菜单展示“按上下文搜索”。

当前服务端返回：

```json
"smartSearch": false
```

所以当前真实客户端默认不会进入该模式；该模式仍按源码完整记录。

#### UI 状态变更

选择上下文模式时：

- `textSearchType = TextSearchType.context`
- hint 变为自然语言示例

用户提交文本 `sunrise on the beach` 后：

```dart
filter.copyWith(
  filename: '',
  context: value,
  description: '',
  ocr: '',
)
```

#### 请求 DTO 全字段

由于 `context` 非空，客户端进入 `searchSmart()`。

`SmartSearchDto` 支持全部字段：

| 字段 | 来源 | 语义 |
|---|---|---|
| `albumIds` | 当前基础 filter 默认空 | 相册限制 |
| `city` | Location | 城市 |
| `country` | Location | 国家 |
| `createdAfter` | 当前页面未直接设置 | 记录创建时间下限 |
| `createdBefore` | 当前页面未直接设置 | 记录创建时间上限 |
| `isEncoded` | 当前 UI 未提供 | 是否有转码文件 |
| `isFavorite` | Display Options | 收藏 |
| `isMotion` | 当前 UI 未提供 | Live Photo/运动照片 |
| `isNotInAlbum` | Display Options | 不在相册 |
| `isOffline` | 当前 UI 未提供 | 离线 |
| `language` | 当前 locale | 语义检索语言 |
| `lensModel` | 当前 UI 未提供 | 镜头型号 |
| `libraryId` | 当前 UI 未提供 | 资产库 |
| `make` | Camera | 相机品牌 |
| `model` | Camera | 相机型号 |
| `ocr` | 当前 OCR 文本状态 | OCR |
| `page` | paginator | 页码 |
| `personIds` | People | 人物 |
| `query` | `filter.context` | 自然语言查询 |
| `queryAssetId` | 相关资产入口 | 参考图片 |
| `rating` | Rating | 评分 |
| `size` | 语义路径固定 | `100` |
| `state` | Location | 州/省 |
| `tagIds` | Tags | 标签 |
| `takenAfter` | Date | 拍摄时间下限 |
| `takenBefore` | Date | 拍摄时间上限 |
| `trashedAfter` | 当前 UI 未提供 | 删除时间下限 |
| `trashedBefore` | 当前 UI 未提供 | 删除时间上限 |
| `type` | Media Type | IMAGE/VIDEO |
| `updatedAfter` | 当前 UI 未提供 | 更新下限 |
| `updatedBefore` | 当前 UI 未提供 | 更新上限 |
| `visibility` | archive/timeline | 可见性 |
| `withDeleted` | 当前 UI 未提供 | 是否含删除 |
| `withExif` | 当前 UI 未提供 | 是否返回 EXIF |

#### 服务端和结果处理

客户端仍然要求同一个 `SearchResponseDto`，因此后续解析链与文件名搜索完全相同：

```text
POST /api/search/smart
  -> SearchResponseDto
  -> SearchAssetResponseDto
  -> AssetResponseDto[]
  -> RemoteAsset[]
  -> SearchState
  -> Timeline
```

当前 Go `handleSearchSmart()` 返回空结果，因为没有真实 CLIP/embedding 推理能力。该能力不能用固定空 200 假装完成；后续必须实现真实语义搜索或返回诚实的 501，并记录客户端行为。

### 13.4 描述搜索：完整调用链

#### UI 状态变更

右上角菜单选择“按描述搜索”后：

- `textSearchType = TextSearchType.description`
- hint 变为描述示例

提交 `cat` 后构造：

```dart
filter.copyWith(
  filename: '',
  context: '',
  description: value,
  ocr: '',
)
```

#### 请求 DTO 全字段

因为 `context` 为空、`assetId` 为空，所以走：

```http
POST /api/search/metadata
```

`MetadataSearchDto` 中：

- `description = "cat"`
- `originalFileName` absent
- 其他字段按照页面筛选状态发送
- `page=1`
- `size=1000`
- `visibility=timeline` 或 `archive`
- `albumIds=[]`
- `personIds=[]`
- `tagIds=[]`

#### 当前 Go 服务端处理差异

当前 `handleSearchMetadata()` 已处理：

- `originalFileName`
- `make`
- `model`
- `city`
- `country`

当前仍未处理：

- `description`
- `ocr`
- `albumIds`
- `checksum`
- `createdAfter`
- `createdBefore`
- `encodedVideoPath`
- `id`
- `isEncoded`
- `isFavorite`
- `isMotion`
- `isNotInAlbum`
- `isOffline`
- `lensModel`
- `libraryId`
- `order`
- `originalPath`
- `personIds`
- `previewPath`
- `rating`
- `size`分页限制
- `state`
- `tagIds`
- `takenAfter`
- `takenBefore`
- `thumbnailPath`
- `trashedAfter`
- `trashedBefore`
- `type`
- `updatedAfter`
- `updatedBefore`
- `visibility`
- `withDeleted`
- `withExif`
- `withPeople`
- `withStacked`

因此描述搜索当前即使客户端请求正确，也可能返回错误范围或空结果。

#### 客户端响应处理

描述搜索仍使用：

```text
SearchResponseDto
  -> SearchAssetResponseDto
  -> AssetResponseDto.fromJson
  -> owner/exifInfo/people/tags/stack 嵌套解析
  -> RemoteAsset
  -> PaginatedSearchNotifier
  -> Timeline
```

因此描述搜索必须同时满足两类条件：

1. 服务端真正按 `description` 查询。
2. 每条返回资产完全符合官方 DTO，特别是嵌套 `owner.profileImagePath`。

## 14. 前三项当前结论

| 模式 | UI 调用路径 | 查询状态 | 服务端现状 | 客户端结果现状 |
|---|---|---|---|---|
| 文件名 | metadata | `originalFileName` | 已查询成功 | 因 owner DTO 缺字段而解析失败 |
| 上下文 | smart | `query` + 全部可选筛选 | ML 未实现 | 当前 feature 隐藏；不能伪造空成功 |
| 描述 | metadata | `description` | 尚未实现 | 即使响应 DTO 正确也无法得到正确结果 |

本节前三项的调用链、全部请求字段、响应字段、客户端处理和已知失败点均已记录。后续分析应从 OCR 开始，不应跳过 DTO 和 UI 状态链路。

## 15. OCR、People、Location 三项详细调用链

本节按“UI 入口 -> 本地状态 -> 选项加载 -> 请求 DTO -> 服务端处理 -> 响应解析 -> 结果展示”的顺序记录，不只记录最终 endpoint。

### 15.1 OCR 搜索

#### UI 入口与可见条件

源码：`mobile/lib/presentation/pages/search/drift_search.page.dart`。

OCR 菜单项由 `FeatureCheck` 包裹，只有服务端 feature 中 `ocr == true` 才显示。当前 Go 服务端返回 `ocr:false`，所以真实客户端默认隐藏 OCR 入口。

选择 OCR 模式后，输入框提交会构造：

```dart
filter.copyWith(
  filename: '',
  context: '',
  description: '',
  ocr: value,
)
```

这会清空其他三种文本条件，但保留 People、Location、Camera、Date、Type、Rating、Display Options、Tags 等筛选状态。

#### 请求分流

因为 `context` 和 `assetId` 为空，客户端进入 metadata 路径：

```text
SearchApiRepository.search
  -> SearchApi.searchAssets
  -> POST /api/search/metadata
```

OCR 请求仍会带上所有已存在的筛选字段。简单请求形状为：

```json
{
  "albumIds": [],
  "ocr": "发票",
  "page": 1,
  "personIds": [],
  "size": 1000,
  "tagIds": [],
  "visibility": "timeline"
}
```

官方 `MetadataSearchDto` 还支持以下字段，OCR 模式下它们可由其他筛选器或 API 调用设置：

```text
albumIds, checksum, city, country, createdAfter, createdBefore,
description, encodedVideoPath, id, isEncoded, isFavorite, isMotion,
isNotInAlbum, isOffline, lensModel, libraryId, make, model, ocr,
order, originalFileName, originalPath, page, personIds, previewPath,
rating, size, state, tagIds, takenAfter, takenBefore, thumbnailPath,
trashedAfter, trashedBefore, type, updatedAfter, updatedBefore,
visibility, withDeleted, withExif, withPeople, withStacked
```

#### Go 当前处理

当前 `handleSearchMetadata` 已部分支持：

- `originalFileName`
- `make`
- `model`
- `city`
- `country`

当前没有真实 OCR 查询链路：

- 没有查询 OCR 表/字段。
- 没有对 `ocr` 做全文查询或 `LIKE` 查询。
- 没有根据 `withExif`、`withPeople`、`withStacked` 控制响应。

因此 OCR 当前必须选择真实实现或诚实 `501`，不能返回看似成功但忽略 OCR 条件的结果。

#### 客户端响应处理

OCR 结果与文件名结果共用完整链路：

```text
SearchResponseDto
  -> SearchAssetResponseDto
  -> AssetResponseDto[]
  -> AssetResponseDto.toDto()
  -> RemoteAsset[]
  -> PaginatedSearchNotifier
  -> Timeline
```

任何一条资产或嵌套 DTO 解析失败，`SearchService.search()` 会捕获异常并返回 `null`，UI 表现为空结果。

### 15.2 People 人物筛选

#### UI 入口

搜索页 People chip 打开 `PeoplePicker`：

```text
mobile/lib/widgets/search/search_filter/people_picker.dart
```

界面包括：

- 人物本地过滤输入框。
- 人物列表。
- 人物头像和名称。
- 多选状态。
- Apply 和 Clear。

#### 人物列表不是实时搜索，而是本地数据库查询

`PeoplePicker` 监听：

```text
getAllPeopleProvider
```

调用链：

```text
getAllPeopleProvider
  -> PersonService.getAllPeople
  -> DriftPeopleService.getAllPeople
  -> DriftPeopleRepository.getAllPeople
```

本地 Drift 查询条件：

- 人物未隐藏。
- 人脸未删除且可见。
- 关联资产未删除。
- 资产 visibility 为 timeline。
- 人脸数量至少 3，或者人物已经有名称。
- 有名称的人物优先，再按人脸数量排序。

People Picker 输入名称时不请求服务器，只对已加载列表做不区分大小写、去音标的本地过滤。

点击人物会在 `Set<PersonDto>` 中加入或移除人物，并通过 `onSelect` 回传；Apply 后页面执行：

```dart
search(filter.value.copyWith(people: people))
```

Clear 后执行：

```dart
search(filter.value.copyWith(people: {}))
```

#### People 到请求字段

客户端将选中人物转换为：

```json
"personIds": ["person-id-1", "person-id-2"]
```

如果 `context` 和 `assetId` 都为空，使用：

```text
POST /api/search/metadata
```

如果同时有上下文或参考资产，则改用：

```text
POST /api/search/smart
```

所以 People 同一个 UI 筛选器可能进入两个不同 endpoint。

#### Go 当前处理

当前 Go 有 `POST /api/search/person`，但 People Picker 的主要资产筛选并不调用它，而是把 `personIds` 放入 metadata/smart 请求。

当前 metadata handler 没有完整实现 `personIds` 过滤；因此即使客户端 People 列表有数据，最终资产查询也可能忽略人物条件。

另外，若 `/sync/stream` 没有正确同步 PeopleV1、AssetFacesV2 和相关资产，People Picker 本地列表会为空，用户无法选择人物。

#### People 结果处理

结果仍然走：

```text
personIds
  -> metadata/smart
  -> SearchResponseDto
  -> AssetResponseDto.toDto
  -> RemoteAsset
  -> Timeline
```

People 功能必须同时验证：

1. 人物和人脸同步到客户端本地库。
2. People Picker 能展示和选择人物。
3. `personIds` 被服务端实际用于资产过滤。
4. 返回资产 DTO 能被完整解析。

### 15.3 Location 地点筛选

#### UI 状态

源码：`mobile/lib/widgets/search/search_filter/location_picker.dart`。

Location Picker 管理三个 controller 和三个状态：

- `countryTextController` / `selectedCountry`
- `stateTextController` / `selectedState`
- `cityTextController` / `selectedCity`

国家、州/省、城市是级联选择：

- 国家改变：清空州和城市。
- 州改变：清空城市。
- 城市改变：保留国家和州。

Apply 后回传：

```json
{
  "country": "中国",
  "state": "湖北省",
  "city": "武汉"
}
```

页面再执行：

```dart
search(filter.value.copyWith(location: location))
```

#### Location 选项加载

每个下拉选项由 `getSearchSuggestionsProvider` 加载：

```text
getSearchSuggestionsProvider
  -> SearchService.getSearchSuggestions
  -> SearchApiRepository.getSearchSuggestions
  -> SearchApi.getSearchSuggestions
  -> GET /api/search/suggestions
```

官方客户端使用 query 参数：

```text
type=country|state|city
country=<可选>
state=<可选>
make=<可选>
model=<可选>
includeNull=<可选>
lensModel=<可选>
```

响应必须是：

```json
["中国", "日本", "美国"]
```

客户端将每个字符串转换为 `DropdownMenuEntry`。请求异常会被 `SearchService` 捕获并转换成空列表，因此网络错误和确实没有数据在 UI 上可能表现相同。

#### Location 最终资产搜索

没有 context/assetId 时：

```http
POST /api/search/metadata
```

```json
{
  "country": "中国",
  "state": "湖北省",
  "city": "武汉",
  "page": 1,
  "size": 1000,
  "visibility": "timeline"
}
```

有 context/assetId 时：

```http
POST /api/search/smart
```

并额外使用 `language`、`query` 等 smart 字段，默认 size 为 100。

#### Go 当前 Location 处理

当前 Go 的 suggestions handler 读取的是 JSON：

```go
Q string `json:"q"`
```

而官方客户端使用 GET query 参数 `type/country/state/make/model`，因此两者契约不一致。

当前 Go 已有真实地理搜索 helper：

```text
GET /api/search/cities?name=...
GET /api/search/places?name=...
```

但 Location Picker 使用的是 `/api/search/suggestions`，不能用 cities/places 的存在来替代 suggestions 契约。

资产 metadata 查询当前只部分处理 `city/country` 的 EXIF 条件，尚未完整处理：

- `state`
- `albumIds`
- `personIds`
- `tagIds`
- `type`
- 日期范围
- 收藏/归档/不在相册
- 其他 MetadataSearchDto 条件组合

#### Location 结果处理

地点下拉和资产结果是两条不同处理链：

```text
GET /search/suggestions
  -> List<String>
  -> DropdownMenuEntry
  -> selected location
```

以及：

```text
selected country/state/city
  -> SearchFilter
  -> metadata/smart request
  -> SearchResponseDto
  -> AssetResponseDto[]
  -> RemoteAsset[]
  -> Timeline
```

两条链必须分别验证，不能只验证最终资产搜索。

## 16. OCR、People、Location 完整问题矩阵

| 功能 | UI 前置数据 | 选项请求 | 最终请求 | Go 当前差距 | 客户端失败表现 |
|---|---|---|---|---|---|
| OCR | `ocr=true` 才显示 | 无 | metadata，`ocr` | OCR 查询未实现 | 入口隐藏或忽略条件 |
| People | 本地 People/Face/Asset 同步 | 本地 Drift 查询 | metadata/smart，`personIds` | personIds 过滤不完整 | People 列表为空或结果错误 |
| Location | 无特殊 feature | suggestions 参数/返回不匹配 | metadata/smart，country/state/city | suggestions、state、组合过滤不完整 | 下拉为空或资产范围错误 |

本组三项已经按照 UI、状态、请求、响应、客户端解析和 Go 差距展开记录。后续继续分析前，应先确认这部分记录是否符合预期。

## 17. Camera、Date、Media Type 三项详细调用链

### 17.1 Camera 相机筛选

#### 17.1.1 UI、状态与级联建议

源码：`mobile/lib/widgets/search/search_filter/camera_picker.dart`。

Camera Picker 有两个级联下拉框：`make`（相机品牌）和 `model`（相机型号）。组件保存两个 controller 和两个状态值，并用已有 `SearchCameraFilter` 恢复上一次选择。

品牌下拉框创建时调用 `getSearchSuggestionsProvider(type=cameraMake)`，通过 `SearchService`、`SearchApiRepository`、generated `SearchApi` 请求 `GET /api/search/suggestions?type=cameraMake`。型号下拉框依赖当前品牌，请求 `type=cameraModel&make=<selectedMake>`。成功响应必须是 `List<String>`，失败时客户端返回空列表，因此请求异常和确实没有建议在 UI 上都表现为空菜单。

选择品牌时，客户端清空型号并暂存 `{"make":"Apple","model":null}`；选择型号时暂存 `{"make":"Apple","model":"iPhone SE (2nd generation)"}`。Apply 后写入 `SearchFilter.camera`，Clear 后写入空 `SearchCameraFilter`。

#### 17.1.2 Camera 资产请求

没有 `context`/`assetId` 时请求：

```text
POST /api/search/metadata
```

简单请求形状：

```json
{
  "albumIds": [],
  "make": "Apple",
  "model": "iPhone SE (2nd generation)",
  "page": 1,
  "personIds": [],
  "size": 1000,
  "tagIds": [],
  "visibility": "timeline"
}
```

页面同时有上下文或参考资产时改走 `/api/search/smart`，但仍发送 `make`、`model` 和其他筛选字段。其他字段来源包括：Location 的 `country/state/city`、People 的 `personIds`、Tags 的 `tagIds`、Date 的 `takenAfter/takenBefore`、Media Type 的 `type`、Display Options 的 `isFavorite/isNotInAlbum/visibility`、Rating 的 `rating`，以及分页的 `page/size`。

#### 17.1.3 Camera 服务端和结果链

当前 Go suggestions handler 读取 JSON `q`，而官方使用 GET query `type/make/model`，返回对象也不是官方要求的 `List<String>`，所以品牌/型号下拉当前可能为空。metadata handler虽有 make/model 的 EXIF 查询，但必须确保 make 与 model AND 组合，并且与其他条件共同生效。

完整结果链：

```text
Camera Picker
  -> SearchFilter.camera
  -> metadata/smart
  -> SearchResponseDto
  -> SearchAssetResponseDto
  -> AssetResponseDto[]
  -> RemoteAsset[]
  -> SearchState
  -> Timeline
```

### 17.2 Date 日期筛选

#### 17.2.1 UI 模型和状态

源码：`mobile/lib/presentation/widgets/search/quick_date_picker.dart`。

日期入口有快捷选择器和完整日期范围选择器。内部模型包括：

- `RecentMonthRangeFilter(1/3/9)`：最近 1、3、9 个月，起点是对应月份第一天，终点是当前时刻。
- `YearFilter(year)`：非当前年份使用年初到年末；当前年份使用年初到当前时刻。
- `CustomDateFilter(start,end)`：直接使用自定义日期范围。

页面收到日期模型后保存 `dateInputFilter`，更新 chip 文本，调用 `asDateTimeRange()`，并把结束日期扩展到当天 `23:59:59`，写入 `SearchDateFilter.takenAfter/takenBefore`。清除日期时写入空 `SearchDateFilter` 并清空结果。

#### 17.2.2 Date 请求和时间语义

metadata 请求示例：

```json
{
  "albumIds": [],
  "takenAfter": "2024-01-01T00:00:00.000Z",
  "takenBefore": "2024-12-31T23:59:59.000Z",
  "page": 1,
  "personIds": [],
  "size": 1000,
  "tagIds": [],
  "visibility": "timeline"
}
```

客户端 API client 负责 DateTime 序列化。官方 DTO 还支持 `createdAfter/createdBefore`、`updatedAfter/updatedBefore`、`trashedAfter/trashedBefore`，但当前 Date UI 只生成 `takenAfter/takenBefore`。

#### 17.2.3 Go 服务端和结果链

当前 Go metadata handler 尚未完整处理 `takenAfter/takenBefore`、`createdAfter/createdBefore`、`updatedAfter/updatedBefore`、`trashedAfter/trashedBefore`。实现时不能把这些字段混用：拍摄时间、资产记录创建时间、更新时间和回收时间具有不同语义。

结果链为：

```text
DateFilterInputModel
  -> SearchDateFilter
  -> MetadataSearchDto/SmartSearchDto
  -> SearchResponseDto
  -> AssetResponseDto[]
  -> RemoteAsset[]
  -> Timeline
```

滚动分页时，下一页继续携带相同日期条件。

### 17.3 Media Type 媒体类型筛选

#### 17.3.1 UI 和状态

源码：`mobile/lib/widgets/search/search_filter/media_type_picker.dart`。

单选项为：

- All：`AssetType.other`。
- Image：`AssetType.image`。
- Video：`AssetType.video`。

选择后更新本地状态，Apply 写入 `SearchFilter.mediaType`，Clear 写回 `AssetType.other`。`AssetType.other` 被 `SearchFilter.isEmpty` 视为“不限制”，因此只选择 All 不会发搜索请求。

#### 17.3.2 类型映射和请求

客户端映射：

```text
AssetType.image -> AssetTypeEnum.IMAGE
AssetType.video -> AssetTypeEnum.VIDEO
AssetType.other -> 不发送 type
```

图片请求示例：

```json
{"type":"IMAGE","page":1,"size":1000,"visibility":"timeline"}
```

视频请求示例：

```json
{"type":"VIDEO","page":1,"size":1000,"visibility":"timeline"}
```

没有 context/assetId 时进入 metadata；有 context/assetId 时进入 smart。

#### 17.3.3 Go 服务端和结果链

主 `handleSearch` 已支持 `type`，但官方移动端主要调用 `/api/search/metadata`，该 handler 当前没有完整应用 `type`，所以 Image/Video 筛选可能仍返回混合类型。正确实现应保证 IMAGE/VIDEO 限制，并与文件名、日期、地点、相机、显示选项等条件执行 AND 组合。

返回资产的 `type` 必须是官方枚举 `IMAGE`、`VIDEO`、`AUDIO` 或 `OTHER`，否则客户端枚举解析可能失败。

完整结果链：

```text
MediaTypePicker
  -> SearchFilter.mediaType
  -> type=IMAGE/VIDEO
  -> metadata/smart
  -> SearchResponseDto
  -> AssetResponseDto.type
  -> RemoteAsset.type
  -> Timeline
```

## 18. Camera、Date、Media Type 问题矩阵

| 功能 | UI/前置请求 | 最终请求 | Go 当前差距 | 客户端风险 |
|---|---|---|---|---|
| Camera | `cameraMake/cameraModel` suggestions | metadata/smart 的 `make/model` | suggestions 参数/返回不匹配；过滤仅部分实现 | 下拉为空或结果不按相机过滤 |
| Date | 快捷/月/年/自定义范围 | metadata/smart 的 `takenAfter/takenBefore` | 日期字段未完整处理 | 日期选择成功但结果全量 |
| Media Type | All/Image/Video 单选 | metadata/smart 的 `type` | metadata 路径未完整应用 | 图片/视频结果混合 |

### 19. Rating 评分筛选

#### 19.1 UI 可见条件与选项

源码：`mobile/lib/widgets/search/search_filter/star_rating_picker.dart`。

搜索页只有在用户偏好允许评分功能时显示 Rating chip：

```text
userPreferences.ratingsEnabled == true
    -> 显示 Rating chip
userPreferences.ratingsEnabled == false
    -> 隐藏 Rating chip
```

打开后使用 `RadioGroup<int>`，一共六个选项：

- `0`：未评分。
- `1`：1 星。
- `2`：2 星。
- `3`：3 星。
- `4`：4 星。
- `5`：5 星。

#### 19.2 UI 状态语义

客户端用 `Option<int?>` 区分三种状态：

| 状态 | `SearchRatingFilter.rating` | 语义 |
|---|---|---|
| 未启用筛选 | `Option.none()` | 不限制评分 |
| 选择 0 | `Option.some(null)` | 只返回未评分资产 |
| 选择 1-5 | `Option.some(n)` | 只返回指定评分 |

点击选项时：

```dart
SearchRatingFilter(
  rating: Option.some(newValue == 0 ? null : newValue),
)
```

页面 Apply 时：

- 当前 rating 有值：更新 chip 文本并执行 `search(filter.copyWith(rating: rating))`。
- 当前 rating 为 none：清除评分筛选。

Clear 时写入新的空 `SearchRatingFilter()`。

`SearchFilter.isEmpty` 只有在 `rating.isNone` 时才认为评分没有筛选；`some(null)` 是有效筛选，不能被当成空条件。

#### 19.3 请求字段和分流

没有 `context` 或 `assetId` 时，使用：

```text
POST /api/search/metadata
```

选择 4 星的请求会包含：

```json
{
  "rating": 4,
  "page": 1,
  "size": 1000,
  "visibility": "timeline",
  "albumIds": [],
  "personIds": [],
  "tagIds": []
}
```

选择未评分时请求字段仍然存在，但值是 JSON `null`：

```json
{"rating": null}
```

如果存在上下文或参考资产，`rating` 进入 `POST /api/search/smart`，其余语义字段和筛选字段同时发送。

#### 19.4 Go 当前处理和客户端结果

当前 Go `handleSearchMetadata` 尚未处理 `rating`，因此 Rating UI 会产生正确请求，但服务端可能忽略评分条件。

正确服务端语义应为：

- `rating = null`：匹配 EXIF/资产评分为空。
- `rating = 1..5`：匹配对应评分。
- 不存在 `rating`：不增加评分限制。

评分结果仍走标准链：

```text
Rating Picker
  -> SearchRatingFilter
  -> MetadataSearchDto/SmartSearchDto.rating
  -> SearchResponseDto
  -> AssetResponseDto[]
  -> RemoteAsset[]
  -> Timeline
```

必须区分“字段不存在”和“字段存在但值为 null”；否则未评分筛选会被错误当成无筛选。

### 20. Display Options 筛选

#### 20.1 UI 和状态

源码：`mobile/lib/widgets/search/search_filter/display_option_picker.dart`。

Display Options 是三个独立 Checkbox：

- Not in album：`isNotInAlbum`
- Favorite：`isFavorite`
- Archive：`isArchive`

组件用 `Map<DisplayOption, bool>` 暂存复选框状态。每次复选框变化都会调用 `onSelect(options)`，但页面只有在 Apply 时才正式写入 `SearchFilter.display`。

页面 Apply：

1. 读取三个 bool。
2. 生成 chip 文本，可同时包含多个标签。
3. 调用：
   ```dart
   search(filter.value.copyWith(display: display))
   ```

Clear：

```dart
SearchDisplayFilters(
  isNotInAlbum: false,
  isArchive: false,
  isFavorite: false,
)
```

#### 20.2 三个选项的请求语义

| UI 选项 | 请求字段 | 预期服务端语义 |
|---|---|---|
| Not in album | `isNotInAlbum=true` | 资产不出现在任何相册关系中 |
| Favorite | `isFavorite=true` | 资产 `is_favorite=true` |
| Archive | `visibility=archive` | 资产可见性为 archive |

三个选项可以同时启用，服务端应按 AND 组合，而不是 OR。

例如：

```json
{
  "isNotInAlbum": true,
  "isFavorite": true,
  "visibility": "archive",
  "page": 1,
  "size": 1000
}
```

#### 20.3 请求分流

没有 `context`/`assetId` 时：

```text
POST /api/search/metadata
```

存在 `context`/`assetId` 时：

```text
POST /api/search/smart
```

在两条路径中，Display Options 都会和以下条件组合：

- filename/description/OCR 或 context。
- country/state/city。
- make/model。
- takenAfter/takenBefore。
- type。
- rating。
- personIds。
- tagIds。
- albumIds。

#### 20.4 Go 当前处理和客户端结果

当前 Go metadata handler 还没有完整实现：

- `isNotInAlbum`：需要对 `albums_assets_assets` 做不存在关系判断。
- `isFavorite`：需要按 `assets.is_favorite` 过滤。
- `visibility=archive`：当前不能只依赖 `isArchived` 的简单条件，应按官方 visibility 语义处理。
- 与其他字段组合时必须全部使用 AND。

客户端结果链仍为：

```text
DisplayOptionPicker
  -> SearchDisplayFilters
  -> MetadataSearchDto/SmartSearchDto
  -> SearchResponseDto
  -> AssetResponseDto[]
  -> RemoteAsset[]
  -> SearchState
  -> Timeline
```

如果服务端返回的 `visibility`、`isFavorite`、`isArchived` 与筛选语义不一致，客户端可能仍能显示结果，但结果范围错误；这类问题不能只用 HTTP 200 判断通过。

## 21. Rating、Display Options 问题矩阵

| 功能 | UI 状态 | 请求字段 | Go 当前差距 | 关键风险 |
|---|---|---|---|---|
| Rating | none / `some(null)` / `some(1..5)` | `rating` | 未完整处理 | 未评分与无筛选混淆 |
| Not in album | bool | `isNotInAlbum` | 未实现关系反查 | 返回已在相册资产 |
| Favorite | bool | `isFavorite` | metadata 路径未完整处理 | 返回非收藏资产 |
| Archive | bool | `visibility=archive` | 可见性组合未完整处理 | archive/timeline 混淆 |

至此，专项文档已覆盖全部搜索 UI 项目：文件名、上下文、描述、OCR、People、Location、Camera、Date、Media Type、Rating、Display Options。

## 22. OCR/ML 网络视觉后端配置记录

OCR/ML 的远程视觉请求统一使用图片 base64 data URL，不依赖本地文件 URL，适合私有部署和 OpenAI-compatible 服务。

### 22.1 OpenAI Chat Completions

请求目标：

```text
POST {IMMICH_OCR_BASE_URL}/chat/completions
Authorization: Bearer ${IMMICH_OCR_API_KEY}
Content-Type: application/json
```

请求结构：

```json
{
  "model": "gpt-4.1-mini",
  "temperature": 0,
  "messages": [
    {
      "role": "user",
      "content": [
        {"type":"text","text":"Transcribe every piece of visible text..."},
        {
          "type":"image_url",
          "image_url": {
            "url":"data:image/jpeg;base64,...",
            "detail":"high"
          }
        }
      ]
    }
  ]
}
```

响应读取：

```text
choices[0].message.content
```

### 22.2 OpenAI Responses

请求目标：

```text
POST {IMMICH_OCR_BASE_URL}/responses
Authorization: Bearer ${IMMICH_OCR_API_KEY}
Content-Type: application/json
```

请求结构：

```json
{
  "model": "gpt-4.1-mini",
  "input": [
    {
      "role": "user",
      "content": [
        {"type":"input_text","text":"Transcribe every piece of visible text..."},
        {
          "type":"input_image",
          "image_url":"data:image/jpeg;base64,...",
          "detail":"high"
        }
      ]
    }
  ]
}
```

响应优先读取：

```text
output_text
```

没有 `output_text` 时，遍历 `output[].content[].text` 拼接文本。

### 22.3 配置环境变量

当前代码支持：

```text
IMMICH_OCR_PROVIDER=none|openai-chat|openai-responses|http
IMMICH_OCR_BASE_URL=https://api.openai.com/v1
IMMICH_OCR_API_KEY=<secret>
IMMICH_OCR_MODEL=gpt-4.1-mini
IMMICH_OCR_PROMPT=<optional custom OCR prompt>
IMMICH_OCR_DETAIL=high
IMMICH_OCR_TIMEOUT_SECONDS=120
```

兼容 OpenAI API 的第三方服务只需修改 `IMMICH_OCR_BASE_URL` 和模型名，不需要修改 Go handler。

### 22.4 后端边界

统一接口位于：

```text
internal/ocr
```

后端链设计为：

```text
macOS native adapter
  -> Windows/Linux native or community adapter
  -> configured HTTP/OpenAI adapter
  -> explicit terminal unavailable/error
```

当前已落地：

- `Processor` / `Request` / `Result` / `Word` 抽象。
- 有序 `Chain` fallback。
- OpenAI Chat Completions adapter。
- OpenAI Responses adapter。
- 通用 JSON HTTP adapter。
- 明确的错误传播和空结果校验。

当前尚未落地：

- macOS Vision 原生 adapter。
- Windows native/community 动态库 adapter。
- Linux community 动态库 adapter。
- OCR 结果持久化到资产 OCR 表。

上述未落地项不能由空实现伪造成功；实际 adapter 注册后，才可以把对应 feature 报为可用。
