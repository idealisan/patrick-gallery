package app

import (
	"log"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"immich-go/internal/app/geo"
	"immich-go/internal/video"
)

// App holds shared dependencies for all HTTP handlers.
type App struct {
	cfg   *Config
	store *Store
	video video.Processor

	// geocoder provides offline reverse-geocoding (lat/lon -> place name).
	// It is nil if the embedded dataset failed to load; callers must guard.
	geocoder *geo.Geocoder

	// bus is the in-memory realtime event pub/sub backing the websocket sync
	// endpoint.
	bus *EventBus

	// jobStates tracks progress of background jobs keyed by job id.
	jobStates sync.Map
}

func NewApp(cfg *Config, store *Store) *App {
	a := &App{cfg: cfg, store: store, video: video.New()}
	a.bus = newEventBus()
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
	r.GET("/api/server/ping", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"res": "pong"}) })
	r.GET("/api/server/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok", "checks": []any{}}) })
	r.GET("/api/server/about", a.handleAbout)
	r.GET("/api/server/version", a.handleServerVersion)
	r.GET("/api/server/config", a.handleServerConfig)
	r.GET("/api/server/features", a.handleServerFeatures)
	r.GET("/api/system-config/defaults", a.handleSystemConfigDefaults)
	r.GET("/api/auth/status", a.handleAuthStatus)
	r.POST("/api/auth/validateToken", a.handleAuthValidateToken)
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
		api.POST("/auth/change-password", a.handleChangePassword)
		api.POST("/auth/logout", a.handleLogout)
		api.GET("/api-keys", a.handleApiKeys)
		api.POST("/api-keys", a.handleApiKeys)
		api.DELETE("/api-keys/:id", a.handleApiKeys)

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
		api.GET("/users/:id", a.handleGetUser)
		api.PUT("/users/:id", a.handleUpdateUser)
		api.DELETE("/users/:id", a.handleDeleteUser)
		api.GET("/users/:id/thumb", a.handleUserThumb)

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
		api.GET("/assets/:id", a.handleAssetGet)
		api.PUT("/assets/:id", a.handleAssetUpdate)
		api.DELETE("/assets", a.handleAssetBulkDelete)
		api.GET("/assets/:id/original", a.handleAssetOriginal)
		api.GET("/assets/:id/original/download", a.handleAssetOriginalDownload)
		api.GET("/assets/:id/metadata", a.handleAssetMetadata)
		api.GET("/assets/:id/thumbnail", a.handleAssetThumbnail)
		api.GET("/assets/:id/thumbnail/:ts", a.handleAssetThumbnail)
		api.GET("/assets/:id/preview", a.handleAssetPreview)
		api.GET("/assets/:id/encoded-video/:ts", a.handleAssetEncodedVideo)
		api.GET("/assets/:id/live-photo", a.handleAssetLivePhoto)

		// albums
		api.GET("/albums", a.handleAlbumList)
		api.GET("/albums/statistics", a.handleAlbumStatistics)
		api.POST("/albums", a.handleAlbumCreate)
		api.GET("/albums/:id", a.handleAlbumGet)
		api.GET("/albums/:id/assets", a.handleAlbumAssets)
		api.PUT("/albums/:id", a.handleAlbumUpdate)
		api.DELETE("/albums/:id", a.handleAlbumDelete)
		api.POST("/albums/:id/assets", a.handleAlbumAddAssets)
		api.DELETE("/albums/:id/assets", a.handleAlbumRemoveAssets)
		api.PATCH("/albums/:id/assets", a.handleAlbumUpdateAssets)
		api.PUT("/albums/:id/cover", a.handleAlbumSetCover)

		// libraries
		api.GET("/libraries", a.handleLibraryList)
		api.POST("/libraries", a.handleLibraryCreate)
		api.GET("/libraries/:id", a.handleLibraryGet)
		api.PUT("/libraries/:id", a.handleLibraryUpdate)
		api.DELETE("/libraries/:id", a.handleLibraryDelete)
		api.GET("/libraries/:id/statistics", a.handleLibraryStats)
		api.POST("/libraries/:id/scan", a.handleLibraryScan)

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

		// tags
		api.GET("/tags", a.handleTagList)
		api.POST("/tags", a.handleTagCreate)
		api.GET("/tags/:id", a.handleTagGet)
		api.PUT("/tags/:id", a.handleTagUpdate)
		api.DELETE("/tags/:id", a.handleTagDelete)
		api.POST("/tags/:id/assets", a.handleTagAddAssets)
		api.DELETE("/tags/:id/assets/:assetId", a.handleTagRemoveAsset)

		// partners
		api.GET("/partners", a.handlePartnerList)
		api.POST("/partners", a.handlePartnerCreate)
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

		// shared links
		api.GET("/shared-links", a.handleSharedLinkList)
		api.POST("/shared-links", a.handleSharedLinkCreate)
		api.PUT("/shared-links/:id", a.handleSharedLinkUpdate)
		api.DELETE("/shared-links/:id", a.handleSharedLinkDelete)

		// people (stub)
		api.GET("/people", a.handlePeopleList)
		api.GET("/people/:id", a.handlePersonGet)
		api.GET("/people/:id/assets", a.handlePersonAssets)

		// system config / jobs
		api.GET("/system-config", a.handleSystemConfigGet)
		api.PUT("/system-config", a.handleSystemConfigUpdate)
		api.GET("/server/statistics", a.handleServerStatistics)
		api.GET("/jobs", a.handleJobsList)
		api.POST("/jobs/:id", a.handleJobCommand)
		api.GET("/jobs/:id", a.handleJobStatus)

		// realtime sync (websocket)
		api.GET("/events", a.handleEventsWS)

		// Socket.IO (Engine.IO v4) for official Immich clients
		api.GET("/socket.io", a.handleSocketIO)
		api.GET("/socket.io/", a.handleSocketIO)
		api.POST("/socket.io", a.handleSocketIO)
		api.POST("/socket.io/", a.handleSocketIO)

		// download (basic)
		api.GET("/download/archive", a.handleDownloadArchive)
	}
}

func boolToStatus(b bool) string {
	if b {
		return "required"
	}
	return "disabled"
}
