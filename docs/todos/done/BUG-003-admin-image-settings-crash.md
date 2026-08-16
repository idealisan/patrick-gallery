# BUG-003：管理后台「图像设置」页崩溃 + 用户管理页 i18n 日期报错

> 状态：Open（已分析，未修复） · 日期：2026-08-16 · 严重度：High（图像设置页整页崩溃）/ Medium（用户管理页仅控制台告警）
> 组件：
> - 图像设置崩溃：`internal/app/misc.go:592`（`handleSystemConfigGet`，路由 `app.go:290` `/api/system-config`）；官方 Web `web/src/routes/admin/system-settings/ImageSettings.svelte`。
> - i18n 日期报错：官方 Web `web/src/lib/services/user-admin.service.ts:70-82`（路由 `/admin/users` 经 `/api/admin/users` 提供数据）；immich-go `/api/admin/users` 返回 `deletedAt: null`。
> 与 `NO_STUBS.md` 区分：
>   - 错误 1（colorspace）是**契约/DTO 形状缺失**——`/api/system-config` 缺整个 `image` 配置块，官方 Web 读 `config.image.colorspace` → `config.image` 为 `undefined` → `Uncaught TypeError` 整页崩溃。**属真实 bug，必须修复后端 DTO。**
>   - 错误 2（i18n `Invalid time value`）是**官方 Web 前端对 `deletedAt: null` 的空值处理缺陷**——immich-go 返回的 `deletedAt: null` 是 `UserAdminResponseDto` 契约下的**正确值**（活跃用户即应为 null）。后端已按契约返回正确数据，问题在前端急切格式化该 null 为 Invalid Date。详见「根因」与「修复方向」。

## 现象（来自用户报告）

在管理后台点击「图像设置」(`/admin/system-settings` 的 Image 子页) 以及部分其他管理功能时，浏览器控制台报错：

```
[svelte-i18n] Message "admin.user_restore_scheduled_removal" has syntax error: Invalid time value
...
Uncaught TypeError: Cannot read properties of undefined (reading 'colorspace')
    at 58.D2O3ByV7.js:1:43263
    at get checked (58.D2O3ByV7.js:1:43418)
```

两处独立的客户端错误：

1. **`Cannot read properties of undefined (reading 'colorspace')`** —— 出现在图像设置页。**官方 Web 读取 `config.image.colorspace`，而 immich-go 的 `/api/system-config` 返回的 DTO 中根本没有 `image` 这个对象**（连 `image` 块整体都缺失），于是 `config.image` 为 `undefined`，`.colorspace` 抛 `TypeError`，图像设置页崩溃。
2. **`[svelte-i18n] Message "admin.user_restore_scheduled_removal" has syntax error: Invalid time value`** —— 出现在用户管理页。`admin_restore_scheduled_removal` 这条 i18n 消息含 ICU 日期占位符 `{date, date, long}`，官方 Web 在构造“恢复用户”操作按钮的副标题时**急切地**把每个用户的 `deletedAt` 喂给 `DateTime.fromISO(...)`；对活跃用户（immich-go 返回 `deletedAt: null`，这是契约正确值）而言得到 Invalid Date，svelte-i18n 格式化时抛 “Invalid time value”。

## 证据（本地真实请求，admin@immich.app 登录后）

### 证据 A —— `/api/system-config` 实际返回（缺 `image` 块）

```
$ curl -s -b <cookie> http://127.0.0.1:8081/api/system-config
{"externalDomain":"","id":"singleton","isPublic":false,"loginRequired":true,
 "newPasswordRequired":false,"releaseChannel":"nightly","repository":"immich-go",
 "trashDays":30,"version":"1.0.0-go"}
```

对照官方 `SystemConfigDto`（`server/src/dtos/system-config.dto.ts:409-435`）要求的嵌套块：`backup / ffmpeg / logging / machineLearning / map / newVersionCheck / nightlyTasks / oauth / passwordLogin / reverseGeocoding / metadata / storageTemplate / job / image / trash / theme / library / notifications / templates / server / user / integrityChecks`。

immich-go 当前**一个嵌套块都没返回**，只回了一组扁平的遗留键。其中直接被图像设置页消费的 `image` 块整体缺失。

### 证据 B —— `/api/admin/users` 实际返回（活跃用户 `deletedAt: null`）

