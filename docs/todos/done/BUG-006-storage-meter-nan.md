# BUG-006：侧边栏「存储空间」meter 显示 `0 B/0 B` 且 `aria-valuenow="NaN"`，同时版本显示「未知」

> 状态：**Resolved（已修复、构建、部署并用真实浏览器验证）** · 日期：2026-08-16 · 严重度：Medium（存储 meter 与版本标签不可读；不影响上传/同步核心功能）

## 现象（来自用户报告 + 真实浏览器复现）

Web UI 侧边栏同时出现两个问题：

1. 「存储空间」卡片显示 `已用：0 B/0 B`，meter `aria-valuenow="NaN"`：
   ```html
   <div class="ms-4 min-w-52 rounded-lg bg-light-100 p-4 text-sm" title="已用：0 B/0 B">
     ... aria-valuenow="NaN" ...
   ```
2. 服务器状态显示 `<p class="text-red-500">未知</p>`（应为版本号如 `v3.1.0`）。

## 权威依据（官方 Immich v3.1.0 源码，本地 `/root/immich-src`）

### 子问题 1（存储 meter NaN）—— 根因在 `/api/users/me` 的 quota 字段缺失

`web/src/lib/components/shared-components/side-bar/StorageSpace.svelte`：

```js
let hasQuota = $derived(authManager.user.quotaSizeInBytes !== null);   // ← 关键：用 !== null 判断
let availableBytes = $derived(
  (hasQuota && authManager.authenticated
    ? authManager.user.quotaSizeInBytes
    : userInteraction.serverInfo?.diskSizeRaw) || 0,
);
let usedBytes = $derived(
  (hasQuota && authManager.authenticated
    ? authManager.user.quotaUsageInBytes
    : userInteraction.serverInfo?.diskUseRaw) || 0,
);
```

- `userInteraction.serverInfo` 由 `requestServerInfo` → `getStorage()` → `GET /api/server/storage` 填充，返回真实 `diskSizeRaw`/`diskUseRaw`（本仓库 `handleServerStorage` 早已正确返回，**非根因**）。
- 但 `hasQuota` 使用 `quotaSizeInBytes !== null`。当 `/api/users/me` 的 JSON 中**完全没有 `quotaSizeInBytes` 字段**时，JS 侧为 `undefined`，而 `undefined !== null` 为 **`true`** → 误判用户「有配额」→ `availableBytes = quotaSizeInBytes (undefined → 0)`、`usedBytes = quotaUsageInBytes (undefined → 0)` → `0/0 = NaN`、`getByteUnitString(0)` = `0 B` → 用户看到的 `0 B/0 B` 与 `NaN`。

根因在本仓库 `User` 模型：

```go
// internal/app/models.go
QuotaSizeInBytes *int64 `gorm:"type:bigint" json:"quotaSizeInBytes,omitempty"`  // ← omitempty：nil 时整字段被丢弃 → JSON 无此键
```

`handleMe`（`internal/app/user.go`）直接 `c.JSON(200, u)` 返回原始 `User` 模型，于是无配额用户序列化后**根本没有 `quotaSizeInBytes` 键**（且模型压根没有 `quotaUsageInBytes` 字段），双双为 `undefined`。

### 子问题 2（版本「未知」）—— 根因在 websocket 未推送 server version

`web/src/lib/components/shared-components/side-bar/ServerStatus.svelte:38`：

```js
let version = $derived($serverVersion ? semverToName($serverVersion) : null);
```

`$serverVersion` 来自 `web/src/lib/stores/websocket.ts:70`：

```js
.on('on_server_version', (serverVersion) => websocketStore.serverVersion.set(serverVersion))
```

即仅当服务端通过 websocket 推送 `on_server_version` 事件时，`$serverVersion` 才有值；否则为 `null` → 渲染 `<p class="text-red-500">未知</p>`。

本仓库 `internal/app/socketio.go` 的 `socketIOWebsocket` / `socketIOPolling` 只转发内部 `EventBus` 事件，**从未在客户端连接时主动推送 `on_server_version`**，故官方 web 永远显示「未知」。

## 修复记录（已实施并部署）

### 子问题 1
- `internal/app/models.go`：`QuotaSizeInBytes` 去掉 `omitempty`（nil 时序列化为 `null` 而非省略）；新增非持久化字段 `QuotaUsageInBytes *int64 gorm:"-" json:"quotaUsageInBytes"`（nil → `null`，有用量时填计算值）。
- `internal/app/user.go` `handleMe`：返回前调用已有的 `a.userQuotaUsage(uid)` 填充 `QuotaUsageInBytes`，使 `/api/users/me` 始终带 `quotaSizeInBytes`（null 或值）与 `quotaUsageInBytes`（null 或值）。这样 `hasQuota` 在无配额时为 `false`，meter 正确回退到 `serverInfo.diskSizeRaw`。

### 子问题 2
- `internal/app/socketio.go`：新增 `socketIOServerVersion()` 构造 `on_server_version` 事件（`ServerVersionResponseDto = {major,minor,patch,prerelease:null}`，取自 `cfg.CompatMajor/Minor/Patch`）；在 websocket 连接建立后（open 报文之后、以及收到 `40` 命名空间连接后）主动推送；polling 传输则在客户端 POST `40` 后于下一次长轮询 GET 优先返回该事件。

## 验证（真实浏览器，符合 AGENTS.md 规则 8）

用 `chromium` 经反向代理公网 URL 登录后检查：

- 存储 meter：`title="已用：214.922 MiB/512 GiB"`、`aria-valuenow` 为有限数值（不再 NaN）。
- 版本标签：`text-red-500` 列表为空；侧边栏文本为 `服务器在线 v3.1.0`（不再「未知」）。
- `curl /api/users/me`：`"quotaSizeInBytes":null`、`"quotaUsageInBytes":26738628`。
- `curl /api/server/storage`：`diskSizeRaw/diskUseRaw` 为真实非零字节（此前已正确）。
- 无 `pageerror`。

## 备注

早期（错误的）分析曾把根因归咎于 `serverInfo.diskSizeRaw` 缺失，并新增了 `GET /api/server/info`（`handleServerInfo`）作为前向兼容增强。该增强保留，但**本 bug 真正修复点是 `/api/users/me` 的 quota 字段序列化与 websocket 的 `on_server_version` 推送**——这两点由真实浏览器复现（而非仅 curl）才得以定位。
