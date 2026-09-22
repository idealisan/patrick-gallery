package video

import "log"

// New returns the best available video backend, in priority order:
//   1. OS-native hardware-accelerated backend (Video Toolbox / Media
//      Foundation / MediaCodec) when present,
//   2. software FFmpeg shared libs loaded via purego,
//   3. placeholder (always works; no thumbnails/transcode).
//
// It never returns nil. Each constructor returns an error if its required
// native library cannot be loaded, so selection is purely capability-based.
func New() Processor {
	for _, c := range []struct {
		name string
		ctor func() (Processor, error)
	}{
		{"videotoolbox", newVideoToolbox},       // macOS / Apple
		{"mediafoundation", newMediaFoundation}, // Windows
		{"mediacodec", newMediaCodec},           // Android / Linux native
		{"ffmpeg", newFFmpeg},                   // software, cross-platform
	} {
		p, err := c.ctor()
		if err == nil && p != nil {
			log.Printf("[video] using backend: %s (hardwareAccel=%v)", p.Name(), p.HardwareAccel())
			return p
		}
		// Log WHY a backend was skipped: an ABI mismatch (e.g. a distro
		// FFmpeg that is not the pinned 7.x) is otherwise invisible.
		if err != nil {
			log.Printf("[video] backend %s unavailable: %v", c.name, err)
		}
	}
	log.Println("[video] no native/ffmpeg backend available; using placeholder (no thumbnails/transcode)")
	return placeholder{}
}

// backends below are provided by other files in this package:
//   - ffmpeg.go        -> newFFmpeg (software, purego-loaded libav*)
//   - native_darwin.go -> newVideoToolbox
//   - native_windows.go-> newMediaFoundation
//   - native_linux.go  -> newMediaCodec
// Each returns (nil, err) when its library is absent so New() falls through.
