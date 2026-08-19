package app

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

// handleSharedLinksMe returns the shared links the current user has access to
// via albums they own or collaborate on (GET /shared-links/me).
func (a *App) handleSharedLinksMe(c *gin.Context) {
	uid := currentUserID(c)
	// Links created by the user, plus links on albums they own or are added to.
	albumIDs := []string{}
	a.store.DB.Model(&AlbumUser{}).Where("user_id = ?", uid).Pluck("album_id", &albumIDs)
	var ownAlbums []string
	a.store.DB.Model(&Album{}).Where("owner_id = ?", uid).Pluck("id", &ownAlbums)
	albumIDs = append(albumIDs, ownAlbums...)

	var links []SharedLink
	if len(albumIDs) > 0 {
		a.store.DB.Where("user_id = ? OR album_id IN ?", uid, albumIDs).Order("created_at DESC").Find(&links)
	} else {
		a.store.DB.Where("user_id = ?", uid).Order("created_at DESC").Find(&links)
	}
	out := make([]SharedLinkResponse, 0, len(links))
	for i := range links {
		out = append(out, a.toSharedLinkResponse(&links[i]))
	}
	c.JSON(http.StatusOK, out)
}

// handleSharedLinkLogin validates a shared-link key (and optional password) and
// returns the link metadata (POST /shared-links/login). Mirrors the official
// endpoint used by the public share view to gate password-protected links.
func (a *App) handleSharedLinkLogin(c *gin.Context) {
	var b struct {
		Key      string  `json:"key"`
		Password *string `json:"password"`
	}
	if err := c.ShouldBindJSON(&b); err != nil || b.Key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "key required", "statusCode": 400})
		return
	}
	link, err := a.loadShare(b.Key)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "share not found", "statusCode": 404})
		return
	}
	if link.Password != "" {
		if b.Password == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "password required", "statusCode": 401, "needsPassword": true})
			return
		}
		if bcrypt.CompareHashAndPassword([]byte(link.Password), []byte(*b.Password)) != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid password", "statusCode": 401})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"key":          link.Key,
		"type":         link.Type,
		"albumId":      link.AlbumID,
		"assetId":      link.AssetID,
		"allowDownload": link.AllowDownload,
		"allowUpload":   link.AllowUpload,
		"showMetadata":  link.ShowMetadata,
		"description":   link.Description,
		"expiresAt":     link.ExpiresAt,
		"userId":        link.UserID,
	})
}

// handleSharedLinkAssetAdd adds an asset to an album-type shared link
// (GET /shared-links/:id/assets/:assetId is the official "add" verb — Immich
// treats the GET as the add action for share links). We register both GET and
// the more conventional POST/PATCH aliases.
func (a *App) handleSharedLinkAssetAdd(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	assetID := c.Param("assetId")
	var link SharedLink
	if err := a.store.DB.Where("id = ? AND user_id = ?", id, uid).First(&link).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found", "statusCode": 404})
		return
	}
	if link.Type != "ALBUM" || link.AlbumID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "only album links support assets", "statusCode": 400})
		return
	}
	var asset Asset
	if a.store.DB.Where("id = ?", assetID).First(&asset).Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "asset not found", "statusCode": 404})
		return
	}
	// add to the underlying album (which is what the link exposes)
	var cnt int64
	a.store.DB.Model(&AlbumAsset{}).Where("album_id = ? AND asset_id = ?", link.AlbumID, assetID).Count(&cnt)
	if cnt == 0 {
		a.store.DB.Create(&AlbumAsset{AlbumID: link.AlbumID, AssetID: assetID, CreatedAt: time.Now().UTC()})
	}
	c.Status(http.StatusNoContent)
}

// handleSharedLinkAssetRemove removes an asset from an album-type shared link
// (PUT/DELETE /shared-links/:id/assets/:assetId).
func (a *App) handleSharedLinkAssetRemove(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	assetID := c.Param("assetId")
	var link SharedLink
	if err := a.store.DB.Where("id = ? AND user_id = ?", id, uid).First(&link).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found", "statusCode": 404})
		return
	}
	if link.Type != "ALBUM" || link.AlbumID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "only album links support assets", "statusCode": 400})
		return
	}
	a.store.DB.Where("album_id = ? AND asset_id = ?", link.AlbumID, assetID).Delete(&AlbumAsset{})
	c.Status(http.StatusNoContent)
}
