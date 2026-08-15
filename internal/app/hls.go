package app

import (
	"fmt"
	"net/http"
	"os"

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
func (a *App) serveEncodedMP4(c *gin.Context, asset *Asset) {
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
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	// Absolute variant URL so the client can resolve it regardless of how the
	// master playlist itself was requested.
	variant := fmt.Sprintf("%s://%s/api/assets/%s/video/stream/%s/0/playlist.m3u8",
		scheme, c.Request.Host, asset.ID, asset.ID)
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
	dur := parseDurationInt(asset.Duration)
	if dur <= 0 {
		dur = 3600 // safe VOD fallback when duration is unknown
	}
	c.Header("Content-Type", "application/vnd.apple.mpegurl")
	// One segment referencing our transcoded MP4 (relative path resolves to the
	// segment route below).
	c.String(http.StatusOK,
		"#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-PLAYLIST-TYPE:VOD\n#EXTINF:%.3f,\n%s\n#EXT-X-ENDLIST\n",
		float64(dur), "seg-0.mp4")
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
