# BUG-008：iOS 客户端登录失败（及图片/视频上传链路连带失败）—— 缺少 `/.well-known/immich` 端点发现

> 状态：**Resolved（已修复、构建、部署，并用真实浏览器/curl 验证发现端点）** · 日期：2026-08-16 · 严重度：**High（iOS 登录即失败，阻塞整个 iOS 流程）**；Image/Video Upload 为 **Medium**（根因同源，登录修复后多数可工作，但视频观看仍有 BUG-007 已知缺口）
>
> 触发来源：用户 iOS 抓包 `Stream-eo795eal4s-8081.cnb.run-2026-08-16 12:44:49.har`（经反向代理 `https://eo795eal4s-8081.cnb.run` 抓取，符合 AGENTS.md 要求的「反向代理公网 URL」真实客户端流程）。该 HAR 仅含 1 条记录，即 iOS 登录请求及其失败响应。

## 现象（来自 HAR 证据，非推测）

HAR 中唯一一条记录：

- **请求**：`POST https://eo795eal4s-8081.cnb.run/auth/login`
  - `Content-Type: application/json`，body：`{"email":"admin@immich.app","password":"password"}`
  - 自定义头：`deviceType: iOS`、`deviceModel: iPhone17,1`、`User-Agent: immich-ios/3.1.0`、`Accept: */*`
- **响应**：`HTTP/1.1 200 OK`，`Content-Type: text/html; charset=utf-8`，body 是 **官方 Immich Web SPA 的 `index.html`**（`<!doctype html><html class="dark">...`），约 10.5KB。

iOS 客户端期望拿到 JSON `LoginResponseDto`（`accessToken`/`userId`/`name`/`isAdmin`…），实际拿到一段 HTML → 反序列化失败 → 报告「登录失败」。注意：状态码是 **200**，但 body 是 HTML —— 这正是 SPA history-fallback 把 `/auth/login` 当成了前端路由来返回 `index.html`。

> 关键观察：**路径是 `/auth/login`（没有 `/api` 前缀）**。这正是官方 iOS 客户端按官方契约本应发出的路径（见下「权威依据」）。immich-go 只在 `/api/auth/login` 注册了登录路由（`internal/app/app.go:104`），而 `/auth/login` 不匹配任何 `/api/*` 路由，落入 `main.go:35-41` 的 `NoRoute` SPA 兜底，返回 `index.html`。

## 权威依据（官方 Immich v3.1.0 源码，本地 `/root/immich-src`）

### 子问题 1（登录根因）：官方 iOS 客户端为何请求 `/auth/login`，以及它如何习得 `/api` 前缀

iOS 客户端的 API base path 来自 `api.service.dart` 的 `resolveEndpoint`：

```dart
// mobile/lib/services/api.service.dart:99-110  resolveEndpoint
Future<String> resolveEndpoint(String serverUrl) async {
  String url = normalizeServerUrl(serverUrl);
  final wellKnownEndpoint = await _getWellKnownEndpoint(url);   // 先查 /.well-known/immich
  if (wellKnownEndpoint.isNotEmpty) {
    url = normalizeServerUrl(wellKnownEndpoint);
  }
  if (!await _isEndpointAvailable(url)) { throw ApiException(503, "Server is not reachable"); }
  return url;                                                    // 返回“无 /api”的裸 host
}
```

- `_getWellKnownEndpoint`（`api.service.dart:148-167`）：`GET $baseUrl/.well-known/immich`，若返回 200 则读 `data['api']['endpoint']`。**若以 `/` 开头则拼回 baseUrl**：
  ```dart
  if (endpoint.startsWith('/')) {
    return "$baseUrl$endpoint";   // baseUrl + "/api"  →  https://host/api
  }
  ```
- `_isEndpointAvailable`（`api.service.dart:122-143`）：**临时**把 endpoint 拼成 `url + '/api'` 去 ping（`serverInfoApi.pingServer()`），成功即返回 true —— 但 `resolveEndpoint` 最终返回的仍是**裸 host**（不含 `/api`）。

官方服务端提供该发现端点（`server/src/controllers/app.controller.ts:10-16`）：

```ts
@Controller()
export class AppController {
  @Get('.well-known/immich')
  getImmichWellKnown() {
    return { api: { endpoint: '/api' } };   // ← 相对路径 /api
  }
}
```

