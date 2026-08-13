package app

import (
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"immich-go/internal/video"
)

// AssetResponse nests EXIF under the asset, matching Immich's JSON shape.
type AssetResponse struct {
	Asset
	Exif *Exif `json:"exif,omitempty"`
}

func (a *App) toResponse(asset Asset) AssetResponse {
	var exif Exif
	if asset.ExifID != "" {
		if err := a.store.DB.First(&exif, "id = ?", asset.ExifID).Error; err != nil {
			return AssetResponse{Asset: asset}
		}
		return AssetResponse{Asset: asset, Exif: &exif}
	}
	return AssetResponse{Asset: asset}
}

func (a *App) handleAssetUpload(c *gin.Context) {
	uid := currentUserID(c)
	metaRaw := c.Request.FormValue("asset")
	var meta struct {
		DeviceAssetId string `json:"deviceAssetId"`
		DeviceId      string `json:"deviceId"`
		FileCreatedAt string `json:"fileCreatedAt"`
		FileModifiedAt string `json:"fileModifiedAt"`
		LocalDateTime string `json:"localDateTime"`
		FileExtension string `json:"fileExtension"`
		IsFavorite    bool   `json:"isFavorite"`
		IsArchived    bool   `json:"isArchived"`
		Duration      string `json:"duration"`
		Type          string `json:"type"`
		Checksum      string `json:"checksum"`
		IsExternal    bool   `json:"isExternal"`
	}
	_ = decodeJSONString(metaRaw, &meta)

	fh, err := c.FormFile("assetData")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing assetData", "statusCode": 400})
		return
	}
	src, err := fh.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	raw, err := io.ReadAll(src)
	src.Close()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// default library for the user
	var lib Library
	if err := a.store.DB.Where("owner_id = ?", uid).First(&lib).Error; err != nil {
		lib = Library{ID: "", OwnerID: uid}
	}

	now := time.Now().UTC()
	localDateTime := parseTime(meta.LocalDateTime, now)
	assetType := normalizeType(meta.Type)

	ext := strings.ToLower(filepath.Ext(fh.Filename))
	if ext == "" {
		ext = "." + strings.TrimPrefix(strings.ToLower(meta.FileExtension), ".")
	}
	if assetType == "" {
		assetType = extToType(ext)
	}

	// Persist the original to the upload dir, then run it through the shared
	// ingest path (thumbnail + EXIF + asset rows), identical to a disk scan.
	upDir := filepath.Join(a.cfg.ResourceDir, "upload")
	_ = os.MkdirAll(upDir, 0o755)
	origPath := filepath.Join(upDir, newUUID()+ext)
	if werr := os.WriteFile(origPath, raw, 0o644); werr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": werr.Error()})
		return
	}

	asset, err := a.ingestStoredFile(ingestOptions{
		OwnerID:      uid,
		LibraryID:    lib.ID,
		FileName:     fh.Filename,
		Ext:          ext,
		Type:         assetType,
		OriginalPath: origPath,
		IsExternal:   false,
		FileCreated:  now,
		FileModified: now,
		LocalDate:    localDateTime,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": asset.ID, "status": "created", "assetId": asset.ID})
}

func normalizeType(t string) string {
	switch strings.ToLower(t) {
	case "image":
		return "IMAGE"
	case "video":
		return "VIDEO"
	}
	if strings.EqualFold(t, "IMAGE") {
		return "IMAGE"
	}
	if strings.EqualFold(t, "VIDEO") {
		return "VIDEO"
	}
	return strings.ToUpper(t)
}

func parseTime(s string, def time.Time) time.Time {
	if s == "" {
		return def
	}
	for _, l := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000Z"} {
		if t, err := time.Parse(l, s); err == nil {
			return t
		}
	}
	return def
}

func (a *App) handleAssetGet(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	var asset Asset
	if err := a.store.DB.First(&asset, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found", "statusCode": 404})
		return
	}
	if asset.OwnerID != uid && !a.isAdmin(uid) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	c.JSON(http.StatusOK, a.toResponse(asset))
}

type assetUpdateBody struct {
	IsFavorite *bool  `json:"isFavorite"`
	IsArchived *bool  `json:"isArchived"`
	IsTrash    *bool  `json:"isTrash"`
	DateTimeOriginal string `json:"dateTimeOriginal,omitempty"`
	Latitude   *float64 `json:"latitude,omitempty"`
	Longitude  *float64 `json:"longitude,omitempty"`
}

