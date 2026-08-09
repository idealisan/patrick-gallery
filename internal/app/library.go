package app

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func (a *App) handleLibraryList(c *gin.Context) {
	uid := currentUserID(c)
	var libs []Library
	a.store.DB.Where("owner_id = ?", uid).Find(&libs)
	c.JSON(http.StatusOK, libs)
}

type libraryBody struct {
	Name          string `json:"name"`
	Type          string `json:"type"`
	ImportPaths   string `json:"importPaths"`
	ExcludedPaths string `json:"excludedPaths"`
}

func (a *App) handleLibraryCreate(c *gin.Context) {
	uid := currentUserID(c)
	var b libraryBody
	_ = c.ShouldBindJSON(&b)
	if b.Name == "" {
		b.Name = "New Library"
	}
	if b.Type == "" {
		b.Type = "UPLOAD"
	}
	now := time.Now().UTC()
	lib := Library{
		ID:            newUUID(),
		OwnerID:       uid,
		Name:          b.Name,
		Type:          b.Type,
		ImportPaths:   b.ImportPaths,
		ExcludedPaths: b.ExcludedPaths,
		Watched:       false,
		Status:        "inactive",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	a.store.DB.Create(&lib)
	c.JSON(http.StatusCreated, lib)
}

func (a *App) handleLibraryGet(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	var lib Library
	if err := a.store.DB.First(&lib, "id = ? AND owner_id = ?", id, uid).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, lib)
}

func (a *App) handleLibraryUpdate(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	var lib Library
	if err := a.store.DB.First(&lib, "id = ? AND owner_id = ?", id, uid).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	var b libraryBody
	_ = c.ShouldBindJSON(&b)
	if b.Name != "" {
		lib.Name = b.Name
	}
	if b.ImportPaths != "" {
		lib.ImportPaths = b.ImportPaths
	}
	if b.ExcludedPaths != "" {
		lib.ExcludedPaths = b.ExcludedPaths
	}
	lib.UpdatedAt = time.Now().UTC()
	a.store.DB.Save(&lib)
	c.JSON(http.StatusOK, lib)
}

func (a *App) handleLibraryDelete(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	a.store.DB.Where("id = ? AND owner_id = ?", id, uid).Delete(&Library{})
	c.Status(http.StatusOK)
}

func (a *App) handleLibraryStats(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	var total, photos, videos int64
	a.store.DB.Model(&Asset{}).Where("owner_id = ? AND library_id = ? AND is_trash = ?", uid, id, false).Count(&total)
	a.store.DB.Model(&Asset{}).Where("owner_id = ? AND library_id = ? AND is_trash = ? AND type = ?", uid, id, false, "IMAGE").Count(&photos)
	a.store.DB.Model(&Asset{}).Where("owner_id = ? AND library_id = ? AND is_trash = ? AND type = ?", uid, id, false, "VIDEO").Count(&videos)
	c.JSON(http.StatusOK, gin.H{
		"total":  total,
		"photos": photos,
		"videos": videos,
		"usage":  gin.H{"total": total, "photos": photos, "videos": videos},
	})
}

func (a *App) handleLibraryScan(c *gin.Context) {
	// Filesystem crawling / thumbnail generation is not performed by the Go
	// port; the upload endpoint is the ingestion path. Acknowledge the request.
	c.JSON(http.StatusAccepted, gin.H{"status": "queued (no-op in Go port)"})
}
