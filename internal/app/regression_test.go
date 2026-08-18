package app

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// newTestServer wires the full HTTP surface (every Immich-compatible route,
// including the auth guard and public share endpoints) against a real temp
// SQLite store + resource dir. It is the backbone of the regression suite:
// each feature is exercised through the real handlers, not mocked.
func newTestServer(t *testing.T) (*App, *gin.Engine, string) {
	t.Helper()
	app := newTestApp(t)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	app.RegisterRoutes(r)

	var admin User
	if err := app.store.DB.First(&admin).Error; err != nil {
		t.Fatalf("no admin seeded: %v", err)
	}
	token, err := app.issueToken(admin.ID)
	if err != nil {
		t.Fatalf("issueToken: %v", err)
	}
	return app, r, token
}

// do performs an HTTP request against the test engine.
func do(r *gin.Engine, method, path, token string, body []byte, contentType string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// uploadAsset posts a generated JPEG as a multipart asset upload and returns
// the created asset id (or "" on failure).
func uploadAsset(t *testing.T, r *gin.Engine, token, fileName string) string {
	t.Helper()
	dir := t.TempDir()
	imgPath := filepath.Join(dir, fileName)
	writeRegressionJPEG(t, imgPath, 64, 48, 200, 100, 50)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	meta := `{"deviceAssetId":"reg-` + fileName + `","deviceId":"reg","fileCreatedAt":"2024-01-01T00:00:00.000Z","localDateTime":"2024-01-01T00:00:00.000Z","type":"IMAGE","fileExtension":"jpg"}`
	if err := mw.WriteField("asset", meta); err != nil {
		t.Fatal(err)
	}
	fw, err := mw.CreateFormFile("assetData", fileName)
	if err != nil {
		t.Fatal(err)
	}
	src, _ := os.Open(imgPath)
	_, _ = io.Copy(fw, src)
	src.Close()
	mw.Close()

	w := do(r, "POST", "/api/assets", token, buf.Bytes(), mw.FormDataContentType())
	if w.Code != http.StatusCreated {
		t.Fatalf("upload %s -> %d: %s", fileName, w.Code, w.Body.String())
	}
	var resp struct {
		ID      string `json:"id"`
		AssetID string `json:"assetId"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.ID != "" {
		return resp.ID
	}
	return resp.AssetID
}

func writeRegressionJPEG(t *testing.T, path string, w, h, r, g, b int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8((x * 7) % 256), uint8((y * 11) % 256), uint8(b), 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatal(err)
	}
}

// TestRegression exercises every major feature area through the real HTTP
// handlers. A 2xx with a meaningful payload is "fully working"; a 2xx with an
// empty/placeholder body marks a stub (recorded in REGRESSION_REPORT.md); a
// 4xx/5xx is "not working".
func TestRegression(t *testing.T) {
	app, r, token := newTestServer(t)

	t.Run("server-meta", func(t *testing.T) {
		for _, p := range []string{"/api/server/ping", "/api/server/health", "/api/server/about", "/api/server/version", "/api/server/config", "/api/server/features"} {
			if w := do(r, "GET", p, "", nil, ""); w.Code != 200 {
				t.Errorf("%s -> %d", p, w.Code)
			}
		}
	})

	t.Run("auth", func(t *testing.T) {
		// wrong creds
		w := do(r, "POST", "/api/auth/login", "", mustJSON(t, map[string]string{"email": "admin@immich.app", "password": "wrong"}), "application/json")
		if w.Code != 401 {
			t.Errorf("bad login expected 401, got %d", w.Code)
		}
		// correct creds
		w = do(r, "POST", "/api/auth/login", "", mustJSON(t, map[string]string{"email": "admin@immich.app", "password": "password"}), "application/json")
		if w.Code != 201 {
			t.Errorf("login expected 201, got %d", w.Code)
		}
		if w := do(r, "GET", "/api/users/me", token, nil, ""); w.Code != 200 {
			t.Errorf("users/me -> %d", w.Code)
		}
		if w := do(r, "POST", "/api/auth/validateToken", token, mustJSON(t, gin.H{}), "application/json"); w.Code != 200 && w.Code != 204 {
			t.Errorf("validateToken -> %d", w.Code)
		}
	})

	var assetID string
	t.Run("assets-upload-and-serve", func(t *testing.T) {
		assetID = uploadAsset(t, r, token, "reg1.jpg")
		if assetID == "" {
			t.Fatal("upload returned no id")
		}
		if w := do(r, "GET", "/api/assets/"+assetID, token, nil, ""); w.Code != 200 {
			t.Errorf("asset get -> %d", w.Code)
		}
		if w := do(r, "GET", "/api/assets/"+assetID+"/thumbnail", token, nil, ""); w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "image") {
			t.Errorf("thumbnail -> %d ct=%s", w.Code, w.Header().Get("Content-Type"))
		}
		if w := do(r, "GET", "/api/assets/"+assetID+"/original", token, nil, ""); w.Code != 200 {
			t.Errorf("original -> %d", w.Code)
		}
		if w := do(r, "GET", "/api/assets/"+assetID+"/preview", token, nil, ""); w.Code != 200 {
			t.Errorf("preview -> %d", w.Code)
		}
	})

	t.Run("assets-queries", func(t *testing.T) {
		for _, p := range []string{"/api/assets?take=50", "/api/assets/count", "/api/assets/random", "/api/assets/statistics", "/api/assets/duplicates"} {
			if w := do(r, "GET", p, token, nil, ""); w.Code != 200 {
				t.Errorf("%s -> %d", p, w.Code)
			}
		}
		// search should return the uploaded asset
		w := do(r, "GET", "/api/assets?take=50", token, nil, "")
		var sr struct {
			Assets []json.RawMessage `json:"assets"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &sr)
		if len(sr.Assets) == 0 {
			t.Errorf("asset search returned no assets")
		}

		// Metadata search uses the same endpoint and combines conditions with
		// AND, matching the official mobile MetadataSearchDto behavior.
		var asset Asset
		if err := app.store.DB.First(&asset, "id = ?", assetID).Error; err != nil {
			t.Fatal(err)
		}
		if err := app.store.DB.Model(&Exif{}).Where("asset_id = ?", assetID).Updates(map[string]any{"description": "regression searchable text", "rating": 4, "make": "TestCam"}).Error; err != nil {
			t.Fatal(err)
		}
		w = do(r, "POST", "/api/search/metadata", token, mustJSON(t, map[string]any{"originalFileName": asset.OriginalFileName, "description": "regression searchable text", "type": asset.Type, "rating": 4}), "application/json")
		if w.Code != 200 {
			t.Fatalf("metadata search -> %d: %s", w.Code, w.Body.String())
		}
		var found searchResponse
		if err := json.Unmarshal(w.Body.Bytes(), &found); err != nil {
			t.Fatal(err)
		}
		if found.Assets.Total != 1 || found.Assets.Items[0].ID != assetID {
			t.Fatalf("metadata search result=%+v", found.Assets)
		}
	})

	t.Run("albums", func(t *testing.T) {
		w := do(r, "POST", "/api/albums", token, mustJSON(t, map[string]string{"albumName": "Reg Album"}), "application/json")
		if w.Code != 201 {
			t.Fatalf("album create -> %d", w.Code)
		}
		var al struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &al)
		if al.ID == "" {
			t.Fatal("album id empty")
		}
		if w := do(r, "POST", "/api/albums/"+al.ID+"/assets", token, mustJSON(t, map[string][]string{"ids": {assetID}}), "application/json"); w.Code != 200 {
			t.Errorf("album add assets -> %d", w.Code)
		}
		if w := do(r, "GET", "/api/albums", token, nil, ""); w.Code != 200 {
			t.Errorf("album list -> %d", w.Code)
		}
		if w := do(r, "GET", "/api/albums/"+al.ID, token, nil, ""); w.Code != 200 {
			t.Errorf("album get -> %d", w.Code)
		}
		if w := do(r, "PUT", "/api/albums/"+al.ID+"/cover", token, mustJSON(t, map[string]string{"assetId": assetID}), "application/json"); w.Code != 200 {
			t.Errorf("album cover -> %d", w.Code)
		}
	})

	t.Run("libraries-scan", func(t *testing.T) {
		scanDir := t.TempDir()
		writeRegressionJPEG(t, filepath.Join(scanDir, "a.jpg"), 80, 60, 10, 20, 30)
		writeRegressionJPEG(t, filepath.Join(scanDir, "b.jpg"), 48, 32, 220, 60, 90)
		w := do(r, "POST", "/api/libraries", token, mustJSON(t, map[string]string{"name": "RegLib", "type": "EXTERNAL", "importPaths": scanDir}), "application/json")
		if w.Code != 201 {
			t.Fatalf("library create -> %d: %s", w.Code, w.Body.String())
		}
		var lib struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &lib)
		w = do(r, "POST", "/api/libraries/"+lib.ID+"/scan", token, nil, "")
		if w.Code != 200 {
			t.Fatalf("scan -> %d: %s", w.Code, w.Body.String())
		}
		var sr struct {
			Imported int `json:"imported"`
			Skipped  int `json:"skipped"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &sr)
		if sr.Imported != 2 {
			t.Errorf("expected 2 imported, got %d (skipped=%d)", sr.Imported, sr.Skipped)
		}
		if w := do(r, "GET", "/api/libraries/"+lib.ID+"/statistics", token, nil, ""); w.Code != 200 {
			t.Errorf("library stats -> %d", w.Code)
		}
	})

	t.Run("map-markers", func(t *testing.T) {
		var adm User
		if err := app.store.DB.First(&adm).Error; err != nil {
			t.Fatalf("admin lookup: %v", err)
		}
		now := time.Now().UTC()
		a1 := Asset{ID: newUUID(), OwnerID: adm.ID, Type: "IMAGE", IsTrash: false, CreatedAt: now, UpdatedAt: now}
		_ = app.store.DB.Create(&a1).Error
		app.store.DB.Create(&Exif{ID: newUUID(), AssetID: a1.ID, Latitude: 35.68, Longitude: 139.69, City: "Tokyo", Country: "Japan"})
		w := do(r, "GET", "/api/map/markers", token, nil, "")
		if w.Code != 200 {
			t.Fatalf("map markers -> %d", w.Code)
		}
		var markers []MapMarker
		_ = json.Unmarshal(w.Body.Bytes(), &markers)
		if len(markers) == 0 {
			t.Errorf("expected at least 1 map marker")
		}
	})

	t.Run("sharing-public", func(t *testing.T) {
		w := do(r, "POST", "/api/shared-links", token, mustJSON(t, map[string]string{"type": "INDIVIDUAL", "assetId": assetID}), "application/json")
		if w.Code != 201 {
			t.Fatalf("shared-link create -> %d: %s", w.Code, w.Body.String())
		}
		var sl struct {
			Key string `json:"key"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &sl)
		if sl.Key == "" {
			t.Fatal("share key empty")
		}
		// public (no auth) view
		if w := do(r, "GET", "/api/share/"+sl.Key, "", nil, ""); w.Code != 200 {
			t.Errorf("public share view -> %d", w.Code)
		}
		if w := do(r, "GET", "/api/share/"+sl.Key+"/thumbnail/"+assetID, "", nil, ""); w.Code != 200 {
			t.Errorf("public share thumbnail -> %d", w.Code)
		}
	})

	t.Run("timeline", func(t *testing.T) {
		w := do(r, "GET", "/api/timeline/buckets?size=MONTH", token, nil, "")
		if w.Code != 200 {
			t.Errorf("timeline buckets -> %d", w.Code)
		}
		if w := do(r, "GET", "/api/timeline/assets?take=10", token, nil, ""); w.Code != 200 {
			t.Errorf("timeline assets -> %d", w.Code)
		}
	})

	t.Run("tags", func(t *testing.T) {
		w := do(r, "POST", "/api/tags", token, mustJSON(t, map[string]string{"name": "RegTag", "type": "MEDIA"}), "application/json")
		if w.Code != 201 {
			t.Errorf("tag create -> %d", w.Code)
		}
		if w := do(r, "GET", "/api/tags", token, nil, ""); w.Code != 200 {
			t.Errorf("tag list -> %d", w.Code)
		}
	})

	t.Run("search", func(t *testing.T) {
		// metadata search returns 200 (results may be empty)
		w := do(r, "POST", "/api/search/metadata", token, mustJSON(t, map[string]string{"query": "reg"}), "application/json")
		if w.Code != 200 {
			t.Errorf("search/metadata -> %d", w.Code)
		}
		for _, p := range []string{"/api/search/person", "/api/search/suggestions"} {
			if w := do(r, "POST", p, token, mustJSON(t, gin.H{}), "application/json"); w.Code != 200 {
				t.Errorf("%s -> %d", p, w.Code)
			}
		}
		// /api/search/explore is a GET endpoint
		if w := do(r, "GET", "/api/search/explore", token, nil, ""); w.Code != 200 {
			t.Errorf("/api/search/explore -> %d", w.Code)
		}
	})

	t.Run("people-stub", func(t *testing.T) {
		// people is a known stub: endpoint should not 500, but returns empty
		w := do(r, "GET", "/api/people", token, nil, "")
		if w.Code != 200 {
			t.Errorf("people list -> %d", w.Code)
		}
	})

	t.Run("jobs-stub", func(t *testing.T) {
		if w := do(r, "GET", "/api/jobs", token, nil, ""); w.Code != 200 {
			t.Errorf("jobs list -> %d", w.Code)
		}
		// The Go port runs real background jobs for the supported ids
		// (thumbnailGeneration / metadataExtraction / videoConversion /
		// duplicateDetection); unsupported AI jobs return 200 with an
		// unsupported marker; unknown ids return 400.
		if w := do(r, "POST", "/api/jobs/thumbnailGeneration", token, mustJSON(t, gin.H{}), "application/json"); w.Code != 200 {
			t.Errorf("jobs command (supported) -> %d", w.Code)
		}
		if w := do(r, "POST", "/api/jobs/facialRecognition", token, mustJSON(t, gin.H{}), "application/json"); w.Code != 200 {
			t.Errorf("jobs command (unsupported) -> %d", w.Code)
		}
		if w := do(r, "POST", "/api/jobs/notARealJob", token, mustJSON(t, gin.H{}), "application/json"); w.Code != 400 {
			t.Errorf("jobs command (unknown) -> %d", w.Code)
		}
	})

	t.Run("trash-and-activities", func(t *testing.T) {
		if w := do(r, "GET", "/api/trash", token, nil, ""); w.Code != 200 {
			t.Errorf("trash list -> %d", w.Code)
		}
		if w := do(r, "GET", "/api/activities", token, nil, ""); w.Code != 200 {
			t.Errorf("activities list -> %d", w.Code)
		}
	})
}

