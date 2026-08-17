package app

import (
	"bytes"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"time"

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

// serveEncodedMP4 transcodes (or falls back to the original) and streams the
// MP4. Shared with /api/assets/:id/encoded-video and the HLS segment.
//
// If the asset already has a cached transcoded file (EncodedVideoPath), it is
// served directly via http.ServeContent which supports HTTP Range requests for
// seeking. Otherwise the video is transcoded in-process, saved for reuse, and
// then served.
func (a *App) serveEncodedMP4(c *gin.Context, asset *Asset) {
	// Fast path: serve cached transcoded file if it exists.
	if asset.EncodedVideoPath != "" {
		if f, err := os.Open(asset.EncodedVideoPath); err == nil {
			defer f.Close()
			info, err := f.Stat()
			if err == nil && info.Size() > 0 {
				c.Header("Content-Type", "video/mp4")
				// http.ServeContent handles Range requests, Last-Modified,
				// and Content-Length automatically.
				http.ServeContent(c.Writer, c.Request, info.Name(), info.ModTime(), f)
				return
			}
		}
	}

	// Slow path: transcode from original.
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
		// Fallback: serve original.
		c.File(asset.OriginalPath)
		return
	}

	// Cache the transcoded result for future requests.
	encDir := filepath.Join(a.cfg.ResourceDir, "encoded-video")
	_ = os.MkdirAll(encDir, 0o755)
	dst := filepath.Join(encDir, asset.ID+".mp4")
	if werr := os.WriteFile(dst, out, 0o644); werr == nil {
		a.store.DB.Model(&Asset{}).Where("id = ?", asset.ID).Updates(map[string]any{
			"encoded_video_path": dst,
		})
		asset.EncodedVideoPath = dst
	}

	c.Header("Content-Type", "video/mp4")
	// Serve via http.ServeContent for Range request support.
	http.ServeContent(c.Writer, c.Request, "", time.Time{}, bytes.NewReader(out))
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
