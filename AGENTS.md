# AGENTS.md — immich-go (patrick-gallery)

Project goal and hard rules for agents working on this repo. The current
release (`v1.0.0-go`) is the first; the **second release** is the active
target described below.

## Active goal

**即时目标（已达成，持续保持）**：让**单个用户**用自己的**手机 APP** 和**网页**，
完成**个人相册的管理与跨端同步**——即照片/视频的备份上传、浏览（时间线/相册/
搜索/地图）、收藏/归档/回收站、分享链接、库扫描，以及手机与网页之间的实时刷新。
这一范围已在 `docs/GAP_ANALYSIS.md` §14 逐项正确认为「已可用且与 v3.1.0 契约兼容」。

**方向目标（当前活动阶段）**：在单人闭环稳固的基础上，逐步补齐「**多用户**」
能力面——先落地账户与权限基座（用户管理后台 `/admin/users/*` 已完成），再实现
伙伴/相册内资产共享透出、按用户的资源隔离与配额、以及更完整的同步协议与体验细化。
多用户能力必须在既有硬规则（纯 Go / 无 CGO / SQLite 单实例 / 进程内视频）内以纯
Go 方式重建，或显式引入受控的外部组件；水平多租户扩展仍属延后项（见下）。

**显式延后项（受 `AGENTS.md` 硬规则约束，需单独评估，不阻塞多用户基础）**：
- **机器学习相关**：人物聚类/人脸、CLIP 语义搜索、OCR——纯 Go 内无成熟推理
  模型，需外接推理或纯 Go 移植，工程量巨大。
- **架构/集成类延后项**：完整多租户水平扩展高并发（需 Postgres 后端，
  `store.Store` 已留接口）、插件/工作流子系统、OAuth/SSO（需外部 IdP）、
  邮件/外部通知、Memories、Stacks。这些在 `docs/GAP_ANALYSIS.md` 按
  「约束内可行性 ✅/⚠️/❌」逐块标注，归入 P3；其中**水平扩展**与 SQLite 单用户
  约束冲突，需 Postgres 后端落地后才对等。
- **已实现基础（不再延后）**：用户/账户管理后台 `/admin/users/*` 已完成；
  伙伴关系、相册内用户共享的关系模型已存在，下一步是把共享资产透出到
  timeline/search（见 `docs/GAP_ANALYSIS.md` §13）。多用户能力已转入**活动阶段**，
  在纯 Go / SQLite 单实例约束内逐步推进。

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
5. **Single-instance / private-LAN optimization (default).** The default
   deployment targets a single immich-go instance on a LAN: SQLite + WAL,
   sensible busy_timeout, cheap concurrency. **Multiple user accounts within
   one instance are now supported** (the user/account management base is
   implemented), but **true multi-tenant horizontal scaling is still deferred**
   (pending a Postgres backend behind `store.Store`). This does **not** exempt
   the parity goal: partner sharing and in-album user sharing must work, and
   all core media endpoints must stay contract-compatible with the official
   clients. New multi-user code MUST keep working under SQLite (no assumption
   of a horizontally-sharded store) until the Postgres backend lands.
6. **Package the OFFICIAL Immich Web UI — no hand-written SPA as the product UI.**
   The shipped web UI MUST be the **official Immich web frontend** built from
   the immich monorepo and embedded into the Go binary, NOT a from-scratch
   replacement. A hand-written vanilla-JS SPA (the interim one under
   `internal/webroot/assets/`) is explicitly **not** the product UI and is being
   removed/replaced. Concrete requirements:
   - Build the official Immich web app at a pinned immich release tag; embed its
     compiled static `dist` via `//go:embed` and serve it (with SPA
     history-fallback to `index.html` for every non-`/api` route — already wired
     in `main.go`).
   - The advertised server version (`IMMICH_COMPAT_VERSION` / `Config.CompatVersion`)
     MUST match the immich release whose web is packaged, so the official client
     accepts the connection instead of refusing on a version mismatch.
   - Any conflict between the official web's expectations and our backend (missing
     endpoints, DTO-shape gaps, base-href / asset-path differences) must be
     resolved by **fixing the backend to the official contract** (per rule 7) or,
     where a capability is genuinely infeasible under our hard rules, returning an
     **honest** 4xx/501 — never by quietly swapping in a custom UI or a fake
     endpoint. The official web UI is the source of truth for the contract.
   - The official immich web source / release used is a build-time dependency and
     MUST be recorded in `THIRD_PARTY.md` (exact tag, URL, license).
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

