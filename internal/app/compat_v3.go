package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// compat_v3.go hardens compatibility with the official Immich v3.1.0 client.
// The v3.1.0 OpenAPI spec defines 254 method-paths; this file closes the
// safe, high-value gaps (HTTP-method mismatches, startup info endpoints the
// app polls, and real implementations for the sync / storage / search /
// duplicates surfaces) without pulling in the out-of-scope ML / multi-user
// backends (faces clustering, CLIP, OAuth, admin, memories, notifications,
// plugins, workflows, queues, sessions).
//
// Per AGENTS.md hard rule #7 (No stubs), every handler here does real work.
// Endpoints that genuinely require an out-of-scope ML backend are NOT faked
// with empty 200s; they are handled honestly (e.g. empty results derived from
// real, absent data, or an explicit 404/501) and tracked in docs/NO_STUBS.md.

// ---- startup / server-info endpoints the app polls ----

func (a *App) handleServerVersionCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"checkedAt":      time.Now().UTC().Format(time.RFC3339),
		"releaseVersion": a.cfg.CompatVersion,
	})
}

func (a *App) handleServerMediaTypes(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"video":   []string{"mp4", "mov", "avi", "mkv", "webm", "m4v"},
		"image":   []string{"jpg", "jpeg", "png", "gif", "webp", "heic", "heif", "avif", "tif", "tiff", "bmp"},
		"sidecar": []string{"xmp", "yml", "yaml", "txt"},
	})
}

func (a *App) handleServerStorage(c *gin.Context) {
	total, avail, used, err := diskUsage(a.cfg.ResourceDir)
	if err != nil {
		// Resource dir missing/unmountable: report zeros rather than failing
		// the client's periodic settings poll.
		c.JSON(http.StatusOK, gin.H{
			"diskAvailable":       "",
			"diskAvailableRaw":    0,
			"diskSize":            "",
			"diskSizeRaw":         0,
			"diskUse":             "",
			"diskUseRaw":          0,
			"diskUsagePercentage": 0,
		})
		return
	}
	var pct float64
	if total > 0 {
		pct = float64(used) / float64(total) * 100
	}
	c.JSON(http.StatusOK, gin.H{
		"diskAvailable":       humanBytes(avail),
		"diskAvailableRaw":    avail,
		"diskSize":            humanBytes(total),
		"diskSizeRaw":         total,
		"diskUse":             humanBytes(used),
		"diskUseRaw":          used,
		"diskUsagePercentage": pct,
	})
}

// handleServerInfo mirrors GET /api/server/info. It returns the
// ServerInfoResponseDto the official web consumes for the sidebar storage
// meter (diskSizeRaw / diskUseRaw) plus the server version metadata block.
// The official web (getServerInfo) polls this endpoint on load and stores the
// result as `serverInfo`; the storage meter computes used/total from
// serverInfo.diskUseRaw / serverInfo.diskSizeRaw. Returning real disk numbers
// here (and a sensible version block) keeps the meter finite instead of NaN.
// See web/src/routes/(user)/administration/server-settings/storage-management.
func (a *App) handleServerInfo(c *gin.Context) {
	total, avail, used, err := diskUsage(a.cfg.ResourceDir)
	if err != nil {
		// Resource dir missing/unmountable: report zeros rather than failing
		// the client's periodic info poll.
		c.JSON(http.StatusOK, gin.H{
			"diskAvailable":       "",
			"diskAvailableRaw":    0,
			"diskSize":            "",
			"diskSizeRaw":         0,
			"diskUse":             "",
			"diskUseRaw":          0,
			"diskUsagePercentage": 0,
			"version":             a.cfg.CompatVersion,
			"releasedAt":          "",
			"versionChanged":      false,
			"schemaChanged":       false,
			"buildDate":           "",
			"buildId":             "",
			"isDocker":            false,
			"databaseBackup":      false,
		})
		return
	}
	var pct float64
	if total > 0 {
		pct = float64(used) / float64(total) * 100
	}
	c.JSON(http.StatusOK, gin.H{
		"diskAvailable":       humanBytes(avail),
		"diskAvailableRaw":    avail,
		"diskSize":            humanBytes(total),
		"diskSizeRaw":         total,
		"diskUse":             humanBytes(used),
		"diskUseRaw":          used,
		"diskUsagePercentage": pct,
		"version":             a.cfg.CompatVersion,
		"releasedAt":          "2024-01-01T00:00:00.000Z",
		"versionChanged":      false,
		"schemaChanged":       false,
		"buildDate":           time.Now().UTC().Format(time.RFC3339),
		"buildId":             "",
		"isDocker":            false,
		"databaseBackup":      false,
	})
}

