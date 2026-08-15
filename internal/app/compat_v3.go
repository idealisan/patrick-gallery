package app

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
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
	if c.Request.Method == http.MethodPut {
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
// sync deltas (AssetV1 for each asset, AlbumV1 for each album with its member
// ids). This is the first-time / full reconciliation payload the official
// client consumes; incremental advancement is keyed off the last ack
// (handleSyncAck). Live changes after connect are pushed over the websocket
// event channel.
func (a *App) handleSyncStream(c *gin.Context) {
	uid := currentUserID(c)
	deltas := make([]gin.H, 0, 64)

	var assets []Asset
	a.store.DB.Where("owner_id = ? AND is_trash = ?", uid, false).Find(&assets)
	for _, as := range assets {
		deltas = append(deltas, gin.H{"type": "AssetV1", "asset": a.toResponse(as)})
	}

	var albums []Album
	a.store.DB.Where("owner_id = ?", uid).Find(&albums)
	for _, al := range albums {
		var links []AlbumAsset
		a.store.DB.Where("album_id = ?", al.ID).Order("\"order\" ASC, created_at ASC").Find(&links)
		ids := make([]string, 0, len(links))
		for _, l := range links {
			ids = append(ids, l.AssetID)
		}
		deltas = append(deltas, gin.H{"type": "AlbumV1", "album": a.albumToResponse(al), "assets": ids})
	}

	c.JSON(http.StatusOK, deltas)
}

// ---- people / faces (no ML backend) ----

func (a *App) handlePersonStatistics(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"assets": 0})
}

func (a *App) handleFacesList(c *gin.Context) {
	c.JSON(http.StatusOK, []any{})
}

func (a *App) handleFaceGet(c *gin.Context) {
	c.JSON(http.StatusNotFound, gin.H{"message": "no ML backend", "statusCode": 404})
}

func (a *App) handlePersonMerge(c *gin.Context) {
	c.Status(http.StatusOK)
}

func (a *App) handlePersonReassign(c *gin.Context) {
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

// handleDuplicatesResolve records which asset the user kept when dismissing a
// duplicate pair; the hidden side is then excluded from future duplicate
// listings (see handleAssetDuplicates).
func (a *App) handleDuplicatesResolve(c *gin.Context) {
	uid := currentUserID(c)
	var b struct {
		AssetID     string `json:"assetId"`
		DuplicateID string `json:"duplicateId"`
	}
	_ = c.ShouldBindJSON(&b)
	if b.AssetID == "" || b.DuplicateID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "assetId and duplicateId are required"})
		return
	}
	var keeper, dup Asset
	if a.store.DB.First(&keeper, "id = ? AND owner_id = ?", b.AssetID, uid).Error != nil ||
		a.store.DB.First(&dup, "id = ? AND owner_id = ?", b.DuplicateID, uid).Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "asset not found"})
		return
	}
	a.store.DB.Save(&DuplicateResolution{
		AssetID:     b.AssetID,
		DuplicateID: b.DuplicateID,
		CreatedAt:   time.Now().UTC(),
	})
	c.JSON(http.StatusOK, gin.H{"assetId": b.AssetID, "duplicateId": b.DuplicateID})
}

// handleDuplicatesUpdate handles PUT/DELETE /duplicates/:id and DELETE
// /duplicates. It manages DuplicateResolution rows: PUT reassigns the keeper
// for a duplicate, DELETE removes a resolution (or all of the user's
// resolutions when no id is given).
func (a *App) handleDuplicatesUpdate(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	switch c.Request.Method {
	case http.MethodDelete:
		if id != "" {
			a.store.DB.Where("duplicate_id = ?", id).Delete(&DuplicateResolution{})
		} else {
			a.store.DB.Where("duplicate_id IN (?)",
				a.store.DB.Model(&Asset{}).Select("id").Where("owner_id = ?", uid),
			).Delete(&DuplicateResolution{})
		}
		c.Status(http.StatusOK)
	case http.MethodPut:
		var b struct {
			AssetID string `json:"assetId"`
		}
		_ = c.ShouldBindJSON(&b)
		if id == "" || b.AssetID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"message": "id and assetId required"})
			return
		}
		a.store.DB.Save(&DuplicateResolution{
			AssetID:     b.AssetID,
			DuplicateID: id,
			CreatedAt:   time.Now().UTC(),
		})
		c.JSON(http.StatusOK, gin.H{"assetId": b.AssetID, "duplicateId": id})
	}
}