8. **The official client is the contract — verify against REAL clients, not
   assumptions.** Every contract claim MUST be checked against the actual
   official Immich source and confirmed with a REAL client, never inferred:
   - **Read the original.** Before implementing or "fixing" any endpoint, read
     the matching official server code (`immich-src/server/src/...`) AND the
     official client code (`immich-src/web/src/...`, `packages/sdk/src/...`)
     to learn the EXACT contract: request **path, method, headers, query,
     body**, AND the response DTO shape. Do not guess at auth flow, cookie
     names, or DTO fields.
   - **Behavioral parity, statement by statement.** The backend MUST match the
     original's logic at every **step, branch, and loop** — the same
     precedence of auth sources, the same cookie attributes, the same DTO
     fields (including nested sub-objects), the same error codes. A response
     that omits a nested object the client reads (e.g. `preferences.folders`
     must be `{enabled, sidebarWeb}`, not `null`) is a contract violation even
     if it returns 200.
   - **Real-browser test is a HARD REQUIREMENT (not optional).** Web UI changes
     MUST be verified with a **real headless browser driving the official web
     UI** through the **reverse-proxied public URL** (the service is designed
     to sit behind a reverse proxy, so test exactly that way). The test MUST:
     1. perform a real login (fill the form, click Sign in) — not just `curl`,
     2. assert the app reaches the authenticated home (`/photos`) and the
        **login form is NOT visible**,
     3. confirm authenticated calls (`/api/users/me`, `/api/users/me/preferences`,
        `/api/notifications?unread=true`, timeline/albums) return 200,
     4. capture **console + network** and confirm there is **no client-side
        pageerror** (a 200 with a DTO-shape mismatch still crashes the SPA and
        looks like "login failed").
     `curl` alone is NOT sufficient proof of Web UI correctness — the browser
     is the only authority. A change that passes `curl` but fails in the real
     browser is NOT done.
   - **Why this is a hard rule:** we shipped two consecutive Web UI regressions
     that `curl` could not catch — (a) login returned 401 in the browser because
     auth was keyed to the `x-api-key` header while the official web authenticates
     via the `immich_access_token` **cookie**; (b) login "succeeded" (201) but the
     photos page crashed with `Cannot read properties of null (reading 'enabled')`
     because `/api/users/me/preferences` returned a legacy flat shape instead of
     the official nested `UserPreferencesResponseDto`. Both were invisible to
     `curl` and only surfaced in a real browser. See the post-mortem in
     `STATUS.md` §P.

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
- **Real-browser Web UI smoke test (rule 8).** Before claiming any Web UI /
  auth change works, run the headless-browser smoke test:
  `node scripts/browser-smoke.mjs` (setup in the script header). It performs a
  real login through the reverse-proxied URL and fails unless the app reaches
  `/photos`, the login form is gone, authenticated calls return 200, and there
  is no client-side `pageerror`. `curl` does NOT satisfy this requirement.
- Record any intentional limitation in `STATUS.md`.
- The authoritative gap tracker (endpoint coverage + parity verdict) is
  [docs/GAP_ANALYSIS.md](docs/GAP_ANALYSIS.md). Core media functions
  (upload/sync/management) are tracked there as "done & contract-compatible";
  ML and horizontal-scaling features remain the explicitly deferred categories,
  while multi-user account management is now an active workstream (see §13).

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

