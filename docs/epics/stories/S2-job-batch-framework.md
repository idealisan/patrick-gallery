# S2: job 批次框架（chunked items）

- **Epic**: [维护与自愈体系](../maintenance-self-healing.md) · WI-1
- **优先级**: P0 · **估算**: 1d · **状态**: 待开工
- **依赖**: 无（与 S1 可并行）

## User Story

作为开发者，我需要一个通用的"批次"抽象，让任何全量 job 都能以固定大小的
chunk 推进——这是 S3 链式全量重建的地基，也保护 SQLite 单写者不被写放大压垮。

## 现状

jobRegistry 的 items() 一次性返回全部待处理项；run 循环在一个任务里跑完所有
item，无批次边界、无让出、无断点。

## 验收标准

1. 新增批次原语：`batchSize`（默认 50，可配）与游标（offset 或 id 游标）
2. dispatcher 支持以「批」为粒度推进：每批结束更新进度快照
   （completed += 本批数），并释放写锁间隙
3. 批次内单 item 失败不影响后续 item（沿用现有 per-item error 收集）
4. 现有五个 supported job 全部改造走批次框架，行为回归一致（非 force 路径）
5. 单元测试：批次切分边界（0 条 / <batchSize / 整除 / 余数）

## 实现要点

- 游标建议用 `id > lastID ORDER BY id LIMIT n`（避免大 OFFSET 成本）
- 进度快照结构增加 `batchesDone/batchesTotal`（可选展示字段）

## Out of scope

- 批次间的链式自动续跑 → S3
