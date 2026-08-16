package app

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// compat.go adds Immich-compatible endpoints that the official web/mobile
// clients expect on first load. These are lightly-mounted so the Go port can
// act as a drop-in for the core library experience.

// handleAuthStatus reports the caller's auth state without requiring a body.
// Immich returns one of: authorized | anonymous | locked | password-recheck.
func (a *App) handleAuthStatus(c *gin.Context) {
	h := c.GetHeader("Authorization")
	if len(h) > 7 && h[:7] == "Bearer " {
		if _, err := a.parseToken(h[7:]); err == nil {
			c.JSON(http.StatusOK, gin.H{"authStatus": "authorized"})
			return
		}
	}
	if a.cfg.LoginRequired {
		c.JSON(http.StatusOK, gin.H{"authStatus": "unauthorized"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"authStatus": "anonymous"})
}

// handleAuthValidateToken re-validates a token (POST, body ignored).
func (a *App) handleAuthValidateToken(c *gin.Context) {
	h := c.GetHeader("Authorization")
	if len(h) > 7 && h[:7] == "Bearer " {
		if _, err := a.parseToken(h[7:]); err == nil {
			c.Status(http.StatusNoContent)
			return
		}
	}
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "statusCode": 401})
}

// handleServerConfig mirrors GET /api/server/config.
// Field set follows the current Immich ServerConfigDto (all required keys
// present). Extra keys the official clients also read are kept for safety.
func (a *App) handleServerConfig(c *gin.Context) {
	var n int64
	a.store.DB.Model(&User{}).Count(&n)
	c.JSON(http.StatusOK, gin.H{
		// required by ServerConfigDto
		"externalDomain":   a.cfg.ExternalDomain,
		"isInitialized":    n > 0,
		"isOnboarded":      true,
		"loginPageMessage": "",
		"maintenanceMode":  false,
		"mapDarkStyleUrl":  "",
		"mapLightStyleUrl": "",
		"minFaces":         1,
		"oauthButtonText":  "",
		"publicUsers":      true,
		"trashDays":        a.cfg.TrashDays,
		"userDeleteDelay":  0,
		// extra keys official clients also read
		"isConnected":                true,
		"isReadOnly":                 false,
		"isPasswordLoginEnabled":     true,
		"isOauthEnabled":             false,
		"isOauthAutoLaunch":          false,
		"isFirstUser":                n == 0,
		"isInMemory":                 false,
		"isCoreDown":                 false,
		"isLogInWithPasskeyEnabled":  false,
		"isSharedLinkLoginEnabled":   true,
		"isSidebarSharedLinkEnabled": true,
		"isLibraryWipeEnabled":       false,
		"isDownloadAvailable":        true,
	})
}

// handleServerVersion mirrors GET /api/server/version.
// Required keys per ServerVersionResponseDto: major, minor, patch, prerelease.
func (a *App) handleServerVersion(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"major":      a.cfg.CompatMajor,
		"minor":      a.cfg.CompatMinor,
		"patch":      a.cfg.CompatPatch,
		"prerelease": 0,
		"version":    a.cfg.CompatVersion,
	})
}

// handleServerFeatures mirrors GET /api/server/features.
// Field set + required keys follow the current Immich ServerFeaturesDto.
// Booleans reflect what immich-go actually implements so the official clients
// show the correct UI (e.g. map/duplicateDetection must be true or the tabs
// are hidden).
func (a *App) handleServerFeatures(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"configFile":          false,
		"duplicateDetection":  true,
		"email":               false,
		"facialRecognition":   false,
		"importFaces":         false,
		"map":                 true,
		"oauth":               false,
		"oauthAutoLaunch":     false,
		"ocr":                 false,
		"passwordLogin":       true,
		"realtimeTranscoding": true,
		"reverseGeocoding":    true,
		"search":              true,
		"sidecar":             false,
		"smartSearch":         false,
		"trash":               true,
	})
}

// handleAssetStatistics mirrors GET /api/assets/statistics.
func (a *App) handleAssetStatistics(c *gin.Context) {
	uid := currentUserID(c)
	var images, videos, total int64
	a.store.DB.Model(&Asset{}).Where("owner_id = ? AND is_trash = ?", uid, false).Count(&total)
	a.store.DB.Model(&Asset{}).Where("owner_id = ? AND is_trash = ? AND type = ?", uid, false, "IMAGE").Count(&images)
	a.store.DB.Model(&Asset{}).Where("owner_id = ? AND is_trash = ? AND type = ?", uid, false, "VIDEO").Count(&videos)
	c.JSON(http.StatusOK, gin.H{"images": images, "videos": videos, "total": total})
}

// userStatsEntry is the per-user usage shape the official web expects
// (UsageByUserDto). QuotaSizeInBytes is a *int64 so it serializes to null when
// a user has no quota (matches the web's `quotaSizeInBytes !== null` check).
type userStatsEntry struct {
	UserID           string `json:"userId"`
	UserName         string `json:"userName"`
	Photos           int64  `json:"photos"`
	Videos           int64  `json:"videos"`
	Usage            int64  `json:"usage"`
	UsagePhotos      int64  `json:"usagePhotos"`
	UsageVideos      int64  `json:"usageVideos"`
	QuotaSizeInBytes *int64 `json:"quotaSizeInBytes"`
}

