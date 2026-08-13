package app

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// MapMarker is a single clustered geo point for the map view. It mirrors
// Immich's /api/map/markers shape closely enough for our SPA: an id, lat/lon,
// optional reverse-geocoded city/country, a count of assets at that location,
// and a representative assetId used to open the lightbox.
type MapMarker struct {
	ID       string  `json:"id"`
	Lat      float64 `json:"lat"`
	Lon      float64 `json:"lon"`
	City     string  `json:"city"`
	Country  string  `json:"country"`
	Count    int     `json:"count"`
	AssetID  string  `json:"assetId"`
}

// handleMapMarkers returns aggregated GPS markers for the requesting user's
// non-trashed assets that carry EXIF coordinates. Photos are clustered by
// rounding their coordinates to ~1.1km grid cells; if reverse-geocoded
// city/country is present it is surfaced on the marker.
func (a *App) handleMapMarkers(c *gin.Context) {
	uid := currentUserID(c)

	type row struct {
		AssetID string
		Lat     float64
		Lon     float64
		City    string
		Country string
	}
	var rows []row
	a.store.DB.
		Model(&Asset{}).
		Select("assets.id as asset_id, exif.latitude as lat, exif.longitude as lon, exif.city as city, exif.country as country").
		Joins("JOIN exif ON exif.asset_id = assets.id").
		Where("assets.owner_id = ? AND assets.is_trash = ? AND exif.latitude <> 0 AND exif.longitude <> 0", uid, false).
		Scan(&rows)

	groups := map[string]*MapMarker{}
	for _, r := range rows {
		// ~1.1km clustering (2 decimal places of degrees)
		key := fmt.Sprintf("%.2f,%.2f", r.Lat, r.Lon)
		g, ok := groups[key]
		if !ok {
			g = &MapMarker{
				ID:      newUUID(),
				Lat:     r.Lat,
				Lon:     r.Lon,
				City:    r.City,
				Country: r.Country,
				AssetID: r.AssetID,
			}
			groups[key] = g
		}
		g.Count++
		// prefer a marker that has a place name over a bare coordinate
		if g.City == "" && g.Country == "" && (r.City != "" || r.Country != "") {
			g.City, g.Country = r.City, r.Country
		}
	}

	out := make([]MapMarker, 0, len(groups))
	for _, g := range groups {
		out = append(out, *g)
	}
	c.JSON(http.StatusOK, gin.H{"markers": out})
}
