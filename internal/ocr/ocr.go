// Package ocr defines the OCR boundary used by immich-go.
//
// Backends are deliberately isolated from HTTP handlers and asset storage so
// native, community, and remote OCR implementations can be added without
// changing the search/upload contract.
package ocr

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrUnavailable       = errors.New("ocr backend unavailable")
	ErrInvalidResult     = errors.New("ocr backend returned an invalid result")
	ErrAllBackendsFailed = errors.New("all ocr backends failed")
)

// Request is the backend-neutral OCR input. Data is copied by callers only
// when needed; backends must not retain it after Recognize returns.
type Request struct {
	Context  context.Context
	Data     []byte
	MIME     string
	Name     string
	Language string
}

// Word is one recognized text region. Coordinates are optional and expressed
// in source-image pixels. Confidence is in the [0,1] range when provided.
type Word struct {
	Text       string  `json:"text"`
	Confidence float32 `json:"confidence,omitempty"`
	X          int     `json:"x,omitempty"`
	Y          int     `json:"y,omitempty"`
	Width      int     `json:"width,omitempty"`
	Height     int     `json:"height,omitempty"`
}

// Result is normalized across all OCR providers.
type Result struct {
	Text  string `json:"text"`
	Words []Word `json:"words,omitempty"`
}

// Processor is the stable OCR backend contract.
type Processor interface {
	Name() string
	Recognize(Request) (Result, error)
}

func validateResult(result Result) error {
	if result.Text == "" && len(result.Words) == 0 {
		return ErrInvalidResult
	}
	return nil
}

// Chain tries real OCR providers in order. A nil provider means that the
// provider is not configured for this build and is skipped; no empty result is
// ever produced as a fallback.
type Chain struct {
	providers []Processor
}

func NewChain(providers ...Processor) *Chain {
	active := make([]Processor, 0, len(providers))
	for _, provider := range providers {
		if provider != nil {
			active = append(active, provider)
		}
	}
	return &Chain{providers: active}
}

func (c *Chain) Name() string {
	if len(c.providers) == 0 {
		return "ocr-chain(empty)"
	}
	names := make([]string, 0, len(c.providers))
	for _, provider := range c.providers {
		names = append(names, provider.Name())
	}
	return "ocr-chain(" + strings.Join(names, ",") + ")"
}

func (c *Chain) Recognize(req Request) (Result, error) {
	if len(c.providers) == 0 {
		return Result{}, fmt.Errorf("%w: no configured provider", ErrAllBackendsFailed)
	}
	errs := make([]error, 0, len(c.providers))
	for _, provider := range c.providers {
		result, err := provider.Recognize(req)
		if err == nil {
			return result, nil
		}
		errs = append(errs, fmt.Errorf("%s: %w", provider.Name(), err))
	}
	return Result{}, fmt.Errorf("%w: %w", ErrAllBackendsFailed, errors.Join(errs...))
}
