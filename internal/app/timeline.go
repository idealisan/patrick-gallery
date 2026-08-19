package app

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// timelineOwners returns the owner IDs whose assets should appear in the
// current user's timeline. By default Immich surfaces partner-shared assets in
// the timeline; we honor the `withPartners` query (default true) and fall back
// to the viewer's own assets only when explicitly disabled.
func (a *App) timelineOwners(c *gin.Context, uid string) []string {
	if c.Query("withPartners") == "false" {
		return []string{uid}
	}
	return a.sharedOwnerIDs(uid)
}

func (a *App) handleTimelineBuckets(c *gin.Context) {
	uid := currentUserID(c)
	size := c.Query("size")
	if size == "" {
		size = "MONTH"
	}
	var assets []Asset
	owners := a.timelineOwners(c, uid)
	a.store.DB.Where("owner_id IN ? AND is_trash = ?", owners, false).Find(&assets)

	counts := map[string]int{}
	for _, as := range assets {
		t := as.LocalDateTime
		var key string
		switch size {
		case "YEAR":
			key = t.Format("2006")
		default: // MONTH
			key = t.Format("2006-01")
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
		var bucket time.Time
		if size == "YEAR" {
			bucket, _ = time.Parse("2006", k)
		} else {
			bucket, _ = time.Parse("2006-01", k)
		}
		out = append(out, gin.H{
			"timeBucket": bucket.UTC().Format("2006-01-02T00:00:00.000Z"),
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
	// accept "YYYY-MM" or "YYYY-MM-DD"
	ym := prefix
	if len(prefix) >= 7 {
		ym = prefix[:7]
	}

	var assets []Asset
	owners := a.timelineOwners(c, uid)
	a.store.DB.Where("owner_id IN ? AND is_trash = ?", owners, false).Order("local_date_time DESC").Find(&assets)
	matched := make([]Asset, 0, len(assets))
	for _, as := range assets {
		if as.LocalDateTime.Format("2006-01") == ym {
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
// bucket still satisfies the schema's required-array contract.
type timeBucketAssetsResponse struct {
	City             []string   `json:"city"`
	Country          []string   `json:"country"`
	CreatedAt        []string   `json:"createdAt"`
	Duration         []int      `json:"duration"`
	FileCreatedAt    []string   `json:"fileCreatedAt"`
	ID               []string   `json:"id"`
	IsFavorite       []bool     `json:"isFavorite"`
	IsImage          []bool     `json:"isImage"`
	IsTrashed        []bool     `json:"isTrashed"`
	Latitude         []float64  `json:"latitude"`
	LivePhotoVideoID []string   `json:"livePhotoVideoId"`
	LocalOffsetHours []float64  `json:"localOffsetHours"`
	Longitude        []float64  `json:"longitude"`
	OwnerID          []string   `json:"ownerId"`
	ProjectionType   []string   `json:"projectionType"`
	Ratio            []float64  `json:"ratio"`
	Stack            [][]string `json:"stack"`
	Thumbhash        []string   `json:"thumbhash"`
	Visibility       []string   `json:"visibility"`
}

func (a *App) buildTimeBucketAssets(assets []Asset) timeBucketAssetsResponse {
	n := len(assets)
	r := timeBucketAssetsResponse{
		City:             make([]string, n),
		Country:          make([]string, n),
		CreatedAt:        make([]string, n),
		Duration:         make([]int, n),
		FileCreatedAt:    make([]string, n),
		ID:               make([]string, n),
		IsFavorite:       make([]bool, n),
		IsImage:          make([]bool, n),
		IsTrashed:        make([]bool, n),
		Latitude:         make([]float64, n),
		LivePhotoVideoID: make([]string, n),
		LocalOffsetHours: make([]float64, n),
		Longitude:        make([]float64, n),
		OwnerID:          make([]string, n),
		ProjectionType:   make([]string, n),
		Ratio:            make([]float64, n),
		Stack:            make([][]string, n),
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
		// as.Duration is now stored in milliseconds (see handleAssetUpload /
		// assetDurationResponse), matching the official AssetResponseDto
		// contract, so the bucket mirrors it directly.
		r.Duration[i] = parseDurationInt(as.Duration)
		r.IsFavorite[i] = as.IsFavorite
		r.IsImage[i] = as.Type == "IMAGE"
		r.IsTrashed[i] = as.IsTrash
		r.LivePhotoVideoID[i] = as.LivePhotoVideoID
		r.ProjectionType[i] = "" // no 360°/equirectangular support yet
		r.Thumbhash[i] = as.Thumbhash
		r.Visibility[i] = visibilityOf(as.IsArchived)
		if as.Width > 0 && as.Height > 0 {
			r.Ratio[i] = float64(as.Width) / float64(as.Height)
		}
		// local offset (hours) between the photo's local time and its UTC stamp
		r.LocalOffsetHours[i] = as.LocalDateTime.Sub(as.FileCreatedAt).Hours()
		// Stacking is unsupported. The official server emits `null` (not an
		// empty array) for a non-stacked asset's slot in the parallel `stack`
		// array. The web reconstructs `asset.stack` from this slot and, for a
		// truthy (non-null) value, does `Number.parseInt(slot[1])`. An empty
		// array `[]` is truthy in JS, slot[1] is undefined, and the parse yields
		// NaN, which the thumbnail renders as the literal text "NaN". Leaving the
		// slice element nil makes it serialize to `null`, matching the contract.
		r.Stack[i] = nil
		if e, ok := exifByID[as.ExifID]; ok {
			r.City[i] = e.City
			r.Country[i] = e.Country
			r.Latitude[i] = e.Latitude
			r.Longitude[i] = e.Longitude
		}
	}
	return r
}
