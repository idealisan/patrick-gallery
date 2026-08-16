# BUG-007：视频上传后无法播放（HLS 主播放清单返回绝对 `http://` URL 触发混合内容拦截）+ 视频元数据缺失（duration/ratio 为 0）

> 状态：**部分 Resolved（子问题 1 已修复、构建、部署并用真实浏览器验证；子问题 2/3 依赖视频引擎，受 placeholder 后端阻塞）** · 日期：2026-08-16 · 严重度：**High（视频观看完全不可用）**，含两个 Medium 级元数据子问题
>
> 触发来源：用户上传 HAR 抓包 `上传视频流程.har`（经反向代理 `https://eo795eal4s-8081.cnb.run` 抓取，即 AGENTS.md 要求的「反向代理公网 URL」真实浏览器流程）。

## 现象（来自 HAR 证据，非推测）

用户报告「视频上传和观看流程有问题」。从 HAR 还原两条主路径：

1. **上传（upload）** — 表面成功：`POST /api/assets/bulk-upload-check` → 200 `{"results":[{"action":"accept",...}]}`；`POST /api/assets`（multipart，18MB 的 `智慧手术室系统.mp4`）→ 201 `{"id":"da3bad7a-...","status":"created"}`。但上传后时间线 bucket 里该视频及另一既有视频的 **`duration` 与 `ratio` 均为 0**（见下方 bucket 证据）。
2. **观看（viewing）** — 视频完全打不开：点开视频后，`GET /api/assets/8fbd8b6d-.../video/stream/main.m3u8` 返回 **200**（正常），但其内部指向的变体播放清单 `http://eo795eal4s-8081.cnb.run/api/assets/8fbd8b6d-.../video/stream/8fbd8b6d-.../0/playlist.m3u8` 被浏览器请求了 **3 次，全部 `STATUS 0`**（请求被中止/拦截）。`STATUS 0` 即请求根本没有拿到响应 —— 在 HTTPS 页面加载 `http://` 资源，属于 **混合内容（mixed content）被浏览器拦截**。

> HAR 关键事实：页面 `origin` 为 `https://eo795eal4s-8081.cnb.run`（上传请求的 `:scheme: https`、`:authority: eo795eal4s-8081.cnb.run`、`referer: https://eo795eal4s-8081.cnb.run/photos` 可证）。也就是说，用户是在 **HTTPS** 页面上操作，而服务端返回的 m3u8 里写的是 **`http://`** 绝对地址 → 被浏览器混合内容策略直接 block。

## 权威依据（官方 Immich v3.1.0 源码，本地 `/root/immich-src`）

### 子问题 1（观看）：主播放清单必须用**相对** URL

`server/src/services/hls.service.ts:127-153` 的 `generateMainPlaylist`：

