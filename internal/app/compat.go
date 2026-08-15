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
		"externalDomain":     a.cfg.ExternalDomain,
		"isInitialized":      n > 0,
		"isOnboarded":        true,
		"loginPageMessage":   "",
		"maintenanceMode":    false,
		"mapDarkStyleUrl":    "",
		"mapLightStyleUrl":   "",
		"minFaces":           1,
		"oauthButtonText":    "",
		"publicUsers":        true,
		"trashDays":          30,
		"userDeleteDelay":    0,
		// extra keys official clients also read
		"isConnected":                 true,
		"isReadOnly":                  false,
		"isPasswordLoginEnabled":      true,
		"isOauthEnabled":              false,
		"isOauthAutoLaunch":           false,
		"isFirstUser":                 n == 0,
		"isInMemory":                  false,
		"isCoreDown":                  false,
		"isLogInWithPasskeyEnabled":   false,
		"isSharedLinkLoginEnabled":    true,
		"isSidebarSharedLinkEnabled":  true,
		"isLibraryWipeEnabled":        false,
		"isDownloadAvailable":         true,
	})
}

// handleServerVersion mirrors GET /api/server/version.
// Required keys per ServerVersionResponseDto: major, minor, patch, prerelease.
func (a *App) handleServerVersion(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"major":       a.cfg.CompatMajor,
		"minor":       a.cfg.CompatMinor,
		"patch":       a.cfg.CompatPatch,
		"prerelease":  0,
		"version":     a.cfg.CompatVersion,
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
		"reverseGeocoding":    false,
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
		"albums":      albums,
		"people":      []any{},
		"recentAssets": []any{},
		"locations":   []any{},
		"tags":        []any{},
	}
	c.JSON(http.StatusOK, out)
}

// handleSystemConfigDefaults mirrors GET /api/system-config/defaults.
func (a *App) handleSystemConfigDefaults(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"loginPageMessage":         "",
		"trashDays":                30,
		"isSavedPhotosHidden":      false,
		"isEmailEnabled":           false,
		"isOauthAutoLaunch":        false,
		"storageTemplate":          "{{y}}/{{yyyy}}/{{MM}}-{{dd}}/{{filename}}",
		"theme":                    "system",
		"isPublicUsersEnabled":     false,
		"isSingleUserMode":         false,
		"isSingleUserModeAllowed":  false,
	})
}
