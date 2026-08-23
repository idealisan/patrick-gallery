//go:build (linux || darwin) && !(arm || 386 || mips || mipsle || loong64)

package image

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego"
)

// heif_native.go — bundle-aware purego loader for the native libheif shared
// library. gen2brain/heic v0.5.0 only dlopens bare "libheif.dylib"/"libheif.so"
// (plus /opt/homebrew), so a lib bundled next to our binary in libs/ is never
// found and HEIC decoding silently falls back to its embedded WASM — which
// mis-renders iPhone grid HEICs (green-tinted output). This loader searches
// the bundle first, then the well-known system/brew locations, and exposes
// decodeNativeHEIC used by Decode when the vendored dynamic path is absent.

const (
	heifColorspaceYCbCr  = 0
	heifColorspaceRGB    = 1
	heifColorspaceMono   = 2
	heifChroma420        = 1
	heifChroma422        = 2
	heifChroma444        = 3
	heifChromaInterleaved = 99
	heifChannelY         = 0
	heifChannelCb        = 1
	heifChannelCr        = 2
	heifChannelInterleaved = 10
)

type (
	heifContext     struct{}
	heifImageHandle struct{}
	heifImage       struct{}
)

type heifError struct {
	Code    uint32
	Subcode uint32
	Message *int8
}

type heifDecodingOptions struct {
	Version          uint8
	startProgress    *[0]byte
	onProgress       *[0]byte
	endProgress      *[0]byte
	progressUserData *byte
	ConvertHdrTo8bit uint8
}

var (
	nativeLibheif uintptr

	nCheckFiletype   func(*uint8, uint64) int
	nContextAlloc    func() *heifContext
	nContextFree     func(*heifContext)
	nCtxReadMem      func(*heifContext, *uint8, uint64) heifError
	nCtxPrimary      func(*heifContext, **heifImageHandle) heifError
	nHandleW         func(*heifImageHandle) int
	nHandleH         func(*heifImageHandle) int
	nHandlePremul    func(*heifImageHandle) int
	nHandleRelease   func(*heifImageHandle)
	nOptAlloc        func() *heifDecodingOptions
	nOptFree         func(*heifDecodingOptions)
	nDecodeImage     func(*heifImageHandle, **heifImage, int, int, *heifDecodingOptions) heifError
	nPlaneReadonly   func(*heifImage, int, *int) *uint8
	nPreferredCS     func(*heifImageHandle, *int, *int) heifError
	hasPreferredCS   bool
)

func heifNativeAvailable() bool { return nativeLibheif != 0 }

// initNativeHeif loads the native libheif once, searching the release bundle
// (libs/ next to the executable) before system locations.
func initNativeHeif() {
	if nativeLibheif != 0 {
		return
	}
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(exe)
		for _, c := range []string{
			filepath.Join(dir, "libs", "libheif.dylib"),
			filepath.Join(dir, "libs", "libheif.so"),
			filepath.Join(dir, "libheif.dylib"),
			filepath.Join(dir, "libheif.so"),
		} {
			if _, serr := os.Stat(c); serr == nil {
				if h, lerr := purego.Dlopen(c, purego.RTLD_NOW|purego.RTLD_GLOBAL); lerr == nil {
					nativeLibheif = h
					break
				}
			}
		}
	}
	if nativeLibheif == 0 {
		names := []string{"libheif.dylib", "/opt/homebrew/lib/libheif.dylib", "/usr/local/lib/libheif.dylib", "libheif.so"}
		for _, n := range names {
			if h, lerr := purego.Dlopen(n, purego.RTLD_NOW|purego.RTLD_GLOBAL); lerr == nil {
				nativeLibheif = h
				break
			}
		}
	}
	if nativeLibheif == 0 {
		return
	}
	purego.RegisterLibFunc(&nCheckFiletype, nativeLibheif, "heif_check_filetype")
	purego.RegisterLibFunc(&nContextAlloc, nativeLibheif, "heif_context_alloc")
	purego.RegisterLibFunc(&nContextFree, nativeLibheif, "heif_context_free")
	purego.RegisterLibFunc(&nCtxReadMem, nativeLibheif, "heif_context_read_from_memory_without_copy")
	purego.RegisterLibFunc(&nCtxPrimary, nativeLibheif, "heif_context_get_primary_image_handle")
	purego.RegisterLibFunc(&nHandleW, nativeLibheif, "heif_image_handle_get_width")
	purego.RegisterLibFunc(&nHandleH, nativeLibheif, "heif_image_handle_get_height")
	purego.RegisterLibFunc(&nHandlePremul, nativeLibheif, "heif_image_handle_is_premultiplied_alpha")
	purego.RegisterLibFunc(&nHandleRelease, nativeLibheif, "heif_image_handle_release")
	purego.RegisterLibFunc(&nOptAlloc, nativeLibheif, "heif_decoding_options_alloc")
	purego.RegisterLibFunc(&nOptFree, nativeLibheif, "heif_decoding_options_free")
	purego.RegisterLibFunc(&nDecodeImage, nativeLibheif, "heif_decode_image")
	purego.RegisterLibFunc(&nPlaneReadonly, nativeLibheif, "heif_image_get_plane_readonly")
	// Optional (libheif >= 1.17): preferred decoding colorspace.
	func() {
		defer func() { recover() }()
		purego.RegisterLibFunc(&nPreferredCS, nativeLibheif, "heif_image_handle_get_preferred_decoding_colorspace")
		hasPreferredCS = true
	}()
}

