package app

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// repair.go — S5: startup consistency self-healing (Epic WI-3).
//
// repairSteps is an ordered, idempotent list of data-repair passes executed at
// app start. Each step is independent: a panic in one does not block the rest.
// Steps log `[repair] step=<name> scanned=N fixed=M`.

type repairStep struct {
	Name string
	Fn   func(a *App) (scanned, fixed int64, err error)
}

// repairSteps returns the ordered registry. Append new passes at the end
// unless ordering matters (later steps may rely on earlier fixes).
func repairSteps() []repairStep {
	return []repairStep{
		{"livePhotoDanglingRef", (*App).repairLivePhotoDanglingRef},
		{"motionVideoVisibility", (*App).repairMotionVideoVisibility},
		{"archiveVisibilitySync", (*App).repairArchiveVisibilitySync},
		{"thumbnailStateSync", (*App).repairThumbnailStateSync},
		{"orphanPreviewCache", (*App).repairOrphanPreviewCache},
		{"orphanThumbnailFiles", (*App).repairOrphanThumbnailFiles},
		{"albumAssetDanglingRows", (*App).repairAlbumAssetDanglingRows},
	}
}

// runRepairPass executes every step; failures are logged and skipped.
func (a *App) runRepairPass() {
	for _, step := range repairSteps() {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[repair] step=%s PANIC: %v", step.Name, r)
				}
			}()
			scanned, fixed, err := step.Fn(a)
			if err != nil {
				log.Printf("[repair] step=%s error: %v", step.Name, err)
				return
			}
			if fixed > 0 {
				log.Printf("[repair] step=%s scanned=%d fixed=%d", step.Name, scanned, fixed)
			}
		}()
	}
}

// ---- Step 1: live_photo_video_id pointing at deleted/missing assets ----

func (a *App) repairLivePhotoDanglingRef() (int64, int64, error) {
	res := a.store.DB.Exec("UPDATE assets SET live_photo_video_id = '', updated_at = ? "+
		"WHERE live_photo_video_id != '' "+
		"AND NOT EXISTS (SELECT 1 FROM assets v WHERE v.id = assets.live_photo_video_id)",
		time.Now().UTC())
	return res.RowsAffected, res.RowsAffected, res.Error
}

// ---- Step 2: motion-part videos must be hidden ----

func (a *App) repairMotionVideoVisibility() (int64, int64, error) {
	res := a.store.DB.Exec("UPDATE assets SET visibility = 'hidden', is_archived = 0, updated_at = ? "+
		"WHERE (visibility IS NULL OR visibility = '' OR visibility = 'timeline') "+
		"AND id IN (SELECT live_photo_video_id FROM assets WHERE live_photo_video_id != '')",
		time.Now().UTC())
	return res.RowsAffected, res.RowsAffected, res.Error
}

// ---- Step 3: archive boolean must follow canonical visibility ----

func (a *App) repairArchiveVisibilitySync() (int64, int64, error) {
	var fixed int64
	res := a.store.DB.Exec("UPDATE assets SET is_archived = 1, updated_at = ? "+
		"WHERE visibility = 'archive' AND is_archived = 0", time.Now().UTC())
	fixed += res.RowsAffected
	res2 := a.store.DB.Exec("UPDATE assets SET is_archived = 0, updated_at = ? "+
		"WHERE (visibility IS NULL OR visibility = '' OR visibility IN ('timeline','hidden')) AND is_archived = 1",
		time.Now().UTC())
	fixed += res2.RowsAffected
	return fixed, fixed, nil
}

// ---- Step 4: has_thumbnail flag vs resize_path file reality ----

func (a *App) repairThumbnailStateSync() (int64, int64, error) {
	var assets []Asset
	if err := a.store.DB.Where("type = ? AND is_trash = ?", "IMAGE", false).
		Find(&assets).Error; err != nil {
		return 0, 0, err
	}
	var fixed int64
	for _, as := range assets {
		fileExists := false
		if as.ResizePath != "" {
			if _, err := os.Stat(as.ResizePath); err == nil {
				fileExists = true
			}
		}
		switch {
		case as.HasThumbnail && !fileExists:
			a.store.DB.Model(&Asset{}).Where("id = ?", as.ID).Updates(map[string]any{
				"has_thumbnail": false, "resize_path": "", "updated_at": time.Now().UTC(),
			})
			fixed++
		case !as.HasThumbnail && fileExists:
			a.store.DB.Model(&Asset{}).Where("id = ?", as.ID).Updates(map[string]any{
				"has_thumbnail": true, "updated_at": time.Now().UTC(),
			})
			fixed++
		}
	}
	return int64(len(assets)), fixed, nil
}

// ---- Step 5: orphan preview caches (.preview.jpg with no asset row) ----

func (a *App) repairOrphanPreviewCache() (int64, int64, error) {
	dir := filepath.Join(a.cfg.ResourceDir, "upload")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0, nil // dir may not exist; not an error for repair purposes
	}
	valid := map[string]bool{}
	rows, err := a.store.DB.Raw("SELECT id FROM assets").Rows()
	if err != nil {
		return 0, 0, err
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			valid[id] = true
		}
	}
	rows.Close()

	var scanned, removed int64
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".preview.jpg") {
			continue
		}
		scanned++
		// name format: <uuid>.<assetID>.preview.jpg — extract assetID.
		base := strings.TrimSuffix(name, ".preview.jpg")
		parts := strings.SplitN(base, ".", 2)
		if len(parts) != 2 || valid[parts[1]] {
			continue
		}
		if err := os.Remove(filepath.Join(dir, name)); err == nil {
			removed++
		} else {
			log.Printf("[repair] orphanPreviewCache remove %s: %v", name, err)
		}
	}
	return scanned, removed, nil
}

// ---- Step 6: orphan thumbnail files (id not in assets) ----

func (a *App) repairOrphanThumbnailFiles() (int64, int64, error) {
	dir := filepath.Join(a.cfg.ResourceDir, "thumbnail")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0, nil
	}
	valid := map[string]bool{}
	rows, err := a.store.DB.Raw("SELECT id FROM assets").Rows()
	if err != nil {
		return 0, 0, err
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			valid[id] = true
		}
	}
	rows.Close()

	var scanned, removed int64
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jpg") {
			continue
		}
		scanned++
		id := strings.TrimSuffix(e.Name(), ".jpg")
		if valid[id] {
			continue
		}
		if err := os.Remove(filepath.Join(dir, e.Name())); err == nil {
			removed++
		}
	}
	return scanned, removed, nil
}

// ---- Step 7: album membership rows pointing at missing assets/albums ----

func (a *App) repairAlbumAssetDanglingRows() (int64, int64, error) {
	res := a.store.DB.Exec("DELETE FROM albums_assets_assets "+
		"WHERE asset_id NOT IN (SELECT id FROM assets) "+
		"OR album_id NOT IN (SELECT id FROM albums)")
	return res.RowsAffected, res.RowsAffected, res.Error
}
