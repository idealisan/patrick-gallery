package app

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
)

// runTrashCleanup permanently deletes assets that have been in the trash for
// longer than cfg.TrashDays. It removes the on-disk original / thumbnail /
// encoded-video files and the orphaned Exif row for each, then the asset row
// itself. It returns the number of assets purged. This is the Go port's
// equivalent of Immich's trash-expiry cron (userDeleteCheck).
func (a *App) runTrashCleanup() (int, error) {
	cutoff := time.Now().UTC().AddDate(0, 0, -a.cfg.TrashDays)

	var trashed []Asset
	if err := a.store.DB.Where("is_trash = ?", true).Find(&trashed).Error; err != nil {
		return 0, err
	}

	deleted := 0
	for i := range trashed {
		asset := trashed[i]
		ts := asset.TrashedAt
		if ts == nil {
			// Assets trashed before TrashedAt tracking existed fall back to
			// their last update time.
			t := asset.UpdatedAt
			ts = &t
		}
		if !ts.Before(cutoff) {
			continue
		}
		for _, p := range []string{asset.OriginalPath, asset.ResizePath, asset.EncodedVideoPath} {
			if p != "" {
				_ = os.Remove(p)
			}
		}
		_ = a.store.DB.Where("asset_id = ?", asset.ID).Delete(&Exif{})
		if err := a.store.DB.Delete(&asset).Error; err != nil {
			log.Printf("[trash] failed to delete asset %s: %v", asset.ID, err)
			continue
		}
		deleted++
	}
	if deleted > 0 {
		log.Printf("[trash] cleaned up %d expired asset(s) (older than %d days)", deleted, a.cfg.TrashDays)
	}
	return deleted, nil
}

// handleTrashCleanup mirrors the admin/periodic trash-expiry action and lets a
// client force it on demand: POST /api/trash/cleanup.
func (a *App) handleTrashCleanup(c *gin.Context) {
	n, err := a.runTrashCleanup()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error(), "statusCode": 500})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": n})
}

// startSchedulers launches background periodic tasks. Today this is only the
// trash-expiry cleanup, run once shortly after boot and then every 24h. The
// scheduler is best-effort: failures are logged and never stop the server.
func (a *App) startSchedulers() {
	stopTrash := make(chan struct{})
	a.trashStop = stopTrash
	go func() {
		// First run a little after startup so it doesn't compete with boot.
		select {
		case <-time.After(30 * time.Second):
		case <-stopTrash:
			return
		}
		if n, err := a.runTrashCleanup(); err != nil {
			log.Printf("[trash] scheduler run failed: %v", err)
		} else if n > 0 {
			log.Printf("[trash] scheduler purged %d asset(s)", n)
		}
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-stopTrash:
				return
			case <-ticker.C:
			}
			if n, err := a.runTrashCleanup(); err != nil {
				log.Printf("[trash] scheduler run failed: %v", err)
			} else if n > 0 {
				log.Printf("[trash] scheduler purged %d asset(s)", n)
			}
		}
	}()
}

// stopTrashScheduler stops the periodic trash cleanup loop (used by tests so
// the 30s first-run timer doesn't hold the test process open).
func (a *App) stopTrashScheduler() {
	if a.trashStop != nil {
		close(a.trashStop)
		a.trashStop = nil
	}
}
