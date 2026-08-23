package image

// pipeline_version.go — S7 (Epic: maintenance-self-healing).
//
// CacheVersion declares, per format family, the current generation of the
// decode/encode pipeline that produces thumbnails and previews. Bump a family
// ONLY when a release changes its output incompatibly (e.g. a decoder fix that
// alters pixels): S8 then treats caches stamped with an older version as stale
// and regenerates them. Compatible upgrades must NOT bump — no rebuild churn.



type CacheFamily string

const (
	FamilyJPEG CacheFamily = "jpeg"
	FamilyPNG  CacheFamily = "png"
	FamilyHEIC CacheFamily = "heic"
	FamilyWebP CacheFamily = "webp"
	FamilyGIF  CacheFamily = "gif"
)

// cacheVersions maps each family to its current generator version. Start at 1.
var cacheVersions = map[CacheFamily]uint32{
	FamilyJPEG: 1,
	FamilyPNG:  1,
	FamilyHEIC: 1,
	FamilyWebP: 1,
	FamilyGIF:  1,
}

// CurrentCacheVersion returns the active version for a family (0 if unknown
// family — callers treat unknown families as always-fresh single-version).
func CurrentCacheVersion(f CacheFamily) uint32 {
	return cacheVersions[f]
}

// BumpCacheVersion advances a family's version by one (release-time action;
// not expected to be called at runtime).
func BumpCacheVersion(f CacheFamily) uint32 {
	v := cacheVersions[f] + 1
	cacheVersions[f] = v
	return v
}

// FamilyForMIME maps an asset's original format to its cache family. Unknown
// formats map to "" so version checks pass trivially.
func FamilyForFormat(format string) CacheFamily {
	switch format {
	case "jpeg":
		return FamilyJPEG
	case "png":
		return FamilyPNG
	case "heic", "heif", "avif":
		return FamilyHEIC
	case "webp":
		return FamilyWebP
	case "gif":
		return FamilyGIF
	default:
		return ""
	}
}

// SniffFormat returns a coarse format family for raw bytes ("jpeg", "png",
// "heic", "webp", "gif", or ""). Cheap magic-byte sniffing; no full decode.
func SniffFormat(raw []byte) string {
	switch {
	case len(raw) >= 3 && raw[0] == 0xFF && raw[1] == 0xD8 && raw[2] == 0xFF:
		return "jpeg"
	case len(raw) >= 4 && raw[0] == 0x89 && string(raw[1:4]) == "PNG":
		return "png"
	case HasHEICBrand(raw):
		return "heic"
	case len(raw) >= 12 && string(raw[0:4]) == "RIFF" && string(raw[8:12]) == "WEBP":
		return "webp"
	case len(raw) >= 6 && (string(raw[0:3]) == "GIF"):
		return "gif"
	default:
		return ""
	}
}
