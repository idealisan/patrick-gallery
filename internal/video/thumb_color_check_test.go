package video

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"os"
	"testing"
)

// TestThumbnailColorReal verifies the decoded frame is colour-correct (the
// bug we fixed produced a dark purple/grey frame because the source pixel
// format and chroma planes were misread). For a white-screen recording the
// average should be bright; we assert it is not a near-black purple.
func TestThumbnailColorReal(t *testing.T) {
	path := os.Getenv("VIDEO_TEST_FILE")
	if path == "" {
		t.Skip("VIDEO_TEST_FILE not set")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	p := New()
	thumb, err := p.Thumbnail(data, ThumbnailOptions{MaxEdge: 320, Format: "jpeg"})
	if err != nil {
		t.Fatalf("thumbnail: %v", err)
	}
	img, err := jpeg.Decode(bytes.NewReader(thumb))
	if err != nil {
		t.Fatalf("decode thumbnail: %v", err)
	}
	rgba := image.NewRGBA(img.Bounds())
	draw.Draw(rgba, rgba.Bounds(), img, img.Bounds().Min, draw.Src)
	img = rgba
	var r, g, b, n int64
	min := color.RGBA{255, 255, 255, 255}
	max := color.RGBA{0, 0, 0, 255}
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := img.At(x, y).(color.RGBA)
			r += int64(c.R)
			g += int64(c.G)
			b += int64(c.B)
			n++
			if c.R < min.R {
				min.R = c.R
			}
			if c.G < min.G {
				min.G = c.G
			}
			if c.B < min.B {
				min.B = c.B
			}
			if c.R > max.R {
				max.R = c.R
			}
			if c.G > max.G {
				max.G = c.G
			}
			if c.B > max.B {
				max.B = c.B
			}
		}
	}
	ar, ag, ab := float64(r)/float64(n), float64(g)/float64(n), float64(b)/float64(n)
	t.Logf("thumbnail avg RGB=(%.0f,%.0f,%.0f) min=(%d,%d,%d) max=(%d,%d,%d)",
		ar, ag, ab, min.R, min.G, min.B, max.R, max.G, max.B)
	// The recorded screen is white with black text: average must be bright
	// (not a dark purple). These thresholds reject the regression.
	if ar < 80 || ag < 80 || ab < 80 {
		t.Fatalf("thumbnail is too dark (avg RGB=%.0f,%.0f,%.0f) — colour bug not fixed", ar, ag, ab)
	}
	_ = image.Black
}
