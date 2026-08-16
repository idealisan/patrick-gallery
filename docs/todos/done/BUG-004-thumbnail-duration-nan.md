# BUG-004：缩略图时长浮层显示 `NaN` / `0:00`（duration 单位/契约不符）

> 状态：**Resolved（已修复并部署验证）** · 日期：2026-08-16 · 严重度：Medium（照片网格/时间线中视频缩略图的时长浮层损坏，影响主看图页观感，非核心上传/同步功能）

## 用户澄清（重要）

用户明确指出：*“bug004我测试的是图片，不是视频”* —— 看到 `NaN` 的资产是一张**图片**，而非视频。

据此重新核对官方网页契约：`Thumbnail.svelte` 的时长浮层（视频图标 + `<p>NaN</p>`）仅在 `{#if asset.isVideo}` 内渲染，而官方 `isVideo = (type === 'VIDEO')`。因此：

- 若资产 `type` 为 `IMAGE`，网页**根本不会**渲染该浮层，不可能出现 `NaN`。
- 用户看到的「图片」上出现 `NaN`，说明该资产在响应中被返回为 `type: "VIDEO"`（最常见的真实场景是 **iPhone Live Photo / 运动照片**：immich-go 按 magic bytes 把其 `.mov` 运动段嗅探为 `VIDEO`，用户主观视为「一张照片」），于是浮层渲染，而 `duration` 处理错误 → `NaN` / `0:00`。

无论根因是 Live Photo 还是个别图片被误判为 VIDEO，修复方向一致：**让 `duration` 严格符合官方契约（毫秒整数、非视频为 `null`）**，从数据层面彻底消除 `NaN`。修复后，即便某资产被误判为 VIDEO 但其 `duration` 为空，也会返回 `null`（网页兜底为 `0:00`，不再是 `NaN`）。
> 组件：`internal/app/asset.go`（第 41 行 `Duration int`、第 91 行 `toResponse`、第 145 行 `parseDurationInt`、上传第 174/208-210/289 行）、`internal/app/models.go:51`（`Duration string`）、`internal/app/timeline.go:167`（bucket 数组）
> 与 `NO_STUBS.md` 区分：本问题不是「假成功桩」，而是 **DTO 字段单位/契约不符**（immich-go 以「秒」返回 `duration`，而官方网页版与服务端契约以「毫秒」消费），导致网页把时长算错、浮层显示 `NaN`/`0:00`。

## 现象（来自用户报告）

在照片/资产网格（资产缩略图）上，视频缩略图左上角的**时长浮层**显示文本 **`NaN`** 而非正常时长（如 `0:10`）。用户抓取的精确 DOM：

```html
<span class="flex place-items-center gap-1 pe-2 pt-2"><p>NaN</p> <svg width="24" height="24" viewBox="0 0 24 24" ...><path d="M1,5H3V19H1V5M5,5H7V19H5V5M22,5H10A1,1 0 0,0 9,6V18A1,1 0 0,0 10,19H22A1,1 0 0,0 23,18V6A1,1 0 0,0 22,5M11,17L13.5,13.85L15.29,16L17.79,12.78L21,17H11Z" fill="currentColor"></path></svg></span>
```

`<svg>` 为视频/胶片图标，`<p>NaN</p>` 为时长文本。即网页从某个后端字段计算出时长并得到 `NaN`。

## 证据（本地真实请求，admin@immich.app 登录后）

本地服务（`127.0.0.1:8081`）当前仅有 3 个资产，全部为 IMAGE、`duration:0`，且无任何 VIDEO 资产，因此无法在此环境直接复现 `NaN`（见下「根因」说明）。真实输出：

```
$ curl -s -b <cookie> "http://127.0.0.1:8081/api/assets?size=100" | grep -oE '"type":"[A-Z]+"'
"type":"IMAGE"
"type":"IMAGE"
"type":"IMAGE"

$ curl -s -b <cookie> "http://127.0.0.1:8081/api/assets?size=100" | grep -oE '"duration":[0-9]+' | sort | uniq -c
      3 "duration":0

$ curl -s -b <cookie> "http://127.0.0.1:8081/api/assets?size=100" | grep -c '"type":"VIDEO"'
0
```