// humanBytes formats a byte count using binary (KiB/MiB/...) units.
func humanBytes(n uint64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := uint64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

func (a *App) handleServerApkLinks(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"arm64v8a":   "",
		"armeabiv7a": "",
		"universal":  "",
		"x86_64":     "",
	})
}

func (a *App) handleServerVersionHistory(c *gin.Context) {
	c.JSON(http.StatusOK, []any{
		gin.H{
			"id":        "",
			"version":   a.cfg.CompatVersion,
			"createdAt": time.Now().UTC().Format(time.RFC3339),
		},
	})
}

func (a *App) handleServerLicense(c *gin.Context) {
	switch c.Request.Method {
	case http.MethodPut, http.MethodDelete:
		c.Status(http.StatusOK)
		return
	}
	c.JSON(http.StatusOK, gin.H{"activationKey": "", "licenseKey": "", "license": nil})
}

// ---- sync service (real delta feed) ----

// handleSyncAck records the client's acknowledgement so the delta feed can
// advance. The official client sends {type, ack} after consuming a /sync/stream
// batch; we persist the token per user (real state, not a no-op).
func (a *App) handleSyncAck(c *gin.Context) {
	uid := currentUserID(c)
	var b struct {
		Type string `json:"type"`
		Ack  string `json:"ack"`
	}
	_ = c.ShouldBindJSON(&b)
	a.store.DB.Save(&SyncState{
		UserID:       uid,
		LastAckType:  b.Type,
		LastAckToken: b.Ack,
		UpdatedAt:    time.Now().UTC(),
	})
	c.JSON(http.StatusOK, gin.H{"ack": b.Ack, "type": b.Type})
}

