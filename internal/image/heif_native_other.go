//go:build !linux && !darwin || arm || 386 || mips || mipsle || loong64

package image

import "image"

// initNativeHeif is a no-op on platforms that do not build the purego libheif
// loader (heif_native.go): Windows and 32-bit architectures. HEIC decoding
// falls back to the pure-Go gen2brain/heic path in Decode.
func initNativeHeif() {}

// decodeNativeHEIC reports that the native libheif backend is unavailable on
// this platform, so Decode falls through to the vendored gen2brain/heic path.
func decodeNativeHEIC(raw []byte) (image.Image, int, int, bool) {
	return nil, 0, 0, false
}
