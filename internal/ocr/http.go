package ocr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HTTPConfig describes a remote OCR service. The service receives JSON:
// {"name":"...","mime":"...","data":"<base64>","language":"..."}
// and returns {"text":"...","words":[...]}.
type HTTPConfig struct {
	Endpoint string
	Token    string
	Timeout  time.Duration
}

type HTTPProcessor struct {
	config HTTPConfig
	client *http.Client
}

func NewHTTP(config HTTPConfig) (*HTTPProcessor, error) {
	if strings.TrimSpace(config.Endpoint) == "" {
		return nil, fmt.Errorf("ocr http endpoint is required")
	}
	if config.Timeout <= 0 {
		config.Timeout = 60 * time.Second
	}
	return &HTTPProcessor{config: config, client: &http.Client{Timeout: config.Timeout}}, nil
}

func (p *HTTPProcessor) Name() string { return "http" }

func (p *HTTPProcessor) Recognize(req Request) (Result, error) {
	if req.Context == nil {
		req.Context = context.Background()
	}
	payload := struct {
		Name string `json:"name"`
		MIME string `json:"mime"`
		Data []byte `json:"data"`
	}{Name: req.Name, MIME: req.MIME, Data: req.Data}
	body, err := json.Marshal(payload)
	if err != nil {
		return Result{}, fmt.Errorf("ocr http encode: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(req.Context, http.MethodPost, p.config.Endpoint, bytes.NewReader(body))
	if err != nil {
		return Result{}, fmt.Errorf("ocr http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.config.Token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.config.Token)
	}
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return Result{}, fmt.Errorf("ocr http transport: %w", err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return Result{}, fmt.Errorf("ocr http read: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Result{}, fmt.Errorf("ocr http status %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	var result Result
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return Result{}, fmt.Errorf("ocr http decode: %w", err)
	}
	if err := validateResult(result); err != nil {
		return Result{}, err
	}
	return result, nil
}
