# Epic: 维护与自愈体系（Maintenance & Self-Healing）

> 状态：**讨论中（Draft）** — 方向已定，任务边界待逐项确认
> 负责人：patrick-gallery + agent
> 创建：2026-08-23
> 关联：`docs/GAP_ANALYSIS.md`、`STATUS.md`、`internal/app/jobs.go`

## 1. 背景与目标

内部错误修复、解码器升级、数据模型演进之后，目前只能靠手工 SQL / 手动删文件 /
临时 curl 来重建元数据与缩略图。本 Epic 的目标是把这套能力**产品化**：

- 以 **API 驱动**为主策略（与官方 Immich 契约保持一致，不引入 CLI 子命令）
- 让"重建/修复"成为可发现、可审计、可重复的正常运维动作
- 数据库具备**引用完整性与一致性的自我愈合**能力
- 缩略图/预览缓存携带**生成器版本信息**，程序升级后能识别并按需失效重建

## 2. 范围决策（2026-08-23 讨论结论）

| 能力 | 决策 | 备注 |
|---|---|---|
| API 触发重建/修复 | ✅ **做**（主策略） | 保持现有 `/api/assets/jobs`、`/api/jobs` 风格 |
| 强制全量重建（缩略图等） | ✅ **做** | 对齐官方 `force:true` 参数语义 |
| 备份数据库 `backupDatabase` | ✅ **做** | 队列名已注册但无实现 |
| 数据库一致性自我愈合 | ✅ **做** | 启动 repairPass 扩展成通用机制 |
| CLI 子命令（repair/vacuum 等） | ❌ **暂不做**（明确放弃） | 至少本 Epic 内不做；API 已覆盖场景 |
| 预览缓存版本化失效 | ✅ **做** | 元数据记录生成器版本，新版本检查兼容性 |

## 3. 现状盘点（2026-08-23 调查）

### 已有

| 端点 | 行为 | 局限 |
|---|---|---|
| `POST /api/assets/jobs` `{assetIds,name}` | 按资产列表触发任务；无条件重跑 | 只能显式列 id |
| `POST /api/jobs` `{name}` / `PUT /api/jobs/:name` | 全库队列启动；只捞缺失项（如 `has_thumbnail=false`） | **`force:true` 被忽略**，无法全量重跑 |
| `GET /api/jobs`、`/api/queues*` | 进度快照、暂停/恢复、清空 | — |
| `POST /api/admin/maintenance` | 维护模式开关（锁 UI） | 纯前端锁定，无数据操作 |

支持的 job：thumbnailGeneration / metadataExtraction / videoConversion /
duplicateDetection / ocr（smartSearch 需 ML backend）。

### 缺失（= 本 Epic 工作项来源）

1. 官方 `JobCreateDto.force` 未实现
2. `backupDatabase` 只有队列名，无实现
3. 无通用一致性自愈（现有 livephoto hidden 回填是一次性专项代码）
4. 缩略图/预览缓存无版本元数据，解码器升级后旧缓存不失效
5. 孤儿文件清理（preview cache、无主 thumbnail、指向已删除资产的引用）

## 4. 任务分解（Work Items）

### WI-1 force 全量参数（对齐官方）

- [ ] `PUT /api/jobs/:name` body `{force:true}` 时忽略"缺失项"过滤，全量重跑
- [ ] 适用 job：thumbnailGeneration、metadataExtraction、videoConversion、ocr
- [ ] 官方契约核对：`server/src/dtos/job.dto.ts` JobCreateDto（读原版确认字段）
- [ ] 契约测试：force=true 与缺省两条路径

验收：`curl -X PUT .../jobs/thumbnailGeneration -d '{"force":true}'`
重跑全部资产的缩略图。

### WI-2 backupDatabase 实现

- [ ] SQLite online-backup（`glebarez/sqlite` 下用 `VACUUM INTO 'file'` 或
      backup API），输出到 `resources/backups/immich-<ts>.db`
- [ ] 队列 job 形式跑（可从 admin UI 触发）；保留最近 N 份（默认 3，可配）
- [ ] `GET /api/backups` 列出可用备份？（待定：官方是否有对应端点，先查）
- [ ] WAL checkpoint 配合，保证备份一致性

验收：触发 job 后 `resources/backups/` 出现可用 db 文件，`sqlite3 ... 'PRAGMA integrity_check'` 通过。

### WI-3 启动 repairPass（一致性自愈）

把现有 livephoto 回填泛化为有序的修复步骤清单，每步幂等、可跳过：

- [ ] livePhotoVideoId 引用悬空 → 清空引用（官方行为：删除旧 motion asset）
- [ ] 被引用的 motion 视频 visibility ≠ hidden → 修正（已实现，保留）
- [ ] `visibility='archive'` 与 `is_archived` 不同步 → 同步
- [ ] `has_thumbnail` 与 `resize_path` 不一致 → 修正
- [ ] 孤儿 preview 缓存（`.preview.jpg` 无对应资产）→ 删除
- [ ] 孤儿 thumbnail 文件（thumbnail 目录里 id 不在 assets 表）→ 删除
- [ ] albums_assets_assets 指向不存在资产 → 删行
- [ ] 每步结果记日志 `[repair] step=X fixed=N`
- [ ] （可选）暴露 `GET /api/admin/repair/report` dry-run 报告端点

### WI-4 缓存版本化失效

- [ ] SystemConfig 增加 `cacheVersion`（或独立 kv）：当前生成器版本号
      = 解码器/图像处理管线的语义版本（如 `imgproc/1`、升级解码器时 bump）
- [ ] preview 缓存文件名或 sidecar 记录生成时版本（倾向：文件名带版本段，
      如 `<id>.v<N>.preview.jpg`，避免额外 IO）
- [ ] 启动/任务运行时发现旧版本缓存 → 视为无效（删除或忽略并重生成）
- [ ] thumbnail 同理（resize_path 是单值路径，需加列或在路径中编码版本）
- [ ] 与 WI-1 的 force 组合：版本 bump 后自动进入"待重建"集合

## 5. 待讨论 / 开放问题

1. **备份触发方式**：纯手动 job？还是加每日定时（复用 scheduler）？
2. **repairPass 是否需要 UI 入口**：官方 Admin 有 "Repair" 页（探测缺失文件等）
   ——是否要对齐其端点契约？（具体路径需读原版确认）
3. **缓存版本粒度**：全局一个版本号 vs 按"解码器种类"分版本（HEIC 升级不应
   使 JPEG 缩略图全部失效）→ 倾向按格式族分版本
4. **WI-4 存储位置**：版本编码进文件名 vs 存 DB 列 —— 文件名方案的目录枚举成本
5. **force 的并发安全**：全量重跑期间的写放大与 SQLite 单写者影响

## 6. 变更记录

- 2026-08-23: 创建 Epic，录入现状调查与范围决策（放弃 CLI 子命令）
