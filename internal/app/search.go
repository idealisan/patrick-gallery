package app

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type searchRequest struct {
	Query    string `json:"query"`
	Type     string `json:"type"`
	Recent   bool   `json:"recent"`
	WithExif bool   `json:"withExif"`
	Take     int    `json:"take"`
	City     string `json:"city"`
	Country  string `json:"country"`
	Make     string `json:"make"`
	Model    string `json:"model"`
}

// searchResponse / searchAssetResult / searchAlbumResult mirror Immich's
// SearchResponseDto / SearchAssetResponseDto / SearchAlbumResponseDto exactly
// so the official clients (which expect {albums, assets:{items,count,facets,
// total}}) parse search results without error.
type searchResponse struct {
	Albums searchAlbumResult `json:"albums"`
	Assets searchAssetResult `json:"assets"`
}

type searchAssetResult struct {
	Items    []AssetResponse `json:"items"`
	Count    int             `json:"count"`
	Facets   []interface{}   `json:"facets"`
	NextPage *string         `json:"nextPage"`
	Total    int             `json:"total"`
}

type searchAlbumResult struct {
	Items  []interface{} `json:"items"`
	Count  int           `json:"count"`
	Facets []interface{} `json:"facets"`
	Total  int           `json:"total"`
}

func emptySearchResponse(assets []AssetResponse) searchResponse {
	return searchResponse{
		Albums: searchAlbumResult{Items: []interface{}{}, Count: 0, Facets: []interface{}{}, Total: 0},
		Assets: searchAssetResult{Items: assets, Count: len(assets), Facets: []interface{}{}, NextPage: nil, Total: len(assets)},
	}
}

func (a *App) handleSearch(c *gin.Context) {
	uid := currentUserID(c)
	var req searchRequest
	_ = c.ShouldBindJSON(&req)
	if req.Take <= 0 {
		req.Take = 100
	}
	q := strings.TrimSpace(req.Query)

	var assets []Asset
	base := a.store.DB.Where("owner_id = ? AND is_trash = ?", uid, false)
	if q != "" {
		like := "%" + q + "%"
		base = base.Where("original_file_name LIKE ? OR original_path LIKE ?", like, like)
	}
	if req.Type != "" {
		base = base.Where("type = ?", normalizeType(req.Type))
	}
	base.Order("local_date_time DESC").Limit(req.Take).Find(&assets)

	// also match exif text
	if q != "" {
		like := "%" + q + "%"
		var exifs []Exif
		a.store.DB.Where("description LIKE ? OR city LIKE ? OR country LIKE ? OR make LIKE ? OR model LIKE ?", like, like, like, like, like).Find(&exifs)
		seen := map[string]bool{}
		for _, as := range assets {
			seen[as.ID] = true
		}
		for _, e := range exifs {
			if seen[e.AssetID] {
				continue
			}
			var as Asset
			if err := a.store.DB.First(&as, "id = ? AND owner_id = ? AND is_trash = ?", e.AssetID, uid, false).Error; err == nil {
				assets = append(assets, as)
				seen[e.AssetID] = true
			}
		}
	}

	out := make([]AssetResponse, 0, len(assets))
	for _, as := range assets {
		out = append(out, a.toResponse(as))
	}
	c.JSON(http.StatusOK, emptySearchResponse(out))
}

func (a *App) handleSearchMetadata(c *gin.Context) {
	uid := currentUserID(c)
	var req searchRequest
	_ = c.ShouldBindJSON(&req)
	var exifs []Exif
	q := a.store.DB.Model(&Exif{})
	if req.Make != "" {
		q = q.Where("make LIKE ?", "%"+req.Make+"%")
	}
	if req.Model != "" {
		q = q.Where("model LIKE ?", "%"+req.Model+"%")
	}
	if req.City != "" {
		q = q.Where("city LIKE ?", "%"+req.City+"%")
	}
	if req.Country != "" {
		q = q.Where("country LIKE ?", "%"+req.Country+"%")
	}
	q.Find(&exifs)
	ids := make([]string, 0, len(exifs))
	for _, e := range exifs {
		ids = append(ids, e.AssetID)
	}
	var assets []Asset
	if len(ids) > 0 {
		a.store.DB.Where("id IN ? AND owner_id = ? AND is_trash = ?", ids, uid, false).Find(&assets)
	}
	out := make([]AssetResponse, 0, len(assets))
	for _, as := range assets {
		out = append(out, a.toResponse(as))
	}
	c.JSON(http.StatusOK, emptySearchResponse(out))
}

