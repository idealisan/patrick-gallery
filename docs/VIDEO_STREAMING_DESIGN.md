# 视频串流与转码架构设计

> 状态：设计讨论中，待实现
> 创建：2026-08-18

## 目标

1. 首次播放新上传视频，从点击播放到第一帧画面+声音，延迟 ≤1 秒（localhost 空载）
2. 进度条 seek 正常
3. 内存峰值低（流式处理，不全量 buffer）
4. 磁盘缓存 LRU，上限 2GB
5. 硬件加速优先，软件编码保底

---

## 1. 转码决策 — 码率阈值策略

不按"浏览器是否支持编码格式"判断，纯看原始码率是否超过串流阈值。

### 阈值表

| 分辨率档位 | 宽度范围 | 最大串流码率 |
|-----------|---------|------------|
| ≤ 1080p   | w ≤ 1920 | 3 Mbps     |
| 2K        | w ≤ 2560 | 5 Mbps     |
| 4K        | w ≤ 3840 | 8 Mbps     |

### 决策流程

```
probe 原始文件 → 获取 width + bit_rate
  → 原始码率 ≤ 对应阈值 → 直接 serve 原始文件（零转码）
  → 原始码率 > 阈值     → 转码，输出码率 cap 到阈值
```

注意：即使浏览器支持原始编码（如 h264），码率超限仍需转码。因为高码率串流会卡顿，
用户体验差。

---

## 2. 硬件加速 — 统一 fallback 顺序

编码器和解码器使用相同的优先级顺序：

```
1. macOS Video Toolbox（Apple 硬件）
2. NVIDIA NVENC
3. Intel QSV（Quick Sync Video）
4. AMD AMF/VCE
5. 软件（libx264 / libx265）
```

### 实现要点

- `selectEncoder()` / `selectDecoder()` 按此顺序逐个 probe
- 成功打开即用，失败则尝试下一个
- 软件编码器（libx264）作为保底，确保任何平台都能工作
- macOS 上 Video Toolbox 应为首选，转码速度比软件快一个数量级

### 当前代码问题

`ffmpeg.go` 的 `selectEncoder` 只在软件编码器中选择，没有尝试硬件编码器。
解码器只用了软件解码。需要重构为按优先级逐个尝试。

---

## 3. Faststart — 必须实现

### 问题

MP4 文件的 moov atom（包含索引信息）默认在文件末尾。浏览器播放需要先拿到 moov
才能解码，如果没有 faststart，浏览器必须下载整个文件才能开始播放。

### 方案：Go 层 qt-faststart 后处理

不在 FFmpeg muxer 层面做 faststart（avio seek 有兼容性问题），而是在转码完成后
在 Go 层做 atom 搬运：

1. 解析 MP4 的 atom 树（ftyp / moov / mdat / free）
2. 把 moov atom 从文件末尾搬到 ftyp 之后、mdat 之前
3. 更新文件中的 offset 引用

这是确定性的 atom 搬运，纯 Go 实现，不依赖 FFmpeg 的 movflags。

### 验证

转码完成后用 ffprobe 检查 atom 顺序：`ftyp → moov → mdat`。

---

## 4. 转码架构 — file-to-file 流式处理

### 新增 API

```go
// Processor 接口新增
TranscodeToFile(srcPath, dstPath string, opts TranscodeOptions) error
```

### 内存模型

```
原始文件 ──流式读取──→ 解码器 ──→ 缩放器 ──→ 编码器 ──→ 流式写入输出文件
           (FFmpeg I/O)    (帧缓冲)   (帧缓冲)   (帧缓冲)    (FFmpeg I/O)
```

- FFmpeg `avformat_open_input(path)` 做流式 I/O，不全量读文件
- 输出直接写到目标文件，不经过 Go 堆
- 内存峰值 ≈ 解码器帧 + 缩放器帧 + 编码器帧 ≈ 几 MB
- 不再使用 `Transcode(in []byte) ([]byte, error)` 的批量 API 处理播放路径

---

## 5. serveEncodedMP4 完整流程

```
请求到达
  │
  ├─ 查缓存（DB: encoded_video_path != "" && 文件存在）
  │   └─ 命中 → http.ServeContent 从磁盘 serve（几 ms）
  │
  └─ 无缓存
      │
      ├─ probe 原始文件（读头部获取 width / bit_rate）
      │
      ├─ 码率 ≤ 阈值？
      │   └─ 是 → http.ServeContent serve 原始文件（零转码）
      │
      └─ 码率 > 阈值
          │
          ├─ TranscodeToFile（硬件加速优先）
          ├─ qt-faststart 后处理（搬 moov 到文件头）
          ├─ 写入 encoded-video/{id}.mp4
          ├─ 更新 DB（encoded_video_path + last_played_at）
          └─ http.ServeContent serve 缓存文件
```

