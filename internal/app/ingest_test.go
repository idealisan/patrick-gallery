package app

import (
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeTestJPEG encodes a small distinct-colour JPEG to path (a valid
// decodable image so the pure-Go thumbnailer can produce a poster). w/h and
// the rgb base are varied per file so different files get different checksums.
func writeTestJPEG(t *testing.T, path string, w, h int, r, g, b uint8) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 4), uint8(y * 5), b, 255})
		}
	}
	_ = r
	_ = g
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create jpeg: %v", err)
	}
	defer f.Close()
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
}

// newTestApp boots a real SQLite store (migrations + seed) under a temp
// resource dir and returns the app plus a cleanup.
func newTestApp(t *testing.T) *App {
	t.Helper()
	resDir := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := OpenDB(dbPath, resDir)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	cfg := LoadConfig()
	cfg.ResourceDir = resDir
	t.Cleanup(func() { _ = store.Close() })
	return NewApp(cfg, store)
}

func TestRunScanIngestsAndDedups(t *testing.T) {
	app := newTestApp(t)

	// scan root with two distinct JPEGs
	scanDir := t.TempDir()
	writeTestJPEG(t, filepath.Join(scanDir, "a.jpg"), 64, 48, 200, 100, 50)
	writeTestJPEG(t, filepath.Join(scanDir, "b.jpg"), 32, 24, 10, 20, 200)
	// a non-media file that must be ignored
	if err := os.WriteFile(filepath.Join(scanDir, "notes.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	// grab the seeded admin as owner
	var admin User
	if err := app.store.DB.First(&admin).Error; err != nil {
		t.Fatalf("no admin: %v", err)
	}

	lib := Library{
		ID:          newUUID(),
		OwnerID:     admin.ID,
		Name:        "ScanTest",
		Type:        "EXTERNAL",
		ImportPaths: scanDir,
		Status:      "inactive",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if err := app.store.DB.Create(&lib).Error; err != nil {
		t.Fatalf("create library: %v", err)
	}

	imported, skipped, err := app.runScan(admin.ID, &lib)
	if err != nil {
		t.Fatalf("runScan: %v", err)
	}
	if imported != 2 {
		t.Fatalf("expected 2 imported, got %d (skipped=%d)", imported, skipped)
	}
	if skipped != 0 {
		t.Fatalf("expected 0 skipped on first scan, got %d", skipped)
	}

	// assets persisted with thumbnails + exif
	var assets []Asset
	app.store.DB.Where("owner_id = ? AND library_id = ?", admin.ID, lib.ID).Find(&assets)
	if len(assets) != 2 {
		t.Fatalf("expected 2 asset rows, got %d", len(assets))
	}
	for _, a := range assets {
		if a.Type != "IMAGE" {
			t.Errorf("asset %s type = %q, want IMAGE", a.ID, a.Type)
		}
		if !a.HasThumbnail || a.ResizePath == "" {
			t.Errorf("asset %s missing thumbnail", a.ID)
		}
		if a.IsExternal != true {
			t.Errorf("asset %s IsExternal = false, want true for EXTERNAL lib", a.ID)
		}
		if _, statErr := os.Stat(a.ResizePath); statErr != nil {
			t.Errorf("thumbnail file missing: %v", statErr)
		}
	}

	// second scan must dedup everything
	imported2, skipped2, err := app.runScan(admin.ID, &lib)
	if err != nil {
		t.Fatalf("runScan 2: %v", err)
	}
	if imported2 != 0 {
		t.Fatalf("expected 0 imported on re-scan, got %d", imported2)
	}
	if skipped2 != 2 {
		t.Fatalf("expected 2 skipped on re-scan, got %d", skipped2)
	}

	// excluded path: add a subdir and exclude it, then re-scan freshly with a
	// new owner so nothing is deduped away.
	sub := filepath.Join(scanDir, "ignoreme")
	_ = os.MkdirAll(sub, 0o755)
	writeTestJPEG(t, filepath.Join(sub, "c.jpg"), 48, 36, 90, 90, 90)
	lib2 := Library{
		ID:            newUUID(),
		OwnerID:       admin.ID,
		Name:          "ScanExcl",
		Type:          "EXTERNAL",
		ImportPaths:   scanDir,
		ExcludedPaths: sub,
		Status:        "inactive",
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	if err := app.store.DB.Create(&lib2).Error; err != nil {
		t.Fatal(err)
	}
	// remove previous assets so they don't skew the count, then confirm the
	// excluded file is skipped (not imported) while a.jpg/b.jpg are.
	app.store.DB.Where("library_id = ?", lib.ID).Delete(&Asset{})
	imp, skp, err := app.runScan(admin.ID, &lib2)
	if err != nil {
		t.Fatalf("runScan excluded: %v", err)
	}
	if imp != 2 {
		t.Fatalf("expected 2 imported with exclusion, got %d (skipped=%d)", imp, skp)
	}
}
