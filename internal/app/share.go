package app

import (
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"immich-go/internal/video"
)

// ---------------- public shared-link access ----------------
// Shared links are created from the authenticated UI but viewed by anyone who
// holds the (unguessable) key, with no login. These handlers are registered
// OUTSIDE the auth guard in app.go.

var errShareExpired = errors.New("share expired")

// loadShare fetches a share by key and rejects expired links.
func (a *App) loadShare(key string) (*SharedLink, error) {
	var link SharedLink
	if err := a.store.DB.First(&link, "key = ?", key).Error; err != nil {
		return nil, err
	}
	if link.ExpiresAt != nil && link.ExpiresAt.Before(time.Now().UTC()) {
		return nil, errShareExpired
	}
	return &link, nil
}

// shareAssetIDs returns the set of asset IDs reachable through the link.
func (a *App) shareAssetIDs(link *SharedLink) map[string]bool {
	set := map[string]bool{}
	if link.Type == "ALBUM" && link.AlbumID != "" {
		var links []AlbumAsset
		a.store.DB.Where("album_id = ?", link.AlbumID).Find(&links)
		for _, l := range links {
			set[l.AssetID] = true
		}
	} else if link.AssetID != "" {
		set[link.AssetID] = true
	}
	return set
}

// shareAssets returns the ordered AssetResponses for the link.
func (a *App) shareAssets(link *SharedLink) ([]AssetResponse, string) {
	var ids []string
	var albumName string
	if link.Type == "ALBUM" && link.AlbumID != "" {
		var links []AlbumAsset
		a.store.DB.Where("album_id = ?", link.AlbumID).Order("\"order\" ASC, created_at ASC").Find(&links)
		for _, l := range links {
			ids = append(ids, l.AssetID)
		}
		var al Album
		if err := a.store.DB.First(&al, "id = ?", link.AlbumID).Error; err == nil {
			albumName = al.AlbumName
		}
	} else if link.AssetID != "" {
		ids = append(ids, link.AssetID)
	}

	out := make([]AssetResponse, 0, len(ids))
	if len(ids) > 0 {
		var assets []Asset
		a.store.DB.Where("id IN ?", ids).Find(&assets)
		byID := map[string]Asset{}
		for _, as := range assets {
			byID[as.ID] = as
		}
		for _, id := range ids {
			if as, ok := byID[id]; ok {
				out = append(out, a.toResponse(as))
			}
		}
	}
	return out, albumName
}

// handleShareView returns the shared assets (album or single) as JSON.
func (a *App) handleShareView(c *gin.Context) {
	key := c.Param("key")
	link, err := a.loadShare(key)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "share not found"})
		return
	}
	assets, albumName := a.shareAssets(link)
	c.JSON(http.StatusOK, gin.H{
		"key":       key,
		"type":      link.Type,
		"albumName": albumName,
		"assets":    assets,
		"count":     len(assets),
	})
}

// handleShareThumbnail serves a thumbnail of a shared asset (public).
func (a *App) handleShareThumbnail(c *gin.Context) {
	a.serveSharedAsset(c, false)
}

// handleShareOriginal serves the original (or transcoded video) of a shared
// asset (public).
func (a *App) handleShareOriginal(c *gin.Context) {
	a.serveSharedAsset(c, true)
}

func (a *App) serveSharedAsset(c *gin.Context, original bool) {
	key := c.Param("key")
	assetID := c.Param("assetId")
	link, err := a.loadShare(key)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	if !a.shareAssetIDs(link)[assetID] {
		c.Status(http.StatusNotFound)
		return
	}
	var asset Asset
	if err := a.store.DB.First(&asset, "id = ?", assetID).Error; err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	if original && asset.Type == "VIDEO" {
		// best-effort in-process transcode so browsers can play it
		if raw, rerr := os.ReadFile(asset.OriginalPath); rerr == nil {
			if out, terr := a.video.Transcode(raw, video.TranscodeOptions{Format: "mp4", VideoCodec: "h264", AudioCodec: "copy", Preset: "software"}); terr == nil && len(out) > 0 {
				c.Header("Content-Type", "video/mp4")
				c.Data(http.StatusOK, "video/mp4", out)
				return
			}
		}
		c.File(asset.OriginalPath)
		return
	}
	if !original && asset.ResizePath != "" {
		c.File(asset.ResizePath)
		return
	}
	c.File(asset.OriginalPath)
}
