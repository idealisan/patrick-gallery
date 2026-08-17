package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
)

// These tests guard AGENTS.md hard rule #7 (No stubs): the handlers below must
// perform real work, never return a fake-success empty/constant body.

func TestServerStorageIsReal(t *testing.T) {
	_, r, token := newTestServer(t)
	w := do(r, "GET", "/api/server/storage", token, nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("storage -> %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		DiskSizeRaw         uint64  `json:"diskSizeRaw"`
		DiskAvailableRaw    uint64  `json:"diskAvailableRaw"`
		DiskUseRaw          uint64  `json:"diskUseRaw"`
		DiskUsagePercentage float64 `json:"diskUsagePercentage"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v body=%s", err, w.Body.String())
	}
	// The temp resource dir sits on a real filesystem, so size/available must
	// be reported truthfully (not the old constant 0).
	if body.DiskSizeRaw == 0 || body.DiskAvailableRaw == 0 {
		t.Errorf("expected real disk figures, got size=%d avail=%d", body.DiskSizeRaw, body.DiskAvailableRaw)
	}
	if body.DiskUsagePercentage < 0 || body.DiskUsagePercentage > 100 {
		t.Errorf("diskUsagePercentage out of range: %v", body.DiskUsagePercentage)
	}
}

func TestSearchCitiesIsReal(t *testing.T) {
	_, r, token := newTestServer(t)
	w := do(r, "GET", "/api/search/cities?name=Tokyo", token, nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("search/cities -> %d: %s", w.Code, w.Body.String())
	}
	var cities []struct {
		Name    string  `json:"name"`
		Country string  `json:"country"`
		Lat     float64 `json:"latitude"`
		Lon     float64 `json:"longitude"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &cities); err != nil {
		t.Fatalf("decode: %v body=%s", err, w.Body.String())
	}
	if len(cities) == 0 {
		t.Fatal("expected real city matches from the embedded GeoNames dataset, got empty")
	}
	found := false
	for _, c := range cities {
		if c.Name == "Tokyo" && c.Country != "" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a Tokyo entry with a resolved country, got %+v", cities[0])
	}
}

func TestSyncStreamIsReal(t *testing.T) {
	app, r, token := newTestServer(t)
	id := uploadAsset(t, r, token, "sync-stream.jpg")
	if id == "" {
		t.Fatal("upload failed")
	}
	w := do(r, "GET", "/api/sync/stream", token, nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("sync/stream -> %d: %s", w.Code, w.Body.String())
	}
	// The official server streams `application/jsonlines+json`: one JSON
	// object per line, each `{type, data, ack}`, terminated by SyncCompleteV1.
	// The official Flutter client splits on "\n" and jsonDecodes each line,
	// so we parse it the same way (not as a single JSON array).
	lines := bytes.Split(bytes.TrimSpace(w.Body.Bytes()), []byte("\n"))
	if len(lines) < 1 {
		t.Fatalf("sync/stream produced no lines")
	}
	var sawAsset bool
	for _, ln := range lines {
		var d map[string]any
		if err := json.Unmarshal(ln, &d); err != nil {
			t.Fatalf("sync/stream line not json: %v (line=%s)", err, ln)
		}
		if d["type"] == "AssetV2" {
			asset, ok := d["data"].(map[string]any)
			if ok && asset["id"] == id {
				sawAsset = true
			}
		}
	}
	if !sawAsset {
		t.Errorf("sync/stream did not include the uploaded asset %s; got %d lines", id, len(lines))
	}
	_ = app
}

func TestDownloadInfoIsReal(t *testing.T) {
	_, r, token := newTestServer(t)
	id := uploadAsset(t, r, token, "download-info.jpg")
	if id == "" {
		t.Fatal("upload failed")
	}
	body, _ := json.Marshal(map[string]any{"assetIds": []string{id}})
	w := do(r, "POST", "/api/download/info", token, body, "application/json")
	if w.Code != http.StatusOK {
		t.Fatalf("download/info -> %d: %s", w.Code, w.Body.String())
	}
	var out struct {
		Size int64 `json:"size"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v body=%s", err, w.Body.String())
	}
	if out.Size <= 0 {
		t.Errorf("expected real byte size > 0 for uploaded asset, got %d", out.Size)
	}
}

func TestDuplicatesResolveHidesPair(t *testing.T) {
	app, r, token := newTestServer(t)
	var admin User
	if err := app.store.DB.First(&admin).Error; err != nil {
		t.Fatalf("no admin: %v", err)
	}
	// Two assets sharing a checksum form a duplicate pair.
	resDir := app.cfg.ResourceDir
	app.store.DB.Create(&Asset{ID: "dup-keeper", OwnerID: admin.ID, Type: "IMAGE",
		Checksum: "shared-checksum", OriginalPath: filepath.Join(resDir, "keeper.jpg"), OriginalFileName: "keeper.jpg"})
	app.store.DB.Create(&Asset{ID: "dup-hidden", OwnerID: admin.ID, Type: "IMAGE",
		Checksum: "shared-checksum", OriginalPath: filepath.Join(resDir, "hidden.jpg"), OriginalFileName: "hidden.jpg"})

	if w := do(r, "GET", "/api/assets/duplicates", token, nil, ""); w.Code != http.StatusOK {
		t.Fatalf("duplicates -> %d", w.Code)
	} else {
		var groups []struct {
			Assets []string `json:"assets"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &groups)
		if len(groups) != 1 {
			t.Fatalf("expected 1 duplicate group before resolve, got %d", len(groups))
		}
	}

	// Resolve: keep dup-keeper, hide dup-hidden.
	rb, _ := json.Marshal(map[string]any{"assetId": "dup-keeper", "duplicateId": "dup-hidden"})
	w := do(r, "POST", "/api/duplicates/resolve", token, rb, "application/json")
	if w.Code != http.StatusOK {
		t.Fatalf("duplicates/resolve -> %d: %s", w.Code, w.Body.String())
	}

	// After resolve the pair must no longer surface as a duplicate.
	if w := do(r, "GET", "/api/assets/duplicates", token, nil, ""); w.Code != http.StatusOK {
		t.Fatalf("duplicates -> %d", w.Code)
	} else {
		var groups []struct {
			Assets []string `json:"assets"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &groups)
		if len(groups) != 0 {
			t.Errorf("expected 0 duplicate groups after resolve, got %d", len(groups))
		}
	}
}

// TestPeopleCRUDAndFaces501 guards that people endpoints do real work and the
// ML-dependent face endpoints return an honest 501 (no fake-empty stub).
func TestPeopleCRUDAndFaces501(t *testing.T) {
	_, r, token := newTestServer(t)

	w := do(r, "POST", "/api/people", token, mustJSON(t, map[string]string{"name": "Alice"}), "application/json")
	if w.Code != http.StatusCreated {
		t.Fatalf("people create -> %d: %s", w.Code, w.Body.String())
	}
	var created struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil || created.ID == "" {
		t.Fatalf("create decode: %v body=%s", err, w.Body.String())
	}

	w = do(r, "GET", "/api/people", token, nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("people list -> %d", w.Code)
	}
	var list struct {
		People []struct {
			ID     string `json:"id"`
			Assets struct {
				Total int64 `json:"total"`
			} `json:"assets"`
		} `json:"people"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("list decode: %v", err)
	}
	found := false
	for _, p := range list.People {
		if p.ID == created.ID {
			found = true
		}
	}
	if !found {
		t.Error("created person missing from list")
	}

	w = do(r, "GET", "/api/people/"+created.ID+"/statistics", token, nil, "")
	var stat struct {
		Assets int64 `json:"assets"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &stat); err != nil {
		t.Fatalf("stat decode: %v", err)
	}
	if stat.Assets != 0 {
		t.Errorf("expected 0 linked assets initially, got %d", stat.Assets)
	}

	assetID := uploadAsset(t, r, token, "person-reassign.jpg")
	if assetID == "" {
		t.Fatal("upload failed")
	}
	rb, _ := json.Marshal(map[string]string{"personId": created.ID})
	w = do(r, "PUT", "/api/people/"+assetID+"/reassign", token, rb, "application/json")
	if w.Code != http.StatusOK {
		t.Fatalf("reassign -> %d: %s", w.Code, w.Body.String())
	}
	w = do(r, "GET", "/api/people/"+created.ID+"/statistics", token, nil, "")
	if err := json.Unmarshal(w.Body.Bytes(), &stat); err != nil {
		t.Fatalf("stat decode: %v", err)
	}
	if stat.Assets != 1 {
		t.Errorf("expected 1 linked asset after reassign, got %d", stat.Assets)
	}

	w = do(r, "POST", "/api/people", token, mustJSON(t, map[string]string{"name": "Bob"}), "application/json")
	var bob struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &bob)
	mb, _ := json.Marshal(map[string]any{"ids": []string{created.ID}})
	w = do(r, "POST", "/api/people/"+bob.ID+"/merge", token, mb, "application/json")
	if w.Code != http.StatusOK {
		t.Fatalf("merge -> %d: %s", w.Code, w.Body.String())
	}
	w = do(r, "GET", "/api/people/"+created.ID, token, nil, "")
	if w.Code != http.StatusNotFound {
		t.Errorf("merged person should be 404, got %d", w.Code)
	}
	w = do(r, "GET", "/api/people/"+bob.ID+"/statistics", token, nil, "")
	if err := json.Unmarshal(w.Body.Bytes(), &stat); err != nil {
		t.Fatalf("stat decode: %v", err)
	}
	if stat.Assets != 1 {
		t.Errorf("expected Bob to inherit 1 asset, got %d", stat.Assets)
	}

	if w := do(r, "DELETE", "/api/people/"+bob.ID, token, nil, ""); w.Code != http.StatusOK {
		t.Errorf("delete -> %d", w.Code)
	}

	for _, m := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete} {
		path := "/api/faces"
		if m != http.MethodGet && m != http.MethodPost {
			path = "/api/faces/some-id"
		}
		w := do(r, m, path, token, nil, "")
		if w.Code != http.StatusNotImplemented {
			t.Errorf("%s /faces -> %d, want 501", m, w.Code)
		}
	}
}
