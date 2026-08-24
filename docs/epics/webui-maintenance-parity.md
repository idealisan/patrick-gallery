# Epic: Web UI 维护页功能对齐（Maintenance & Integrity Parity）

> 状态：**待开发（已记录，未实现）**
> 创建：2026-08-23
> 来源：用户报告「Web UI 上的维护菜单里的各种功能似乎都不可用」，要求先对照官方原版
> Web UI / 服务端源码分析清楚契约，记录成 Epic 后续实施。
> 契约权威：官方 Immich 源码（`server/src/services/integrity.service.ts`、
> `maintenance.service.ts`）+ 打包的官方 Web bundle（`internal/webroot/webui/`），
> 并在本地 8888 实例上用真实请求复现了每一处失败。

## 1. 背景与症状

官方 Web UI 的 **管理后台 → 维护（Maintenance）** 页面提供：

1. 三类完整性检查的「检查 / 刷新 / 全部删除」按钮：
   - 未跟踪文件（untracked files）
   - 缺失文件（missing files）
   - 校验和不匹配（checksum mismatch）
2. 「切换到维护模式」（maintenance mode）开关 —— 点击后直接报错。

在本项目当前版本上的实测结果（2026-08-23，端口 8888）：

| Web 操作 | 发出的请求 | 我们的结果 |
| --- | --- | --- |
| 页面加载/每 2s 轮询 | `GET /api/jobs` | 200 但 DTO 形状错误 → 页面 JS 抛 TypeError |
| 检查（Check）按钮 ×3 | `POST /api/jobs` `{name:"integrity-untracked-files"}` 等 | **400 unknown job** |
| 刷新（Refresh）按钮 ×3 | `POST /api/jobs` `{name:"integrity-…-refresh"}` | **400 unknown job** |
| 全部删除按钮 ×3 | `POST /api/jobs` `{name:"integrity-…-delete-all"}` | **400 unknown job** |
| 切换维护模式 | `POST /api/admin/maintenance` `{action:"start"}` | **400 unknown action: start** |

## 2. 官方契约（逐条核实）

### 2.1 GET /jobs（legacy 队列状态）

官方返回 `QueuesResponseLegacyDto`——**嵌套结构**：

```json
{
  "integrityCheck": {
    "jobCounts":  { "active": 0, "completed": 0, "failed": 0, "delayed": 0, "paused": 0 },
    "queueStatus": { "isActive": false, "isPaused": false }
  },
  "...每个队列同构..."
}
```

维护页与完整性报告页都以 2 秒间隔轮询该端点并读取
`integrityCheck.queueStatus.isActive` 来判断扫描是否进行中。
我们目前返回的是**扁平计数** `{active,completed,failed,delayed,paused}`，
`.queueStatus` 为 `undefined` → 页面轮询回调抛 `TypeError`，整页交互失效。
这是「维护菜单全部不可用」的**第一根因**。

### 2.2 POST /jobs（手动任务 ManualJobName）

`JobCreateDto { name: ManualJobName }`，与完整性相关的枚举值共 9 个：

```
integrity-untracked-files        integrity-untracked-files-refresh        integrity-untracked-files-delete-all
integrity-missing-files          integrity-missing-files-refresh          integrity-missing-files-delete-all
integrity-checksum-mismatch      integrity-checksum-mismatch-refresh      integrity-checksum-mismatch-delete-all
```

（同枚举还有 `person-cleanup`、`tag-cleanup`、`user-cleanup`、`memory-cleanup`、
`memory-create`、`backup-database`，需一并核对注册情况。）

官方语义（`integrity.service.ts`，全部走 `integrityCheck` 队列、分批小任务）：