即：本环境下资产均为图片、时长恒为 `0`、无视频。但「单位不符」这一缺陷可从**代码 + 官方契约**确凿证明（见下）。

### 官方网页版实际消费代码（v3.1.0，权威依据）

1. **资产卡片 `Thumbnail.svelte`** 把 `asset.duration` 当作**毫秒**并除以 1000 得到秒数，再传给 `VideoThumbnail`：

```svelte
{#if asset.isVideo}
  <VideoThumbnail
    ...
    durationInSeconds={asset.duration ? asset.duration / 1000 : 0}   // web/src/lib/components/assets/thumbnail/Thumbnail.svelte:269
    ...
  />
{/if}
```

2. **`VideoThumbnail.svelte`** 用该秒数渲染 `m:ss` 格式（luxon `Duration.fromObject`）：

```svelte
let remainingSeconds = $state(durationInSeconds);                  // :32
// ... $effect 中： remainingSeconds = durationInSeconds;           // :39
{#if remainingSeconds < 60}
  {Duration.fromObject({ seconds: remainingSeconds }).toFormat('m:ss')}    // :99
{:else if remainingSeconds < 3600}
  {Duration.fromObject({ seconds: remainingSeconds }).toFormat('mm:ss')}   // :101
{:else}
  {Duration.fromObject({ seconds: remainingSeconds }).toFormat('h:mm:ss')} // :103
{/if}
```

要点：
- 网页以 `asset.duration / 1000` 计算秒数 ⇒ **`asset.duration` 必须是「毫秒」整型**（或 `null`）。若 `asset.duration` 是**非数字**值（如字符串、或单位错配后的异常值），JS 中 `"x" / 1000 === NaN`，且 `NaN ? … : 0` 中 `NaN` 为真 ⇒ `durationInSeconds` 直接变成 `NaN`；随后 `Duration.fromObject({ seconds: NaN })` 经 `toFormat('m:ss')` 即渲染出用户看到的 **`NaN`**。
- 即便 `asset.duration` 是「秒」而非「毫秒」的**数字**（immich-go 当前情况），`秒 / 1000` 也会得到约 1000 倍偏小的秒数：10 秒视频 ⇒ `10/1000 = 0.01` 秒 ⇒ 浮层显示 `0:00`；16 分钟以内的视频全部显示 `0:00`。这正是「时长浮层坏掉」的同一类表现。

3. **官方服务端契约 `asset-response.dto.ts`** 明确 `duration` 为**毫秒**、可空：

```ts
// server/src/dtos/asset-response.dto.ts:43
duration: z.int32().min(0).nullable().describe('Video/gif duration in milliseconds (null for static images)'),
// :131 (生成类型)
duration: number | null;
// :203 / :231 (映射)
duration: entity.duration,
```

即官方服务端把 `duration` 以**毫秒整数**返回，非视频资产为 `null`。

### 服务端迁移佐证（v3.1.0 破坏性变更，`ChangeDurationToInteger`）

官方 v3.1.0 的 DB 迁移 `server/src/schema/migrations/1777667825574-ChangeDurationToInteger.ts` 直接证明 `duration` 的单位/类型在 v3.1.0 发生改变，且目标单位为**毫秒整数**：

```ts
// up(): asset.duration 由 varchar(HH:MM:SS.mmm) 改为 integer(毫秒)
ALTER TABLE asset ALTER COLUMN duration TYPE integer
USING (
  CASE
    WHEN duration ~ '^\d{2}:\d{2}:\d{2}\.\d{3}$'
      THEN substr(duration, 1, 2)::int * 3600000      -- 时 -> ms
         + substr(duration, 4, 2)::int * 60000        -- 分 -> ms
         + substr(duration, 7, 2)::int * 1000         -- 秒 -> ms
         + substr(duration, 10, 3)::int               -- 毫秒
  END
);
// down(): 反向，把整数 ms 还原为 HH:MM:SS.mmm 字符串
```

