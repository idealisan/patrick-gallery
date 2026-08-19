# immich-go Web 后台 UI 元素与工作流程分析（基于官方 immich v3.1.0）

> 来源：官方 immich 仓库 `web/src/routes/admin/*`、`web/src/lib/components/admin-settings/*`、
> 以及配套 SDK `packages/sdk/src/fetch-client.ts`（v3.1.0 tag）。
> 本文档是「逐个实现 Go 后端」的任务清单（plan）。每个条目列出：UI 元素 → 触发的工作流
> → 对应 API 端点（方法+路径）→ 需要的 DTO 形状 → 实现注意点（纯 Go / SQLite 约束下）。
>
> 原则（AGENTS.md 硬规则 7/8）：每个端点必须**真实实现**，不可返回假成功 200；受约束做不了
> 的（如 ML、OAuth 外部 IdP、SMTP 发信）返回诚实 4xx/501 并登记到 `docs/GAP_ANALYSIS.md` /
> `scripts/schemathesis-allowlist.txt`。

---

## 0. 后台整体结构（路由树）

`/admin` 布局（`+layout.ts`）要求 `admin` 角色，并在加载时初始化 `systemConfigManager`
（即调用 `GET /system-config` + `GET /system-config/defaults`）。

子页面：
- `/admin/system-settings` — 系统配置（重定向目标，原名 `/admin`）
- `/admin/users` — 用户管理（已部分实现 `/admin/users/*`）
- `/admin/library-management` — 资料库管理（已部分实现 `/libraries/*`）
- `/admin/jobs-status` — 任务状态（`/jobs`）
- `/admin/queues` + `/admin/queues/[name]` — 队列监控（`/queues`）
- `/admin/maintenance` — 维护模式 + 完整性报告（`/admin/maintenance`、`/admin/integrity/*`）
- `/admin/server-status` — 服务器状态（`/server/statistics`、`/server/storage`）

---

## 1. 系统设置（System Settings）— `/system-config`

UI 由 19 个 `Setting*Card` 组成，每个卡片读写 `SystemConfigDto` 的子对象，整体通过
`PUT /system-config` 持久化。读取：`GET /system-config`；默认值：`GET /system-config/defaults`。

### 1.1 各设置卡片 → 子 DTO → 字段

| UI 卡片 | 子 DTO 字段 | 关键字段 |
|---------|------------|---------|
| ServerSettings | `server` | externalDomain, loginPageMessage, publicUsers |
| UserSettings | `user` | deleteDelay |
| AuthSettings | `passwordLogin` / `oauth` | passwordLogin.enabled；oauth：enabled, autoRegister, autoLaunch, buttonText, clientId, clientSecret, issuerUrl, scope, ...（OAuth 需外部 IdP → 诚实 501） |
| ThemeSettings | `theme` | customCss |
| LoggingSettings | `logging` | enabled, level |
| MapSettings | `map` | enabled, lightStyle, darkStyle |
| TrashSettings | `trash` | enabled, days |
| LibrarySettings | `library` | scan.cronExpression/enabled, watch.enabled |
| ImageSettings | `image` | colorspace, extractEmbedded, thumbnail/preview/fullsize(size/quality/format) |
| FFmpegSettings | `ffmpeg` | crf, preset, targetResolution, targetVideoCodec, accel, transcode, realtime.enabled/resolutions/videoCodecs, accepted*Codecs/containers |
| JobSettings | `job` | 13 个 JobSettingsDto（concurrency）：thumbnailGeneration, metadataExtraction, videoConversion, smartSearch, facialRecognition, duplicateDetection, ocr, sidecar, library, migration, backgroundTask, editor, integrityCheck, notifications, search, workflow |
| MachineLearningSettings | `machineLearning` | enabled, urls[], clip/facialRecognition/ocr/duplicateDetection 各 enabled+modelName+阈值；availabilityChecks |
| MetadataSettings | `metadata` | faces.import |
| NewVersionCheckSettings | `newVersionCheck` | enabled, channel |
| NightlyTasksSettings | `nightlyTasks` | startTime, 多项 boolean 开关 |
| NotificationSettings | `notifications.smtp` | enabled, from, replyTo, transport{host,port,secure,username,password,ignoreCert} |
| BackupSettings | `backup.database` | DatabaseBackupConfig（SQLite 不适用 PG 式备份 → 自研方案，见 §5） |
| StorageTemplateSettings | `storageTemplate` + `GET /system-config/storage-template-options` | enabled, template, hashVerificationEnabled；options 来自 storage-template-options |
| IntegrityChecksSettings | `integrityChecks` | checksumFiles{cronExpression,enabled,percentageLimit,timeLimit}, missingFiles, untrackedFiles |

### 1.2 端点清单（实现目标）