### HTTP 响应

使用 `http.ServeContent` 替代 `c.Data()`：
- 自动处理 HTTP Range 请求 → 进度条 seek 正常
- 自动设置 Content-Length / Content-Type
- 从磁盘文件读取，不全量加载到内存

---

## 6. 磁盘缓存 — LRU 2GB

### 存储

- 转码结果存 `resources/encoded-video/{asset_id}.mp4`
- DB `assets` 表字段：
  - `encoded_video_path`（已有）：缓存文件路径
  - `last_played_at`（新增）：最后播放时间，用于 LRU 淘汰

### 淘汰策略

每次 serve 缓存文件时检查总缓存大小：
1. 计算 `encoded-video/` 目录总大小
2. 超过 2GB 时，按 `last_played_at` 升序查询
3. 逐个淘汰（删文件 + 清 `encoded_video_path`）直到 ≤ 2GB

### 简单实现

不需要复杂的定时任务，在每次缓存 miss（需要新转码）时顺带检查即可。

---

## 7. 缩略图与预览图质量改进

### 问题

当前 `sws_scale` 使用 `SWS_BILINEAR`（双线性插值），对文字内容的缩放效果差，
文字模糊不可读。

### 方案

改用 `SWS_LANCZOS`（Lanczos 插值），FFmpeg 内置，改动最小：
- 仅需改一个常量：`SWS_BILINEAR` → `SWS_LANCZOS`
- Lanczos 对文字边缘和高频细节的保持远优于 bilinear

如果效果仍不够，可换纯 Go 库（`nfnt/resize` 或 `disintegration/imaging`）。

### 适用范围

- 视频缩略图（`Thumbnail` 方法）
- 图片缩略图（如有类似逻辑）

---

## 8. 惰性转码

- 上传时不预转码
- 首次播放时按需转码（上述 `serveEncodedMP4` 流程）
- 后台 `videoConversion` job 暂不自动触发
- 转码结果缓存供后续播放复用

---

## 9. 兼容性约束

- 不修改官方 Web UI（immich web frontend）
- 不修改官方客户端（Android / iOS）
- 所有 API 契约以官方 Immich v3.1.0 为准
- faststart MP4 是标准格式，官方客户端均能播放
- HLS 路径（`video/stream/`）不变，mobile 客户端继续走 HLS

---

## 10. 内存预算

| 组件 | 峰值内存 |
|------|---------|
| 解码器帧缓冲 | ~几 MB |
| 缩放器帧缓冲 | ~几 MB |
| 编码器帧缓冲 | ~几 MB |
| HTTP 响应 | 从磁盘流式读取，不占堆内存 |
| **总计** | **< 10 MB** |

远低于用户要求的"10秒未播放缓冲"。流式架构天然低内存。

---

## 11. 已修复的已知问题（供参考）

| 问题 | 根因 | 状态 |
|------|------|------|
| 视频播放倍速（50x） | `av_rescale_q` AVRational by-value ABI 不兼容 | ✅ 已修复 |
| 视频进度条无法 seek | `c.Data()` 不支持 HTTP Range | ✅ 已修复 |
| 重复转码慢 | 每次请求都从头转码，无缓存 | ✅ 已修复 |
| 缩略图颜色错误 | 源像素格式硬编码 YUV420P | ✅ 已修复 |
| 缩略图比例错误 | 只缩放一个维度 | ✅ 已修复 |
| HLS 时长错误 | ms 当作 s 写入 EXTINF | ✅ 已修复 |

---

## 12. 实现优先级

1. **P0**：`TranscodeToFile` + 硬件加速 fallback + 码率阈值决策
2. **P0**：Faststart（qt-faststart Go 后处理）
3. **P0**：磁盘缓存 LRU + `last_played_at`
4. **P1**：缩略图/预览图 Lanczos 改进
5. **P1**：重构 `selectEncoder` / `selectDecoder` 按硬件优先级

---

## 13. 测试策略

### 13.1 Unit Test

每个模块必须有详尽的单元测试，覆盖正常路径和边界情况。

**转码模块 (`internal/video/`)：**

| 测试文件 | 覆盖内容 |
|---------|---------|
| `transcode_test.go` | `TranscodeToFile` 正常转码、输出文件有效性（ffprobe 校验）、帧数/时长/码率一致 |
| `transcode_test.go` | 输入文件不存在/损坏时返回正确错误 |
| `transcode_test.go` | 不同分辨率输入（480p/720p/1080p/2K/4K）的输出尺寸正确 |
| `faststart_test.go` | qt-faststart atom 搬运：moov 从末尾搬到 ftyp 之后 |
| `faststart_test.go` | 已经是 faststart 的文件不被破坏 |
| `faststart_test.go` | 损坏/非 MP4 文件返回错误而非 panic |
| `encoder_select_test.go` | 硬件编码器 probe：存在时选中，不存在时 fallback 到下一个 |
| `encoder_select_test.go` | 最终 fallback 到软件编码器 |
| `bitrate_decision_test.go` | 码率阈值决策：≤阈值返回 false，>阈值返回 true |
| `bitrate_decision_test.go` | 各分辨率档位的阈值边界值测试 |
| `thumbnail_test.go` | Lanczos 插值：文字图片缩放后可读性（OCR 或边缘检测） |
| `thumbnail_test.go` | 不同输入格式（NV12/YUV420P/RGBA）的颜色正确性 |

