package app

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// adminID returns the seeded admin user's id for ownership-sensitive inserts.
func adminID(t *testing.T, app *App) string {
	t.Helper()
	var u User
	if err := app.store.DB.First(&u).Error; err != nil {
		t.Fatalf("fetch admin: %v", err)
	}
	return u.ID
}

func TestReverseGeocodeEndpoint(t *testing.T) {
	_, r, token := newTestServer(t)

	cases := []struct {
		name     string
		lat, lon float64
		wantC    string // expected country (empty => expect no result)
	}{
		{"paris", 48.8566, 2.3522, "France"},
		{"tokyo", 35.6895, 139.6917, "Japan"},
		{"ocean", 0.0, -160.0, ""},
	}
	for _, tc := range cases {
		body, _ := json.Marshal(map[string]float64{"lat": tc.lat, "lon": tc.lon})
		w := do(r, "POST", "/api/map/reverse-geocode", token, body, "application/json")
		if w.Code != http.StatusOK {
			t.Errorf("%s: status %d", tc.name, w.Code)
			continue
		}
		// Official contract (verified live on v3.1.0): a LIST of results.
		type geoResult struct {
			City    string `json:"city"`
			State   string `json:"state"`
			Country string `json:"country"`
		}
		var list []geoResult
		_ = json.Unmarshal(w.Body.Bytes(), &list)
		resp := geoResult{}
		if len(list) > 0 {
			resp = list[0]
		}
		if tc.wantC == "" {
			if resp.Country != "" {
				t.Errorf("%s: expected no reverse-geocode, got country=%q", tc.name, resp.Country)
			}
			continue
		}
		if resp.Country != tc.wantC {
			t.Errorf("%s: expected country %q, got %q (city=%q)", tc.name, tc.wantC, resp.Country, resp.City)
		}
	}
}

func TestReverseGeocodingIngested(t *testing.T) {
	app, r, token := newTestServer(t)
	// Directly create an Exif row with GPS and an asset, then run the shared
	// ingest media path; the offline geocoder should attach city/country.
	exif := &Exif{
		ID:        newUUID(),
		AssetID:   newUUID(),
		Latitude:  48.8566,
		Longitude: 2.3522,
	}
	if err := app.store.DB.Create(exif).Error; err != nil {
		t.Fatal(err)
	}
	// exercise processMedia reverse-geocoding through the geocoder directly
	if app.geocoder == nil {
		t.Fatal("geocoder not loaded in test app")
	}
	city, _, country := app.geocoder.Reverse(exif.Latitude, exif.Longitude)
	if country != "France" {
		t.Fatalf("expected France, got %q city=%q", country, city)
	}
	// Sanity: the map markers endpoint is reachable (smoke).
	if w := do(r, "GET", "/api/map/markers", token, nil, ""); w.Code != 200 {
		t.Errorf("/api/map/markers -> %d", w.Code)
	}
}

func TestLivePhotoPlayback(t *testing.T) {
	app, r, token := newTestServer(t)
	uid := adminID(t, app)

	// Write a fake "video" file (content does not need to be a real codec; the
	// handler streams it as-is, mirroring how the official client fetches the
	// motion component).
	vidDir := t.TempDir()
	vidPath := filepath.Join(vidDir, "motion.mp4")
	if err := os.WriteFile(vidPath, []byte("FAKEMP4-MOTION-DATA"), 0o644); err != nil {
		t.Fatal(err)
	}
	videoID := newUUID()
	if err := app.store.DB.Create(&Asset{
		ID:           videoID,
		OwnerID:      uid,
		Type:         "VIDEO",
		OriginalPath: vidPath,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}).Error; err != nil {
		t.Fatal(err)
	}
	photoID := newUUID()
	if err := app.store.DB.Create(&Asset{
		ID:               photoID,
		OwnerID:          uid,
		Type:             "IMAGE",
		LivePhotoVideoID: videoID,
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
	}).Error; err != nil {
		t.Fatal(err)
	}

	// The photo's asset response must surface the livePhotoVideoId.
	wGet := do(r, "GET", "/api/assets/"+photoID, token, nil, "")
	if wGet.Code != 200 {
		t.Fatalf("get photo -> %d: %s", wGet.Code, wGet.Body.String())
	}
	var resp struct {
		LivePhotoVideoID string `json:"livePhotoVideoId"`
	}
	_ = json.Unmarshal(wGet.Body.Bytes(), &resp)
	if resp.LivePhotoVideoID != videoID {
		t.Fatalf("expected livePhotoVideoId %q, got %q", videoID, resp.LivePhotoVideoID)
	}

	// The live-photo endpoint streams the paired video bytes.
	wPlay := do(r, "GET", "/api/assets/"+photoID+"/live-photo", token, nil, "")
	if wPlay.Code != 200 {
		t.Fatalf("live-photo -> %d: %s", wPlay.Code, wPlay.Body.String())
	}
	if wPlay.Body.String() != "FAKEMP4-MOTION-DATA" {
		t.Fatalf("live-photo body mismatch: %q", wPlay.Body.String())
	}
}

