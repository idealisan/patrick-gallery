package app

import (
	"log"
	"net/http"
	"os"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
)

// Duplicates utility, mirroring the official duplicate.service.ts contract:
//
//	GET    /duplicates          → groups with {duplicateId, assets, suggestedKeepAssetIds}
//	POST   /duplicates/resolve  → trash/keep per group, BulkIdResponseDto[]
//	DELETE /duplicates {ids}    → clear the named duplicate groups
//
// Group membership lives on assets.duplicate_id (assigned by the real
// checksum-based duplicateDetection job).

// exifRichness counts populated EXIF fields — the official keep-suggestion
// tie-breaker ("largest count of EXIF data").
func (a *App) exifRichness(exifID string) int {
	if exifID == "" {
		return 0
	}
	var e Exif
	if err := a.store.DB.First(&e, "id = ?", exifID).Error; err != nil {
		return 0
	}
	n := 0
	if e.Make != "" {
		n++
	}
	if e.Model != "" {
		n++
	}
	if e.DateTimeOriginal != nil && *e.DateTimeOriginal != "" {
		n++
	}
	if e.ExposureTime != "" {
		n++
	}
	if e.FNumber != 0 {
		n++
	}
	if e.ISO != 0 {
		n++
	}
	if e.FocalLength != 0 {
		n++
	}
	if e.Latitude != 0 || e.Longitude != 0 {
		n++
	}
	if e.City != "" {
		n++
	}
	if e.State != "" {
		n++
	}
	if e.Country != "" {
		n++
	}
	if e.Description != "" {
		n++
	}
	if e.Orientation != nil {
		n++
	}
	if e.Rating != nil {
		n++
	}
	return n
}

func (a *App) fileSizeOf(p string) int64 {
	info, err := os.Stat(p)
	if err != nil {
		return 0
	}
	return info.Size()
}

// suggestKeepAssetIds ports suggestDuplicateKeepAssetIds: largest file size
// first, most EXIF fields as tie-breaker; all tied best assets are returned.
func (a *App) suggestKeepAssetIds(assets []Asset) []string {
	if len(assets) == 0 {
		return []string{}
	}
	type scored struct {
		id   string
		size int64
		exif int
	}
	list := make([]scored, 0, len(assets))
	for _, as := range assets {
		list = append(list, scored{as.ID, a.fileSizeOf(as.OriginalPath), a.exifRichness(as.ExifID)})
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].size < list[j].size })
	best := list[len(list)-1].size
	tied := make([]scored, 0)
	for _, s := range list {
		if s.size == best {
			tied = append(tied, s)
		}
	}
	maxExif := 0
	for _, s := range tied {
		if s.exif > maxExif {
			maxExif = s.exif
		}
	}
	out := make([]string, 0, len(tied))
	for _, s := range tied {
		if s.exif == maxExif {
			out = append(out, s.id)
		}
	}
	return out
}

// cleanupSingletonGroups clears assignments whose group has fewer than two
// live members (official cleanupSingletonGroups).
func (a *App) cleanupSingletonGroups() {
	type g struct {
		DuplicateID string
		Cnt         int64
	}
	var groups []g
	a.store.DB.Model(&Asset{}).
		Select("duplicate_id as duplicate_id, count(*) as cnt").
		Where("duplicate_id != '' AND is_trash = ?", false).
		Group("duplicate_id").Scan(&groups)
	for _, gr := range groups {
		if gr.Cnt < 2 {
			a.store.DB.Model(&Asset{}).Where("duplicate_id = ?", gr.DuplicateID).
				Update("duplicate_id", "")
		}
	}
}

// handleDuplicatesList mirrors GET /api/duplicates.
func (a *App) handleDuplicatesList(c *gin.Context) {
	uid := currentUserID(c)
	a.cleanupSingletonGroups()
	var ids []string
	a.store.DB.Model(&Asset{}).
		Where("duplicate_id != '' AND is_trash = ? AND owner_id = ?", false, uid).
		Distinct("duplicate_id").Pluck("duplicate_id", &ids)
	out := make([]gin.H, 0, len(ids))
	for _, gid := range ids {
		var assets []Asset
		a.store.DB.Where("duplicate_id = ? AND is_trash = ?", gid, false).
			Order("file_created_at ASC").Find(&assets)
		resp := make([]AssetResponse, 0, len(assets))
		for i := range assets {
			resp = append(resp, a.toResponse(assets[i]))
		}
		out = append(out, gin.H{
			"duplicateId":           gid,
			"assets":                resp,
			"suggestedKeepAssetIds": a.suggestKeepAssetIds(assets),
		})
	}
	c.JSON(http.StatusOK, out)
}

