# S5: repairPass 一致性自愈核心

- **Epic**: [维护与自愈体系](../maintenance-self-healing.md) · WI-3
- **优先级**: P1 · **估算**: 1.5d · **状态**: 待开工
- **依赖**: 无（可先行；S6 的 API 层建立在本 Story 的步骤清单之上）

## User Story

作为系统本身，我希望每次启动时按**有序、幂等、可跳过**的步骤清单自动修复
已知的数据不一致——把一次性专项回填（livephoto hidden）泛化为可持续演进的
repairPass 框架。

## 修复步骤清单（首批，按序执行）

| # | 步骤 | 动作 |
|---|---|---|
| 1 | livePhotoVideoId 悬空引用 | 清空引用（官方语义：旧 motion asset 已删） |
| 2 | 被引用 motion 视频 visibility ≠ hidden | 置 hidden + 移出相册（已有实现，迁入框架） |
| 3 | visibility='archive' 与 is_archived 不同步 | 以 visibility 为准同步布尔 |
| 4 | has_thumbnail 与 resize_path 不一致 | 以文件存在性为准双向修正 |
| 5 | 孤儿 preview 缓存 `.preview.jpg` | 删除文件 |
| 6 | 孤儿 thumbnail 文件 | 删除文件 |
| 7 | albums_assets_assets 悬空行 | 删行 |

## 验收标准

1. 框架：`[]repairStep{Name, Fn}` 有序执行；每步独立 try，单步 panic 不影响后续
2. 每步输出 `[repair] step=<name> scanned=N fixed=M` 日志
3. 全部步骤幂等：连跑两次第二次 fixed=0（测试断言）
4. 现有 backfillLivePhotoHidden 迁入框架并删除旧调用点
5. 集成测试：构造七类脏数据 → 跑 repairPass → 全部修复且正常数据未被误伤
6. 步骤清单以注册表形式暴露，便于 S6 做 dry-run 报告

## Out of scope

- HTTP API / dry-run 报告端点 → S6
