package app

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type searchRequest struct {
	Query            string   `json:"query"`
	OriginalFileName string   `json:"originalFileName"`
	Description      string   `json:"description"`
	OCR              string   `json:"ocr"`
	Type             string   `json:"type"`
	Recent           bool     `json:"recent"`
	WithExif         bool     `json:"withExif"`
	Take             int      `json:"take"`
	City             string   `json:"city"`
	Country          string   `json:"country"`
	State            string   `json:"state"`
	Make             string   `json:"make"`
	Model            string   `json:"model"`
	AlbumIDs         []string `json:"albumIds"`
	PersonIDs        []string `json:"personIds"`
	TagIDs           []string `json:"tagIds"`
	IsFavorite       *bool    `json:"isFavorite"`
	IsNotInAlbum     *bool    `json:"isNotInAlbum"`
	IsTrash          *bool    `json:"isTrash"`
	Visibility       string   `json:"visibility"`
	TakenAfter       string   `json:"takenAfter"`
	TakenBefore      string   `json:"takenBefore"`
	Rating           *int     `json:"rating"`
	Page             int      `json:"page"`
	Size             int      `json:"size"`
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
	items := assets
	if items == nil {
		items = []AssetResponse{}
	}
	return searchResponse{
		Albums: searchAlbumResult{Items: []interface{}{}, Count: 0, Facets: []interface{}{}, Total: 0},
		Assets: searchAssetResult{Items: items, Count: len(items), Facets: []interface{}{}, NextPage: nil, Total: len(items)},
	}
}

func searchResponseForAssets(assets []AssetResponse, page, size int, hasMore bool) searchResponse {
	response := emptySearchResponse(assets)
	if hasMore {
		next := fmt.Sprintf("%d", page+1)
		response.Assets.NextPage = &next
	}
	return response
}

