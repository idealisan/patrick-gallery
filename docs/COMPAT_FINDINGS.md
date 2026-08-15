# 与官方 Immich 客户端的兼容性核查（v3.1.0）

> 本文记录对官方仓库 `immich-app/immich@v3.1.0`（与 immich-go 的 `IMMICH_COMPAT_VERSION`
> 一致）**真实客户端代码**的比对结果，以及已修复的实时同步兼容性问题。
> 比对方法：sparse 克隆官方 `web/src`，读取 `lib/stores/websocket.ts`、
> `lib/managers/event-manager.svelte.ts`、`lib/utils/file-uploader.ts`、
> `lib/managers/auth-manager.svelte.ts`，交叉核对 `internal/app/*`。

---

## 1. 已修复（🔴 高级）

### 1.1 Socket.IO 实时事件名大小写不匹配

- **现象**：官方 web/mobile 客户端通过 `io({ path: '/api/socket.io', transports: ['websocket'] })`
  订阅的是 **snake_case** 事件名（`web/src/lib/stores/websocket.ts`）：
  `on_asset_delete` / `on_asset_update` / `on_asset_trash` / `on_user_delete` /
  `on_new_release` / `on_session_delete` / `on_config_update` / `on_server_version` /
  `on_person_thumbnail` / `on_asset_hidden` / `on_asset_restore` / `on_asset_stack_update` /
  `on_upload_success` / `on_notification`。
- **原因**：旧 `internal/app/socketio.go` 的 `socketIOEventName` 把它映射成了
  **camelCase**（`onAssetDelete` 等）。Socket.IO 事件名是精确字符串匹配，
  `.on('on_asset_delete')` 永远收不到 `onAssetDelete`。
- **影响**：官方客户端的实时刷新（上传成功、资产/相册变更、回收站、用户删除、
  版本/配置更新、会话踢出）**全部不触发**。传输层（Engine.IO v4 over websocket，
  `0`/`40`/`42`/`2`/`3` 帧）实现正确，握手能建立，问题仅在事件名。
- **修复**：`socketIOEventName` 改为返回 snake_case（含 `user.delete`、
  `config.update`、`server.version`、`new_release`、`session.delete`、
  `person.thumbnail`、`notification` 等全部客户端订阅名）。

### 1.2 删除/回收站事件的 payload 形状

- **现象**：官方客户端把 `on_asset_delete` / `on_asset_trash` 的 payload **直接作为
  `string[]`（资产 id 数组）** 转发给本地 store（`AssetsDelete: [string[]]`）。
  旧实现发送的是 `{"ids":[...]}`（对象），客户端收到后会把 `{ids:[...]}` 当数组用，
  删除/回收站实时刷新会失效或错乱。
- **修复**：新增 `socketIOPayload(typ, payload)`，对 `asset.delete` / `asset.trash` /
  `asset.restore` 提取 `payload["ids"]` 作为裸数组发送。**该变换只作用于 Socket.IO
  线格式**，plain `/api/events` websocket 仍使用内部 `{"ids":[...]}` 形状，自带 SPA 不受影响。

### 1.3 补齐缺失的实时事件 emit

- `user.delete`：在 `handleAdminDeleteUser`（`admin_users.go`）软删除用户后 emit，
  官方客户端据此刷新/登出该用户。
- `config.update`：在 `handleSystemConfigUpdate`（`misc.go`）保存系统配置后 emit，
  官方客户端据此重新拉取配置。

---

## 2. 待修复（🟠 中级，本次未改）

### 2.1 上传端点不幂等

`handleAssetUpload`（`asset.go`）永远返回 `{"id":..,"status":"created"}`，
不按 `deviceAssetId`+`deviceId`/checksum 去重返回 `status:"duplicate"`。
官方客户端依赖 `status:"duplicate"`（带现有 id）做幂等；绕过预检或并发上传时
会产生重复资产。缓解：客户端上传前会调 `POST /assets/bulk-upload-check` 预检
（已实现），常规流程不会重复；但上传路径本身仍非幂等。

### 2.2 bulk-upload-check 去重维度与官方不同

`handleAssetBulkUploadCheck`（`asset.go`）按 **checksum（内容）** 判定；
官方按 **deviceAssetId+deviceId（设备身份）** 判定。同一文件从第二台设备上传时，
immich-go 会误判为 duplicate 拒绝（官方会接受）。单设备/单人场景无感，多设备场景
会出现“新设备传不上去”。建议改为以 `deviceAssetId`+`deviceId` 为主判定。

### 2.3 资产 update / upload_success 的 payload 为 id-only

`asset.create` / `asset.update` 经 Socket.IO 发送 `{"id":...}`。官方客户端期望完整
`AssetResponseDto`，但其事件处理通常以 id 触发重新拉取，故可接受；若要 100% 对齐，
应在 emit 时附带完整资产 DTO。

### 2.4 相册实时事件客户端未订阅

官方 web 客户端的 `websocket.ts` **不订阅任何 `on_album_*` 事件**（仅 web 端；
移动端可能不同）。因此即便 immich-go 正确发出 `on_album_update` 等，web 端的相册
实时刷新仍不会触发（属上游客户端行为，非 immich-go 单侧可解）。

---

## 3. 已确认兼容（🟢）

| 项 | 结论 |
|----|------|
| 登录响应字段 | `handleLogin` 返回 `accessToken/userId/userEmail/name/isAdmin/shouldChangePassword`，与官方 `LoginResponseDto` 一致 |
| 上传 multipart 字段 | 客户端发 `asset`(JSON)+`assetData`(file)；immich-go 正确读取（实测上传成功） |
| `/server/features` 与 `/server/config` 标志 | 返回的 flags 集与官方 `FeatureFlagsResponseDto` 一致，客户端据此开关节点功能 |
| 资产响应 DTO 形状 | `GET /assets/:id` 返回完整 `AssetResponseDto`（exifInfo/people/tags/owner/thumbhash/visibility…） |

---

## 4. 结论

- 修复后，官方 web/mobile 客户端的**核心实时刷新**（上传成功、资产变更、回收站、
  用户被删、配置热更新）可正常推送；版本提示/会话踢出/人脸缩略图等需在对应后端能力
  落地后补 emit（映射已预留）。
- 传输层 Engine.IO v4 握手实现正确，无需改动。
- _upload 幂等 / 去重维度_（§2.1–2.2）为后续待办，建议在多用户/多设备场景前修复。
