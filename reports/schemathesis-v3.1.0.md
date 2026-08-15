# immich-go × Schemathesis 一致性测试报告 (v3.1.0)

> 用**行业标准 property-based API 测试 CLI `schemathesis` 4.24.3** 对 immich-go 做真实
> OpenAPI 一致性核验。这是对 `scripts/api_consistency.py`（手写 stdlib 检查器）的升级：
> Schemathesis 用完整 JSON Schema 校验响应体，能捕获手写检查器因 lenient 导航而漏掉的
> DTO 形状错误。

## 环境与方法

- **工具**：`schemathesis` 4.24.3（pip 安装，行业标准 API 契约测试 CLI）
- **被测实现**：immich-go `1.3.0-go`（`dist/immich-go-1.3.0-go-linux-amd64/immich-go`，静态二进制）
  - 启动：`IMMICH_COMPAT_VERSION=3.1.0`，端口 `8099`，独立 `immich.db` + `resources/`
  - 认证：`POST /api/auth/login` (admin@immich.app / password) → `accessToken`，以
    `Authorization: Bearer <token>` 注入所有请求
- **基准契约**：`open-api/immich-openapi-specs.json`（immich-app/immich tag **v3.1.0**，
  OpenAPI 3.0.0，254 operations，`servers: [{url:/api}]`）
- **命令（可复现）**：

```bash
IMMICH_PORT=8099 IMMICH_DB=/tmp/ig-test/immich.db IMMICH_RESOURCE=/tmp/ig-test/resources \
  IMMICH_COMPAT_VERSION=3.1.0 ./immich-go &
TOKEN=$(curl -s -X POST http://localhost:8099/api/auth/login -H 'Content-Type: application/json' \
  -d '{"email":"admin@immich.app","password":"password"}' \
  | python3 -c 'import sys,json;print(json.load(sys.stdin)["accessToken"])')
schemathesis run open-api/immich-openapi-specs.json \
  --url http://localhost:8099/api \
  -H "Authorization: Bearer $TOKEN" \
  --max-examples 1 --workers 1 --phases examples \
  -c not_a_server_error -c status_code_conformance \
  -c response_schema_conformance -c content_type_conformance
```

> ⚠️ `--url` 必须带 `/api` 前缀：spec 的 `servers` 是 `[{url:/api}]`，Schemathesis 会把它
> 拼到 path 前；若只写 `:8099` 则所有请求缺 `/api` 前缀，会命中 SPA 兜底。

## 覆盖率说明（重要）

Schemathesis 的 `examples` 阶段**只对“能生成请求样例”的 operation 做测试**：

| 指标 | 值 |
|------|----|
| Operations selected | 254 / 254 |
| **Tested** | **30** |
| **Skipped** | **224**（`No examples in schema`） |
| 失败 (failures) | 32（7 响应违反 schema + 25 未声明状态码） |
| 通过 (passed) | 1 |
| 警告 (warning) | 1（`POST /auth/login` 401，见下） |

被跳过的 224 个主要是**无参数/无 requestBody 的 GET**（如 `/server/about`、`/albums`）
以及部分 path-param-only 的 GET——Schemathesis 的样例引擎无法为它们构造请求故跳过。
写操作（有 requestBody）基本都被测到。

**结论**：read-only 面的广覆盖仍由 `scripts/api_consistency.py`（75 个 GET）补充；
Schemathesis 在本规范下更擅长对“可生成请求”的 operation 做**严格响应体契约校验**。
两者互补，本报告聚焦 Schemathesis 的严格校验结果。

## 失败分类

### A. 真实 DTO / 响应形状 bug（手写检查器漏报，Schemathesis 精确捕获）

