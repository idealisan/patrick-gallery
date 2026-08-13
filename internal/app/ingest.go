package app

import (
	"crypto/sha1"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	imgproc "immich-go/internal/image"
	"immich-go/internal/video"
)

// media extensions recognised during a filesystem scan / ingestion.
var imageExts = map[string]bool{
	"jpg": true, "jpeg": true, "png": true, "gif": true, "webp": true,
	"tif": true, "tiff": true, "bmp": true, "heic": true, "heif": true, "avif": true,
}
var videoExts = map[string]bool{
	"mp4": true, "mov": true, "avi": true, "mkv": true, "m4v": true,
	"webm": true, "flv": true, "wmv": true, "mpg": true, "mpeg": true, "3gp": true,
}

// extToType maps a lower-cased file extension to an asset type ("IMAGE" /
// "VIDEO"). Returns "" for unsupported types.
func extToType(ext string) string {
	ext = strings.TrimPrefix(strings.ToLower(ext), ".")
	if imageExts[ext] {
		return "IMAGE"
	}
	if videoExts[ext] {
		return "VIDEO"
	}
	return ""
}

// ingestOptions carries the metadata needed to register one media file that
// has already been written to disk (either the upload copy or an external
// file referenced in place).
type ingestOptions struct {
	OwnerID      string
	LibraryID    string
	FileName     string // original file name (for display / device asset id)
	Ext          string // file extension with leading dot, lower-cased
	Type         string // IMAGE | VIDEO (pre-resolved)
	OriginalPath string // on-disk path of the original bytes
	IsExternal   bool   // true => referenced in place, not copied
	Checksum     string // optional pre-computed hex sha1
	FileCreated  time.Time
	FileModified time.Time
	LocalDate    time.Time
}

// mediaResult carries the byproducts of analysing a media file: the generated
// thumbnail bytes, the EXIF extracted (for images), and the preferred local
// datetime (EXIF capture time when available).
type mediaResult struct {
	thumbBytes []byte // jpeg-encoded thumbnail, empty if none could be made
	exif       *Exif   // populated for images; nil for videos / failures
	localDate  time.Time
}

// processMedia reads the file at path, generates a thumbnail and (for images)
// extracts EXIF. It is the shared, side-effect-free core used both when
// registering a brand-new asset (ingestStoredFile) and when backfilling an
// existing asset in the background job system. The thumbnail bytes are NOT yet
// written to disk — the caller decides the path and persistence so this stays
// reusable for in-place updates.
func (a *App) processMedia(path, assetID, ownerID, typ string, fallbackDate time.Time) (*mediaResult, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	res := &mediaResult{localDate: fallbackDate}

	// Thumbnails live under the shared thumbnail dir regardless of origin.
	thumbDir := filepath.Join(a.cfg.ResourceDir, "thumbnail")
	_ = os.MkdirAll(thumbDir, 0o755)
	tp := filepath.Join(thumbDir, assetID+".jpg")

	if typ == "IMAGE" {
		exif := &Exif{ID: newUUID(), AssetID: assetID}
		if info, eerr := imgproc.Extract(raw); eerr == nil && info != nil {
			exif.Make = info.Make
			exif.Model = info.Model
			if info.DateTimeOriginal != "" {
				v := info.DateTimeOriginal
				exif.DateTimeOriginal = &v
			}
			exif.Latitude = info.Latitude
			exif.Longitude = info.Longitude
			if info.Orientation != 0 {
				o := info.Orientation
				exif.Orientation = &o
			}
			res.exif = exif
		}
		if out, terr := imgproc.Thumbnail(raw, 256); terr == nil && len(out) > 0 {
			res.thumbBytes = out
			_ = os.WriteFile(tp, out, 0o644)
		}
	} else if typ == "VIDEO" {
		if out, terr := a.video.Thumbnail(raw, video.ThumbnailOptions{
			MaxEdge: 320,
			Format:  "jpeg",
		}); terr == nil && len(out) > 0 {
			res.thumbBytes = out
			_ = os.WriteFile(tp, out, 0o644)
		}
	}

	// Prefer EXIF capture time for the timeline grouping when present.
	if res.exif != nil && res.exif.DateTimeOriginal != nil {
		if t, perr := time.Parse(time.RFC3339, *res.exif.DateTimeOriginal); perr == nil {
			res.localDate = t
		}
	}
	return res, nil
}

// ingestStoredFile generates the thumbnail / EXIF and creates the Asset (and
// Exif) rows for a media file already present at opts.OriginalPath. It is
// shared by the upload endpoint and the library filesystem scan so both paths
// produce identical assets.
func (a *App) ingestStoredFile(opts ingestOptions) (*Asset, error) {
	raw, err := os.ReadFile(opts.OriginalPath)
	if err != nil {
		return nil, err
	}

	// Checksum (sha1 over content), used for dedup and the wire field.
	checksum := opts.Checksum
	if checksum == "" {
		h := sha1.New()
		h.Write(raw)
		checksum = hex.EncodeToString(h.Sum(nil))
	}

	id := newUUID()
	now := time.Now().UTC()

	res, err := a.processMedia(opts.OriginalPath, id, opts.OwnerID, opts.Type, opts.LocalDate)
	if err != nil {
		return nil, err
	}
	thumbPath := ""
	if res.thumbBytes != nil {
		thumbPath = filepath.Join(a.cfg.ResourceDir, "thumbnail", id+".jpg")
	}
	local := res.localDate

	asset := Asset{
		ID:               id,
		DeviceAssetId:    opts.OwnerID + "-" + checksum[:min(16, len(checksum))],
		DeviceId:         "immich-go-ingest",
		OwnerID:          opts.OwnerID,
		Type:             opts.Type,
		OriginalPath:     opts.OriginalPath,
		OriginalFileName: opts.FileName,
		ResizePath:       thumbPath,
		Checksum:         checksum,
		FileCreatedAt:    opts.FileCreated,
		FileModifiedAt:   opts.FileModified,
		LocalDateTime:    local,
		IsExternal:       opts.IsExternal,
		LibraryId:        opts.LibraryID,
		HasThumbnail:     thumbPath != "",
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	if res.exif != nil {
		if err := a.store.DB.Create(res.exif).Error; err != nil {
			return nil, err
		}
		asset.ExifID = res.exif.ID
	}
	if err := a.store.DB.Create(&asset).Error; err != nil {
		return nil, err
	}
	return &asset, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// copyToUploadDir copies src into the resource upload dir under a fresh UUID
// name and returns the destination path.
func (a *App) copyToUploadDir(src, ext string) (string, error) {
	upDir := filepath.Join(a.cfg.ResourceDir, "upload")
	if err := os.MkdirAll(upDir, 0o755); err != nil {
		return "", err
	}
	dst := filepath.Join(upDir, newUUID()+"."+strings.TrimPrefix(ext, "."))
	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return "", err
	}
	return dst, nil
}
