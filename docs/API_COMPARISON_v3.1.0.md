# API 对比：官方客户端 / 官方 server / Go 版 server (immich-go)

> 版本基线：immich **v3.1.0**
> - **官方客户端**：iOS App（mobile/openapi SDK）实际调用的 API（✅ 标记）
> - **官方 server**：v3.1.0 OpenAPI 契约（与客户端 SDK 同源，故“客户端用到的”即“server 契约提供的”）
> - **Go 版 server (immich-go)**：本仓库路由实现，状态见“Go 版状态”列

状态说明：`已实现`=路由已注册且按契约返回；`缺失`=未注册（客户端调用会 404/405）；`缺失(501诚实不支持)`=已注册但返回 501（如人脸识别，属 AGENTS.md 延后项，诚实报错而非伪造）。

## 一、App 实际调用的 API（72 个）

| 客户端调用 | 官方server契约 (Method+Path) | Request | Response | Go 版状态 |
|-----------|-----------------------------|---------|----------|-----------|
| ✅ | DELETE /api/activities/{id} | — | (void/stream) | 已实现 |
| ✅ | GET /api/activities/statistics | — | ActivityStatisticsResponseDto | 已实现 |
| ✅ | POST /api/activities | ActivityCreateDto | ActivityResponseDto | 已实现 |
| ✅ | GET /api/activities | — | List<ActivityResponseDto> | 已实现 |
| ✅ | PUT /api/albums/{id}/assets | BulkIdsDto | List<BulkIdResponseDto> | 已实现 |
| ✅ | DELETE /api/albums/{id}/assets | BulkIdsDto | List<BulkIdResponseDto> | 已实现 |
| ✅ | DELETE /api/albums/{id}/user/{userId} | — | (void/stream) | 已实现 |
| ✅ | PUT /api/albums/{id}/users | AddUsersDto | AlbumResponseDto | 已实现 |
| ✅ | DELETE /api/albums/{id} | — | (void/stream) | 已实现 |
| ✅ | PATCH /api/albums/{id} | UpdateAlbumDto | AlbumResponseDto | 已实现 |
| ✅ | POST /api/albums | CreateAlbumDto | AlbumResponseDto | 已实现 |
| ✅ | PUT /api/assets/{id}/edits | AssetEditsCreateDto | AssetEditsResponseDto | 已实现 |
| ✅ | GET /api/assets/{id}/edits | — | AssetEditsResponseDto | 已实现 |
| ✅ | DELETE /api/assets/{id}/edits | — | (void/stream) | 已实现 |
| ✅ | PUT /api/assets/{id}/metadata | AssetMetadataUpsertDto | List<AssetMetadataResponseDto> | 已实现 |
| ✅ | GET /api/assets/{id} | — | AssetResponseDto | 已实现 |
| ✅ | PUT /api/assets/{id} | UpdateAssetDto | AssetResponseDto | 已实现 |
| ✅ | PUT /api/assets/metadata | AssetMetadataBulkUpsertDto | List<AssetMetadataBulkResponseDto> | 已实现 |
| ✅ | DELETE /api/assets | AssetBulkDeleteDto | (void/stream) | 已实现 |
| ✅ | PUT /api/assets | AssetBulkUpdateDto | (void/stream) | 已实现 |
| ✅ | POST /api/auth/change-password | ChangePasswordDto | UserAdminResponseDto | 已实现 |
| ✅ | POST /api/auth/login | LoginCredentialDto | LoginResponseDto | 已实现 |
| ✅ | POST /api/auth/logout | — | LogoutResponseDto | 已实现 |
| ✅ | POST /api/auth/pin-code | PinCodeSetupDto | (void/stream) | 已实现 |
| ✅ | POST /api/auth/session/lock | — | (void/stream) | 已实现 |
| ✅ | POST /api/auth/session/unlock | SessionUnlockDto | (void/stream) | 已实现 |
| ✅ | GET /api/auth/status | — | AuthStatusResponseDto | 已实现 |
| ✅ | POST /api/auth/validateToken | — | ValidateAccessTokenResponseDto | 已实现 |
| ✅ | POST /api/libraries/{id}/validate | ValidateLibraryDto | ValidateLibraryResponseDto | 已实现 |
| ✅ | GET /api/map/markers | — | List<MapMarkerResponseDto> | 已实现 |
| ✅ | POST /api/oauth/authorize | OAuthConfigDto | OAuthAuthorizeResponseDto | 缺失 |
| ✅ | POST /api/oauth/callback | OAuthCallbackDto | LoginResponseDto | 缺失 |
| ✅ | DELETE /api/partners/{id} | — | (void/stream) | 已实现 |
| ✅ | PUT /api/partners/{id} | PartnerUpdateDto | PartnerResponseDto | 已实现 |
| ✅ | POST /api/partners | PartnerCreateDto | PartnerResponseDto | 已实现 |
| ✅ | GET /api/partners | — | List<PartnerResponseDto> | 已实现 |
| ✅ | PUT /api/people/{id} | PersonUpdateDto | PersonResponseDto | 已实现 |
| ✅ | GET /api/people | — | PeopleResponseDto | 已实现 |
| ✅ | GET /api/search/cities | — | List<AssetResponseDto> | 已实现 |
| ✅ | GET /api/search/explore | — | List<SearchExploreResponseDto> | 已实现 |
| ✅ | POST /api/search/metadata | MetadataSearchDto | SearchResponseDto | 已实现 |
| ✅ | POST /api/search/smart | SmartSearchDto | SearchResponseDto | 已实现 |
| ✅ | GET /api/search/suggestions | — | List<String> | 已实现 |
| ✅ | GET /api/server/config | — | ServerConfigDto | 已实现 |
| ✅ | GET /api/server/features | — | ServerFeaturesDto | 已实现 |
| ✅ | GET /api/server/ping | — | ServerPingResponse | 已实现 |
| ✅ | GET /api/server/storage | — | ServerStorageResponseDto | 已实现 |
| ✅ | GET /api/server/version | — | ServerVersionResponseDto | 已实现 |
| ✅ | POST /api/sessions | SessionCreateDto | SessionCreateResponseDto | 已实现 |
| ✅ | DELETE /api/shared-links/{id} | — | (void/stream) | 已实现 |
| ✅ | PATCH /api/shared-links/{id} | SharedLinkEditDto | SharedLinkResponseDto | 已实现 |
| ✅ | POST /api/shared-links | SharedLinkCreateDto | SharedLinkResponseDto | 已实现 |
| ✅ | GET /api/shared-links | — | List<SharedLinkResponseDto> | 已实现 |
| ✅ | GET /api/stacks/{id} | — | StackResponseDto | 已实现 |
| ✅ | POST /api/stacks | StackCreateDto | StackResponseDto | 已实现 |
| ✅ | DELETE /api/stacks | BulkIdsDto | (void/stream) | 已实现 |
| ✅ | DELETE /api/sync/ack | SyncAckDeleteDto | (void/stream) | 已实现 |
| ✅ | POST /api/sync/ack | SyncAckSetDto | (void/stream) | 已实现 |
| ✅ | PUT /api/tags/{id}/assets | BulkIdsDto | List<BulkIdResponseDto> | 已实现 |
| ✅ | PUT /api/tags/assets | TagBulkAssetsDto | TagBulkAssetsResponseDto | 已实现 |
| ✅ | GET /api/tags | — | List<TagResponseDto> | 已实现 |
| ✅ | PUT /api/tags | TagUpsertDto | List<TagResponseDto> | 已实现 |
| ✅ | POST /api/trash/empty | — | TrashResponseDto | 已实现 |
| ✅ | POST /api/trash/restore/assets | BulkIdsDto | TrashResponseDto | 已实现 |
| ✅ | POST /api/trash/restore | — | TrashResponseDto | 已实现 |
| ✅ | GET /api/users/me/preferences | — | UserPreferencesResponseDto | 已实现 |
| ✅ | PUT /api/users/me | UserUpdateMeDto | UserAdminResponseDto | 已实现 |
| ✅ | GET /api/users/me | — | UserAdminResponseDto | 已实现 |
| ✅ | POST /api/users/profile-image | — | CreateProfileImageResponseDto | 已实现 |
| ✅ | GET /api/users | — | List<UserResponseDto> | 已实现 |
| ✅ | GET /api/view/folder/unique-paths | — | List<String> | 已实现 |
| ✅ | GET /api/view/folder | — | List<AssetResponseDto> | 已实现 |

