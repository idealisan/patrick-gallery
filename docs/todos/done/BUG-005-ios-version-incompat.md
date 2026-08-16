# BUG-005：iOS 客户端 3.1.0 连接服务器提示「版本不兼容」

> 状态：Resolved（已修复并本地验证） · 日期：2026-08-16 · 严重度：High（阻止官方 iOS 客户端连接，影响核心「手机 APP 备份/浏览」闭环）
> 组件：`internal/app/app.go`（`RegisterRoutes` 路由注册）、`internal/app/misc.go:608`（`handleServerConfig` 硬编码版本）
> 与 `NO_STUBS.md` 区分：本问题不是「假成功桩」，而是 **路由路径不匹配官方 iOS 客户端的启动轮询路径**，导致客户端拿到的不是版本 JSON 而是 SPA 的 HTML，解析失败报「版本不兼容」。

## 现象（来自用户报告）

官方 Immich **iOS 客户端（自报版本 3.1.0，`User-Agent: immich-ios/3.1.0`）** 尝试连接服务器时，提示**「版本不兼容」**（`version incompatible`）并拒绝连接。

用户明确要求：**默认情况下（即便不带任何环境变量），服务器程序的兼容版本必须与该 iOS 客户端 3.1.0 兼容。**

## 根因（经 iOS 端真实抓包 HAR 确凿证明）

用户提供了 iOS 端登录时的抓包文件 `Stream-eo795eal4s-8081.cnb.run-2026-08-16 11:27:59.har`。其中**唯一一条**请求即为启动阶段的版本校验：

```
GET /server/version        ← 注意：没有 /api 前缀
Host: eo795eal4s-8081.cnb.run
User-Agent: immich-ios/3.1.0
Accept: */*
```

服务器当时只注册了 `/api/server/version`（带 `/api` 前缀，见 `app.go`），因此这条 `/server/version`（无 `/api`）请求**没有命中任何显式路由**，落入 `main.go` 的 SPA history-fallback（`r.NoRoute`），返回：

```
HTTP/1.1 200 OK
Content-Type: text/html; charset=utf-8
(body = 官方 web 的 index.html，共 10565 字节)
```

客户端把这段 HTML 当作版本 JSON 解析 → 解析失败 → 判定为「版本不兼容」，于是在版本校验这一步就中止（所以抓包里只有这一条请求，根本还没走到登录）。

**结论**：immich-go 把版本端点只挂在 `/api/server/*` 下，而该 iOS 客户端在 bootstrap/版本校验阶段请求的是**根路径** `/server/version`（以及可能同族的其他 `/server/*` 端点）。路径前缀不一致 → 客户端拿到 HTML 而非 JSON → 报「版本不兼容」。

> 说明：默认 `CompatVersion` 值本身早已是 `"3.1.0"`（`config.go:82`），运行时 `/api/server/version` 也确实返回 `{"major":3,"minor":1,"patch":0,"prerelease":0,"version":"3.1.0"}`（经反向代理实测）。所以「值」没问题，问题纯粹是**客户端所请求的 URL 路径服务器未应答 JSON**。

## 修复（已实现）

1. **补齐根路径别名（`app.go` `RegisterRoutes`）**：在公开路由块中，为 iOS/CLI 客户端在 bootstrap 阶段轮询的 `/server/*`（无 `/api` 前缀）端点逐一添加与 `/api/server/*` 同处理器、同形状的别名，且显式路由优先于 `NoRoute` 的 SPA 兜底，因此会返回 JSON 而非 HTML：

   ```go
   r.GET("/server/ping", ...)
   r.GET("/server/health", ...)
   r.GET("/server/about", a.handleAbout)
   r.GET("/server/version", a.handleServerVersion)
   r.GET("/server/config", a.handleServerConfig)
   r.GET("/server/features", a.handleServerFeatures)
   r.GET("/server/media-types", a.handleServerMediaTypes)
   r.GET("/server/version-history", a.handleServerVersionHistory)
   r.GET("/server/version-check", a.handleServerVersionCheck)
   ```

   其中 `/server/version-check` 之前挂在鉴权分组下、`/api/server/version-check` 会返回 401；根路径别名使其公开可达（与官方 immich 行为一致，该端点在官方服务端本就是公开端点）。

2. **消除 `/api/server/config` 的硬编码陈旧版本（`misc.go:608`）**：原代码返回 `"version": "1.0.0-go"`，与兼容版本不一致；改为 `a.cfg.CompatVersion`，使所有版本出口统一为 `3.1.0`。

两项改动均不改 Dart/前端，也不改 `/api/*` 既有契约；`/api/server/version` 等原有端点行为完全不变（回归安全）。

## 验证方式

- 本地以新二进制在 `:8099` 启动（`IMMICH_LOGIN_REQUIRED=false`），实测：
  - `GET /server/version` → `200 application/json` `{"major":3,"minor":1,"patch":0,"prerelease":0,"version":"3.1.0"}`（**修复前返回 200 text/html**）
  - `GET /server/version-check` → `200` `{"checkedAt":"...","releaseVersion":"3.1.0"}`（修复前 `/api/server/version-check` 为 401）
  - `GET /server/about` → `200` 含 `"version":"3.1.0"`
  - 回归：`GET /api/server/version` → `200` 仍返回正确 JSON
- `go build ./...` 通过（CGO_ENABLED=0）。
- **端到端（用户在真机验证）**：用 iOS 客户端 3.1.0 重新连接 `https://eo795eal4s-8081.cnb.run`，预期不再提示「版本不兼容」，可完成登录并到达资产页。建议再用 Charles/系统抓包确认首条请求 `/server/version` 现在收到 JSON（`application/json`）而非 HTML。

## 影响范围

- 仅影响以**根路径 `/server/*`（无 `/api`）**发起启动/版本轮询的客户端（本报告中的 iOS 客户端 3.1.0）。web 端、以 `/api/*` 为基路径的客户端不受影响（契约不变）。
- 修复后默认（不设置任何环境变量）即与 iOS 客户端 3.1.0 兼容，符合用户要求。