func (a *App) handleAssetUpdate(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	var asset Asset
	if err := a.store.DB.First(&asset, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if asset.OwnerID != uid && !a.isAdmin(uid) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	var b assetUpdateBody
	_ = c.ShouldBindJSON(&b)
	if b.IsFavorite != nil {
		asset.IsFavorite = *b.IsFavorite
	}
	if b.IsArchived != nil {
		asset.IsArchived = *b.IsArchived
	}
	if b.IsTrash != nil {
		asset.IsTrash = *b.IsTrash
	}
	if b.DateTimeOriginal != "" || b.Latitude != nil || b.Longitude != nil {
		var exif Exif
		if err := a.store.DB.First(&exif, "id = ?", asset.ExifID).Error; err == nil {
			if b.DateTimeOriginal != "" {
				v := b.DateTimeOriginal
				exif.DateTimeOriginal = &v
			}
			if b.Latitude != nil {
				exif.Latitude = *b.Latitude
			}
			if b.Longitude != nil {
				exif.Longitude = *b.Longitude
			}
			a.store.DB.Save(&exif)
		}
	}
	asset.UpdatedAt = time.Now().UTC()
	a.store.DB.Save(&asset)
	c.JSON(http.StatusOK, a.toResponse(asset))
}

func (a *App) handleAssetBulkDelete(c *gin.Context) {
	uid := currentUserID(c)
	var b struct {
		IDs    []string `json:"ids"`
		Force  bool     `json:"force"`
	}
	_ = c.ShouldBindJSON(&b)
	for _, id := range b.IDs {
		var asset Asset
		if err := a.store.DB.First(&asset, "id = ?", id).Error; err != nil {
			continue
		}
		if asset.OwnerID != uid && !a.isAdmin(uid) {
			continue
		}
		if b.Force {
			a.store.DB.Delete(&asset)
			_ = os.Remove(asset.OriginalPath)
			if asset.ResizePath != "" {
				_ = os.Remove(asset.ResizePath)
			}
		} else {
			asset.IsTrash = true
			a.store.DB.Save(&asset)
		}
	}
	c.Status(http.StatusOK)
}

// handleAssetCheck lets clients probe which device asset IDs already exist
// (used by the official apps before upload to skip duplicates).
func (a *App) handleAssetCheck(c *gin.Context) {
	uid := currentUserID(c)
	var b struct {
		DeviceId       string   `json:"deviceId"`
		DeviceAssetIds []string `json:"deviceAssetIds"`
	}
	_ = c.ShouldBindJSON(&b)
	res := map[string]string{}
	for _, da := range b.DeviceAssetIds {
		var n int64
		a.store.DB.Model(&Asset{}).Where("owner_id = ? AND device_asset_id = ?", uid, da).Count(&n)
		if n > 0 {
			res[da] = "duplicate"
		} else {
			res[da] = "new"
		}
	}
	c.JSON(http.StatusOK, res)
}

// handleAssetBulkUpdate applies favorite/archive/trash/exif edits to many
// assets at once.
func (a *App) handleAssetBulkUpdate(c *gin.Context) {
	uid := currentUserID(c)
	var b struct {
		IDs              []string  `json:"ids"`
		IsFavorite       *bool     `json:"isFavorite"`
		IsArchived       *bool     `json:"isArchived"`
		IsTrash          *bool     `json:"isTrash"`
		DateTimeOriginal string    `json:"dateTimeOriginal"`
		Latitude         *float64  `json:"latitude"`
		Longitude        *float64  `json:"longitude"`
	}
	_ = c.ShouldBindJSON(&b)
	for _, id := range b.IDs {
		var asset Asset
		if err := a.store.DB.First(&asset, "id = ?", id).Error; err != nil {
			continue
		}
		if asset.OwnerID != uid && !a.isAdmin(uid) {
			continue
		}
		if b.IsFavorite != nil {
			asset.IsFavorite = *b.IsFavorite
		}
		if b.IsArchived != nil {
			asset.IsArchived = *b.IsArchived
		}
		if b.IsTrash != nil {
			asset.IsTrash = *b.IsTrash
		}
		if b.DateTimeOriginal != "" || b.Latitude != nil || b.Longitude != nil {
			var exif Exif
			if err := a.store.DB.First(&exif, "id = ?", asset.ExifID).Error; err == nil {
				if b.DateTimeOriginal != "" {
					v := b.DateTimeOriginal
					exif.DateTimeOriginal = &v
				}
				if b.Latitude != nil {
					exif.Latitude = *b.Latitude
				}
				if b.Longitude != nil {
					exif.Longitude = *b.Longitude
				}
				a.store.DB.Save(&exif)
			}
		}
		asset.UpdatedAt = time.Now().UTC()
		a.store.DB.Save(&asset)
	}
	c.Status(http.StatusOK)
}

func (a *App) handleAssetOriginal(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	var asset Asset
	if err := a.store.DB.First(&asset, "id = ?", id).Error; err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	if asset.OwnerID != uid && !a.isAdmin(uid) {
		c.Status(http.StatusForbidden)
		return
	}
	c.File(asset.OriginalPath)
}

// handleAssetOriginalDownload serves the original bytes as a forced
// attachment download (Content-Disposition: attachment), matching Immich's
// /api/assets/:id/original/download endpoint used by the official apps to save
// a copy to the device.
func (a *App) handleAssetOriginalDownload(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	var asset Asset
	if err := a.store.DB.First(&asset, "id = ?", id).Error; err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	if asset.OwnerID != uid && !a.isAdmin(uid) {
		c.Status(http.StatusForbidden)
		return
	}
	name := asset.OriginalFileName
	if name == "" {
		name = id
	}
	c.Header("Content-Disposition", "attachment; filename=\""+name+"\"")
	c.File(asset.OriginalPath)
}

// handleAssetMetadata returns the EXIF metadata for a single asset, matching
// Immich's /api/assets/:id/metadata endpoint. Returns an empty object when no
// EXIF row is attached (rather than 404) so clients can render gracefully.
func (a *App) handleAssetMetadata(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	var asset Asset
	if err := a.store.DB.First(&asset, "id = ?", id).Error; err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	if asset.OwnerID != uid && !a.isAdmin(uid) {
		c.Status(http.StatusForbidden)
		return
	}
	var exif Exif
	if err := a.store.DB.First(&exif, "asset_id = ?", id).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"assetId": id})
		return
	}
	c.JSON(http.StatusOK, exif)
}

