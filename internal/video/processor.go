// Package video defines the in-process video processing abstraction.
//
// Video is NEVER handled by shelling out to ffmpeg or any CLI. Backends are
// loaded as dynamically-linked native libraries and called in-process via
// github.com/ebitengine/purego (pure Go, no CGO). See AGENTS.md.
//
// A Processor may be:
//   - a software backend (FFmpeg shared libs loaded via purego) — universal,
//   - an OS-native hardware-accelerated backend (Video Toolbox / Media
//     Foundation / MediaCodec / QSV / AMF / NVENC) — preferred when present.
//
// Every backend must degrade gracefully: if it cannot process an input it
// returns a clear error and the caller falls back to the placeholder.
package video

import "image"

// Metadata describes a decoded media container.
type Metadata struct {
	Width      int
	Height     int
	DurationSec float64
	VideoCodec string
	AudioCodec string
	HasVideo   bool
	HasAudio   bool
	// Raw is the original file bytes (echoed back for convenience).
	Raw []byte
}

// ThumbnailOptions controls poster/thumbnail generation.
type ThumbnailOptions struct {
	// MaxEdge is the longest-edge target in px (0 => 320).
	MaxEdge int
	// SeekSec seeks to this position (seconds) for the poster frame.
	SeekSec float64
	// Format is "jpeg" or "png".
	Format string
}

// TranscodeOptions controls re-encoding (e.g. for /encoded-video).
type TranscodeOptions struct {
	Format     string // "mp4"
	VideoCodec string // "h264" | "hevc"
	AudioCodec string // "aac" | "copy" | ""
	Preset     string // "hardware" | "software" | "" (backend chooses)
	MaxWidth   int
	MaxHeight  int
	// Quality 0..100 (software h264 crf-ish hint).
	Quality int
}

// Processor is the video engine contract.
type Processor interface {
	// Name identifies the backend (e.g. "ffmpeg-software",
	// "videotoolbox", "placeholder").
	Name() string
	// HardwareAccel reports whether this backend uses GPU/native accel.
	HardwareAccel() bool
	// Probe returns container/stream metadata without full decode.
	Probe(in []byte) (*Metadata, error)
	// Thumbnail decodes a frame and returns encoded image bytes (jpeg/png).
	Thumbnail(in []byte, opts ThumbnailOptions) ([]byte, error)
	// Transcode re-encodes in to the requested options and returns the bytes.
	Transcode(in []byte, opts TranscodeOptions) ([]byte, error)
}

// ToImage is a helper for backends that produce a raw *image.Image; it is
// kept here so all backends share the same encode path if they choose.
func EncodeImage(img image.Image, format string) ([]byte, error) {
	return encodeImage(img, format)
}
