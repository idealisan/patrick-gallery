# AGENTS.md — immich-go (patrick-gallery)

Project goal and hard rules for agents working on this repo. The current
release (`v1.0.0-go`) is the first; the **second release** is the active
target described below.

## Active goal

**即时目标（必须可用）**：让**单个用户**用自己的**手机 APP** 和**网页**，
完成**个人相册的管理与跨端同步**——即照片/视频的备份上传、浏览（时间线/相册/
搜索/地图）、收藏/归档/回收站、分享链接、库扫描，以及手机与网页之间的实时刷新。
这一范围已在 `docs/GAP_ANALYSIS.md` §14 逐项正确认为「已可用且与 v3.1.0 契约兼容」。

**方向目标（更长线）**：在即时目标稳固后，再逐步向原版更多能力面靠拢
（管理后台、更完整的同步协议、用户体验细化）。

**显式延后项（不在即时目标内，受 `AGENTS.md` 硬规则约束，需单独评估）**：
- **机器学习相关**：人物聚类/人脸、CLIP 语义搜索、OCR——纯 Go 内无成熟推理
  模型，需外接推理或纯 Go 移植，工程量巨大。
- **强多用户 / 个性化**：完整多租户、水平扩展高并发、伙伴/相册内多用户共享、
  插件/工作流子系统、OAuth/SSO（需外部 IdP）、邮件/外部通知、Memories、Stacks。
  这些项在 `docs/GAP_ANALYSIS.md` 中按「约束可行性 ✅/⚠️/❌」逐块标注，归入 P3；
  它们不属于「单人个人管理+同步」的即时目标。

> 权威差距跟踪见 [docs/GAP_ANALYSIS.md](docs/GAP_ANALYSIS.md)。该文档以「手机 APP
> + Web UI 功能对等」做完整端点 diff；其中的 §14 专门核验了「单人个人管理+同步」
> 这一即时目标所需的全部功能。

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
7. **No stubs, no mocking — every endpoint must do real work.** This is a
   real, production-grade product, **not** an API mock. It is forbidden to
   register any API route or ship any feature that:
   - returns a 2xx with an empty / zeroed / constant DTO while performing no
     real logic (the old "graceful stub that lets the client proceed"),
   - is a no-op that silently ignores its inputs, or
   - fakes a result the server cannot actually produce.
   Every endpoint that is part of the official client contract MUST be
   genuinely implemented. For a capability that is genuinely infeasible in
   this codebase (e.g. ML clustering under the pure-Go constraint), the only
   acceptable alternatives are:
   - implement it for real (preferred), or
   - return an explicit, honest error (4xx / `501 Not Implemented`) so the
     client knows the capability is unsupported. This is **NOT** a stub, but
     it MUST be recorded in the gap tracker (`docs/GAP_ANALYSIS.md`) and added
     to the contract-test allowlist (`scripts/schemathesis-allowlist.txt`) so
     it is never silently shipped as a fake success.
   Returning an empty `200` "so the client proceeds" is explicitly prohibited.
   **Runtime degradation that still does real work** (e.g. video falls back to
   a software transcoder, or serves the original file, when an optional
   hardware backend is absent) is NOT a "feature stub" and remains allowed; a
   backend that returns nothing while claiming success is not allowed.

### No-stub policy (operational)

- **Definition.** A *stub* is any code path that claims success/availability
  but does not perform its real function. Two shapes:
  (a) *fake-success* — hard-coded empty / zero / constant responses or no-op
  `200`s that ignore their inputs (e.g. `handleSyncStream` returning `[]any{}`,
  `handleServerStorage` returning `0 B` for everything, `handlePersonMerge`
  returning `200` with no state change);
  (b) *honest-but-missing* — returns the truthful (often empty) current state
  because an upstream capability is absent (e.g. an empty people list because
  face clustering is not implemented). Both violate this rule: the endpoint
  must either do the real work or return an honest error.
- **No new stubs.** A handler that returns a constant / empty success body
  without reading or acting on its inputs is a review blocker. Do not add them.
- **Inventory.** The current known stubs and their required disposition are
  tracked in `docs/NO_STUBS.md` (file:line, contract operation, stub shape,
  target = implement / honest-error). Drive the list to zero.
- **CI gate.** The contract-test allowlist (`scripts/schemathesis-allowlist.txt`)
  may only cover endpoints that return an *honest* 4xx/501, never fake-success
  `200`s. See `docs/CONTRACT_TESTING.md`.

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

