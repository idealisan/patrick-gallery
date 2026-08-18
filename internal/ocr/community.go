package ocr

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego"
)

// CommunityConfig describes a community OCR shared library exposing the same
// stable C ABI as the macOS shim. The library may be built with CGO/C++/Rust;
// the Go server itself remains CGO-free and loads it through purego.
type CommunityConfig struct {
	Path string
}

type communityBackend struct {
	name      string
	recognize func(data uintptr, length uintptr, mime uintptr, language uintptr, result *uintptr, resultLength *uintptr) int
	free      func(ptr uintptr)
}

func NewCommunity(config CommunityConfig) (Processor, error) {
	if config.Path == "" {
		return nil, fmt.Errorf("community OCR library path is required")
	}
	if _, err := os.Stat(config.Path); err != nil {
		return nil, fmt.Errorf("community OCR library: %w", err)
	}
	handle, err := purego.Dlopen(config.Path, purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return nil, fmt.Errorf("community OCR dlopen: %w", err)
	}
	b := &communityBackend{name: "community-" + runtime.GOOS}
	purego.RegisterLibFunc(&b.recognize, handle, "immich_ocr_recognize")
	purego.RegisterLibFunc(&b.free, handle, "immich_ocr_free")
	return b, nil
}

func (b *communityBackend) Name() string { return b.name }

func (b *communityBackend) Recognize(req Request) (Result, error) {
	if len(req.Data) == 0 {
		return Result{}, fmt.Errorf("%w: empty input", ErrInvalidResult)
	}
	mime := append([]byte(req.MIME), 0)
	language := append([]byte(req.Language), 0)
	var ptr, length uintptr
	code := b.recognize(uintptr(unsafe.Pointer(&req.Data[0])), uintptr(len(req.Data)), uintptr(unsafe.Pointer(&mime[0])), uintptr(unsafe.Pointer(&language[0])), &ptr, &length)
	if code != 0 {
		return Result{}, fmt.Errorf("community OCR failed: code=%d", code)
	}
	defer b.free(ptr)
	raw := unsafe.Slice((*byte)(unsafe.Pointer(ptr)), length)
	var result Result
	if err := json.Unmarshal(raw, &result); err != nil {
		// Community C libraries commonly expose plain UTF-8 text rather than
		// JSON. Keep JSON support for structured adapters with boxes/confidence.
		result = Result{Text: string(raw)}
	}
	return result, validateResult(result)
}
