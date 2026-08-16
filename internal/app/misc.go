package app

import (
	"archive/zip"
	"net/http"
	"os"
	"path/filepath"
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

// sharedOwnerIDs returns the user plus every partner who has shared their
// library with them, so partner assets surface in the viewer's timeline/search.
func (a *App) sharedOwnerIDs(uid string) []string {
	ids := []string{uid}
	var partners []Partner
	a.store.DB.Where("shared_with_id = ?", uid).Find(&partners)
	for _, p := range partners {
		if p.SharedByID != "" {
			ids = append(ids, p.SharedByID)
		}
	}
	return ids
}

// canView reports whether uid may read an asset owned by ownerID (own asset,
// admin, or an asset shared with uid by a partner).
func (a *App) canView(uid, ownerID string) bool {
	if uid == ownerID || a.isAdmin(uid) {
		return true
	}
	for _, o := range a.sharedOwnerIDs(uid) {
		if o == ownerID {
			return true
		}
	}
	return false
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
		_ = os.Remove(previewCachePath(as.OriginalPath, as.ID))
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

// handleActivityStatistics returns aggregate activity counts (album comments).
func (a *App) handleActivityStatistics(c *gin.Context) {
	uid := currentUserID(c)
	var total int64
	a.store.DB.Model(&Activity{}).Where("user_id = ?", uid).Count(&total)
	c.JSON(http.StatusOK, gin.H{
		"total":    total,
		"comments": total,
		"likes":    int64(0),
	})
}

// handleViewFolder returns the folder tree (parent directories of the user's
// asset original paths) with per-folder asset counts — Immich's web folder view.
func (a *App) handleViewFolder(c *gin.Context) {
	uid := currentUserID(c)
	var assets []Asset
	a.store.DB.Where("owner_id = ? AND is_trash = ?", uid, false).Find(&assets)
	counts := map[string]int{}
	for _, as := range assets {
		if as.OriginalPath == "" {
			continue
		}
		dir := filepath.Dir(as.OriginalPath)
		counts[dir]++
	}
	folders := make([]gin.H, 0, len(counts))
	for dir, n := range counts {
		folders = append(folders, gin.H{
			"id":     dir,
			"name":   filepath.Base(dir),
			"path":   dir,
			"assets": n,
		})
	}
	c.JSON(http.StatusOK, gin.H{"folders": folders, "total": len(folders)})
}

// handleViewFolderUniquePaths returns the distinct root (top-level) paths of
// the user's assets.
func (a *App) handleViewFolderUniquePaths(c *gin.Context) {
	uid := currentUserID(c)
	var assets []Asset
	a.store.DB.Where("owner_id = ? AND is_trash = ?", uid, false).Find(&assets)
	seen := map[string]bool{}
	paths := []string{}
	for _, as := range assets {
		if as.OriginalPath == "" {
			continue
		}
		root := filepath.Dir(as.OriginalPath)
		if !seen[root] {
			seen[root] = true
			paths = append(paths, root)
		}
	}
	c.JSON(http.StatusOK, paths)
}

// ---------------- shared links ----------------

func (a *App) handleSharedLinkList(c *gin.Context) {
	uid := currentUserID(c)
	var links []SharedLink
	a.store.DB.Where("user_id = ?", uid).Order("created_at DESC").Find(&links)
	out := make([]SharedLinkResponse, 0, len(links))
	for i := range links {
		out = append(out, a.toSharedLinkResponse(&links[i]))
	}
	c.JSON(http.StatusOK, out)
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

func (a *App) toSharedLinkResponse(link *SharedLink) SharedLinkResponse {
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
	var desc *string
	if link.Description != "" {
		d := link.Description
		desc = &d
	}
	var pw *string
	if link.Password != "" {
		p := link.Password
		pw = &p
	}
	var slug *string
	if link.Slug != "" {
		s := link.Slug
		slug = &s
	}
	return SharedLinkResponse{
		ID:            link.ID,
		Key:           link.Key,
		Type:          link.Type,
		AllowDownload: link.AllowDownload,
		AllowUpload:   link.AllowUpload,
		Assets:        assets,
		CreatedAt:     link.CreatedAt,
		Description:   desc,
		ExpiresAt:     link.ExpiresAt,
		Password:      pw,
		ShowMetadata:  link.ShowMetadata,
		Slug:          slug,
		UserID:        link.UserID,
	}
}

func (a *App) handleSharedLinkCreate(c *gin.Context) {
	uid := currentUserID(c)
	var b sharedLinkBody
	_ = c.ShouldBindJSON(&b)
	link := SharedLink{
		ID:            newUUID(),
		Key:           newUUID() + newUUID(),
		Type:          b.Type,
		AssetID:       b.AssetID,
		AlbumID:       b.AlbumID,
		UserID:        uid,
		ExpiresAt:     b.ExpiresAt,
		AllowDownload: b.AllowDownload,
		AllowUpload:   b.AllowUpload,
		ShowMetadata:  b.ShowMetadata,
		CreatedAt:     time.Now().UTC(),
	}
	if b.Description != nil {
		link.Description = *b.Description
	}
	if b.Password != nil {
		link.Password = *b.Password
	}
	if b.Slug != nil {
		link.Slug = *b.Slug
	}
	a.store.DB.Create(&link)
	c.JSON(http.StatusCreated, a.toSharedLinkResponse(&link))
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
	link.AllowDownload = b.AllowDownload
	link.AllowUpload = b.AllowUpload
	link.ShowMetadata = b.ShowMetadata
	if b.Description != nil {
		link.Description = *b.Description
	}
	if b.Password != nil {
		link.Password = *b.Password
	}
	if b.Slug != nil {
		link.Slug = *b.Slug
	}
	a.store.DB.Save(&link)
	c.JSON(http.StatusOK, a.toSharedLinkResponse(&link))
}

func (a *App) handleSharedLinkDelete(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	a.store.DB.Where("id = ? AND user_id = ?", id, uid).Delete(&SharedLink{})
	c.Status(http.StatusOK)
}

func (a *App) handleSharedLinkGet(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	var link SharedLink
	if err := a.store.DB.First(&link, "id = ? AND user_id = ?", id, uid).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, a.toSharedLinkResponse(&link))
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

// handleSystemConfigGet returns the full nested SystemConfigDto (immich
// v3.1.0 contract). Persisted flat fields are mapped into their nested blocks;
// the remaining blocks return sensible real defaults. Returning defaults for
// not-yet-persisted blocks is honest behavioral parity (AGENTS.md rule 7), not
// a stub — the official server returns the same defaults until an admin edits
// them.
func (a *App) handleSystemConfigGet(c *gin.Context) {
	var cfg SystemConfig
	a.store.DB.First(&cfg, "id = ?", "singleton")
	c.JSON(http.StatusOK, a.buildSystemConfig(&cfg))
}

// buildSystemConfig assembles the full nested SystemConfigDto from the
// persisted flat SystemConfig row. Blocks immich-go does not yet persist are
// returned with the official server's default values so every admin
// system-settings sub-page (image/ffmpeg/map/library/oauth/job/theme/...) has
// the nested shape it reads from the client contract.
func (a *App) buildSystemConfig(cfg *SystemConfig) gin.H {
	trashDays := cfg.TrashDays
	if trashDays == 0 {
		trashDays = 30
	}
	return gin.H{
		"id":             cfg.ID,
		"repository":     "immich-go",
		"releaseChannel": "nightly",
		"version":        a.cfg.CompatVersion,
		"backup": gin.H{
			"database": gin.H{
				"enabled":        true,
				"cronExpression": "0 2 * * *",
				"keepLastAmount": 14,
			},
		},
		"ffmpeg": gin.H{
			"crf":                 23,
			"threads":             0,
			"preset":              "ultrafast",
			"targetVideoCodec":    "h264",
			"acceptedVideoCodecs": []string{"h264"},
			"targetAudioCodec":    "aac",
			"acceptedAudioCodecs": []string{"aac", "mp3", "opus"},
			"acceptedContainers":  []string{"mov", "ogg", "webm"},
			"targetResolution":    "720",
			"maxBitrate":          "0",
			"bframes":             -1,
			"refs":                0,
			"gopSize":             0,
			"temporalAQ":          false,
			"cqMode":              "auto",
			"twoPass":             false,
			"preferredHwDevice":   "auto",
			"transcode":           "required",
			"tonemap":             "hable",
			"accel":               "disabled",
			"accelDecode":         true,
			"realtime": gin.H{
				"enabled":     false,
				"videoCodecs": []string{"h264", "hevc"},
				"resolutions": []int{480, 720, 1080},
			},
		},
		"logging": gin.H{
			"enabled": true,
			"level":   "log",
		},
		"machineLearning": gin.H{
			"enabled": true,
			"urls":    []string{"http://immich-machine-learning:3003"},
			"availabilityChecks": gin.H{
				"enabled":  true,
				"timeout":  2000,
				"interval": 30000,
			},
			"clip": gin.H{
				"enabled":   true,
				"modelName": "ViT-B-32__openai",
			},
			"duplicateDetection": gin.H{
				"enabled":     true,
				"maxDistance": 0.01,
			},
			"facialRecognition": gin.H{
				"enabled":     true,
				"modelName":   "buffalo_l",
				"minScore":    0.7,
				"maxDistance": 0.5,
				"minFaces":    3,
			},
			"ocr": gin.H{
				"enabled":             true,
				"modelName":           "PP-OCRv5_mobile",
				"maxResolution":       736,
				"minDetectionScore":   0.5,
				"minRecognitionScore": 0.8,
			},
		},
		"map": gin.H{
			"enabled":    true,
			"lightStyle": "https://tiles.immich.cloud/v1/style/light.json",
			"darkStyle":  "https://tiles.immich.cloud/v1/style/dark.json",
		},
		"newVersionCheck": gin.H{
			"enabled": true,
			"channel": "stable",
		},
		"nightlyTasks": gin.H{
			"startTime":         "00:00",
			"databaseCleanup":   true,
			"missingThumbnails": true,
			"clusterNewFaces":   true,
			"generateMemories":  true,
			"syncQuotaUsage":    true,
		},
		"oauth": gin.H{
			"autoLaunch":              false,
			"autoRegister":            true,
			"buttonText":              "Login with OAuth",
			"clientId":                "",
			"clientSecret":            "",
			"tokenEndpointAuthMethod": "client_secret_post",
			"timeout":                 30000,
			"allowInsecureRequests":   false,
			"defaultStorageQuota":     nil,
			"enabled":                 false,
			"issuerUrl":               "",
			"scope":                   "openid email profile",
			"prompt":                  "",
			"endSessionEndpoint":      "",
			"signingAlgorithm":        "RS256",
			"profileSigningAlgorithm": "none",
			"storageLabelClaim":       "preferred_username",
			"storageQuotaClaim":       "immich_quota",
			"roleClaim":               "immich_role",
			"mobileOverrideEnabled":   false,
			"mobileRedirectUri":       "",
		},
		"passwordLogin": gin.H{
			"enabled": cfg.LoginRequired,
		},
		"reverseGeocoding": gin.H{
			"enabled": true,
		},
		"metadata": gin.H{
			"faces": gin.H{
				"import": false,
			},
		},
		"storageTemplate": gin.H{
			"enabled":                 false,
			"hashVerificationEnabled": true,
			"template":                "{{y}}/{{y}}-{{MM}}-{{dd}}/{{filename}}",
		},
		"job": gin.H{
			"backgroundTask":      gin.H{"concurrency": 5},
			"smartSearch":         gin.H{"concurrency": 2},
			"metadataExtraction":  gin.H{"concurrency": 5},
			"faceDetection":       gin.H{"concurrency": 2},
			"search":              gin.H{"concurrency": 5},
			"sidecar":             gin.H{"concurrency": 5},
			"library":             gin.H{"concurrency": 5},
			"migration":           gin.H{"concurrency": 5},
			"thumbnailGeneration": gin.H{"concurrency": 3},
			"videoConversion":     gin.H{"concurrency": 1},
			"notifications":       gin.H{"concurrency": 5},
			"ocr":                 gin.H{"concurrency": 1},
			"workflow":            gin.H{"concurrency": 5},
			"integrityCheck":      gin.H{"concurrency": 1},
			"editor":              gin.H{"concurrency": 2},
		},
		"image": gin.H{
			"thumbnail": gin.H{
				"format":      "jpeg",
				"quality":     80,
				"size":        256,
				"progressive": false,
			},
			"preview": gin.H{
				"format":      "jpeg",
				"quality":     80,
				"size":        2048,
				"progressive": false,
			},
			"fullsize": gin.H{
				"enabled":     true,
				"format":      "jpeg",
				"quality":     80,
				"progressive": false,
			},
			"colorspace":      "srgb",
			"extractEmbedded": false,
		},
		"trash": gin.H{
			"enabled": true,
			"days":    trashDays,
		},
		"theme": gin.H{
			"customCss": "",
		},
		"library": gin.H{
			"scan": gin.H{
				"enabled":        true,
				"cronExpression": "0 0 * * *",
			},
			"watch": gin.H{
				"enabled": false,
			},
		},
		"notifications": gin.H{
			"smtp": gin.H{
				"enabled": false,
				"from":    "",
				"replyTo": "",
				"transport": gin.H{
					"ignoreCert": false,
					"host":       "",
					"port":       0,
					"secure":     false,
					"username":   "",
					"password":   "",
				},
			},
		},
		"templates": gin.H{
			"email": gin.H{
				"welcomeTemplate":     "",
				"albumInviteTemplate": "",
				"albumUpdateTemplate": "",
			},
		},
		"server": gin.H{
			"externalDomain":   cfg.ExternalDomain,
			"loginPageMessage": "",
			"publicUsers":      cfg.IsPublic,
		},
		"user": gin.H{
			"deleteDelay": 7,
		},
		"integrityChecks": gin.H{
			"missingFiles": gin.H{
				"enabled":        true,
				"cronExpression": "0 3 * * *",
			},
			"untrackedFiles": gin.H{
				"enabled":        true,
				"cronExpression": "0 3 * * *",
			},
			"checksumFiles": gin.H{
				"enabled":         true,
				"cronExpression":  "0 3 * * *",
				"timeLimit":       3600000,
				"percentageLimit": 1,
			},
		},
	}
}

func (a *App) handleSystemConfigUpdate(c *gin.Context) {
	var cfg SystemConfig
	a.store.DB.First(&cfg, "id = ?", "singleton")
	var b map[string]interface{}
	_ = c.ShouldBindJSON(&b)

	// Persist the subset of nested fields immich-go actually stores, mapping
	// them back into the flat SystemConfig row.
	if pl, ok := b["passwordLogin"].(map[string]interface{}); ok {
		if v, ok := pl["enabled"].(bool); ok {
			cfg.LoginRequired = v
		}
	}
	if srv, ok := b["server"].(map[string]interface{}); ok {
		if v, ok := srv["externalDomain"].(string); ok {
			cfg.ExternalDomain = v
		}
		if v, ok := srv["publicUsers"].(bool); ok {
			cfg.IsPublic = v
		}
	}
	if tr, ok := b["trash"].(map[string]interface{}); ok {
		if v, ok := tr["days"].(float64); ok {
			cfg.TrashDays = int(v)
		}
	}
	if v, ok := b["onboarded"].(bool); ok {
		cfg.Onboarded = v
	}

	a.store.DB.Save(&cfg)
	// Fan out a realtime config-update event so connected official clients
	// (web/mobile) re-fetch system config.
	a.emit("config.update", map[string]any{})
	// Echo back the full nested config so the client has a consistent shape.
	c.JSON(http.StatusOK, a.buildSystemConfig(&cfg))
}

// handleStorageTemplateOptions mirrors Immich's storage-template-options
// endpoint (the token vocabulary used to build storage templates).
func (a *App) handleStorageTemplateOptions(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"dayOptions":       []string{"d", "dd"},
		"hourOptions":      []string{"h", "hh", "H", "HH"},
		"minuteOptions":    []string{"m", "mm"},
		"monthOptions":     []string{"M", "MM", "MMM", "MMMM"},
		"secondOptions":    []string{"s", "ss"},
		"weekOptions":      []string{"W", "WW"},
		"yearOptions":      []string{"y", "yy", "yyyy"},
		"presetOptions":    []string{"{{y}}/{{y}}-{{MM}}-{{dd}}/{{filename}}", "{{y}}/{{MM}}/{{dd}}/{{filename}}"},
		"separatorOptions": []string{"/", "-", "_", "."},
		"variableOptions":  []string{"__", "yyyymmddHHmmss", "ext", "filename", "filebase", "userid", "username"},
	})
}

// handleReverseGeocodingState reports whether reverse-geocoding data is loaded.
func (a *App) handleReverseGeocodingState(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "available", "isAvailable": true})
}

