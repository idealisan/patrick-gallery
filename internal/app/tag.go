package app

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func (a *App) handleTagList(c *gin.Context) {
	uid := currentUserID(c)
	var tags []Tag
	a.store.DB.Where("user_id = ?", uid).Order("name").Find(&tags)
	c.JSON(http.StatusOK, tags)
}

type tagBody struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Color string `json:"color"`
}

func (a *App) handleTagCreate(c *gin.Context) {
	uid := currentUserID(c)
	var b tagBody
	_ = c.ShouldBindJSON(&b)
	t := Tag{
		ID:     newUUID(),
		UserID: uid,
		Name:   b.Name,
		Type:   b.Type,
		Color:  b.Color,
	}
	a.store.DB.Create(&t)
	c.JSON(http.StatusCreated, t)
}

func (a *App) handleTagGet(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	var t Tag
	if err := a.store.DB.First(&t, "id = ? AND user_id = ?", id, uid).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, t)
}

func (a *App) handleTagUpdate(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	var t Tag
	if err := a.store.DB.First(&t, "id = ? AND user_id = ?", id, uid).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	var b tagBody
	_ = c.ShouldBindJSON(&b)
	if b.Name != "" {
		t.Name = b.Name
	}
	if b.Color != "" {
		t.Color = b.Color
	}
	a.store.DB.Save(&t)
	c.JSON(http.StatusOK, t)
}

func (a *App) handleTagDelete(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	a.store.DB.Where("id = ? AND user_id = ?", id, uid).Delete(&Tag{})
	a.store.DB.Where("tag_id = ?", id).Delete(&AssetTag{})
	c.Status(http.StatusOK)
}

func (a *App) handleTagAddAssets(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	var t Tag
	if err := a.store.DB.First(&t, "id = ? AND user_id = ?", id, uid).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	var b struct {
		IDs []string `json:"ids"`
	}
	_ = c.ShouldBindJSON(&b)
	now := time.Now().UTC()
	for _, aid := range b.IDs {
		var cnt int64
		a.store.DB.Model(&AssetTag{}).Where("asset_id = ? AND tag_id = ?", aid, id).Count(&cnt)
		if cnt == 0 {
			a.store.DB.Create(&AssetTag{AssetID: aid, TagID: id})
		}
		_ = now
	}
	c.JSON(http.StatusOK, gin.H{"tagId": id, "added": b.IDs})
}

func (a *App) handleTagRemoveAsset(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	aid := c.Param("assetId")
	var t Tag
	if err := a.store.DB.First(&t, "id = ? AND user_id = ?", id, uid).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	a.store.DB.Where("asset_id = ? AND tag_id = ?", aid, id).Delete(&AssetTag{})
	c.Status(http.StatusOK)
}
