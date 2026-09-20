package app

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// timelineFilter carries the /timeline/bucket(s) query parameters, mirroring
// the official TimeBucketDto (server/src/dtos/time-bucket.dto.ts) and the
// query semantics of AssetRepository.getTimeBuckets / getTimeBucket.
type timelineFilter struct {
	userID       string
	albumID      string
	personID     string
	tagID        string
	isFavorite   *bool
	isTrashed    bool
	withStacked  bool
	withPartners bool
	order        string // asc | desc
	orderBy      string // takenAt | createdAt
	visibility   string // "" = official default: IN ('archive','timeline')
	bbox         *[4]float64
}

func parseTimelineFilter(c *gin.Context) timelineFilter {
	f := timelineFilter{
		userID:       c.Query("userId"),
		albumID:      c.Query("albumId"),
		personID:     c.Query("personId"),
		tagID:        c.Query("tagId"),
		withStacked:  c.Query("withStacked") == "true",
		withPartners: c.Query("withPartners") == "true",
		order:        strings.ToLower(c.Query("order")),
		orderBy:      c.Query("orderBy"),
		visibility:   c.Query("visibility"),
	}
	if v := c.Query("isFavorite"); v != "" {
		b := v == "true"
		f.isFavorite = &b
	}
	f.isTrashed = c.Query("isTrashed") == "true" || c.Query("isTrash") == "true"
	if b := c.Query("bbox"); b != "" {
		parts := strings.Split(b, ",")
		if len(parts) == 4 {
			vals := make([]float64, 4)
			okAll := true
			for i, p := range parts {
				v, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
				if err != nil {
					okAll = false
					break
				}
				vals[i] = v
			}
			if okAll {
				f.bbox = &[4]float64{vals[0], vals[1], vals[2], vals[3]}
			}
		}
	}
	if f.order != "asc" {
		f.order = "desc"
	}
	switch f.orderBy {
	case "createdAt":
	default:
		f.orderBy = "takenAt"
	}
	return f
}

// orderColumn maps orderBy to the SQL column used for bucketing/sorting,
// matching truncatedDate(orderBy): takenAt → localDateTime,
// createdAt → createdAt (date added).
func (f timelineFilter) orderColumn() string {
	if f.orderBy == "createdAt" {
		return "created_at"
	}
	return "local_date_time"
}

// authorizeTimeline mirrors TimelineService.timeBucketChecks: album access via
// AlbumRead when albumId is present; otherwise the viewer's own (or the named
// user's) timeline; withPartners only for non-archived/non-trashed/
// non-favorited/non-locked requests.
func (a *App) authorizeTimeline(f timelineFilter, uid string) (int, string) {
	if f.albumID != "" {
		if a.albumRole(uid, f.albumID) == "" {
			return http.StatusForbidden, "no access to this album"
		}
		return 0, ""
	}
	if f.userID != "" && f.userID != uid && !a.isAdmin(uid) {
		return http.StatusForbidden, "no access to this user's timeline"
	}
	if f.withPartners {
		requestedArchived := f.visibility == "" || f.visibility == "archive"
		requestedLocked := f.visibility == "locked"
		if requestedLocked || requestedArchived || f.isFavorite != nil || f.isTrashed {
			return http.StatusBadRequest,
				"withPartners is only supported for non-archived, non-trashed, non-favorited, non-locked assets"
		}
	}
	return 0, ""
}

// buildTimelineQuery assembles the asset query with official semantics:
// albumId path joins membership WITHOUT any owner restriction (official
// leaves userIds unset there); visibility unset → IN ('archive','timeline')
// (withDefaultVisibility); trash flips deleted-row semantics.
func (a *App) buildTimelineQuery(f timelineFilter, uid string) *gorm.DB {
	q := a.store.DB.Model(&Asset{})
	if f.isTrashed {
		q = q.Where("is_trash = ?", true)
	} else {
		q = q.Where("is_trash = ?", false)
	}
	if f.visibility == "" {
		q = q.Where("visibility IN (?, ?)", "archive", "timeline")
	} else {
		q = q.Where("visibility = ?", strings.ToLower(f.visibility))
	}
	if f.albumID != "" {
		q = q.Joins("JOIN album_assets aa ON aa.asset_id = assets.id AND aa.album_id = ?", f.albumID)
	} else {
		ids := []string{uid}
		if f.userID != "" {
			ids = []string{f.userID}
		}
		if f.withPartners {
			for _, pid := range a.sharedOwnerIDs(uid) {
				found := false
				for _, x := range ids {
					if x == pid {
						found = true
						break
					}
				}
				if !found {
					ids = append(ids, pid)
				}
			}
		}
		q = q.Where("owner_id IN ?", ids)
	}
	if f.personID != "" {
		q = q.Where("person_id = ?", f.personID)
	}
	if f.tagID != "" {
		q = q.Where("EXISTS (SELECT 1 FROM tags_assets ta WHERE ta.asset_id = assets.id AND ta.tag_id = ?)", f.tagID)
	}
	if f.isFavorite != nil {
		q = q.Where("is_favorite = ?", *f.isFavorite)
	}
	if f.bbox != nil {
		w, s2, e, n := f.bbox[0], f.bbox[1], f.bbox[2], f.bbox[3]
		q = q.Joins("JOIN exifs ex ON ex.asset_id = assets.id").
			Where("ex.latitude BETWEEN ? AND ? AND ex.longitude BETWEEN ? AND ?", s2, n, w, e)
	}
	q = q.Order(f.orderColumn() + " " + strings.ToUpper(f.order))
	return q
}