func (a *App) handleSearch(c *gin.Context) {
	uid := currentUserID(c)
	var req searchRequest
	_ = c.ShouldBindJSON(&req)
	if req.Take <= 0 {
		req.Take = 100
	}
	q := strings.TrimSpace(req.Query)

	// Honor the isTrash flag (the trash gallery queries with isTrashed:true).
	// Default to non-trashed when the flag is absent.
	trash := false
	if req.IsTrash != nil {
		trash = *req.IsTrash
	}

	var assets []Asset
	base := a.store.DB.Where("owner_id = ? AND is_trash = ? AND (visibility IS NULL OR visibility = '' OR visibility != 'hidden')", uid, trash)
	if q != "" {
		like := "%" + q + "%"
		base = base.Where("original_file_name LIKE ? OR original_path LIKE ?", like, like)
	}
	if strings.TrimSpace(req.OCR) != "" {
		base = base.Where("assets.id IN (SELECT asset_id FROM asset_ocrs WHERE text LIKE ?)",
			"%"+strings.TrimSpace(req.OCR)+"%")
	}
	if req.Type != "" {
		base = base.Where("type = ?", normalizeType(req.Type))
	}
	base.Order("local_date_time DESC").Limit(req.Take).Find(&assets)

	// also match exif text
	if q != "" {
		like := "%" + q + "%"
		var exif []Exif
		a.store.DB.Where("description LIKE ? OR city LIKE ? OR country LIKE ? OR make LIKE ? OR model LIKE ?", like, like, like, like, like).Find(&exif)
		seen := map[string]bool{}
		for _, as := range assets {
			seen[as.ID] = true
		}
		for _, e := range exif {
			if seen[e.AssetID] {
				continue
			}
			var as Asset
			if err := a.store.DB.First(&as, "id = ? AND owner_id = ? AND is_trash = ?", e.AssetID, uid, trash).Error; err == nil {
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
	trash := false
	if req.IsTrash != nil {
		trash = *req.IsTrash
	}
	var assets []Asset
	assetQuery := a.store.DB.Where("assets.owner_id = ? AND assets.is_trash = ? AND (assets.visibility IS NULL OR assets.visibility = '' OR assets.visibility != 'hidden')", uid, trash)
	if query := strings.TrimSpace(req.Query); query != "" {
		like := "%" + query + "%"
		assetQuery = assetQuery.Where(`(
			assets.original_file_name LIKE ? OR
			assets.original_path LIKE ? OR
			assets.id IN (SELECT asset_id FROM exif WHERE description LIKE ?) OR
			assets.id IN (SELECT asset_id FROM asset_ocrs WHERE text LIKE ?)
		)`, like, like, like, like)
	}
	if req.OriginalFileName != "" {
		assetQuery = assetQuery.Where("assets.original_file_name LIKE ?", "%"+req.OriginalFileName+"%")
	}
	if req.Description != "" {
		assetQuery = assetQuery.Where("assets.id IN (SELECT asset_id FROM exif WHERE description LIKE ?)", "%"+req.Description+"%")
	}
	if req.OCR != "" {
		assetQuery = assetQuery.Where("assets.id IN (SELECT asset_id FROM asset_ocrs WHERE text LIKE ?)", "%"+req.OCR+"%")
	}
	if req.Make != "" {
		assetQuery = assetQuery.Where("assets.id IN (SELECT asset_id FROM exif WHERE make LIKE ?)", "%"+req.Make+"%")
	}
	if req.Model != "" {
		assetQuery = assetQuery.Where("assets.id IN (SELECT asset_id FROM exif WHERE model LIKE ?)", "%"+req.Model+"%")
	}
	if req.City != "" {
		assetQuery = assetQuery.Where("assets.id IN (SELECT asset_id FROM exif WHERE city LIKE ?)", "%"+req.City+"%")
	}
	if req.Country != "" {
		assetQuery = assetQuery.Where("assets.id IN (SELECT asset_id FROM exif WHERE country LIKE ?)", "%"+req.Country+"%")
	}
	if req.State != "" {
		assetQuery = assetQuery.Where("assets.id IN (SELECT asset_id FROM exif WHERE state LIKE ?)", "%"+req.State+"%")
	}
	if req.Type != "" {
		assetQuery = assetQuery.Where("assets.type = ?", normalizeType(req.Type))
	}
	if req.IsFavorite != nil {
		assetQuery = assetQuery.Where("assets.is_favorite = ?", *req.IsFavorite)
	}
	if req.Visibility == "archive" {
		assetQuery = assetQuery.Where("assets.is_archived = ?", true)
	}
	if req.Visibility == "timeline" {
		assetQuery = assetQuery.Where("assets.is_archived = ?", false)
	}
	if len(req.PersonIDs) > 0 {
		assetQuery = assetQuery.Where("assets.person_id IN ?", req.PersonIDs)
	}
	if len(req.TagIDs) > 0 {
		assetQuery = assetQuery.Where("assets.id IN (SELECT asset_id FROM tags_assets WHERE tag_id IN ?)", req.TagIDs)
	}
	if len(req.AlbumIDs) > 0 {
		assetQuery = assetQuery.Where("assets.id IN (SELECT asset_id FROM albums_assets_assets WHERE album_id IN ?)", req.AlbumIDs)
	}
	if req.IsNotInAlbum != nil && *req.IsNotInAlbum {
		assetQuery = assetQuery.Where("NOT EXISTS (SELECT 1 FROM albums_assets_assets aa WHERE aa.asset_id = assets.id)")
	}
	if req.TakenAfter != "" {
		assetQuery = assetQuery.Where("assets.local_date_time >= ?", req.TakenAfter)
	}
	if req.TakenBefore != "" {
		assetQuery = assetQuery.Where("assets.local_date_time <= ?", req.TakenBefore)
	}
	if req.Rating != nil {
		if *req.Rating == 0 {
			assetQuery = assetQuery.Where("assets.id IN (SELECT asset_id FROM exif WHERE rating IS NULL)")
		} else {
			assetQuery = assetQuery.Where("assets.id IN (SELECT asset_id FROM exif WHERE rating = ?)", *req.Rating)
		}
	}
	page := req.Page
	if page < 1 {
		page = 1
	}
	size := req.Size
	if size < 1 || size > 1000 {
		size = 1000
	}
	assetQuery.Order("assets.local_date_time DESC").Offset((page - 1) * size).Limit(size + 1).Find(&assets)
	hasMore := len(assets) > size
	if hasMore {
		assets = assets[:size]
	}
	out := make([]AssetResponse, 0, len(assets))
	for _, as := range assets {
		out = append(out, a.toResponse(as))
	}
	c.JSON(http.StatusOK, searchResponseForAssets(out, page, size, hasMore))
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
	c.JSON(http.StatusOK, out)
}

// handleSearchLargeAssets returns assets larger than a byte threshold (default
// 100 MiB), useful for storage cleanup. Real size from asset.size.
func (a *App) handleSearchLargeAssets(c *gin.Context) {
	uid := currentUserID(c)
	var req struct {
		Size        int64 `json:"size"`
		MinFileSize int64 `json:"minFileSize"`
		Take        int   `json:"take"`
	}
	_ = c.ShouldBindJSON(&req)
	if req.Take <= 0 {
		req.Take = 100
	}
	// The official web sends minFileSize; honor both it and the legacy size key.
	minSize := req.MinFileSize
	if minSize == 0 {
		minSize = req.Size
	}
	q := a.store.DB.Where("owner_id = ? AND is_trash = ?", uid, false)
	if minSize > 0 {
		q = q.Where("size >= ?", minSize)
	}
	var assets []Asset
	q.Order("size DESC").Limit(req.Take).Find(&assets)
	out := make([]AssetResponse, 0, len(assets))
	for _, as := range assets {
		out = append(out, a.toResponse(as))
	}
	c.JSON(http.StatusOK, out)
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

// handleSearchSmart performs "thing" search. Images are described in the
// background by the configured LLM (POST /api/jobs smartSearch), and the query
// is matched as a substring against that description, the asset filename, and
// the OCR text. When no LLM descriptions have been generated yet (or smart
// search is disabled) this degrades gracefully to a plain text search over
// filename + OCR text, so the Web UI "things" facet always returns real results
// instead of a hard 501.
func (a *App) handleSearchSmart(c *gin.Context) {
	uid := currentUserID(c)
	// Official SmartSearchDto: {query} drives semantic search while {ocr}
	// (BaseSearchSchema) filters by recognized text; both may arrive on the
	// same endpoint depending on the web query type.
	var req struct {
		Query         string `json:"query"`
		OCR           string `json:"ocr"`
		QueryAssetID  string `json:"queryAssetId"`
		Type          string `json:"type"`
		IsFavorite    *bool  `json:"isFavorite"`
		Visibility    string `json:"visibility"`
		Page          int    `json:"page"`
		Size          int    `json:"size"`
	}
	_ = c.ShouldBindJSON(&req)
	if req.Query == "" && req.OCR == "" && req.QueryAssetID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query is required", "statusCode": 400})
		return
	}
	if req.Size <= 0 {
		req.Size = 100
	}
	q := a.store.DB.Where("assets.owner_id = ? AND assets.is_trash = ?", uid, false)
	if req.Visibility == "timeline" || req.Visibility == "" {
		q = q.Where("(assets.visibility IS NULL OR assets.visibility = '' OR assets.visibility != 'hidden')")
	} else if req.Visibility != "" {
		q = q.Where("assets.visibility = ?", strings.ToLower(req.Visibility))
	}
	switch {
	case req.OCR != "":
		// OCR text search (web query type "ocr"): match recognized words.
		q = q.Where("assets.id IN (SELECT asset_id FROM asset_ocrs WHERE text LIKE ?)",
			"%"+req.OCR+"%")
	default:
		like := "%" + req.Query + "%"
		q = q.Where(`assets.id IN (
			SELECT asset_id FROM asset_mls WHERE description LIKE ? OR labels_json LIKE ?
			UNION
			SELECT id FROM assets WHERE original_file_name LIKE ?
			UNION
			SELECT asset_id FROM asset_ocrs WHERE text LIKE ?
		)`, like, like, like, like)
	}
	if req.Type != "" {
		q = q.Where("assets.type = ?", normalizeType(req.Type))
	}
	if req.IsFavorite != nil {
		q = q.Where("assets.is_favorite = ?", *req.IsFavorite)
	}
	var assets []Asset
	if err := q.Order("assets.local_date_time DESC").Limit(req.Size).Find(&assets).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := make([]AssetResponse, 0, len(assets))
	for _, asset := range assets {
		out = append(out, a.toResponse(asset))
	}
	c.JSON(http.StatusOK, emptySearchResponse(out))
}
