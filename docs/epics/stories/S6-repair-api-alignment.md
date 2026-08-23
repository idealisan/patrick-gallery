# S6: Repair 端点对齐官方 Admin UI

- **Epic**: [维护与自愈体系](../maintenance-self-healing.md) · WI-3
- **优先级**: P2 · **估算**: 1d · **状态**: 待开工（依赖 S5；**先读原版**）
- **依赖**: S5

## User Story

作为使用官方 Web UI 的管理员，我希望在 Admin → Repair 页看到并触发与官方
Immich 一致的修复能力——因为我们直接复用原版 Web UI，端点契约必须一致。

## 前置调查（本 Story 第一步，产出写回本文件）

- [ ] 读原版确认：Admin Repair 页调用的端点路径、方法、请求/响应 DTO
      （预期涉及 `POST /api/admin/...` 之类的 detect/queue 动作，以原版为准）
- [ ] 确认官方"修复"覆盖哪些检查项，映射到 S5 步骤清单的对应关系
- [ ] 差异决策：官方检查项我们做不到的（如文件系统级 missing-file 探测的
      细节差异）如何诚实降级

## 验收标准

1. 官方 Web UI 的 Admin Repair 页在本服务上可用：页面加载、探测、执行修复全流程
2. 端点响应 DTO 与官方逐字段一致（嵌套对象不得缺失）
3. 执行修复实际调用 S5 repairPass（或其子集），结果真实反映到数据库
4. 真实浏览器验证（Hard rule 8）：headless Chrome 走通 Repair 页全流程，
   无 client-side pageerror
5. schemathesis 允许清单同步更新

## Out of scope

- 官方没有的额外修复项（保留在启动 repairPass 中，不在该页暴露）