// handleServerStatistics mirrors GET /api/server/statistics. It returns the
// ServerStatsResponseDto the official web consumes (photo/video counts plus
// storage usage in bytes, both as totals and per user). The web reads `usage`
// as a byte count (passes it to getBytesWithUnit) and iterates `usageByUser`,
// so all of these fields MUST be present and numeric — returning `usage` as a
// counts object (the old behavior) broke the admin "Server Status" page.
// See web/src/routes/admin/server-status/ServerStatisticsPanel.svelte.
func (a *App) handleServerStatistics(c *gin.Context) {
	uid := currentUserID(c)
	if !a.isAdmin(uid) {
		c.Status(http.StatusForbidden)
		return
	}

	// Seed a per-user accumulator for every known user (so users with zero
	// assets still appear with zeros rather than being omitted).
	byUser := map[string]*userStatsEntry{}
	var users []User
	a.store.DB.Find(&users)
	for i := range users {
		byUser[users[i].ID] = &userStatsEntry{
			UserID:           users[i].ID,
			UserName:         users[i].Name,
			QuotaSizeInBytes: users[i].QuotaSizeInBytes,
		}
	}

	var assets []Asset
	a.store.DB.Where("is_trash = ?", false).Find(&assets)

	var photos, videos int64
	var usage, usagePhotos, usageVideos int64
	for _, as := range assets {
		sz := as.Size // original file size in bytes (set at ingest)
		usage += sz
		e, ok := byUser[as.OwnerID]
		if !ok {
			e = &userStatsEntry{UserID: as.OwnerID, QuotaSizeInBytes: nil}
			byUser[as.OwnerID] = e
		}
		e.Usage += sz
		switch as.Type {
		case "IMAGE":
			photos++
			e.Photos++
			usagePhotos += sz
			e.UsagePhotos += sz
		case "VIDEO":
			videos++
			e.Videos++
			usageVideos += sz
			e.UsageVideos += sz
		}
	}

	list := make([]userStatsEntry, 0, len(byUser))
	for _, e := range byUser {
		list = append(list, *e)
	}

	c.JSON(http.StatusOK, gin.H{
		"photos":      photos,
		"videos":      videos,
		"usage":       usage,
		"usagePhotos": usagePhotos,
		"usageVideos": usageVideos,
		"usageByUser": list,
	})
}

// handlePersonAssets mirrors GET /api/people/:id/assets; returns the assets
// actually linked to the person (asset.person_id).
func (a *App) handlePersonAssets(c *gin.Context) {
	id := c.Param("id")
	uid := currentUserID(c)
	var assets []Asset
	a.store.DB.Where("person_id = ? AND owner_id = ? AND is_trash = ?", id, uid, false).Find(&assets)
	out := make([]AssetResponse, 0, len(assets))
	for _, as := range assets {
		out = append(out, a.toResponse(as))
	}
	c.JSON(http.StatusOK, gin.H{"assets": out, "count": len(out), "total": len(out)})
}

// handleAlbumStatistics mirrors GET /api/albums/statistics.
func (a *App) handleAlbumStatistics(c *gin.Context) {
	uid := currentUserID(c)
	var owned, shared, notShared int64
	a.store.DB.Model(&Album{}).Where("owner_id = ?", uid).Count(&owned)
	a.store.DB.Model(&Album{}).Where("owner_id <> ?", uid).Count(&shared)
	a.store.DB.Model(&Album{}).Where("owner_id = ? AND (select count(*) from albums_assets_assets where albums_assets_assets.albums_id = albums.id) = 0", uid).Count(&notShared)
	c.JSON(http.StatusOK, gin.H{"count": owned + shared, "owned": owned, "shared": shared, "notShared": notShared})
}

// handleSearchSuggestions mirrors POST /api/search/suggestions.
func (a *App) handleSearchSuggestions(c *gin.Context) {
	uid := currentUserID(c)
	var q struct {
		Q string `json:"q"`
	}
	_ = c.ShouldBindJSON(&q)
	like := "%" + q.Q + "%"
	var albums []Album
	a.store.DB.Where("owner_id = ? AND album_name LIKE ?", uid, like).Limit(10).Find(&albums)
	out := gin.H{
		"albums":       albums,
		"people":       []any{},
		"recentAssets": []any{},
		"locations":    []any{},
		"tags":         []any{},
	}
	c.JSON(http.StatusOK, out)
}

// handleSystemConfigDefaults mirrors GET /api/system-config/defaults.
func (a *App) handleSystemConfigDefaults(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"loginPageMessage":        "",
		"trashDays":               a.cfg.TrashDays,
		"isSavedPhotosHidden":     false,
		"isEmailEnabled":          false,
		"isOauthAutoLaunch":       false,
		"storageTemplate":         "{{y}}/{{yyyy}}/{{MM}}-{{dd}}/{{filename}}",
		"theme":                   "system",
		"isPublicUsersEnabled":    false,
		"isSingleUserMode":        false,
		"isSingleUserModeAllowed": false,
	})
}
