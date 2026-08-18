package ml

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestOpenAIVisionRealImage(t *testing.T) {
	baseURL := os.Getenv("IMMICH_ML_TEST_BASE_URL")
	apiKey := os.Getenv("IMMICH_ML_TEST_API_KEY")
	model := os.Getenv("IMMICH_ML_TEST_MODEL")
	imagePath := os.Getenv("IMMICH_ML_TEST_IMAGE")
	if baseURL == "" || apiKey == "" || model == "" || imagePath == "" {
		t.Skip("set IMMICH_ML_TEST_BASE_URL, IMMICH_ML_TEST_API_KEY, IMMICH_ML_TEST_MODEL, and IMMICH_ML_TEST_IMAGE")
	}
	data, err := os.ReadFile(imagePath)
	if err != nil {
		t.Fatal(err)
	}
	backend, err := NewOpenAIVision(OpenAIConfig{BaseURL: baseURL, APIKey: apiKey, Model: model, Timeout: 120 * time.Second}, false)
	if err != nil {
		t.Fatal(err)
	}
	result, err := backend.Infer(Request{Capability: CapabilitySemanticSearch, Data: data, MIME: "image/png", Name: imagePath, Query: "OCR and machine learning architecture design goals"})
	if err != nil {
		t.Fatal(err)
	}
	answer := strings.ToUpper(strings.TrimSpace(result.Text))
	if answer != "YES" && !strings.HasPrefix(answer, "YES ") && answer != "NO" && !strings.HasPrefix(answer, "NO ") {
		t.Fatalf("unexpected semantic result: %q", result.Text)
	}
	t.Logf("real ML semantic result: %q score=%v", result.Text, result.Score)
}

func TestOpenAIDescriptionRealImage(t *testing.T) {
	baseURL := os.Getenv("IMMICH_ML_TEST_BASE_URL")
	apiKey := os.Getenv("IMMICH_ML_TEST_API_KEY")
	model := os.Getenv("IMMICH_ML_TEST_MODEL")
	imagePath := os.Getenv("IMMICH_ML_TEST_IMAGE")
	if baseURL == "" || apiKey == "" || model == "" || imagePath == "" {
		t.Skip("set ML integration variables")
	}
	data, err := os.ReadFile(imagePath)
	if err != nil {
		t.Fatal(err)
	}
	backend, err := NewOpenAIVision(OpenAIConfig{BaseURL: baseURL, APIKey: apiKey, Model: model, Timeout: 120 * time.Second}, false)
	if err != nil {
		t.Fatal(err)
	}
	result, err := backend.Infer(Request{Capability: CapabilityImageDescription, Data: data, MIME: "image/png", Name: imagePath})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(result.Text) == "" {
		t.Fatal("real image description is empty")
	}
	t.Logf("real image description:\n%s", result.Text)
}
