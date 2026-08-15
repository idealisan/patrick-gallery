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
	if len(b.IDs) > 0 {
		a.emit("asset.restore", map[string]any{"ids": b.IDs})
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
	a.emit("asset.delete", map[string]any{"ids": []string{}})
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
	Type          string     `json:"type"`
	AssetID       string     `json:"assetId"`
	AlbumID       string     `json:"albumId"`
	ExpiresAt     *time.Time `json:"expiresAt,omitempty"`
	AllowDownload bool       `json:"allowDownload"`
	AllowUpload   bool       `json:"allowUpload"`
	Description   *string    `json:"description"`
	Password      *string    `json:"password"`
	ShowMetadata  bool       `json:"showMetadata"`
	Slug          *string    `json:"slug"`
}

// SharedLinkResponse mirrors Immich's SharedLinkResponseDto (id/userId are
// dashed v4 UUIDs; allowDownload/allowUpload/assets/description/password/
// showMetadata/slug are all required and emitted even when null).
type SharedLinkResponse struct {
	ID            string          `json:"id"`
	Key           string          `json:"key"`
	Type          string          `json:"type"`
	Album         interface{}     `json:"album,omitempty"`
	AllowDownload bool            `json:"allowDownload"`
	AllowUpload   bool            `json:"allowUpload"`
	Assets        []AssetResponse `json:"assets"`
	CreatedAt     time.Time       `json:"createdAt"`
	Description   *string         `json:"description"`
	ExpiresAt     *time.Time      `json:"expiresAt"`
	Password      *string         `json:"password"`
	ShowMetadata  bool            `json:"showMetadata"`
	Slug          *string         `json:"slug"`
	UserID        string          `json:"userId"`
}

func (a *App) toSharedLinkResponse(link *SharedLink, b *sharedLinkBody) SharedLinkResponse {
	assets := make([]AssetResponse, 0)
	if link.AssetID != "" {
		var as Asset
		if a.store.DB.First(&as, "id = ? AND owner_id = ?", link.AssetID, link.UserID).Error == nil {
			assets = append(assets, a.toResponse(as))
		}
	} else if link.AlbumID != "" {
		var aa []AlbumAsset
		a.store.DB.Where("album_id = ?", link.AlbumID).Order("\"order\" ASC").Find(&aa)
		ids := make([]string, 0, len(aa))
		for _, x := range aa {
			ids = append(ids, x.AssetID)
		}
		if len(ids) > 0 {
			var as []Asset
			a.store.DB.Where("id IN ? AND owner_id = ? AND is_trash = ?", ids, link.UserID, false).Find(&as)
			for _, x := range as {
				assets = append(assets, a.toResponse(x))
			}
		}
	}
	return SharedLinkResponse{
		ID:            link.ID,
		Key:           link.Key,
		Type:          link.Type,
		AllowDownload: b.AllowDownload,
		AllowUpload:   b.AllowUpload,
		Assets:        assets,
		CreatedAt:     link.CreatedAt,
		Description:   b.Description,
		ExpiresAt:     link.ExpiresAt,
		Password:      b.Password,
		ShowMetadata:  b.ShowMetadata,
		Slug:          b.Slug,
		UserID:        link.UserID,
	}
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
	c.JSON(http.StatusCreated, a.toSharedLinkResponse(&link, &b))
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
	if b.Type != "" {
		link.Type = b.Type
	}
	a.store.DB.Save(&link)
	c.JSON(http.StatusOK, a.toSharedLinkResponse(&link, &b))
}

func (a *App) handleSharedLinkDelete(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	a.store.DB.Where("id = ? AND user_id = ?", id, uid).Delete(&SharedLink{})
	c.Status(http.StatusOK)
}

// ---------------- people (real; ML face detection is deferred) ----------------