- **`integrity-*`（检查）**：
  - untracked：先刷新旧报告（见 refresh），再爬取 `encoded-video/`、`library/`、
    `upload/`（对照 asset 表）与 `thumbs/`（对照 asset_file 表），不在库中的路径
    写入报告；分批（JOBS_LIBRARY_PAGINATION_SIZE）投递子任务。
  - missing：流式枚举 asset/originalPath、encodedVideoPath、asset_file.path，
    逐批 `stat`；文件回来了→删过期报告，仍缺失→写报告（带 assetId/fileAssetId 溯源）。
  - checksum：对 asset.originalPath 流式 sha1 与库中 checksum 比较；带
    timeLimit/percentageLimit 断点续扫（checkpoint 存 system metadata）；
    失败写报告（带 assetId）；ENOENT 视为 missing 不重复报。
- **`integrity-*-refresh`（刷新）**：只复核已有报告——文件重新出现/校验和恢复
  一致→删除对应报告行；不做全盘扫描。
- **`integrity-*-delete-all`（全部删除）**：按报告行的溯源处置文件——有
  `assetId` → 资产进回收站（trash）；有 `fileAssetId` → 直接删除该文件（如动态视频）；
  否则 `unlink` 裸路径（未跟踪文件）——然后删除报告行。分批投递。

注意：**delete-all 会真的删文件/进回收站**，Web 端有确认对话框。

### 2.3 完整性报告端点

| 端点 | 官方行为 | 我们的现状 |
| --- | --- | --- |
| `GET /admin/integrity/summary` | `{checksum_mismatch, missing_file, untracked_file}` 计数 | ✅ 形状一致 |
| `GET /admin/integrity/report?type=&cursor=&limit=` | `{items:[{id,type,path}], nextCursor}`，**id 为 UUID v4** | ⚠️ 形状一致但 id 用的是相对路径而非 UUID；无 assetId/fileAssetId 溯源 |
| `GET /admin/integrity/report/{type}/csv` | CSV 表头 `id,type,assetId,fileAssetId,path`，流式 | ⚠️ 有端点但列不同 |
| `GET /admin/integrity/report/{id}/file` | 按 UUID 取报告行，以 `ImmichFileResponse` 下载被标记的文件 | ❌ 404 |
| `DELETE /admin/integrity/report/{id}` | 有 assetId→资产进回收站；fileAssetId→删文件；否则 unlink 并删行 | ⚠️ 目前一律直接 unlink |

### 2.4 维护模式（POST /admin/maintenance）

- 请求体 `SetMaintenanceModeDto { action: MaintenanceAction, restoreBackupFilename? }`，
  `MaintenanceAction = start | end | select_database_restore | restore_database`。
  我们当前只接受 `enable/disable/restore` → **第二根因**。
- `GET /admin/maintenance/status` 正常时返回 `{ active:false, action:"end" }`
  （我们的空串 `action:""` 不符合枚举）。
- **官方语义是重量级的**：`start` 生成 maintenance secret、写入 system metadata、
  发出 `AppRestart` 事件——**服务进程退出并以「维护模式」重新启动**，进入一个
  独立的极简维护 UI（恢复数据库备份等）；接口返回 `{ jwt }`（maintenance JWT，
  用于进入维护 UI）。另有 `/admin/maintenance/detect-install`（检测既有安装）、
  `/admin/maintenance/login`（token 换 jwt）配套端点。
- 我们目前只是进程内布尔开关 + 自定义动作名，两者都不是官方契约。

## 3. 根因汇总

| # | 根因 | 影响 |
| --- | --- | --- |
| G1 | `GET /jobs` 返回扁平计数而非 `{jobCounts,queueStatus}` 嵌套形状 | 维护页 + 报告页轮询回调 TypeError，页面整体不可用 |
| G2 | jobRegistry 未注册 9 个 `integrity-*` 手动任务名（及相关 cleanup 任务名） | 检查/刷新/全部删除按钮全部 400 |
| G3 | 维护模式动作枚举不符（enable/disable/restore vs start/end/select_database_restore/restore_database），响应缺 `{jwt}` | 切换维护模式点击即报错 |
| G4 | 报告行无 UUID id、无 assetId/fileAssetId 溯源；CSV 列不同；`/{id}/file` 下载缺失；DELETE 不做回收站语义 | 报告详情页下载/删除行为与官方不一致 |
| G5 | 无 cron 驱动的定期完整性检查（SystemConfig.integrityChecks: enabled+cronExpression，checksum 另有 percentageLimit/timeLimit） | 深度项，非按钮失效原因 |

