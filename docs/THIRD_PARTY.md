# 第三方依赖登记（THIRD_PARTY.md）

按仓库 `AGENTS.md` 的「依赖登记」硬规则，凡是二进制在**运行时**加载的第三方制品
（含本地共享库、嵌入的前端、官方客户端 APK 等），必须在此登记其版本、来源、许可，
并在发布包中随附。

---

## 1. FFmpeg 共享库（视频处理，运行时经 purego 动态加载）

- 用途：视频转码 / 缩略图 / 流式处理，由 `internal/video.Processor` 接口背后通过
  `github.com/ebitengine/purego` 在同一进程内动态加载，绝不 `exec` 外部 CLI。
- 版本：7.1（软件回退后端；硬件加速后端为可选，缺失时自动降级到软件，服务仍可启动）
- 许可：LGPL-2.1+ / GPL（按具体构建配置）
- 放置：`dist/libs/`（与 `immich-go` 同目录，`LD_LIBRARY_PATH=./dist/libs`）
- 下载（需记录精确、带校验和的固定 URL；发布流程 `scripts/bundle-deps.sh` 下载并随包分发）：
  - 待补：在 `scripts/bundle-deps.sh` 中登记各平台/arch 的 FFmpeg 7.1 共享库固定下载地址与 sha256。

## 2. 官方 immich Web UI（已嵌入二进制，作为产品 UI）

- 用途：网页端管理界面，**不是**手写 SPA。由 immich 官方 monorepo 在固定发布 tag 处
  构建，编译后的 `dist` 经 `//go:embed` 嵌入 Go 二进制并随 SPA history 回退路由提供。
- 版本：v3.1.0（与 `IMMICH_COMPAT_VERSION` / `Config.CompatVersion`、移动端契约一致）
- 来源（build-time 依赖，需记录精确 tag + URL + license）：
  - 仓库：`https://github.com/immich-app/immich`
  - tag：`v3.1.0`
  - 许可：AGPL-3.0
- 契约权威：以官方 `web/`、`server/`、`packages/sdk/` 真实代码为准；OpenAPI 仅作回归校验。

## 3. 官方 immich Android 客户端 APK（本地模拟器端到端测试用）

- 用途：在本地 Android 模拟器中运行真实官方客户端，对 `immich-go` 做端到端契约验证。
  详见 [`ANDROID_TESTING.md`](./ANDROID_TESTING.md)。
- 版本：v3.1.0
- 许可：GPL-3.0
- 下载（GitHub Release，按架构选）：
  - 模拟器（x86_64）：`https://github.com/immich-app/immich/releases/download/v3.1.0/app-x86_64-release.apk`
  - 真机（arm64-v8a）：`https://github.com/immich-app/immich/releases/download/v3.1.0/app-arm64-v8a-release.apk`
  - 真机（armeabi-v7a）：`https://github.com/immich-app/immich/releases/download/v3.1.0/app-armeabi-v7a-release.apk`
  - 通用（含全部 ABI）：`https://github.com/immich-app/immich/releases/download/v3.1.0/app-release.apk`
- 注意：不要使用 `immich-android.apk` 这一文件名（v3.1.0 该路径返回 404）。
- 包名：`app.alextran.immich`；启动 Activity：`app.alextran.immich/app.alextran.immich.MainActivity`
- 测试时客户端地址填 `http://127.0.0.1:8081`（经 `adb reverse tcp:8081 tcp:8081` 转发到宿主机）。