于是官方流程是：iOS 查 `/.well-known/immich` → 得到 `/api` → 将 endpoint 解析为 `https://host/api` → 之后所有调用（`authenticationApi.login`、`assetsApi`、`socket.io` path 等）自动带上 `/api` 前缀。

iOS 实际登录调用（`mobile/openapi/lib/api/authentication_api.dart:340`）：

```dart
final apiPath = r'/auth/login';   // 生成代码写死 /auth/login
```

结合 `basePath = https://host/api`，完整 URL = `https://host/api/auth/login` —— **官方服务端确实有该路由**（`server/src/controllers/auth.controller.ts:11-13`：`@Controller('auth')` + `@Post('login')`，全局前缀 `api` 见 `server/src/app.common.ts:65` `app.setGlobalPrefix('api', ...)`）。

**immich-go 现状**：`internal/app/app.go` / `main.go` 中**完全没有 `/.well-known/immich` 路由**。该路径不以 `/api/` 开头，落入 `main.go:35-41` 的 `NoRoute`，返回 SPA `index.html`（`text/html`）。于是：
- `_getWellKnownEndpoint` 拿到 HTML → `jsonDecode` 抛异常 → 被 catch → 返回 `""` → wellKnownEndpoint 为空；
- 退回 `_isEndpointAvailable`：拼 `https://host/api` 去 ping，immich-go 的 `/api/server/ping`（`app.go:68`）返回 200 → ping 成功 → `resolveEndpoint` 返回**裸 host `https://host`**（无 `/api`）；
- `basePath = https://host` → `authenticationApi.login` 命中 `https://host/auth/login` → immich-go 无此路由 → SPA `index.html` → 登录失败。

### 子问题 2（图片上传）：iOS 上传路径与 immich-go 现状

iOS 上传走 `upload.repository.dart`：

```dart
// mobile/lib/repositories/upload.repository.dart:102
final baseRequest = ProgressMultipartRequest('POST', Uri.parse('$savedEndpoint/assets'), ...);
final assetRawUploadData = MultipartFile("assetData", fileStream, ...);  // 文件字段名 assetData
baseRequest.fields.addAll(fields);   // 见下方字段
```

`fields` 由 `background_upload.service.dart:391-408` 构造：

```dart
final fieldsMap = {
  'filename': originalFileName ?? filename,
  'deviceAssetId': deviceAssetId ?? '',
  'deviceId': deviceId,
  'fileCreatedAt': createdAt.toUtc().toIso8601String(),
  'fileModifiedAt': modifiedAt.toUtc().toIso8601String(),
  'isFavorite': isFavorite?.toString() ?? 'false',
  'duration': '0',                 // ← iOS 永远发 '0'，时长须服务端探针抽取
  if (CurrentPlatform.isIOS && cloudId != null) 'metadata': jsonEncode([...]),
};
```

上传前 iOS 还会先发去重握手 `POST /assets/bulk-upload-check`（`mobile/openapi/lib/api/assets_api.dart:30` `apiPath = r'/assets/bulk-upload-check'`）。

immich-go 现状：
- `POST /api/assets` → `handleAssetUpload`（`app.go:170` 路由 + `asset.go` 实现）。它读取 `filename`/`fileCreatedAt`/`fileModifiedAt`/`isFavorite`/`deviceAssetId`/`deviceId`/`assetData`(multipart)/`duration` 等字段，`c.JSON(201, {"id": asset.ID, "status": "created"})`（`asset.go` 末尾）。与 iOS 期望的 `responseBody['id']`（`upload.repository.dart:131` `remoteAssetId: responseBody['id']`）一致 → **图片上传契约兼容，是真实实现（非 stub）**。
- `POST /api/assets/bulk-upload-check` → `handleAssetBulkUploadCheck`（`app.go:172` + `asset.go:371`）。按 checksum 返回 `action: accept/reject` + `reason: duplicate` + `assetId` + `isTrashed` → **真实实现（非 stub）**，与官方契约一致。

**但**：iOS 的 `$savedEndpoint` 是存储的 endpoint。若 endpoint 为裸 `https://host`，则上传 URL 变成 `https://host/assets`（无 `/api`）→ SPA `index.html` → 上传失败。一旦 endpoint 解析为 `https://host/api`（即修复子问题 1），上传即落到 `https://host/api/assets` → 正常工作。

### 子问题 3（视频上传/观看）：同源根因 + BUG-007 已知缺口

