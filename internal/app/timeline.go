package app

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func (a *App) handleTimelineBuckets(c *gin.Context) {
	uid := currentUserID(c)
	size := c.Query("size")
	if size == "" {
		size = "MONTH"
	}
	var assets []Asset
	a.store.DB.Where("owner_id = ? AND is_trash = ?", uid, false).Find(&assets)

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
	a.store.DB.Where("owner_id = ? AND is_trash = ?", uid, false).Order("local_date_time DESC").Find(&assets)
	out := make([]AssetResponse, 0)
	for _, as := range assets {
		if as.LocalDateTime.Format("2006-01") == ym {
			out = append(out, a.toResponse(as))
		}
	}
	c.JSON(http.StatusOK, gin.H{"assets": out, "count": len(out)})
}
