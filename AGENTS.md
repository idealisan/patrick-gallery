# AGENTS.md — immich-go (patrick-gallery)

Project goal and hard rules for agents working on this repo. The current
release (`v1.0.0-go`) is the first; the **second release** is the active
target described below.

## Active goal

**功能对等原版 Immich**：在**手机 APP**（API + 功能）与 **Web UI**（界面 +
功能）两端都与原版 Immich 保持一致。即不仅打通单人核心闭环，还要逐步补齐
原版的全部能力面——管理后台、用户体验、同步协议等。

落地优先级：
1. **核心媒体闭环**（照片/视频的**上传、去重、同步、管理**）必须真正可用、
   且与原版 **v3.1.0** 契约完全兼容——这是手机 APP 与 Web UI 都能跑起来的地基。
2. 补齐手机 APP / Web UI 对等所需的其余端点与界面（相册/标签/回收站/时间线/
   地图/分享/库扫描/作业/实时同步等）。
3. **显式延后项（受 `AGENTS.md` 硬规则约束，需单独评估，非一次性工作）**：
   - **机器学习相关**：人物聚类/人脸、CLIP 语义搜索、OCR——纯 Go 内无成熟
     推理模型，需外接推理或纯 Go 移植，工程量巨大。
   - **强多用户 / 个性化**：完整多租户、水平扩展高并发、插件/工作流子系统、
     OAuth/SSO（需外部 IdP）、邮件/外部通知。
   这些项在 `docs/GAP_ANALYSIS.md` 中按「约束可行性 ✅/⚠️/❌」逐块标注，并归入
   P3 优先级；其余功能（含相册内共享、伙伴共享）在单人/私域范围内应优先补齐。

> 权威差距跟踪见 [docs/GAP_ANALYSIS.md](docs/GAP_ANALYSIS.md)（以「手机 APP +
> Web UI 功能对等」为目标，逐端点 diff 官方 v3.1.0 契约，并按客户端面归类）。

### Hard rules (must never be violated)

1. **Pure Go, no CGO.** `CGO_ENABLED=0` is mandatory. The binary must stay
   a single static executable (it already cross-compiles for
   linux/darwin/windows × amd64/arm64).
2. **Video processing is in-process only.** Never shell out to `ffmpeg` or
   any external CLI tool. Calling a video engine must happen by invoking
   functions in a **dynamically loaded library** (e.g. via
   `github.com/ebitengine/purego`) within the same process.
3. **Video processing abstraction layer.** The video engine is behind an
   interface (`internal/video.Processor`) so multiple backends can coexist:
   - software (pure-CPU) as the universal fallback,
   - FFmpeg shared libs loaded via purego,
   - OS-native engines loaded via purego:
     - **macOS / Apple**: Video Toolbox
     - **Windows**: Media Foundation
     - **Linux / Android**: corresponding system APIs (Android → MediaCodec)
   - Hardware acceleration is preferred where available: Intel QSV, AMD
     (AMF/VCE), NVIDIA (NVENC/NVDEC). Pure-software is the fallback.
4. **Database abstraction layer.** The app must depend on a `store.Store`
   interface, not on `*gorm.DB` directly, so a Postgres (or other) backend
   can be added later without touching handlers.
5. **Single-owner / private-LAN optimization (default).** The default
   deployment targets one owner on a LAN: SQLite + WAL, sensible busy_timeout,
   cheap concurrency. **True multi-tenant horizontal scaling is out of scope
   for now** (pending a Postgres backend behind `store.Store`). This does
   **not** exempt the parity goal: partner sharing and in-album user sharing
   within the owner's sphere must work, and all core media endpoints must stay
   contract-compatible with the official clients.
6. **Original frontend port.** Aim to fully port the official Immich web
   frontend; at minimum the web UI must correctly browse, play, and manage
   images and videos.

### Notes for agents

- Keep `go vet ./...` and `go build` green; CI runs on every published
  Release (`on: release [published]`) and builds 6 binaries + a multi-arch
  GHCR image. Verify locally with the in-repo Go toolchain (`.tools/go`)
  before pushing — there is no Go on the runner's checkout cache, so CI is
  the only safety net if you skip local checks.
- **API 契约回归测试**：CI 的 `contract-test` job 会构建并启动 immich-go，用
  Schemathesis 4.24.3 针对官方 Immich v3.1.0 OpenAPI 契约做一致性校验，并在
  DTO 形状（`response_schema_conformance`）、`content_type_conformance`、5xx
  上回归时失败。本地可运行 `python3 scripts/schemathesis_check.py` 复现；已知
  未实现端点的 4xx 缺口豁免于 `scripts/schemathesis-allowlist.txt`（详见
  `docs/CONTRACT_TESTING.md`）。改动响应体形状前请先跑此脚本。
- Video backends that fail to load (library absent) MUST degrade gracefully
  (server still starts, video endpoints return a placeholder / the original)
  so the cross-platform binaries keep working everywhere.
- Record any intentional limitation in `STATUS.md`.
- The authoritative gap tracker (endpoint coverage + parity verdict) is
  [docs/GAP_ANALYSIS.md](docs/GAP_ANALYSIS.md). Core media functions
  (upload/sync/management) are tracked there as "done & contract-compatible";
  ML and strong multi-user/personalized features are the explicitly deferred
  categories.

### Dependency bundling (hard rule)

- **Every third-party library the binary loads at runtime (e.g. FFmpeg
  shared libs) MUST have its exact, pinned download URL recorded in the
  repo** in `THIRD_PARTY.md`. Include version, checksum/hash when available,
  and the license.
- **Release packages MUST contain these dependencies.** The packaging step
  (`scripts/bundle-deps.sh`) downloads the pinned artifacts per platform/arch
  and bundles the shared libraries inside each `immich-go-*` package so video
  works out of the box without the user installing anything.
- The Go module dependencies are already captured by `go.mod` / `go.sum`;
  `THIRD_PARTY.md` is specifically for *runtime* native libraries (FFmpeg
  shared builds, etc.) that are loaded via purego.