func (a *App) handleAssetThumbnail(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	var asset Asset
	if err := a.store.DB.First(&asset, "id = ?", id).Error; err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	if asset.OwnerID != uid && !a.isAdmin(uid) {
		c.Status(http.StatusForbidden)
		return
	}
	if asset.ResizePath == "" {
		c.Status(http.StatusNotFound)
		return
	}
	c.File(asset.ResizePath)
}

// handleAssetPreview serves a mid-size representation: the generated
// thumbnail/poster (ResizePath). For video this is the decoded poster frame;
// for images it is the scaled thumbnail. Falls back to original when no
// thumbnail exists (e.g. placeholder backend).
func (a *App) handleAssetPreview(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	var asset Asset
	if err := a.store.DB.First(&asset, "id = ?", id).Error; err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	if asset.OwnerID != uid && !a.isAdmin(uid) {
		c.Status(http.StatusForbidden)
		return
	}
	if asset.ResizePath != "" {
		c.File(asset.ResizePath)
		return
	}
	a.handleAssetOriginal(c)
}

func (a *App) handleAssetEncodedVideo(c *gin.Context) {
	// In-process, pure-Go transcoding via the video backend (FFmpeg shared
	// libs loaded through purego; no CLI). Falls back to the original file if
	// the backend cannot transcode (placeholder backend / unsupported input).
	uid := currentUserID(c)
	id := c.Param("id")
	var asset Asset
	if err := a.store.DB.First(&asset, "id = ?", id).Error; err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	if asset.OwnerID != uid && !a.isAdmin(uid) {
		c.Status(http.StatusForbidden)
		return
	}
	raw, err := os.ReadFile(asset.OriginalPath)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	out, err := a.video.Transcode(raw, video.TranscodeOptions{
		Format:     "mp4",
		VideoCodec: "h264",
		AudioCodec: "copy",
		Preset:     "software",
	})
	if err != nil || len(out) == 0 {
		c.File(asset.OriginalPath)
		return
	}
	c.Header("Content-Type", "video/mp4")
	c.Data(http.StatusOK, "video/mp4", out)
}

func (a *App) handleAssetRandom(c *gin.Context) {
	uid := currentUserID(c)
	var b struct {
		Count int `json:"count"`
	}
	_ = c.ShouldBindJSON(&b)
	if b.Count <= 0 {
		b.Count = 1
	}
	var assets []Asset
	a.store.DB.Where("owner_id = ? AND is_trash = ? AND type = ?", uid, false, "IMAGE").
		Order("RANDOM()").Limit(b.Count).Find(&assets)
	out := make([]AssetResponse, 0, len(assets))
	for _, ast := range assets {
		out = append(out, a.toResponse(ast))
	}
	c.JSON(http.StatusOK, out)
}

func (a *App) handleAssetCount(c *gin.Context) {
	uid := currentUserID(c)
	var photos, videos, total int64
	a.store.DB.Model(&Asset{}).Where("owner_id = ? AND is_trash = ?", uid, false).Count(&total)
	a.store.DB.Model(&Asset{}).Where("owner_id = ? AND is_trash = ? AND type = ?", uid, false, "IMAGE").Count(&photos)
	a.store.DB.Model(&Asset{}).Where("owner_id = ? AND is_trash = ? AND type = ?", uid, false, "VIDEO").Count(&videos)
	c.JSON(http.StatusOK, gin.H{"photos": photos, "videos": videos, "total": total, "usage": gin.H{"photos": photos, "videos": videos, "total": total}})
}

