package ocr

import (
	"fmt"
	"runtime"
)

// PlatformFactory is the seam for native/community implementations. The
// functions are injected by platform-specific packages or future adapters;
// nil means that backend is not bundled on this build.
type PlatformFactory struct {
	MacOSNative      func() Processor
	WindowsNative    func() Processor
	LinuxNative      func() Processor
	WindowsCommunity func() Processor
	LinuxCommunity   func() Processor
}

func NewPlatform(factory PlatformFactory) Processor {
	var processor Processor
	switch runtime.GOOS {
	case "darwin":
		if factory.MacOSNative != nil {
			processor = factory.MacOSNative()
		}
	case "windows":
		if factory.WindowsNative != nil {
			processor = factory.WindowsNative()
		} else if factory.WindowsCommunity != nil {
			processor = factory.WindowsCommunity()
		}
	case "linux":
		if factory.LinuxNative != nil {
			processor = factory.LinuxNative()
		} else if factory.LinuxCommunity != nil {
			processor = factory.LinuxCommunity()
		}
	}
	return processor
}

type unavailable struct{ name string }

func (b unavailable) Name() string { return b.name }
func (b unavailable) Recognize(Request) (Result, error) {
	return Result{}, fmt.Errorf("%w: %s", ErrUnavailable, b.name)
}

func Unavailable(name string) Processor { return unavailable{name: name} }
