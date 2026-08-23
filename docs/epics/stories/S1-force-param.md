# S1: jobs API 支持 `force:true`

- **Epic**: [维护与自愈体系](../maintenance-self-healing.md) · WI-1
- **优先级**: P0 · **估算**: 0.5d · **状态**: 待开工
- **依赖**: 无

## User Story

作为管理员，我希望通过 API 触发**全量**重建缩略图/元数据/转码，而不只是补齐
缺失项——这样解码器升级或修复逻辑变更后，我能让所有资产按新逻辑重新处理。

## 现状

`PUT /api/jobs/:name` 与 `POST /api/jobs` 接收 `{name}` 但忽略 `force` 字段；
jobRegistry 的 items() 只捞缺失项（如 `has_thumbnail=false`）。

## 验收标准

1. 读官方原版确认契约：`server/src/dtos/job.dto.ts` 的 JobCreateDto 字段名与类型
   （预期为 `force?: boolean`），以及 `PUT /api/jobs/:id` 的 body 形状
2. `{force:true}` 时：thumbnailGeneration / metadataExtraction / videoConversion /
   ocr 四个 job 的 items() 返回**全部**资产（含已有缩略图的）
3. 缺省（无 force）行为不变：仍只捞缺失项 —— 回归验证
4. force 任务在进度快照中可见（/api/jobs 的 active/completed 计数正常滚动）
5. 契约测试覆盖两条路径

## 实现要点

- jobSpec.items 增加 `force bool` 参数；dispatcher 从 body 透传
- 不改存储 schema

## Out of scope

- 分批执行 → S2/S3（本 Story 允许一次性全量跑通，S3 落地后替换执行方式）