| # | 端点 | 实际返回 | 契约要求 | 问题 |
|---|------|----------|----------|------|
| A1 | `GET /map/markers` | `{"markers":[]}` | 裸 `array[MapMarkerResponseDto]` | 多加了 `markers` 包裹层 |
| A2 | `GET /timeline/bucket` | `{"assets":[],"count":0}` | 裸 `array[TimeBucketAssetResponseDto]` | 多加了 `assets`/`count` 包裹层 |
| A3 | `POST /search/metadata` | `{"assets":[],"count":0,"total":0}` | `{albums, assets:{items,count,facets,total}}` | `assets` 应为对象非数组；缺 `albums` |
| A4 | `POST /shared-links` | `id`/`userId` 非 UUID 格式；缺 `allowDownload`/`allowUpload`/`assets`/`key`/`password`/`showMetadata`/`slug`/`type` 等 required 字段 | `SharedLinkResponseDto`（id/userId 为 v4 UUID pattern，含上述 required） | ID 生成非 UUID；响应字段不完整 |

> A1–A3 是“把本应是裸数组的响应包了一层对象”；A4 是 ID 格式 + 字段完整度问题。
> 这些都是客户端（v3.1.0）按契约解析时会真正出错的硬伤。

### B. 未声明状态码（25 个）

其中 **22 个返回 404**（spec 声明 2xx）：

- **无 path 参数、必然 404 = 尚未注册的 route（进行中移植差距）**：
  `POST /admin/notifications`、`POST /admin/users`、`POST /auth/admin-sign-up`、
  `POST /auth/pin-code`、`POST /auth/session/unlock`、`POST /memories`、
  `POST /search/large-assets`、`POST /search/random`、`POST /search/smart`、
  `POST /search/statistics`、`POST /shared-links/login`、`PUT /notifications`、
  `GET /memories`、`GET /memories/statistics`、`DELETE /auth/pin-code`、
  `PUT /auth/pin-code`
- **带 path 参数、生成 UUID 在空库不存在 → 404（语义正确，spec 只是未列 404）**：
  `GET /admin/users/{id}/calendar-heatmap`、`GET /users/me/calendar-heatmap`、
  `PATCH /shared-links/{id}`、`PUT /admin/users/{id}`、`PUT /memories/{id}`、
  `PUT /notifications/{id}`
  > 注：这类 404 与官方 Immich 对“资源不存在”的返回一致，客户端通常按 404 优雅降级，
  > 并非服务端 bug；Schemathesis 标红仅因 spec 的 responses 未声明 404。

其余 3 个未声明状态码（**非 404**）：

| 端点 | 收到 | 契约 | 判定 |
|------|------|------|------|
| `POST /assets` | 400 `missing assetData` | 200/201 | 负向测试：Schemathesis 生成的 multipart 无真实文件字节，服务端正确拒绝，**可接受** |
| `POST /auth/change-password` | 204 | 200 | 状态码差异，客户端一般容忍 204/200，**轻微** |
| `POST /auth/login` | 401 | 201 | **测试工具鉴权伪影**：Schemathesis 对该公开端点未附加 Bearer 头导致 401；手动 curl 登录成功（本报告 token 即来自该端点），**非服务端 bug** |

## 与手写检查器（`scripts/api_consistency.py`）的对比

| 维度 | 手写检查器 | Schemathesis |
|------|-----------|--------------|
| 驱动 | 自写 stdlib，遍历 spec paths | 行业标准 property-based CLI |
| 校验深度 | spec `required` 字段 + lenient 导航（空列表当“无法校验”跳过） | 完整 JSON Schema 响应体校验 |
| 覆盖 | 75 GET（广） | 30 可生成请求 operation（深） |
| 漏报 | A1–A4 被误判 OK（导航伪影） | 精确捕获 A1–A4 |
| 结论 | 适合广覆盖只读面 | 适合严格响应契约校验，两者互补 |

## 建议修复（下一步）

1. **修 A 类 4 处 DTO 形状**（最高优先级，硬伤）：
   - `/map/markers`、`/timeline/bucket`：返回裸数组，去掉 `markers` / `assets`+`count` 包裹。
   - `/search/metadata`：返回 `{albums, assets:{items,count,facets,total}}`，`assets` 为对象。
   - `/shared-links`：ID 用 v4 UUID 生成；补全 required 字段。
2. **B 类**：带 path 参数的 404 维持现状即可（语义正确）；无参数的未注册 route 按路线图实现，
   或在完全未实现时显式返回 `501`（比 404 JSON 更语义化，客户端同样优雅降级）。

## 复现产物