```
$ curl -s -b <cookie> http://127.0.0.1:8081/api/admin/users
[{"id":"215bf812-970d-4b8b-bb72-89a0968c129c","email":"admin@immich.app","name":"Administrator",
  "isAdmin":true,"avatarColor":"primary","createdAt":"2026-08-16T01:33:05.023572625Z",
  "updatedAt":"2026-08-16T01:33:05.023572825Z","deletedAt":null,
  "profileChangedAt":"2026-08-16T01:33:05.023572825Z","profileImagePath":"",
  "quotaSizeInBytes":null,"quotaUsageInBytes":2220294,"shouldChangePassword":false,
  "status":"active","storageLabel":null,"oauthId":"","license":null}]
```

`deletedAt: null` 对活跃用户是 `UserAdminResponseDto.deletedAt: Date | null` 的**契约正确值**（官方服务端对未删除用户同样返回 `null`）。后端未违反契约。

## 官方网页版实际消费代码（v3.1.0，权威依据）

> 以下代码取自 immich 仓库 **v3.1.0** 官方 Web 源码（raw.githubusercontent.com），并与本仓库实际嵌入的官方构建产物核对一致。OpenAPI 规范信息不足，以真实 Web 源码为准。

### 错误 1：`config.image.colorspace` 读取（图像设置页崩溃）

官方组件 `web/src/routes/admin/system-settings/ImageSettings.svelte` 直接读取 `config.image` 的多个子字段：

```svelte
// 第 200-202 行（读取 config.image.colorspace，与本次崩溃栈 58.D2O3ByV7.js:1:43263 对应）
checked={configToEdit.image.colorspace === Colorspace.P3}
onToggle={(isChecked) => (configToEdit.image.colorspace = isChecked ? Colorspace.P3 : Colorspace.Srgb)}
isEdited={configToEdit.image.colorspace !== config.image.colorspace}

// 同文件还读取（第 38/60/69/78/97/118/127/136/151/166/180/189/213 行）：
config.image.thumbnail.format / .thumbnail.size / .thumbnail.quality / .thumbnail.progressive
config.image.preview.format / .preview.size / .preview.quality / .preview.progressive
config.image.fullsize.enabled / .fullsize.format / .fullsize.quality / .fullsize.progressive
config.image.extractEmbedded
```

官方后端契约 `SystemConfigImageDto`（`server/src/dtos/system-config.dto.ts:386-394`）：

```ts
const SystemConfigImageSchema = z.object({
  thumbnail:  { format, quality, size, progressive },          // SystemConfigGeneratedImageDto
  preview:    { format, quality, size, progressive },
  fullsize:   { enabled, format, quality, progressive },        // SystemConfigGeneratedFullsizeImageDto
  colorspace: ColorspaceSchema,                                 // 'srgb' | 'p3'
  extractEmbedded: boolean,
});
```

**本仓库实际嵌入的官方构建产物核对一致**：`internal/webroot/webui/_app/immutable/nodes/58.D2O3ByV7.js`（即用户崩溃栈所指文件）确含 `n(m).image.colorspace` 读取：

```
...r=v(()=>n(m).image.colorspace===re.P3),a=v(()=>n(m).image.colorspace!==re.Srgb)...
```

由于 immich-go `/api/system-config` 不返回 `image` 键，`config.image` 为 `undefined`，`.colorspace` 抛 `Uncaught TypeError`，图像设置页整页崩溃。

### 错误 2：`user_restore_scheduled_removal` 日期格式化（用户管理页告警）

i18n 文案 `i18n/en.json:476`：

```json
"user_restore_scheduled_removal": "Restore user - scheduled removal on {date, date, long}",
```

官方服务 `web/src/lib/services/user-admin.service.ts:70-82`：

```ts
const getDeleteDate = (deletedAt: string): Date =>
  DateTime.fromISO(deletedAt).plus({ days: serverConfigManager.value.userDeleteDelay }).toJSDate();

const Restore: HeaderButtonActionItem = {
  icon: mdiDeleteRestore,
  title: $t('restore'),
  color: 'primary',
  data: {
    title: $t('admin.user_restore_scheduled_removal', { values: { date: getDeleteDate(user.deletedAt!) } }),
  },
  $if: () => !!user.deletedAt && user.status === UserStatus.Deleted,
  onAction: () => modalManager.show(UserRestoreConfirmModal, { user }),
};
```

注意：`data.title` 在**构造每个用户操作对象时即被急切求值**（不受 `$if` 守卫影响）。`getUserAdminActions($t, user)` 在用户管理列表布局 `web/src/routes/admin/users/(list)/+layout.svelte:53` 中对**每一行**调用，因此对每个活跃用户都会执行 `getDeleteDate(user.deletedAt!)`。