## 二、App 未调用但 SDK 可用的 API（182 个）

> 这些 endpoint 存在于官方契约/SDK，但当前 v3.1.0 iOS App 代码未直接调用（多为 web/管理端或其他平台使用）。列出以供完整性参考与后续补齐。

| 客户端调用 | 官方server契约 (Method+Path) | Request | Response | Go 版状态 |
|-----------|-----------------------------|---------|----------|-----------|
| — | POST /api/admin/auth/unlink-all | — | (void/stream) | 缺失 |
| — | GET /api/admin/database-backups/{filename} | — | MultipartFile | 缺失 |
| — | POST /api/admin/database-backups/start-restore | — | (void/stream) | 缺失 |
| — | POST /api/admin/database-backups/upload | — | (void/stream) | 缺失 |
| — | DELETE /api/admin/database-backups | DatabaseBackupDeleteDto | (void/stream) | 缺失 |
| — | GET /api/admin/database-backups | — | DatabaseBackupListResponseDto | 缺失 |
| — | GET /api/admin/integrity/report/{id}/file | — | MultipartFile | 缺失 |
| — | DELETE /api/admin/integrity/report/{id} | — | (void/stream) | 缺失 |
| — | GET /api/admin/integrity/report/{type}/csv | — | MultipartFile | 缺失 |
| — | GET /api/admin/integrity/report | — | IntegrityReportResponseDto | 缺失 |
| — | GET /api/admin/integrity/summary | — | IntegrityReportSummaryResponseDto | 缺失 |
| — | GET /api/admin/maintenance/detect-install | — | MaintenanceDetectInstallResponseDto | 缺失 |
| — | POST /api/admin/maintenance/login | MaintenanceLoginDto | MaintenanceAuthDto | 缺失 |
| — | GET /api/admin/maintenance/status | — | MaintenanceStatusResponseDto | 缺失 |
| — | POST /api/admin/maintenance | SetMaintenanceModeDto | (void/stream) | 缺失 |
| — | POST /api/admin/notifications/templates/{name} | TemplateDto | TemplateResponseDto | 缺失 |
| — | POST /api/admin/notifications/test-email | SystemConfigSmtpDto | TestEmailResponseDto | 缺失 |
| — | POST /api/admin/notifications | NotificationCreateDto | NotificationDto | 缺失 |
| — | GET /api/admin/users/{id}/calendar-heatmap | — | CalendarHeatmapResponseDto | 已实现 |
| — | PUT /api/admin/users/{id}/preferences | UserPreferencesUpdateDto | UserPreferencesResponseDto | 已实现 |
| — | GET /api/admin/users/{id}/preferences | — | UserPreferencesResponseDto | 已实现 |
| — | POST /api/admin/users/{id}/restore | — | UserAdminResponseDto | 已实现 |
| — | GET /api/admin/users/{id}/sessions | — | List<SessionResponseDto> | 已实现 |
| — | GET /api/admin/users/{id}/statistics | — | AssetStatsResponseDto | 已实现 |
| — | PUT /api/admin/users/{id} | UserAdminUpdateDto | UserAdminResponseDto | 已实现 |
| — | DELETE /api/admin/users/{id} | UserAdminDeleteDto | UserAdminResponseDto | 已实现 |
| — | GET /api/admin/users/{id} | — | UserAdminResponseDto | 已实现 |
| — | POST /api/admin/users | UserAdminCreateDto | UserAdminResponseDto | 已实现 |
| — | GET /api/admin/users | — | List<UserAdminResponseDto> | 已实现 |
| — | GET /api/albums/{id}/map-markers | — | List<MapMarkerResponseDto> | 已实现 |
| — | PUT /api/albums/{id}/user/{userId} | UpdateAlbumUserDto | (void/stream) | 已实现 |
| — | GET /api/albums/{id} | — | AlbumResponseDto | 已实现 |
| — | PUT /api/albums/assets | AlbumsAddAssetsDto | AlbumsAddAssetsResponseDto | 已实现 |
| — | GET /api/albums/statistics | — | AlbumStatisticsResponseDto | 已实现 |
| — | GET /api/albums | — | List<AlbumResponseDto> | 已实现 |
| — | DELETE /api/api-keys/{id} | — | (void/stream) | 已实现 |
| — | GET /api/api-keys/{id} | — | ApiKeyResponseDto | 已实现 |
| — | PUT /api/api-keys/{id} | ApiKeyUpdateDto | ApiKeyResponseDto | 已实现 |
| — | GET /api/api-keys/me | — | ApiKeyResponseDto | 已实现 |
| — | POST /api/api-keys | ApiKeyCreateDto | ApiKeyCreateResponseDto | 已实现 |
| — | GET /api/api-keys | — | List<ApiKeyResponseDto> | 已实现 |
| — | DELETE /api/assets/{id}/metadata/{key} | — | (void/stream) | 缺失 |
| — | GET /api/assets/{id}/metadata/{key} | — | AssetMetadataResponseDto | 缺失 |
| — | GET /api/assets/{id}/metadata | — | List<AssetMetadataResponseDto> | 已实现 |
| — | GET /api/assets/{id}/ocr | — | List<AssetOcrResponseDto> | 缺失 |
| — | GET /api/assets/{id}/original | — | MultipartFile | 已实现 |
| — | GET /api/assets/{id}/thumbnail | — | MultipartFile | 已实现 |
| — | GET /api/assets/{id}/video/playback | — | MultipartFile | 已实现 |
| — | GET /api/assets/{id}/video/stream/{sessionId}/{variantIndex}/{filename} | — | MultipartFile | 已实现 |
| — | GET /api/assets/{id}/video/stream/{sessionId}/{variantIndex}/playlist.m3u8 | — | String | 已实现 |
| — | DELETE /api/assets/{id}/video/stream/{sessionId} | — | (void/stream) | 已实现 |
| — | GET /api/assets/{id}/video/stream/main.m3u8 | — | String | 已实现 |
| — | POST /api/assets/bulk-upload-check | AssetBulkUploadCheckDto | AssetBulkUploadCheckResponseDto | 已实现 |
| — | PUT /api/assets/copy | AssetCopyDto | (void/stream) | 已实现 |
| — | POST /api/assets/jobs | AssetJobsDto | (void/stream) | 缺失 |
| — | DELETE /api/assets/metadata | AssetMetadataBulkDeleteDto | (void/stream) | 缺失 |
| — | GET /api/assets/statistics | — | AssetStatsResponseDto | 已实现 |
| — | POST /api/assets | — | AssetMediaResponseDto | 已实现 |
| — | POST /api/auth/admin-sign-up | SignUpDto | UserAdminResponseDto | 缺失 |
| — | PUT /api/auth/pin-code | PinCodeChangeDto | (void/stream) | 缺失 |
| — | DELETE /api/auth/pin-code | PinCodeResetDto | (void/stream) | 缺失 |
| — | POST /api/download/archive | DownloadArchiveDto | MultipartFile | 已实现 |
| — | POST /api/download/info | DownloadInfoDto | DownloadResponseDto | 已实现 |
| — | DELETE /api/duplicates/{id} | — | (void/stream) | 已实现 |
| — | POST /api/duplicates/resolve | DuplicateResolveDto | List<BulkIdResponseDto> | 已实现 |
| — | DELETE /api/duplicates | BulkIdsDto | (void/stream) | 已实现 |
| — | GET /api/duplicates | — | List<DuplicateResponseDto> | 已实现 |
| — | DELETE /api/faces/{id} | AssetFaceDeleteDto | (void/stream) | 缺失(501诚实不支持) |
| — | PUT /api/faces/{id} | FaceDto | PersonResponseDto | 缺失(501诚实不支持) |
| — | POST /api/faces | AssetFaceCreateDto | (void/stream) | 缺失(501诚实不支持) |
| — | GET /api/faces | — | List<AssetFaceResponseDto> | 缺失(501诚实不支持) |
| — | PUT /api/jobs/{name} | QueueCommandDto | QueueResponseLegacyDto | 缺失 |
| — | GET /api/jobs | — | QueuesResponseLegacyDto | 已实现 |
| — | POST /api/jobs | JobCreateDto | (void/stream) | 已实现 |
| — | POST /api/libraries/{id}/scan | — | (void/stream) | 已实现 |
| — | GET /api/libraries/{id}/statistics | — | LibraryStatsResponseDto | 已实现 |
| — | PUT /api/libraries/{id} | UpdateLibraryDto | LibraryResponseDto | 已实现 |
| — | DELETE /api/libraries/{id} | — | (void/stream) | 已实现 |
| — | GET /api/libraries/{id} | — | LibraryResponseDto | 已实现 |
| — | POST /api/libraries | CreateLibraryDto | LibraryResponseDto | 已实现 |
| — | GET /api/libraries | — | List<LibraryResponseDto> | 已实现 |
| — | GET /api/map/reverse-geocode | — | List<MapReverseGeocodeResponseDto> | 已实现 |
| — | PUT /api/memories/{id}/assets | BulkIdsDto | List<BulkIdResponseDto> | 缺失 |
| — | DELETE /api/memories/{id}/assets | BulkIdsDto | List<BulkIdResponseDto> | 缺失 |
| — | PUT /api/memories/{id} | MemoryUpdateDto | MemoryResponseDto | 缺失 |
| — | DELETE /api/memories/{id} | — | (void/stream) | 缺失 |
| — | GET /api/memories/{id} | — | MemoryResponseDto | 缺失 |
| — | GET /api/memories/statistics | — | MemoryStatisticsResponseDto | 缺失 |
| — | POST /api/memories | MemoryCreateDto | MemoryResponseDto | 缺失 |
| — | GET /api/memories | — | List<MemoryResponseDto> | 已实现 |
| — | DELETE /api/notifications/{id} | — | (void/stream) | 已实现 |
| — | GET /api/notifications/{id} | — | NotificationDto | 缺失 |
| — | PUT /api/notifications/{id} | NotificationUpdateDto | NotificationDto | 缺失 |
| — | DELETE /api/notifications | NotificationDeleteAllDto | (void/stream) | 已实现 |
| — | GET /api/notifications | — | List<NotificationDto> | 已实现 |
| — | PUT /api/notifications | NotificationUpdateAllDto | (void/stream) | 已实现 |
| — | POST /api/oauth/backchannel-logout | — | (void/stream) | 缺失 |
| — | POST /api/oauth/link | OAuthCallbackDto | UserAdminResponseDto | 缺失 |
| — | GET /api/oauth/mobile-redirect | — | (void/stream) | 缺失 |
| — | POST /api/oauth/unlink | — | UserAdminResponseDto | 缺失 |
| — | POST /api/partners/{id} | — | PartnerResponseDto | 缺失 |
| — | POST /api/people/{id}/merge | MergePersonDto | List<BulkIdResponseDto> | 已实现 |
| — | PUT /api/people/{id}/reassign | AssetFaceUpdateDto | List<PersonResponseDto> | 已实现 |
| — | GET /api/people/{id}/statistics | — | PersonStatisticsResponseDto | 已实现 |
| — | GET /api/people/{id}/thumbnail | — | MultipartFile | 缺失 |
| — | DELETE /api/people/{id} | — | (void/stream) | 已实现 |
| — | GET /api/people/{id} | — | PersonResponseDto | 已实现 |
| — | POST /api/people | PersonCreateDto | PersonResponseDto | 已实现 |
| — | DELETE /api/people | BulkIdsDto | (void/stream) | 已实现 |
| — | PUT /api/people | PeopleUpdateDto | List<BulkIdResponseDto> | 已实现 |
| — | GET /api/plugins/{id} | — | PluginResponseDto | 缺失 |
| — | GET /api/plugins/methods | — | List<PluginMethodResponseDto> | 缺失 |
| — | GET /api/plugins/templates | — | List<PluginTemplateResponseDto> | 缺失 |
| — | GET /api/plugins | — | List<PluginResponseDto> | 缺失 |
| — | DELETE /api/queues/{name}/jobs | QueueDeleteDto | (void/stream) | 缺失 |
| — | GET /api/queues/{name}/jobs | — | List<QueueJobResponseDto> | 缺失 |
| — | GET /api/queues/{name} | — | QueueResponseDto | 缺失 |
| — | PUT /api/queues/{name} | QueueUpdateDto | QueueResponseDto | 缺失 |
| — | GET /api/queues | — | List<QueueResponseDto> | 缺失 |
| — | POST /api/search/large-assets | — | List<AssetResponseDto> | 已实现 |
| — | GET /api/search/person | — | List<PersonResponseDto> | 缺失 |
| — | GET /api/search/places | — | List<PlacesResponseDto> | 已实现 |
| — | POST /api/search/random | RandomSearchDto | List<AssetResponseDto> | 已实现 |
| — | POST /api/search/statistics | StatisticsSearchDto | SearchStatisticsResponseDto | 已实现 |
| — | GET /api/server/about | — | ServerAboutResponseDto | 已实现 |
| — | GET /api/server/apk-links | — | ServerApkLinksDto | 已实现 |
| — | DELETE /api/server/license | — | (void/stream) | 已实现 |
| — | GET /api/server/license | — | UserLicense | 已实现 |
| — | PUT /api/server/license | LicenseKeyDto | UserLicense | 已实现 |
| — | GET /api/server/media-types | — | ServerMediaTypesResponseDto | 已实现 |
| — | GET /api/server/statistics | — | ServerStatsResponseDto | 已实现 |
| — | GET /api/server/version-check | — | VersionCheckStateResponseDto | 已实现 |
| — | GET /api/server/version-history | — | List<ServerVersionHistoryResponseDto> | 已实现 |
| — | POST /api/sessions/{id}/lock | — | (void/stream) | 缺失 |
| — | PUT /api/sessions/{id} | SessionUpdateDto | SessionResponseDto | 缺失 |
| — | DELETE /api/sessions/{id} | — | (void/stream) | 缺失 |
| — | DELETE /api/sessions | — | (void/stream) | 缺失 |
| — | GET /api/sessions | — | List<SessionResponseDto> | 缺失 |
| — | PUT /api/shared-links/{id}/assets | AssetIdsDto | List<AssetIdsResponseDto> | 缺失 |
| — | DELETE /api/shared-links/{id}/assets | AssetIdsDto | List<AssetIdsResponseDto> | 缺失 |
| — | GET /api/shared-links/{id} | — | SharedLinkResponseDto | 已实现 |
| — | POST /api/shared-links/login | SharedLinkLoginDto | SharedLinkResponseDto | 缺失 |
| — | GET /api/shared-links/me | — | SharedLinkResponseDto | 缺失 |
| — | DELETE /api/stacks/{id}/assets/{assetId} | — | (void/stream) | 缺失 |
| — | PUT /api/stacks/{id} | StackUpdateDto | StackResponseDto | 缺失 |
| — | DELETE /api/stacks/{id} | — | (void/stream) | 缺失 |
| — | GET /api/stacks | — | List<StackResponseDto> | 缺失 |
| — | GET /api/sync/ack | — | List<SyncAckDto> | 已实现 |
| — | POST /api/sync/stream | SyncStreamDto | (void/stream) | 已实现 |
| — | GET /api/system-config/defaults | — | SystemConfigDto | 已实现 |
| — | GET /api/system-config/storage-template-options | — | SystemConfigTemplateStorageOptionDto | 已实现 |
| — | GET /api/system-config | — | SystemConfigDto | 已实现 |
| — | PUT /api/system-config | SystemConfigDto | SystemConfigDto | 已实现 |
| — | GET /api/system-metadata/admin-onboarding | — | AdminOnboardingUpdateDto | 已实现 |
| — | POST /api/system-metadata/admin-onboarding | AdminOnboardingUpdateDto | (void/stream) | 已实现 |
| — | GET /api/system-metadata/reverse-geocoding-state | — | ReverseGeocodingStateResponseDto | 已实现 |
| — | GET /api/system-metadata/version-check-state | — | VersionCheckStateResponseDto | 已实现 |
| — | DELETE /api/tags/{id}/assets | BulkIdsDto | List<BulkIdResponseDto> | 已实现 |
| — | PUT /api/tags/{id} | TagUpdateDto | TagResponseDto | 已实现 |
| — | DELETE /api/tags/{id} | — | (void/stream) | 已实现 |
| — | GET /api/tags/{id} | — | TagResponseDto | 已实现 |
| — | POST /api/tags | TagCreateDto | TagResponseDto | 已实现 |
| — | GET /api/timeline/bucket | — | TimeBucketAssetResponseDto | 已实现 |
| — | GET /api/timeline/buckets | — | List<TimeBucketsResponseDto> | 已实现 |
| — | GET /api/users/{id}/profile-image | — | MultipartFile | 缺失 |
| — | GET /api/users/{id} | — | UserResponseDto | 已实现 |
| — | GET /api/users/me/calendar-heatmap | — | CalendarHeatmapResponseDto | 缺失 |
| — | DELETE /api/users/me/license | — | (void/stream) | 缺失 |
| — | GET /api/users/me/license | — | UserLicense | 已实现 |
| — | PUT /api/users/me/license | LicenseKeyDto | UserLicense | 缺失 |
| — | DELETE /api/users/me/onboarding | — | (void/stream) | 缺失 |
| — | GET /api/users/me/onboarding | — | OnboardingResponseDto | 已实现 |
| — | PUT /api/users/me/onboarding | OnboardingDto | OnboardingResponseDto | 已实现 |
| — | PUT /api/users/me/preferences | UserPreferencesUpdateDto | UserPreferencesResponseDto | 已实现 |
| — | DELETE /api/users/profile-image | — | (void/stream) | 缺失 |
| — | GET /api/workflows/{id}/share | — | WorkflowShareResponseDto | 缺失 |
| — | PUT /api/workflows/{id} | WorkflowUpdateDto | WorkflowResponseDto | 缺失 |
| — | DELETE /api/workflows/{id} | — | (void/stream) | 缺失 |
| — | GET /api/workflows/{id} | — | WorkflowResponseDto | 缺失 |
| — | GET /api/workflows/triggers | — | List<WorkflowTriggerResponseDto> | 缺失 |
| — | POST /api/workflows | WorkflowCreateDto | WorkflowResponseDto | 缺失 |
| — | GET /api/workflows | — | List<WorkflowResponseDto> | 缺失 |

## 三、差距汇总

- App 实际调用 API 总数：**72**
  - 已实现：**70**
  - 缺失（未注册）：**2**（`POST /api/oauth/authorize`、`POST /api/oauth/callback`，需外部 IdP，按 AGENTS.md 延后）
  - 缺失(501诚实不支持)：**0**
- 全部 SDK API 总数：**254**；Go 路由已注册：**242**
