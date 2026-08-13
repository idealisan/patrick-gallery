package video

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
)

// encodeImage encodes a raw image to jpeg/png bytes.
func encodeImage(img image.Image, format string) ([]byte, error) {
	var buf bytes.Buffer
	switch format {
	case "png":
		if err := png.Encode(&buf, img); err != nil {
			return nil, err
		}
	default: // jpeg
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 82}); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}
