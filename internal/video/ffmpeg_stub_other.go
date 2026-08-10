//go:build !linux && !darwin

package video

// On platforms without a usable purego FFmpeg loader (notably Windows, where
// purego.Dlopen / RTLD_* are unavailable), the in-process software video
// backend cannot be built. We still compile everywhere by providing a stub
// that reports the backend as unavailable, so New() cleanly falls through to
// the placeholder (no thumbnails/transcode on those platforms). Live on
// linux/darwin the real implementation lives in ffmpeg.go.
func newFFmpeg() (Processor, error) {
	return nil, errBackendUnavailable
}
