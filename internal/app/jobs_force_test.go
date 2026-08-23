package app

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

// S1: force:true must make job items() return ALL eligible assets, while the
// default (no force) keeps the "only missing byproducts" filter.

func ids(items []jobItem) map[string]string {
	m := make(map[string]string, len(items))
	for _, it := range items {
		m[it.ID] = it.Path
	}
	return m
}

func TestJobItemsForceThumbnailGeneration(t *testing.T) {
	app := newTestApp(t)
	reg := app.jobRegistry()
	spec, ok := reg["thumbnailGeneration"]
	if !ok || !spec.supported {
		t.Fatal("thumbnailGeneration spec missing")
	}

	// One asset WITH a thumbnail (simulated) — default run must skip it.
	if err := app.store.DB.Create(&Asset{
		ID: "with-thumb", OwnerID: "owner", Type: "IMAGE",
		OriginalPath: filepath.Join(t.TempDir(), "a.jpg"),
		HasThumbnail: true,
	}).Error; err != nil {
		t.Fatal(err)
	}
	// One asset WITHOUT a thumbnail — always included.
	if err := app.store.DB.Create(&Asset{
		ID: "no-thumb", OwnerID: "owner", Type: "IMAGE",
		OriginalPath: filepath.Join(t.TempDir(), "b.jpg"),
	}).Error; err != nil {
		t.Fatal(err)
	}

	def, err := spec.items(app, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(def) != 1 || def[0].ID != "no-thumb" {
		t.Fatalf("default items = %v, want only [no-thumb]", ids(def))
	}

	forced, err := spec.items(app, true)
	if err != nil {
		t.Fatal(err)
	}
	got := ids(forced)
	if len(got) != 2 || got["with-thumb"] == "" || got["no-thumb"] == "" {
		t.Fatalf("forced items = %v, want both assets", got)
	}
}

func TestJobItemsForceMetadataExtraction(t *testing.T) {
	app := newTestApp(t)
	spec := app.jobRegistry()["metadataExtraction"]

	// IMAGE with exif already set — skipped by default, forced otherwise.
	if err := app.store.DB.Create(&Asset{
		ID: "has-exif", OwnerID: "owner", Type: "IMAGE", ExifID: "exif-1",
		OriginalPath: filepath.Join(t.TempDir(), "c.jpg"),
	}).Error; err != nil {
		t.Fatal(err)
	}
	def, _ := spec.items(app, false)
	if len(def) != 0 {
		t.Fatalf("default items = %v, want empty", ids(def))
	}
	forced, _ := spec.items(app, true)
	if g := ids(forced); g["has-exif"] == "" {
		t.Fatalf("forced items = %v, want has-exif", g)
	}
}

func TestDispatchJobForceEndpoint(t *testing.T) {
	app := newTestApp(t)
	// Asset with thumbnail present; force start via API must still enqueue it.
	if err := app.store.DB.Create(&Asset{
		ID: "thumb-exists", OwnerID: "owner", Type: "IMAGE", HasThumbnail: true,
		OriginalPath: filepath.Join(t.TempDir(), "d.jpg"),
	}).Error; err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/jobs/thumbnailGeneration",
		bytes.NewBufferString(`{"command":"start","force":true}`))
	req.Header.Set("Content-Type", "application/json")
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: "thumbnailGeneration"}}
	// admin check: seed an admin user + auth context (ctxUserID = uid string)
	if err := app.store.DB.Create(&User{
		ID: "owner", Email: "a@b.c", Name: "a", IsAdmin: true,
	}).Error; err != nil {
		t.Fatal(err)
	}
	c.Set(ctxUserID, "owner")
	app.handleJobCommand(c)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	st := app.jobStateFor("thumbnailGeneration")
	st.mu.Lock()
	total := st.total
	st.mu.Unlock()
	if total != 1 {
		t.Fatalf("force dispatch total = %d, want 1 (asset re-processed despite thumbnail)", total)
	}
}