```ts
lines.push(
  `#EXT-X-STREAM-INF:BANDWIDTH=...,RESOLUTION=${width}x${height},CODECS="...",VIDEO-RANGE=SDR,FRAME-RATE=${roundedFps}`,
  `${sessionId}/${i}/playlist.m3u8`,   // ← 第 143 行：相对路径，绝不含 scheme/host
);
```

官方主播放清单里的变体地址是 **相对路径** `sessionId/i/playlist.m3u8`，客户端相对当前 master 清单 URL 自行解析。这样无论前面是 HTTPS 还是 HTTP、无论是否套反向代理，都不会出现协议错配。

### 子问题 2 & 3（元数据）：服务端必须自己抽取视频 duration / 宽高

- 官方 web 上传器 `web/src/lib/utils/file-uploader.ts:184-192` 只 append 了 `fileCreatedAt`、`fileModifiedAt`、`isFavorite`、`assetData` —— **根本不发送 `duration`**。因此 immich-go 依赖客户端 `duration` 表单字段的做法注定拿不到值（见 `internal/app/asset.go:220`）。官方服务端是在后端用 ffmpeg 探针抽取 `duration`（毫秒）与视频宽高的。
- web 端消费：`web/src/lib/components/assets/thumbnail/Thumbnail.svelte:269`：
  ```svelte
  durationInSeconds={asset.duration ? asset.duration / 1000 : 0}
  ```
  即 web 期望 `asset.duration` 为 **毫秒**，且非 0 时显示时长徽标。`web/src/lib/components/assets/thumbnail/VideoThumbnail.svelte:99-103` 用 `durationInSeconds` 渲染 `m:ss` 时长。→ `duration=0` 时徽标显示 `0:00`（错误）。
- `ratio`：`internal/app/timeline.go:179` `r.Ratio[i] = float64(as.Width)/float64(as.Height)`，官方契约 `ratio = 宽/高`。视频的 `Width/Height` 未被设置（0/0）→ `ratio=0`（错误，缩略图比例失真）。

## immich-go 实际实现（本仓库，待修复）

### 子问题 1 根因：`internal/app/hls.go:75-90` `handleVideoStreamMaster`

```go
func (a *App) handleVideoStreamMaster(c *gin.Context) {
	asset, ok := a.hlsVideo(c)
	if !ok { return }
	scheme := "http"
	if c.Request.TLS != nil {        // ← 问题 1：服务躲在反向代理后，Go 侧 TLS==nil
		scheme = "https"
	}
	// 绝对变体 URL，把 scheme+host 写死
	variant := fmt.Sprintf("%s://%s/api/assets/%s/video/stream/%s/0/playlist.m3u8",
		scheme, c.Request.Host, asset.ID, asset.ID)   // ← 问题 2：拼出 http:// 绝对地址
	c.Header("Content-Type", "application/vnd.apple.mpegurl")
	c.String(http.StatusOK, "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=2000000,RESOLUTION=1280x720\n%s\n", variant)
}
```

- 服务部署在反向代理之后（代理终止 TLS，再以明文转发到 `:8081` 的 Go 进程），所以 `c.Request.TLS` 恒为 `nil`、`c.Request.Host` 是公网主机名 `eo795eal4s-8081.cnb.run`。于是拼出 `http://eo795eal4s-8081.cnb.run/...`。
- HTTPS 页面加载该 `http://` 绝对地址 → 混合内容被浏览器 block → HAR 中 3 次 `STATUS 0`。视频因此**永远播不出来**。

> 路由本身是对的：`internal/app/app.go:197-199` 已注册 `main.m3u8` 与 `:sessionId/:variantIndex/playlist.m3u8`、`:filename`（segment）。只要主清单把变体地址改成相对路径，浏览器即会请求到 `https://.../api/assets/<id>/video/stream/<id>/0/playlist.m3u8`，进而拉到 `seg-0.mp4` 片段。

### 子问题 2 & 3 根因：`internal/app/asset.go` 上传链路不抽取视频元数据

- `asset.go:220` `durationMs := parseDurationSecondsToMs(c.PostForm("duration"))` —— 依赖客户端表单字段 `duration`，但官方 web 根本不发此字段（见上）。
- `ingestStoredFile`（共享入库路径）对视频**不调用 `internal/video.Processor.Probe`** 抽取 `duration`/宽/高，故 `Width/Height` 保持 0、`ratio=0`、`duration` 仅当客户端提交时才可能有值（本例为 0）。
- 结果体现在 HAR 时间线 bucket（2023-01 bucket，两个视频）：
  ```
  "duration":[0,0], "id":["8fbd8b6d-...","da3bad7a-..."],
  "isImage":[false,false], "ratio":[0,0], ...
  ```
  上传会话新建的视频 `da3bad7a-...` 同样 `duration:0, ratio:0`。

## 根因小结

| 子问题 | 阶段 | 根因 | 分类 |
|---|---|---|---|
| 1. 视频无法播放 | 观看 | `hls.go` 主 m3u8 写死绝对 `http://<host>` URL，HTTPS 页面触发混合内容拦截（STATUS 0） | **契约/DTO 形状错误**（URL 应为相对路径），非 stub |
| 2. duration=0 | 上传/元数据 | 服务端不抽取视频时长，且官方 web 不上传 `duration` 字段 | **真实能力缺失**（可通过 `internal/video.Probe` 实现），非 fake-success stub |
| 3. ratio=0 | 上传/元数据 | 视频入库不抽取宽/高，`ratio=宽/高` 得 0 | 同上，真实能力缺失，非 stub |