**缓存模块 (`internal/app/`)：**

| 测试文件 | 覆盖内容 |
|---------|---------|
| `cache_test.go` | 缓存命中：`encoded_video_path` 有值且文件存在 → 直接 serve |
| `cache_test.go` | 缓存 miss：文件不存在 → 触发转码 |
| `cache_test.go` | LRU 淘汰：总大小超 2GB 时删除最久未播放的文件 |
| `cache_test.go` | 并发访问：多个请求同时 cache miss 同一资产 → 只转码一次 |

**HTTP 响应：**

| 测试文件 | 覆盖内容 |
|---------|---------|
| `range_test.go` | Range 请求：`Range: bytes=0-` 返回206 + 正确 Content-Range |
| `range_test.go` | Range 请求：`Range: bytes=1000-2000` 返回正确字节范围 |
| `range_test.go` | 无 Range 头：返回200 + 完整内容 |

### 13.2 End-to-End Test

用真实浏览器和真实文件验证完整链路。

**E2E 场景：**

| 场景 | 步骤 | 预期 |
|------|------|------|
| 首次播放 | 上传视频 → 打开 Web UI → 点击播放 | ≤1 秒出现画面+声音 |
| 二次播放 | 再次点击播放 | 秒开（命中缓存） |
| 进度条 seek | 拖拽进度条到中间位置 | 从新位置继续播放，不从头开始 |
| 高码率转码 | 上传4K高码率视频 → 播放 | 自动转码到阈值内，播放流畅 |
| 低码率直传 | 上传720p低码率视频 → 播放 | 直接 serve 原始文件，零转码延迟 |
| 缓存淘汰 | 上传多个大视频 → 播放使缓存超2GB → 再播放新视频 | 最久未播放的缓存被淘汰 |
| 硬件加速 | macOS 上播放视频 → 检查日志 | 使用 Video Toolbox 而非软件编码 |

**E2E 工具：**

- `scripts/browser-smoke.mjs`：已有，扩展支持视频播放场景
- 或 Playwright/Puppeteer 脚本：自动化播放+断言

### 13.3 Performance Test

量化性能指标，确保满足目标。

**性能指标：**

| 指标 | 目标 | 测量方法 |
|------|------|---------|
| 首帧延迟（首次播放） | ≤1 秒 | 从点击播放到第一帧画面的时间 |
| 首帧延迟（缓存命中） | ≤200 ms | 从点击播放到开始播放的时间 |
| 转码吞吐量 | ≥30 fps | 1080p 输入的转码帧率 |
| 内存峰值 | <50 MB | 转码过程中的最大 RSS |
| 磁盘 I/O | <100 MB 写入 | 单次转码的磁盘写入量 |
| 缓存命中率 | >90% | 重复播放同一视频时的命中比例 |

**性能测试工具：**

```go
// internal/video/bench_test.go
func BenchmarkTranscodeToFile1080p(b *testing.B) {
    // 用真实1080p视频文件
    // 测量 TranscodeToFile 的吞吐量
}

func BenchmarkTranscodeToFile4K(b *testing.B) {
    // 用真实4K视频文件
}

func BenchmarkFirstFrameLatency(b *testing.B) {
    // 模拟 serveEncodedMP4 完整流程
    // 测量从请求到首字节的时间
}
```

**性能回归检测：**

- CI 中运行 benchmark test
- 对比基准线，性能退化超过10%时告警
- 记录在 `docs/PERFORMANCE_BASELINE.md`

### 13.4 测试数据

| 文件 | 用途 | 特征 |
|------|------|------|
| `test.mov` | 基础转码 | 1080x720, 30fps, h264+aac, 38s |
| `test_4k.mp4` | 高码率转码 | 3840x2160, 高码率 |
| `test_720p.mp4` | 低码率直传 | 1280x720, 低码率 |
| `test_text.png` | 缩略图质量 | 文字内容，测试 Lanczos |
| `test_corrupt.mp4` | 错误处理 | 损坏文件 |
| `test_faststart.mp4` | faststart | 已有 faststart 的文件 |

测试数据放在 `testdata/` 目录，不纳入 git（通过 `.gitignore` 排除大文件）。

