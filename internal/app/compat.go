package app

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// compat.go adds Immich-compatible endpoints that the official web/mobile
// clients expect on first load. These are lightly-mounted so the Go port can
// act as a drop-in for the core library experience.

// handleAuthStatus implements GET /api/auth/status. The live v3.1.0 server
// returns the AuthStatusResponseDto {isElevated, password, pinCode,
// pinExpiresAt} — NOT the legacy `{authStatus: "authorized"}` string, which
// the current official web does not understand.
func (a *App) handleAuthStatus(c *gin.Context) {
	pin := false
	if tok := requestToken(c); tok != "" {
		if claims, err := a.parseToken(tok); err == nil {
			var u User
			if a.store.DB.First(&u, "id = ?", claims.UserID).Error == nil && u.PinCode != "" {
				pin = true
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"isElevated":   false,
		"password":     true,
		"pinCode":      pin,
		"pinExpiresAt": nil,
	})
}

// handleAuthValidateToken re-validates the caller's session. This matches the
// official v3.1.0 contract exactly: the endpoint is an @Authenticated route
// (401 when no valid token from any source) and returns HTTP 200 with the
// ValidateAccessTokenResponseDto body {authStatus:true}. The route is
// registered inside the authenticated group (see app.go), so the token is
// already resolved by AuthGuard from the cookie / Bearer / x-api-key sources
// the official Android client uses (native OkHttp cookie jar sends
// `immich_access_token`).
func (a *App) handleAuthValidateToken(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"authStatus": true})
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
		"maintenanceMode":  a.inMaintenance(),
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
		"configFile":         false,
		"duplicateDetection": true,
		"email":              false,
		"facialRecognition":  false,
		"importFaces":        false,
		"map":                true,
		"oauth":              false,
		"oauthAutoLaunch":    false,
		"ocr":                a.ocr != nil && a.ocr.Name() != "ocr-chain(empty)",
		"passwordLogin":      true,
		// realtimeTranscoding:false is HONEST. The official Immich web uses
		// hls.js when this flag is true and expects the real HLS contract: an
		// fMP4 segmented stream (init.mp4 + seg_N.m4s, EXT-X-MAP, VERSION 7),
		// which hls.js demuxes via MSE. Serving a single whole-MP4 as a media
		// segment makes hls.js allow it to fail (levelParsingError / the
		// segment is probed as MPEG-TS and aborts playback). Our in-process
		// transcoder produces one MP4, so we advertise false and let the
		// official web fall back to GET /api/assets/:id/video/playback, which
		// streams that real transcoded MP4 natively. See docs/GAP_ANALYSIS.md.
		"realtimeTranscoding": false,
		"reverseGeocoding":    true,
		"search":              true,
		"sidecar":             false,
		"smartSearch":         a.ml != nil && len(a.ml.Capabilities()) > 0,
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

	// Safety net: expose the same disk fields the web reads from
	// /api/server/storage (diskSizeRaw / diskUseRaw) on the statistics
	// response too, so any consumer keyed to /api/server/statistics still
	// sees finite, real disk numbers instead of a missing/NaN value.
	total, _, used, derr := diskUsage(a.cfg.ResourceDir)
	var diskSizeRaw, diskUseRaw int64
	if derr == nil {
		diskSizeRaw = int64(total)
		diskUseRaw = int64(used)
	}

	c.JSON(http.StatusOK, gin.H{
		"photos":      photos,
		"videos":      videos,
		"usage":       usage,
		"usagePhotos": usagePhotos,
		"usageVideos": usageVideos,
		"usageByUser": list,
		"diskSizeRaw": diskSizeRaw,
		"diskUseRaw":  diskUseRaw,
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

// handleAlbumStatistics mirrors GET /api/albums/statistics: owned = albums I
// own; shared = my albums that are shared (member or link); notShared = my
// albums with no sharing at all. (Official AlbumStatisticsResponseDto.)
func (a *App) handleAlbumStatistics(c *gin.Context) {
	uid := currentUserID(c)
	var owned, shared, notShared int64
	a.store.DB.Model(&Album{}).Where("owner_id = ?", uid).Count(&owned)
	a.store.DB.Model(&Album{}).Where("owner_id = ? AND (EXISTS "+
		"(SELECT 1 FROM albums_users_album au WHERE au.album_id = albums.id AND au.user_id <> albums.owner_id)"+
		" OR EXISTS (SELECT 1 FROM shared_links sl WHERE sl.album_id = albums.id))", uid).Count(&shared)
	notShared = owned - shared
	c.JSON(http.StatusOK, gin.H{"owned": owned, "shared": shared, "notShared": notShared})
}

// handleSearchSuggestions mirrors POST /api/search/suggestions.
func (a *App) handleSearchSuggestions(c *gin.Context) {
	if c.Request.Method == http.MethodGet {
		column := map[string]string{"country": "country", "state": "state", "city": "city", "cameraMake": "make", "cameraModel": "model"}[c.Query("type")]
		if column == "" {
			c.JSON(http.StatusOK, []string{})
			return
		}
		var values []string
		query := a.store.DB.Model(&Exif{}).Where(column + " <> ''")
		if c.Query("type") == "cameraModel" && c.Query("make") != "" {
			query = query.Where("make = ?", c.Query("make"))
		}
		if c.Query("country") != "" {
			query = query.Where("country = ?", c.Query("country"))
		}
		if c.Query("state") != "" {
			query = query.Where("state = ?", c.Query("state"))
		}
		query.Distinct().Order(column+" ASC").Pluck(column, &values)
		c.JSON(http.StatusOK, values)
		return
	}
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