func (a *App) handleAssetSearch(c *gin.Context) {
	uid := currentUserID(c)
	// Build the WHERE clause as a string + args so we can create two fresh
	// *gorm.DB queries (reusing one after Count consumes its statement).
	conds := "owner_id = ? AND is_trash = ?"
	args := []interface{}{uid, false}
	if t := c.Query("type"); t != "" {
		conds += " AND type = ?"
		args = append(args, normalizeType(t))
	}
	if c.Query("isFavorite") == "true" {
		conds += " AND is_favorite = ?"
		args = append(args, true)
	}
	if c.Query("isArchived") == "true" {
		conds += " AND is_archived = ?"
		args = append(args, true)
	}
	if c.Query("isTrash") == "true" {
		conds += " AND is_trash = ?"
		args[len(args)-1] = true
	}
	var total int64
	a.store.DB.Model(&Asset{}).Where(conds, args...).Count(&total)
	take, skip := atoiDefault(c.Query("take"), 100), atoiDefault(c.Query("skip"), 0)
	var assets []Asset
	a.store.DB.Where(conds, args...).Order("local_date_time DESC").Offset(skip).Limit(take).Find(&assets)
	out := make([]AssetResponse, 0, len(assets))
	for _, ast := range assets {
		out = append(out, a.toResponse(ast))
	}
	c.JSON(http.StatusOK, gin.H{"assets": out, "count": len(out), "total": total})
}

func (a *App) handleAssetBulkInfo(c *gin.Context) {
	uid := currentUserID(c)
	var b struct {
		IDs []string `json:"ids"`
	}
	_ = c.ShouldBindJSON(&b)
	res := map[string]AssetResponse{}
	for _, id := range b.IDs {
		var asset Asset
		if err := a.store.DB.First(&asset, "id = ? AND owner_id = ?", id, uid).Error; err == nil {
			res[id] = a.toResponse(asset)
		}
	}
	c.JSON(http.StatusOK, res)
}

func (a *App) handleAssetDuplicates(c *gin.Context) {
	// Group by checksum; return assets that share a checksum with another.
	type dup struct {
		Checksum string
		Count    int
	}
	var dups []dup
	a.store.DB.Model(&Asset{}).Select("checksum, count(*) as count").
		Where("is_trash = ?", false).Group("checksum").Having("count > 1").Scan(&dups)
	out := []gin.H{}
	for _, d := range dups {
		var assets []Asset
		a.store.DB.Where("checksum = ? AND is_trash = ?", d.Checksum, false).Find(&assets)
		ids := make([]string, 0, len(assets))
		for _, a2 := range assets {
			ids = append(ids, a2.ID)
		}
		out = append(out, gin.H{"assets": ids})
	}
	c.JSON(http.StatusOK, out)
}

// ---- helpers ----

func (a *App) isAdmin(uid string) bool {
	var u User
	if err := a.store.DB.First(&u, "id = ?", uid).Error; err != nil {
		return false
	}
	return u.IsAdmin
}

// makeThumbnail decodes an image and writes a scaled JPEG (max 256px on the
// long edge) using only the standard library. Non-decodable files are skipped.
func makeThumbnail(src, dst string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		// try png explicitly
		f2, err2 := os.Open(src)
		if err2 != nil {
			return err
		}
		defer f2.Close()
		p, errp := png.Decode(f2)
		if errp != nil {
			return err
		}
		img = p
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return fmt.Errorf("empty image")
	}
	const maxEdge = 256
	long := w
	if h > long {
		long = h
	}
	if long <= maxEdge {
		// copy original content as jpg by re-encoding at same size
	}
	scale := float64(maxEdge) / float64(long)
	nw, nh := int(float64(w)*scale), int(float64(h)*scale)
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	dstImg := image.NewRGBA(image.Rect(0, 0, nw, nh))
	// simple box/nearest-neighbour downscale
	for y := 0; y < nh; y++ {
		sy := b.Min.Y + int(float64(y)/scale)
		if sy > b.Max.Y-1 {
			sy = b.Max.Y - 1
		}
		for x := 0; x < nw; x++ {
			sx := b.Min.X + int(float64(x)/scale)
			if sx > b.Max.X-1 {
				sx = b.Max.X - 1
			}
			dstImg.Set(x, y, img.At(sx, sy))
		}
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	return jpeg.Encode(out, dstImg, &jpeg.Options{Quality: 80})
}

// decodeJSONString parses a JSON string (e.g. a multipart form field) into v.
func decodeJSONString(s string, v interface{}) error {
	if s == "" {
		return nil
	}
	return jsonUnmarshal([]byte(s), v)
}

// ensure json helper import used

// atoiDefault parses an int with a fallback.
func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return def
		}
		n = n*10 + int(r-'0')
	}
	return n
}
