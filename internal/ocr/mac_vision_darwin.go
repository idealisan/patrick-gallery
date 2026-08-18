//go:build darwin

package ocr

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"github.com/ebitengine/purego"
)

type macVisionLibrary struct {
	recognize func(data uintptr, length uintptr, language uintptr, result *uintptr, resultLength *uintptr) int
	free      func(ptr uintptr)
}

// NewMacVision loads the separately built Vision shim. It does not link
// Vision.framework into the CGO-disabled Go executable.
func NewMacVision(path string) (Processor, error) {
	if path == "" {
		path = filepath.Join("dist", "ocr", "macos", "libimmich_ocr_macos.dylib")
	}
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("mac vision library: %w", err)
	}
	handle, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return nil, fmt.Errorf("mac vision dlopen: %w", err)
	}
	b := &macVisionLibrary{}
	purego.RegisterLibFunc(&b.recognize, handle, "immich_ocr_recognize")
	purego.RegisterLibFunc(&b.free, handle, "immich_ocr_free")
	return b, nil
}

func (b *macVisionLibrary) Name() string { return "macos-vision" }
func (b *macVisionLibrary) Recognize(req Request) (Result, error) {
	var ptr, length uintptr
	language := uintptr(0)
	var languageBytes []byte
	if req.Language != "" {
		languageBytes = append([]byte(req.Language), 0)
		language = uintptr(unsafe.Pointer(&languageBytes[0]))
	}
	if len(req.Data) == 0 {
		return Result{}, fmt.Errorf("%w: empty image", ErrInvalidResult)
	}
	var data unsafe.Pointer
	data = unsafe.Pointer(&req.Data[0])
	code := b.recognize(uintptr(data), uintptr(len(req.Data)), language, &ptr, &length)
	if code != 0 {
		return Result{}, fmt.Errorf("mac vision recognize failed: code=%d", code)
	}
	defer b.free(ptr)
	raw := unsafe.Slice((*byte)(unsafe.Pointer(ptr)), length)
	var result Result
	if err := json.Unmarshal(raw, &result); err != nil {
		return Result{}, fmt.Errorf("mac vision decode: %w", err)
	}
	return result, validateResult(result)
}
