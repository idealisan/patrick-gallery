//go:build darwin

package ocr

import (
	"os"
	"strings"
	"testing"
)

// TestOCRChainRealImage calls the complete OCR chain. On macOS the native
// Vision adapter is first; network/community providers are not configured in
// this test, so Vision must produce the result.
// Run explicitly with:
//
//	IMMICH_OCR_NATIVE_PATH=/path/to/libimmich_ocr_macos.dylib \
//	IMMICH_OCR_TEST_IMAGE=/path/to/screenshot.png \
//	go test ./internal/ocr -run TestMacVisionRealImage -v
//
// The test is skipped during ordinary CGO-free CI, where the macOS shim and
// private fixture are intentionally not bundled in the Go test package.
func TestOCRChainRealImage(t *testing.T) {
	shim := os.Getenv("IMMICH_OCR_NATIVE_PATH")
	imagePath := os.Getenv("IMMICH_OCR_TEST_IMAGE")
	if shim == "" || imagePath == "" {
		t.Skip("set IMMICH_OCR_NATIVE_PATH and IMMICH_OCR_TEST_IMAGE for the real Vision integration test")
	}
	data, err := os.ReadFile(imagePath)
	if err != nil {
		t.Fatal(err)
	}
	native, err := NewMacVision(shim)
	if err != nil {
		t.Fatal(err)
	}
	processor := NewChain(native)
	result, err := processor.Recognize(Request{
		Data:     data,
		MIME:     "image/png",
		Name:     imagePath,
		Language: "zh-Hans",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Text == "" {
		t.Fatal("OCR chain returned empty OCR text")
	}
	for _, expected := range []string{"设计目标", "OCR", "机器学习"} {
		if !strings.Contains(result.Text, expected) {
			t.Errorf("OCR text missing %q: %s", expected, result.Text)
		}
	}
	t.Logf("Vision OCR result:\n%s", result.Text)
}

func TestOCRChainCommunityFallbackRealImage(t *testing.T) {
	communityPath := os.Getenv("IMMICH_OCR_COMMUNITY_PATH")
	imagePath := os.Getenv("IMMICH_OCR_TEST_IMAGE")
	if communityPath == "" || imagePath == "" {
		t.Skip("set IMMICH_OCR_COMMUNITY_PATH and IMMICH_OCR_TEST_IMAGE for the real community fallback test")
	}
	data, err := os.ReadFile(imagePath)
	if err != nil {
		t.Fatal(err)
	}
	community, err := NewCommunity(CommunityConfig{Path: communityPath})
	if err != nil {
		t.Fatal(err)
	}
	chain := NewChain(Unavailable("native-test-failure"), community)
	result, err := chain.Recognize(Request{Data: data, MIME: "image/png", Name: imagePath, Language: "zh-Hans"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Text == "" {
		t.Fatal("community fallback returned empty OCR text")
	}
	if !strings.Contains(result.Text, "设计目标") && !strings.Contains(result.Text, "OCR") {
		t.Fatalf("unexpected community OCR result: %s", result.Text)
	}
}
