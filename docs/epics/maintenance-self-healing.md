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

> Story 拆分见 [stories/README.md](stories/README.md) —— 9 个可独立开发验证的
> Story（S1–S9）及其依赖图、优先级与实施顺序建议。本节保留 WI 作为需求级描述。

### WI-1 force 全量参数（对齐官方）

- [ ] `PUT /api/jobs/:name` body `{force:true}` 时忽略"缺失项"过滤，全量重跑
- [ ] **分批执行（见 §5.5 决策）**：每批 N 个资产（如 50），批次完成后再链式
      创建下一批，禁止一次性全量任务
- [ ] 适用 job：thumbnailGeneration、metadataExtraction、videoConversion、ocr
- [ ] 官方契约核对：`server/src/dtos/job.dto.ts` JobCreateDto（读原版确认字段）
- [ ] 契约测试：force=true 与缺省两条路径

验收：`curl -X PUT .../jobs/thumbnailGeneration -d '{"force":true}'`
重跑全部资产的缩略图。

### WI-2 backupDatabase 实现

- [ ] SQLite online-backup（`glebarez/sqlite` 下用 `VACUUM INTO 'file'` 或
      backup API），输出到 `resources/backups/immich-<ts>.db`
- [ ] 队列 job 形式跑（可从 admin UI 触发）；保留最近 N 份（默认 3，可配）
- [ ] 触发方式跟随官方原版行为（§5.1 决策）：实现前读原版确认是否支持定时
- [ ] WAL checkpoint 配合，保证备份一致性

验收：触发 job 后 `resources/backups/` 出现可用 db 文件，`sqlite3 ... 'PRAGMA integrity_check'` 通过。

### WI-3 启动 repairPass（一致性自愈）

把现有 livephoto 回填泛化为有序的修复步骤清单，每步幂等、可跳过。
**UI 入口对齐官方 Admin "Repair" 页**（§5.2 决策），实现前读原版确认端点契约。

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
- [ ] 版本信息**存数据库**（§5.4 决策），不编码进文件名
- [ ] **按格式族分粒度**（§5.3 决策）：如 `heic/1`、`jpeg/1`；是否 bump 由
      每次发版时的兼容性评估决定——有 breaking change 才 bump
- [ ] 启动/任务运行时发现旧版本缓存 → 视为无效（删除或忽略并重生成）
- [ ] thumbnail 同理（resize_path 需配套版本记录列）
- [ ] 与 WI-1 的 force 组合：版本 bump 后自动进入"待重建"集合

## 5. 待讨论 / 开放问题（✅ 已全部讨论并决策，2026-08-23）

1. **备份触发方式**：纯手动 job？还是加每日定时（复用 scheduler）？
   1. 纯手动还是定时，这个参考官方原版的Web UI的功能去做就好了。如果官方原版就是纯手动的，那我们就保持纯手动。如果官方原版支持定时的，那么就做成定时。
2. **repairPass 是否需要 UI 入口**：官方 Admin 有 "Repair" 页（探测缺失文件等）
   ——是否要对齐其端点契约？（具体路径需读原版确认）
   1. 这个当然也要跟官方原版的Web UI做一致的功能。我们始终的一个原则就是与官方原版的Web UI和客户端保持兼容性和一致性，并且我们是直接复用的原版Web UI。
3. **缓存版本粒度**：全局一个版本号 vs 按"解码器种类"分版本（HEIC 升级不应
   使 JPEG 缩略图全部失效）→ 倾向按格式族分版本
   1. 这个我觉得应该按照新版本发布的时候，需要自己去明确跟旧版本的兼容性，而不是简单的按照编码器的种类或者是版本号去划分。也有可能升级了新版本的之后，仍然能够跟以前的兼容，它不需要去重建或者是修复，那就不用做什么。如果需要重建和修复的话，那意味着新版本开发过程中已经意识到了有break change，那么新版本才需要去做重建。这个在每次发新版的时候，内部去处理就好了。但这里提到的按不同的格式来区分，比如你提到的这个例子是很有道理的。就这个问题而言，确实按照格式来分版本是好的决策。
4. **WI-4 存储位置**：版本编码进文件名 vs 存 DB 列 —— 文件名方案的目录枚举成本
   1. 这个当然存数据库啊，不要依赖文件名，因为文件名可能受到外部的干扰破坏，以及各种可能的边缘场景，可能有文件系统兼容性啊等等。
5. **force 的并发安全**：全量重跑期间的写放大与 SQLite 单写者影响
   1. 这个全量的话，通过一些策略做一些改进。比如说把这整个全量的做成一个，分成很多个小批次任务的去做不就好了吗？这个小批次任务我考虑是不是不应该做成一次性触发全量的时候，就创建好很多个小批次任务，而是在每个小批次任务结束的时候，再去创建新的小批次任务，这样的话避免任务队列。短时间内急剧膨胀。其实就相当于给做成了一个分散很多步的一个多部的小任务去完成。虽然这可能导致比如两个小任务之间有用户的动作导致插入了新的数据啊等等。但是这种场景那点数据其实不需要太详细的考虑它，因为用户的新插入的数据的话，他自然会按照新的流程去处理。那也不应该涉及到重建，所以那点影响其实不大。全量重建一定不要做成一次性的，把所有的数据在一个任务里全做完的，那样的话风险很高，而且性能可能也会容易出现问题，做成很多个小批量的。

## 6. 变更记录

- 2026-08-23: 创建 Epic，录入现状调查与范围决策（放弃 CLI 子命令）
- 2026-08-23: §5 五个开放问题全部讨论并决策（备份跟随官方、Repair UI 对齐、
  格式族版本粒度+发版时评估 bump、版本存 DB、force 分批链式执行）；
  WI-1/2/3/4 按决策更新；新增开发流程文档 docs/README.md
- 2026-08-23: WI 拆分为 S1–S9 九个 Story（stories/ 目录），含依赖图、优先级与
  建议实施顺序 S1→S2→S3
