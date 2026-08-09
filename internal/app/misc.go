package app

import (
	"archive/zip"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ---------------- partners ----------------

func (a *App) handlePartnerList(c *gin.Context) {
	uid := currentUserID(c)
	var partners []Partner
	a.store.DB.Where("shared_with_id = ? OR shared_by_id = ?", uid, uid).Find(&partners)
	out := make([]gin.H, 0, len(partners))
	for _, p := range partners {
		other := p.SharedWithID
		if other == uid {
			other = p.SharedByID
		}
		var u User
		if err := a.store.DB.First(&u, "id = ?", other).Error; err == nil {
			out = append(out, gin.H{
				"sharedById":   p.SharedByID,
				"sharedWithId": p.SharedWithID,
				"name":         u.Name,
				"email":        u.Email,
				"avatarColor":  u.AvatarColor,
			})
		}
	}
	c.JSON(http.StatusOK, out)
}

func (a *App) handlePartnerCreate(c *gin.Context) {
	uid := currentUserID(c)
	var b struct {
		SharedUserId string `json:"sharedUserId"`
	}
	_ = c.ShouldBindJSON(&b)
	if b.SharedUserId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sharedUserId required"})
		return
	}
	p := Partner{SharedByID: uid, SharedWithID: b.SharedUserId}
	a.store.DB.Create(&p)
	c.JSON(http.StatusCreated, p)
}

func (a *App) handlePartnerDelete(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	a.store.DB.Where("(shared_by_id = ? AND shared_with_id = ?) OR (shared_with_id = ? AND shared_by_id = ?)", uid, id, uid, id).Delete(&Partner{})
	c.Status(http.StatusOK)
}

// ---------------- trash ----------------

func (a *App) handleTrashList(c *gin.Context) {
	uid := currentUserID(c)
	var assets []Asset
	a.store.DB.Where("owner_id = ? AND is_trash = ?", uid, true).Order("updated_at DESC").Find(&assets)
	out := make([]AssetResponse, 0, len(assets))
	for _, as := range assets {
		out = append(out, a.toResponse(as))
	}
	c.JSON(http.StatusOK, gin.H{"assets": out, "count": len(out), "total": len(out)})
}

func (a *App) handleTrashRestore(c *gin.Context) {
	uid := currentUserID(c)
	var b struct {
		IDs []string `json:"ids"`
	}
	_ = c.ShouldBindJSON(&b)
	for _, id := range b.IDs {
		a.store.DB.Model(&Asset{}).Where("id = ? AND owner_id = ?", id, uid).Update("is_trash", false)
	}
	c.JSON(http.StatusOK, gin.H{"restored": b.IDs})
}

func (a *App) handleTrashEmpty(c *gin.Context) {
	uid := currentUserID(c)
	var assets []Asset
	a.store.DB.Where("owner_id = ? AND is_trash = ?", uid, true).Find(&assets)
	for _, as := range assets {
		_ = os.Remove(as.OriginalPath)
		if as.ResizePath != "" {
			_ = os.Remove(as.ResizePath)
		}
	}
	a.store.DB.Where("owner_id = ? AND is_trash = ?", uid, true).Delete(&Asset{})
	c.Status(http.StatusOK)
}

// ---------------- activity ----------------

func (a *App) handleActivityList(c *gin.Context) {
	uid := currentUserID(c)
	var acts []Activity
	q := a.store.DB.Where("user_id = ?", uid)
	if aid := c.Query("assetId"); aid != "" {
		q = q.Where("asset_id = ?", aid)
	}
	if aid := c.Query("albumId"); aid != "" {
		q = q.Where("album_id = ?", aid)
	}
	q.Order("created_at DESC").Find(&acts)
	c.JSON(http.StatusOK, acts)
}

func (a *App) handleActivityCreate(c *gin.Context) {
	uid := currentUserID(c)
	var b struct {
		AssetID string `json:"assetId"`
		AlbumID string `json:"albumId"`
		Comment string `json:"comment"`
	}
	_ = c.ShouldBindJSON(&b)
	act := Activity{
		ID:        newUUID(),
		AssetID:   b.AssetID,
		AlbumID:   b.AlbumID,
		UserID:    uid,
		Comment:   b.Comment,
		CreatedAt: time.Now().UTC(),
	}
	a.store.DB.Create(&act)
	c.JSON(http.StatusCreated, act)
}

func (a *App) handleActivityDelete(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	a.store.DB.Where("id = ? AND user_id = ?", id, uid).Delete(&Activity{})
	c.Status(http.StatusOK)
}

func (a *App) handleActivityByAsset(c *gin.Context) {
	aid := c.Param("id")
	var acts []Activity
	a.store.DB.Where("asset_id = ?", aid).Order("created_at DESC").Find(&acts)
	c.JSON(http.StatusOK, acts)
}

func (a *App) handleActivityByAlbum(c *gin.Context) {
	aid := c.Param("id")
	var acts []Activity
	a.store.DB.Where("album_id = ?", aid).Order("created_at DESC").Find(&acts)
	c.JSON(http.StatusOK, acts)
}

// ---------------- shared links ----------------

func (a *App) handleSharedLinkList(c *gin.Context) {
	uid := currentUserID(c)
	var links []SharedLink
	a.store.DB.Where("user_id = ?", uid).Order("created_at DESC").Find(&links)
	c.JSON(http.StatusOK, links)
}