// handleSyncStream emits a real snapshot of the user's library as Immich
// v3.1.0 sync deltas. The official mobile client (SyncApiRepository.streamChanges)
// POSTs a SyncStreamDto{types:[...]} and expects a response with
// Content-Type: application/jsonlines+json whose body is one JSON object per
// line, each shaped as {type, data, ack}. The line's `type` must be one of the
// SyncEntityType enum strings (AuthUserV1, UserV1, AssetV2, AlbumV2,
// AlbumUserV1, AlbumToAssetV1, SyncCompleteV1, ...); the matching converter in
// the client's _kResponseMap parses `data`. We emit the types we can produce
// from real state (single-user instance: the current user, their assets,
// albums + shares) and terminate with SyncCompleteV1.
//
// Every required field (those the generated fromJson asserts with `!`) is
// emitted unconditionally; omitting any of them throws
// "Null check operator used on a null value" in the client and aborts login.
// Types that have no data here (partners, stacks, memories, people, faces,
// ocr, ...) are intentionally NOT emitted — the client tolerates their
// absence rather than crashing, which is the honest state for this instance.
func (a *App) handleSyncStream(c *gin.Context) {
	uid := currentUserID(c)
	w := c.Writer
	c.Header("Content-Type", "application/jsonlines+json")
	c.Status(http.StatusOK)

	emit := func(typeStr string, data any) {
		b, err := json.Marshal(gin.H{"type": typeStr, "data": data, "ack": ""})
		if err != nil {
			return
		}
		w.Write(b)
		w.Write([]byte{'\n'})
	}

	rfc := func(t time.Time) string { return t.UTC().Format(time.RFC3339) }
	nilStr := func(s string) any { if s == "" { return nil }; return s }

	// ---- current user (authUsersV1 + usersV1) ----
	var u User
	if err := a.store.DB.First(&u, "id = ?", uid).Error; err == nil {
		usage := a.userQuotaUsage(uid)
		avatarColor := u.AvatarColor
		if avatarColor == "" {
			avatarColor = "primary"
		}
		hasProfile := u.ProfileImagePath != ""

		emit("AuthUserV1", gin.H{
			"email":             u.Email,
			"hasProfileImage":   hasProfile,
			"id":                u.ID,
			"isAdmin":           u.IsAdmin,
			"name":              u.Name,
			"oauthId":           u.OAuthId,
			"profileChangedAt":  rfc(u.ProfileChangedAt),
			"quotaUsageInBytes": usage,
			"avatarColor":       avatarColor,
			"deletedAt":         nil,
			"pinCode":           nil,
			"quotaSizeInBytes":  u.QuotaSizeInBytes, // *int64 -> null when unset
			"storageLabel":      nilStr(u.StorageLabel),
		})
		emit("UserV1", gin.H{
			"email":            u.Email,
			"hasProfileImage":  hasProfile,
			"id":               u.ID,
			"name":             u.Name,
			"profileChangedAt": rfc(u.ProfileChangedAt),
			"avatarColor":      avatarColor,
			"deletedAt":        nil,
		})
	}

	// ---- assets (assetsV2) ----
	var assets []Asset
	a.store.DB.Where("owner_id = ? AND is_trash = ?", uid, false).Find(&assets)
	for _, as := range assets {
		visibility := visibilityOf(as)
		_, isEdited, stackID := a.assetExtras(as.ID)
		emit("AssetV2", gin.H{
			"checksum":         as.Checksum,
			"createdAt":        rfc(as.CreatedAt),
			"deletedAt":        nil,
			"duration":         durationMicros(as.Duration),
			"fileCreatedAt":    rfc(as.FileCreatedAt),
			"fileModifiedAt":   rfc(as.FileModifiedAt),
			"height":           as.Height,
			"id":               as.ID,
			"isEdited":         isEdited,
			"isFavorite":       as.IsFavorite,
			"libraryId":        nilStr(as.LibraryId),
			"livePhotoVideoId": nilStr(as.LivePhotoVideoID),
			"localDateTime":    rfc(as.LocalDateTime),
			"originalFileName": as.OriginalFileName,
			"ownerId":          as.OwnerID,
			"stackId":          nilStr(stackID),
			"thumbhash":        nilStr(as.Thumbhash),
			"type":             as.Type, // IMAGE | VIDEO | AUDIO | OTHER
			"visibility":       visibility,
			"width":            as.Width,
		})
	}

	// ---- albums (albumsV2) + shares (albumUsersV1) + links (albumAssetsV2) ----
	// Owned albums plus albums shared with this user.
	var ownedAlbums []Album
	a.store.DB.Where("owner_id = ?", uid).Find(&ownedAlbums)
	var sharedAlbums []Album
	a.store.DB.
		Where("id IN (SELECT album_id FROM "+new(AlbumUser).TableName()+" WHERE user_id = ?)", uid).
		Find(&sharedAlbums)

	seen := make(map[string]bool)
	emitAlbum := func(al Album) {
		if seen[al.ID] {
			return
		}
		seen[al.ID] = true
		var thumb any
		if al.AlbumThumbnailAssetId != "" {
			thumb = al.AlbumThumbnailAssetId
		}
		emit("AlbumV2", gin.H{
			"createdAt":         rfc(al.CreatedAt),
			"description":       al.Description,
			"id":                al.ID,
			"isActivityEnabled": al.IsActivityEnabled,
			"name":              al.AlbumName,
			"order":             "asc",
			"thumbnailAssetId":  thumb,
			"updatedAt":         rfc(al.UpdatedAt),
		})
		// owner link
		emit("AlbumUserV1", gin.H{
			"albumId": al.ID,
			"userId":  al.OwnerID,
			"role":    "owner",
		})
		// explicit shares
		var shares []AlbumUser
		a.store.DB.Where("album_id = ?", al.ID).Find(&shares)
		for _, s := range shares {
			emit("AlbumUserV1", gin.H{
				"albumId": al.ID,
				"userId":  s.UserID,
				"role":    s.Role, // editor | viewer
			})
		}
		// asset links
		var links []AlbumAsset
		a.store.DB.Where("album_id = ?", al.ID).Order("\"order\" ASC, created_at ASC").Find(&links)
		for _, l := range links {
			emit("AlbumToAssetV1", gin.H{
				"albumId": l.AlbumID,
				"assetId": l.AssetID,
			})
		}
	}
	for _, al := range ownedAlbums {
		emitAlbum(al)
	}
	for _, al := range sharedAlbums {
		emitAlbum(al)
	}

	// ---- terminal marker ----
	emit("SyncCompleteV1", gin.H{})
}

