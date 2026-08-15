// Package geo provides a small, dependency-free offline reverse geocoder
// used to attach city / state / country names to GPS-tagged assets without
// calling any external service (keeping immich-go CGO-free and usable on a
// private LAN with no internet egress).
//
// The reference dataset is a trimmed extract of GeoNames (cities15000) plus
// the ISO country list, both embedded at build time. A 1°×1° spatial grid
// makes nearest-city lookup O(neighbours) instead of O(all cities).
package geo

import (
	"embed"
	"math"
	"strconv"
	"strings"
	"sync"
)

//go:embed cities.tsv countryInfo.txt
var geoFS embed.FS

// city is one reference point from the embedded GeoNames dump.
type city struct {
	name    string
	lat     float64
	lon     float64
	country string // ISO 3166-1 alpha-2
	admin1  string // admin1 code (state/province), GeoNames style
}

// Geocoder answers reverse-geocoding queries against the embedded dataset.
type Geocoder struct {
	mu        sync.RWMutex
	countries map[string]string // ISO2 -> country name
	grid      map[string][]int  // bucket key ("floorLat,floorLon") -> city indices
	cities    []city
}

// shared is the process-wide singleton, loaded once via Load.
var (
	shared     *Geocoder
	sharedErr  error
	sharedOnce sync.Once
)

// Load returns the process-wide Geocoder, loading the embedded reference data
// exactly once. On failure it returns (nil, err) (callers must degrade
// gracefully).
func Load() (*Geocoder, error) {
	sharedOnce.Do(func() {
		g, err := NewGeocoder()
		if err != nil {
			sharedErr = err
			shared = nil
			return
		}
		shared = g
	})
	return shared, sharedErr
}

// NewGeocoder loads the embedded reference data. It never fails fatally: if a
// file is somehow missing the returned Geocoder simply has no data and
// Reverse returns empty results (callers degrade gracefully).
func NewGeocoder() (*Geocoder, error) {
	g := &Geocoder{
		countries: map[string]string{},
		grid:      map[string][]int{},
	}
	if err := g.loadCountries(); err != nil {
		return nil, err
	}
	if err := g.loadCities(); err != nil {
		return nil, err
	}
	return g, nil
}

func (g *Geocoder) loadCountries() error {
	data, err := geoFS.ReadFile("countryInfo.txt")
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) < 5 {
			continue
		}
		code, name := f[0], f[4]
		if code != "" && name != "" {
			g.countries[code] = name
		}
	}
	return nil
}

func (g *Geocoder) loadCities() error {
	data, err := geoFS.ReadFile("cities.tsv")
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) < 5 {
			continue
		}
		lat, err1 := strconv.ParseFloat(f[1], 64)
		lon, err2 := strconv.ParseFloat(f[2], 64)
		if err1 != nil || err2 != nil {
			continue
		}
		idx := len(g.cities)
		g.cities = append(g.cities, city{
			name:    f[0],
			lat:     lat,
			lon:     lon,
			country: f[3],
			admin1:  f[4],
		})
		key := bucketKey(lat, lon)
		g.grid[key] = append(g.grid[key], idx)
	}
	return nil
}

func bucketKey(lat, lon float64) string {
	return strconv.Itoa(int(math.Floor(lat))) + "," + strconv.Itoa(int(math.Floor(lon)))
}

// Cities returns the number of reference cities loaded (for diagnostics/log).
func (g *Geocoder) Cities() int { return len(g.cities) }

// CityResult is a city match returned by SearchCities for the /search/cities
// and /search/places endpoints.
type CityResult struct {
	Name      string  `json:"name"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Country   string  `json:"country"` // resolved country name
	State     string  `json:"state"`   // admin1 code (state/province)
}

// SearchCities returns up to limit cities whose name contains q
// (case-insensitive substring). An empty q returns the first `limit` cities in
// load order. This powers real city/place search instead of an empty stub.
func (g *Geocoder) SearchCities(q string, limit int) []CityResult {
	if limit <= 0 {
		limit = 100
	}
	needle := strings.ToLower(strings.TrimSpace(q))
	out := make([]CityResult, 0, limit)
	for _, c := range g.cities {
		if needle == "" || strings.Contains(strings.ToLower(c.name), needle) {
			out = append(out, CityResult{
				Name:      c.name,
				Latitude:  c.lat,
				Longitude: c.lon,
				Country:   g.countries[c.country],
				State:     c.admin1,
			})
			if len(out) >= limit {
				break
			}
		}
	}
	return out
}

// Reverse returns the nearest known city (within maxRadiusKm) of the supplied
// coordinate along with its admin1 code (state) and resolved country name.
// When nothing is within range it returns empty strings, which callers treat
// as "no reverse-geocode result".
func (g *Geocoder) Reverse(lat, lon float64) (cityName, state, country string) {
	const maxRadiusKm = 100.0
	const span = 2 // search ±2° grid cells in each axis

	best := -1
	bestDist := maxRadiusKm + 1
	for dlat := -span; dlat <= span; dlat++ {
		for dlon := -span; dlon <= span; dlon++ {
			key := strconv.Itoa(int(math.Floor(lat))+dlat) + "," + strconv.Itoa(int(math.Floor(lon))+dlon)
			for _, idx := range g.grid[key] {
				c := g.cities[idx]
				if d := haversine(lat, lon, c.lat, c.lon); d < bestDist {
					bestDist = d
					best = idx
				}
			}
		}
	}
	if best < 0 {
		return "", "", ""
	}
	c := g.cities[best]
	return c.name, c.admin1, g.countries[c.country]
}

// haversine returns the great-circle distance in kilometres between two
// WGS84 coordinates.
func haversine(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371.0
	rad := func(d float64) float64 { return d * math.Pi / 180.0 }
	dlat := rad(lat2 - lat1)
	dlon := rad(lon2 - lon1)
	a := math.Sin(dlat/2)*math.Sin(dlat/2) +
		math.Cos(rad(lat1))*math.Cos(rad(lat2))*math.Sin(dlon/2)*math.Sin(dlon/2)
	return R * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
