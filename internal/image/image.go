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
	"math"

	"golang.org/x/image/draw"
	"golang.org/x/image/webp"
	"github.com/gen2brain/heic"
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
// JPEG and TIFF carry EXIF readable by goexif directly; HEIC/HEIF containers
// embed the TIFF block as an 'Exif' meta item which we slice out first.
func Extract(raw []byte) (*Info, error) {
	info := &Info{}

	// EXIF only meaningfully exists in JPEG/TIFF containers — or inside a
	// HEIC/HEIF 'Exif' item, which we unwrap into a plain TIFF block.
	exifBytes := raw
	format, _ := peekFormat(raw)
	if format != "jpeg" && format != "tiff" {
		if HasHEICBrand(raw) {
			payload := heicExtractEXIF(raw)
			if payload == nil {
				return info, nil
			}
			exifBytes = payload
		} else {
			return info, nil
		}
	}

	x, err := exif.Decode(bytes.NewReader(exifBytes))
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
// decodable raster format (JPEG/PNG/GIF/WebP/HEIC/...). Returns 0,0 if the
// bytes cannot be decoded. Used to populate asset dimensions for the response
// regardless of container (Extract only resolves dimensions for JPEG/TIFF).
func Dimensions(raw []byte) (int, int) {
	if w, h := heicDimensions(raw); w > 0 {
		return w, h
	}
	cfg, _, err := stdimage.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

// heicDimensions reads the pixel size of a HEIC/HEIF container without a full
// pixel decode (heic.DecodeConfig is cheap; falls back to 0,0 when not HEIC).
func heicDimensions(raw []byte) (int, int) {
	if len(raw) < 12 || !bytes.Equal(raw[4:8], []byte("ftyp")) ||
		(!bytes.Contains(raw[8:12], []byte("heic")) &&
			!bytes.Contains(raw[8:12], []byte("heix")) &&
			!bytes.Contains(raw[8:12], []byte("hevc")) &&
			!bytes.Contains(raw[8:12], []byte("heif")) &&
			!bytes.Contains(raw[8:12], []byte("mif1"))) {
		return 0, 0
	}
	cfg, err := heic.DecodeConfig(bytes.NewReader(raw))
	if err != nil || cfg.Width <= 0 {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

// Decode decodes raw bytes into an stdimage.Image, trying the standard library
// decoders first (jpeg/png/gif, and tiff/bmp when the toolchain provides them)
// and then WebP and HEIC. HEIC prefers the native libheif loaded from our
// release bundle via purego (correct iPhone grid rendering); when unavailable
// it falls back to gen2brain/heic (dynamic libheif, else its embedded WASM).
func Decode(raw []byte) (stdimage.Image, string, error) {
	if img, format, err := stdimage.Decode(bytes.NewReader(raw)); err == nil {
		return img, format, nil
	}
	// Fall back to WebP (pure Go, no CGO).
	if img, err := webp.Decode(bytes.NewReader(raw)); err == nil {
		return img, "webp", nil
	}
	// Fall back to HEIC/HEIF (iPhone stills). All paths are CGO-free.
	initNativeHeif()
	if img, _, _, ok := decodeNativeHEIC(raw); ok {
		return img, "heic", nil
	}
	if img, err := heic.Decode(bytes.NewReader(raw)); err == nil {
		return img, "heic", nil
	}
	return nil, "unknown", errUnsupported
}

// Thumbnail decodes raw and returns a longest-edge JPEG thumbnail, applying
// EXIF Orientation. A maxEdge <= 0 defaults to 256. The image is only
// downscaled (never upscaled). It delegates to Preview so the thumbnail also
// benefits from high-quality resampling (no more distorted lines).
func Thumbnail(raw []byte, maxEdge int) ([]byte, error) {
	if maxEdge <= 0 {
		maxEdge = 256
	}
	return Preview(raw, maxEdge, 82)
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

// scaleImage scales the longest edge of src down to at most maxEdge using
// high-quality resampling (golang.org/x/image/draw). The image is only
// downscaled, never upscaled. For large reductions (ratio > 2) an intermediate
// pass is applied to suppress aliasing. If src is already no larger than
// maxEdge on its longest edge, the original image is returned unchanged.
func scaleImage(src stdimage.Image, maxEdge int) stdimage.Image {
	if maxEdge <= 0 {
		return src
	}
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
	nw := int(math.Round(float64(w) * factor))
	nh := int(math.Round(float64(h) * factor))
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}

	dst := stdimage.NewRGBA(stdimage.Rect(0, 0, nw, nh))

	// A single cubic/B-spline pass aliases badly for large downscaling ratios
	// (sharp lines break up into artifacts). Apply a cheaper intermediate pass
	// to roughly twice the target size, then a final high-quality pass. This
	// mirrors how production image pipelines (libvips/sharp) downscale.
	ratio := float64(longest) / float64(maxEdge)
	if ratio > 2 {
		midFactor := float64(maxEdge*2) / float64(longest)
		midW := int(math.Round(float64(w) * midFactor))
		midH := int(math.Round(float64(h) * midFactor))
		if midW < 1 {
			midW = 1
		}
		if midH < 1 {
			midH = 1
		}
		mid := stdimage.NewRGBA(stdimage.Rect(0, 0, midW, midH))
		draw.ApproxBiLinear.Scale(mid, mid.Bounds(), src, src.Bounds(), draw.Over, nil)
		src = mid
	}

	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
	return dst
}

// Preview decodes raw and returns a longest-edge JPEG using high-quality
// resampling. A maxEdge <= 0 defaults to 2560; a quality <= 0 defaults to 85.
// The image is only downscaled (never upscaled). EXIF orientation is applied.
func Preview(raw []byte, maxEdge, quality int) ([]byte, error) {
	if maxEdge <= 0 {
		maxEdge = 2560
	}
	if quality <= 0 {
		quality = 85
	}
	img, _, err := Decode(raw)
	if err != nil {
		return nil, err
	}
	info, _ := Extract(raw)
	img = orientImage(img, info.Orientation)
	img = scaleImage(img, maxEdge)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