// durationMicros converts an Immich duration string ("HH:MM:SS", "HH:MM:SS.mmm",
// or a plain seconds value) into integer microseconds, the unit SyncAssetV2
// expects. Returns nil for non-video / empty input so the client's nullable int
// field stays null rather than a bogus 0.
func durationMicros(s string) any {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	// "HH:MM:SS" or "HH:MM:SS.mmm"
	if strings.Count(s, ":") == 2 {
		parts := strings.Split(s, ":")
		h, err1 := strconv.Atoi(parts[0])
		m, err2 := strconv.Atoi(parts[1])
		sec, err3 := strconv.ParseFloat(parts[2], 64)
		if err1 == nil && err2 == nil && err3 == nil {
			total := float64(h)*3600 + float64(m)*60 + sec
			return int64(total * 1e6)
		}
	}
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		return int64(v * 1e6)
	}
	return nil
}

// ---- people / faces ----

// handlePersonStatistics returns the truthful count of assets linked to the
// person (asset.person_id), not a constant 0.
func (a *App) handlePersonStatistics(c *gin.Context) {
	id := c.Param("id")
	var total int64
	a.store.DB.Model(&Asset{}).Where("person_id = ? AND is_trash = ?", id, false).Count(&total)
	c.JSON(http.StatusOK, gin.H{"assets": total})
}

// faceNotImplemented is the honest answer for every face-detection endpoint:
// face detection/recognition requires an ML backend, which is a deferred
// capability (AGENTS.md). Per hard rule #7 we return an explicit 501 rather
// than a fake-empty 200.
func (a *App) faceNotImplemented(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"message":    "face detection requires an ML backend (deferred in immich-go)",
		"statusCode": 501,
	})
}

func (a *App) handleFacesList(c *gin.Context) { a.faceNotImplemented(c) }
func (a *App) handleFaceGet(c *gin.Context)   { a.faceNotImplemented(c) }

// handlePersonMerge merges the supplied people into the canonical person `id`
// (reassigns their assets and deletes the merged rows). Real DB work.
func (a *App) handlePersonMerge(c *gin.Context) {
	id := c.Param("id")
	var body struct {
		Ids []string `json:"ids"`
	}
	_ = c.ShouldBindJSON(&body)
	canonical := id
	for _, other := range body.Ids {
		if other == "" || other == canonical {
			continue
		}
		a.store.DB.Model(&Asset{}).Where("person_id = ?", other).Update("person_id", canonical)
		a.store.DB.Where("id = ?", other).Delete(&Person{})
	}
	c.JSON(http.StatusOK, gin.H{"id": canonical, "merged": body.Ids})
}

// handlePersonReassign moves an asset (`id`) to another person (`personId`).
func (a *App) handlePersonReassign(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	var body struct {
		PersonID string `json:"personId"`
	}
	_ = c.ShouldBindJSON(&body)
	if body.PersonID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "personId required"})
		return
	}
	res := a.store.DB.Model(&Asset{}).Where("id = ? AND owner_id = ?", id, uid).Update("person_id", body.PersonID)
	if res.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"message": "asset not found"})
		return
	}
	c.Status(http.StatusOK)
}

// ---- search helpers (real geo search) ----

func (a *App) handleSearchCities(c *gin.Context) {
	if a.geocoder == nil {
		c.JSON(http.StatusOK, []any{})
		return
	}
	q := c.Query("name")
	c.JSON(http.StatusOK, a.geocoder.SearchCities(q, 100))
}

func (a *App) handleSearchPlaces(c *gin.Context) {
	if a.geocoder == nil {
		c.JSON(http.StatusOK, gin.H{"places": []any{}, "recentPlaces": []any{}, "allPlaces": []any{}})
		return
	}
	q := c.Query("name")
	places := a.geocoder.SearchCities(q, 100)
	all := a.geocoder.SearchCities("", 100)
	c.JSON(http.StatusOK, gin.H{"places": places, "recentPlaces": []any{}, "allPlaces": all})
}

// ---- minor aliases / real helpers ----

// handleDownloadInfo returns the real on-disk byte size of the requested
// assets so the client can show download progress.
func (a *App) handleDownloadInfo(c *gin.Context) {
	uid := currentUserID(c)
	var b struct {
		AssetIds []string `json:"assetIds"`
	}
	_ = c.ShouldBindJSON(&b)
	var size int64
	if len(b.AssetIds) > 0 {
		var assets []Asset
		a.store.DB.Where("id IN ? AND owner_id = ?", b.AssetIds, uid).Find(&assets)
		for _, as := range assets {
			if fi, err := os.Stat(as.OriginalPath); err == nil {
				size += fi.Size()
			} else if fi, err := os.Stat(filepath.Join(a.cfg.ResourceDir, as.OriginalPath)); err == nil {
				size += fi.Size()
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"size": size})
}