## 4. 工作项（Work Items）

- **WI-1 修正 GET /jobs legacy 形状**【P0，解锁整个维护页】
  每个队列输出 `{jobCounts:{active,completed,failed,delayed,paused},
  queueStatus:{isActive,isPaused}}`；`isActive` 取该队列 jobState.running。
  同步核对 `GET /queues`（新版形状）不受影响。
- **WI-2 注册 9 个 integrity-* 手动任务**【P0】
  映射到现有 `runIntegrityScan` 能力并补齐官方语义：
  - 检查=刷新旧报告+全量扫描（沿用现有 fs 审计）；
  - refresh=仅复核已有报告行（stat/sha1），过期则删行；
  - delete-all=按溯源处置文件（trash/unlink）后清空该类型报告。
  全部走 integrityCheck 队列、复用 S2/S3 的分批与链式机制（大批量拆小批）。
- **WI-3 报告模型升级为官方形状**【P1】
  报告行增加 UUID 主键、assetId/fileAssetId 可空溯源列（SQLite 迁移）；
  report/csv/file 端点对齐；`GET /report/{id}/file` 实现文件下载（走
  ImmichFileResponse 等价物：Content-Disposition octet-stream）；
  DELETE 实现 trash/unlink 分支语义。
- **WI-4 维护模式对齐**【P1，需设计决策】
  动作枚举改官方四值；status 返回 `action:"end"`（非空枚举值）。
  `start` 的完整官方语义（进程重启进入独立维护 UI + maintenance JWT）在
  「单静态二进制」硬规则下需要一个受控方案（如 exec 自身替换 argv 进入
  维护模式、或同进程切换路由面），实施前单独评审；至少先做到契约层兼容
  （正确的枚举、响应形状 `{jwt}`、status 语义），再逐步逼近重启流。
- **WI-5 定期完整性检查（cron 配置）**【P2】
  SystemConfig 增加 `integrityChecks.{untrackedFiles,missingFiles,checksumFiles}`
  （enabled + cronExpression；checksum 含 percentageLimit/timeLimit），
  复用现有 scheduler；与 maintenance-self-healing Epic 的定时备份决策保持一致风格。
- **WI-6 回归防护**【贯穿】
  schemathesis 允许清单同步更新；新增针对上述端点形状的 Go 测试；
  浏览器冒烟测试扩展「维护页可达、三类按钮可点击且 200、无 pageerror」。

## 5. 开放问题

1. ~~WI-4 维护模式「重启进入维护 UI」的实现载体~~ **已决议（2026-08-23）**：
   Go 版**不做真实进程重启**，采用**虚拟重启 / 进程内模式切换**——进入维护模式时
   优雅停掉所有其他功能（调度器、任务队列、媒体管线等），路由面切到维护 UI，
   进程保持存活；对外契约层仍返回 `{jwt}` 等官方形状。
2. delete-all / 单条删除的文件处置语义 —— **已核实官方做法（2026-08-23）**：
   官方 `integrity_report` 表含可空外键 `assetId`(→asset)、`fileAssetId`(→asset_file)，
   删除时三分支：有 assetId → 资产进回收站（deletedAt+status=trashed，可在回收站
   恢复）；有 fileAssetId → 永久删除该派生文件（如转码视频）；否则 unlink 裸路径，
   最后删报告行。我们当前一律直接 unlink，缺回收站分支。
   待确认：采用同一三分支映射到我们的模型（IsTrash/TrashedAt 已具备；
   EncodedVideoPath 对应派生文件分支；预览/缩略图文件归入哪支实施时定）。
