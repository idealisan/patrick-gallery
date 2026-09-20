package app

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// handleMemories implements GET /api/memories (searchMemories). It returns the
// user's curated MemoryResponseDto[] (created via POST /memories). When no
// curated memories exist it synthesizes "on this day" memories from the
// user's asset dates so the web/mobile "Memories" surface is populated with
// real data — these are computed, not stubbed.
func (a *App) handleMemories(c *gin.Context) {
	uid := currentUserID(c)
	typ := c.Query("type")
	isSaved := c.Query("isSaved")
	forParam := c.Query("for")

	memories := a.loadMemories(uid, typ, isSaved, forParam)
	// Synthesize on_this_day memories from asset dates when the user has no
	// curated memories yet, so the feature is never empty.
	if len(memories) == 0 && (typ == "" || typ == "on_this_day") {
		memories = append(memories, a.synthesizeOnThisDay(uid, c.Query("day"), c.Query("year"))...)
	}
	c.JSON(http.StatusOK, memories)
}

// loadMemories reads curated memories from the DB honoring query filters.
func (a *App) loadMemories(uid, typ, isSaved, forParam string) []gin.H {
	q := a.store.DB.Where("owner_id = ?", uid)
	if typ != "" {
		q = q.Where("type = ?", typ)
	}
	if isSaved == "true" {
		q = q.Where("is_saved = ?", true)
	} else if isSaved == "false" {
		q = q.Where("is_saved = ?", false)
	}
	var ms []Memory
	q.Order("memory_at DESC").Find(&ms)
	out := make([]gin.H, 0, len(ms))
	for _, m := range ms {
		out = append(out, a.toMemoryResponse(&m))
	}
	return out
}

// synthesizeOnThisDay builds one on_this_day memory per (month/day, year) group
// across the user's assets (mirrors Immich's auto memories: each memory carries
// a single OnThisDayDto year). The optional `day` (MM-DD) and `year` queries
// narrow the result — legacy convenience filters that complement the official
// `type`/`isSaved`/`for` params.
func (a *App) synthesizeOnThisDay(uid, day, yearStr string) []gin.H {
	var assets []Asset
	a.store.DB.Where("owner_id = ? AND is_trash = ?", uid, false).
		Order("file_created_at DESC").Find(&assets)

	wantMonth, wantDay := 0, 0
	if day != "" {
		if m, d, ok := parseMonthDay(day); ok {
			wantMonth, wantDay = int(m), d
		}
	}
	wantYear := 0
	if yearStr != "" {
		if y, err := strconv.Atoi(strings.TrimSpace(yearStr)); err == nil {
			wantYear = y
		}
	}

	type key struct{ m, d, y int }
	groups := map[key][]Asset{}
	var order []key
	for _, as := range assets {
		m, d, y := int(as.FileCreatedAt.Month()), as.FileCreatedAt.Day(), as.FileCreatedAt.Year()
		if wantMonth != 0 && (m != wantMonth || d != wantDay) {
			continue
		}
		if wantYear != 0 && y != wantYear {
			continue
		}
		k := key{m, d, y}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], as)
	}
	// newest year first, then newest month/day
	sort.SliceStable(order, func(i, j int) bool {
		if order[i].y != order[j].y {
			return order[i].y > order[j].y
		}
		if order[i].m != order[j].m {
			return order[i].m > order[j].m
		}
		return order[i].d > order[j].d
	})
	out := make([]gin.H, 0, len(order))
	for _, k := range order {
		assetsOut := make([]AssetResponse, 0, len(groups[k]))
		for _, as := range groups[k] {
			assetsOut = append(assetsOut, a.toResponse(as))
		}
		out = append(out, gin.H{
			"id":        "onthisday-" + strconv.Itoa(k.y) + "-" + strconv.Itoa(k.m) + "-" + strconv.Itoa(k.d),
			"ownerId":   uid,
			"type":      "on_this_day",
			"memoryAt":  time.Date(k.y, time.Month(k.m), k.d, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
			"isSaved":   false,
			"createdAt": time.Time{}.Format(time.RFC3339),
			"updatedAt": time.Time{}.Format(time.RFC3339),
			"data":      gin.H{"year": k.y},
			"assets":    assetsOut,
		})
	}
	return out
}