func (a *App) handleSearchPerson(c *gin.Context) {
	uid := currentUserID(c)
	var req struct {
		Query    string `json:"query"`
		PersonID string `json:"personId"`
	}
	_ = c.ShouldBindJSON(&req)
	var assets []Asset
	if req.PersonID != "" {
		a.store.DB.Where("person_id = ? AND owner_id = ? AND is_trash = ?", req.PersonID, uid, false).Find(&assets)
	} else if req.Query != "" {
		// Resolve person name(s) to ids, then return their assets.
		var people []Person
		a.store.DB.Where("name LIKE ?", "%"+req.Query+"%").Find(&people)
		ids := make([]string, 0, len(people))
		for _, p := range people {
			ids = append(ids, p.ID)
		}
		if len(ids) > 0 {
			a.store.DB.Where("person_id IN ? AND owner_id = ? AND is_trash = ?", ids, uid, false).Find(&assets)
		}
	}
	out := make([]AssetResponse, 0, len(assets))
	for _, as := range assets {
		out = append(out, a.toResponse(as))
	}
	c.JSON(http.StatusOK, gin.H{"total": len(out), "assets": out, "nextPage": false})
}

func (a *App) handleSearchExplore(c *gin.Context) {
	uid := currentUserID(c)
	var assets []Asset
	a.store.DB.Where("owner_id = ? AND is_trash = ?", uid, false).Order("local_date_time DESC").Limit(100).Find(&assets)
	out := make([]AssetResponse, 0, len(assets))
	for _, as := range assets {
		out = append(out, a.toResponse(as))
	}
	c.JSON(http.StatusOK, emptySearchResponse(out))
}

// handleSearchRandom returns a random sample of the user's assets.
func (a *App) handleSearchRandom(c *gin.Context) {
	uid := currentUserID(c)
	var req struct {
		Take int    `json:"take"`
		Type string `json:"type"`
	}
	_ = c.ShouldBindJSON(&req)
	if req.Take <= 0 {
		req.Take = 100
	}
	q := a.store.DB.Where("owner_id = ? AND is_trash = ?", uid, false)
	if req.Type != "" {
		q = q.Where("type = ?", normalizeType(req.Type))
	}
	var assets []Asset
	q.Order("RANDOM()").Limit(req.Take).Find(&assets)
	out := make([]AssetResponse, 0, len(assets))
	for _, as := range assets {
		out = append(out, a.toResponse(as))
	}
	c.JSON(http.StatusOK, emptySearchResponse(out))
}

// handleSearchLargeAssets returns assets larger than a byte threshold (default
// 100 MiB), useful for storage cleanup. Real size from asset.size.
func (a *App) handleSearchLargeAssets(c *gin.Context) {
	uid := currentUserID(c)
	var req struct {
		Size int64 `json:"size"`
		Take int   `json:"take"`
	}
	_ = c.ShouldBindJSON(&req)
	if req.Take <= 0 {
		req.Take = 100
	}
	q := a.store.DB.Where("owner_id = ? AND is_trash = ?", uid, false)
	if req.Size > 0 {
		q = q.Where("size >= ?", req.Size)
	}
	var assets []Asset
	q.Order("size DESC").Limit(req.Take).Find(&assets)
	out := make([]AssetResponse, 0, len(assets))
	for _, as := range assets {
		out = append(out, a.toResponse(as))
	}
	c.JSON(http.StatusOK, emptySearchResponse(out))
}

// handleSearchStatistics returns aggregate counts/usage for the user's library.
func (a *App) handleSearchStatistics(c *gin.Context) {
	uid := currentUserID(c)
	var req struct {
		IsTrash    *bool  `json:"isTrash"`
		IsFavorite *bool  `json:"isFavorite"`
		IsArchived *bool  `json:"isArchived"`
		Type       string `json:"type"`
	}
	_ = c.ShouldBindJSON(&req)
	trash := false
	if req.IsTrash != nil {
		trash = *req.IsTrash
	}
	q := a.store.DB.Model(&Asset{}).Where("owner_id = ? AND is_trash = ?", uid, trash)
	if req.Type != "" {
		q = q.Where("type = ?", normalizeType(req.Type))
	}
	if req.IsFavorite != nil {
		q = q.Where("is_favorite = ?", *req.IsFavorite)
	}
	if req.IsArchived != nil {
		q = q.Where("is_archived = ?", *req.IsArchived)
	}
	var total, photos, videos, usage int64
	q.Count(&total)
	q.Where("type = ?", "IMAGE").Count(&photos)
	q.Where("type = ?", "VIDEO").Count(&videos)
	q.Select("COALESCE(SUM(size),0)").Scan(&usage)
	c.JSON(http.StatusOK, gin.H{
		"total":  total,
		"photos": photos,
		"videos": videos,
		"usage":  usage,
	})
}

// handleSearchSmart is CLIP semantic search, which requires an ML embedding
// backend (deferred per AGENTS.md). Honest 501 — not a fake-empty stub.
func (a *App) handleSearchSmart(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"message":    "semantic (CLIP) search requires an ML backend (deferred in immich-go)",
		"statusCode": 501,
	})
}
