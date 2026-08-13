package app

import (
	"crypto/sha1"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func (a *App) handleLibraryList(c *gin.Context) {
	uid := currentUserID(c)
	var libs []Library
	a.store.DB.Where("owner_id = ?", uid).Find(&libs)
	c.JSON(http.StatusOK, libs)
}

type libraryBody struct {
	Name          string `json:"name"`
	Type          string `json:"type"`
	ImportPaths   string `json:"importPaths"`
	ExcludedPaths string `json:"excludedPaths"`
}

func (a *App) handleLibraryCreate(c *gin.Context) {
	uid := currentUserID(c)
	var b libraryBody
	_ = c.ShouldBindJSON(&b)
	if b.Name == "" {
		b.Name = "New Library"
	}
	if b.Type == "" {
		b.Type = "UPLOAD"
	}
	now := time.Now().UTC()
	lib := Library{
		ID:            newUUID(),
		OwnerID:       uid,
		Name:          b.Name,
		Type:          b.Type,
		ImportPaths:   b.ImportPaths,
		ExcludedPaths: b.ExcludedPaths,
		Watched:       false,
		Status:        "inactive",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	a.store.DB.Create(&lib)
	c.JSON(http.StatusCreated, lib)
}

func (a *App) handleLibraryGet(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	var lib Library
	if err := a.store.DB.First(&lib, "id = ? AND owner_id = ?", id, uid).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, lib)
}

func (a *App) handleLibraryUpdate(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	var lib Library
	if err := a.store.DB.First(&lib, "id = ? AND owner_id = ?", id, uid).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	var b libraryBody
	_ = c.ShouldBindJSON(&b)
	if b.Name != "" {
		lib.Name = b.Name
	}
	if b.ImportPaths != "" {
		lib.ImportPaths = b.ImportPaths
	}
	if b.ExcludedPaths != "" {
		lib.ExcludedPaths = b.ExcludedPaths
	}
	lib.UpdatedAt = time.Now().UTC()
	a.store.DB.Save(&lib)
	c.JSON(http.StatusOK, lib)
}

func (a *App) handleLibraryDelete(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	a.store.DB.Where("id = ? AND owner_id = ?", id, uid).Delete(&Library{})
	c.Status(http.StatusOK)
}

func (a *App) handleLibraryStats(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	var total, photos, videos int64
	a.store.DB.Model(&Asset{}).Where("owner_id = ? AND library_id = ? AND is_trash = ?", uid, id, false).Count(&total)
	a.store.DB.Model(&Asset{}).Where("owner_id = ? AND library_id = ? AND is_trash = ? AND type = ?", uid, id, false, "IMAGE").Count(&photos)
	a.store.DB.Model(&Asset{}).Where("owner_id = ? AND library_id = ? AND is_trash = ? AND type = ?", uid, id, false, "VIDEO").Count(&videos)
	c.JSON(http.StatusOK, gin.H{
		"total":  total,
		"photos": photos,
		"videos": videos,
		"usage":  gin.H{"total": total, "photos": photos, "videos": videos},
	})
}

func splitPaths(s string) []string {
	var out []string
	for _, p := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '\n' || r == ';' }) {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// handleLibraryScan crawls the library's ImportPaths on disk and ingests any
// media files not already present (dedup by checksum for the owner). This is
// the real ingestion path for disk-backed libraries; it reuses ingestStoredFile
// so thumbnails/EXIF are generated identically to uploads.
func (a *App) handleLibraryScan(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	var lib Library
	if err := a.store.DB.First(&lib, "id = ? AND owner_id = ?", id, uid).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if lib.ImportPaths == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "library has no import paths"})
		return
	}

	lib.Status = "scanning"
	lib.UpdatedAt = time.Now().UTC()
	a.store.DB.Save(&lib)

	imported, skipped, scanErr := a.runScan(uid, &lib)

	lib.Status = "active"
	lib.UpdatedAt = time.Now().UTC()
	a.store.DB.Save(&lib)

	if scanErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": scanErr.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":   "completed",
		"imported": imported,
		"skipped":  skipped,
		"library":  lib.Name,
	})
}

// runScan walks the library's ImportPaths and ingests every media file not
// already present for the owner (dedup by content checksum). It is the
// filesystem-crawl core used by handleLibraryScan and is exposed separately so
// it can be exercised without an HTTP context.
func (a *App) runScan(uid string, lib *Library) (imported, skipped int, err error) {
	excluded := splitPaths(lib.ExcludedPaths)
	isExternal := lib.Type == "EXTERNAL"

	for _, root := range splitPaths(lib.ImportPaths) {
		root = strings.TrimRight(root, "/")
		werr := filepath.Walk(root, func(path string, info os.FileInfo, e error) error {
			if e != nil || info == nil || info.IsDir() {
				return nil
			}
			// skip excluded prefixes
			for _, ex := range excluded {
				if ex != "" && strings.HasPrefix(path, strings.TrimRight(ex, "/")+"/") {
					return nil
				}
			}
			ext := strings.ToLower(filepath.Ext(path))
			assetType := extToType(ext)
			if assetType == "" {
				return nil
			}
			// dedup by content checksum for this owner
			raw, rerr := os.ReadFile(path)
			if rerr != nil {
				skipped++
				return nil
			}
			sum := sha1.Sum(raw)
			checksum := hex.EncodeToString(sum[:])
			var n int64
			a.store.DB.Model(&Asset{}).Where("owner_id = ? AND checksum = ?", uid, checksum).Count(&n)
			if n > 0 {
				skipped++
				return nil
			}
			mod := info.ModTime()
			if _, serr := a.ingestStoredFile(ingestOptions{
				OwnerID:      uid,
				LibraryID:    lib.ID,
				FileName:     info.Name(),
				Ext:          ext,
				Type:         assetType,
				OriginalPath: path,
				IsExternal:   isExternal,
				Checksum:     checksum,
				FileCreated:  mod,
				FileModified: mod,
				LocalDate:    mod,
			}); serr != nil {
				skipped++
				return nil
			}
			imported++
			return nil
		})
		if werr != nil {
			return imported, skipped, werr
		}
	}
	return imported, skipped, nil
}
