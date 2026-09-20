package ocr

// CommunityConfig describes a community OCR shared library exposing the same
// stable C ABI as the macOS shim. The library may be built with CGO/C++/Rust;
// the Go server itself remains CGO-free and loads it through purego.
//
// NewCommunity is only implemented on platforms whose dynamic loader purego
// supports (linux/darwin); elsewhere it returns ErrUnavailable (see
// community_other.go).
type CommunityConfig struct {
	Path string
}