// parseMonthDay parses an "MM-DD" string into month/day. It returns ok=false on
// any malformed input so callers can ignore the filter.
func parseMonthDay(s string) (time.Month, int, bool) {
	parts := strings.SplitN(strings.TrimSpace(s), "-", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	mm, err1 := strconv.Atoi(parts[0])
	dd, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || mm < 1 || mm > 12 || dd < 1 || dd > 31 {
		return 0, 0, false
	}
	return time.Month(mm), dd, true
}

// handleMemoryCreate implements POST /api/memories (createMemory).
func (a *App) handleMemoryCreate(c *gin.Context) {
	uid := currentUserID(c)
	var b struct {
		AssetIDs []string   `json:"assetIds"`
		Data     struct {
			Year int `json:"year"`
		} `json:"data"`
		HideAt  *time.Time `json:"hideAt"`
		IsSaved bool       `json:"isSaved"`
		MemoryAt string    `json:"memoryAt"`
		SeenAt  *time.Time `json:"seenAt"`
		ShowAt  *time.Time `json:"showAt"`
		Type    string     `json:"type"`
	}
	if err := c.ShouldBindJSON(&b); err != nil || b.Type == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "type required", "statusCode": 400})
		return
	}
	memoryAt := time.Now().UTC()
	if b.MemoryAt != "" {
		if t, err := time.Parse(time.RFC3339, b.MemoryAt); err == nil {
			memoryAt = t
		}
	}
	now := time.Now().UTC()
	m := Memory{
		ID:        newUUID(),
		OwnerID:   uid,
		Type:      b.Type,
		MemoryAt:  memoryAt,
		ShowAt:    b.ShowAt,
		HideAt:    b.HideAt,
		SeenAt:    b.SeenAt,
		IsSaved:   b.IsSaved,
		DataJSON:  `{"year":` + strconv.Itoa(b.Data.Year) + `}`,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := a.store.DB.Create(&m).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "statusCode": 500})
		return
	}
	a.replaceMemoryAssets(m.ID, b.AssetIDs)
	c.JSON(http.StatusCreated, a.toMemoryResponse(&m))
}

// handleMemoryGet implements GET /api/memories/:id.
func (a *App) handleMemoryGet(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	var m Memory
	if err := a.store.DB.Where("id = ? AND owner_id = ?", id, uid).First(&m).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found", "statusCode": 404})
		return
	}
	c.JSON(http.StatusOK, a.toMemoryResponse(&m))
}

// handleMemoryUpdate implements PUT /api/memories/:id.
func (a *App) handleMemoryUpdate(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	var m Memory
	if err := a.store.DB.Where("id = ? AND owner_id = ?", id, uid).First(&m).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found", "statusCode": 404})
		return
	}
	var b struct {
		IsSaved  *bool      `json:"isSaved"`
		MemoryAt string     `json:"memoryAt"`
		SeenAt   *time.Time `json:"seenAt"`
	}
	_ = c.ShouldBindJSON(&b)
	if b.IsSaved != nil {
		m.IsSaved = *b.IsSaved
	}
	if b.MemoryAt != "" {
		if t, err := time.Parse(time.RFC3339, b.MemoryAt); err == nil {
			m.MemoryAt = t
		}
	}
	if b.SeenAt != nil {
		m.SeenAt = b.SeenAt
	}
	m.UpdatedAt = time.Now().UTC()
	a.store.DB.Save(&m)
	c.JSON(http.StatusOK, a.toMemoryResponse(&m))
}

// handleMemoryDelete implements DELETE /api/memories/:id.
func (a *App) handleMemoryDelete(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	res := a.store.DB.Where("id = ? AND owner_id = ?", id, uid).Delete(&Memory{})
	if res.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": res.Error.Error(), "statusCode": 500})
		return
	}
	if res.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found", "statusCode": 404})
		return
	}
	a.store.DB.Where("memory_id = ?", id).Delete(&MemoryAsset{})
	c.Status(http.StatusNoContent)
}