| 方法 | 路径 | 说明 | 实现方式 |
|------|------|------|---------|
| GET | `/system-config` | 读当前配置 | 从 `system_config` 表（singleton）返回 `SystemConfigDto` |
| PUT | `/system-config` | 写配置 | 校验并持久化；影响运行时（如 trash days、library scan cron） |
| GET | `/system-config/defaults` | 默认配置 | 返回硬编码默认 `SystemConfigDto` |
| GET | `/system-config/storage-template-options` | 模板选项 | 返回 dayOptions/hourOptions/.../presetOptions 常量 |

> **当前状态**：`GET /system-config`、`GET /system-config/defaults`、`GET /system-config/storage-template-options`
> 可能已有处理（见 `misc.go` / `compat.go`）。需核对 DTO 形状完整性与 PUT 实现。

---

## 2. 用户管理（User Management）— 已基本完成

端点（`/admin/users/*`）已在 `admin_users.go` 实现 11 个。UI 元素：列表、新建、编辑
（含 FeatureSetting：归档/收藏/合作伙伴权限等开关）、删除/恢复、会话、统计、日历热力。
**本批主要补强**：伙伴共享资产透出（P0-4）、相册内共享（P0-5）——见多用户路线。

---

## 3. 任务状态（Jobs Status）— `/jobs`

UI：展示各队列任务计数与运行状态。端点：
- `GET /jobs`（`getQueuesLegacy`）→ `QueuesResponseLegacyDto`
- `POST /jobs`（`createJob`）→ 手工触发某 job（`JobCreateDto{name, command?}`）
- `POST /jobs/:name`（`runQueueCommandLegacy`）→ `QueueCommandDto{command: 'empty'|'pause'|'resume'|'clear-failed'}`

**实现**：immich-go 已有 `/jobs` 真实执行（thumbnailGeneration/metadataExtraction/
videoConversion/duplicateDetection）。需补齐：返回 legacy 计数形状、手工触发命令语义、
clear-failed 等。`/jobs` 当前实现需核对 DTO。

---

## 4. 队列监控（Queues）— `/queues`

UI：队列卡片（名字、active/paused、waiting/active/completed/failed 计数）+ 单队列详情页
（JobGraph、失败任务列表、清空/暂停按钮）。

端点：
- `GET /queues` → `QueueResponseDto[]`（`{name, isPaused, statistics}`）
- `GET /queues/:name` → `QueueResponseDto`
- `PUT /queues/:name` → `QueueUpdateDto{isPaused}`（暂停/恢复）
- `DELETE /queues/:name/jobs` → `QueueDeleteDto{failed?}`（清空队列/失败任务）
- `GET /queues/:name/jobs?status=` → `QueueJobResponseDto[]`

**实现**：immich-go 当前用自研内存作业总线。需提供队列统计（各 job 类型计数、paused 状态）
并支持暂停/恢复/清空。队列状态可从内存 job 管理器导出，真实可得。

---

## 5. 维护与完整性（Maintenance & Integrity）

UI：
- 维护模式开关（MaintenanceSettings）：开启后只读/不可写，提供 restore 备份选项
- 完整性报告（integrity-report）：展示 checksum_mismatch / missing_file / untracked_file 计数

端点：
- `POST /admin/maintenance` → `SetMaintenanceModeDto{action: 'enable'|'disable'|'restore', restoreBackupFilename?}`
- `GET /admin/maintenance/status` → `MaintenanceStatusResponseDto{action, active, error?, progress?, task?}`
- `POST /admin/maintenance/login` → 维护令牌登录（可选）
- `GET /admin/maintenance/detect-install` → 检测旧安装文件夹（`MaintenanceDetectInstallResponseDto`）
- `GET /admin/integrity/summary` → `IntegrityReportSummaryResponseDto{checksum_mismatch, missing_file, untracked_file}`
- `GET /admin/integrity/:type`（`getIntegrityReport`）→ 报告明细（分页）
- `DELETE /admin/integrity/:id` → 删除报告项
- `GET /admin/integrity/file/:id`、`GET /admin/integrity/csv` → 导出

**实现**：
- 维护模式：可真实实现（一个全局 `maintenanceMode` 标志 + 写操作拒绝）。immich-go 是单实例，易做。
- 完整性检查：可真实跑——遍历 `resources/` 比对 `assets` 表记录的 checksum/文件存在性，
  生成 `checksum_mismatch`/`missing_file`/`untracked_file` 三类计数（纯 Go `sha1`/`os.Stat`）。
  这是**可真实实现**的，不是 stub。
- detect-install：检查 `resources`/`immich.db` 是否存在旧数据，返回可读写状态。

---

## 6. 数据库备份（Database Backups）— P2 / SQLite 专属方案

UI：列出备份、创建备份、上传、恢复、删除。

官方端点（PG 式，**不适用 SQLite**）：
- `GET /admin/database-backups`、`POST /admin/database-backups/start-restore`、
  `POST /admin/database-backups/upload`、`DELETE /admin/database-backups`

