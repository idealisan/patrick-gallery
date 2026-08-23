# S9: 维护类端点契约测试回归

- **Epic**: [维护与自愈体系](../maintenance-self-healing.md) · 全部 WI
- **优先级**: P1 · **估算**: 0.5d（随各 Story 滚动交付，收尾统一回归）
- **状态**: 待开工

## User Story

作为项目，我需要维护类新端点（force、backup、repair）全部纳入 Schemathesis
契约回归与 Go 单测覆盖——防止后续改动悄悄破坏形状或引入 fake-success。

## 验收标准

1. 每个 Story 交付时已自带：正常路径 + 边界路径的单测/集成测试（各 Story 内完成）
2. 本 Story 收尾统一回归：
   - `python3 scripts/schemathesis_check.py` 全绿（新增端点如不在官方 spec 中，
     记录到允许清单并说明理由——遵守 No-stub 政策，禁止用允许清单掩盖假成功）
   - `go test ./internal/app/ ./internal/image/` 全绿
3. 官方 spec 未覆盖的 force/backup 行为：以原版源码为准补 DTO 断言测试
4. 更新 `docs/GAP_ANALYSIS.md`：维护类端点从"缺失"移入"已完成"
5. Epic 文档变更记录注明回归结果

## Out of scope

- 性能/压力测试（分批策略的量化验证可在 S3 集成测试中附带）