func mustJSON(t *testing.T, v interface{}) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestSharedLinksPersistFields guards that shared-link fields are persisted
// (not just echoed from the request body) and re-read correctly.
func TestSharedLinksPersistFields(t *testing.T) {
	_, r, token := newTestServer(t)

	body := mustJSON(t, map[string]any{
		"type":          "INDIVIDUAL",
		"allowDownload": true,
		"allowUpload":   false,
		"description":   "family trip",
		"showMetadata":  true,
		"slug":          "mytrip",
	})
	w := do(r, "POST", "/api/shared-links", token, body, "application/json")
	if w.Code != http.StatusCreated {
		t.Fatalf("create -> %d: %s", w.Code, w.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil || created.ID == "" {
		t.Fatalf("create decode: %v body=%s", err, w.Body.String())
	}

	// Re-read via GET /shared-links/:id — fields must come from the DB.
	w = do(r, "GET", "/api/shared-links/"+created.ID, token, nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("get -> %d", w.Code)
	}
	var got struct {
		AllowDownload bool    `json:"allowDownload"`
		AllowUpload   bool    `json:"allowUpload"`
		Description   *string `json:"description"`
		ShowMetadata  bool    `json:"showMetadata"`
		Slug          *string `json:"slug"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("get decode: %v", err)
	}
	if !got.AllowDownload || got.AllowUpload {
		t.Errorf("allowDownload/allowUpload not persisted: %+v", got)
	}
	if got.Description == nil || *got.Description != "family trip" {
		t.Errorf("description not persisted: %+v", got)
	}
	if !got.ShowMetadata {
		t.Errorf("showMetadata not persisted: %+v", got)
	}
	if got.Slug == nil || *got.Slug != "mytrip" {
		t.Errorf("slug not persisted: %+v", got)
	}

	// List must also surface persisted fields.
	w = do(r, "GET", "/api/shared-links", token, nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("list -> %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "mytrip") {
		t.Errorf("list missing persisted slug: %s", w.Body.String())
	}
}

// TestApiKeySingleReadAndUpdate guards GET/PUT /api-keys/:id.
func TestApiKeySingleReadAndUpdate(t *testing.T) {
	_, r, token := newTestServer(t)

	w := do(r, "POST", "/api/api-keys", token, mustJSON(t, map[string]string{"name": "mobile"}), "application/json")
	if w.Code != http.StatusCreated {
		t.Fatalf("create -> %d: %s", w.Code, w.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil || created.ID == "" {
		t.Fatalf("create decode: %v", err)
	}

	w = do(r, "GET", "/api/api-keys/"+created.ID, token, nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("get single -> %d", w.Code)
	}

	w = do(r, "PUT", "/api/api-keys/"+created.ID, token, mustJSON(t, map[string]string{"name": "renamed"}), "application/json")
	if w.Code != http.StatusOK {
		t.Fatalf("update -> %d: %s", w.Code, w.Body.String())
	}
	var upd struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &upd); err != nil {
		t.Fatalf("update decode: %v", err)
	}
	if upd.Name != "renamed" {
		t.Errorf("name not updated: %q", upd.Name)
	}
}

// TestSearchAggregations guards the real search/statistics/random/large-assets
// endpoints (asset.size is now persisted on ingest).
func TestSearchAggregations(t *testing.T) {
	app, r, token := newTestServer(t)
	uploadAsset(t, r, token, "agg1.jpg")
	uploadAsset(t, r, token, "agg2.jpg")

	w := do(r, "POST", "/api/search/statistics", token, mustJSON(t, map[string]any{}), "application/json")
	if w.Code != http.StatusOK {
		t.Fatalf("statistics -> %d: %s", w.Code, w.Body.String())
	}
	var stats struct {
		Total  int64 `json:"total"`
		Photos int64 `json:"photos"`
		Videos int64 `json:"videos"`
		Usage  int64 `json:"usage"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &stats); err != nil {
		t.Fatalf("statistics decode: %v", err)
	}
	if stats.Total < 2 {
		t.Errorf("expected >=2 assets, got %d", stats.Total)
	}

	w = do(r, "POST", "/api/search/random", token, mustJSON(t, map[string]any{"take": 10}), "application/json")
	if w.Code != http.StatusOK {
		t.Fatalf("random -> %d", w.Code)
	}
	var rnd []AssetResponse
	if err := json.Unmarshal(w.Body.Bytes(), &rnd); err != nil {
		t.Fatalf("random decode: %v", err)
	}
	if len(rnd) < 1 {
		t.Errorf("random returned no assets")
	}

	// large-assets with size 0 must include everything (size >= 0).
	w = do(r, "POST", "/api/search/large-assets", token, mustJSON(t, map[string]any{"size": 0, "take": 50}), "application/json")
	if w.Code != http.StatusOK {
		t.Fatalf("large-assets -> %d", w.Code)
	}
	var large []AssetResponse
	if err := json.Unmarshal(w.Body.Bytes(), &large); err != nil {
		t.Fatalf("large-assets decode: %v", err)
	}
	if len(large) < 2 {
		t.Errorf("large-assets returned %d, want >=2", len(large))
	}

	// Smart search requires an ML embedding backend, which is not available
	// under the pure-Go baseline. It must fail honestly instead of returning
	// a fake successful empty result.
	w = do(r, "POST", "/api/search/smart", token, mustJSON(t, map[string]string{"query": "cat"}), "application/json")
	if w.Code != http.StatusNotImplemented {
		t.Errorf("smart search -> %d, want 501", w.Code)
	}

	_ = app
}

// TestAlbumMapMarkers guards GET /albums/:id/map-markers uses real GPS EXIF.
func TestAlbumMapMarkers(t *testing.T) {
	app, r, token := newTestServer(t)
	id := uploadAsset(t, r, token, "marker.jpg")
	// attach GPS exif
	app.store.DB.Create(&Exif{ID: id, AssetID: id, Latitude: 48.85, Longitude: 2.35, City: "Paris"})

	// create album and add the asset
	w := do(r, "POST", "/api/albums", token, mustJSON(t, map[string]any{"albumName": "Trip", "assetIds": []string{id}}), "application/json")
	var al struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &al); err != nil || al.ID == "" {
		t.Fatalf("album create: %v body=%s", err, w.Body.String())
	}
	w = do(r, "GET", "/api/albums/"+al.ID+"/map-markers", token, nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("map-markers -> %d: %s", w.Code, w.Body.String())
	}
	var mm struct {
		Markers []struct {
			Lat float64 `json:"lat"`
			Lon float64 `json:"lon"`
		} `json:"markers"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &mm); err != nil {
		t.Fatalf("map-markers decode: %v", err)
	}
	found := false
	for _, m := range mm.Markers {
		if m.Lat == 48.85 && m.Lon == 2.35 {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a Paris marker among %+v", mm.Markers)
	}
}

// TestAssetCopyAndBulkMetadata guards PUT /assets/copy and PUT /assets/metadata.
func TestAssetCopyAndBulkMetadata(t *testing.T) {
	_, r, token := newTestServer(t)
	id := uploadAsset(t, r, token, "copy.jpg")

	// copy
	w := do(r, "PUT", "/api/assets/copy", token, mustJSON(t, map[string]any{"ids": []string{id}}), "application/json")
	if w.Code != http.StatusOK {
		t.Fatalf("copy -> %d: %s", w.Code, w.Body.String())
	}
	var cp struct {
		IDs []string `json:"ids"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &cp); err != nil || len(cp.IDs) != 1 {
		t.Fatalf("copy decode: %v body=%s", err, w.Body.String())
	}
	newID := cp.IDs[0]
	if newID == id {
		t.Error("copy returned same id")
	}

	// bulk metadata
	w = do(r, "PUT", "/api/assets/metadata", token,
		mustJSON(t, map[string]any{"ids": []string{newID}, "description": "copied!"}), "application/json")
	if w.Code != http.StatusOK {
		t.Fatalf("bulk metadata -> %d: %s", w.Code, w.Body.String())
	}
	w = do(r, "GET", "/api/assets/"+newID+"/metadata", token, nil, "")
	var ex struct {
		Description string `json:"description"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &ex); err != nil {
		t.Fatalf("metadata get: %v", err)
	}
	if ex.Description != "copied!" {
		t.Errorf("description not applied: %q", ex.Description)
	}
}
