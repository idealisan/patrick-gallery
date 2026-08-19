package app

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"immich-go/internal/app/geo"
	"immich-go/internal/ml"
	"immich-go/internal/ocr"
	"immich-go/internal/video"
)

// App holds shared dependencies for all HTTP handlers.
type App struct {
	cfg   *Config
	store *Store
	video video.Processor
	ocr   ocr.Processor
	ml    ml.Backend

	// videoCache manages the on-disk cache of transcoded video files with LRU
	// eviction (max 2 GB default). Nil if cache dir creation failed.
	videoCache *video.CacheManager

	// jwtSecret is the per-instance HMAC key (loaded from SystemConfig). It
	// overrides cfg.JWTSecret for token signing/verification so a browser
	// token from a prior deployment is rejected.
	jwtSecret string

	// geocoder provides offline reverse-geocoding (lat/lon -> place name).
	// It is nil if the embedded dataset failed to load; callers must guard.
	geocoder *geo.Geocoder

	// bus is the in-memory realtime event pub/sub backing the websocket sync
	// endpoint.
	bus *EventBus

	// jobStates tracks progress of background jobs keyed by job id.
	jobStates sync.Map

	// queuePaused tracks the paused flag for each named queue (keyed by
	// QueueName). Real queue backends would persist this; for the in-memory
	// bus a sync.Map is sufficient and the flag is honored by /queues and the
	// job runner.
	queuePaused sync.Map

	// maintenance holds the global maintenance-mode state (set via
	// POST /admin/maintenance). While active, mutating endpoints reject writes
	// with 503 so operators can safely restore/inspect the instance.
	maintenanceMu   sync.Mutex
	maintenanceMode bool
	maintenanceTask string

	// sioConnectAck tracks Socket.IO polling transports that have sent a
	// namespace-connect packet (40) over POST, so the next long-poll GET can
	// return the v4 connect ack (40{"sid":...}) the client expects. Keyed by
	// the Engine.IO sid; entries are short-lived (deleted once acked).
	sioConnectAck sync.Map

	// sioVersionPending tracks polling transports that still need the
	// on_server_version event delivered on their next long-poll GET (the
	// websocket transport emits it immediately on connect).
	sioVersionPending sync.Map
}

func NewApp(cfg *Config, store *Store) *App {
	networkOCR, err := ocr.BuildNetwork(cfg.OCRProvider, ocr.OpenAIConfig{
		BaseURL: cfg.OCRBaseURL,
		APIKey:  cfg.OCRAPIKey,
		Model:   cfg.OCRModel,
		Prompt:  cfg.OCRPrompt,
		Detail:  cfg.OCRDetail,
		Timeout: time.Duration(cfg.OCRTimeout) * time.Second,
	}, ocr.HTTPConfig{Endpoint: cfg.OCRBaseURL, Token: cfg.OCRAPIKey, Timeout: time.Duration(cfg.OCRTimeout) * time.Second})
	if err != nil {
		log.Printf("[ocr] network backend configuration error: %v", err)
	}
	var nativeOCR ocr.Processor
	if provider, loadErr := ocr.NewMacVision(cfg.OCRNativePath); loadErr == nil {
		nativeOCR = provider
		log.Printf("[ocr] using macOS native Vision backend")
	} else if cfg.OCRNativePath != "" {
		log.Printf("[ocr] native backend unavailable: %v", loadErr)
	}
	var communityOCR ocr.Processor
	if provider, loadErr := ocr.NewCommunity(ocr.CommunityConfig{Path: cfg.OCRCommunityPath}); loadErr == nil {
		communityOCR = provider
		log.Printf("[ocr] using community backend: %s", provider.Name())
	} else if cfg.OCRCommunityPath != "" {
		log.Printf("[ocr] community backend unavailable: %v", loadErr)
	}
	var mlBackend ml.Backend
	if cfg.OCRProvider == "openai-chat" || cfg.OCRProvider == "chat" || cfg.OCRProvider == "openai-responses" || cfg.OCRProvider == "responses" {
		responses := cfg.OCRProvider == "openai-responses" || cfg.OCRProvider == "responses"
		if backend, mlErr := ml.NewOpenAIVision(ml.OpenAIConfig{BaseURL: cfg.OCRBaseURL, APIKey: cfg.OCRAPIKey, Model: cfg.OCRModel, Timeout: time.Duration(cfg.OCRTimeout) * time.Second}, responses); mlErr == nil {
			mlBackend = backend
		}
	}
	a := &App{cfg: cfg, store: store, video: video.New(), ocr: ocr.NewChain(nativeOCR, networkOCR, communityOCR), ml: ml.NewChain(mlBackend)}
	// Resolve the effective JWT secret: prefer the per-instance value
	// persisted in SystemConfig; fall back to the configured secret only if
	// the row has none (should not happen after ensureJWTSecret).
	var sc SystemConfig
	if err := store.DB.First(&sc, "id = ?", "singleton").Error; err == nil && sc.JWTSecret != "" {
		a.jwtSecret = sc.JWTSecret
	} else {
		a.jwtSecret = cfg.JWTSecret
	}
	a.bus = newEventBus()
	// Initialize video cache (2 GB LRU).
	encDir := filepath.Join(cfg.ResourceDir, "encoded-video")
	if err := os.MkdirAll(encDir, 0o755); err == nil {
		a.videoCache = video.NewCacheManager(encDir, 2<<30)
	}
	if g, err := geo.Load(); err != nil {
		log.Printf("[geo] reverse-geocoder unavailable: %v", err)
	} else {
		a.geocoder = g
		log.Printf("[geo] reverse-geocoder loaded with %d cities", g.Cities())
	}
	a.startSchedulers()
	return a
}