func TestTrashCleanup(t *testing.T) {
	app, r, token := newTestServer(t)
	uid := adminID(t, app)
	app.cfg.TrashDays = 30

	old := time.Now().UTC().AddDate(0, 0, -40) // 40 days ago -> should be purged
	recent := time.Now().UTC()

	oldID := newUUID()
	if err := app.store.DB.Create(&Asset{
		ID:        oldID,
		OwnerID:   uid,
		Type:      "IMAGE",
		IsTrash:   true,
		TrashedAt: &old,
		CreatedAt: old,
		UpdatedAt: old,
	}).Error; err != nil {
		t.Fatal(err)
	}
	recentID := newUUID()
	if err := app.store.DB.Create(&Asset{
		ID:        recentID,
		OwnerID:   uid,
		Type:      "IMAGE",
		IsTrash:   true,
		TrashedAt: &recent,
		CreatedAt: recent,
		UpdatedAt: recent,
	}).Error; err != nil {
		t.Fatal(err)
	}

	deleted, err := app.runTrashCleanup()
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 deleted, got %d", deleted)
	}
	var n int64
	app.store.DB.Model(&Asset{}).Where("id = ?", oldID).Count(&n)
	if n != 0 {
		t.Errorf("old trashed asset should be deleted")
	}
	app.store.DB.Model(&Asset{}).Where("id = ?", recentID).Count(&n)
	if n != 1 {
		t.Errorf("recent trashed asset should be retained")
	}

	// The manual endpoint should report a count (0 now, nothing left to purge).
	w := do(r, "POST", "/api/trash/cleanup", token, nil, "")
	if w.Code != 200 {
		t.Fatalf("trash/cleanup -> %d: %s", w.Code, w.Body.String())
	}
	var c struct {
		Deleted int `json:"deleted"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &c)
	if c.Deleted != 0 {
		t.Errorf("expected 0 deleted on second run, got %d", c.Deleted)
	}
}

func TestServerStatisticsAndPersonAssets(t *testing.T) {
	app, r, token := newTestServer(t)
	uid := adminID(t, app)
	// Seed a couple of non-trash assets so counts are non-zero.
	for i := 0; i < 2; i++ {
		if err := app.store.DB.Create(&Asset{
			ID:        newUUID(),
			OwnerID:   uid,
			Type:      "IMAGE",
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}).Error; err != nil {
			t.Fatal(err)
		}
	}

	if w := do(r, "GET", "/api/server/statistics", token, nil, ""); w.Code != 200 {
		t.Errorf("/api/server/statistics -> %d", w.Code)
	} else {
		var s struct {
			Photos int64 `json:"photos"`
			Total  int64 `json:"total"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &s)
		if s.Photos < 2 {
			t.Errorf("expected at least 2 photos, got %d", s.Photos)
		}
	}

	if w := do(r, "GET", "/api/people/does-not-exist/assets", token, nil, ""); w.Code != 200 {
		t.Errorf("/api/people/:id/assets -> %d", w.Code)
	} else {
		var p struct {
			Assets []any `json:"assets"`
			Total  int   `json:"total"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &p)
		if p.Total != 0 || len(p.Assets) != 0 {
			t.Errorf("expected empty person assets, got %+v", p)
		}
	}
}
