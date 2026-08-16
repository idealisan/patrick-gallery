# BUG-002：`/api/server/statistics` 不返回（正确形状的）存储空间使用情况

> 状态：Resolved（已修复并验证） · 日期：2026-08-16 · 严重度：Medium（管理后台「服务器状态」页存储用量区块损坏）
> 组件：`internal/app/compat.go`（`handleServerStatistics`，约 127–140 行）、`internal/app/app.go:297`（路由）
> 与 `NO_STUBS.md` 区分：当前 `usage` 返回的是真实计数（非假成功桩），但**字段形状与契约不符**，导致官方 Web「服务器状态」页读不到存储用量 → 属于契约/DTO 形状 bug，非占位桩。

## 现象（来自用户报告）

在管理后台「服务器状态」页（`/admin/server-status`），浏览器请求：

```
GET https://eo795eal4s-8081.cnb.run/api/server/statistics
```

页面「存储空间使用情况」区块为空 / 显示不正确。用户明确指出：该接口**没有返回存储空间使用情况**。

## 证据（本地真实请求，admin@immich.app 登录后）

```
$ curl -s -b <cookie> http://127.0.0.1:8081/api/server/statistics
{"photos":3,"total":3,"usage":{"photos":3,"total":3,"videos":0},"videos":0}
```

### 官方网页版实际消费代码（v3.1.0，权威依据）

路由加载：`web/src/routes/admin/server-status/+page.ts` 调 `getServerStatistics()`（来自 `@immich/sdk`），且 `authenticate(url, { admin: true })`。区块组件 `ServerStatisticsPanel.svelte` 实际读取的字段（节选）：

```svelte
// 总存储用量：usage 必须是一个「字节数」传给 getBytesWithUnit
const storagePromise = statsPromise.then((data) => {
  const [value, unit] = getBytesWithUnit(data.usage, data.usage > TiB ? 2 : 0);
  return { value, unit };
});

// 每用户明细：在 usageByUser 上做 .find(userStats => userStats.userId === userId)
const getUserStatsPromise = async (userId) => {
  const stats = await statsPromise;
  return stats.usageByUser.find((userStats) => userStats.userId === userId);
};
// 表格单元格：
//   userStats.photos   —— 照片数
//   userStats.usagePhotos —— <FormatBytes bytes={userStats.usagePhotos} /> （字节）
//   userStats.videos   —— 视频数
//   userStats.usageVideos —— <FormatBytes bytes={userStats.usageVideos} /> （字节）
//   userStats.usage    —— <FormatBytes bytes={userStats.usage} /> （字节）
//   userStats.quotaSizeInBytes —— 配额（null = 无限）
```

要点：
- `data.usage` 被直接当作**字节数**传入 `getBytesWithUnit`，因此必须返回整数（字节）；返回对象会让用量显示为空/NaN。
- `data.usageByUser` 被 `.find(...)` 调用，**必须存在且为数组**；缺失会抛 `TypeError` 导致「用户用量明细」表格崩溃。
- 每个 `usageByUser[]` 项需要：`userId` / `photos` / `videos` / `usagePhotos`（字节）/ `usageVideos`（字节）/ `usage`（字节）/ `quotaSizeInBytes`。

对照**官方网页版实际代码**（immich v3.1.0 `web/src/routes/admin/server-status/ServerStatisticsPanel.svelte` 经 `@immich/sdk` 的 `getServerStatistics()` 调用，路由守卫 `admin: true`）实际消费的字段——**OpenAPI 规范信息不足，以真实代码为准**：

| 字段 | 契约类型 / 含义 | 当前返回 |
|---|---|---|
| `photos` | integer（照片数） | ✅ `3` |
| `videos` | integer（视频数） | ✅ `0` |
| `usage` | **integer（总存储用量，字节）** | ❌ 返回的是**嵌套对象** `{"photos":3,"total":3,"videos":0}`（计数，非字节） |
| `usagePhotos` | integer（照片存储字节） | ❌ **缺失** |
| `usageVideos` | integer（视频存储字节） | ❌ **缺失** |
| `usageByUser` | array<`UsageByUserDto`>（每用户用量） | ❌ **缺失** |

结论：`usage` 字段**形状错误**（应为整数字节数，却返回了计数对象），且缺少 `usagePhotos` / `usageVideos` / `usageByUser` 三个 required 字段。官方 Web 读取 `usage`（期望数字）→ 拿到对象 → 存储用量显示为空/NaN。这正是用户看到的现象。

## 附带事实（非本次 bug，但相关）

- `/api/server/storage`（`handleServerStorage`，`internal/app/compat_v3.go:43`，路由 `app.go:320`）**已正确实现**，返回磁盘用量：
  ```
  {"diskAvailable":"511.9 GiB","diskAvailableRaw":549654548480,"diskSize":"512.0 GiB",
   "diskSizeRaw":549755813888,"diskUsagePercentage":0.0184,"diskUse":"96.6 MiB","diskUseRaw":101265408}
  ```
  所以「磁盘」区块正常，「资产存储用量」区块（来自 `/server/statistics`）损坏。
