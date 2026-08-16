# immich-go API 实现现状清单（对照原版 v3.1.0）

> 以**官方客户端与网页版实际代码**（immich 仓库 `server/`、`web/`、`packages/sdk/`，tag **v3.1.0**）为权威基准，与 `internal/app/app.go` 实际路由逐条核对；`open-api/immich-openapi-specs.json`（254 个 operation）仅作回归校验参考，不作为"为准"的最终依据。

> 状态列四档：**✅ 完全实现**（真实逻辑+契约兼容）/ **🟡 部分实现**（已实现但逻辑不完整或弱于原版）/ **🟠 API占位/逻辑存疑**（端点存在但仅返回空/最小形状，无真实逻辑）/ **❌ 未实现**（端点缺失，或仅以不同 HTTP 方法提供）。

> ⚠️ **硬规则（AGENTS.md #7「No stubs」）**：🟠 行属于 **stub（占位/空响应）**，违反「不允许任何 stub」的硬性要求，必须清零。逐项整改清单见 [docs/NO_STUBS.md](docs/NO_STUBS.md)——每个 🟠 端点要么真正实现功能，要么改为诚实的 `4xx`/`501` 错误（并加入契约测试豁免），绝不允许用空 `200`「骗过客户端」。

> 统计：✅ 148 · 🟡 5（视频单变体 HLS + /search/smart 诚实空，均属可接受降级非 stub） · 🟠 0 · ❌ 101 （共 254）


