# AGENTS.md — immich-go (patrick-gallery)

Project goal and hard rules for agents working on this repo. The current
release (`v1.0.0-go`) is the first; the **second release** is the active
target described below.

## Active goal: Release 2

Make **video fully functional** (poster/cover, transcoding, thumbnails) for
single-user / private-LAN personal use, and complete the image+video API
surface for that use case. Port the original Immich frontend.

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
5. **Single-user / private-LAN priority.** Optimize for one owner on a LAN:
   SQLite + WAL, sensible busy_timeout, cheap concurrency. Multi-user
   scaling is explicitly out of scope for now.
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