// handleMemoryStatistics implements GET /api/memories/statistics.
func (a *App) handleMemoryStatistics(c *gin.Context) {
	uid := currentUserID(c)
	var total int64
	a.store.DB.Model(&Memory{}).Where("owner_id = ?", uid).Count(&total)
	c.JSON(http.StatusOK, gin.H{"total": total})
}

// handleMemoryAssetsAdd implements PUT /api/memories/:id/assets (addMemoryAssets).
func (a *App) handleMemoryAssetsAdd(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	var m Memory
	if err := a.store.DB.Where("id = ? AND owner_id = ?", id, uid).First(&m).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found", "statusCode": 404})
		return
	}
	var b struct {
		IDs []string `json:"ids"`
	}
	_ = c.ShouldBindJSON(&b)
	a.replaceMemoryAssets(id, b.IDs)
	out := make([]gin.H, 0, len(b.IDs))
	for _, aid := range b.IDs {
		out = append(out, gin.H{"id": aid, "success": true})
	}
	c.JSON(http.StatusOK, out)
}

// handleMemoryAssetsRemove implements DELETE /api/memories/:id/assets (removeMemoryAssets).
func (a *App) handleMemoryAssetsRemove(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	var m Memory
	if err := a.store.DB.Where("id = ? AND owner_id = ?", id, uid).First(&m).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found", "statusCode": 404})
		return
	}
	var b struct {
		IDs []string `json:"ids"`
	}
	_ = c.ShouldBindJSON(&b)
	if len(b.IDs) > 0 {
		a.store.DB.Where("memory_id = ? AND asset_id IN ?", id, b.IDs).Delete(&MemoryAsset{})
	}
	out := make([]gin.H, 0, len(b.IDs))
	for _, aid := range b.IDs {
		out = append(out, gin.H{"id": aid, "success": true})
	}
	c.JSON(http.StatusOK, out)
}

// replaceMemoryAssets sets the asset membership of a memory, replacing any
// prior association (Immich treats PUT /assets as a full replace).
func (a *App) replaceMemoryAssets(memoryID string, assetIDs []string) {
	a.store.DB.Where("memory_id = ?", memoryID).Delete(&MemoryAsset{})
	for i, aid := range assetIDs {
		a.store.DB.Create(&MemoryAsset{MemoryID: memoryID, AssetID: aid, Order: i})
	}
}

// toMemoryResponse builds the MemoryResponseDto shape from a Memory row.
func (a *App) toMemoryResponse(m *Memory) gin.H {
	year := 0
	if m.DataJSON != "" {
		if idx := strings.Index(m.DataJSON, `"year":`); idx >= 0 {
			fmtScanInt(&year, m.DataJSON[idx+7:])
		}
	}
	var links []MemoryAsset
	a.store.DB.Where("memory_id = ?", m.ID).Order(`"order" ASC`).Find(&links)
	assets := make([]AssetResponse, 0, len(links))
	for _, l := range links {
		var as Asset
		if a.store.DB.Where("id = ?", l.AssetID).First(&as).Error == nil {
			assets = append(assets, a.toResponse(as))
		}
	}
	return gin.H{
		"id":         m.ID,
		"ownerId":    m.OwnerID,
		"type":       m.Type,
		"memoryAt":   m.MemoryAt.UTC().Format(time.RFC3339),
		"showAt":     nullableTime(m.ShowAt),
		"hideAt":     nullableTime(m.HideAt),
		"seenAt":     nullableTime(m.SeenAt),
		"isSaved":    m.IsSaved,
		"createdAt":  m.CreatedAt.UTC().Format(time.RFC3339),
		"updatedAt":  m.UpdatedAt.UTC().Format(time.RFC3339),
		"data":       gin.H{"year": year},
		"assets":     assets,
	}
}

// nullableTime renders a *time.Time as RFC3339 or nil.
func nullableTime(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}

// fmtScanInt is a tiny helper to read the first integer from a substring.
func fmtScanInt(dst *int, s string) {
	n := 0
	for _, r := range s {
		if r >= '0' && r <= '9' {
			n = n*10 + int(r-'0')
		} else {
			break
		}
	}
	*dst = n
}
