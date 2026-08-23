# S4: backupDatabase 实现

- **Epic**: [维护与自愈体系](../maintenance-self-healing.md) · WI-2
- **优先级**: P1 · **估算**: 1d · **状态**: 待开工
- **依赖**: 无

## User Story

作为管理员，我希望一键把 SQLite 数据库备份到磁盘上的受管目录，并自动滚动
保留最近 N 份——在误操作或升级失败时能快速回滚。

## 现状

`backupDatabase` 仅存在于 queueNames 列表，jobRegistry 无实现（触发返回
unsupported）。

## 验收标准

1. jobRegistry 注册 `backupDatabase`（supported: true）：
   - 备份文件：`resources/backups/immich-<RFC3339ts>.db`
   - 实现方式优先 `VACUUM INTO`（glebarez/sqlite 支持则用；否则退 backup API）
2. 备份前执行 `PRAGMA wal_checkpoint(TRUNCATE)` 保证一致性
3. 滚动保留：默认保留最近 3 份，超出删除最旧（数量可经环境变量配置）
4. 触发方式**跟随官方原版行为**（§5.1 决策）：实现前读原版确认 admin UI 是纯
   手动按钮还是支持定时；按官方契约对齐端点与 DTO
5. 验证：备份完成后 `sqlite3 <file> 'PRAGMA integrity_check;'` 返回 ok；
   文件大小 > 0；进度快照可见完成态
6. 失败路径：磁盘满/目录不可写 → job failed + 日志明确原因

## Out of scope

- 自动恢复/还原端点（官方亦无；手工 `cp` 回滚即可）
- 远端存储（S3/OSS）
