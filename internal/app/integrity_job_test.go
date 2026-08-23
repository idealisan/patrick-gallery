package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// Epic webui-maintenance-parity WI-2/WI-3: the nine integrity-* manual jobs
// drive a persistent report table with official semantics, and the report
// endpoints expose the official DTO shapes.

// ctxAsAdmin builds a test gin context authenticated as the seeded admin.
// Handlers write their response into the supplied recorder.
func ctxAsAdmin(t *testing.T, app *App, w *httptest.ResponseRecorder, req *http.Request) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	var admin User
	if err := app.store.DB.First(&admin).Error; err != nil {
		t.Fatalf("no seeded admin: %v", err)
	}
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Set(ctxUserID, admin.ID)
	return c
}

func runIntegrityJob(t *testing.T, app *App, name string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	spec := app.jobRegistry()[name]
	if !spec.supported {
		t.Fatalf("%s must be supported", name)
	}
	items, err := spec.items(app, false)
	if err != nil {
		t.Fatalf("%s items: %v", name, err)
	}
	for _, it := range items {
		ok, err := spec.run(app, it)
		if !ok || err != nil {
			t.Fatalf("%s run(%s): ok=%v err=%v", name, it.ID, ok, err)
		}
	}
}

func TestIntegrityUntrackedScanRefreshDeleteAll(t *testing.T) {
	app := newTestApp(t)

	stray := filepath.Join(app.cfg.ResourceDir, "upload", "stray.bin")
	if err := os.WriteFile(stray, []byte("orphan"), 0o644); err != nil {
		t.Fatal(err)
	}

	runIntegrityJob(t, app, "integrity-untracked-files")

	var rows []IntegrityReport
	app.store.DB.Where("type = ?", "untracked_file").Find(&rows)
	found := false
	for _, r := range rows {
		if r.Path == stray {
			found = true
		}
		if len(r.ID) != 36 {
			t.Fatalf("report id %q is not a uuid", r.ID)
		}
	}
	if !found {
		t.Fatal("stray file not reported as untracked")
	}

	// refresh: file gone → row dropped
	os.Remove(stray)
	runIntegrityJob(t, app, "integrity-untracked-files-refresh")
	app.store.DB.Where("type = ?", "untracked_file").Find(&rows)
	for _, r := range rows {
		if r.Path == stray {
			t.Fatal("stale untracked row survived refresh")
		}
	}

	// delete-all: flag again then wipe (file + row)
	os.WriteFile(stray, []byte("orphan2"), 0o644)
	runIntegrityJob(t, app, "integrity-untracked-files")
	runIntegrityJob(t, app, "integrity-untracked-files-delete-all")
	app.store.DB.Where("type = ?", "untracked_file").Find(&rows)
	for _, r := range rows {
		if r.Path == stray {
			t.Fatal("untracked row survived delete-all")
		}
	}
	if _, err := os.Stat(stray); !os.IsNotExist(err) {
		t.Fatal("delete-all did not remove the flagged file")
	}
}

func TestIntegrityMissingAndDeleteTrashesAsset(t *testing.T) {
	app := newTestApp(t)
	gone := filepath.Join(t.TempDir(), "vanished.jpg")
	as := Asset{ID: newUUID(), OwnerID: "owner", Type: "IMAGE", OriginalPath: gone,
		OriginalFileName: "vanished.jpg", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := app.store.DB.Create(&as).Error; err != nil {
		t.Fatal(err)
	}

	runIntegrityJob(t, app, "integrity-missing-files")

	var row IntegrityReport
	if err := app.store.DB.Where("type = ? AND path = ?", "missing_file", gone).First(&row).Error; err != nil {
		t.Fatalf("missing file not reported: %v", err)
	}
	if row.AssetID == nil || *row.AssetID != as.ID {
		t.Fatal("missing report lacks assetId origin")
	}

	// single DELETE endpoint trashes the asset (official disposition)
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c := ctxAsAdmin(t, app, rec, httptest.NewRequest(http.MethodDelete, "/api/admin/integrity/report/"+row.ID, nil))
	c.Params = gin.Params{{Key: "id", Value: row.ID}}
	app.handleIntegrityReportDelete(c)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete status %d: %s", rec.Code, rec.Body.String())
	}

	var after Asset
	app.store.DB.First(&after, "id = ?", as.ID)
	if !after.IsTrash || after.TrashedAt == nil {
		t.Fatal("asset not moved to trash by report delete")
	}
	var cnt int64
	app.store.DB.Model(&IntegrityReport{}).Where("type = ?", "missing_file").Count(&cnt)
	if cnt != 0 {
		t.Fatal("report row not removed")
	}
}

