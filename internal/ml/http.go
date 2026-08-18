package ml

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

type HTTPConfig struct {
	Endpoint string
	Token    string
	Timeout  time.Duration
}

type HTTPBackend struct {
	config HTTPConfig
	client *http.Client
}

func NewHTTP(config HTTPConfig) (*HTTPBackend, error) {
	if strings.TrimSpace(config.Endpoint) == "" {
		return nil, fmt.Errorf("ml http endpoint is required")
	}
	if config.Timeout <= 0 {
		config.Timeout = 120 * time.Second
	}
	return &HTTPBackend{config: config, client: &http.Client{Timeout: config.Timeout}}, nil
}

func (b *HTTPBackend) Name() string { return "http" }

func (b *HTTPBackend) Capabilities() []Capability {
	return []Capability{
		CapabilityEmbedding, CapabilitySemanticSearch, CapabilityFaceDetection,
		CapabilityFaceEmbedding, CapabilityObjectDetection, CapabilityClassification,
	}
}

func (b *HTTPBackend) Infer(req Request) (Result, error) {
	ctx := req.Context
	if ctx == nil {
		ctx = context.Background()
	}
	payload := struct {
		Capability Capability `json:"capability"`
		Name       string     `json:"name"`
		MIME       string     `json:"mime"`
		Data       []byte     `json:"data,omitempty"`
		Query      string     `json:"query,omitempty"`
		Language   string     `json:"language,omitempty"`
	}{req.Capability, req.Name, req.MIME, req.Data, req.Query, req.Language}
	body, err := json.Marshal(payload)
	if err != nil {
		return Result{}, fmt.Errorf("ml http encode: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, b.config.Endpoint, bytes.NewReader(body))
	if err != nil {
		return Result{}, fmt.Errorf("ml http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if b.config.Token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+b.config.Token)
	}
	resp, err := b.client.Do(httpReq)
	if err != nil {
		return Result{}, fmt.Errorf("ml http transport: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return Result{}, fmt.Errorf("ml http read: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Result{}, fmt.Errorf("ml http status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var result Result
	if err := json.Unmarshal(data, &result); err != nil {
		return Result{}, fmt.Errorf("ml http decode: %w", err)
	}
	return result, nil
}
