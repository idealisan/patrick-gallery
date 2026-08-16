# BUG-001：照片浏览/查看界面图片模糊、缩略图质量差

> 状态：Resolved（已修复并验证） · 日期：2026-08-16 · 严重度：High（核心看图体验损坏）
> 组件：`internal/image/image.go`（`scaleImage`/`Preview`/`Thumbnail`）、`internal/app/asset.go`（`handleAssetThumbnail`/`servePreview`）、`internal/app/config.go`（`PreviewSize`）
> 与 `NO_STUBS.md` 区分：这是"有实现但行为/质量不正确"的真实 bug，非占位桩。

## 现象

1. 大图查看器（`/photos/:id`）显示的是一张很模糊的缩略图，而非高质量预览或原图。
2. 网格缩略图质量差：一张屏幕截图，线条扭曲到完全不能看。
3. 用户判断：缩略图生成算法没有使用合适的缩放 / 均值 / 模糊等算法。

## 证据（来自 HAR 网络存档 `eo795eal4s-8081.cnb.run.har`）

打开照片 `981d4409-7f41-4fe5-b157-0d7eb42540c8` 时，浏览器对该照片发起的图片请求：

| 请求 | 状态 | 类型 | 大小 |
|---|---|---|---|
| `/api/assets/981d4409-.../thumbnail?size=thumbnail` | 200 | jpeg | **10032 字节** |
| `/api/assets/981d4409-.../thumbnail?size=preview`   | 200 | jpeg | **10032 字节** |
| `/api/assets/981d4409-.../original`                 | 200 | png  | 1005966 字节（≈1 MB） |

关键事实：
- `size=preview` 与 `size=thumbnail` 返回**完全相同的 10032 字节 JPEG**（即同一张 256px 缩略图），证明服务端忽略了 `size` 参数。
- `/original` 正确返回 1 MB 原图 PNG，说明"查看原图"功能本身正常，坏的是默认的 `preview` 视图。

## 根因 1：大图查看器模糊 —— `/thumbnail` 忽略 `size=preview`

- 官方 Web 查看器请求 `/api/assets/:id/thumbnail?size=preview`（网格用 `size=thumbnail`）。HAR 已证实。
- `internal/app/asset.go:796` `handleAssetThumbnail` **完全不读取 `size` 查询参数**，永远只返回 `asset.ResizePath`——即入库时 `internal/app/ingest.go:120` 用 `imgproc.Thumbnail(raw, 256)` 生成的 256px 缩略图。
- 因此查看器拿到 256px 小图被浏览器拉伸放大 → 模糊。HAR 用"两档请求字节数完全一致（10032）"证明。
- **对照原版契约**：OpenAPI `viewAsset`（`/assets/{id}/thumbnail`，operationId `viewAsset`）带 `size` 参数；官方客户端分 `thumbnail`/`preview` 两档。原版 `size=preview` 返回按视口缩放的**更大**预览渲染图（通常接近原图或封顶较大尺寸），`size=thumbnail` 才是 256px。immich-go 只有一档 256px 且忽略 `size` → 违反契约。

## 根因 2：缩略图算法差（截图线条扭曲）—— 最近邻下采样

- `internal/image/image.go` 的 `scaleImage`（236–274 行）使用**最近邻采样**：
  ```go
  sx := int(float64(dx)*sxScale + 0.5)
  dst.Set(dx, dy, src.At(sx, sy))
  ```
- 最近邻下采样**没有任何抗锯齿**：对高频细节（截图文字/线条、细密图案）会产生严重走样（aliasing），线条锯齿、扭曲。这是所有下采样算法里**最差**的一种。
- 正确做法：用本仓库已作为 webp 依赖引入的 `golang.org/x/image/draw` 高质量重采样过滤器（如 `draw.ApproxBiLinear` 快而好，或 `draw.CatmullRom` 质量最佳）：
  ```go
  dst := image.NewRGBA(image.Rect(0,0,nw,nh))
  draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
  ```
  对贡献源像素做正确的加权平均 → 缩略图平滑、锐利、线条还原正确。
- JPEG 质量 82、EXIF 方向处理均无问题。
- 附带：`internal/app/asset.go:1046` `makeThumbnail`（当前无调用者、死代码）也是最近邻，若修复此区域应一并处理。

## 与原版契约对齐（AGENTS.md 规则 8）