func (a *App) handlePeopleList(c *gin.Context) {
	uid := currentUserID(c)
	var people []Person
	q := a.store.DB.Order("name ASC")
	if !a.isAdmin(uid) {
		// non-admins only see non-hidden people
		q = q.Where("is_hidden = ?", false)
	}
	q.Find(&people)
	// People are real rows (created manually or imported); facial recognition
	// (auto-clustering) is a deferred ML job, so this reflects whatever person
	// rows exist. Asset counts are computed truthfully from asset.person_id.
	out := make([]gin.H, 0, len(people))
	var hidden int
	for _, p := range people {
		var total int64
		a.store.DB.Model(&Asset{}).Where("person_id = ? AND is_trash = ?", p.ID, false).Count(&total)
		if p.IsHidden {
			hidden++
		}
		out = append(out, gin.H{
			"id":            p.ID,
			"name":          p.Name,
			"thumbnailPath": p.ThumbnailPath,
			"isHidden":      p.IsHidden,
			"assets":        gin.H{"total": total},
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"people": out,
		"total":  len(out),
		"count":  len(out),
		"hidden": hidden,
	})
}

func (a *App) handlePersonGet(c *gin.Context) {
	id := c.Param("id")
	var p Person
	if err := a.store.DB.First(&p, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	var total int64
	a.store.DB.Model(&Asset{}).Where("person_id = ? AND is_trash = ?", p.ID, false).Count(&total)
	c.JSON(http.StatusOK, gin.H{
		"id":            p.ID,
		"name":          p.Name,
		"thumbnailPath": p.ThumbnailPath,
		"isHidden":      p.IsHidden,
		"assets":        gin.H{"total": total},
	})
}

// handlePersonCreate creates a person manually (Immich allows creating people
// without waiting for the ML clustering job).
func (a *App) handlePersonCreate(c *gin.Context) {
	uid := currentUserID(c)
	var b struct {
		Name      string `json:"name"`
		BirthDate string `json:"birthDate"`
	}
	_ = c.ShouldBindJSON(&b)
	if b.Name == "" {
		b.Name = "Unnamed"
	}
	p := Person{ID: newUUID(), Name: b.Name, IsHidden: false}
	a.store.DB.Create(&p)
	_ = uid
	c.JSON(http.StatusCreated, gin.H{
		"id":            p.ID,
		"name":          p.Name,
		"thumbnailPath": p.ThumbnailPath,
		"isHidden":      p.IsHidden,
		"assets":        gin.H{"total": 0},
	})
}

// handlePersonUpdate updates a single person (name / hidden flag).
func (a *App) handlePersonUpdate(c *gin.Context) {
	id := c.Param("id")
	var p Person
	if err := a.store.DB.First(&p, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	var b struct {
		Name     string `json:"name"`
		IsHidden *bool  `json:"isHidden"`
	}
	_ = c.ShouldBindJSON(&b)
	if b.Name != "" {
		p.Name = b.Name
	}
	if b.IsHidden != nil {
		p.IsHidden = *b.IsHidden
	}
	a.store.DB.Save(&p)
	c.JSON(http.StatusOK, gin.H{
		"id":            p.ID,
		"name":          p.Name,
		"thumbnailPath": p.ThumbnailPath,
		"isHidden":      p.IsHidden,
		"assets":        gin.H{"total": 0},
	})
}

// handlePeopleUpdateMany updates several people in one call (Immich PUT /people).
func (a *App) handlePeopleUpdateMany(c *gin.Context) {
	var people []struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		IsHidden *bool  `json:"isHidden"`
	}
	_ = c.ShouldBindJSON(&people)
	for _, p := range people {
		if p.ID == "" {
			continue
		}
		var row Person
		if err := a.store.DB.First(&row, "id = ?", p.ID).Error; err != nil {
			continue
		}
		if p.Name != "" {
			row.Name = p.Name
		}
		if p.IsHidden != nil {
			row.IsHidden = *p.IsHidden
		}
		a.store.DB.Save(&row)
	}
	c.Status(http.StatusOK)
}

// handlePersonDelete removes a person and detaches its assets.
func (a *App) handlePersonDelete(c *gin.Context) {
	id := c.Param("id")
	a.store.DB.Model(&Asset{}).Where("person_id = ?", id).Update("person_id", "")
	a.store.DB.Where("id = ?", id).Delete(&Person{})
	c.Status(http.StatusOK)
}

// handlePeopleDeleteMany removes several people (Immich DELETE /people).
func (a *App) handlePeopleDeleteMany(c *gin.Context) {
	var b struct {
		Ids []string `json:"ids"`
	}
	_ = c.ShouldBindJSON(&b)
	for _, id := range b.Ids {
		if id == "" {
			continue
		}
		a.store.DB.Model(&Asset{}).Where("person_id = ?", id).Update("person_id", "")
		a.store.DB.Where("id = ?", id).Delete(&Person{})
	}
	c.Status(http.StatusOK)
}

// ---------------- system config ----------------

func (a *App) handleSystemConfigGet(c *gin.Context) {
	var cfg SystemConfig
	a.store.DB.First(&cfg, "id = ?", "singleton")
	c.JSON(http.StatusOK, gin.H{
		"id":                  cfg.ID,
		"loginRequired":       cfg.LoginRequired,
		"isPublic":            cfg.IsPublic,
		"externalDomain":      cfg.ExternalDomain,
		"newPasswordRequired": cfg.NewPasswordRequired,
		"repository":          "immich-go",
		"releaseChannel":      "nightly",
		"version":             "1.0.0-go",
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

// ---------------- jobs ----------------
// The job handlers (handleJobsList / handleJobCommand / handleJobStatus) are
// implemented in jobs.go with a real background worker pool. They are wired in
// app.go and dispatched here for documentation clarity.

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