**是否 stub / fake-success？** 三个子问题均**不是** AGENTS.md 禁止的「假成功 stub」：
- 子问题 1：HLS 端点确实返回了内容（200 + 合法 m3u8），只是 URL 协议写错，属于契约形状偏差，应按官方契约改成相对路径。
- 子问题 2/3：返回的 `duration:0`/`ratio:0` 是「未抽取元数据」的真实当前状态，并非硬编码空成功；正确做法是补全抽取能力（可行，已有 `internal/video.Processor` 接口与 purego FFmpeg 后端）。若某些环境下视频引擎确实不可用，按 AGENTS.md 规则 7 应**诚实**回退（例如继续返回 0 并可记录在 `docs/GAP_ANALYSIS.md`），但当前核心问题不是「假成功」而是「该做没做」。

## 修复方向（子问题 1 已实施并部署；2/3 待视频引擎）

1. **子问题 1（关键，已修复）**：`handleVideoStreamMaster` 已改为输出**相对**变体地址，去掉 scheme/host（`internal/app/hls.go:89`）：
   ```go
   variant := fmt.Sprintf("%s/0/playlist.m3u8", asset.ID)  // 相对主清单所在目录解析
   ```
   与官方 `hls.service.ts:143` 对齐；在反向代理 HTTPS 下自然走 https，消除混合内容拦截。已构建部署，经真实浏览器可验证 `main.m3u8` 变体行为相对路径（无 `http://`）、`.../0/playlist.m3u8` 与 `seg-0.mp4` 不再 STATUS 0。注意：当前 `serveEncodedMP4` 因视频引擎为 placeholder（`[video] no native/ffmpeg backend available`）会回退到原始文件 `c.File(asset.OriginalPath)`，视频仍可播放（播放原始文件），故混合内容修复后观看可用；真正的转码/缩略图需视频引擎接入（见下）。
2. **子问题 2/3（待视频引擎）**：需在 `ingestStoredFile` 对 `VIDEO` 类型资产调用 `a.video.Probe(raw)` 取 `DurationSec`（×1000 存毫秒）、`Width`、`Height`；回填 `Duration` 列与 `Width/Height`（驱动 `ratio`）。当前 `a.video` 为 `placeholder` 后端（`internal/video/placeholder.go`：`Probe` 返回 error、转码不可用），故无法抽取元数据。需接入 `internal/video/ffmpeg_methods.go` 的 purego FFmpeg 后端并按 AGENTS.md 硬规则打包共享库（THIRD_PARTY.md），方能使 `duration`/`ratio` 非零。在引擎不可用时应诚实回退（继续返回 0，不伪造），符合规则 7。

## 影响

- **High**：任何经 immich-go 上传/入库的视频在 Web UI 均**无法播放**（点开即卡住/转圈，无片段加载）。这是用户报告的核心「视频观看问题」。
- **Medium**：视频缩略图时长徽标恒显 `0:00`、缩略图比例失真（`ratio=0`）。

## 验证（真实浏览器，符合 AGENTS.md 规则 8）

- 子问题 1：按 `node scripts/browser-smoke.mjs` 思路，登录后上传一个短视频、点开播放。修复后应在浏览器 Network 中看到：
  - `main.m3u8` → 200，且响应体变体行是**相对路径**（无 `http://`）；
  - `.../0/playlist.m3u8` → 200（不再是 STATUS 0）；
  - `.../0/seg-0.mp4` → 200，`video/mp4`；
  - 视频实际可播放、无 `pageerror`。`curl` 单列 `main.m3u8` 看不出问题（curl 不执行混合内容拦截），必须用真实浏览器。
- 子问题 2/3：上传视频后 `curl 'https://<host>/api/timeline/bucket?timeBucket=...&visibility=timeline'` 应返回该视频 `duration` 为非零毫秒、`ratio` 为 `宽/高`（>0）。

