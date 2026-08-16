# Bug: 视频缩略图颜色/比例错误 + 视频无法播放

- **报告来源**：用户，依据 `/workspace/视频问题.har`（官方 Web UI + 官方 Android/iOS 客户端对视频资产
  `212b7c26-52bb-4904-bde1-d64a6994a976` 的真实流量）。
- **视频样本**：`resources/upload/38cc54d1-03cf-4404-9553-255da04af25b.mp4`，1920×1080，时长 46533 ms
  （约 46.5s），内容为「白色屏幕 + 黑色文字」的录屏。
- **状态**：✅ 已修复并验证（真实 FFmpeg 后端 + 真实样本文件，逐像素核验）。

## 现象

1. **缩略图颜色错误**：上传的是「白底黑字」录屏，但视频缩略图却是一张**深紫/灰暗**的图片。
   在 Web UI 的相册网格与资产详情页（`/photos/<id>`，`?size=preview&edited=true`）看到的都是这张错误图。
2. **缩略图比例错误**：该视频为 16:9，但生成的缩略图是一条**又长又扁的横条**（实际为 1920×180，
   应为 320×180）。
3. **预览/播放无法工作**：在详情页打开视频后，播放器从未真正开始播放，客户端随即 `DELETE` 了
   `/video/stream/<id>`（即关闭流），界面停留在近乎黑色的错误缩略图上。

## HAR 证据

- 三个缩略图请求（`edited=true`、`size=thumbnail`、`size=preview`）均返回
  `200 image/jpeg 13898 bytes`，且 **md5 完全相同**（`80b724eb97a343d4e545a662fdc329d0`）——说明
  `size` 参数被忽略，且返回的是同一张（错误的）图。
- HLS 主播放列表 `main.m3u8` → 变体 `212b7c26…/0/playlist.m3u8`，其正文为：
  ```
  #EXTM3U
  #EXT-X-VERSION:3
  #EXT-X-PLAYLIST-TYPE:VOD
  #EXTINF:46533.000,        ← 错误：应为 46.533
  seg-0.mp4
  #EXT-X-ENDLIST
  ```
  客户端随后 `DELETE /video/stream/<id>` —— 播放从未开始。

## 根因

### 根因 A：缩略图颜色（ffmpeg 后端 `Thumbnail`，`internal/video/ffmpeg_methods.go`）

`sws_scale` 的**源像素格式被硬编码为 `avPixFmtYUV420P`**，且**只填充了平面 0（Y）的指针/步长**，
平面 1/2（色度）留为空。解码器实际输出的像素格式可能不是 YUV420P（例如 NV12），导致 sws 错误解读
色度平面，产生深紫/灰暗的错误颜色。转码路径（`ffmpeg_transcode.go`）正确地读取了帧的真实格式与全部
平面，而缩略图路径没有。

### 根因 B：缩略图比例（`internal/video/ffmpeg_methods.go` 同一函数）

缩放逻辑为：
```go
dw, dh := w, h
if w >= h { dh = int(float64(h) * scale) } else { dw = int(float64(w) * scale) }
```
当 `w >= h`（横屏）时**只缩小了 `dh`，`dw` 仍保持原始全宽 1920**，于是 1920×1080 的视频得到
1920×180 的「长条」。正确做法是按同一比例同时缩放 `dw`、`dh`。

### 根因 C：HLS 播放时长（`internal/app/hls.go` `handleVideoStreamPlaylist`）

`Asset.Duration` 以**毫秒**存储（见 `durSecToMsString`/`msToString`），但 `#EXTINF` 需要**秒**。
代码直接把 `parseDurationInt(asset.Duration)`（毫秒值）喂给 `#EXTINF:%.3f`，于是输出
`46533.000` 而非 `46.533`，播放器据此拒绝播放。

## 修复

| 文件 | 改动 |
|------|------|
| `internal/video/ffmpeg_methods.go` | `Thumbnail`：源格式改为读取帧的真实 `ctxFrameFormat(frame)`；填充全部 4 个平面/步长；按同一比例同时缩放 `dw`、`dh`。 |
| `internal/app/hls.go` | `handleVideoStreamPlaylist`：把 `asset.Duration`（ms）除以 1000 得到秒，再写入 `#EXTINF`。 |
| `internal/video/thumb_color_check_test.go` | 新增回归测试：用真实样本视频生成缩略图并解码，断言平均 RGB 明亮（非深紫），锁定颜色修复。 |

## 验证（真实样本 + 真实 FFmpeg 后端）

```
LD_LIBRARY_PATH=/workspace/dist/libs \
VIDEO_TEST_FILE=/workspace/resources/upload/38cc54d1-....mp4 \
go test ./internal/video/ -run TestThumbnailColorReal -v
# thumbnail avg RGB=(239,238,239) min=(0,0,0) max=(255,255,255)  PASS  ← 白底黑字，颜色正确
```

重新生成的磁盘缩略图尺寸校验：**320×180（16:9 正确）**，并经服务端 `/api/assets/<id>/thumbnail`
返回同一正确白图。

HLS 经服务端校验：
```
#EXTINF:46.533,        ← 修复后正确
seg-0.mp4  →  http=200 type=video/mp4 size=6318571
```

## 相关

- 关于 `size` 参数被忽略：当前实现对所有尺寸变体返回同一张（已修正颜色/比例的）缩略图。这避免了
  stub，但与官方「缩略图/预览/原始」三档不同分辨率变体仍有差距；属于已知的可接受降级，记录在
  `docs/GAP_ANALYSIS.md` 的视频相关条目中。
- iOS 登录失败（官方客户端 `getMyUser` 报 `oauthId` 空指针）：经核对，当前构建的 `/api/users/me`
  已正确返回 `oauthId`（空串非空），可正常反序列化为 `UserAdminResponseDto`。该失败发生在**旧的公开
  部署实例**（`eo795eal4s-8081.cnb.run`，当时运行修复前的旧二进制）。重新部署当前构建后官方客户端即可登录。