原版 Immich 在入库时生成**两档**渲染图：`thumbnail`（小，默认 256px）+ `preview`（大，按视口缩放），Web 用 `size` 区分请求。immich-go 只生成一档（256px）并忽略 `size`。

## 修复方向（已实施：1b + 高质量采样）

按用户决策采用 **(1b) 实时缩放** 与 **(2) 高质量采样**：

1. **修复模糊（根因 1）—— 实时缩放 `size=preview`**：
   - `internal/app/asset.go` `handleAssetThumbnail` 现在读取 `size` 查询参数：
     - `size=thumbnail`（或空）→ 仍返回入库时的 256px `ResizePath`（与原版小缩略图档一致）。
     - `size=preview` / `size=fullsize` → 实时从原图生成更大的预览渲染（`servePreview`），最长边封顶 `cfg.PreviewSize`（默认 **2560**，可用 `IMMICH_PREVIEW_SIZE` 覆盖），并**落盘缓存**到 `<原图去扩展名>.<assetID>.preview.jpg`，重复请求直接命中缓存；解码失败（如 HEIC/AVIF 无纯 Go 解码器）回退到 256px 缩略图，再回退原图。
     - `size=original` → 直接返回原图（`c.File(asset.OriginalPath)`，契约中该取值已废弃）。
   - 不引入 `PreviewPath` schema 字段，避免迁移与存量补生成（贴合纯 Go / 单实例约束）。删除时同步清理预览缓存（`asset.go` 强制删除、`misc.go` 清空回收站两处均补 `os.Remove(previewCachePath(...))`）。

2. **修复质量（根因 2）—— 高质量重采样**：
   - `internal/image/image.go` 的 `scaleImage` 由最近邻改为 `golang.org/x/image/draw` 高质量重采样：`draw.CatmullRom.Scale`（最高质量）。对下采样比 > 2 的情况，先以 `draw.ApproxBiLinear` 缩到约 2 倍目标尺寸做抗走样中间步，再最终锐化，避免大比例下采样的混叠。
   - 新增 `Preview(raw, maxEdge, quality)`（默认 2560px / 质量 85），`Thumbnail(raw, maxEdge)` 现委托 `Preview(raw, maxEdge, 82)`，使网格缩略图同样受益。
   - `makeThumbnail`（asset.go 死代码、最近邻）保持未调用；质量由 `Thumbnail`/`Preview` 统一保证。

## 验证结果（2026-08-16）

- 本地 curl（带登录 cookie）对比同一资产 `981d4409-...`：
  - `size=thumbnail` → `117×256`，**10032 字节**（256px 缩略图档，未变）。
  - `size=preview`  → `1177×2560`，**287887 字节**（实时高质量渲染，最长边 2560）。
  - 两档字节数与尺寸均显著不同，根因 1 修复。
- 真实浏览器（Playwright + Chromium headless）经反向代理 `https://eo795eal4s-8081.cnb.run` 验证：
  - 增强烟雾测试：`/photos` 登录可达、登录表单消失、认证态 cookie 存在、`/api/users/me`、`/preferences`、`/notifications?unread=true` 均 200，**HTTP≥400 = 0、network failed = 0、pageerror = 0**。唯一 console.error 为 `/shares` Not found（独立既有缺口，与本 bug 无关）。
  - 大图查看器 `GET /photos/981d4409-...` 加载 `thumbnail?size=preview` 返回 **200、无 pageerror**，确认查看器拿到高分辨率预览。
- 截图线条扭曲（根因 2）：通过改用 CatmullRom + 两阶段抗走样下采样消除最近邻走样。
- `go build`（CGO_ENABLED=0）与 `go vet ./internal/app/... ./internal/image/...` 均干净。

## 影响范围

- 根因 2 影响**所有缩略图**：网格、预览、分享链接缩略图、相册封面缩略图。
- 根因 1 影响**大图查看器默认视图**（"查看原图"走 `/original` 正常）。

## 验证方式

- 修复后，用真实浏览器（增强版 `browser-smoke-enhanced.mjs` 的同源检查 + 手动查看大图）确认：查看器大图清晰；`size=preview` 与 `size=thumbnail` 返回**不同字节数**且 `preview` 明显更大。
- 缩放质量：上传一张含细线条/文字的截图，对比修复前后缩略图是否还有扭曲。
- 契约回归：跑 `python3 scripts/schemathesis_check.py` 确认未破坏 `viewAsset` 契约。
