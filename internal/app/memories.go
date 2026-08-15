package app

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// handleMemories implements Immich's "On This Day" / Memories feature:
// GET /api/memories?day=MM-DD&year=YYYY
//
// It returns, for the requesting user's non-trashed assets whose
// fileCreatedAt falls on the given month/day, a list grouped by year in the
// shape Immich's web/mobile clients expect:
//
//	[ { "years": [<year>], "assets": [<AssetResponseDto>...] }, ... ]
//
// When `year` is supplied only that year's group is returned. Matching is done
// in Go (not via SQLite date functions) so it is independent of how the
// timestamp is stored/serialised.
func (a *App) handleMemories(c *gin.Context) {
	uid := currentUserID(c)

	day := strings.TrimSpace(c.Query("day"))
	m, d := time.Now().Month(), time.Now().Day()
	if day != "" {
		if pm, pd, ok := parseMonthDay(day); ok {
			m, d = pm, pd
		}
	}

	var assets []Asset
	if err := a.store.DB.
		Where("owner_id = ? AND is_trash = ?", uid, false).
		Order("file_created_at DESC").
		Find(&assets).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error(), "statusCode": 500})
		return
	}

	if yearStr := strings.TrimSpace(c.Query("year")); yearStr != "" {
		if y, err := strconv.Atoi(yearStr); err == nil {
			kept := assets[:0]
			for _, as := range assets {
				if as.FileCreatedAt.Year() == y {
					kept = append(kept, as)
				}
			}
			assets = kept
		}
	}

	byYear := map[int][]AssetResponse{}
	var years []int
	for _, as := range assets {
		if int(as.FileCreatedAt.Month()) != int(m) || as.FileCreatedAt.Day() != d {
			continue
		}
		y := as.FileCreatedAt.Year()
		if _, ok := byYear[y]; !ok {
			years = append(years, y)
		}
		byYear[y] = append(byYear[y], a.toResponse(as))
	}
	sort.Sort(sort.Reverse(sort.IntSlice(years)))

	out := make([]gin.H, 0, len(years))
	for _, y := range years {
		out = append(out, gin.H{
			"years":  []int{y},
			"assets": byYear[y],
		})
	}
	c.JSON(http.StatusOK, out)
}

// parseMonthDay parses an "MM-DD" string into month/day. It returns ok=false
// on any malformed input so callers can fall back to today.
func parseMonthDay(s string) (time.Month, int, bool) {
	parts := strings.SplitN(s, "-", 2)
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
