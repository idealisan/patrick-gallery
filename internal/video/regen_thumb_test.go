package video

import (
	"os"
	"testing"
)

// TestRegenUserThumb regenerates the on-disk thumbnail for an existing asset
// after a code fix, so the running server serves the corrected image without
// a full re-ingest. Run:
//
//	LD_LIBRARY_PATH=/workspace/dist/libs \
//	REGEN_SRC=/workspace/resources/upload/38cc54d1-....mp4 \
//	REGEN_OUT=/workspace/resources/thumbnail/212b7c26-....jpg \
//	go test ./internal/video/ -run TestRegenUserThumb -v
func TestRegenUserThumb(t *testing.T) {
	src := os.Getenv("REGEN_SRC")
	out := os.Getenv("REGEN_OUT")
	if src == "" || out == "" {
		t.Skip("REGEN_SRC/REGEN_OUT not set")
	}
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read src: %v", err)
	}
	p := New()
	thumb, err := p.Thumbnail(data, ThumbnailOptions{MaxEdge: 320, Format: "jpeg"})
	if err != nil {
		t.Fatalf("thumbnail: %v", err)
	}
	if err := os.WriteFile(out, thumb, 0o644); err != nil {
		t.Fatalf("write out: %v", err)
	}
	t.Logf("wrote %d bytes to %s", len(thumb), out)
}
