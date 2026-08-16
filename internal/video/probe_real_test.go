package video

import (
	"os"
	"testing"
)

// TestProbeThumbnailReal exercises the purego FFmpeg backend against a real
// video file. It is skipped unless VIDEO_TEST_FILE points at one, so it never
// fails in CI (where no FFmpeg shared libs are bundled). Run locally with:
//
//	LD_LIBRARY_PATH=dist/libs VIDEO_TEST_FILE=path/to/clip.mp4 \
//	  go test ./internal/video/ -run TestProbeThumbnailReal -v
func TestProbeThumbnailReal(t *testing.T) {
	path := os.Getenv("VIDEO_TEST_FILE")
	if path == "" {
		t.Skip("VIDEO_TEST_FILE not set")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	p := New()
	t.Logf("backend: %s (hwAccel=%v)", p.Name(), p.HardwareAccel())

	md, err := p.Probe(data)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	t.Logf("probe: hasVideo=%v %dx%d dur=%.3fs vcodec=%s acodec=%s",
		md.HasVideo, md.Width, md.Height, md.DurationSec, md.VideoCodec, md.AudioCodec)
	if !md.HasVideo || md.Width == 0 || md.Height == 0 {
		t.Fatalf("probe returned no usable video dimensions")
	}
	if md.DurationSec <= 0 {
		t.Fatalf("probe returned non-positive duration")
	}

	thumb, err := p.Thumbnail(data, ThumbnailOptions{MaxEdge: 320, Format: "jpeg"})
	if err != nil {
		t.Fatalf("thumbnail: %v", err)
	}
	// A JPEG always starts with SOI (0xFFD8FF); the 4th byte is the marker
	// (0xE0 JFIF, 0xE1 EXIF, 0xDB quant table, ...) so accept any SOI prefix.
	if len(thumb) < 3 || thumb[0] != 0xff || thumb[1] != 0xd8 || thumb[2] != 0xff {
		end := len(thumb)
		if end > 4 {
			end = 4
		}
		t.Fatalf("thumbnail not a JPEG (len=%d prefix=%x)", len(thumb), thumb[:end])
	}
	t.Logf("thumbnail: %d bytes JPEG", len(thumb))
}