func (a *App) handleTimelineBuckets(c *gin.Context) {
	uid := currentUserID(c)
	size := c.Query("size")
	if size == "" {
		size = "MONTH"
	}
	f := parseTimelineFilter(c)
	if status, msg := a.authorizeTimeline(f, uid); status != 0 {
		c.JSON(status, gin.H{"message": msg, "statusCode": status})
		return
	}
	var assets []Asset
	a.buildTimelineQuery(f, uid).Find(&assets)

	col := f.orderColumn()
	counts := map[string]int{}
	for _, as := range assets {
		t := as.LocalDateTime
		if col == "created_at" {
			t = as.CreatedAt
		}
		key := t.Format("2006-01")
		if size == "YEAR" {
			key = t.Format("2006")
		}
		counts[key]++
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]gin.H, 0, len(keys))
	for _, k := range keys {
		layout := "2006-01"
		if size == "YEAR" {
			layout = "2006"
		}
		bucket, _ := time.Parse(layout, k)
		out = append(out, gin.H{
			"timeBucket": bucket.UTC().Format("2006-01-02"),
			"count":      counts[k],
		})
	}
	c.JSON(http.StatusOK, out)
}

func (a *App) handleTimelineBucketAssets(c *gin.Context) {
	uid := currentUserID(c)
	tb := c.Query("timeBucket")
	if tb == "" {
		if q := c.Query("bucket"); q != "" {
			tb = q
		}
	}
	prefix := strings.TrimSuffix(tb, "T00:00:00.000Z")
	prefix = strings.TrimSuffix(prefix, "T00:00:00Z")
	prefix = strings.TrimSuffix(prefix, "Z")
	ym := prefix
	if len(prefix) >= 7 {
		ym = prefix[:7]
	}

	f := parseTimelineFilter(c)
	if status, msg := a.authorizeTimeline(f, uid); status != 0 {
		c.JSON(status, gin.H{"message": msg, "statusCode": status})
		return
	}
	var assets []Asset
	a.buildTimelineQuery(f, uid).Find(&assets)

	col := f.orderColumn()
	matched := make([]Asset, 0, len(assets))
	for _, as := range assets {
		t := as.LocalDateTime
		if col == "created_at" {
			t = as.CreatedAt
		}
		if t.Format("2006-01") == ym {
			matched = append(matched, as)
		}
	}

	// Immich's /timeline/bucket returns a single TimeBucketAssetResponseDto:
	// parallel arrays (one slot per asset, index-aligned). /timeline/assets
	// returns a bare array of assets, so branch on the requested path.
	// (Match on the actual request path; gin's c.FullPath() includes the
	// "/api" group prefix, so a literal compare would miss it.)
	if strings.HasSuffix(c.Request.URL.Path, "/timeline/bucket") {
		c.JSON(http.StatusOK, a.buildTimeBucketAssets(matched))
		return
	}
	out := make([]AssetResponse, 0, len(matched))
	for _, as := range matched {
		out = append(out, a.toResponse(as))
	}
	c.JSON(http.StatusOK, out)
}

