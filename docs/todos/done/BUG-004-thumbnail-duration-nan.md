# BUG-004：缩略图标题上方显示字面 `NaN`（时间线 bucket 的 stack 槽位为空数组）

> 状态：**Resolved（已修复、构建、部署并用真实 API + 单测验证）** · 日期：2026-08-16 · 严重度：Medium（照片网格每张缩略图左上角显示字面 `NaN`，主看图页观感受损，非上传/同步功能阻断）
>
> 重要更正：本 bug 的**真正根因是时间线 bucket 的 `stack` 字段**，不是 `duration`。早期一份报告把用户看到的 `NaN` 归因为视频时长浮层（`duration`），但用户明确说明复现所用是**图片（Web UI 上传）**——而官方网页 `Thumbnail.svelte` 的时长浮层仅在 `asset.isVideo` 内渲染，图片根本不会渲染该浮层。下文为经官方网页源码核对后的真实根因。

## 现象（来自用户报告）

在 Web UI 登录、上传一张图片、刷新界面后，**每一张图片缩略图的标题上方**都显示字面文本 `NaN`。用户抓取的精确 DOM 为该 `NaN` 文本节点。

## 权威依据（官方 Immich v3.1.0 网页源码，本地 `/root/immich-src`）

### 渲染 `NaN` 的组件

`web/src/lib/components/assets/thumbnail/Thumbnail.svelte:373-385`（堆叠资产计数徽标）：

```svelte
{#if asset.stack && showStackedIcon}
  <div class="... top-0 ...">
    <span class="... gap-1 ...">
      <p>{asset.stack.assetCount.toLocaleString($locale)}</p>   <!-- 第 381 行 -->
      <Icon icon={mdiCameraBurst} .../>
    </span>
  </div>
{/if}
```

关键点：`asset.stack.assetCount.toLocaleString($locale)` —— 若 `asset.stack.assetCount` 为 `NaN`，则 `NaN.toLocaleString()` 返回字符串 `"NaN"`，正是用户看到的字面文本。该徽标位于缩略图左上角（标题/图标上方），与用户描述一致。

### `asset.stack` 如何由时间线 bucket 重建

`web/src/lib/managers/timeline-manager/timeline-month.svelte.ts:200-205`：

```ts
stack: bucketAssets.stack?.at(i)
  ? {
      id: bucketAssets.stack[i]![0],
      primaryAssetId: bucketAssets.id[i],
      assetCount: Number.parseInt(bucketAssets.stack[i]![1]),
    }
  : null,
```

- 对第 `i` 个资产，取 bucket 平行数组 `stack` 的第 `i` 个槽位。
- 若该槽位为**真值**（非 `null`、非 `undefined`、非空数组 `[]` 之外的真值）→ 进入真值分支，读 `stack[i][1]` 作 `assetCount`。
- 若槽位为 `[]`（空数组）：在 JS 中 `[]` 是**真值**，于是进入真值分支；`stack[i][0]` 与 `stack[i][1]` 均为 `undefined`，`Number.parseInt(undefined)` → `NaN` → 徽标渲染字面 `NaN`。
- 若槽位为 `null`：`null` 是**假值** → 走 `: null` 分支 → `asset.stack = null` → `Thumbnail.svelte:373` 的 `{#if asset.stack ...}` 为假 → 不渲染徽标。安全。

官方契约佐证：`web/src/test-data/factories/asset-factory.ts:92` 中，非堆叠资产的 `stack` 测试工厂值为 `null`；堆叠资产为 2 元组 `[stackId, assetCount]`。即「非堆叠资产 → `null`」才是官方契约。

## immich-go 实际实现（本仓库，修复前）

`internal/app/timeline.go` 的 `buildTimeBucketAssets`：

```go
Stack: make([][]string, n),   // 每个元素默认 nil
...
r.Stack[i] = []string{}       // 修复前：每个资产都塞一个空数组！
```

`[]string{}` 经 `json.Marshal` 序列化为 `[]`（空数组），于是整个 bucket 的 `stack` 字段变成 `[[]]`（或更一般地 `[[] 、[] 、[]]`）——每个槽位都是空数组。官方网页据此得到 `NaN`。

用户抓包的 HAR 复现中，2026-08 bucket（3 个资产）的 `stack` 确实为 `[[]]`（经 `curl /api/timeline/bucket` 在旧二进制上复现确认）。

> 注：早期把症状误判为 `duration` 浮层是错误的——图片不渲染该浮层。不过 `duration` 的契约修正（毫秒整数、非视频为 `null`，commit `5cdd99d`）仍是合法且有价值的契约改进，与本次 `stack` 修复互不冲突、均已落地。

## 根因

时间线 bucket 响应里，每个资产的 `stack` 槽位被 immich-go 序列化成了空数组 `[]`（JS 真值），而官方网页时间线加载器对真值槽位执行 `Number.parseInt(stack[i][1])`，空数组的 `[1]` 为 `undefined` → `NaN` → 缩略图堆叠徽标渲染字面 `NaN`。immich-go 当前不支持堆叠，正确契约应为每个槽位返回 `null`。

## 修复记录（已实施）

`internal/app/timeline.go` `buildTimeBucketAssets`：

- 移除 `r.Stack[i] = []string{}`，保留 `make([][]string, n)` 的默认 `nil` 元素，使每个非堆叠槽位序列化为 `null`，与官方契约一致（并补注释说明「`[]` 在 JS 为真值会触发 `NaN`」）。
- 新增回归测试 `internal/app/timeline_bucket_stack_test.go`：断言非空 bucket 每个槽位序列化为 `null`（`"stack":[null,null]`），绝不出现内层空数组（`[[]]`），且空 bucket 仍发出 `"stack":[]` 满足 schema 必填数组契约。

`go test ./internal/app/` 通过（含 `TestBuildTimeBucketAssetsStackIsNullNotEmptyArray`、`TestBuildTimeBucketAssetsEmptyBucket`、`TestAssetDurationResponseContract`、`TestParseDurationSecondsToMs`）。

## 验证（真实 API + 单测）

- 单测证明序列化形状：`[null, null, ...]`，无 `[[]]`。
- 部署新二进制后 `curl /api/timeline/bucket?timeBucket=2026-08-01T00:00:00.000Z` 实测返回 `"stack":[null,null,null]`（旧二进制为 `"stack":[[]]`）。
- 官方网页时间线加载器对 `null` 槽位走 `: null` 分支 → 不渲染堆叠徽标 → 不再出现字面 `NaN`。
- 用户可在 Web UI 登录、上传图片、刷新后确认缩略图左上角不再有 `NaN`。
- 契约回归：`scripts/schemathesis_check.py` 对时间线端点的 `response_schema_conformance` 通过（`stack` 为 `null` 或 `StackEntityResponseDto[][]` 兼容形状）。