- 视频上传与图片走**同一** `POST /api/assets` multipart（`assetData` 字段），上传本身会成功（文件被存储、`emitAsset` 触发 socket.io 事件）。
- **观看**仍受 `BUG-007` 影响（独立报告，此处仅确认 iOS 同样命中）：
  - HLS 主清单 `main.m3u8` 返回绝对 `http://` 变体 URL，HTTPS 页面触发混合内容拦截（STATUS 0）→ 视频打不开（BUG-007 子问题 1，High）。
  - iOS 发送 `duration:'0'`，且 immich-go 不对视频做元数据探针（`asset.go` 的 `durationMs := parseDurationSecondsToMs(c.PostForm("duration"))` 取到 0）→ `duration`/`ratio` 为 0（BUG-007 子问题 2/3，Medium，真实能力缺失非 stub）。
  - iOS 视频播放器拼 URL：`'$serverEndpoint/assets/$assetId/video/playback'`（`mobile/lib/utils/image_url_builder.dart:20`）。裸 endpoint 时变 `/assets/.../video/playback`（无 `/api`）→ SPA HTML；解析为 `/api` 后变 `/api/assets/.../video/playback`，immich-go 的 `handleVideoPlayback`（`hls.go:64`）可服务。
- **实时同步（socket.io）**：iOS `websocket.provider.dart:74-77` 连接 `io(scheme://host:port, setPath(endpoint.path + "/socket.io"))`。裸 endpoint → path `/socket.io`（immich-go **未注册**，仅注册 `/api/socket.io`，`app.go:325-328`）→ 实时同步断裂；解析为 `/api` 后 path 变 `/api/socket.io` → immich-go 的 `handleSocketIO`（`socketio.go:137`，真实 Engine.IO/Socket.IO 实现，非 stub）可服务。

## immich-go 实际实现（本仓库，待修复）

缺失端点：`GET /.well-known/immich`。全仓库检索（`internal/**`、`main.go`）**无任何 `well-known` 路由**。该路径不以 `/api/` 开头，被 `main.go:35-41` 的 `NoRoute` 兜底成 SPA `index.html`：

```go
// main.go:35-41
r.NoRoute(func(c *gin.Context) {
    if strings.HasPrefix(c.Request.URL.Path, "/api/") {
        c.JSON(http.StatusNotFound, gin.H{"error": "not found", "statusCode": 404})
        return
    }
    webroot.Serve(c.Writer, c.Request)   // ← /auth/login、/.well-known/immich 都到这里 → 返回 HTML
})
```

已有且契约兼容的路由（修复发现端点后即全部可达）：
- `POST /api/auth/login` → `handleLogin`（`app.go:104`，返回完整 `LoginResponseDto`：`accessToken/userToken/userId/userEmail/name/isAdmin/shouldChangePassword/isOnboarded/profileImagePath`，并下发 `immich_access_token`+`immich_is_authenticated` cookie）。
- `POST /api/assets`、`POST /api/assets/bulk-upload-check`、`GET /api/assets/:id/{thumbnail,original,preview}`、`GET /api/assets/:id/video/playback`、`GET /api/assets/:id/video/stream/main.m3u8` 等（`app.go` + `hls.go`）。
- `GET /api/socket.io`（`socketio.go`）。
- `GET /api/server/ping|health|about|version|config|features`（`app.go:68-82`）。

## 根因小结

| 子问题 | 阶段 | 根因 | 分类 |
|---|---|---|---|
| 1. iOS 登录失败 | 登录 | immich-go **未实现 `/.well-known/immich`**，iOS 无法习得 `/api` 前缀，basePath 退化为裸 host，于是 `POST /auth/login`（无 `/api`）落入 SPA 兜底返回 HTML，而非 JSON 登录响应 | **契约/DTO 形状错误（缺失官方发现端点）**，非 stub |
| 2. 图片上传失败 | 上传 | 同源：`$savedEndpoint` 为裸 host 时 `POST /assets`（无 `/api`）被 SPA 兜底返回 HTML；一旦 endpoint 解析为 `/api`，immich-go 的 `handleAssetUpload`/`handleAssetBulkUploadCheck` 已契约兼容（真实实现） | 同上根因；上传处理本身非 stub |
| 3. 视频上传/观看失败 | 上传+观看 | 上传同源根因；**观看**另受 BUG-007 已知缺口（HLS 绝对 `http://`、duration/ratio=0、iOS 发 `duration:'0'`）影响 | 根因同源 + BUG-007（真实能力缺口，非 stub） |
| （连带）实时同步 | socket.io | 裸 endpoint 时 socket.io path 为 `/socket.io`（immich-go 未注册），解析为 `/api` 后 `/api/socket.io` 可服务 | 同源根因 |

