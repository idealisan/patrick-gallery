package ml

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

type OpenAIConfig struct {
	BaseURL string
	APIKey  string
	Model   string
	Timeout time.Duration
}

type OpenAIVision struct {
	config     OpenAIConfig
	path, name string
	client     *http.Client
}

func NewOpenAIVision(config OpenAIConfig, responses bool) (*OpenAIVision, error) {
	if config.BaseURL == "" || config.APIKey == "" || config.Model == "" {
		return nil, fmt.Errorf("ml OpenAI base URL, API key, and model are required")
	}
	if config.Timeout <= 0 {
		config.Timeout = 120 * time.Second
	}
	path, name := "/chat/completions", "openai-ml-chat"
	if responses {
		path, name = "/responses", "openai-ml-responses"
	}
	return &OpenAIVision{config: config, path: path, name: name, client: &http.Client{Timeout: config.Timeout}}, nil
}

func (b *OpenAIVision) Name() string { return b.name }
func (b *OpenAIVision) Capabilities() []Capability {
	return []Capability{CapabilitySemanticSearch, CapabilityImageDescription}
}

func (b *OpenAIVision) Infer(req Request) (Result, error) {
	if req.Capability != CapabilitySemanticSearch && req.Capability != CapabilityImageDescription {
		return Result{}, ErrUnsupported
	}
	ctx := req.Context
	if ctx == nil {
		ctx = context.Background()
	}
	dataURL := "data:" + req.MIME + ";base64," + base64.StdEncoding.EncodeToString(req.Data)
	prompt := "Describe this image in concise searchable text, including visible objects, scenes, activities, and readable text. Return only the description."
	if req.Capability == CapabilitySemanticSearch {
		prompt = "Answer only YES or NO. Does this image match the user's search request? User request: " + req.Query
	}
	var payload any
	if strings.HasSuffix(b.path, "completions") {
		payload = map[string]any{"model": b.config.Model, "temperature": 0, "messages": []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": prompt}, map[string]any{"type": "image_url", "image_url": map[string]any{"url": dataURL, "detail": "high"}}}}}}
	} else {
		payload = map[string]any{"model": b.config.Model, "input": []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": prompt}, map[string]any{"type": "input_image", "image_url": dataURL, "detail": "high"}}}}}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Result{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(b.config.BaseURL, "/")+b.path, bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+b.config.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := b.client.Do(httpReq)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return Result{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Result{}, fmt.Errorf("ml OpenAI status %d: %s", resp.StatusCode, raw)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return Result{}, err
	}
	text := ""
	if b.path == "/chat/completions" {
		if cs, ok := decoded["choices"].([]any); ok && len(cs) > 0 {
			m, _ := cs[0].(map[string]any)
			msg, _ := m["message"].(map[string]any)
			text = contentString(msg["content"])
		}
	} else if v, ok := decoded["output_text"].(string); ok {
		text = v
	}
	if req.Capability == CapabilityImageDescription {
		if strings.TrimSpace(text) == "" {
			return Result{}, ErrInvalidResult
		}
		return Result{Text: strings.TrimSpace(text)}, nil
	}
	answer := strings.ToUpper(strings.TrimSpace(text))
	score := float32(0)
	if answer == "YES" || strings.HasPrefix(answer, "YES ") {
		score = 1
	}
	if answer != "YES" && answer != "NO" && !strings.HasPrefix(answer, "YES ") && !strings.HasPrefix(answer, "NO ") {
		return Result{}, fmt.Errorf("ml semantic response was not YES/NO: %q", text)
	}
	return Result{Text: text, Score: score}, nil
}

func contentString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	var p []string
	if a, ok := v.([]any); ok {
		for _, x := range a {
			if m, ok := x.(map[string]any); ok {
				if s, ok := m["text"].(string); ok {
					p = append(p, s)
				}
			}
		}
	}
	return strings.Join(p, "\n")
}