// RegisterRoutes wires every Immich-compatible endpoint. Public endpoints
// (health, about, auth login) are registered outside the auth guard.
func (a *App) RegisterRoutes(r *gin.Engine) {
	// ---- public ----
	// RFC-style discovery endpoint the official iOS/Android clients hit first
	// (GET /.well-known/immich) to learn the API base path. Without it the
	// mobile apps POST /auth/login with no /api prefix and fall through to the
	// SPA fallback (HTML 200), so login never succeeds. The official response
	// shape is {"api":{"endpoint":"/api"}}.
	r.GET("/.well-known/immich", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"api": gin.H{"endpoint": "/api"}})
	})
	r.GET("/api/server/ping", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"res": "pong"}) })
	r.GET("/api/server/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok", "checks": []any{}}) })
	r.GET("/api/server/about", a.handleAbout)
	r.GET("/api/server/version", a.handleServerVersion)
	r.GET("/api/server/config", a.handleServerConfig)
	r.GET("/api/server/features", a.handleServerFeatures)
	// Per the official Immich v3.1.0 contract (open-api/immich-openapi-specs.json,
	// /server/media-types and /server/version-history carry no security scheme),
	// these are PUBLIC server-info endpoints. The official web calls them during
	// app bootstrap — often before the auth cookie exists — so gating them behind
	// AuthGuard returns 401 and the client logs "Failed to load supported media
	// types". They must be served without the auth guard, exactly like the
	// original server. DTO shapes are unchanged (already match the contract).
	r.GET("/api/server/media-types", a.handleServerMediaTypes)
	r.GET("/api/server/version-history", a.handleServerVersionHistory)

	// Root-path aliases (/server/* without the /api prefix) for clients such as
	// the official Immich iOS app that poll /server/version (and other
	// server-info endpoints) during bootstrap/version-check. Without these
	// aliases the request falls through to the SPA history-fallback
	// (main.go NoRoute) and returns HTML, which the client cannot parse as a
	// version and reports as "version incompatible" (see docs/todos/BUG-005).
	// Explicit routes here take precedence over NoRoute, so JSON is served.
	r.GET("/server/ping", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"res": "pong"}) })
	r.GET("/server/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok", "checks": []any{}}) })
	r.GET("/server/about", a.handleAbout)
	r.GET("/server/version", a.handleServerVersion)
	r.GET("/server/config", a.handleServerConfig)
	r.GET("/server/features", a.handleServerFeatures)
	r.GET("/server/media-types", a.handleServerMediaTypes)
	r.GET("/server/version-history", a.handleServerVersionHistory)
	r.GET("/server/version-check", a.handleServerVersionCheck)

	r.GET("/api/system-config/defaults", a.handleSystemConfigDefaults)
	r.GET("/api/auth/status", a.handleAuthStatus)
	r.POST("/api/auth/login", a.handleLogin)
	r.POST("/api/auth/signup", a.handleSignup)
	r.GET("/api/auth/check", func(c *gin.Context) {
		var cfg SystemConfig
		a.store.DB.First(&cfg, "id = ?", "singleton")
		c.JSON(http.StatusOK, gin.H{"authStatus": boolToStatus(cfg.LoginRequired)})
	})

	// public shared-link access (no auth): view + asset streaming
	r.GET("/api/share/:key", a.handleShareView)
	r.GET("/api/share/:key/thumbnail/:assetId", a.handleShareThumbnail)
	r.GET("/api/share/:key/original/:assetId", a.handleShareOriginal)

	// ---- authenticated ----
	api := r.Group("/api")
	api.Use(a.AuthGuard())
	{
		// auth
		api.GET("/auth/validate", a.handleValidate)
		api.POST("/auth/validateToken", a.handleAuthValidateToken)
		api.POST("/auth/change-password", a.handleChangePassword)
		api.POST("/auth/logout", a.handleLogout)
		api.POST("/auth/pin-code", a.handlePinCodeSetup)
		api.POST("/auth/session/lock", a.handleSessionLock)
		api.POST("/auth/session/unlock", a.handleSessionUnlock)

		// notifications (list / update / delete — single-user instance has no
		// generator, so these return the real, usually-empty state)
		api.GET("/notifications", a.handleListNotifications)
		api.PUT("/notifications", a.handleUpdateNotifications)
		api.DELETE("/notifications/:id", a.handleDeleteNotification)
		api.GET("/api-keys", a.handleApiKeys)
		api.POST("/api-keys", a.handleApiKeys)
		api.GET("/api-keys/me", a.handleApiKeys)
		api.GET("/api-keys/:id", a.handleApiKeys)
		api.PUT("/api-keys/:id", a.handleApiKeys)
		api.DELETE("/api-keys/:id", a.handleApiKeys)
		api.POST("/sessions", a.handleSessionCreate)

		// users
		api.GET("/users", a.handleListUsers)
		api.GET("/users/me", a.handleMe)
		api.PUT("/users/me", a.handleUpdateMe)
		api.GET("/user/me", a.handleMe)
		api.PUT("/user/me", a.handleUpdateMe)
		api.GET("/user/me/preferences", a.handlePreferences)
		api.PUT("/user/me/preferences", a.handlePreferences)
		api.GET("/users/me/preferences", a.handlePreferences)
		api.PUT("/users/me/preferences", a.handlePreferences)
		api.GET("/users/me/onboarding", a.handleUserOnboardingGet)
		api.PUT("/users/me/onboarding", a.handleUserOnboardingPost)
		api.GET("/users/me/license", a.handleUserLicenseGet)
		api.GET("/users/:id", a.handleGetUser)
		api.PUT("/users/:id", a.handleUpdateUser)
		api.DELETE("/users/:id", a.handleDeleteUser)
		api.GET("/users/:id/thumb", a.handleUserThumb)
		api.POST("/users/profile-image", a.handleProfileImageCreate)
		api.GET("/users/:id/profile-image", a.handleProfileImageGet)

		// admin user management
		api.GET("/admin/users", a.handleAdminListUsers)
		api.POST("/admin/users", a.handleAdminCreateUser)
		api.GET("/admin/users/:id", a.handleAdminGetUser)
		api.PUT("/admin/users/:id", a.handleAdminUpdateUser)
		api.DELETE("/admin/users/:id", a.handleAdminDeleteUser)
		api.POST("/admin/users/:id/restore", a.handleAdminRestoreUser)
		api.GET("/admin/users/:id/preferences", a.handleAdminUserPreferences)
		api.PUT("/admin/users/:id/preferences", a.handleAdminUserPreferences)
		api.GET("/admin/users/:id/calendar-heatmap", a.handleAdminUserCalendarHeatmap)
		api.GET("/admin/users/:id/sessions", a.handleAdminUserSessions)
		api.GET("/admin/users/:id/statistics", a.handleAdminUserStatistics)

		// assets
		api.POST("/assets", a.handleAssetUpload)
		api.POST("/assets/check", a.handleAssetCheck)
		api.POST("/assets/bulk-upload-check", a.handleAssetBulkUploadCheck)
		api.PUT("/assets", a.handleAssetBulkUpdate)
		api.GET("/assets", a.handleAssetSearch) // query-based listing
		api.GET("/assets/random", a.handleAssetRandom)
		api.GET("/assets/count", a.handleAssetCount)
		api.GET("/assets/statistics", a.handleAssetStatistics)
		api.POST("/assets/urls", a.handleAssetBulkInfo)
		api.GET("/assets/duplicates", a.handleAssetDuplicates)
		api.PUT("/assets/metadata", a.handleAssetBulkMetadata)
		api.PUT("/assets/copy", a.handleAssetCopy)
		api.GET("/assets/:id", a.handleAssetGet)
		api.PUT("/assets/:id", a.handleAssetUpdate)
		api.DELETE("/assets", a.handleAssetBulkDelete)
		api.GET("/assets/:id/edits", a.handleAssetEditsGet)
		api.PUT("/assets/:id/edits", a.handleAssetEditsUpdate)
		api.DELETE("/assets/:id/edits", a.handleAssetEditsDelete)
		api.GET("/assets/:id/original", a.handleAssetOriginal)
		api.GET("/assets/:id/original/download", a.handleAssetOriginalDownload)
		api.GET("/assets/:id/metadata", a.handleAssetMetadata)
		api.PUT("/assets/:id/metadata", a.handleAssetMetadataUpdate)
		api.GET("/assets/:id/thumbnail", a.handleAssetThumbnail)
		api.GET("/assets/:id/thumbnail/:ts", a.handleAssetThumbnail)
		api.GET("/assets/:id/preview", a.handleAssetPreview)
		api.GET("/assets/:id/encoded-video/:ts", a.handleAssetEncodedVideo)
		api.GET("/assets/:id/live-photo", a.handleAssetLivePhoto)

		// HLS video streaming (v3.1.0 client compatibility)
		api.GET("/assets/:id/video/playback", a.handleVideoPlayback)
		api.GET("/assets/:id/video/stream/main.m3u8", a.handleVideoStreamMaster)
		api.GET("/assets/:id/video/stream/:sessionId/:variantIndex/playlist.m3u8", a.handleVideoStreamPlaylist)
		api.GET("/assets/:id/video/stream/:sessionId/:variantIndex/:filename", a.handleVideoStreamSegment)
		api.DELETE("/assets/:id/video/stream/:sessionId", a.handleVideoStreamDelete)

		// albums
		api.GET("/albums", a.handleAlbumList)
		api.GET("/albums/statistics", a.handleAlbumStatistics)
		api.POST("/albums", a.handleAlbumCreate)
		api.GET("/albums/:id", a.handleAlbumGet)
		api.GET("/albums/:id/assets", a.handleAlbumAssets)
		api.PUT("/albums/:id", a.handleAlbumUpdate)
		api.DELETE("/albums/:id", a.handleAlbumDelete)
		api.POST("/albums/:id/assets", a.handleAlbumAddAssets)
		api.PUT("/albums/:id/assets", a.handleAlbumAddAssets)
		api.DELETE("/albums/:id/assets", a.handleAlbumRemoveAssets)
		api.PATCH("/albums/:id/assets", a.handleAlbumUpdateAssets)
		api.PUT("/albums/:id/cover", a.handleAlbumSetCover)
		api.GET("/albums/:id/map-markers", a.handleAlbumMapMarkers)
		api.PUT("/albums/:id/users", a.handleAlbumSetUsers)
		api.PUT("/albums/:id/user/:userId", a.handleAlbumAddUser)
		api.DELETE("/albums/:id/user/:userId", a.handleAlbumRemoveUser)
		api.PUT("/albums/assets", a.handleAlbumBulkAddAssets)

		// libraries
		api.GET("/libraries", a.handleLibraryList)
		api.POST("/libraries", a.handleLibraryCreate)
		api.GET("/libraries/:id", a.handleLibraryGet)
		api.PUT("/libraries/:id", a.handleLibraryUpdate)
		api.DELETE("/libraries/:id", a.handleLibraryDelete)
		api.GET("/libraries/:id/statistics", a.handleLibraryStats)
		api.POST("/libraries/:id/scan", a.handleLibraryScan)
		api.POST("/libraries/:id/validate", a.handleLibraryValidate)

		// timeline
		api.GET("/timeline/buckets", a.handleTimelineBuckets)
		api.GET("/timeline/bucket", a.handleTimelineBucketAssets)
		api.GET("/timeline/assets", a.handleTimelineBucketAssets)

		// map (geo-tagged assets)
		api.GET("/map/markers", a.handleMapMarkers)
		api.POST("/map/reverse-geocode", a.handleMapReverseGeocode)

		// search
		api.POST("/search", a.handleSearch)
		api.POST("/search/person", a.handleSearchPerson)
		api.POST("/search/metadata", a.handleSearchMetadata)
		api.POST("/search/suggestions", a.handleSearchSuggestions)
		api.GET("/search/explore", a.handleSearchExplore)
		api.POST("/search/random", a.handleSearchRandom)
		api.POST("/search/large-assets", a.handleSearchLargeAssets)
		api.POST("/search/statistics", a.handleSearchStatistics)
		api.POST("/search/smart", a.handleSearchSmart)

		// memories (on-this-day) + notifications + oauth config
		api.GET("/memories", a.handleMemories)
		api.POST("/notifications", a.handleNotificationRegister)
		api.DELETE("/notifications", a.handleNotificationRemove)
		api.GET("/oauth/config", a.handleOAuthConfig)

		// tags
		api.GET("/tags", a.handleTagList)
		api.POST("/tags", a.handleTagCreate)
		api.PUT("/tags", a.handleTagBulkUpdate)
		api.GET("/tags/:id", a.handleTagGet)
		api.PUT("/tags/:id", a.handleTagUpdate)
		api.DELETE("/tags/:id", a.handleTagDelete)
		api.POST("/tags/:id/assets", a.handleTagAddAssets)
		api.PUT("/tags/:id/assets", a.handleTagAddAssets)
		api.DELETE("/tags/:id/assets", a.handleTagRemoveAssets)
		api.DELETE("/tags/:id/assets/:assetId", a.handleTagRemoveAsset)
		api.PUT("/tags/assets", a.handleTagBulkAssets)

		// partners
		api.GET("/partners", a.handlePartnerList)
		api.POST("/partners", a.handlePartnerCreate)
		api.PUT("/partners/:id", a.handlePartnerUpdate)
		api.DELETE("/partners/:id", a.handlePartnerDelete)

		// trash
		api.GET("/trash", a.handleTrashList)
		api.POST("/trash/restore", a.handleTrashRestore)
		api.POST("/trash/empty", a.handleTrashEmpty)
		api.POST("/trash/cleanup", a.handleTrashCleanup)

		// activity
		api.GET("/activities", a.handleActivityList)
		api.POST("/activities", a.handleActivityCreate)
		api.DELETE("/activities/:id", a.handleActivityDelete)
		api.GET("/activities/asset/:id", a.handleActivityByAsset)
		api.GET("/activities/album/:id", a.handleActivityByAlbum)
		api.GET("/activities/statistics", a.handleActivityStatistics)

		// folder view (web UI)
		api.GET("/view/folder", a.handleViewFolder)
		api.GET("/view/folder/unique-paths", a.handleViewFolderUniquePaths)

		// shared links
		api.GET("/shared-links", a.handleSharedLinkList)
		api.POST("/shared-links", a.handleSharedLinkCreate)
		api.GET("/shared-links/:id", a.handleSharedLinkGet)
		api.PUT("/shared-links/:id", a.handleSharedLinkUpdate)
		api.DELETE("/shared-links/:id", a.handleSharedLinkDelete)

		// stacks (manual grouping; no ML)
		api.POST("/stacks", a.handleStackCreate)
		api.DELETE("/stacks", a.handleStackDelete)
		api.GET("/stacks/:id", a.handleStackGet)

		// people (real; ML face detection is deferred)
		api.GET("/people", a.handlePeopleList)
		api.GET("/people/:id", a.handlePersonGet)
		api.GET("/people/:id/assets", a.handlePersonAssets)
		api.POST("/people", a.handlePersonCreate)
		api.PUT("/people", a.handlePeopleUpdateMany)
		api.PUT("/people/:id", a.handlePersonUpdate)
		api.DELETE("/people/:id", a.handlePersonDelete)
		api.DELETE("/people", a.handlePeopleDeleteMany)

		// system config / jobs
		api.GET("/system-config", a.handleSystemConfigGet)
		api.PUT("/system-config", a.handleSystemConfigUpdate)
		api.GET("/system-config/storage-template-options", a.handleStorageTemplateOptions)
		api.GET("/system-metadata/reverse-geocoding-state", a.handleReverseGeocodingState)
		api.GET("/system-metadata/version-check-state", a.handleVersionCheckState)
		api.GET("/system-metadata/admin-onboarding", a.handleAdminOnboardingGet)
		api.POST("/system-metadata/admin-onboarding", a.handleAdminOnboardingPost)
		api.GET("/server/statistics", a.handleServerStatistics)
		api.GET("/jobs", a.handleJobsList)
		api.POST("/jobs", a.handleJobCreate)
		api.POST("/jobs/:id", a.handleJobCommand)
		api.GET("/jobs/:id", a.handleJobStatus)
		api.GET("/queues", a.handleQueuesList)
		api.GET("/queues/:name", a.handleQueueGet)
		api.PUT("/queues/:name", a.handleQueueUpdate)
		api.GET("/queues/:name/jobs", a.handleQueueJobs)
		api.DELETE("/queues/:name/jobs", a.handleQueueEmpty)

		// maintenance + integrity (admin)
		api.POST("/admin/maintenance", a.handleMaintenanceSet)
		api.GET("/admin/maintenance/status", a.handleMaintenanceStatus)
		api.POST("/admin/maintenance/login", a.handleMaintenanceLogin)
		api.GET("/admin/maintenance/detect-install", a.handleMaintenanceDetectInstall)
		api.GET("/admin/integrity/summary", a.handleIntegritySummary)
		api.GET("/admin/integrity/:type", a.handleIntegrityReport)
		api.DELETE("/admin/integrity/:id", a.handleIntegrityReportDelete)
		api.GET("/admin/integrity/file/:id", a.handleIntegrityReportFile)
		api.GET("/admin/integrity/csv", a.handleIntegrityReportCsv)
		api.GET("/admin/database-backups", a.handleDatabaseBackupsList)
		api.POST("/admin/database-backups/start-restore", a.handleDatabaseBackupRestore)
		api.DELETE("/admin/database-backups", a.handleDatabaseBackupDelete)
		api.POST("/admin/database-backups/upload", a.handleDatabaseBackupUpload)
		api.POST("/admin/notifications", a.handleAdminNotificationCreate)
		api.POST("/admin/notifications/test-email", a.handleAdminTestEmail)

		// realtime sync (websocket)
		api.GET("/events", a.handleEventsWS)

		// Socket.IO (Engine.IO v4) for official Immich clients
		api.GET("/socket.io", a.handleSocketIO)
		api.GET("/socket.io/", a.handleSocketIO)
		api.POST("/socket.io", a.handleSocketIO)
		api.POST("/socket.io/", a.handleSocketIO)

		// v3.1.0 compatibility hardening (method aliases + startup info +
		// graceful ML/sync stubs). See internal/app/compat_v3.go.
		api.PATCH("/albums/:id", a.handleAlbumUpdate)
		api.PATCH("/shared-links/:id", a.handleSharedLinkUpdate)
		api.GET("/map/reverse-geocode", a.handleMapReverseGeocode)
		api.GET("/search/suggestions", a.handleSearchSuggestions)
		api.GET("/server/version-check", a.handleServerVersionCheck)
		api.GET("/server/storage", a.handleServerStorage)
		api.GET("/server/info", a.handleServerInfo)
		api.GET("/server/apk-links", a.handleServerApkLinks)
		api.GET("/server/license", a.handleServerLicense)
		api.PUT("/server/license", a.handleServerLicense)
		api.DELETE("/server/license", a.handleServerLicense)
		api.GET("/sync/ack", a.handleSyncAck)
		api.POST("/sync/ack", a.handleSyncAck)
		api.DELETE("/sync/ack", a.handleSyncAck)
		api.POST("/sync/stream", a.handleSyncStream)
		api.GET("/sync/stream", a.handleSyncStream)
		api.GET("/people/:id/statistics", a.handlePersonStatistics)
		// faces: detection/recognition need an ML backend (deferred) -> 501.
		api.GET("/faces", a.handleFacesList)
		api.GET("/faces/:id", a.handleFaceGet)
		api.POST("/faces", a.faceNotImplemented)
		api.PUT("/faces/:id", a.faceNotImplemented)
		api.DELETE("/faces/:id", a.faceNotImplemented)
		api.POST("/people/:id/merge", a.handlePersonMerge)
		api.PUT("/people/:id/reassign", a.handlePersonReassign)
		api.GET("/search/cities", a.handleSearchCities)
		api.GET("/search/places", a.handleSearchPlaces)
		api.POST("/download/info", a.handleDownloadInfo)
		api.POST("/trash/restore/assets", a.handleTrashRestore)
		api.GET("/duplicates", a.handleAssetDuplicates)
		api.POST("/duplicates/resolve", a.handleDuplicatesResolve)
		api.PUT("/duplicates/:id", a.handleDuplicatesUpdate)
		api.DELETE("/duplicates/:id", a.handleDuplicatesUpdate)
		api.DELETE("/duplicates", a.handleDuplicatesUpdate)

		// download (basic)
		api.GET("/download/archive", a.handleDownloadArchive)
		api.POST("/download/archive", a.handleDownloadArchive)
	}
}

func boolToStatus(b bool) string {
	if b {
		return "required"
	}
	return "disabled"
}
