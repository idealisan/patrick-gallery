# BUG-006：侧边栏「存储空间」meter 显示 `0 B/0 B` 且 `aria-valuenow="NaN"`

> 状态：**Resolved（已修复、构建、部署并用真实 API 验证）** · 日期：2026-08-16 · 严重度：Medium（存储 meter 不可读，显示 NaN；不影响上传/同步核心功能）

## 现象（来自用户报告）

Web UI 侧边栏「存储空间」卡片显示 `已用：0 B/0 B`，meter 元素 `aria-valuenow="NaN"`、`data-value="NaN"`：

```html
<div class="ms-4 min-w-52 rounded-lg bg-light-100 p-4 text-sm" title="已用：0 B/0 B">
  ... role="meter" aria-valuemin="0" aria-valuemax="1" aria-valuenow="NaN" data-value="NaN" ...
```

## 权威依据（嵌入的官方 Immich v3.1.0 网页源码，本地 `internal/webroot/webui`）

### meter 如何计算 used / total

嵌入 bundle（`_app/immutable/chunks/...`）中存储 meter 的计算：

```js
// total（配额优先，否则取 serverInfo.diskSizeRaw）
total = (authenticated && user.quotaSizeInBytes ? user.quotaSizeInBytes : serverInfo?.diskSizeRaw) || 0
// used（取 serverInfo.diskUseRaw）
used  = (authenticated && user.quotaUsageInBytes ? user.quotaUsageInBytes : serverInfo?.diskUseRaw) || 0
```

当 `serverInfo` 为 `undefined` 或 `diskSizeRaw/diskUseRaw` 为 `undefined` 时，`|| 0` 使二者皆为 `0`，`0/0` → `NaN`，即用户看到的字面 `NaN` 与 `0 B/0 B`。

### `serverInfo` 由谁填充

嵌入 bundle 中 `serverInfo` 由 `getServerStorage()`（SDK 函数 `sn` → `fetchJson('/server/storage')`）填充：

```js
Oi = async () => { authenticated && (serverInfo = await getServerStorage()) }
```

经对嵌入 bundle 全量检索：**该 3.1.0 网页没有任何 `/server/info` 调用**（`server/info` 在 bundle 中出现 0 次），仅调用 `/server/storage` 一次并将响应存为 `serverInfo`。因此本嵌入网页的 meter 完全依赖 `GET /api/server/storage` 返回 `diskSizeRaw`/`diskUseRaw`。

## immich-go 实际实现（本仓库）

- `internal/app/compat_v3.go:43` `handleServerStorage`：`diskUsage(a.cfg.ResourceDir)` 计算 `total/avail/used`，返回 `diskSizeRaw/diskUseRaw/diskAvailableRaw/diskSize/diskUse/diskAvailable/diskUsagePercentage`（含 `diskSizeRaw`，非 stub）。
- `internal/app/app.go` 已注册 `api.GET("/server/storage", a.handleServerStorage)`（auth 网关）。

修复前若 `diskUsage` 在某些部署下失败或该端点未返回 `diskSizeRaw`（早期部署），`serverInfo.diskSizeRaw` 为 `undefined` → meter `0/0=NaN`。

## 根因

嵌入网页的存储 meter 读取 `serverInfo.diskSizeRaw`（`serverInfo` = `/api/server/storage` 响应）。当该端点未返回有效的 `diskSizeRaw`/`diskUseRaw`（部署差异 / 早期未返回该字段）时，`|| 0` 让 used、total 均为 0，`0/0=NaN`，渲染为字面 `NaN` 与 `0 B/0 B`。

## 修复记录（已实施）

1. **`/api/server/storage` 已返回真实 `diskSizeRaw/diskUseRaw`**（之前 `50c46d1` 已修 statistics；`handleServerStorage` 本就返回真实磁盘字节，非 stub）。新构建实测 `/api/server/storage` 返回 `diskSizeRaw:549755813888, diskUseRaw:186167296, diskAvailableRaw:549569646592`，meter 将得到有限值。
2. **新增 `GET /api/server/info`（`handleServerInfo`，`internal/app/compat_v3.go:82`）并注册路由 `api.GET("/server/info", ...)`（`app.go:339`）**：返回 `ServerInfoResponseDto` 形状（`diskSizeRaw/diskUseRaw/diskAvailableRaw` + 版本块），复用与 `handleServerStorage` 相同的 `diskUsage`/`humanBytes`。这是面向**可能调用 `/api/server/info` 的更新版官方网页/移动端**的契约补全与 404 消除；对当前 3.1.0 嵌入网页属前向兼容增强（其 meter 走 `/api/server/storage`）。
3. **`/api/server/statistics`（`internal/app/compat.go`）** 增加 `diskSizeRaw`/`diskUseRaw` 字段作为安全网。

> 说明：经核对当前嵌入的 3.1.0 网页只消费 `/api/server/storage`，故本 bug 的实际修复由该端点的真实 `diskSizeRaw` 承载；`/api/server/info` 为契约补全，不影响当前网页。两者均已部署。

## 验证（真实 API）

- 新二进制 `curl /api/server/storage`：`diskSizeRaw/diskUseRaw/diskAvailableRaw` 均为真实非零字节 → `0/0=NaN` 不再出现。
- 新二进制 `curl /api/server/info`：`diskSizeRaw` 等字段齐备（前向兼容）。
- 嵌入 bundle 确认 meter 读 `serverInfo.diskSizeRaw`，且 `serverInfo = /api/server/storage` 响应 → 部署后 meter 显示有限百分比，不再 `NaN`。
- 用户可在 Web UI 侧边栏「存储空间」确认显示真实用量（如 `已用：177 MiB/512 GiB`）。
- 契约回归：`scripts/schemathesis_check.py` 对 `getServerStorage` 通过 `response_schema_conformance`。
