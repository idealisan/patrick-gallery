package app

import (
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"immich-go/internal/video"
)

// hls.go adds HTTP Live Streaming (HLS) compatibility for the official Immich
// v3.1.0 clients. Immich serves videos as HLS: a master .m3u8 that points to a
// variant playlist, which in turn lists segments. immich-go transcodes
// in-process to a single MP4; we wrap that MP4 in a single-variant HLS
// presentation so the official players (ExoPlayer on Android, AVPlayer on iOS)
// play it without changes.
//
// The asset id is used as the HLS session id, which keeps every URL in the
// playlists self-consistent so the client can simply follow them.

// hlsVideo loads + authorizes a VIDEO asset, or writes a 4xx and returns false.
func (a *App) hlsVideo(c *gin.Context) (*Asset, bool) {
	uid := currentUserID(c)
	id := c.Param("id")
	var asset Asset
	if err := a.store.DB.First(&asset, "id = ?", id).Error; err != nil {
		c.Status(http.StatusNotFound)
		return nil, false
	}
	if asset.OwnerID != uid && !a.isAdmin(uid) {
		c.Status(http.StatusForbidden)
		return nil, false
	}
	if asset.Type != "VIDEO" {
		c.Status(http.StatusNotFound)
		return nil, false
	}
	return &asset, true
}

// serveEncodedMP4 streams a video with the following decision logic:
//  1. If a cached transcoded file exists -> serve it (with LRU access update).
//  2. Probe the original file -> check bitrate threshold for resolution.
//  3. If within threshold -> serve original directly (no transcode needed).
//  4. If over threshold -> transcode to file -> faststart -> cache -> serve.
func (a *App) serveEncodedMP4(c *gin.Context, asset *Asset) {
	// 1. Fast path: serve cached transcoded file if it exists.
	if asset.EncodedVideoPath != "" {
		if f, err := os.Open(asset.EncodedVideoPath); err == nil {
			defer f.Close()
			info, err := f.Stat()
			if err == nil && info.Size() > 0 {
				if a.videoCache != nil {
					a.videoCache.RecordAccess(filepath.Base(asset.EncodedVideoPath))
				}
				c.Header("Content-Type", "video/mp4")
				http.ServeContent(c.Writer, c.Request, info.Name(), info.ModTime(), f)
				return
			}
		}
	}

	// 2. Probe the original file to get resolution and bitrate.
	origPath := asset.OriginalPath
	meta, err := a.video.ProbeFile(origPath)
	if err != nil || meta == nil {
		// Probe failed — serve original as fallback.
		c.File(origPath)
		return
	}

	// 3. Check bitrate threshold: if within limit, serve original directly.
	thresholds := video.DefaultBitrateThresholds()
	needsTranscode, _ := thresholds.NeedsTranscode(meta.Width, meta.Bitrate)
	if !needsTranscode {
		c.File(origPath)
		return
	}

	// 4. Transcode: file-to-file with hardware acceleration fallback.
	encDir := filepath.Join(a.cfg.ResourceDir, "encoded-video")
	_ = os.MkdirAll(encDir, 0o755)
	dst := filepath.Join(encDir, asset.ID+".mp4")

	opts := video.TranscodeOptions{
		Format:     "mp4",
		VideoCodec: "h264",
		AudioCodec: "copy",
		Preset:     "fast",
	}
	if err := a.video.TranscodeToFile(origPath, dst, opts); err != nil {
		// Transcode failed — serve original as fallback.
		c.File(origPath)
		return
	}

	// Post-process: remux with faststart so moov is at the front for streaming.
	// If this fails, serve the non-faststart version — it still works via Range
	// requests, just needs the full download before playback starts.
	tmpDst := dst + ".faststart.mp4"
	if err := a.video.RemuxFaststart(dst, tmpDst); err == nil {
		os.Rename(tmpDst, dst)
	}

	// Record in cache.
	if a.videoCache != nil {
		info, err := os.Stat(dst)
		if err == nil {
			a.videoCache.Add(filepath.Base(dst), info.Size())
			a.videoCache.Evict() // enforce LRU budget
		}
	}

	// Update asset in DB.
	a.store.DB.Model(&Asset{}).Where("id = ?", asset.ID).Updates(map[string]any{
		"encoded_video_path": dst,
	})
	asset.EncodedVideoPath = dst

	// Serve the transcoded file.
	if f, err := os.Open(dst); err == nil {
		defer f.Close()
		info, err := f.Stat()
		if err == nil {
			c.Header("Content-Type", "video/mp4")
			http.ServeContent(c.Writer, c.Request, info.Name(), info.ModTime(), f)
			return
		}
	}
	// Last resort fallback.
	c.File(origPath)
}

