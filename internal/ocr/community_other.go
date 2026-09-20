//go:build !linux && !darwin

package ocr

import (
	"fmt"
	"runtime"
)

// NewCommunity reports that the dlopen-based community OCR backend is not
// available on this platform (purego has no dynamic loader here, e.g. on
// Windows). Returning an honest error lets callers fall back to another
// backend (see NewPlatform).
func NewCommunity(config CommunityConfig) (Processor, error) {
	return nil, fmt.Errorf("community OCR library: %w on %s", ErrUnavailable, runtime.GOOS)
}
