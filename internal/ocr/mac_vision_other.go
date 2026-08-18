//go:build !darwin

package ocr

import "fmt"

func NewMacVision(string) (Processor, error) {
	return nil, fmt.Errorf("%w: macOS Vision is only available on darwin", ErrUnavailable)
}