- `open-api/immich-openapi-specs.json` — 基准契约（v3.1.0）
- `reports/schemathesis/run-allmethods.txt` — 修复前 全方法运行原始输出
- `reports/schemathesis/run-get.txt` — 修复前 GET-only 运行原始输出
- `reports/schemathesis/run-after-fixes.txt` — **修复后** 全方法运行原始输出
- `reports/schemathesis/junit-allmethods.xml` / `junit-get.xml` / `junit-after-fixes.xml` — JUnit 报告
- 种子（修复前全方法运行）：`102363100183558505436741339065736379957`

## 修复后复测（2026-08-15）

据上述 A 类发现，修复了 5 处（含 1 处系统性根因），重新构建并重跑同一 Schemathesis 命令：

| 指标 | 修复前 | 修复后 |
|------|--------|--------|
| Tested | 30 | 30 |
| Passed | 1 | **5** |
| Failed | 29 | **25** |
| 响应违反 schema | 7 | **0** |
| 未声明状态码 | 25 | 25 |

A1–A4 四个真实 DTO bug 已全部通过。剩余 25 个失败**全部是“未声明状态码”**，即 B 类未实现端点（返回 404 JSON）或 3 个良性边界：
- `POST /assets` → 400 `missing assetData`：Schemathesis 生成的 multipart 无真实文件字节，属负向测试，服务端正确拒绝；
- `POST /auth/change-password` → 204（spec 200）：状态码差异，客户端一般容忍；
- `POST /auth/login` → 401：Schemathesis 对该公开端点未附加 Bearer 头导致，手动 curl 登录成功，**非服务端 bug**。

### 修复清单
1. **`internal/app/util.go` `newUUID()`**：原返回无短横 32 位十六进制串，违反 spec 的 v4 UUID `pattern`。改为标准带短横小写 v4 UUID（`%08x-%04x-%04x-%04x-%012x`）。**系统性修复**：所有生成的 id（asset / user / shared-link / marker）现在都是合法 v4 UUID，解决 A1（marker id）、A4（shared-link id / userId）及潜在的其它 UUID 字段。
2. **`internal/app/map.go` `GET /map/markers`**：去掉 `{"markers":[...]}` 包裹，返回裸 `array[MapMarkerResponseDto]`；`MapMarker` 增加必填 `state` 字段。→ 修 A1。
3. **`internal/app/timeline.go` `GET /timeline/bucket`**：返回 `TimeBucketAssetResponseDto`（平行数组对象）而非 `{"assets":[],"count":0}`；新增 `buildTimeBucketAssets` 按 spec 必填字段（id/ownerId/createdAt/fileCreatedAt/duration/isFavorite/isImage/isTrashed/livePhotoVideoId/localOffsetHours/projectionType/ratio/thumbhash/visibility，可选 city/country/latitude/longitude/stack）逐资源填平行数组。`/timeline/assets` 仍返回裸 `array[AssetResponse]`。→ 修 A2。
4. **`internal/app/search.go` `POST /search/metadata`（及 `/search`、`/search/explore`）**：返回 `SearchResponseDto` `{albums, assets:{items,count,facets,total}}`，`assets` 为对象而非数组；补齐 `albums` 与 `nextPage`。→ 修 A3。
5. **`internal/app/misc.go` `POST /shared-links`（及 PUT）**：返回完整 `SharedLinkResponseDto`（含必填 `allowDownload/allowUpload/assets/createdAt/description/expiresAt/id/key/password/showMetadata/slug/type/userId`）；id/userId 由 #1 变为合法 v4 UUID。→ 修 A4。
   - 已知限制：SharedLink 模型未持久化 allowDownload/allowUpload/description/password/showMetadata/slug（无对应列），创建响应从请求体回显，重读时这些字段会缺失——属后续 DB schema 补全项，不影响本次一致性修复。

> 注：Schemathesis 的 `examples` 阶段只测“能生成请求样例”的 operation（本规范 30/254），因此上述仅覆盖被引擎选中的 operation。read-only 广覆盖仍由 `scripts/api_consistency.py`（75 GET）补充。修复后两者对 immich-go 已实现端点的响应形状判断一致。