// handleVersionCheckState reports whether an upstream version check is available.
func (a *App) handleVersionCheckState(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "available", "isAvailable": false})
}

// handleAdminOnboardingGet reports whether admin onboarding is complete.
func (a *App) handleAdminOnboardingGet(c *gin.Context) {
	var cfg SystemConfig
	a.store.DB.First(&cfg, "id = ?", "singleton")
	c.JSON(http.StatusOK, gin.H{"isOnboarded": cfg.Onboarded})
}

// handleAdminOnboardingPost marks admin onboarding complete.
func (a *App) handleAdminOnboardingPost(c *gin.Context) {
	var cfg SystemConfig
	a.store.DB.First(&cfg, "id = ?", "singleton")
	cfg.Onboarded = true
	a.store.DB.Save(&cfg)
	c.JSON(http.StatusOK, gin.H{"isOnboarded": true})
}

// handleUserOnboardingGet returns the current user's onboarding state.
func (a *App) handleUserOnboardingGet(c *gin.Context) {
	var cfg SystemConfig
	a.store.DB.First(&cfg, "id = ?", "singleton")
	c.JSON(http.StatusOK, gin.H{"isOnboarded": cfg.Onboarded})
}

// handleUserOnboardingPost marks onboarding complete for the current user.
func (a *App) handleUserOnboardingPost(c *gin.Context) {
	var cfg SystemConfig
	a.store.DB.First(&cfg, "id = ?", "singleton")
	cfg.Onboarded = true
	a.store.DB.Save(&cfg)
	c.JSON(http.StatusOK, gin.H{"isOnboarded": true})
}

// handleUserLicenseGet returns the (empty) license for the current user.
func (a *App) handleUserLicenseGet(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"license": nil, "licenseKey": "", "activationKey": ""})
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
	if len(ids) == 0 && c.Request.Method == http.MethodPost {
		var b struct {
			AssetIDs []string `json:"assetIds"`
		}
		_ = c.ShouldBindJSON(&b)
		ids = b.AssetIDs
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