// handleVideoPlayback mirrors GET /api/assets/:id/video/playback: returns the
// playable video bytes directly (application/octet-stream per the spec).
func (a *App) handleVideoPlayback(c *gin.Context) {
	asset, ok := a.hlsVideo(c)
	if !ok {
		return
	}
	a.serveEncodedMP4(c, asset)
}

// handleVideoStreamMaster mirrors GET /api/assets/:id/video/stream/main.m3u8.
func (a *App) handleVideoStreamMaster(c *gin.Context) {
	asset, ok := a.hlsVideo(c)
	if !ok {
		return
	}
	// Relative (scheme-less) variant URL. The official web resolves the
	// playlist against the page origin, so we must NOT hard-code http:// here:
	// behind a TLS-terminating reverse proxy the Go side sees
	// c.Request.TLS == nil and would otherwise emit an http:// URL, which the
	// HTTPS page blocks as mixed content (the variant playlist request fails
	// with STATUS 0 and the video never plays). A path-relative URL keeps the
	// page's scheme (https) and resolves against the master playlist URL
	// (/api/assets/:id/video/stream/ -> /api/assets/:id/video/stream/:id/0/
	// playlist.m3u8), matching the official hls.service.ts contract.
	variant := fmt.Sprintf("%s/0/playlist.m3u8", asset.ID)
	c.Header("Content-Type", "application/vnd.apple.mpegurl")
	c.String(http.StatusOK, "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=2000000,RESOLUTION=1280x720\n%s\n", variant)
}

// handleVideoStreamPlaylist mirrors
// GET /api/assets/:id/video/stream/:sessionId/:variantIndex/playlist.m3u8.
func (a *App) handleVideoStreamPlaylist(c *gin.Context) {
	asset, ok := a.hlsVideo(c)
	if !ok {
		return
	}
	// Asset.Duration is stored in MILLISECONDS (see durSecToMsString /
	// msToString), but HLS #EXTINF expects SECONDS. Emitting the raw value
	// (e.g. 46533.000) makes players refuse to start playback. Convert ms->s.
	durMs := parseDurationInt(asset.Duration)
	durSec := float64(durMs) / 1000.0
	if durSec <= 0 {
		durSec = 3600 // safe VOD fallback when duration is unknown
	}
	c.Header("Content-Type", "application/vnd.apple.mpegurl")
	// hls.js treats `#EXT-X-TARGETDURATION` as REQUIRED: a VOD playlist without
	// it fails level parsing with a fatal `levelParsingError` ("Missing Target
	// Duration"). The official Immich API (getVideoStreamPlaylist /
	// stream-playlist.gateway.ts) always emits `#EXT-X-TARGETDURATION` >= the
	// segment duration. Compute it from the asset's real duration (ceil), not
	// the fallback, so real streams never trip the parser.
	target := int(math.Ceil(durSec))
	if target < 1 {
		target = 1
	}
	// One segment referencing our transcoded MP4 (relative path resolves to the
	// segment route below).
	c.String(http.StatusOK,
		"#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-PLAYLIST-TYPE:VOD\n#EXT-X-TARGETDURATION:%d\n#EXTINF:%.3f,\n%s\n#EXT-X-ENDLIST\n",
		target, durSec, "seg-0.mp4")
}

// handleVideoStreamSegment mirrors
// GET /api/assets/:id/video/stream/:sessionId/:variantIndex/:filename and
// streams the transcoded MP4 (the filename is advisory).
func (a *App) handleVideoStreamSegment(c *gin.Context) {
	asset, ok := a.hlsVideo(c)
	if !ok {
		return
	}
	a.serveEncodedMP4(c, asset)
}

// handleVideoStreamDelete mirrors
// DELETE /api/assets/:id/video/stream/:sessionId. immich-go is stateless
// (segments are produced on demand), so this is a no-op 204.
func (a *App) handleVideoStreamDelete(c *gin.Context) {
	if _, ok := a.hlsVideo(c); !ok {
		return
	}
	c.Status(http.StatusNoContent)
}
