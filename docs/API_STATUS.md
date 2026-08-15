# immich-go API 实现现状清单（对照原版 v3.1.0）

> 以官方 OpenAPI 契约 `open-api/immich-openapi-specs.json`（tag **v3.1.0**，254 个 operation）为基准，与 `internal/app/app.go` 实际路由逐条核对。

> 状态列四档：**✅ 完全实现**（真实逻辑+契约兼容）/ **🟡 部分实现**（已实现但逻辑不完整或弱于原版）/ **🟠 API占位/逻辑存疑**（端点存在但仅返回空/最小形状，无真实逻辑）/ **❌ 未实现**（端点缺失，或仅以不同 HTTP 方法提供）。

> 统计：✅ 76 · 🟡 23 · 🟠 25 · ❌ 130 （共 254）


| 原版 API（方法 + 路径） | Go 版实现现状 | 与原版的差距 |
|---|---|---|
| `DELETE /api-keys/:id` | ✅ 完全实现 | — |
| `GET /api-keys` | 🟡 部分实现 | 缺单 key 读取/更新与 /me 别名 |
| `GET /api-keys/:id` | 🟡 部分实现 | 方法不一致（Go 版为 DELETE，原版为 GET） |
| `GET /api-keys/me` | ❌ 未实现 | 单 key 读取/更新与 /me 别名未实现 |
| `POST /api-keys` | ✅ 完全实现 | — |
| `PUT /api-keys/:id` | 🟡 部分实现 | 方法不一致（Go 版为 DELETE，原版为 PUT） |
| `DELETE /activities/:id` | ✅ 完全实现 | — |
| `GET /activities` | ✅ 完全实现 | — |
| `GET /activities/statistics` | ❌ 未实现 | 统计端点未实现 |
| `POST /activities` | ✅ 完全实现 | — |
| `DELETE /albums/:id` | ✅ 完全实现 | — |
| `DELETE /albums/:id/assets` | ✅ 完全实现 | — |
| `DELETE /albums/:id/user/:userId` | ❌ 未实现 | 相册内用户共享/批量/地图标记未实现 |
| `GET /albums` | ✅ 完全实现 | — |
| `GET /albums/:id` | ✅ 完全实现 | — |
| `GET /albums/:id/map-markers` | ❌ 未实现 | 相册内用户共享/批量/地图标记未实现 |
| `GET /albums/statistics` | ✅ 完全实现 | — |
| `PATCH /albums/:id` | ✅ 完全实现 | — |
| `POST /albums` | ✅ 完全实现 | — |
| `PUT /albums/:id/assets` | ✅ 完全实现 | — |
| `PUT /albums/:id/user/:userId` | ❌ 未实现 | 相册内用户共享/批量/地图标记未实现 |
| `PUT /albums/:id/users` | ❌ 未实现 | 相册内用户共享/批量/地图标记未实现 |
| `PUT /albums/assets` | ❌ 未实现 | 相册内用户共享/批量/地图标记未实现 |
| `DELETE /assets` | ✅ 完全实现 | — |
| `DELETE /assets/:id/edits` | ❌ 未实现 | 元数据写入/复制/edits 历史/OCR/资产级 job 未实现 |
| `DELETE /assets/:id/metadata/:key` | ❌ 未实现 | 元数据写入/复制/edits 历史/OCR/资产级 job 未实现 |
| `DELETE /assets/:id/video/stream/:sessionId` | ✅ 完全实现 | 无状态删除，正确 204 |
| `DELETE /assets/metadata` | ❌ 未实现 | 元数据写入/复制/edits 历史/OCR/资产级 job 未实现 |
| `GET /assets/:id` | ✅ 完全实现 | — |
| `GET /assets/:id/edits` | ❌ 未实现 | 元数据写入/复制/edits 历史/OCR/资产级 job 未实现 |
| `GET /assets/:id/metadata` | 🟡 部分实现 | 仅读；写入端点 PUT /assets/:id/metadata 未实现，visibility 不持久化 |
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
| `PUT /assets/:id/edits` | ❌ 未实现 | 元数据写入/复制/edits 历史/OCR/资产级 job 未实现 |
| `PUT /assets/:id/metadata` | 🟡 部分实现 | 仅读；写入端点 PUT /assets/:id/metadata 未实现，visibility 不持久化 〔Go 版该方法为 GET〕 |
| `PUT /assets/copy` | ❌ 未实现 | 元数据写入/复制/edits 历史/OCR/资产级 job 未实现 |
| `PUT /assets/metadata` | ❌ 未实现 | 元数据写入/复制/edits 历史/OCR/资产级 job 未实现 |
| `DELETE /auth/pin-code` | ❌ 未实现 | OAuth/SSO、PIN 锁、设备会话锁、admin-signup 未实现 |
| `GET /auth/status` | ✅ 完全实现 | — |
| `GET /oauth/mobile-redirect` | ❌ 未实现 | OAuth/SSO、PIN 锁、设备会话锁、admin-signup 未实现 |
| `POST /auth/admin-sign-up` | ❌ 未实现 | OAuth/SSO、PIN 锁、设备会话锁、admin-signup 未实现 |
| `POST /auth/change-password` | ✅ 完全实现 | — |
| `POST /auth/login` | ✅ 完全实现 | — |
| `POST /auth/logout` | ✅ 完全实现 | — |
| `POST /auth/pin-code` | ❌ 未实现 | OAuth/SSO、PIN 锁、设备会话锁、admin-signup 未实现 |
| `POST /auth/session/lock` | ❌ 未实现 | OAuth/SSO、PIN 锁、设备会话锁、admin-signup 未实现 |
| `POST /auth/session/unlock` | ❌ 未实现 | OAuth/SSO、PIN 锁、设备会话锁、admin-signup 未实现 |
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
| `POST /download/archive` | 🟡 部分实现 | 实现为 GET（原版为 POST），方法不一致 〔Go 版该方法为 GET〕 |
| `POST /download/info` | ✅ 完全实现 | — |
| `DELETE /duplicates` | 🟠 API占位/逻辑存疑 | API 存在但仅返回正确空/最小形状，无真实 ML/同步逻辑（原版依赖 ML 聚类或完整增量同步） |
| `DELETE /duplicates/:id` | 🟠 API占位/逻辑存疑 | API 存在但仅返回正确空/最小形状，无真实 ML/同步逻辑（原版依赖 ML 聚类或完整增量同步） |
| `GET /duplicates` | ✅ 完全实现 | — |
| `POST /duplicates/resolve` | 🟠 API占位/逻辑存疑 | API 存在但仅返回正确空/最小形状，无真实 ML/同步逻辑（原版依赖 ML 聚类或完整增量同步） |
| `DELETE /faces/:id` | 🟠 API占位/逻辑存疑 | API 存在但仅返回正确空/最小形状，无真实 ML/同步逻辑（原版依赖 ML 聚类或完整增量同步） 〔Go 版该方法为 GET〕 |
| `GET /faces` | 🟠 API占位/逻辑存疑 | API 存在但仅返回正确空/最小形状，无真实 ML/同步逻辑（原版依赖 ML 聚类或完整增量同步） |
| `POST /faces` | 🟠 API占位/逻辑存疑 | API 存在但仅返回正确空/最小形状，无真实 ML/同步逻辑（原版依赖 ML 聚类或完整增量同步） 〔Go 版该方法为 GET〕 |
| `PUT /faces/:id` | 🟠 API占位/逻辑存疑 | API 存在但仅返回正确空/最小形状，无真实 ML/同步逻辑（原版依赖 ML 聚类或完整增量同步） 〔Go 版该方法为 GET〕 |
| `GET /jobs` | ✅ 完全实现 | — |
| `POST /jobs` | 🟡 部分实现 | 方法不一致（Go 版为 GET，原版为 POST） |
| `PUT /jobs/:name` | ❌ 未实现 | POST /jobs、PUT /jobs/:name 方法差异 |
| `DELETE /libraries/:id` | ✅ 完全实现 | — |
| `GET /libraries` | ✅ 完全实现 | — |
| `GET /libraries/:id` | ✅ 完全实现 | — |
| `GET /libraries/:id/statistics` | ✅ 完全实现 | — |
| `POST /libraries` | ✅ 完全实现 | — |
| `POST /libraries/:id/scan` | ✅ 完全实现 | — |
| `POST /libraries/:id/validate` | ❌ 未实现 | /validate 未实现 |
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
| `DELETE /partners/:id` | 🟡 部分实现 | 解除关系可用，共享资产未透出 |
| `GET /partners` | 🟡 部分实现 | 关系列出可用，但伙伴共享资产未在时间线/搜索透出 |
| `POST /partners` | 🟡 部分实现 | 建立关系可用，共享资产未透出 |
| `POST /partners/:id` | 🟡 部分实现 | 解除关系可用，共享资产未透出 〔Go 版该方法为 DELETE〕 |
| `PUT /partners/:id` | 🟡 部分实现 | 解除关系可用，共享资产未透出 〔Go 版该方法为 DELETE〕 |
| `DELETE /people` | 🟠 API占位/逻辑存疑 | API 存在但仅返回正确空/最小形状，无真实 ML/同步逻辑（原版依赖 ML 聚类或完整增量同步） 〔Go 版该方法为 GET〕 |
| `DELETE /people/:id` | 🟠 API占位/逻辑存疑 | API 存在但仅返回正确空/最小形状，无真实 ML/同步逻辑（原版依赖 ML 聚类或完整增量同步） 〔Go 版该方法为 GET〕 |
| `GET /people` | 🟠 API占位/逻辑存疑 | API 存在但仅返回正确空/最小形状，无真实 ML/同步逻辑（原版依赖 ML 聚类或完整增量同步） |
| `GET /people/:id` | 🟠 API占位/逻辑存疑 | API 存在但仅返回正确空/最小形状，无真实 ML/同步逻辑（原版依赖 ML 聚类或完整增量同步） |
| `GET /people/:id/statistics` | 🟠 API占位/逻辑存疑 | API 存在但仅返回正确空/最小形状，无真实 ML/同步逻辑（原版依赖 ML 聚类或完整增量同步） |
| `GET /people/:id/thumbnail` | ❌ 未实现 | 人物写入/聚类（ML）未实现 |
| `POST /people` | 🟠 API占位/逻辑存疑 | API 存在但仅返回正确空/最小形状，无真实 ML/同步逻辑（原版依赖 ML 聚类或完整增量同步） 〔Go 版该方法为 GET〕 |
| `POST /people/:id/merge` | 🟠 API占位/逻辑存疑 | API 存在但仅返回正确空/最小形状，无真实 ML/同步逻辑（原版依赖 ML 聚类或完整增量同步） |
| `PUT /people` | 🟠 API占位/逻辑存疑 | API 存在但仅返回正确空/最小形状，无真实 ML/同步逻辑（原版依赖 ML 聚类或完整增量同步） 〔Go 版该方法为 GET〕 |
| `PUT /people/:id` | 🟠 API占位/逻辑存疑 | API 存在但仅返回正确空/最小形状，无真实 ML/同步逻辑（原版依赖 ML 聚类或完整增量同步） 〔Go 版该方法为 GET〕 |
| `PUT /people/:id/reassign` | 🟠 API占位/逻辑存疑 | API 存在但仅返回正确空/最小形状，无真实 ML/同步逻辑（原版依赖 ML 聚类或完整增量同步） |
| `GET /plugins` | ❌ 未实现 | 插件系统未实现 |
| `GET /plugins/:id` | ❌ 未实现 | 插件系统未实现 |
| `GET /plugins/methods` | ❌ 未实现 | 插件系统未实现 |
| `GET /plugins/templates` | ❌ 未实现 | 插件系统未实现 |
| `DELETE /queues/:name/jobs` | ❌ 未实现 | 任务队列可视化未实现 |
| `GET /queues` | ❌ 未实现 | 任务队列可视化未实现 |
| `GET /queues/:name` | ❌ 未实现 | 任务队列可视化未实现 |
| `GET /queues/:name/jobs` | ❌ 未实现 | 任务队列可视化未实现 |
| `PUT /queues/:name` | ❌ 未实现 | 任务队列可视化未实现 |
| `GET /search/cities` | 🟠 API占位/逻辑存疑 | API 存在但仅返回正确空/最小形状，无真实 ML/同步逻辑（原版依赖 ML 聚类或完整增量同步） |
| `GET /search/explore` | ✅ 完全实现 | — |
| `GET /search/person` | 🟠 API占位/逻辑存疑 | API 存在但仅返回正确空/最小形状，无真实 ML/同步逻辑（原版依赖 ML 聚类或完整增量同步） 〔Go 版该方法为 POST〕 |
| `GET /search/places` | 🟠 API占位/逻辑存疑 | API 存在但仅返回正确空/最小形状，无真实 ML/同步逻辑（原版依赖 ML 聚类或完整增量同步） |
| `GET /search/suggestions` | ✅ 完全实现 | — |
| `POST /search/large-assets` | ❌ 未实现 | large-assets/random/statistics 未实现；smart(CLIP)/person(ML) 未实现 |
| `POST /search/metadata` | ✅ 完全实现 | — |
| `POST /search/random` | ❌ 未实现 | large-assets/random/statistics 未实现；smart(CLIP)/person(ML) 未实现 |
| `POST /search/smart` | ❌ 未实现 | large-assets/random/statistics 未实现；smart(CLIP)/person(ML) 未实现 |
| `POST /search/statistics` | ❌ 未实现 | large-assets/random/statistics 未实现；smart(CLIP)/person(ML) 未实现 |
| `DELETE /server/license` | 🟡 部分实现 | 方法不一致（Go 版为 GET，原版为 DELETE） |
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
| `POST /sessions` | ❌ 未实现 | 设备会话管理未实现 |
| `POST /sessions/:id/lock` | ❌ 未实现 | 设备会话管理未实现 |
| `PUT /sessions/:id` | ❌ 未实现 | 设备会话管理未实现 |
| `DELETE /shared-links/:id` | ✅ 完全实现 | — |
| `DELETE /shared-links/:id/assets` | ❌ 未实现 | 查看/登录/资产增删未实现 |
| `GET /shared-links` | ✅ 完全实现 | — |
| `GET /shared-links/:id` | 🟡 部分实现 | 仅更新 type/expiresAt；其余布尔/文本字段未持久化 〔Go 版该方法为 PUT〕 |
| `GET /shared-links/me` | ❌ 未实现 | 查看/登录/资产增删未实现 |
| `PATCH /shared-links/:id` | ✅ 完全实现 | — |
| `POST /shared-links` | 🟡 部分实现 | 创建可用，但 allowDownload/allowUpload/description/password/showMetadata/slug 未持久化，重读丢失 |
| `POST /shared-links/login` | ❌ 未实现 | 查看/登录/资产增删未实现 |
| `PUT /shared-links/:id/assets` | ❌ 未实现 | 查看/登录/资产增删未实现 |
| `DELETE /stacks` | ❌ 未实现 | 连拍/相似堆叠未实现 |
| `DELETE /stacks/:id` | ❌ 未实现 | 连拍/相似堆叠未实现 |
| `DELETE /stacks/:id/assets/:assetId` | ❌ 未实现 | 连拍/相似堆叠未实现 |
| `GET /stacks` | ❌ 未实现 | 连拍/相似堆叠未实现 |
| `GET /stacks/:id` | ❌ 未实现 | 连拍/相似堆叠未实现 |
| `POST /stacks` | ❌ 未实现 | 连拍/相似堆叠未实现 |
| `PUT /stacks/:id` | ❌ 未实现 | 连拍/相似堆叠未实现 |
| `DELETE /sync/ack` | 🟠 API占位/逻辑存疑 | API 存在但仅返回正确空/最小形状，无真实 ML/同步逻辑（原版依赖 ML 聚类或完整增量同步） |
| `GET /sync/ack` | 🟠 API占位/逻辑存疑 | API 存在但仅返回正确空/最小形状，无真实 ML/同步逻辑（原版依赖 ML 聚类或完整增量同步） |
| `POST /sync/ack` | 🟠 API占位/逻辑存疑 | API 存在但仅返回正确空/最小形状，无真实 ML/同步逻辑（原版依赖 ML 聚类或完整增量同步） |
| `POST /sync/stream` | 🟠 API占位/逻辑存疑 | API 存在但仅返回正确空/最小形状，无真实 ML/同步逻辑（原版依赖 ML 聚类或完整增量同步） 〔Go 版该方法为 GET〕 |
| `GET /system-config` | ✅ 完全实现 | — |
| `GET /system-config/defaults` | ✅ 完全实现 | — |
| `GET /system-config/storage-template-options` | ❌ 未实现 | 存储模板选项未实现 |
| `PUT /system-config` | 🟡 部分实现 | 读取完整；部分运行时字段未持久化 |
| `GET /system-metadata/admin-onboarding` | ❌ 未实现 | onboarding/版本检查/反地理状态端点未实现 |
| `GET /system-metadata/reverse-geocoding-state` | ❌ 未实现 | onboarding/版本检查/反地理状态端点未实现 |
| `GET /system-metadata/version-check-state` | ❌ 未实现 | onboarding/版本检查/反地理状态端点未实现 |
| `POST /system-metadata/admin-onboarding` | ❌ 未实现 | onboarding/版本检查/反地理状态端点未实现 |
| `DELETE /tags/:id` | ✅ 完全实现 | — |
| `DELETE /tags/:id/assets` | 🟡 部分实现 | 方法不一致（Go 版为 POST，原版为 DELETE） |
| `GET /tags` | ✅ 完全实现 | — |
| `GET /tags/:id` | ✅ 完全实现 | — |
| `POST /tags` | ✅ 完全实现 | — |
| `PUT /tags` | 🟡 部分实现 | 方法不一致（Go 版为 GET，原版为 PUT） |
| `PUT /tags/:id` | ✅ 完全实现 | — |
| `PUT /tags/:id/assets` | 🟡 部分实现 | 方法不一致（Go 版为 POST，原版为 PUT） |
| `PUT /tags/assets` | ❌ 未实现 | 批量标签操作未实现 |
| `GET /timeline/bucket` | ✅ 完全实现 | — |
| `GET /timeline/buckets` | ✅ 完全实现 | — |
| `POST /trash/empty` | ✅ 完全实现 | — |
| `POST /trash/restore` | ✅ 完全实现 | — |
| `POST /trash/restore/assets` | 🟠 API占位/逻辑存疑 | API 存在但仅返回正确空/最小形状，无真实 ML/同步逻辑（原版依赖 ML 聚类或完整增量同步） |
| `DELETE /users/me/license` | ❌ 未实现 | 资料图、license、onboarding、calendar-heatmap 等未实现 |
| `DELETE /users/me/onboarding` | ❌ 未实现 | 资料图、license、onboarding、calendar-heatmap 等未实现 |
| `DELETE /users/profile-image` | ❌ 未实现 | 资料图、license、onboarding、calendar-heatmap 等未实现 |
| `GET /users` | ✅ 完全实现 | — |
| `GET /users/:id` | ✅ 完全实现 | — |
| `GET /users/:id/profile-image` | ❌ 未实现 | 资料图、license、onboarding、calendar-heatmap 等未实现 |
| `GET /users/me` | ✅ 完全实现 | — |
| `GET /users/me/calendar-heatmap` | ❌ 未实现 | 资料图、license、onboarding、calendar-heatmap 等未实现 |
| `GET /users/me/license` | ❌ 未实现 | 资料图、license、onboarding、calendar-heatmap 等未实现 |
| `GET /users/me/onboarding` | ❌ 未实现 | 资料图、license、onboarding、calendar-heatmap 等未实现 |
| `GET /users/me/preferences` | ✅ 完全实现 | — |
| `POST /users/profile-image` | ❌ 未实现 | 资料图、license、onboarding、calendar-heatmap 等未实现 |
| `PUT /users/me` | ✅ 完全实现 | — |
| `PUT /users/me/license` | ❌ 未实现 | 资料图、license、onboarding、calendar-heatmap 等未实现 |
| `PUT /users/me/onboarding` | ❌ 未实现 | 资料图、license、onboarding、calendar-heatmap 等未实现 |
| `PUT /users/me/preferences` | ✅ 完全实现 | — |
| `DELETE /admin/users/:id` | ❌ 未实现 | 多用户管理后台（创建/恢复/统计/会话/偏好）未实现 |
| `GET /admin/users` | ❌ 未实现 | 多用户管理后台（创建/恢复/统计/会话/偏好）未实现 |
| `GET /admin/users/:id` | ❌ 未实现 | 多用户管理后台（创建/恢复/统计/会话/偏好）未实现 |
| `GET /admin/users/:id/calendar-heatmap` | ❌ 未实现 | 多用户管理后台（创建/恢复/统计/会话/偏好）未实现 |
| `GET /admin/users/:id/preferences` | ❌ 未实现 | 多用户管理后台（创建/恢复/统计/会话/偏好）未实现 |
| `GET /admin/users/:id/sessions` | ❌ 未实现 | 多用户管理后台（创建/恢复/统计/会话/偏好）未实现 |
| `GET /admin/users/:id/statistics` | ❌ 未实现 | 多用户管理后台（创建/恢复/统计/会话/偏好）未实现 |
| `POST /admin/users` | ❌ 未实现 | 多用户管理后台（创建/恢复/统计/会话/偏好）未实现 |
| `POST /admin/users/:id/restore` | ❌ 未实现 | 多用户管理后台（创建/恢复/统计/会话/偏好）未实现 |
| `PUT /admin/users/:id` | ❌ 未实现 | 多用户管理后台（创建/恢复/统计/会话/偏好）未实现 |
| `PUT /admin/users/:id/preferences` | ❌ 未实现 | 多用户管理后台（创建/恢复/统计/会话/偏好）未实现 |
| `GET /view/folder` | ❌ 未实现 | 文件夹视图未实现（官方 Web 用） |
| `GET /view/folder/unique-paths` | ❌ 未实现 | 文件夹视图未实现（官方 Web 用） |
| `DELETE /workflows/:id` | ❌ 未实现 | 自动化工作流未实现 |
| `GET /workflows` | ❌ 未实现 | 自动化工作流未实现 |
| `GET /workflows/:id` | ❌ 未实现 | 自动化工作流未实现 |
| `GET /workflows/:id/share` | ❌ 未实现 | 自动化工作流未实现 |
| `GET /workflows/triggers` | ❌ 未实现 | 自动化工作流未实现 |
| `POST /workflows` | ❌ 未实现 | 自动化工作流未实现 |
| `PUT /workflows/:id` | ❌ 未实现 | 自动化工作流未实现 |
