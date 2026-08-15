package image

import (
	"encoding/base64"
	stdimage "image"
	"image/draw"
	"math"
)

// Thumbhash computes a ThumbHash (base64 string) from raw image bytes. The
// algorithm is a faithful Go port of the reference implementation
// (github.com/evanw/thumbhash, npm "thumbhash" v0.1.1) used by the official
// Immich mobile/web clients, so the generated placeholder is byte-compatible
// with what the apps decode. Input is decoded, downscaled to fit 100px (the
// reference rejects anything larger), converted to RGBA and DCT-encoded.
func Thumbhash(raw []byte) (string, error) {
	img, _, err := Decode(raw)
	if err != nil {
		return "", err
	}
	// The reference refuses inputs > 100x100; downscale to fit on the long edge.
	img = scaleImage(img, 100)
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return "", nil
	}
	dst := stdimage.NewRGBA(stdimage.Rect(0, 0, w, h))
	draw.Draw(dst, dst.Bounds(), img, b.Min, draw.Src)
	hash := rgbaToThumbHash(w, h, dst.Pix)
	return base64.StdEncoding.EncodeToString(hash), nil
}

func round(x float64) float64 { return math.Round(x) }

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// rgbaToThumbHash encodes an RGBA image (row-major, w*h*4 bytes, NOT
// premultiplied) of at most 100x100 into a ThumbHash byte slice. Ported
// verbatim from the reference TypeScript implementation.
func rgbaToThumbHash(w, h int, rgba []byte) []byte {
	if w > 100 || h > 100 {
		return nil
	}
	n := w * h

	// Average color (weighted by alpha, in 0..1 space).
	avgR, avgG, avgB, avgA := 0.0, 0.0, 0.0, 0.0
	for i := 0; i < n; i++ {
		j := i * 4
		alpha := float64(rgba[j+3]) / 255.0
		avgR += alpha / 255.0 * float64(rgba[j])
		avgG += alpha / 255.0 * float64(rgba[j+1])
		avgB += alpha / 255.0 * float64(rgba[j+2])
		avgA += alpha
	}
	if avgA != 0 {
		avgR /= avgA
		avgG /= avgA
		avgB /= avgA
	}

	hasAlpha := avgA < float64(n)
	lLimit := 7
	if hasAlpha {
		lLimit = 5
	}
	lx := max(1, int(round(float64(lLimit)*float64(w)/float64(max(w, h)))))
	ly := max(1, int(round(float64(lLimit)*float64(h)/float64(max(w, h)))))

	// Convert RGBA -> LPQA (composited atop the average color).
	l := make([]float64, n)
	p := make([]float64, n)
	q := make([]float64, n)
	a := make([]float64, n)
	for i := 0; i < n; i++ {
		j := i * 4
		alpha := float64(rgba[j+3]) / 255.0
		r := avgR*(1-alpha) + alpha/255.0*float64(rgba[j])
		g := avgG*(1-alpha) + alpha/255.0*float64(rgba[j+1])
		b := avgB*(1-alpha) + alpha/255.0*float64(rgba[j+2])
		l[i] = (r + g + b) / 3
		p[i] = (r + g)/2 - b
		q[i] = r - g
		a[i] = alpha
	}

	// Forward DCT per channel into DC + normalized AC terms.
	encodeChannel := func(channel []float64, nx, ny int) (dc float64, ac []float64, scale float64) {
		for cy := 0; cy < ny; cy++ {
			for cx := 0; cx*ny < nx*(ny-cy); cx++ {
				f := 0.0
				fx := make([]float64, w)
				for x := 0; x < w; x++ {
					fx[x] = math.Cos(math.Pi/float64(w)*float64(cx)*(float64(x)+0.5))
				}
				for y := 0; y < h; y++ {
					fy := math.Cos(math.Pi/float64(h)*float64(cy)*(float64(y)+0.5))
					for x := 0; x < w; x++ {
						f += channel[x+y*w] * fx[x] * fy
					}
				}
				f /= float64(n)
				if cx != 0 || cy != 0 {
					ac = append(ac, f)
					if math.Abs(f) > scale {
						scale = math.Abs(f)
					}
				} else {
					dc = f
				}
			}
		}
		if scale != 0 {
			for i := range ac {
				ac[i] = 0.5 + 0.5/scale*ac[i]
			}
		}
		return
	}

	lDc, lAc, lScale := encodeChannel(l, max(3, lx), max(3, ly))
	pDc, pAc, pScale := encodeChannel(p, 3, 3)
	qDc, qAc, qScale := encodeChannel(q, 3, 3)
	var aDc float64
	var aAc []float64
	var aScale float64
	if hasAlpha {
		aDc, aAc, aScale = encodeChannel(a, 5, 5)
	}

	isLandscape := w > h
	header24 := int(round(63*lDc)) |
		(int(round(31.5+31.5*pDc)) << 6) |
		(int(round(31.5+31.5*qDc)) << 12) |
		(int(round(31*lScale)) << 18) |
		(b2i(hasAlpha) << 23)
	header16 := (b2iIf(isLandscape, ly, lx)) |
		(int(round(63*pScale)) << 3) |
		(int(round(63*qScale)) << 9) |
		(b2i(isLandscape) << 15)

	hash := []byte{
		byte(header24 & 255),
		byte((header24 >> 8) & 255),
		byte(header24 >> 16),
		byte(header16 & 255),
		byte(header16 >> 8),
	}
	acStart := 5
	acIndex := 0
	if hasAlpha {
		hash = append(hash, byte(int(round(15*aDc))|(int(round(15*aScale))<<4)))
		acStart = 6
	}

	channels := [][]float64{lAc, pAc, qAc}
	if hasAlpha {
		channels = append(channels, aAc)
	}
	for _, acArr := range channels {
		for _, f := range acArr {
			idx := acStart + (acIndex >> 1)
			if idx >= len(hash) {
				hash = append(hash, 0)
			}
			hash[idx] |= byte(int(round(15*f)) << ((acIndex & 1) << 2))
			acIndex++
		}
	}
	return hash
}

func b2iIf(cond bool, a, b int) int {
	if cond {
		return a
	}
	return b
}