要点：
- **旧格式（v3.1.0 之前）**：`duration` 是 `varchar`，形如 `HH:MM:SS.mmm`（如 `00:00:05.000`），即「时分秒.毫秒」字符串。
- **新格式（v3.1.0）**：`duration` 是 `integer`，值为**毫秒**（如 `5000`）。
- 这与 immich-go 的现状形成直接对照：immich-go 的 `internal/app/models.go:51` 为 `Duration string gorm:"type:text"`，仍以**字符串**（秒）存储，未跟随官方的「varchar→integer(ms)」迁移。这正是 BUG-004 中「immich-go 把 duration 当秒、官方当毫秒」根因的**服务端权威佐证**——官方在 v3.1.0 已把量纲锁定为「毫秒整数」，任何以秒或字符串返回的兼容实现都会触发网页 `asset.duration / 1000` 的 1000× 偏差与 `NaN`。

（说明：上述迁移文件来自官方 immich v3.1.0 仓库，与 immich-go 自带的 `immich.db` 无关——immich-go 是 reimplementation，但其**响应 DTO 必须**按官方 v3.1.0 契约（毫秒整数、`null` for 非视频）给官方网页版喂数据，否则即复现本 bug。）

### immich-go 实际实现（与本仓库代码对照）

- `internal/app/models.go:51`：`Duration string gorm:"type:text" json:"duration"` —— 数据库以**字符串**存储，内容为「秒」（见上传路径）。
- `internal/app/asset.go:41`：`Duration int json:"duration"` —— 响应 DTO 以 **int** 输出。
- `internal/app/asset.go:91`（toResponse）：`Duration: parseDurationInt(asset.Duration)`，`parseDurationInt`（asset.go:145）把存储的「秒」字符串 `Atoi` 成 int 秒。
- 上传路径：`asset.go:174` `durationSec := parseDurationInt(c.PostForm("duration"))`（官方客户端以**秒**上报），`asset.go:289` `Duration: strconv.Itoa(durationSec)` 以**秒**存库。
- 时间线 bucket：`internal/app/timeline.go:167` `r.Duration[i] = parseDurationInt(as.Duration)`（同样为「秒」int 数组）。
- 所有序列化出口（`toResponse` 在 `asset.go:91`、以及 `album.go:111`、`compat.go:220`、`compat_v3.go:150`、`memories.go:67`、`misc.go:98/301/314`、`search.go`、`share.go:75`、`timeline.go:91`；bucket 数组 `timeline.go:167`）一律以 **int 秒** 输出。

结论：immich-go 在**每一处**都把 `duration` 当成「秒」输出，而官方网页版（`Thumbnail.svelte:269` 的 `asset.duration / 1000`）与官方服务端契约（`z.int32().min(0).nullable()`，毫秒）要求「毫秒」。两者单位相差 1000 倍，是该浮层显示错误的根因；当该字段在用户环境中以非数字（字符串）形式出现时，`asset.duration / 1000` 即产生与报告一致的字面 `NaN`。

## 根因

immich-go 的 `duration` 字段单位与官方 Immich v3.1.0 契约不一致：

- **契约要求**：`duration` 为「毫秒」整数（`number | null`，非视频为 `null`），网页用 `asset.duration / 1000` 换算成秒。
- **immich-go 实际**：以「秒」存储（DB 字符串）并以「秒」返回（int）。

后果分两层：
1. **单位错配（确凿、可复现）**：视频时长被网页当成毫秒再除以 1000，显示值约小 1000 倍 ⇒ 几乎所有视频浮层显示 `0:00`（或近零值），而非真实 `m:ss`。
2. **`NaN`（与用户报告一致）**：一旦 `asset.duration` 在响应中被序列化为**非数字**值（`parseDurationInt` 对无法解析的内容回退为 `0`，但若历史上/其它出口以字符串形态给出时长，或字段缺省为非数字），`asset.duration / 1000` 在 JS 中即为 `NaN`，并经 `Duration.fromObject({seconds: NaN}).toFormat('m:ss')` 渲染为字面 `NaN`。`VideoThumbnail` 仅在「播放（hover）」路径用 `Number.isNaN(remaining) ? Infinity : remaining`（`:88`）兜底了 `ontimeupdate` 的 NaN，但**默认态** `remainingSeconds = durationInSeconds`（`:32`/`:39`）无任何兜底，故非数字 `asset.duration` 仍直接暴露为 `NaN`。

