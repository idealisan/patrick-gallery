//go:build linux || darwin

package video

import (
	"os"
	"testing"
)

// TestFFmpegBackendProbeThumbnail proves the purego-loaded FFmpeg backend can
// open an in-memory container, probe it, and decode a frame to a JPEG — all
// in-process, no CGO, no CLI. Requires FFmpeg shared libs discoverable via
// dlopen on the host (CI bundles them next to the binary).
func TestFFmpegBackendProbeThumbnail(t *testing.T) {
	src, err := os.ReadFile("/tmp/test.mp4")
	if err != nil {
		t.Skip("generate /tmp/test.mp4 first: ffmpeg -f lavfi -i testsrc=duration=2:size=320x240:rate=15 -pix_fmt yuv420p /tmp/test.mp4 -y")
	}
	p, err := newFFmpeg()
	if err != nil {
		t.Fatalf("newFFmpeg: %v", err)
	}
	t.Logf("backend: %s", p.Name())

	m, err := p.Probe(src)
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	t.Logf("Probe: HasVideo=%v HasAudio=%v %dx%d video=%s audio=%s",
		m.HasVideo, m.HasAudio, m.Width, m.Height, m.VideoCodec, m.AudioCodec)
	if !m.HasVideo {
		t.Fatal("expected HasVideo=true")
	}
	if m.Width != 320 || m.Height != 240 {
		t.Fatalf("expected 320x240, got %dx%d", m.Width, m.Height)
	}

	thumb, err := p.Thumbnail(src, ThumbnailOptions{MaxEdge: 200, Format: "jpeg"})
	if err != nil {
		t.Fatalf("Thumbnail: %v", err)
	}
	t.Logf("thumbnail bytes=%d", len(thumb))
	if len(thumb) < 200 {
		t.Fatalf("thumbnail suspiciously small: %d bytes", len(thumb))
	}
	if err := os.WriteFile("/tmp/thumb_from_purego.jpg", thumb, 0644); err != nil {
		t.Fatal(err)
	}

	// PNG path also exercised.
	png, err := p.Thumbnail(src, ThumbnailOptions{MaxEdge: 128, Format: "png"})
	if err != nil {
		t.Fatalf("Thumbnail png: %v", err)
	}
	if len(png) < 100 {
		t.Fatalf("png thumbnail too small: %d", len(png))
	}
}

// TestFFmpegBackendTranscode proves the purego backend can re-encode an
// in-memory container to a real mp4 (decode -> sws_scale -> libx264 -> mux),
// entirely in-process, no CGO, no CLI. Output is written to
// /tmp/out_purego.mp4 for manual ffprobe inspection.
func TestFFmpegBackendTranscode(t *testing.T) {
	src, err := os.ReadFile("/tmp/test.mp4")
	if err != nil {
		t.Skip("generate /tmp/test.mp4 first")
	}
	p, err := newFFmpeg()
	if err != nil {
		t.Fatalf("newFFmpeg: %v", err)
	}
	out, err := p.Transcode(src, TranscodeOptions{Format: "mp4", Quality: 85})
	if err != nil {
		t.Fatalf("Transcode: %v", err)
	}
	t.Logf("transcoded bytes=%d (original %d)", len(out), len(src))
	if len(out) < 200 {
		t.Fatalf("transcoded output too small: %d", len(out))
	}
	// valid mp4 starts with an ftyp box (possibly after a 4-byte size)
	if !(out[4] == 'f' && out[5] == 't' && out[6] == 'y' && out[7] == 'p') &&
		!(out[0] == 0 && out[1] == 0 && out[2] == 0) {
		t.Fatalf("output does not look like an mp4 (no ftyp box)")
	}
	if err := os.WriteFile("/tmp/out_purego.mp4", out, 0644); err != nil {
		t.Fatal(err)
	}
}