// decodeNativeHEIC decodes raw HEIC bytes through the native libheif loaded by
// initNativeHeif. Returns ok=false when the native library is unavailable or
// the file cannot be decoded.
func decodeNativeHEIC(raw []byte) (img image.Image, w, h int, ok bool) {
	if !heifNativeAvailable() || len(raw) == 0 {
		return nil, 0, 0, false
	}
	if nCheckFiletype(&raw[0], uint64(len(raw))) != 1 {
		return nil, 0, 0, false
	}
	ctx := nContextAlloc()
	defer nContextFree(ctx)
	if e := nCtxReadMem(ctx, &raw[0], uint64(len(raw))); e.Code != 0 {
		return nil, 0, 0, false
	}
	var handle *heifImageHandle
	if e := nCtxPrimary(ctx, &handle); e.Code != 0 {
		return nil, 0, 0, false
	}
	defer nHandleRelease(handle)

	w = nHandleW(handle)
	h = nHandleH(handle)
	premul := nHandlePremul(handle) != 0

	colorspace, chroma := heifColorspaceYCbCr, heifChroma420
	model := color.YCbCrModel
	if hasPreferredCS {
		var cs, ch int
		if e := nPreferredCS(handle, &cs, &ch); e.Code == 0 && cs != 0 && ch != 0 {
			colorspace, chroma = cs, ch
			if cs == heifColorspaceRGB {
				if premul {
					model = color.RGBAModel
				} else {
					model = color.NRGBAModel
				}
			}
		}
	}

	opts := nOptAlloc()
	opts.ConvertHdrTo8bit = 1
	defer nOptFree(opts)

	var himg *heifImage
	if e := nDecodeImage(handle, &himg, colorspace, chroma, opts); e.Code != 0 {
		return nil, 0, 0, false
	}
	rect := image.Rect(0, 0, w, h)

	switch colorspace {
	case heifColorspaceYCbCr:
		var ratio image.YCbCrSubsampleRatio
		switch chroma {
		case heifChroma422:
			ratio = image.YCbCrSubsampleRatio422
		case heifChroma444:
			ratio = image.YCbCrSubsampleRatio444
		default:
			ratio = image.YCbCrSubsampleRatio420
		}
		var ys, cs_ int
		yP := nPlaneReadonly(himg, heifChannelY, &ys)
		cbP := nPlaneReadonly(himg, heifChannelCb, &cs_)
		crP := nPlaneReadonly(himg, heifChannelCr, &cs_)
		chh := (rect.Max.Y+1)/2 - rect.Min.Y/2
		if chroma == heifChroma444 {
			chh = rect.Dy()
		}
		i0 := ys * h
		i1 := i0 + cs_*chh
		i2 := i1 + cs_*chh
		b := make([]byte, i2)
		copy(b[:i0], unsafe.Slice(yP, ys*h))
		out := &image.YCbCr{
			Y: b[:i0:i0], Cb: b[i0:i1:i1], Cr: b[i1:i2:i2],
			SubsampleRatio: ratio, YStride: ys, CStride: cs_, Rect: rect,
		}
		copy(out.Cb, unsafe.Slice(cbP, cs_*chh))
		copy(out.Cr, unsafe.Slice(crP, cs_*chh))
		return out, w, h, true
	case heifColorspaceRGB:
		var stride int
		p := nPlaneReadonly(himg, heifChannelInterleaved, &stride)
		size := h * stride
		if premul {
			i := &image.RGBA{Pix: make([]uint8, size), Stride: stride, Rect: rect}
			copy(i.Pix, unsafe.Slice(p, size))
			return i, w, h, true
		}
		i := &image.NRGBA{Pix: make([]uint8, size), Stride: stride, Rect: rect}
		copy(i.Pix, unsafe.Slice(p, size))
		_ = model
		return i, w, h, true
	default:
		return nil, 0, 0, false
	}
}

// maxReadable removed: plane lengths are computed exactly (stride*height) at
// the call sites.

func minLen(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var _ = fmt.Sprintf // keep fmt for future debug builds
var _ = runtime.GOOS