3. checksum 断点续扫 checkpoint 存放 —— **已核实官方做法（2026-08-23）**：
   官方存于通用 KV 表 `system_metadata(key TEXT PK, value JSON)`，键
   `integrityChecksumCheckpoint`，值 `{date}`＝上次处理到的资产 createdAt 游标；
   扫描按 createdAt 流式进行，触发 timeLimit/percentageLimit 停止条件时回写游标，
   下次从游标续扫。
   建议：我们也建同构的小型 `system_metadata` KV 表（后续维护模式 secret 等
   也可复用），不塞进 SystemConfig 单例的 ConfigJSON。

## 6. 决议记录

- 2026-08-23：Epic 创建。仅记录与分析，未做代码改动（用户明确「以后接下来做」）。
  所有契约均已在本地实例复现验证（G1–G3 的失败响应原文见 §1 表格）。
- 2026-08-23：开放问题 1 决议——虚拟重启/进程内切换，不做真实重启；
  问题 2、3 已对照官方源码核实并记录（integrity.repository.ts /
  integrity-report.table.ts / system-metadata.repository.ts），待确认后转入实施。
- 2026-08-23/24：**实施完成（WI-1..WI-5 + WI-6 验证）**，用户确认三个开放
  问题决议后开工。提交清单：
  - `1dccf8c` WI-1+NaN：QueueStatisticsDto 补 waiting；GET /jobs 改嵌套 legacy 形状
  - `1c74b97` WI-3：IntegrityReport(UUID+溯源)/SystemMetadata KV 模型落库；
    scan 持久化；report/csv/file/delete 官方形状与三分支处置
  - `cb44c38` WI-2：9 个 integrity-* 手动任务注册（检查/刷新/删除全部），
    queueKey 挂 integrityCheck 队列；backup-database 别名；测试覆盖
  - `c6dc81b` WI-4：维护模式官方动作枚举 + {jwt} + 虚拟重启（停调度器、暂停队列）
  - `d11cb40` WI-5：校验和断点续扫，游标存 system_metadata(integrityChecksumCheckpoint)
  - `f4f81e7` 路径归一化修复：untracked 误报 149→3（真实孤儿）
- 验证记录：schemathesis PASS（critical 全 0）；真实浏览器（Chrome headless，
  规则 8）：维护页 `/admin/maintenance` 正常渲染，「检查全部/检查/刷新」真实
  点击全部 200 且触发正确任务名、「切换到维护模式」点击 → 201 {jwt} → 无
  pageerror；退出恢复 `{active:false,action:"end"}`。
- 2026-08-24：补全维护模式客户端链路（`3094702`）。官方 Web 的 toggle 点击后
  本身不做任何导航——全靠 ①进程重启后 `GET /server/config` 的 maintenanceMode
  标志触发根布局重定向 `/maintenance`，②WebSocket 事件 AppRestartV1（显示
  「服务器正在重新启动」弹窗）与 MaintenanceStatusV1（action=end 清除维护态）。
  虚拟重启方案下补齐：config 标志实时化 + 两个 WS 事件下发（SIO 名映射）。
  浏览器实测全流程通过：点击→弹窗→任意导航进入维护 UI（含退出按钮）→退出恢复。
- 2026-08-24：修复相册页崩溃（`3094702`）。官方契约 albumUsers「首项恒为
  owner、形如 {role,user}」，我们此前返回 `{userId,role}` 平铺数组且不含
  owner，相册页 `albumUsers[0].user.id` 取 undefined 崩溃。已按官方形状重构。
- 剩余深度项（P2/P3）：SystemConfig.integrityChecks 定时 cron 配置面
  （WI-5 的配置部分）、checksum timeLimit/percentageLimit 可调参数、
  维护 UI 内的 detect-install/install 流程深化。