**实现决策**：SQLite 下替换为自研方案——`GET /admin/database-backups` 列出 `data/`
下的 `.db.bak` 快照；`POST /admin/database-backups`（或复用 start-restore）执行
`immich.db` 的 `VACUUM INTO` / 文件拷贝快照；恢复即拷贝回。返回诚实结构。
需登记为「SQLite 专属替代」，不伪装成 PG 式。

---

## 7. 服务器状态（Server Status）— `/server/statistics`、`/server/storage`

UI：ServerStatisticsPanel 显示照片数、视频数、总用量、每用户用量。

端点：
- `GET /server/statistics` → `ServerStatsResponseDto{photos, videos, usage, usagePhotos, usageVideos, usageByUser[]}`
- `GET /server/storage` → `ServerStorageResponseDto{diskAvailable(Raw), diskSize(Raw), diskUse(Raw), diskUsagePercentage, ...}`
- `GET /server/version`、`GET /server/version-check`、`GET /server/version-history`
- `GET /server/about` → about 信息

**实现**：
- statistics：可真实统计（COUNT assets + SUM 字节 + 按 owner_id 分组）。`usageByUser` 已有 `User` 配额字段。
- storage：已通过 `disk_unix.go`/`disk_windows.go` 真实实现（NO_STUBS #1 ✅）。
- version/about：已有处理；需核对 DTO。

---

## 8. 通知（Notifications，admin 触发）

UI：系统设置 NotificationSettings 里「发送测试邮件」；维护页可发站内通知。

端点：
- `POST /admin/notifications` → `NotificationCreateDto{title, description?, level?, type?, userId, data?}`（创建站内通知，存 notifications 表）
- `POST /admin/notifications/test-email` → `SystemConfigSmtpDto`（SMTP 发信 → 需外部 SMTP，**诚实 501** 或尽力实现纯 Go SMTP 发送）
- `GET /admin/notifications/templates/:name`、`POST /admin/notifications/templates`（模板渲染 → 可选）

**实现**：站内通知 `POST /admin/notifications` 可真实落库（notifications 表已存在）。
test-email 纯 Go 可用 `net/smtp` 真实发送（不依赖外部库），做到即真实实现。

---

## 9. 系统元数据 / 版本检查状态（System Metadata）

- `GET /system-metadata/admin-onboarding` + `POST /system-metadata/admin-onboarding` → `AdminOnboardingUpdateDto{isOnboarded}`
- `GET /system-metadata/reverse-geocoding-state` → `ReverseGeocodingStateResponseDto{lastImportFileName, lastUpdate}`
- `GET /system-metadata/version-check-state` → 版本检查状态

**实现**：admin-onboarding 可持久化（system_config 或独立表）；reverse-geocoding-state
可返回离线 GeoNames 导入状态（真实可得）。

---

## 10. 资产级 Job（Assets Jobs）

UI：用户页面/工具里「批量运行 job」（缩略图/元数据/转码/重复检测/OCR/人脸）。

端点：`POST /assets/jobs`（`runAssetJobs`）→ `AssetJobsDto{assetIds[], name, command?}`

**实现**：可真实实现——按 assetIds 对所选资产重跑 thumbnailGeneration/metadataExtraction/
videoConversion/duplicateDetection/ocr（ML 类按需降级）。

---

## 11. 多用户共享（P0 活动阶段，跨后台+前台）

不在「后台专属」但由后台/前台共同驱动，必须落地才能对等：
- P0-4 伙伴共享资产透出：timeline/search 的 `withPartners` 参数 join Partner 资产（只读）
- P0-5 相册内用户共享：`PUT /albums/:id/user/:userId`、`DELETE`、`GET /albums/:id/users`
  已存在路由，需补权限校验（editor/viewer）与前端入口，并把共享相册透出到对方时间线

---

## 实现批次规划（建议执行顺序，每批一小步即 commit）

1. **系统配置读写补全**：`GET/PUT /system-config` 完整 `SystemConfigDto` 形状 + 持久化 +
   `GET /system-config/defaults`。核对 storage-template-options。
2. **服务器统计/存储/版本/about** DTO 核对与补全（storage 已 ok）。
3. **队列监控** `/queues` 全套（统计+pause/resume/empty/jobs），基于现有 job 总线。
4. **任务状态** `/jobs` legacy 形状 + `POST /jobs` 触发 + `/jobs/:name` 命令。
5. **维护模式** enable/disable/status + **完整性检查** 真实扫描（checksum/missing/untracked）。
6. **admin 站内通知** 落库 + **test-email** 纯 Go SMTP 真实发送。
7. **system-metadata** admin-onboarding + reverse-geocoding-state + version-check-state。
8. **数据库备份** SQLite 专属快照方案（`/admin/database-backups`）。
9. **资产级 jobs** `POST /assets/jobs`。
10. **多用户共享** P0-4 / P0-5（透出 + 权限）。
11. **schemathesis 契约回归** + 真实浏览器冒烟（rule 8）逐项验证。

> 每个批次完成后立即 `go build`/`go vet` 验证并 commit+push，文件变更 ≤5。
