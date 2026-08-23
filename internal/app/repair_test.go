package app

import (
	"os"
	"path/filepath"
	"testing"

	imgproc "immich-go/internal/image"
)

// S5: repair steps must fix seeded inconsistencies and be idempotent.

func TestRepairLivePhotoDanglingRef(t *testing.T) {
	app := newTestApp(t)
	if err := app.store.DB.Create(&Asset{
		ID: "img", OwnerID: "owner", Type: "IMAGE",
		LivePhotoVideoID: "ghost-video",
	}).Error; err != nil {
		t.Fatal(err)
	}
	scanned, fixed, err := app.repairLivePhotoDanglingRef()
	if err != nil {
		t.Fatal(err)
	}
	if fixed != 1 {
		t.Fatalf("fixed=%d want 1", fixed)
	}
	var img Asset
	app.store.DB.First(&img, "id = ?", "img")
	if img.LivePhotoVideoID != "" {
		t.Fatal("dangling ref not cleared")
	}
	// Idempotent.
	_, fixed2, _ := app.repairLivePhotoDanglingRef()
	if fixed2 != 0 {
		t.Fatalf("second run fixed=%d want 0", fixed2)
	}
	_ = scanned
}

func TestRepairArchiveVisibilitySync(t *testing.T) {
	app := newTestApp(t)
	if err := app.store.DB.Create(&Asset{
		ID: "a1", OwnerID: "owner", Type: "IMAGE", Visibility: "archive",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := app.store.DB.Create(&Asset{
		ID: "a2", OwnerID: "owner", Type: "IMAGE", IsArchived: true,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if _, fixed, _ := app.repairArchiveVisibilitySync(); fixed < 2 {
		t.Fatalf("fixed=%d want >=2", fixed)
	}
	var a1, a2 Asset
	app.store.DB.First(&a1, "id = ?", "a1")
	app.store.DB.First(&a2, "id = ?", "a2")
	if !a1.IsArchived || a2.IsArchived {
		t.Fatalf("sync wrong: a1.archived=%v a2.archived=%v", a1.IsArchived, a2.IsArchived)
	}
}

func TestRepairThumbnailStateSync(t *testing.T) {
	app := newTestApp(t)
	// Flag says yes but file missing -> reset. File exists but flag no -> set.
	missing := filepath.Join(t.TempDir(), "gone.jpg")
	existing := filepath.Join(t.TempDir(), "here.jpg")
	os.WriteFile(existing, []byte("x"), 0o644)
	app.store.DB.Create(&Asset{ID: "m", OwnerID: "o", Type: "IMAGE", HasThumbnail: true, ResizePath: missing})
	app.store.DB.Create(&Asset{ID: "e", OwnerID: "o", Type: "IMAGE", HasThumbnail: false, ResizePath: existing})
	_, fixed, err := app.repairThumbnailStateSync()
	if err != nil {
		t.Fatal(err)
	}
	if fixed != 2 {
		t.Fatalf("fixed=%d want 2", fixed)
	}
	var m, e Asset
	app.store.DB.First(&m, "id = ?", "m")
	app.store.DB.First(&e, "id = ?", "e")
	if m.HasThumbnail || m.ResizePath != "" {
		t.Fatal("missing-file asset not reset")
	}
	if !e.HasThumbnail {
		t.Fatal("existing-file asset not flagged")
	}
}

func TestRepairOrphanFiles(t *testing.T) {
	app := newTestApp(t)
	res := t.TempDir()
	app.cfg.ResourceDir = res
	upDir := filepath.Join(res, "upload")
	thDir := filepath.Join(res, "thumbnail")
	os.MkdirAll(upDir, 0o755)
	os.MkdirAll(thDir, 0o755)

	app.store.DB.Create(&Asset{ID: "real-asset", OwnerID: "o", Type: "IMAGE"})

	// orphan preview (asset gone) + valid preview
	os.WriteFile(filepath.Join(upDir, "u.real-asset.preview.jpg"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(upDir, "u.dead.preview.jpg"), []byte("x"), 0o644)
	// orphan thumb + valid thumb
	os.WriteFile(filepath.Join(thDir, "real-asset.jpg"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(thDir, "dead.jpg"), []byte("x"), 0o644)

	if _, removedP, err := app.repairOrphanPreviewCache(); err != nil || removedP != 1 {
		t.Fatalf("preview removed=%d err=%v want 1", removedP, err)
	}
	if _, removedT, err := app.repairOrphanThumbnailFiles(); err != nil || removedT != 1 {
		t.Fatalf("thumbnail removed=%d err=%v want 1", removedT, err)
	}
	if _, err := os.Stat(filepath.Join(upDir, "u.real-asset.preview.jpg")); err != nil {
		t.Fatal("valid preview wrongly removed")
	}
	if _, err := os.Stat(filepath.Join(thDir, "real-asset.jpg")); err != nil {
		t.Fatal("valid thumbnail wrongly removed")
	}
}

func TestRepairAlbumDanglingRows(t *testing.T) {
	app := newTestApp(t)
	app.store.DB.Exec("INSERT INTO albums (id, owner_id, album_name) VALUES ('al', 'o', 'x')")
	app.store.DB.Exec("INSERT INTO albums_assets_assets (album_id, asset_id) VALUES ('al', 'ghost')")
	_, fixed, err := app.repairAlbumAssetDanglingRows()
	if err != nil {
		t.Fatal(err)
	}
	if fixed != 1 {
		t.Fatalf("fixed=%d want 1", fixed)
	}
	var n int64
	app.store.DB.Raw("SELECT COUNT(*) FROM albums_assets_assets WHERE asset_id='ghost'").Scan(&n)
	if n != 0 {
		t.Fatal("dangling row survived")
	}
}

func TestRunRepairPassIdempotentAndSafe(t *testing.T) {
	app := newTestApp(t)
	// Must not panic with empty DB and must be safely re-runnable.
	app.runRepairPass()
	app.runRepairPass()
}

// S8: stale-version caches must be selected by the default (non-force)
// thumbnailGeneration items query.
func TestStaleVersionSelectedByDefaultItems(t *testing.T) {
	app := newTestApp(t)
	// fresh heic cache (v1, current) — NOT stale
	app.store.DB.Create(&Asset{ID: "fresh-heic", OwnerID: "o", Type: "IMAGE",
		HasThumbnail: true, PreviewFamily: "heic", ThumbVersion: 1, PreviewVer: 1})
	// legacy jpeg cache with NO family recorded and jpeg still at v1 — not stale
	app.store.DB.Create(&Asset{ID: "legacy-jpeg", OwnerID: "o", Type: "IMAGE",
		HasThumbnail: true})
	// With all families at their current versions, nothing with a complete
	// stamp is selected by default items() — only genuinely missing ones.
	spec := app.jobRegistry()["thumbnailGeneration"]
	items, _ := spec.items(app, false)
	got := map[string]bool{}
	for _, it := range items {
		got[it.ID] = true
	}
	if got["fresh-heic"] {
		t.Fatal("current-version heic cache wrongly selected")
	}
	// Bump heic: now the v1 heic asset becomes stale and MUST be picked up.
	imgproc.BumpCacheVersion(imgproc.FamilyHEIC)
	items2, _ := spec.items(app, false)
	got2 := map[string]bool{}
	for _, it := range items2 {
		got2[it.ID] = true
	}
	if !got2["fresh-heic"] {
		t.Fatal("stale heic (v1 < current v2) not selected after bump")
	}
	if !got["legacy-jpeg"] || !got2["legacy-jpeg"] {
		t.Log("legacy-jpeg selection:", got["legacy-jpeg"], got2["legacy-jpeg"])
	}
}
