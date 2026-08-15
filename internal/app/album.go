package app

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type AlbumResponse struct {
	Album
	AssetCount      int    `json:"assetCount"`
	LastModifiedAssetTimestamp *time.Time `json:"lastModifiedAssetTimestamp,omitempty"`
}

func (a *App) albumToResponse(al Album) AlbumResponse {
	var cnt int64
	a.store.DB.Model(&AlbumAsset{}).Where("album_id = ?", al.ID).Count(&cnt)
	return AlbumResponse{Album: al, AssetCount: int(cnt)}
}

func (a *App) handleAlbumList(c *gin.Context) {
	uid := currentUserID(c)
	var albums []Album
	a.store.DB.Where("owner_id = ?", uid).Order("created_at DESC").Find(&albums)
	out := make([]AlbumResponse, 0, len(albums))
	for _, al := range albums {
		out = append(out, a.albumToResponse(al))
	}
	c.JSON(http.StatusOK, out)
}

type albumCreateBody struct {
	AlbumName   string   `json:"albumName"`
	Description string   `json:"description"`
	AssetIDs    []string `json:"assetIds"`
}

func (a *App) handleAlbumCreate(c *gin.Context) {
	uid := currentUserID(c)
	var b albumCreateBody
	_ = c.ShouldBindJSON(&b)
	if b.AlbumName == "" {
		b.AlbumName = "Untitled Album"
	}
	now := time.Now().UTC()
	al := Album{
		ID:        newUUID(),
		OwnerID:   uid,
		AlbumName: b.AlbumName,
		Description: b.Description,
		CreatedAt: now,
		UpdatedAt: now,
	}
	a.store.DB.Create(&al)
	for i, aid := range b.AssetIDs {
		a.store.DB.Create(&AlbumAsset{AlbumID: al.ID, AssetID: aid, Order: i, CreatedAt: now})
	}
	if len(b.AssetIDs) > 0 {
		al.AlbumThumbnailAssetId = b.AssetIDs[0]
		a.store.DB.Save(&al)
	}
	c.JSON(http.StatusCreated, a.albumToResponse(al))
	a.emit("album.create", map[string]any{"id": al.ID})
}

// handleAlbumAssets returns the assets belonging to an album, in the order
// stored on the join table (AlbumAsset.Order). This is the Immich-compatible
// GET /api/albums/:id/assets endpoint the web UI needs to render album detail.
func (a *App) handleAlbumAssets(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	var al Album
	if err := a.store.DB.First(&al, "id = ? AND owner_id = ?", id, uid).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	var links []AlbumAsset
	a.store.DB.Where("album_id = ?", id).Order("\"order\" ASC, created_at ASC").Find(&links)
	ids := make([]string, 0, len(links))
	for _, l := range links {
		ids = append(ids, l.AssetID)
	}
	byID := map[string]Asset{}
	if len(ids) > 0 {
		var assets []Asset
		a.store.DB.Where("id IN ? AND owner_id = ?", ids, uid).Find(&assets)
		for _, as := range assets {
			byID[as.ID] = as
		}
	}
	out := make([]AssetResponse, 0, len(ids))
	for _, aid := range ids {
		if as, ok := byID[aid]; ok {
			out = append(out, a.toResponse(as))
		}
	}
	c.JSON(http.StatusOK, gin.H{"assets": out, "count": len(out), "total": len(out)})
}

func (a *App) handleAlbumGet(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	var al Album
	if err := a.store.DB.First(&al, "id = ? AND owner_id = ?", id, uid).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, a.albumToResponse(al))
}

func (a *App) handleAlbumUpdate(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	var al Album
	if err := a.store.DB.First(&al, "id = ? AND owner_id = ?", id, uid).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	var b albumCreateBody
	_ = c.ShouldBindJSON(&b)
	if b.AlbumName != "" {
		al.AlbumName = b.AlbumName
	}
	if b.Description != "" {
		al.Description = b.Description
	}
	al.UpdatedAt = time.Now().UTC()
	a.store.DB.Save(&al)
	a.emit("album.update", map[string]any{"id": al.ID})
	c.JSON(http.StatusOK, a.albumToResponse(al))
}

func (a *App) handleAlbumDelete(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	a.store.DB.Where("id = ? AND owner_id = ?", id, uid).Delete(&Album{})
	a.store.DB.Where("album_id = ?", id).Delete(&AlbumAsset{})
	a.emit("album.delete", map[string]any{"id": id})
	c.Status(http.StatusOK)
}

type albumAssetsBody struct {
	IDs   []string `json:"ids"`
	Order int      `json:"order"`
}

func (a *App) handleAlbumAddAssets(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	var al Album
	if err := a.store.DB.First(&al, "id = ? AND owner_id = ?", id, uid).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	var b albumAssetsBody
	_ = c.ShouldBindJSON(&b)
	now := time.Now().UTC()
	var added []string
	for i, aid := range b.IDs {
		var cnt int64
		a.store.DB.Model(&AlbumAsset{}).Where("album_id = ? AND asset_id = ?", id, aid).Count(&cnt)
		if cnt > 0 {
			continue
		}
		a.store.DB.Create(&AlbumAsset{AlbumID: id, AssetID: aid, Order: b.Order + i, CreatedAt: now})
		added = append(added, aid)
	}
	if al.AlbumThumbnailAssetId == "" && len(b.IDs) > 0 {
		al.AlbumThumbnailAssetId = b.IDs[0]
		al.UpdatedAt = now
		a.store.DB.Save(&al)
	}
	if len(added) > 0 {
		a.emit("album.addAssets", map[string]any{"id": id, "assetIds": added})
	}
	c.JSON(http.StatusOK, gin.H{"added": added, "album": a.albumToResponse(al)})
}

func (a *App) handleAlbumRemoveAssets(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	var al Album
	if err := a.store.DB.First(&al, "id = ? AND owner_id = ?", id, uid).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	var b albumAssetsBody
	_ = c.ShouldBindJSON(&b)
	a.store.DB.Where("album_id = ? AND asset_id IN ?", id, b.IDs).Delete(&AlbumAsset{})
	if len(b.IDs) > 0 {
		a.emit("album.removeAssets", map[string]any{"id": id, "assetIds": b.IDs})
	}
	c.JSON(http.StatusOK, gin.H{"removed": b.IDs, "album": a.albumToResponse(al)})
}

func (a *App) handleAlbumUpdateAssets(c *gin.Context) {
	// re-insert with given order (mirrors Immich PATCH semantics)
	uid := currentUserID(c)
	id := c.Param("id")
	var al Album
	if err := a.store.DB.First(&al, "id = ? AND owner_id = ?", id, uid).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	var b albumAssetsBody
	_ = c.ShouldBindJSON(&b)
	a.store.DB.Where("album_id = ?", id).Delete(&AlbumAsset{})
	now := time.Now().UTC()
	for i, aid := range b.IDs {
		a.store.DB.Create(&AlbumAsset{AlbumID: id, AssetID: aid, Order: i, CreatedAt: now})
	}
	c.JSON(http.StatusOK, a.albumToResponse(al))
}

func (a *App) handleAlbumSetCover(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	var al Album
	if err := a.store.DB.First(&al, "id = ? AND owner_id = ?", id, uid).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	var b struct {
		AssetID string `json:"assetId"`
	}
	_ = c.ShouldBindJSON(&b)
	al.AlbumThumbnailAssetId = b.AssetID
	al.UpdatedAt = time.Now().UTC()
	a.store.DB.Save(&al)
	c.JSON(http.StatusOK, a.albumToResponse(al))
}
