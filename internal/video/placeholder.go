package video

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"os"
)

// placeholder is the universal fallback backend. It lets the server run and
// the API respond even when no native video library is available on the
// target machine (the cross-platform release binaries degrade to this).
type placeholder struct{}

func (placeholder) Name() string        { return "placeholder" }
func (placeholder) HardwareAccel() bool { return false }

func (placeholder) Probe(in []byte) (*Metadata, error) {
	// We cannot inspect the container; return a minimal, honest record.
	return &Metadata{HasVideo: false, Raw: in}, nil
}

// Thumbnail draws a neutral slate so the web UI has something to show.
func (placeholder) Thumbnail(in []byte, opts ThumbnailOptions) ([]byte, error) {
	edge := opts.MaxEdge
	if edge <= 0 {
		edge = 320
	}
	img := image.NewRGBA(image.Rect(0, 0, edge, edge))
	c := color.RGBA{R: 38, G: 42, B: 51, A: 255}
	for y := 0; y < edge; y++ {
		for x := 0; x < edge; x++ {
			img.Set(x, y, c)
		}
	}
	// simple centered "film" glyph: a lighter rounded rectangle
	pad := edge / 4
	for y := pad; y < edge-pad; y++ {
		for x := pad; x < edge-pad; x++ {
			img.Set(x, y, color.RGBA{R: 90, G: 98, B: 112, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Transcode returns the original untouched (no re-encoding available).
func (placeholder) Transcode(in []byte, opts TranscodeOptions) ([]byte, error) {
	return in, nil
}

func (placeholder) ProbeFile(path string) (*Metadata, error) {
	return &Metadata{HasVideo: false}, nil
}

func (placeholder) TranscodeToFile(srcPath, dstPath string, opts TranscodeOptions) error {
	// Copy the original file as-is (no re-encoding available).
	in, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}
	return os.WriteFile(dstPath, in, 0o644)
}

func (placeholder) RemuxFaststart(srcPath, dstPath string) error {
	in, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}
	return os.WriteFile(dstPath, in, 0o644)
}
