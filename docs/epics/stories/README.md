# Epic: 维护与自愈体系 — Story 拆分

> 父文档: [../maintenance-self-healing.md](../maintenance-self-healing.md)
> 拆分日期: 2026-08-23
> 原则: 每个 Story 独立开发、独立验证、独立合并；Story 间依赖用「依赖」标注

## Story 总览与依赖关系

```
S1 force 参数 ──┐
                ├──> S3 分批执行链
S2 job 批次框架 ┘
S4 backupDatabase（独立）
S5 repairPass 核心（独立，可先行）
S6 Repair API 对齐官方 ──> 依赖 S5 的步骤清单
S7 缓存版本 schema（独立）
S8 版本失效重建 ──> 依赖 S7 + S1(force)
S9 契约测试补全 ──> 随各 Story 同步交付，收尾统一回归
```

| Story | 标题 | WI | 优先级 | 估算 | 状态 |
|---|---|---|---|---|---|
| [S1](stories/S1-force-param.md) | jobs API 支持 `force:true` | WI-1 | P0 | 0.5d | ✅ 完成 |
| [S2](stories/S2-job-batch-framework.md) | job 批次框架（chunked items） | WI-1 | P0 | 1d | ✅ 完成 |
| [S3](stories/S3-chained-batch-execution.md) | 全量重建的链式分批执行 | WI-1/§5.5 | P0 | 1d | ✅ 完成 |
| [S4](stories/S4-backup-database.md) | backupDatabase 实现 | WI-2 | P1 | 1d | ✅ 完成 |
| [S5](stories/S5-repairpass-core.md) | repairPass 一致性自愈核心 | WI-3 | P1 | 1.5d | ✅ 完成 |
| [S6](stories/S6-repair-api-alignment.md) | Repair 端点对齐官方 Admin UI | WI-3 | P2 | 1d | ✅ 完成 |
| [S7](stories/S7-cache-version-schema.md) | 缓存版本元数据落库 | WI-4 | P2 | 0.5d | ✅ 完成 |
| [S8](stories/S8-cache-invalidation.md) | 版本驱动的缓存失效重建 | WI-4 | P2 | 1d | ✅ 完成 |
| [S9](stories/S9-contract-regression.md) | 维护类端点契约测试回归 | 全部 | P1 | 0.5d | ✅ PASS |

建议实施顺序：**S1 → S2 → S3**（一条完整可验收的用户价值线），随后 S4 或 S5 任选。
