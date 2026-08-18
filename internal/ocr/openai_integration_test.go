package ocr

import (
	"os"
	"strings"
	"testing"
	"time"
)

// TestOpenAIChatRealImage exercises the production Go adapter end to end:
// Go reads the image, the adapter creates the base64 data URL, sends the
// Chat Completions request, decodes choices[0].message.content, and returns
// the normalized OCR Result. It is skipped unless explicitly configured.
func TestOpenAIChatRealImage(t *testing.T) {
	endpoint := os.Getenv("IMMICH_OCR_TEST_BASE_URL")
	apiKey := os.Getenv("IMMICH_OCR_TEST_API_KEY")
	model := os.Getenv("IMMICH_OCR_TEST_MODEL")
	imagePath := os.Getenv("IMMICH_OCR_TEST_IMAGE")
	if endpoint == "" || apiKey == "" || model == "" || imagePath == "" {
		t.Skip("set IMMICH_OCR_TEST_BASE_URL, IMMICH_OCR_TEST_API_KEY, IMMICH_OCR_TEST_MODEL, and IMMICH_OCR_TEST_IMAGE")
	}
	data, err := os.ReadFile(imagePath)
	if err != nil {
		t.Fatal(err)
	}
	backend, err := NewOpenAIChat(OpenAIConfig{
		BaseURL: endpoint,
		APIKey:  apiKey,
		Model:   model,
		Detail:  "high",
		Timeout: 120 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := backend.Recognize(Request{
		Data:     data,
		MIME:     "image/png",
		Name:     imagePath,
		Language: "zh-Hans",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Text == "" {
		t.Fatal("real Chat Completions OCR returned empty text")
	}
	if !strings.Contains(result.Text, "设计目标") || !strings.Contains(result.Text, "OCR") {
		t.Fatalf("unexpected real OCR text: %s", result.Text)
	}
	t.Logf("real Go Chat Completions OCR result:\n%s", result.Text)
}