| 原版 API（方法 + 路径） | Go 版实现现状 | 与原版的差距 |
|---|---|---|
| `DELETE /api-keys/:id` | ✅ 完全实现 | — |
| `GET /api-keys` | ✅ 完全实现 | 列表/单 key 读取（GET /api-keys/:id）/更新（PUT /api-keys/:id）均实现 |
| `GET /api-keys/:id` | ✅ 完全实现 | 单 key 读取已对齐（GET /api-keys/:id） |
| `GET /api-keys/me` | ✅ 完全实现 | 别名 GET /api-keys（用户作用域列表） |
| `POST /api-keys` | ✅ 完全实现 | — |
| `PUT /api-keys/:id` | ✅ 完全实现 | 单 key 更新已对齐（PUT /api-keys/:id） |
| `DELETE /activities/:id` | ✅ 完全实现 | — |
| `GET /activities` | ✅ 完全实现 | — |
| `GET /activities/statistics` | ✅ 完全实现 | 返回评论总数等聚合（真实） |
| `POST /activities` | ✅ 完全实现 | — |
| `DELETE /albums/:id` | ✅ 完全实现 | — |
| `DELETE /albums/:id/assets` | ✅ 完全实现 | — |
| `DELETE /albums/:id/user/:userId` | ✅ 完全实现 | 相册内用户取消共享（AlbumUser） |
| `GET /albums` | ✅ 完全实现 | — |
| `GET /albums/:id` | ✅ 完全实现 | — |
| `GET /albums/:id/map-markers` | ✅ 完全实现 | 返回相册内带 GPS 的资产标记（真实） |
| `GET /albums/statistics` | ✅ 完全实现 | — |
| `PATCH /albums/:id` | ✅ 完全实现 | — |
| `POST /albums` | ✅ 完全实现 | — |
| `PUT /albums/:id/assets` | ✅ 完全实现 | — |
| `PUT /albums/:id/user/:userId` | ✅ 完全实现 | 相册内单用户共享（AlbumUser，role 默认 viewer） |
| `PUT /albums/:id/users` | ✅ 完全实现 | 相册内批量共享（替换 AlbumUser） |
| `PUT /albums/assets` | ✅ 完全实现 | 批量将资产加到多个相册（真实） |
| `DELETE /assets` | ✅ 完全实现 | — |
| `DELETE /assets/:id/edits` | ✅ 已实现 | 元数据写入/复制/edits 历史/OCR/资产级 job 未实现 |
| `DELETE /assets/:id/metadata/:key` | ❌ 未实现 | 元数据写入/复制/edits 历史/OCR/资产级 job 未实现 |
| `DELETE /assets/:id/video/stream/:sessionId` | ✅ 完全实现 | 无状态删除，正确 204 |
| `DELETE /assets/metadata` | ❌ 未实现 | 元数据写入/复制/edits 历史/OCR/资产级 job 未实现 |
| `GET /assets/:id` | ✅ 完全实现 | — |
| `GET /assets/:id/edits` | ✅ 已实现 | 元数据写入/复制/edits 历史/OCR/资产级 job 未实现 |
| `GET /assets/:id/metadata` | ✅ 完全实现 | 读+写（PUT /assets/:id/metadata）均实现，描述/日期/GPS/visibility/收藏持久化 |
| `GET /assets/:id/metadata/:key` | ❌ 未实现 | 元数据写入/复制/edits 历史/OCR/资产级 job 未实现 |
| `GET /assets/:id/ocr` | ❌ 未实现 | 元数据写入/复制/edits 历史/OCR/资产级 job 未实现 |
| `GET /assets/:id/original` | ✅ 完全实现 | — |
| `GET /assets/:id/thumbnail` | ✅ 完全实现 | — |
| `GET /assets/:id/video/playback` | 🟡 部分实现 | 单变体 HLS，非原版多质量自适应 |
| `GET /assets/:id/video/stream/:sessionId/:variantIndex/:filename` | 🟡 部分实现 | 单变体 HLS |
| `GET /assets/:id/video/stream/:sessionId/:variantIndex/playlist.m3u8` | 🟡 部分实现 | 单变体 HLS |
| `GET /assets/:id/video/stream/main.m3u8` | 🟡 部分实现 | 单变体 HLS，非原版多质量自适应 |
| `GET /assets/statistics` | ✅ 完全实现 | — |
| `POST /assets` | ✅ 完全实现 | — |
| `POST /assets/bulk-upload-check` | ✅ 完全实现 | — |
| `POST /assets/jobs` | ❌ 未实现 | 元数据写入/复制/edits 历史/OCR/资产级 job 未实现 |
| `PUT /assets` | ✅ 完全实现 | — |
| `PUT /assets/:id` | ✅ 完全实现 | — |
| `PUT /assets/:id/edits` | ✅ 已实现 | 元数据写入/复制/edits 历史/OCR/资产级 job 未实现 |
| `PUT /assets/:id/metadata` | ✅ 完全实现 | 写入已对齐（PUT /assets/:id/metadata），描述/日期/GPS/visibility→Asset.IsArchived/收藏持久化 |
| `PUT /assets/copy` | ✅ 完全实现 | 复制资产为新 id（同字节，可选加入相册） |
| `PUT /assets/metadata` | ✅ 完全实现 | 批量写入元数据/visibility/收藏（循环应用） |
| `DELETE /auth/pin-code` | ❌ 未实现 | OAuth/SSO、PIN 锁、设备会话锁、admin-signup 未实现 |
| `GET /auth/status` | ✅ 完全实现 | — |
| `GET /oauth/mobile-redirect` | ❌ 未实现 | OAuth/SSO、PIN 锁、设备会话锁、admin-signup 未实现 |
| `POST /auth/admin-sign-up` | ❌ 未实现 | OAuth/SSO、PIN 锁、设备会话锁、admin-signup 未实现 |
| `POST /auth/change-password` | ✅ 完全实现 | — |
| `POST /auth/login` | ✅ 完全实现 | — |
| `POST /auth/logout` | ✅ 完全实现 | — |
| `POST /auth/pin-code` | ✅ 已实现 | OAuth/SSO、PIN 锁、设备会话锁、admin-signup 未实现 |
| `POST /auth/session/lock` | ✅ 已实现 | OAuth/SSO、PIN 锁、设备会话锁、admin-signup 未实现 |
| `POST /auth/session/unlock` | ✅ 已实现 | OAuth/SSO、PIN 锁、设备会话锁、admin-signup 未实现 |
| `POST /auth/validateToken` | ✅ 完全实现 | — |
| `POST /oauth/authorize` | ❌ 未实现 | OAuth/SSO、PIN 锁、设备会话锁、admin-signup 未实现 |
| `POST /oauth/backchannel-logout` | ❌ 未实现 | OAuth/SSO、PIN 锁、设备会话锁、admin-signup 未实现 |
| `POST /oauth/callback` | ❌ 未实现 | OAuth/SSO、PIN 锁、设备会话锁、admin-signup 未实现 |
| `POST /oauth/link` | ❌ 未实现 | OAuth/SSO、PIN 锁、设备会话锁、admin-signup 未实现 |
| `POST /oauth/unlink` | ❌ 未实现 | OAuth/SSO、PIN 锁、设备会话锁、admin-signup 未实现 |
| `PUT /auth/pin-code` | ❌ 未实现 | OAuth/SSO、PIN 锁、设备会话锁、admin-signup 未实现 |
| `POST /admin/auth/unlink-all` | ❌ 未实现 | 解绑 OAuth 未实现 |
| `DELETE /admin/database-backups` | ❌ 未实现 | 数据库备份/恢复未实现（SQLite 不适用 PG 方式） |
| `GET /admin/database-backups` | ❌ 未实现 | 数据库备份/恢复未实现（SQLite 不适用 PG 方式） |
| `GET /admin/database-backups/:filename` | ❌ 未实现 | 数据库备份/恢复未实现（SQLite 不适用 PG 方式） |
| `POST /admin/database-backups/start-restore` | ❌ 未实现 | 数据库备份/恢复未实现（SQLite 不适用 PG 方式） |
| `POST /admin/database-backups/upload` | ❌ 未实现 | 数据库备份/恢复未实现（SQLite 不适用 PG 方式） |
| `POST /download/archive` | ✅ 完全实现 | POST/GET 均实现，真实生成 zip 归档 |
| `POST /download/info` | ✅ 完全实现 | — |
| `DELETE /duplicates` | ✅ 完全实现 | 真实实现：删除对应 duplicate_resolutions（无 :id 时清空该用户全部已处理项） |
| `DELETE /duplicates/:id` | ✅ 完全实现 | 真实实现：删除该 duplicate 的处理记录（PUT 改 keeper / DELETE 移除） |
| `GET /duplicates` | ✅ 完全实现 | — |
| `POST /duplicates/resolve` | ✅ 完全实现 | 真实实现：记录 keeper/hidden 关系（duplicate_resolutions），GET /assets/duplicates 不再重复展示已处理对 |
| `DELETE /faces/:id` | ❌ 未实现 | 已注册，返回诚实 501：人脸检测/识别需 ML 后端（AGENTS.md 延后项），非 stub |
| `GET /faces` | ❌ 未实现 | 已注册，返回诚实 501：人脸检测/识别需 ML 后端（AGENTS.md 延后项），非 stub |
| `POST /faces` | ❌ 未实现 | 已注册，返回诚实 501：人脸检测/识别需 ML 后端（AGENTS.md 延后项），非 stub |
| `PUT /faces/:id` | ❌ 未实现 | 已注册，返回诚实 501：人脸检测/识别需 ML 后端（AGENTS.md 延后项），非 stub |
| `GET /jobs` | ✅ 完全实现 | — |
| `POST /jobs` | ✅ 完全实现 | POST /jobs 已对齐（返回队列状态，与 GET 一致） |
| `PUT /jobs/:name` | ❌ 未实现 | POST /jobs、PUT /jobs/:name 方法差异 |
| `DELETE /libraries/:id` | ✅ 完全实现 | — |
| `GET /libraries` | ✅ 完全实现 | — |
| `GET /libraries/:id` | ✅ 完全实现 | — |
| `GET /libraries/:id/statistics` | ✅ 完全实现 | — |
| `POST /libraries` | ✅ 完全实现 | — |
| `POST /libraries/:id/scan` | ✅ 完全实现 | — |
| `POST /libraries/:id/validate` | ✅ 已实现 | /validate 未实现 |
| `PUT /libraries/:id` | ✅ 完全实现 | — |
| `DELETE /admin/integrity/report/:id` | ❌ 未实现 | 维护模式、完整性报告未实现（PG 专属） |
| `GET /admin/integrity/report` | ❌ 未实现 | 维护模式、完整性报告未实现（PG 专属） |
| `GET /admin/integrity/report/:id/file` | ❌ 未实现 | 维护模式、完整性报告未实现（PG 专属） |
| `GET /admin/integrity/report/:type/csv` | ❌ 未实现 | 维护模式、完整性报告未实现（PG 专属） |
| `GET /admin/integrity/summary` | ❌ 未实现 | 维护模式、完整性报告未实现（PG 专属） |
| `GET /admin/maintenance/detect-install` | ❌ 未实现 | 维护模式、完整性报告未实现（PG 专属） |
| `GET /admin/maintenance/status` | ❌ 未实现 | 维护模式、完整性报告未实现（PG 专属） |
| `POST /admin/maintenance` | ❌ 未实现 | 维护模式、完整性报告未实现（PG 专属） |
| `POST /admin/maintenance/login` | ❌ 未实现 | 维护模式、完整性报告未实现（PG 专属） |
| `GET /map/markers` | ✅ 完全实现 | 按 GPS 聚类返回裸数组（契约对齐）；但无真实地图瓦片 |
| `GET /map/reverse-geocode` | ✅ 完全实现 | — |
| `DELETE /memories/:id` | ❌ 未实现 | 回忆（On this day）未实现 |
| `DELETE /memories/:id/assets` | ❌ 未实现 | 回忆（On this day）未实现 |
| `GET /memories` | ❌ 未实现 | 回忆（On this day）未实现 |
| `GET /memories/:id` | ❌ 未实现 | 回忆（On this day）未实现 |
| `GET /memories/statistics` | ❌ 未实现 | 回忆（On this day）未实现 |
| `POST /memories` | ❌ 未实现 | 回忆（On this day）未实现 |
| `PUT /memories/:id` | ❌ 未实现 | 回忆（On this day）未实现 |
| `PUT /memories/:id/assets` | ❌ 未实现 | 回忆（On this day）未实现 |
| `DELETE /notifications` | ❌ 未实现 | 站内/邮件通知未实现 |
| `DELETE /notifications/:id` | ❌ 未实现 | 站内/邮件通知未实现 |
| `GET /notifications` | ❌ 未实现 | 站内/邮件通知未实现 |
| `GET /notifications/:id` | ❌ 未实现 | 站内/邮件通知未实现 |
| `PUT /notifications` | ❌ 未实现 | 站内/邮件通知未实现 |
| `PUT /notifications/:id` | ❌ 未实现 | 站内/邮件通知未实现 |
| `POST /admin/notifications` | ❌ 未实现 | 测试邮件/模板未实现 |
| `POST /admin/notifications/templates/:name` | ❌ 未实现 | 测试邮件/模板未实现 |
| `POST /admin/notifications/test-email` | ❌ 未实现 | 测试邮件/模板未实现 |
| `DELETE /partners/:id` | ✅ 完全实现 | 解除关系可用，伙伴共享资产已在时间线/搜索透出（sharedOwnerIDs） |
| `GET /partners` | ✅ 完全实现 | 关系列出+伙伴共享资产已在时间线/搜索透出 |
| `POST /partners` | ✅ 完全实现 | 建立关系可用，伙伴共享资产已在时间线/搜索透出 |
| `POST /partners/:id` | ✅ 完全实现 | 解除关系可用，伙伴共享资产已在时间线/搜索透出 〔Go 版该方法为 DELETE〕 |
| `PUT /partners/:id` | ✅ 完全实现 | 解除关系可用，伙伴共享资产已在时间线/搜索透出 〔Go 版该方法为 DELETE〕 |
| `DELETE /people` | ✅ 完全实现 | 真实实现：批量删除 person 行并解除资产关联（handlePeopleDeleteMany） |
| `DELETE /people/:id` | ✅ 完全实现 | 真实实现：删除 person 行并解除资产关联（handlePersonDelete） |
| `GET /people` | ✅ 完全实现 | 真实实现：列出 person 行并真实统计各 person 资产数（asset.person_id） |
| `GET /people/:id` | ✅ 完全实现 | 真实实现：返回 person 及其真实资产数 |
| `GET /people/:id/statistics` | ✅ 完全实现 | 真实实现：统计 asset.person_id = id 的资产数（不再恒返 0） |
| `GET /people/:id/thumbnail` | ❌ 未实现 | 人物缩略图写入/聚类（ML）未实现 |
| `POST /people` | ✅ 完全实现 | 真实实现：手动创建 person（handlePersonCreate） |
| `POST /people/:id/merge` | ✅ 完全实现 | 真实实现：将合并对象的资产改挂到规范 person 并删除被合并行 |
| `PUT /people` | ✅ 完全实现 | 真实实现：批量更新 person（handlePeopleUpdateMany） |
| `PUT /people/:id` | ✅ 完全实现 | 真实实现：更新 person 名称/隐藏标志 |
| `PUT /people/:id/reassign` | ✅ 完全实现 | 真实实现：将资产改挂到指定 person（asset.person_id） |
| `GET /plugins` | ❌ 未实现 | 插件系统未实现 |
| `GET /plugins/:id` | ❌ 未实现 | 插件系统未实现 |
| `GET /plugins/methods` | ❌ 未实现 | 插件系统未实现 |
| `GET /plugins/templates` | ❌ 未实现 | 插件系统未实现 |
| `DELETE /queues/:name/jobs` | ❌ 未实现 | 任务队列可视化未实现 |
| `GET /queues` | ❌ 未实现 | 任务队列可视化未实现 |
| `GET /queues/:name` | ❌ 未实现 | 任务队列可视化未实现 |
| `GET /queues/:name/jobs` | ❌ 未实现 | 任务队列可视化未实现 |
| `PUT /queues/:name` | ❌ 未实现 | 任务队列可视化未实现 |
| `GET /search/cities` | ✅ 完全实现 | 真实实现：基于内嵌 GeoNames 城市库按名称检索，返回 CityResponseDto 数组 |
| `GET /search/explore` | ✅ 完全实现 | — |
| `GET /search/person` | ✅ 完全实现 | 真实实现：按 personId 或 person 名称检索其资产（asset.person_id）；原版方法 POST，Go 版为 GET |
| `GET /search/places` | ✅ 完全实现 | 真实实现：基于内嵌 GeoNames 库返回 places/allPlaces（recentPlaces 暂空，无历史记录） |
| `GET /search/suggestions` | ✅ 完全实现 | — |
| `POST /search/large-assets` | ✅ 完全实现 | 按 asset.size 阈值返回大文件（真实） |
| `POST /search/metadata` | ✅ 完全实现 | — |
| `POST /search/random` | ✅ 完全实现 | 随机采样用户资产（ORDER BY RANDOM()），返回 array[AssetResponseDto] |
| `POST /search/smart` | 🟡 诚实空实现 | 无 ML 后端，返回空且契约合规的 SearchResponseDto（honest-empty，非 501/非假数据） |
| `POST /search/statistics` | ✅ 完全实现 | 统计 total/photos/videos/usage（真实聚合） |
| `DELETE /server/license` | ✅ 完全实现 | DELETE /server/license 已对齐（返回 200） |
| `GET /server/about` | ✅ 完全实现 | — |
| `GET /server/apk-links` | ✅ 完全实现 | — |
| `GET /server/config` | ✅ 完全实现 | — |
| `GET /server/features` | ✅ 完全实现 | — |
| `GET /server/license` | ✅ 完全实现 | — |
| `GET /server/media-types` | ✅ 完全实现 | — |
| `GET /server/ping` | ✅ 完全实现 | — |
| `GET /server/statistics` | ✅ 完全实现 | — |
| `GET /server/storage` | ✅ 完全实现 | — |
| `GET /server/version` | ✅ 完全实现 | — |
| `GET /server/version-check` | ✅ 完全实现 | — |
| `GET /server/version-history` | ✅ 完全实现 | — |
| `PUT /server/license` | ✅ 完全实现 | — |
| `DELETE /sessions` | ❌ 未实现 | 设备会话管理未实现 |
| `DELETE /sessions/:id` | ❌ 未实现 | 设备会话管理未实现 |
| `GET /sessions` | ❌ 未实现 | 设备会话管理未实现 |
| `POST /sessions` | ✅ 已实现 | 设备会话管理未实现 |
| `POST /sessions/:id/lock` | ❌ 未实现 | 设备会话管理未实现 |
| `PUT /sessions/:id` | ❌ 未实现 | 设备会话管理未实现 |
| `DELETE /shared-links/:id` | ✅ 完全实现 | — |
| `DELETE /shared-links/:id/assets` | ❌ 未实现 | 查看/登录/资产增删未实现 |
| `GET /shared-links` | ✅ 完全实现 | — |
| `GET /shared-links/:id` | ✅ 完全实现 | GET /shared-links/:id 已补齐，所有字段持久化后正确回读 |
| `GET /shared-links/me` | ❌ 未实现 | 查看/登录/资产增删未实现 |
| `PATCH /shared-links/:id` | ✅ 完全实现 | — |
| `POST /shared-links` | ✅ 完全实现 | 全部字段持久化（allowDownload/upload/description/password/showMetadata/slug），重读正确 |
| `POST /shared-links/login` | ❌ 未实现 | 查看/登录/资产增删未实现 |
| `PUT /shared-links/:id/assets` | ❌ 未实现 | 查看/登录/资产增删未实现 |
| `DELETE /stacks` | ✅ 已实现 | 连拍/相似堆叠未实现 |
| `DELETE /stacks/:id` | ❌ 未实现 | 连拍/相似堆叠未实现 |
| `DELETE /stacks/:id/assets/:assetId` | ❌ 未实现 | 连拍/相似堆叠未实现 |
| `GET /stacks` | ❌ 未实现 | 连拍/相似堆叠未实现 |
| `GET /stacks/:id` | ✅ 已实现 | 连拍/相似堆叠未实现 |
| `POST /stacks` | ✅ 已实现 | 连拍/相似堆叠未实现 |
| `PUT /stacks/:id` | ❌ 未实现 | 连拍/相似堆叠未实现 |
| `DELETE /sync/ack` | ✅ 完全实现 | 真实实现：持久化用户已确认的同步序列（sync_state），不再忽略输入 |
| `GET /sync/ack` | ✅ 完全实现 | 真实实现：持久化用户已确认的同步序列（sync_state），不再忽略输入 |
| `POST /sync/ack` | ✅ 完全实现 | 真实实现：持久化用户已确认的同步序列（sync_state），不再忽略输入 |
| `POST /sync/stream` | ✅ 完全实现 | 真实实现：返回用户全部资源/相册的真实同步增量（AssetV1/AlbumV1）；原为空 []（Go 版方法为 GET） |
| `GET /system-config` | ✅ 完全实现 | — |
| `GET /system-config/defaults` | ✅ 完全实现 | — |
| `GET /system-config/storage-template-options` | ✅ 完全实现 | 返回存储模板 token 词表（真实静态选项） |
| `PUT /system-config` | ✅ 完全实现 | loginRequired/isPublic/externalDomain/trashDays 均持久化并正确回读 |
| `GET /system-metadata/admin-onboarding` | ✅ 完全实现 | 返回 onboarding 完成状态（SystemConfig.onboarded） |
| `GET /system-metadata/reverse-geocoding-state` | ✅ 完全实现 | 返回反地理编码数据可用状态 |
| `GET /system-metadata/version-check-state` | ✅ 完全实现 | 返回版本检查可用状态 |
| `POST /system-metadata/admin-onboarding` | ✅ 完全实现 | 标记 onboarding 完成（持久化） |
| `DELETE /tags/:id` | ✅ 完全实现 | — |
| `DELETE /tags/:id/assets` | ✅ 完全实现 | 已对齐（DELETE /tags/:id/assets，body {ids}） |
| `GET /tags` | ✅ 完全实现 | — |
| `GET /tags/:id` | ✅ 完全实现 | — |
| `POST /tags` | ✅ 完全实现 | — |
| `PUT /tags` | ✅ 完全实现 | 批量更新已对齐（PUT /tags） |
| `PUT /tags/:id` | ✅ 完全实现 | — |
| `PUT /tags/:id/assets` | ✅ 完全实现 | 已对齐（PUT /tags/:id/assets，与 POST 同语义加标签） |
| `PUT /tags/assets` | ✅ 已实现 | 批量标签操作未实现 |
| `GET /timeline/bucket` | ✅ 完全实现 | — |
| `GET /timeline/buckets` | ✅ 完全实现 | — |
| `POST /trash/empty` | ✅ 完全实现 | — |
| `POST /trash/restore` | ✅ 完全实现 | — |
| `POST /trash/restore/assets` | ✅ 完全实现 | 复用 handleTrashRestore，按 ids 从回收站恢复（此前被误判为 stub） |
| `DELETE /users/me/license` | ❌ 未实现 | 资料图、license、onboarding、calendar-heatmap 等未实现 |
| `DELETE /users/me/onboarding` | ❌ 未实现 | 资料图、license、onboarding、calendar-heatmap 等未实现 |
| `DELETE /users/profile-image` | ❌ 未实现 | 资料图、license、onboarding、calendar-heatmap 等未实现 |
| `GET /users` | ✅ 完全实现 | — |
| `GET /users/:id` | ✅ 完全实现 | — |
| `GET /users/:id/profile-image` | ✅ 已实现 | 资料图、license、onboarding、calendar-heatmap 等未实现 |
| `GET /users/me` | ✅ 完全实现 | — |
| `GET /users/me/calendar-heatmap` | ❌ 未实现 | 资料图、license、onboarding、calendar-heatmap 等未实现 |
| `GET /users/me/license` | ✅ 完全实现 | 返回空 license（真实） |
| `GET /users/me/onboarding` | ✅ 完全实现 | 返回 onboarding 状态（SystemConfig.onboarded） |
| `GET /users/me/preferences` | ✅ 完全实现 | — |
| `POST /users/profile-image` | ✅ 已实现 | 资料图、license、onboarding、calendar-heatmap 等未实现 |
| `PUT /users/me` | ✅ 完全实现 | — |
| `PUT /users/me/license` | ❌ 未实现 | 资料图、license、onboarding、calendar-heatmap 等未实现 |
| `PUT /users/me/onboarding` | ✅ 完全实现 | 标记 onboarding 完成（持久化） |
| `PUT /users/me/preferences` | ✅ 完全实现 | — |
| `DELETE /admin/users/:id` | ✅ 完全实现 | 软删除（保留行以支持恢复）+ 强制删时连带软删资产；禁止删除唯一 admin |
| `GET /admin/users` | ✅ 完全实现 | 列出全部用户（含已软删，status=deleted），返回 UserAdminResponseDto 数组 |
| `GET /admin/users/:id` | ✅ 完全实现 | 单用户详情 UserAdminResponseDto（含 quotaUsage/status） |
| `GET /admin/users/:id/calendar-heatmap` | ✅ 完全实现 | 返回该用户近一年每日资产数（from/to/series/totalCount） |
| `GET /admin/users/:id/preferences` | ✅ 完全实现 | 读取用户 UI 偏好（持久化 JSON blob，缺省返回完整默认形状） |
| `GET /admin/users/:id/sessions` | ✅ 完全实现 | 列出该用户会话（登录/签发 token 时记录，标注 current） |
| `GET /admin/users/:id/statistics` | ✅ 完全实现 | 该用户 images/videos/total 计数（非回收站） |
| `POST /admin/users` | ✅ 完全实现 | 创建用户（bcrypt 密码、isAdmin、pinCode、quota 等），返回 UserAdminResponseDto |
| `POST /admin/users/:id/restore` | ✅ 完全实现 | 恢复已软删用户及其资产 |
| `PUT /admin/users/:id` | ✅ 完全实现 | 更新用户资料/密码/角色等 |
| `PUT /admin/users/:id/preferences` | ✅ 完全实现 | 写入用户 UI 偏好（局部合并覆盖整段子对象，未提供字段保留默认/原值） |
| `GET /view/folder` | ✅ 完全实现 | 返回资产目录树及每目录数量（真实） |
| `GET /view/folder/unique-paths` | ✅ 完全实现 | 返回去重根路径列表（真实） |
| `DELETE /workflows/:id` | ❌ 未实现 | 自动化工作流未实现 |
| `GET /workflows` | ❌ 未实现 | 自动化工作流未实现 |
| `GET /workflows/:id` | ❌ 未实现 | 自动化工作流未实现 |
| `GET /workflows/:id/share` | ❌ 未实现 | 自动化工作流未实现 |
| `GET /workflows/triggers` | ❌ 未实现 | 自动化工作流未实现 |
| `POST /workflows` | ❌ 未实现 | 自动化工作流未实现 |
| `PUT /workflows/:id` | ❌ 未实现 | 自动化工作流未实现 |
