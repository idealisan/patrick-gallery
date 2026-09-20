//go:build linux || darwin

package ocr

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego"
)

// communityBackend loads a third-party OCR shared library through purego. The
// pointer parameters and out-parameters are typed as unsafe.Pointer (not
// uintptr) so go vet's unsafeptr check is satisfied — purego maps
// unsafe.Pointer/*T to void*.
type communityBackend struct {
	name      string
	recognize func(data unsafe.Pointer, length uintptr, mime unsafe.Pointer, language unsafe.Pointer, result *unsafe.Pointer, resultLength *uintptr) int
	free      func(ptr unsafe.Pointer)
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
	var ptr unsafe.Pointer
	var length uintptr
	code := b.recognize(unsafe.Pointer(&req.Data[0]), uintptr(len(req.Data)), unsafe.Pointer(&mime[0]), unsafe.Pointer(&language[0]), &ptr, &length)
	if code != 0 {
		return Result{}, fmt.Errorf("community OCR failed: code=%d", code)
	}
	defer b.free(ptr)
	raw := unsafe.Slice((*byte)(ptr), length)
	var result Result
	if err := json.Unmarshal(raw, &result); err != nil {
		// Community C libraries commonly expose plain UTF-8 text rather than
		// JSON. Keep JSON support for structured adapters with boxes/confidence.
		result = Result{Text: string(raw)}
	}
	return result, validateResult(result)
}
