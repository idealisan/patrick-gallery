package ocr

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultPrompt = "Transcribe every piece of visible text in this image. Return only the transcription, preserving reading order and line breaks. If there is no visible text, return an empty string."

// OpenAIConfig is shared by OpenAI and OpenAI-compatible vision services.
// BaseURL should be the API root, for example https://api.openai.com/v1.
type OpenAIConfig struct {
	BaseURL string
	APIKey  string
	Model   string
	Prompt  string
	Detail  string
	Timeout time.Duration
}

// BuildNetwork selects an OpenAI-compatible or generic JSON HTTP backend.
// Empty/off disables network OCR so native/community providers can be used
// alone in the surrounding fallback chain.
func BuildNetwork(provider string, config OpenAIConfig, generic HTTPConfig) (Processor, error) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "", "none", "off":
		return nil, nil
	case "openai-chat", "chat", "chat-completions":
		return NewOpenAIChat(config)
	case "openai-responses", "responses":
		return NewOpenAIResponses(config)
	case "http", "generic":
		return NewHTTP(generic)
	default:
		return nil, fmt.Errorf("unknown ocr network provider %q", provider)
	}
}

type openAIBackend struct {
	config OpenAIConfig
	path   string
	name   string
	client *http.Client
}

func newOpenAIBackend(config OpenAIConfig, path, name string) (*openAIBackend, error) {
	if strings.TrimSpace(config.BaseURL) == "" {
		return nil, fmt.Errorf("ocr %s base url is required", name)
	}
	if strings.TrimSpace(config.APIKey) == "" {
		return nil, fmt.Errorf("ocr %s api key is required", name)
	}
	if strings.TrimSpace(config.Model) == "" {
		return nil, fmt.Errorf("ocr %s model is required", name)
	}
	if config.Prompt == "" {
		config.Prompt = defaultPrompt
	}
	if config.Detail == "" {
		config.Detail = "high"
	}
	if config.Timeout <= 0 {
		config.Timeout = 120 * time.Second
	}
	return &openAIBackend{config: config, path: path, name: name, client: &http.Client{Timeout: config.Timeout}}, nil
}

func NewOpenAIChat(config OpenAIConfig) (Processor, error) {
	return newOpenAIBackend(config, "/chat/completions", "openai-chat")
}

func NewOpenAIResponses(config OpenAIConfig) (Processor, error) {
	return newOpenAIBackend(config, "/responses", "openai-responses")
}

func (b *openAIBackend) Name() string { return b.name }

func (b *openAIBackend) Recognize(req Request) (Result, error) {
	ctx := req.Context
	if ctx == nil {
		ctx = context.Background()
	}
	dataURL := "data:" + req.MIME + ";base64," + base64.StdEncoding.EncodeToString(req.Data)
	var payload any
	if b.name == "openai-chat" {
		payload = map[string]any{
			"model": b.config.Model,
			"messages": []any{map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "text", "text": b.config.Prompt},
					map[string]any{"type": "image_url", "image_url": map[string]any{"url": dataURL, "detail": b.config.Detail}},
				},
			}},
			"temperature": 0,
		}
	} else {
		payload = map[string]any{
			"model": b.config.Model,
			"input": []any{map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "input_text", "text": b.config.Prompt},
					map[string]any{"type": "input_image", "image_url": dataURL, "detail": b.config.Detail},
				},
			}},
		}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Result{}, fmt.Errorf("ocr %s encode: %w", b.name, err)
	}
	endpoint := strings.TrimRight(b.config.BaseURL, "/") + b.path
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Result{}, fmt.Errorf("ocr %s request: %w", b.name, err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+b.config.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := b.client.Do(httpReq)
	if err != nil {
		return Result{}, fmt.Errorf("ocr %s transport: %w", b.name, err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return Result{}, fmt.Errorf("ocr %s read: %w", b.name, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Result{}, fmt.Errorf("ocr %s status %d: %s", b.name, resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	var response map[string]any
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return Result{}, fmt.Errorf("ocr %s decode: %w", b.name, err)
	}
	text := extractOpenAIText(b.name, response)
	result := Result{Text: text}
	if err := validateResult(result); err != nil {
		return Result{}, fmt.Errorf("ocr %s: %w", b.name, err)
	}
	return result, nil
}

func extractOpenAIText(name string, response map[string]any) string {
	if name == "openai-chat" {
		choices, _ := response["choices"].([]any)
		if len(choices) == 0 {
			return ""
		}
		choice, _ := choices[0].(map[string]any)
		message, _ := choice["message"].(map[string]any)
		return contentText(message["content"])
	}
	if text, ok := response["output_text"].(string); ok {
		return strings.TrimSpace(text)
	}
	var parts []string
	items, _ := response["output"].([]any)
	for _, item := range items {
		obj, _ := item.(map[string]any)
		content, _ := obj["content"].([]any)
		for _, part := range content {
			p, _ := part.(map[string]any)
			if text, ok := p["text"].(string); ok {
				parts = append(parts, text)
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func contentText(content any) string {
	if text, ok := content.(string); ok {
		return strings.TrimSpace(text)
	}
	var parts []string
	items, _ := content.([]any)
	for _, item := range items {
		obj, _ := item.(map[string]any)
		if text, ok := obj["text"].(string); ok {
			parts = append(parts, text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}
