package video

// Hardware-accelerated, OS-native backends are planned but not yet wired:
//   - newVideoToolbox  -> macOS / Apple  (Video Toolbox, via purego)
//   - newMediaFoundation -> Windows     (Media Foundation, via purego)
//   - newMediaCodec    -> Android/Linux (MediaCodec / native API, via purego)
//
// Each returns errBackendUnavailable until its purego binding lands, so the
// selector falls through to the software FFmpeg backend, then placeholder.
// Implementations will live in build-tagged files (native_darwin.go, etc.).
func newVideoToolbox() (Processor, error)    { return nil, errBackendUnavailable }
func newMediaFoundation() (Processor, error) { return nil, errBackendUnavailable }
func newMediaCodec() (Processor, error)      { return nil, errBackendUnavailable }
