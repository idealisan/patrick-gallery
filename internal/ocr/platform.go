package ocr

import "runtime"

// PlatformFactory is the seam for native/community implementations. The
// functions are injected by platform-specific packages or future adapters;
// nil means that backend is not bundled on this build.
type PlatformFactory struct {
	MacOS   func() Processor
	Windows func() Processor
	Linux   func() Processor
}

func NewPlatform(factory PlatformFactory) Processor {
	var processor Processor
	switch runtime.GOOS {
	case "darwin":
		if factory.MacOS != nil {
			processor = factory.MacOS()
		}
	case "windows":
		if factory.Windows != nil {
			processor = factory.Windows()
		}
	case "linux":
		if factory.Linux != nil {
			processor = factory.Linux()
		}
	}
	return processor
}