type sharedLinkBody struct {
	Type    string     `json:"type"`
	AssetID string     `json:"assetId"`
	AlbumID string     `json:"albumId"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}

func (a *App) handleSharedLinkCreate(c *gin.Context) {
	uid := currentUserID(c)
	var b sharedLinkBody
	_ = c.ShouldBindJSON(&b)
	link := SharedLink{
		ID:        newUUID(),
		Key:       newUUID() + newUUID(),
		Type:      b.Type,
		AssetID:   b.AssetID,
		AlbumID:   b.AlbumID,
		UserID:    uid,
		ExpiresAt: b.ExpiresAt,
		CreatedAt: time.Now().UTC(),
	}
	a.store.DB.Create(&link)
	c.JSON(http.StatusCreated, link)
}

func (a *App) handleSharedLinkUpdate(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	var link SharedLink
	if err := a.store.DB.First(&link, "id = ? AND user_id = ?", id, uid).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	var b sharedLinkBody
	_ = c.ShouldBindJSON(&b)
	if b.ExpiresAt != nil {
		link.ExpiresAt = b.ExpiresAt
	}
	a.store.DB.Save(&link)
	c.JSON(http.StatusOK, link)
}

func (a *App) handleSharedLinkDelete(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	a.store.DB.Where("id = ? AND user_id = ?", id, uid).Delete(&SharedLink{})
	c.Status(http.StatusOK)
}

// ---------------- people (stub) ----------------

func (a *App) handlePeopleList(c *gin.Context) {
	uid := currentUserID(c)
	var people []Person
	a.store.DB.Find(&people)
	_ = uid
	c.JSON(http.StatusOK, people)
}

func (a *App) handlePersonGet(c *gin.Context) {
	id := c.Param("id")
	var p Person
	if err := a.store.DB.First(&p, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, p)
}

// ---------------- system config ----------------

func (a *App) handleSystemConfigGet(c *gin.Context) {
	var cfg SystemConfig
	a.store.DB.First(&cfg, "id = ?", "singleton")
	c.JSON(http.StatusOK, gin.H{
		"id":                 cfg.ID,
		"loginRequired":      cfg.LoginRequired,
		"isPublic":           cfg.IsPublic,
		"externalDomain":     cfg.ExternalDomain,
		"newPasswordRequired": cfg.NewPasswordRequired,
		"repository":         "immich-go",
		"releaseChannel":     "nightly",
		"version":            "1.0.0-go",
	})
}

func (a *App) handleSystemConfigUpdate(c *gin.Context) {
	var cfg SystemConfig
	a.store.DB.First(&cfg, "id = ?", "singleton")
	var b map[string]interface{}
	_ = c.ShouldBindJSON(&b)
	if v, ok := b["loginRequired"].(bool); ok {
		cfg.LoginRequired = v
	}
	if v, ok := b["isPublic"].(bool); ok {
		cfg.IsPublic = v
	}
	if v, ok := b["externalDomain"].(string); ok {
		cfg.ExternalDomain = v
	}
	a.store.DB.Save(&cfg)
	c.JSON(http.StatusOK, gin.H{"loginRequired": cfg.LoginRequired, "isPublic": cfg.IsPublic, "externalDomain": cfg.ExternalDomain})
}

// ---------------- jobs (stub) ----------------

var jobNames = []string{
	"thumbnailGeneration", "metadataExtraction", "videoConversion",
	"objectDetection", "facialRecognition", "duplicateDetection",
	"smartSearch", "storageTemplateMigration", "tagCopy", "tagImage",
}

func (a *App) handleJobsList(c *gin.Context) {
	out := make([]gin.H, 0, len(jobNames))
	for _, n := range jobNames {
		out = append(out, gin.H{"name": n, "amount": 0, "completedAt": nil, "queueStatus": nil})
	}
	c.JSON(http.StatusOK, out)
}

func (a *App) handleJobCommand(c *gin.Context) {
	id := c.Param("id")
	c.JSON(http.StatusAccepted, gin.H{"id": id, "status": "queued (no-op in Go port)"})
}

func (a *App) handleJobStatus(c *gin.Context) {
	id := c.Param("id")
	c.JSON(http.StatusOK, gin.H{"name": id, "amount": 0, "completedAt": nil})
}

// ---------------- download archive ----------------

func (a *App) handleDownloadArchive(c *gin.Context) {
	uid := currentUserID(c)
	ids := c.QueryArray("assetIds")
	if len(ids) == 0 {
		if s := c.Query("assetIds"); s != "" {
			ids = strings.Split(s, ",")
		}
	}
	if len(ids) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "assetIds required"})
		return
	}
	c.Writer.Header().Set("Content-Type", "application/zip")
	c.Writer.Header().Set("Content-Disposition", "attachment; filename=\"immich-export.zip\"")
	zw := zip.NewWriter(c.Writer)
	defer zw.Close()
	for _, id := range ids {
		var as Asset
		if err := a.store.DB.First(&as, "id = ? AND owner_id = ?", id, uid).Error; err != nil {
			continue
		}
		f, err := os.Open(as.OriginalPath)
		if err != nil {
			continue
		}
		w, err := zw.Create(as.OriginalFileName)
		if err != nil {
			f.Close()
			continue
		}
		_, _ = ioCopy(w, f)
		f.Close()
	}
}
