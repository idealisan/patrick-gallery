package app

import (
	"testing"
)

// TestAssetDurationResponseContract verifies BUG-004 fix: duration must be
// integer milliseconds, and null for non-video assets (so the official web UI
// never renders "NaN" in the video-duration overlay).
func TestAssetDurationResponseContract(t *testing.T) {
	// VIDEO with a stored millisecond value -> returned as-is (ms).
	v := Asset{Type: "VIDEO", Duration: "12000"}
	if got := assetDurationResponse(v); got == nil || *got != 12000 {
		t.Fatalf("VIDEO duration: want 12000ms, got %v", got)
	}

	// VIDEO uploaded with seconds in the (legacy) store -> treated as ms.
	v2 := Asset{Type: "VIDEO", Duration: "12"}
	if got := assetDurationResponse(v2); got == nil || *got != 12 {
		t.Fatalf("legacy VIDEO duration: want 12ms (raw stored), got %v", got)
	}

	// IMAGE must never carry a duration -> null (web guard isVideo=false, but
	// even if mis-typed this prevents NaN).
	i := Asset{Type: "IMAGE", Duration: "12000"}
	if got := assetDurationResponse(i); got != nil {
		t.Fatalf("IMAGE duration: want nil, got %v", got)
	}

	// VIDEO with empty duration -> null (web renders 0:00, not NaN).
	ve := Asset{Type: "VIDEO", Duration: ""}
	if got := assetDurationResponse(ve); got != nil {
		t.Fatalf("empty VIDEO duration: want nil, got %v", got)
	}
}

// TestParseDurationMs verifies the upload path stores the client's millisecond
// value as-is, per the v3.1.0 contract (AssetMediaCreateDto.duration is
// "Duration in milliseconds (for videos)" and the mobile client sends
// asset.durationMs verbatim; the official server stores dto.duration directly).
func TestParseDurationMs(t *testing.T) {
	if got := parseDurationMs("46567"); got == nil || *got != 46567 {
		t.Fatalf("ms passthrough: want 46567, got %v", got)
	}
	if got := parseDurationMs("12000"); got == nil || *got != 12000 {
		t.Fatalf("ms passthrough: want 12000, got %v", got)
	}
	// empty / zero / negative / non-numeric -> nil (null), never NaN.
	for _, in := range []string{"", "0", "-5", "abc"} {
		if got := parseDurationMs(in); got != nil {
			t.Fatalf("%q -> want nil, got %v", in, got)
		}
	}
}
