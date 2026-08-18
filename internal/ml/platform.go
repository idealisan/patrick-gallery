package ml

import "runtime"

// PlatformFactory is the seam for real OS-native and community adapters.
// Factories must return actual model-backed implementations; nil means the
// adapter is not compiled or configured and lets the chain continue.
type PlatformFactory struct {
	MacOSNative Backend
	WindowsNative Backend
	LinuxNative Backend
	WindowsCommunity Backend
	LinuxCommunity Backend
}

func OrderedPlatformBackends(factory PlatformFactory) []Backend {
	switch runtime.GOOS {
	case "darwin":
		return compact(factory.MacOSNative)
	case "windows":
		return compact(factory.WindowsNative, factory.WindowsCommunity)
	case "linux":
		return compact(factory.LinuxNative, factory.LinuxCommunity)
	default:
		return nil
	}
}

func compact(backends ...Backend) []Backend {
	result := make([]Backend, 0, len(backends))
	for _, backend := range backends {
		if backend != nil { result = append(result, backend) }
	}
	return result
}