func TestIntegrityChecksumMismatchFlow(t *testing.T) {
	app := newTestApp(t)
	p := filepath.Join(app.cfg.ResourceDir, "upload", "photo.jpg")
	os.WriteFile(p, []byte("real-bytes"), 0o644)
	sum, _ := sha1File(p)
	wrong := "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	base := time.Now().Add(-2 * time.Hour)
	as := Asset{ID: newUUID(), OwnerID: "owner", Type: "IMAGE", OriginalPath: p,
		Checksum: wrong, HasThumbnail: true,
		FileCreatedAt: base, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	app.store.DB.Create(&as)

	runIntegrityJob(t, app, "integrity-checksum-mismatch")
	var row IntegrityReport
	if err := app.store.DB.Where("type = ? AND path = ?", "checksum_mismatch", p).First(&row).Error; err != nil {
		t.Fatalf("checksum mismatch not reported: %v", err)
	}

	// repair the checksum → refresh drops the row
	app.store.DB.Model(&Asset{}).Where("id = ?", as.ID).Update("checksum", sum)
	runIntegrityJob(t, app, "integrity-checksum-mismatch-refresh")
	var cnt int64
	app.store.DB.Model(&IntegrityReport{}).Where("type = ?", "checksum_mismatch").Count(&cnt)
	if cnt != 0 {
		t.Fatal("stale checksum row survived refresh")
	}

	// checkpoint persisted; a follow-up scan only sees assets created after it
	var cp integrityCheckpoint
	if !app.metaGet(integrityCheckpointKey, &cp) || cp.Date == "" {
		t.Fatal("checksum checkpoint not persisted")
	}
	marker := mustParseRFC(t, cp.Date)
	newer := filepath.Join(app.cfg.ResourceDir, "upload", "later.jpg")
	os.WriteFile(newer, []byte("later-bytes"), 0o644)
	app.store.DB.Create(&Asset{ID: newUUID(), OwnerID: "owner", Type: "IMAGE",
		OriginalPath: newer, Checksum: "ffffffffffffffffffffffffffffffffffffffff",
		FileCreatedAt: marker.Add(time.Hour), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()})
	items2, err := app.integrityChecksumItems(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(items2) != 2 { // 1 new asset + checkpoint sentinel
		t.Fatalf("resume scan should see only the new asset, got %d items", len(items2))
	}
	if items2[0].Path != newer {
		t.Fatalf("resume picked wrong asset: %s", items2[0].Path)
	}
}

func mustParseRFC(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t.Fatalf("bad checkpoint date %q: %v", s, err)
	}
	return ts
}

func TestIntegrityReportEndpointsShape(t *testing.T) {
	app := newTestApp(t)
	p1 := filepath.Join(app.cfg.ResourceDir, "upload", "u1.bin")
	p2 := filepath.Join(app.cfg.ResourceDir, "upload", "u2.bin")
	os.WriteFile(p1, []byte("a"), 0o644)
	os.WriteFile(p2, []byte("b"), 0o644)
	runIntegrityJob(t, app, "integrity-untracked-files")

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/integrity/report?type=untracked_file&limit=1", nil)
	c := ctxAsAdmin(t, app, rec, req)
	app.handleIntegrityReport(c)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var body struct {
		Items      []map[string]any `json:"items"`
		NextCursor string           `json:"nextCursor"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 1 || body.NextCursor == "" {
		t.Fatalf("pagination broken: %+v", body)
	}
	it := body.Items[0]
	for _, k := range []string{"id", "type", "path"} {
		if _, ok := it[k]; !ok {
			t.Fatalf("item missing %s: %v", k, it)
		}
	}
	if len(it) != 3 {
		t.Fatalf("item has extra fields beyond id/type/path: %v", it)
	}

	// CSV uses the official header
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/api/admin/integrity/report/untracked_file/csv", nil)
	c2 := ctxAsAdmin(t, app, rec2, req2)
	c2.Params = gin.Params{{Key: "id", Value: "untracked_file"}, {Key: "sub", Value: "csv"}}
	app.handleIntegrityReportFileOrCsv(c2)
	head, _, _ := strings.Cut(rec2.Body.String(), "\n")
	if head != "id,type,assetId,fileAssetId,path" {
		t.Fatalf("csv header %q", head)
	}

	// summary reflects persisted counts
	rec3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodGet, "/api/admin/integrity/summary", nil)
	c3 := ctxAsAdmin(t, app, rec3, req3)
	app.handleIntegritySummary(c3)
	var summary map[string]int
	json.Unmarshal(rec3.Body.Bytes(), &summary)
	if summary["untracked_file"] < 2 {
		t.Fatalf("summary counts wrong: %v", summary)
	}
}

func TestMaintenanceModeVirtualRestart(t *testing.T) {
	app := newTestApp(t)
	gin.SetMode(gin.TestMode)

	// official toggle: {action:"start"} → 201 {jwt}
	rec := httptest.NewRecorder()
	c := ctxAsAdmin(t, app, rec, httptest.NewRequest(http.MethodPost, "/api/admin/maintenance",
		bytes.NewBufferString(`{"action":"start"}`)))
	app.handleMaintenanceSet(c)
	if rec.Code != http.StatusCreated {
		t.Fatalf("start status %d: %s", rec.Code, rec.Body.String())
	}
	var started struct {
		JWT string `json:"jwt"`
	}
	json.Unmarshal(rec.Body.Bytes(), &started)
	if started.JWT == "" {
		t.Fatal("start response missing jwt")
	}
	if !app.inMaintenance() {
		t.Fatal("maintenance flag not set")
	}

	// status reports the active action (enum-valid)
	rec2 := httptest.NewRecorder()
	c2 := ctxAsAdmin(t, app, rec2, httptest.NewRequest(http.MethodGet, "/api/admin/maintenance/status", nil))
	app.handleMaintenanceStatus(c2)
	var st struct {
		Active bool   `json:"active"`
		Action string `json:"action"`
	}
	json.Unmarshal(rec2.Body.Bytes(), &st)
	if !st.Active || st.Action != "start" {
		t.Fatalf("status %+v", st)
	}

	// end → back to normal; official idle shape {active:false, action:"end"}
	rec3 := httptest.NewRecorder()
	c3 := ctxAsAdmin(t, app, rec3, httptest.NewRequest(http.MethodPost, "/api/admin/maintenance",
		bytes.NewBufferString(`{"action":"end"}`)))
	app.handleMaintenanceSet(c3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("end status %d", rec3.Code)
	}
	rec4 := httptest.NewRecorder()
	c4 := ctxAsAdmin(t, app, rec4, httptest.NewRequest(http.MethodGet, "/api/admin/maintenance/status", nil))
	app.handleMaintenanceStatus(c4)
	json.Unmarshal(rec4.Body.Bytes(), &st)
	if st.Active || st.Action != "end" {
		t.Fatalf("idle status %+v", st)
	}
	if app.inMaintenance() {
		t.Fatal("maintenance still active after end")
	}
}

func TestMaintenanceUnknownActionRejected(t *testing.T) {
	app := newTestApp(t)
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c := ctxAsAdmin(t, app, rec, httptest.NewRequest(http.MethodPost, "/api/admin/maintenance",
		bytes.NewBufferString(`{"action":"enable"}`)))
	app.handleMaintenanceSet(c)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("legacy action must be rejected, got %d", rec.Code)
	}
}
