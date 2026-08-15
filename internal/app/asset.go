package app

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"immich-go/internal/video"
)

// AssetResponse is the immich-go representation of Immich's AssetResponseDto.
// Field names/types follow the current Immich OpenAPI spec so the official
// mobile/web clients parse it without error. It is intentionally a standalone
// DTO (not an embedding of the DB model) so response shape is decoupled from
// storage.
type AssetResponse struct {
	ID               string    `json:"id"`
	OwnerID          string    `json:"ownerId"`
	Type             string    `json:"type"`
	OriginalPath     string    `json:"originalPath"`
	OriginalFileName string    `json:"originalFileName"`
	OriginalMimeType string    `json:"originalMimeType,omitempty"`
	ResizePath       string    `json:"resizePath"`
	EncodedVideoPath string    `json:"encodedVideoPath"`
	Checksum         string    `json:"checksum"`
	FileCreatedAt    time.Time `json:"fileCreatedAt"`
	FileModifiedAt   time.Time `json:"fileModifiedAt"`
	LocalDateTime    time.Time `json:"localDateTime"`
	Duration         int       `json:"duration"`
	IsFavorite       bool      `json:"isFavorite"`
	IsArchived       bool      `json:"isArchived"`
	IsTrashed        bool      `json:"isTrashed"`
	IsExternal       bool      `json:"isExternal"`
	LibraryId        string    `json:"libraryId"`
	HasThumbnail     bool      `json:"hasThumbnail"`
	Resized          bool      `json:"resized"`
	HasMetadata      bool      `json:"hasMetadata"`
	Visibility       string    `json:"visibility,omitempty"`
	Thumbhash        string    `json:"thumbhash,omitempty"`
	Width            int       `json:"width,omitempty"`
	Height           int       `json:"height,omitempty"`
	DuplicateID      string    `json:"duplicateId,omitempty"`
	IsEdited         bool      `json:"isEdited"`
	IsOffline        bool      `json:"isOffline"`
	LivePhotoVideoID string    `json:"livePhotoVideoId,omitempty"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
	ExifInfo         *Exif        `json:"exifInfo,omitempty"`
	People           []any        `json:"people"`
	Tags             []any        `json:"tags"`
	Owner            *UserResponse `json:"owner,omitempty"`
}

// UserResponse mirrors Immich's UserResponseDto (subset) used by the asset
// owner field. It is a standalone DTO (not the GORM User model) so response
// shape is decoupled from storage and no extra DB columns are required.
type UserResponse struct {
	ID               string `json:"id"`
	Email            string `json:"email"`
	Name             string `json:"name"`
	AvatarColor      string `json:"avatarColor"`
	ProfileChangedAt string `json:"profileChangedAt,omitempty"`
	ProfileImagePath string `json:"profileImagePath,omitempty"`
}

func (a *App) toResponse(asset Asset) AssetResponse {
	r := AssetResponse{
		ID:               asset.ID,
		OwnerID:          asset.OwnerID,
		Type:             asset.Type,
		OriginalPath:     asset.OriginalPath,
		OriginalFileName: asset.OriginalFileName,
		ResizePath:       asset.ResizePath,
		EncodedVideoPath: asset.EncodedVideoPath,
		Checksum:         asset.Checksum,
		FileCreatedAt:    asset.FileCreatedAt,
		FileModifiedAt:   asset.FileModifiedAt,
		LocalDateTime:    asset.LocalDateTime,
		Duration:         parseDurationInt(asset.Duration),
		IsFavorite:       asset.IsFavorite,
		IsArchived:       asset.IsArchived,
		IsTrashed:        asset.IsTrash,
		IsExternal:       asset.IsExternal,
		LibraryId:        asset.LibraryId,
		HasThumbnail:     asset.HasThumbnail,
		Resized:          asset.HasThumbnail,
		HasMetadata:      asset.ExifID != "",
		Visibility:       visibilityOf(asset.IsArchived),
		IsEdited:         false,
		IsOffline:        false,
		LivePhotoVideoID: asset.LivePhotoVideoID,
		Width:            asset.Width,
		Height:           asset.Height,
		Thumbhash:        asset.Thumbhash,
		CreatedAt:        asset.CreatedAt,
		UpdatedAt:        asset.UpdatedAt,
		People:           []any{},
		Tags:             []any{},
		OriginalMimeType: mimeByExt(asset.OriginalFileName),
	}
	if asset.OwnerID != "" {
		var owner User
		if a.store.DB.First(&owner, "id = ?", asset.OwnerID).Error == nil {
			r.Owner = &UserResponse{
				ID:               owner.ID,
				Email:            owner.Email,
				Name:             owner.Name,
				AvatarColor:      owner.AvatarColor,
				ProfileChangedAt: owner.UpdatedAt.UTC().Format(time.RFC3339),
				ProfileImagePath: "",
			}
		}
	}
	if asset.ExifID != "" {
		var exif Exif
		if a.store.DB.First(&exif, "id = ?", asset.ExifID).Error == nil {
			r.ExifInfo = &exif
		}
	}
	return r
}

// visibilityOf maps the boolean archive flag to Immich's AssetVisibility enum.
func visibilityOf(isArchived bool) string {
	if isArchived {
		return "archive"
	}
	return "timeline"
}

// parseDurationInt converts the stored duration (seconds, possibly empty or a
// non-numeric legacy value) into an integer the client expects.
func parseDurationInt(s string) int {
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}

// mimeByExt derives a MIME type from a filename's extension.
func mimeByExt(name string) string {
	if t := mime.TypeByExtension(filepath.Ext(name)); t != "" {
		return t
	}
	return ""
}

func (a *App) handleAssetUpload(c *gin.Context) {
	uid := currentUserID(c)

	// --- current Immich contract: individual multipart form fields ---
	filename := c.PostForm("filename")
	fileCreatedAt := c.PostForm("fileCreatedAt")
	fileModifiedAt := c.PostForm("fileModifiedAt")
	isFavorite := c.PostForm("isFavorite") == "true"
	visibility := c.PostForm("visibility")
	livePhotoVideoID := c.PostForm("livePhotoVideoId")
	durationSec := parseDurationInt(c.PostForm("duration"))
	deviceAssetId := c.PostForm("deviceAssetId")
	deviceId := c.PostForm("deviceId")

	// back-compat: older Immich clients send a single JSON `asset` field.
	var legacy struct {
		DeviceAssetId  string `json:"deviceAssetId"`
		DeviceId       string `json:"deviceId"`
		FileCreatedAt  string `json:"fileCreatedAt"`
		FileModifiedAt string `json:"fileModifiedAt"`
		LocalDateTime  string `json:"localDateTime"`
		FileExtension  string `json:"fileExtension"`
		IsFavorite     bool   `json:"isFavorite"`
		IsArchived     bool   `json:"isArchived"`
		Duration       string `json:"duration"`
		Type           string `json:"type"`
		Checksum       string `json:"checksum"`
	}
	_ = decodeJSONString(c.PostForm("asset"), &legacy)
	if deviceAssetId == "" {
		deviceAssetId = legacy.DeviceAssetId
	}
	if deviceId == "" {
		deviceId = legacy.DeviceId
	}
	if fileCreatedAt == "" {
		fileCreatedAt = legacy.FileCreatedAt
	}
	if fileModifiedAt == "" {
		fileModifiedAt = legacy.FileModifiedAt
	}
	if !isFavorite && legacy.IsFavorite {
		isFavorite = true
	}
	if durationSec == 0 {
		durationSec = parseDurationInt(legacy.Duration)
	}
	if visibility == "" && legacy.IsArchived {
		visibility = "archive"
	}

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

	// The current Immich client does NOT send assetType; infer it from the
	// file (extension, then magic bytes).
	ext := strings.ToLower(filepath.Ext(fh.Filename))
	if ext == "" {
		ext = "." + strings.TrimPrefix(strings.ToLower(legacy.FileExtension), ".")
	}
	assetType := sniffType(raw, ext)

	if filename == "" {
		filename = fh.Filename
	}

	// default upload library for the user
	var lib Library
	if err := a.store.DB.Where("owner_id = ?", uid).First(&lib).Error; err != nil {
		lib = Library{ID: "", OwnerID: uid}
	}

	now := time.Now().UTC()
	localDateTime := parseTime(fileCreatedAt, now)

	// Persist the original, then run it through the shared ingest path
	// (thumbnail + EXIF + asset rows), identical to a disk scan.
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
		FileName:     filename,
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

	// Patch client-supplied flags onto the freshly created asset. We do this
	// after ingest (rather than extending ingestOptions) so the shared
	// library-scan path is unaffected. GORM skips zero values, so false/""
	// simply keep the defaults.
	patch := Asset{
		DeviceAssetId:    deviceAssetId,
		DeviceId:         deviceId,
		IsFavorite:       isFavorite,
		IsArchived:       visibility == "archive",
		Duration:         strconv.Itoa(durationSec),
		LivePhotoVideoID: livePhotoVideoID,
	}
	if deviceAssetId != "" || deviceId != "" || isFavorite || visibility == "archive" || durationSec != 0 || livePhotoVideoID != "" {
		a.store.DB.Model(&asset).Updates(patch)
	}

	a.emitAsset("asset.create", asset.ID)
	c.JSON(http.StatusCreated, gin.H{"id": asset.ID, "status": "created"})
}

// sniffType resolves an asset type from the extension, falling back to magic
// bytes for cases where the extension is missing/unknown.
func sniffType(raw []byte, ext string) string {
	if t := extToType(ext); t != "" {
		return t
	}
	if len(raw) >= 12 {
		if bytes.Equal(raw[4:8], []byte("ftyp")) { // mp4 / mov / m4v
			return "VIDEO"
		}
		if bytes.HasPrefix(raw, []byte("RIFF")) && bytes.Contains(raw[:12], []byte("AVI ")) {
			return "VIDEO"
		}
		if bytes.HasPrefix(raw, []byte("\x1A\x45\xDF\xA3")) { // webm / mkv (EBML)
			return "VIDEO"
		}
	}
	return "IMAGE"
}

// handleAssetBulkUploadCheck implements POST /api/assets/bulk-upload-check,
// the current Immich deduplication handshake. Clients send a list of
// {checksum, id} and receive, per item, whether the server already holds the
// asset (action "reject"/reason "duplicate") or wants it uploaded
// (action "accept"). Matching is by content checksum over the user's assets.
func (a *App) handleAssetBulkUploadCheck(c *gin.Context) {
	uid := currentUserID(c)
	var b struct {
		Assets []struct {
			Checksum string `json:"checksum"`
			ID       string `json:"id"`
		} `json:"assets"`
	}
	_ = c.ShouldBindJSON(&b)
	results := make([]gin.H, 0, len(b.Assets))
	for _, item := range b.Assets {
		res := gin.H{"id": item.ID}
		if item.Checksum == "" {
			res["action"] = "accept"
			results = append(results, res)
			continue
		}
		var existing Asset
		err := a.store.DB.Where("owner_id = ? AND checksum = ? AND is_trash = ?", uid, item.Checksum, false).
			First(&existing).Error
		if err == nil {
			res["action"] = "reject"
			res["reason"] = "duplicate"
			res["assetId"] = existing.ID
			res["isTrashed"] = false
		} else {
			res["action"] = "accept"
		}
		results = append(results, res)
	}
	c.JSON(http.StatusOK, gin.H{"results": results})
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
		if *b.IsTrash {
			now := time.Now().UTC()
			asset.TrashedAt = &now
		}
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
	a.emitAsset("asset.update", asset.ID)
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
			now := time.Now().UTC()
			asset.TrashedAt = &now
			a.store.DB.Save(&asset)
		}
	}
	if len(b.IDs) > 0 {
		if b.Force {
			a.emit("asset.delete", map[string]any{"ids": b.IDs})
		} else {
			a.emit("asset.trash", map[string]any{"ids": b.IDs})
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
			if *b.IsTrash {
				now := time.Now().UTC()
				asset.TrashedAt = &now
			}
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
	if len(b.IDs) > 0 {
		if b.IsTrash != nil && *b.IsTrash {
			a.emit("asset.trash", map[string]any{"ids": b.IDs})
		} else {
			a.emit("asset.update", map[string]any{"ids": b.IDs})
		}
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

// handleAssetLivePhoto streams the motion (video) component of a Live Photo.
// A Live Photo is stored as two assets: a still IMAGE and a paired VIDEO whose
// id is recorded on the image's LivePhotoVideoID. The official clients render
// the still and play this endpoint's bytes on long-press / motion. We prefer
// the transcoded encoded-video when present, else the original.
func (a *App) handleAssetLivePhoto(c *gin.Context) {
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
	if asset.LivePhotoVideoID == "" {
		c.Status(http.StatusNotFound)
		return
	}
	var video Asset
	if err := a.store.DB.First(&video, "id = ?", asset.LivePhotoVideoID).Error; err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	if video.OwnerID != uid && !a.isAdmin(uid) {
		c.Status(http.StatusForbidden)
		return
	}
	path := video.EncodedVideoPath
	if path == "" {
		path = video.OriginalPath
	}
	if path == "" {
		c.Status(http.StatusNotFound)
		return
	}
	if ct := mimeByExt(path); ct != "" {
		c.Header("Content-Type", ct)
	} else {
		c.Header("Content-Type", "video/mp4")
	}
	c.File(path)
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
