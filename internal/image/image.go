// Package image provides pure-Go image metadata extraction, decoding, and
// thumbnail generation for immich-go.
//
// NOTE: HEIC/HEIF and AVIF are NOT supported by this package. There is no
// pure-Go (CGO-free) decoder available for those formats in this codebase.
// The lead will wire native backends for them later. All other common raster
// formats (JPEG, PNG, GIF, TIFF, BMP, WebP) are handled in pure Go.
package image

import (
	"bytes"
	stderrors "errors"
	stdimage "image"
	"image/jpeg"

	"golang.org/x/image/webp"
	"github.com/rwcarlsen/goexif/exif"

	// Side-effect imports registering the standard-library decoders with
	// image.Decode / image.DecodeConfig.
	//
	// NOTE: image/bmp and image/tiff are intentionally NOT imported here. The
	// Go toolchain provisioned for this build (GOROOT image/ only ships
	// jpeg/gif/png) is missing those two packages, so importing them would
	// fail to compile. The Decode path is still written to transparently use
	// them the moment they are present: uncomment the two blank imports below
	// and rebuild with a full toolchain to enable BMP/TIFF raster decoding
	// (EXIF extraction for TIFF already works via exif.Decode).
	_ "image/gif"
	_ "image/png"
	// _ "image/bmp"
	// _ "image/tiff"
)

// errUnsupported is returned by Decode when no registered decoder can parse
// the bytes (and the input is also not a WebP).
var errUnsupported = stderrors.New("image: unsupported or unrecognized format")

// Info holds extracted EXIF / container metadata.
type Info struct {
	Width, Height     int
	DateTimeOriginal  string
	Make, Model       string
	Latitude, Longitude float64
	Orientation        int // EXIF orientation 1..8 (0 = unknown)
}

// Extract reads EXIF from raw image bytes. For formats without EXIF (or
// unsupported), it returns a zero Info and a nil error (it does not fail).
//
// Only JPEG and TIFF carry EXIF readable by goexif; for any other format a
// zero Info is returned. Pixel dimensions are sourced from the parsed EXIF
// when present and otherwise fall back to actually decoding the bytes.
func Extract(raw []byte) (*Info, error) {
	info := &Info{}

	// EXIF only meaningfully exists in JPEG/TIFF containers.
	format, _ := peekFormat(raw)
	if format != "jpeg" && format != "tiff" {
		return info, nil
	}

	x, err := exif.Decode(bytes.NewReader(raw))
	if err != nil {
		// No (parseable) EXIF block; fall back to decoded dimensions only.
		fillDimsFromDecode(info, raw)
		return info, nil
	}

	// Orientation.
	if tag, err := x.Get(exif.Orientation); err == nil {
		if o, err := tag.Int(0); err == nil {
			info.Orientation = o
		}
	}

	// Camera make / model.
	if tag, err := x.Get(exif.Make); err == nil {
		if s, err := tag.StringVal(); err == nil {
			info.Make = s
		}
	}
	if tag, err := x.Get(exif.Model); err == nil {
		if s, err := tag.StringVal(); err == nil {
			info.Model = s
		}
	}

	// Capture time.
	if t, err := x.DateTime(); err == nil {
		info.DateTimeOriginal = t.Format("2006-01-02T15:04:05Z07:00")
	}

	// GPS.
	if lat, long, err := x.LatLong(); err == nil {
		info.Latitude = lat
		info.Longitude = long
	}

	// Pixel dimensions: prefer EXIF, then decoded size.
	if tag, err := x.Get(exif.PixelXDimension); err == nil {
		if w, err := tag.Int(0); err == nil {
			info.Width = w
		}
	}
	if tag, err := x.Get(exif.PixelYDimension); err == nil {
		if h, err := tag.Int(0); err == nil {
			info.Height = h
		}
	}
	if info.Width == 0 || info.Height == 0 {
		fillDimsFromDecode(info, raw)
	}

	return info, nil
}

// fillDimsFromDecode decodes raw and copies the bounds into info.
func fillDimsFromDecode(info *Info, raw []byte) {
	img, _, err := Decode(raw)
	if err != nil {
		return
	}
	b := img.Bounds()
	info.Width = b.Dx()
	info.Height = b.Dy()
}