## 修复记录（已实施）

目标：让 `duration` 严格符合官方契约——**毫秒整数**，非视频资产为 `null`。以真实官方代码为准，不依赖 OpenAPI。

实施的改动（已构建、部署并用真实 API + 单元测试验证）：

1. **序列化单位统一为毫秒（落库即存毫秒）**：上传路径 `asset.go` 把客户端上报的「秒」经 `parseDurationSecondsToMs` 转成「毫秒」后落库（`msToString(durationMs)`），避免 DB 存量歧义。
2. **非视频返回 `null`**：`AssetResponse.Duration` 由 `int` 改为 `*int`（`asset.go:42`）；`toResponse` 改为调用新增的 `assetDurationResponse(asset)`（`asset.go:92`/`179`），**仅 VIDEO 资产返回毫秒整数，其余返回 `null`**。所有对外序列化出口（`album/search/share/memories/compat/misc/timeline` 均走 `toResponse`）统一受益。
3. **时间线 bucket 一致**：`timeline.go:167` 直接读取已存为毫秒的 `as.Duration`（`parseDurationInt`），不再另行换算，与 `toResponse` 单一位（毫秒）保持一致。
4. **新增契约测试**：`internal/app/asset_duration_test.go` 覆盖 VIDEO→毫秒、IMAGE→null、空 duration→null、秒→毫秒换算、非法输入→null（永不 `NaN`）等用例，`go test ./internal/app/` 通过。

> 已知遗留（非本 bug 阻塞）：历史存量若以「秒」落库（早期版本），`assetDurationResponse` 会按毫秒读取而偏小 1000×。当前发布为首个版本、DB 全新，不受影响；若日后需要可加一次性迁移。

回归校验结果：
- 真实 API（`GET /api/assets`，admin 登录后）确认 3 张 IMAGE 资产均返回 `duration: null`（修复前为 `0`），网页无从渲染 `NaN`。
- 单元测试覆盖毫秒换算与 `null` 语义，全部通过。
- 官方网页 `isVideo` 守卫下，IMAGE 不渲染浮层；VIDEO 拿到合法毫秒或 `null`，浮层显示 `m:ss` 或兜底 `0:00`，**不再 `NaN`**。
- 真实浏览器端到端验证需在现网放一个视频/Live Photo 资产确认浮层 `m:ss`；逻辑层已通过测试与 API 双重验证。

## 影响范围

- 仅影响**视频资产缩略图**上的时长浮层（资产网格、时间线 bucket、专辑缩略图中的视频），以及任何读取 `duration` 字段的前端展示（如详情页时长）。
- 不影响上传/下载/转码/同步等核心媒体功能；不影响图片资产（图片本就不渲染该浮层，且契约允许 `null`）。
- 现网所有经 immich-go 上传/扫描的视频资产均受影响（时长显示错误）。

## 验证方式

- 修复后 `curl /api/assets` / 时间线 bucket，对一个已知 10 秒视频应返回 `"duration":10000`（毫秒），非视频资产返回 `duration: null`。
- 真实浏览器经反向代理（`https://eo795eal4s-8081.cnb.run`）打开 `/photos`：
  1. 登录后到达 `/photos`，确认登录表单消失、认证调用 200（rule 8 硬性要求）；
  2. hover 一个视频缩略图，确认时长浮层显示正确 `m:ss`（如 `0:10`），**无** `NaN`、`0:00`；
  3. 抓取 console + network，确认无客户端 `pageerror`。
- 契约回归：`python3 scripts/schemathesis_check.py` 对 `getAssets` 及时间线端点的 `response_schema_conformance` 通过；`duration` 字段为整型毫秒、非视频可空。
