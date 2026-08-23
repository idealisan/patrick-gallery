# docs — 文档索引与开发流程

本目录存放 immich-go 的设计文档、差距分析、Epic 与运维手册。本文件同时是
**项目开发流程（Epic / Story 工作方式）的权威说明**。

## 开发流程：Epic / Story

本项目（自 2026-08 起）采用软件工程化的 **Epic → Story → Task** 方式开发和维护。
所有超出"单次小修复"的主题性工作都必须落为一个 Epic 文档，而不是散落在聊天记录
或口头约定里。

### 什么时候开 Epic

满足任一条件即应建 Epic：

- 需要跨越 **多个提交/多次会话** 才能完成的功能或重构
- 需要 **先调查、再讨论、后实现** 的主题（如维护体系、多用户共享）
- 有明确的 **验收标准** 且会被分批交付的能力

纯 bug 修复、单点契约对齐不需要 Epic，直接提交并在 commit message 说明即可。

### Epic 文档规范

位置：`docs/epics/<topic>.md`（kebab-case 命名，如 `maintenance-self-healing.md`）。

每个 Epic 必须包含以下章节：

1. **背景与目标** —— 为什么做，解决什么问题
2. **范围决策表** —— 明确记录 ✅做 / ❌不做 及理由；被放弃的想法也要留档，
   避免将来重复讨论
3. **现状盘点** —— 调查时的代码/端点事实（标注日期；会过时，仅作决策依据）
4. **任务分解 Work Items** —— 每个 WI 可独立交付、可验收；用 checklist 跟踪进度
5. **待讨论 / 开放问题** —— 未定项与倾向方案；讨论后把结论回写为"决策"
6. **变更记录** —— 按日期追加

### Story / Task 粒度

- 一个 WI = 一个可独立合并的 PR 规模；如果太大，拆成多个 Story
- 每个 Story 完成即 commit + push（遵守仓库的提交纪律：待提交文件不超过 5 个）
- 验收标准写在 WI 内，完成后勾选 checklist 并在变更记录注明

### 与既有约定的关系

- 契约类改动仍以 **官方 Immich 源码为准**（见 AGENTS.md Hard rules 7/8）；
  Epic 只组织工作，不改变契约权威来源
- 功能差距的权威跟踪仍在 `docs/GAP_ANALYSIS.md`；Epic 是执行视角，
  GAP 是覆盖视角

## 文档索引

| 文件 | 内容 |
|---|---|
| `GAP_ANALYSIS.md` | 权威功能差距跟踪（端点级 diff） |
| `NO_STUBS.md` | Stub 清单与处置（必须清零） |
| `CONTRACT_TESTING.md` | Schemathesis 契约回归测试说明 |
| `epics/` | 进行中的 Epic 文档 |
| `VIDEO_STREAMING_DESIGN.md` 等 | 专题设计文档 |

## 当前进行中的 Epic

- [maintenance-self-healing.md](epics/maintenance-self-healing.md) — 维护与自愈体系：
  force 全量重建、backupDatabase、启动 repairPass、缓存版本化失效
- [webui-maintenance-parity.md](epics/webui-maintenance-parity.md) — Web UI 维护页对齐：
  GET /jobs legacy 形状、9 个 integrity-* 手动任务、报告 UUID/溯源/下载、维护模式契约