// handleDuplicatesResolve mirrors POST /api/duplicates/resolve: validate each
// group (membership + completeness), trash the losers, dissolve the group,
// and report per-group BulkIdResponseDto results.
func (a *App) handleDuplicatesResolve(c *gin.Context) {
	uid := currentUserID(c)
	var body struct {
		Groups []struct {
			DuplicateID   string   `json:"duplicateId"`
			KeepAssetIds  []string `json:"keepAssetIds"`
			TrashAssetIds []string `json:"trashAssetIds"`
		} `json:"groups"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || len(body.Groups) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "groups required", "statusCode": 400})
		return
	}
	results := make([]gin.H, 0, len(body.Groups))
	now := time.Now().UTC()
	for _, g := range body.Groups {
		var members []Asset
		a.store.DB.Where("duplicate_id = ? AND owner_id = ?", g.DuplicateID, uid).Find(&members)
		if len(members) == 0 {
			results = append(results, gin.H{"id": g.DuplicateID, "success": false, "error": "not_found"})
			continue
		}
		inGroup := map[string]bool{}
		for _, m := range members {
			inGroup[m.ID] = true
		}
		keepSet := map[string]bool{}
		for _, id := range g.KeepAssetIds {
			if inGroup[id] {
				keepSet[id] = true
			}
		}
		trashSet := map[string]bool{}
		for _, id := range g.TrashAssetIds {
			if inGroup[id] {
				trashSet[id] = true
			}
		}
		bad := false
		for _, m := range members {
			if keepSet[m.ID] && trashSet[m.ID] {
				results = append(results, gin.H{"id": g.DuplicateID, "success": false, "error": "validation",
					"errorMessage": "An asset cannot be in both keepAssetIds and trashAssetIds"})
				bad = true
				break
			}
			if !keepSet[m.ID] && !trashSet[m.ID] {
				results = append(results, gin.H{"id": g.DuplicateID, "success": false, "error": "validation",
					"errorMessage": "Every asset must be in either keepAssetIds or trashAssetIds"})
				bad = true
				break
			}
		}
		if bad {
			continue
		}
		trashIDs := make([]string, 0, len(g.TrashAssetIds))
		for _, m := range members {
			if trashSet[m.ID] {
				trashIDs = append(trashIDs, m.ID)
			}
		}
		if len(trashIDs) > 0 {
			if err := a.store.DB.Model(&Asset{}).Where("id IN ?", trashIDs).Updates(map[string]any{
				"is_trash": true, "trashed_at": &now, "updated_at": now,
			}).Error; err != nil {
				results = append(results, gin.H{"id": g.DuplicateID, "success": false, "error": "unknown"})
				continue
			}
		}
		// Dissolve the group: kept assets return to normal review state.
		a.store.DB.Model(&Asset{}).Where("duplicate_id = ?", g.DuplicateID).Update("duplicate_id", "")
		log.Printf("[duplicates] resolved group %s: kept=%d trashed=%d", g.DuplicateID, len(keepSet), len(trashIDs))
		results = append(results, gin.H{"id": g.DuplicateID, "success": true})
	}
	c.JSON(http.StatusOK, results)
}

// handleDuplicatesDeleteAll mirrors DELETE /api/duplicates {ids: [...]}:
// clear the named duplicate groups so they stop surfacing.
func (a *App) handleDuplicatesDeleteAll(c *gin.Context) {
	uid := currentUserID(c)
	var body struct {
		IDs []string `json:"ids"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || len(body.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "ids required", "statusCode": 400})
		return
	}
	a.store.DB.Model(&Asset{}).
		Where("duplicate_id IN ? AND owner_id = ?", body.IDs, uid).
		Update("duplicate_id", "")
	c.Status(http.StatusOK)
}