对活跃用户 `deletedAt` 为 `null`：
- `DateTime.fromISO(null)` → Luxon 返回 **Invalid DateTime**；
- `.plus({days: userDeleteDelay}).toJSDate()` → 返回 **Invalid Date**（`new Date(NaN)`）；
- `$t('admin.user_restore_scheduled_removal', { values: { date: InvalidDate } })` 用 `{date, date, long}` 格式化 Invalid Date → `Intl.DateTimeFormat` 抛 `RangeError: Invalid time value` → svelte-i18n 记 `[svelte-i18n] Message "admin.user_restore_scheduled_removal" has syntax error: Invalid time value`。

**本仓库实际嵌入的官方构建产物核对一致**：`internal/webroot/webui/_app/immutable/chunks/D3fJcYXx.js` 含同一逻辑：

```
_removal`,{values:{date:(e=>z.fromISO(e).plus({days:J.value.userDeleteDelay}).toJSDate())(t.deletedAt)}})},$if:()=>!!t.deletedAt&&t.sta...
```

（其中 `z.fromISO` = Luxon `DateTime.fromISO`，`J.value.userDeleteDelay` = `serverConfigManager.value.userDeleteDelay`，`t.deletedAt` = `user.deletedAt`。）

## 根因

### 错误 1（colorspace）—— 后端 DTO 形状缺失（真实 bug）

`internal/app/misc.go:592` 的 `handleSystemConfigGet` 只返回一组扁平遗留键（`id / loginRequired / isPublic / externalDomain / newPasswordRequired / trashDays / repository / releaseChannel / version`），**完全没有按官方 `SystemConfigDto` 返回任何嵌套配置块**，尤其缺失图像设置页强依赖的 `image` 块（`thumbnail / preview / fullsize / colorspace / extractEmbedded`）。官方 Web 的 `ImageSettings.svelte` 读到 `config.image === undefined`，访问 `.colorspace` 抛 `Uncaught TypeError`，图像设置页崩溃。

### 错误 2（i18n Invalid time value）—— 官方 Web 前端空值处理缺陷（后端已按契约返回正确值）

immich-go 的 `/api/admin/users` 对活跃用户返回 `deletedAt: null`，这是 `UserAdminResponseDto` 契约下的**正确值**（官方服务端对未删除用户同样返回 `null`）。崩溃并非后端 DTO 形状违反契约，而是官方 Web v3.1.0 的 `user-admin.service.ts` 在构造“恢复用户”按钮副标题时，对**每个用户急切**执行 `DateTime.fromISO(user.deletedAt!)`；当 `deletedAt` 为 `null` 时得到 Invalid Date，格式化 i18n 日期占位符失败而告警。

结论：该错误是**上游官方 Web（v3.1.0）的空值处理缺陷**，被 immich-go 正确返回的 `deletedAt: null` 触发。因 `AGENTS.md` 规则 6 要求 Web UI 必须是官方 immich 前端（不可手改 SPA），且 `deletedAt` 对活跃用户**必须**为 `null`（改为其它值会违反契约、造成假数据），此错误无法在不破坏契约的前提下由后端“修掉”。

## 修复方向（待实施 —— 本报告不修改任何源码）

### 错误 1（colorspace）—— 必须修复后端：让 `/api/system-config` 返回完整 `SystemConfigDto`

在 `internal/app/misc.go` 的 `handleSystemConfigGet`（及 `handleSystemConfigUpdate` 的回显）中，按官方 `SystemConfigDto`（`server/src/dtos/system-config.dto.ts:409-435`）补齐嵌套块，至少必须包含被 Web 消费的 `image` 块（`web/src/routes/admin/system-settings/ImageSettings.svelte` 读取 `image.thumbnail/preview/fullsize` 与 `image.colorspace`、`image.extractEmbedded`）。建议字段与默认值（参考官方 server 默认值）：

- `image.thumbnail` / `image.preview`：`{ format: "jpeg"|"webp", quality: 80, size: 256/2048, progressive: false }`
- `image.fullsize`：`{ enabled: true, format: "jpeg"|"webp", quality: 80, progressive: false }`
- `image.colorspace`：`"srgb"`（或 `"p3"`，需与前端 `Colorspace` 枚举一致）
- `image.extractEmbedded`：`false`
- 同时建议补齐其余被各 system-settings 子页消费的块（`ffmpeg / map / library / oauth / job / trash / theme / metadata / storageTemplate / reverseGeocoding / newVersionCheck / notifications / templates / server / user / logging / machineLearning / backup / nightlyTasks / integrityChecks`），否则 FFmpeg、Map、Library 等子页同样会因缺失对应块而崩溃或表现异常（见「影响范围」）。

实现注意：
- 这些配置值目前 immich-go 并未全部在 `SystemConfig` 模型里持久化，可先以合理常量/默认值返回，确保 DTO 形状与官方一致（满足“行为对等、真实工作”的硬规则 7；返回真实默认值不是假成功桩）。
- 后续如要在 UI 上真正可编辑，再把这些子块落到 `SystemConfig` 持久化模型与 `handleSystemConfigUpdate` 的写入/回显逻辑。
- 计算成本：纯常量返回，无开销。

### 错误 2（i18n Invalid time value）—— 上游官方 Web 缺陷，后端无法在不违约前提下修复

- **诚实结论**：后端已按契约返回正确数据（`deletedAt: null`），本错误不属后端 DTO 形状违反，也不属假成功桩（后端做了真实工作）。属 `AGENTS.md` 规则 6 下“官方 Web 前端的已知限制”。
- **影响**：仅为控制台告警，**不会导致用户管理页崩溃或阻断功能**——“恢复用户”按钮本身由 `$if: () => !!user.deletedAt && status===Deleted` 守卫，只对已删除用户显示；那条被急切格式化但永不展示的副标题仅产生告警。
- **可选处置（待团队决策）**：
  1. 在 `docs/GAP_ANALYSIS.md` / `docs/NO_STUBS.md` 中记录为“上游官方 Web v3.1.0 限制（benign 控制台告警）”，并标注官方契约值 `deletedAt: null` 正确；
  2. 若需彻底消除，只能等待/对齐到已修复该空值处理的官方 immich Web 构建版本（即升级所嵌入的官方 Web 到含上游修复的 tag），再重新 pin 并嵌入——这属于“换官方 Web 版本”动作，须由 lead 决策并遵循 `THIRD_PARTY.md` / `AGENTS.md` 规则 6 的版本一致性要求；
  3. **不可接受**的做法：把活跃用户的 `deletedAt` 改成非空日期以“规避”崩溃——那会伪造数据、违反 `UserAdminResponseDto` 契约，违背硬规则 7。

## 影响范围

- **错误 1（colorspace）**：仅影响管理后台「系统设置 → 图像设置」页——点击即整页崩溃（`Uncaught TypeError`）。同时，因 `/api/system-config` 整体缺所有嵌套块，**FFmpeg 设置、地图设置、库设置、OAuth、作业设置、主题、元数据、存储模板等 system-settings 子页大概率同样因缺失对应块而崩溃或显示异常**，建议一并验证。
- **错误 2（i18n）**：仅影响「用户管理」页控制台告警；不阻断页面渲染与任何功能（恢复按钮仅对删除用户出现）。非致命。
- 两者均**不影响**核心看图/上传/同步链路与 `/api/server/storage`、`/api/server/statistics` 等其它端点。

## 验证方式

- **错误 1 修复后**：
  - `curl /api/system-config` 应包含 `"image":{ "thumbnail":{...}, "preview":{...}, "fullsize":{...}, "colorspace":"srgb"|"p3", "extractEmbedded":false }`，且 `colorspace` 为合法枚举值。
  - 经反向代理用**真实无头浏览器**登录后打开 `/admin/system-settings`（Image 子页），确认：
    1. 页面到达且**无 `pageerror`**；
    2. “Colorspace”开关（P3 / srgb）正常显示且可切换；
    3. 控制台不再出现 `Cannot read properties of undefined (reading 'colorspace')`。
  - 契约回归：跑 `scripts/schemathesis_check.py`，确认 `getSystemConfig` 的 `response_schema_conformance` 通过。
- **错误 2 验证（benign 告警）**：
  - 真实浏览器打开 `/admin/users`，确认页面正常渲染、无整页崩溃；控制台中 `admin.user_restore_scheduled_removal ... Invalid time value` 仅在升级官方 Web 版本或上游修复后才消失。
  - 对“已删除用户”行点击「恢复」确认还原流程正常（功能不受该告警影响）。
- 任何 Web UI 变更须以真实浏览器（非仅 curl）按 `AGENTS.md` 规则 8 复核，确认无客户端 `pageerror`。