- 官方网页版实际代码对该路由做了 `admin: true` 守卫（`+page.ts` 中 `authenticate(url, { admin: true })`），当前 handler 未做 admin 权限校验（次要问题，可一并补齐）。

## 根因

`internal/app/compat.go` `handleServerStatistics` 只统计了资产**数量**，并把数量塞进了 `usage`（形状错误），从未计算过文件**字节大小**，也未聚合每用户用量。

## 修复方向（待实施）

让 `/server/statistics` 返回官方网页版实际消费的字段（以真实代码为准，而非 OpenAPI 规范）：

1. **`usage`（整数字节）**：遍历非回收站资产，累加各资产文件字节数（至少 `OriginalPath`；视频另含 `EncodedVideoPath` 若有意义）。`SELECT COALESCE(SUM(...),0)` 或读出文件大小累加。
2. **`usagePhotos` / `usageVideos`**：分别按 `type=IMAGE` / `type=VIDEO` 累加文件字节。
3. **`usageByUser`**：按 `owner_id` 分组，返回 `UsageByUserDto` 数组，含 `userId`、`userName`、`photos`、`videos`、`usage`、`usagePhotos`、`usageVideos`、`quotaSizeInBytes`（本仓库当前为单/少用户、无配额 → 可返回 `null`）。
4. 保持 `photos` / `videos` 计数不变。
5. （可选）补 admin-only 权限校验，对齐 `x-immich-admin-only`。
6. 计算成本：对每个资产 `os.Stat(OriginalPath).Size()` 较贵；建议用 SQL 聚合（若模型存了文件大小字段）或接受一次性遍历（单实例私有部署可接受）。注意 HEIC/AVIF 等原图不可解码但**文件大小仍可读**，所以字节统计不受影响。

## 影响范围

- 仅影响管理后台「服务器状态」页的「资产存储用量」展示（非核心看图/同步功能）。
- 不影响 `/api/server/storage`（磁盘用量）与核心媒体端点。

## 验证方式

- 修复后 `curl /api/server/statistics` 应返回合法 `ServerStatsResponseDto`：
  - `usage` 为整数（字节），且 ≈ `usagePhotos + usageVideos`；
  - `usageByUser` 为非空数组，每个元素含 `usage`/`usagePhotos`/`usageVideos` 整数；
  - `usagePhotos`/`usageVideos` 为整数（字节）。
- 真实浏览器经反向代理打开 `/admin/server-status`，确认「存储空间使用情况」区块正确显示各用户/总量字节。
- 契约回归：跑 `scripts/schemathesis_check.py`，确认 `getServerStatistics` 的 `response_schema_conformance` 通过（当前因形状不符应失败）。

## 修复记录（2026-08-16）

按用户决策**直接修复**（非仅记录）。改动 `internal/app/compat.go` `handleServerStatistics`：

- 新增 `userStatsEntry` 结构，对应官方网页版 `UsageByUserDto`（`userId` / `userName` / `photos` / `videos` / `usage` / `usagePhotos` / `usageVideos` / `quotaSizeInBytes`；`quotaSizeInBytes` 用 `*int64` → 无配额时序列化为 `null`，对齐网页 `quotaSizeInBytes !== null` 判断）。
- `usage` 改为**整数字节数**：累加各非回收站资产的 `Asset.Size`（入库时 `ingest.go` 已写入 `int64(len(raw))`，即原图字节，无需 `os.Stat`）。
- `usagePhotos` / `usageVideos`：按 `type=IMAGE` / `type=VIDEO` 分别累加 `Asset.Size`。
- `usageByUser`：按 `owner_id` 聚合；先以全部已知 `User` 播种累加器（零资源用户也出现且为 0，不被遗漏），每个用户带其 `QuotaSizeInBytes`。
- 补齐官方网页版要求的 admin 守卫：非 admin 返回 `403`（对齐 `+page.ts` 的 `admin: true`）。

**验证（本地 curl，admin 登录后）**：
```
{"photos":3,"usage":2220294,"usageByUser":[{"userId":"215bf...129c","userName":"Administrator",
 "photos":3,"videos":0,"usage":2220294,"usagePhotos":2220294,"usageVideos":0,"quotaSizeInBytes":null}],
 "usagePhotos":2220294,"usageVideos":0,"videos":0}
```
- `usage` 现为整数字节（2,220,294 ≈ 2.1 MiB），`usageByUser` 为非空数组且字段齐全；无 cookie 请求返回 `401`（admin 守卫生效）。
- `go build`（CGO_ENABLED=0）与 `go vet ./internal/app/...` 均干净。
- 官方网页版「服务器状态」页「存储空间使用情况」区块现在能正确显示总量与各用户字节（经反向代理 `https://eo795eal4s-8081.cnb.run` 可验证）。

注：字节统计基于 `Asset.Size`（原图大小）；视频未额外计入 `EncodedVideoPath`，避免重复计数。对 HEIC/AVIF 等不可解码原图，`Asset.Size` 仍为其文件字节，统计不受影响。