// timeBucketAssetsResponse mirrors Immich's TimeBucketAssetResponseDto: every
// field is a parallel array holding that property for each asset in the bucket
// (all index-aligned). Fields are always emitted (no omitempty) so an empty
// bucket still satisfies the schema's required-array contract. Verified live
// against the official v3.1.0 server: the response carries exactly city,
// country, createdAt, duration, fileCreatedAt, id, isFavorite, isImage,
// isTrashed, livePhotoVideoId, localOffsetHours, createdAt, ownerId,
// projectionType, ratio, status, thumbhash, visibility — no latitude,
// longitude or stack (those live in exifInfo / the single-asset DTO).
type timeBucketAssetsResponse struct {
	City             []string  `json:"city"`
	Country          []string  `json:"country"`
	CreatedAt        []string  `json:"createdAt"`
	Duration         []any     `json:"duration"`
	FileCreatedAt    []string  `json:"fileCreatedAt"`
	ID               []string  `json:"id"`
	IsFavorite       []bool    `json:"isFavorite"`
	IsImage          []bool    `json:"isImage"`
	IsTrashed        []bool    `json:"isTrashed"`
	LivePhotoVideoID []string  `json:"livePhotoVideoId"`
	LocalOffsetHours []float64 `json:"localOffsetHours"`
	OwnerID          []string  `json:"ownerId"`
	ProjectionType   []string  `json:"projectionType"`
	Ratio            []float64 `json:"ratio"`
	Status           []string  `json:"status"`
	Thumbhash        []string  `json:"thumbhash"`
	Visibility       []string  `json:"visibility"`
}

func (a *App) buildTimeBucketAssets(assets []Asset) timeBucketAssetsResponse {
	n := len(assets)
	r := timeBucketAssetsResponse{
		City:             make([]string, n),
		Country:          make([]string, n),
		CreatedAt:        make([]string, n),
		Duration:         make([]any, n),
		FileCreatedAt:    make([]string, n),
		ID:               make([]string, n),
		IsFavorite:       make([]bool, n),
		IsImage:          make([]bool, n),
		IsTrashed:        make([]bool, n),
		LivePhotoVideoID: make([]string, n),
		LocalOffsetHours: make([]float64, n),
		OwnerID:          make([]string, n),
		ProjectionType:   make([]string, n),
		Ratio:            make([]float64, n),
		Status:           make([]string, n),
		Thumbhash:        make([]string, n),
		Visibility:       make([]string, n),
	}
	if n == 0 {
		return r
	}
	exifIDs := make([]string, 0, n)
	for _, as := range assets {
		if as.ExifID != "" {
			exifIDs = append(exifIDs, as.ExifID)
		}
	}
	exifByID := make(map[string]Exif, len(exifIDs))
	if len(exifIDs) > 0 {
		var exifs []Exif
		a.store.DB.Where("id IN ?", exifIDs).Find(&exifs)
		for _, e := range exifs {
			exifByID[e.ID] = e
		}
	}
	for i, as := range assets {
		r.ID[i] = as.ID
		r.OwnerID[i] = as.OwnerID
		r.CreatedAt[i] = as.CreatedAt.UTC().Format(time.RFC3339Nano)
		r.FileCreatedAt[i] = as.FileCreatedAt.UTC().Format(time.RFC3339Nano)
		// Official contract (verified live on v3.1.0): the parallel
		// `duration` slot is `null` for assets without a duration (still
		// photos); only videos carry a value.
		if as.Duration == "" {
			r.Duration[i] = nil
		} else {
			r.Duration[i] = parseDurationInt(as.Duration)
		}
		r.IsFavorite[i] = as.IsFavorite
		r.IsImage[i] = as.Type == "IMAGE"
		r.IsTrashed[i] = as.IsTrash
		r.LivePhotoVideoID[i] = as.LivePhotoVideoID
		r.ProjectionType[i] = "" // no 360°/equirectangular support yet
		r.Thumbhash[i] = as.Thumbhash
		r.Visibility[i] = visibilityString(as.Visibility, as.IsArchived)
		if as.Width > 0 && as.Height > 0 {
			r.Ratio[i] = float64(as.Width) / float64(as.Height)
		} else {
			// Missing dimensions must never yield ratio 0/NaN: the web grid
			// divides by it and collapses the tile into a sliver. Default to
			// 4:3 landscape (the dominant still-photo shape).
			r.Ratio[i] = 4.0 / 3.0
		}
		// local offset (hours) between the photo's local time and its UTC stamp
		r.LocalOffsetHours[i] = as.LocalDateTime.Sub(as.FileCreatedAt).Hours()
		// Official contract (verified live on v3.1.0): the bucket response
		// carries a parallel `status` array ("active" | "trashed") and has
		// no latitude/longitude/stack slots at all.
		if as.IsTrash {
			r.Status[i] = "trashed"
		} else {
			r.Status[i] = "active"
		}
		if e, ok := exifByID[as.ExifID]; ok {
			r.City[i] = e.City
			r.Country[i] = e.Country
		}
	}
	return r
}