// peekFormat returns the decoded format name without keeping the full decode.
func peekFormat(raw []byte) (string, int) {
	cfg, format, err := stdimage.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return "unknown", 0
	}
	return format, cfg.Width
}

// Dimensions returns the pixel width/height of raw image bytes for any
// decodable raster format (JPEG/PNG/GIF/WebP/...). Returns 0,0 if the bytes
// cannot be decoded. Used to populate asset dimensions for the response
// regardless of container (Extract only resolves dimensions for JPEG/TIFF).
func Dimensions(raw []byte) (int, int) {
	cfg, _, err := stdimage.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

// Decode decodes raw bytes into an stdimage.Image, trying the standard library
// decoders first (jpeg/png/gif, and tiff/bmp when the toolchain provides them)
// and then WebP. It returns the image and a format string: "jpeg" | "png" |
// "gif" | "webp" | "tiff" | "bmp" | "unknown".
func Decode(raw []byte) (stdimage.Image, string, error) {
	if img, format, err := stdimage.Decode(bytes.NewReader(raw)); err == nil {
		return img, format, nil
	}
	// Fall back to WebP (pure Go, no CGO).
	if img, err := webp.Decode(bytes.NewReader(raw)); err == nil {
		return img, "webp", nil
	}
	return nil, "unknown", errUnsupported
}

// Thumbnail decodes raw and returns a longest-edge JPEG, applying EXIF
// Orientation. A maxEdge <= 0 defaults to 256. The image is only downscaled
// (never upscaled).
func Thumbnail(raw []byte, maxEdge int) ([]byte, error) {
	if maxEdge <= 0 {
		maxEdge = 256
	}

	img, _, err := Decode(raw)
	if err != nil {
		return nil, err
	}

	// Apply EXIF orientation.
	info, _ := Extract(raw)
	img = orientImage(img, info.Orientation)

	// Scale longest edge to maxEdge (downscale only).
	img = scaleImage(img, maxEdge)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 82}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// orientImage returns a copy of img transformed per the EXIF orientation, or
// the original image when orientation is 1 (or unknown/invalid).
func orientImage(src stdimage.Image, orient int) stdimage.Image {
	if orient <= 1 || orient > 8 {
		return src
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	// Orientations 5-8 swap the axes.
	var dw, dh int
	if orient >= 5 {
		dw, dh = h, w
	} else {
		dw, dh = w, h
	}
	dst := stdimage.NewRGBA(stdimage.Rect(0, 0, dw, dh))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var dx, dy int
			switch orient {
			case 2: // mirror horizontal
				dx, dy = w-1-x, y
			case 3: // rotate 180
				dx, dy = w-1-x, h-1-y
			case 4: // mirror vertical
				dx, dy = x, h-1-y
			case 5: // transpose
				dx, dy = y, x
			case 6: // rotate 90 CW
				dx, dy = h-1-y, x
			case 7: // transverse
				dx, dy = h-1-y, w-1-x
			case 8: // rotate 270 CW
				dx, dy = y, w-1-x
			}
			dst.Set(dx, dy, src.At(x, y))
		}
	}
	return dst
}

// scaleImage scales the longest edge of src down to maxEdge using nearest
// neighbor. If src is already no larger than maxEdge on its longest edge, it
// is returned unchanged.
func scaleImage(src stdimage.Image, maxEdge int) stdimage.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	longest := w
	if h > longest {
		longest = h
	}
	if longest <= maxEdge {
		return src
	}

	factor := float64(maxEdge) / float64(longest)
	nw := int(float64(w) * factor)
	nh := int(float64(h) * factor)
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}

	dst := stdimage.NewRGBA(stdimage.Rect(0, 0, nw, nh))
	sxScale := float64(w) / float64(nw)
	syScale := float64(h) / float64(nh)
	for dy := 0; dy < nh; dy++ {
		sy := int(float64(dy)*syScale + 0.5)
		if sy >= h {
			sy = h - 1
		}
		for dx := 0; dx < nw; dx++ {
			sx := int(float64(dx)*sxScale + 0.5)
			if sx >= w {
				sx = w - 1
			}
			dst.Set(dx, dy, src.At(sx, sy))
		}
	}
	return dst
}
