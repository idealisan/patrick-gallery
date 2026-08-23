# S7: 缓存版本元数据落库

- **Epic**: [维护与自愈体系](../maintenance-self-healing.md) · WI-4
- **优先级**: P2 · **估算**: 0.5d · **状态**: 待开工
- **依赖**: 无

## User Story

作为系统，我需要为每份缩略图/预览缓存记录「生成时的管线版本」，使未来程序
升级后能精确判断哪些缓存需要重建——而不是靠文件名猜测或全量作废。

## 决策依据（§5.3 / §5.4）

- **版本信息存数据库**，不编码进文件名（文件名易受外部干扰、有 FS 兼容性风险）
- **按格式族分粒度**：如 `heic/1`、`jpeg/1`、`video/1`；HEIC 解码器升级不应
  使 JPEG 资产的缓存失效
- **是否 bump 由发版时人工评估**：新版解码器向后兼容 → 不 bump、无需重建；
  有 breaking change（开发时已知）→ bump 该格式族版本

## 验收标准

1. schema：assets 表新增列（或旁表）记录 thumbnail/preview 的
   `{formatFamily, generatorVersion}`；AutoMigrate 兼容旧库（旧行视为 v0/未知）
2. 代码定义当前各格式族版本常量（单一出处，如 `internal/image/pipeline_version.go`）
3. ingestStoredFile / thumbnailGeneration / servePreview 写缓存时同时落版本
4. 读取路径暴露版本（内部方法即可），供 S8 判断失效
5. 单元测试：写入后能读回版本；旧库迁移后版本为空不报错

## Out of scope

- 失效判定与重建触发 → S8