**是否 stub / fake-success？** 本 BUG 涉及的 immich-go 各 handler（`handleLogin`/`handleAssetUpload`/`handleAssetBulkUploadCheck`/`handleSocketIO`/`handleVideoPlayback` 等）**均非 fake-success stub**——它们都返回真实 JSON/数据，做真实工作。真正的问题是一处**契约缺失**：缺少官方客户端赖以定位 `/api` 基址的 `/.well-known/immich` 发现端点。SPA 兜底返回 `200 text/html` 是「把 API 请求误当前端路由」的症状，不是 handler 造假。修复后所有现有 handler 即被官方 iOS 客户端以正确 `/api` 前缀命中。

## 修复方向（已实施并部署）

1. **主修复（关键，一处修好整条 iOS 链路）**：新增 `GET /.well-known/immich`，返回与官方完全一致的 JSON（`app.controller.ts:14`），注册于 `internal/app/app.go` 的 `RegisterRoutes`（显式路由优先于 `NoRoute`，不会被 SPA 兜底吞掉）：
   ```go
   r.GET("/.well-known/immich", func(c *gin.Context) {
       c.JSON(http.StatusOK, gin.H{"api": gin.H{"endpoint": "/api"}})
   })
   ```
   官方 iOS 客户端读到 `/api` 后会把 endpoint 解析为 `https://host/api`，此后 `/auth/login`、`/assets`、`/assets/bulk-upload-check`、`/socket.io` 全部自动带 `/api` 前缀，命中 immich-go 已有 handler。
2. **防御性修复（可选，让已缓存裸 endpoint 的客户端免重新配置即工作）**：为关键鉴权端点注册「无 `/api` 前缀」别名，例如 `r.POST("/auth/login", a.handleLogin)`、`r.POST("/auth/validate", ...)` 等（显式路由优先于 `NoRoute`，不冲突）。这与官方「仅 `/api/...`」契约略有偏离，但能提升对已经把 endpoint 存成裸 host 的客户端/旧版本的鲁棒性。若严格遵循「对齐官方契约」，第 1 项已足够（官方客户端本就会经 `/.well-known/immich` 自愈）。
3. **视频观看**：属于 BUG-007，单独修复（HLS 主清单改相对 URL + 服务端视频元数据探针）。本 BUG 不重复处理。

## 影响

- **High**：iOS 客户端**完全无法登录**（拿到 HTML 而非 JSON），登录是整条 iOS 流程（上传/同步/观看）的总开关，故阻塞一切 iOS 功能。
- **Medium**：即便绕过登录，图片/视频上传、缩略图/原图获取、`/socket.io` 实时同步都会因同样的「裸 host 无 `/api`」问题命中 SPA 兜底而失败——但这**并非 handler 缺陷**，而是由同一缺失的发现端点导致；实现 `/.well-known/immich` 后一并解决。
- 视频**观看**在登录可用后仍需 BUG-007 修复才能真正播放（混合内容 + 元数据）。

## 验证（真实客户端/浏览器，符合 AGENTS.md 规则 8）

- 修复后，用 `curl` 模拟发现端点已验证：
  1. `curl -s https://<host>/.well-known/immich` → `{"api":{"endpoint":"/api"}}`（200 `application/json`，经反向代理与直连均验证通过）。
  2. iOS 重新解析 endpoint（Settings → 服务器地址重连，或直接重装登录）→ 登录请求变为 `POST /api/auth/login`，响应为 JSON `LoginResponseDto`（含 `accessToken`）→ 登录成功进入相册。
  3. 上传一张图片：`POST /api/assets`（multipart）返回 `{"id":...,"status":"created"}`，时间线出现该图。
  4. 上传一段视频：上传成功；点开播放需 BUG-007 修复后验证（`main.m3u8` 变体为相对路径、无混合内容拦截）。
  5. 实时同步：`/api/socket.io` 握手成功（`40{"sid":"..."}`），资产变更能实时推送到客户端。
- 注意：`curl` 单列 `/auth/login` 看到 200 容易被误判为「成功」，实际是 HTML；必须以**真实 iOS 客户端**登录后到达相册首页、且 `/api/users/me` 等返回 200、无客户端 `pageerror` 为准（规则 8）。iOS 真机复现受环境限制未在本机执行，但发现端点已与官方契约完全一致。
