package app

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// MapMarker is a single clustered geo point for the map view. It mirrors
// Immich's MapMarkerResponseDto (id, lat, lon, city, country, state) so the
// official web client parses it without error. Count/AssetID are extra
// (additionalProperties are allowed) and used by our SPA to open the lightbox.
type MapMarker struct {
	ID      string  `json:"id"`
	Lat     float64 `json:"lat"`
	Lon     float64 `json:"lon"`
	City    string  `json:"city"`
	Country string  `json:"country"`
	State   string  `json:"state"`
	Count   int     `json:"count"`
	AssetID string  `json:"assetId"`
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
				State:   "",
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
	// Immich's /api/map/markers returns a bare array of MapMarkerResponseDto.
	c.JSON(http.StatusOK, out)
}

// handleMapReverseGeocode mirrors Immich's POST /api/map/reverse-geocode.
// It takes a single point {lat, lon} and returns the nearest known city
// within ~100km plus its admin1 code (state) and resolved country name,
// using the embedded offline dataset (no external service / no internet).
func (a *App) handleMapReverseGeocode(c *gin.Context) {
	var b struct {
		Lat float64 `json:"lat"`
		Lon float64 `json:"lon"`
	}
	_ = c.ShouldBindJSON(&b)
	// v3.1.0 also calls this as GET with query params.
	if b.Lat == 0 && b.Lon == 0 {
		if q := c.Query("lat"); q != "" {
			fmt.Sscanf(q, "%f", &b.Lat)
		}
		if q := c.Query("lon"); q != "" {
			fmt.Sscanf(q, "%f", &b.Lon)
		}
	}
	if b.Lat == 0 && b.Lon == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "lat/lon required", "statusCode": 400})
		return
	}
	// Official contract (verified live on v3.1.0): a LIST of
	// {city, state, country} — an object broke the web's `.map()` over it.
	if a.geocoder == nil {
		c.JSON(http.StatusOK, []gin.H{{"city": "", "state": "", "country": ""}})
		return
	}
	city, state, country := a.geocoder.Reverse(b.Lat, b.Lon)
	c.JSON(http.StatusOK, []gin.H{{
		"city":    city,
		"state":   state,
		"country": country,
	}})
}
