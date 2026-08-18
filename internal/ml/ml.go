// Package ml defines the backend boundary for machine-learning features.
//
// ML is intentionally capability-based: semantic search, face recognition,
// object detection, and classification can be supplied by different native,
// community, or remote providers. No handler depends on a model runtime.
package ml

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type Capability string

const (
	CapabilityEmbedding      Capability = "embedding"
	CapabilitySemanticSearch Capability = "semantic-search"
	CapabilityFaceDetection Capability = "face-detection"
	CapabilityFaceEmbedding Capability = "face-embedding"
	CapabilityObjectDetection Capability = "object-detection"
	CapabilityClassification Capability = "classification"
)

var (
	ErrUnavailable = errors.New("ml backend unavailable")
	ErrUnsupported = errors.New("ml capability unsupported")
	ErrInvalidResult = errors.New("ml backend returned an invalid result")
	ErrAllBackendsFailed = errors.New("all ml backends failed")
)

type Request struct {
	Context    context.Context
	Capability Capability
	Data       []byte
	MIME       string
	Name       string
	Query      string
	Language   string
}

type Vector struct {
	Values []float32 `json:"values"`
}

type Face struct {
	PersonID   string    `json:"personId,omitempty"`
	Confidence float32   `json:"confidence,omitempty"`
	X          int       `json:"x,omitempty"`
	Y          int       `json:"y,omitempty"`
	Width      int       `json:"width,omitempty"`
	Height     int       `json:"height,omitempty"`
	Embedding  *Vector   `json:"embedding,omitempty"`
}

type Detection struct {
	Label      string  `json:"label"`
	Confidence float32 `json:"confidence,omitempty"`
	X          int     `json:"x,omitempty"`
	Y          int     `json:"y,omitempty"`
	Width      int     `json:"width,omitempty"`
	Height     int     `json:"height,omitempty"`
}

type Result struct {
	Embedding   *Vector     `json:"embedding,omitempty"`
	Faces       []Face      `json:"faces,omitempty"`
	Detections  []Detection `json:"detections,omitempty"`
	Labels      []string    `json:"labels,omitempty"`
	Score       float32     `json:"score,omitempty"`
}

type Backend interface {
	Name() string
	Capabilities() []Capability
	Infer(Request) (Result, error)
}

func supports(backend Backend, capability Capability) bool {
	for _, item := range backend.Capabilities() {
		if item == capability {
			return true
		}
	}
	return false
}

func validateResult(capability Capability, result Result) error {
	switch capability {
	case CapabilityEmbedding, CapabilityFaceEmbedding:
		if result.Embedding == nil || len(result.Embedding.Values) == 0 {
			return ErrInvalidResult
		}
	case CapabilitySemanticSearch:
		// Search results may legitimately be empty; the backend must still
		// return a valid Result and the handler owns result ranking semantics.
	case CapabilityFaceDetection:
		if result.Faces == nil {
			return ErrInvalidResult
		}
	case CapabilityObjectDetection:
		if result.Detections == nil {
			return ErrInvalidResult
		}
	case CapabilityClassification:
		if result.Labels == nil {
			return ErrInvalidResult
		}
	}
	return nil
}

type Chain struct{ backends []Backend }

func NewChain(backends ...Backend) *Chain {
	active := make([]Backend, 0, len(backends))
	for _, backend := range backends {
		if backend != nil {
			active = append(active, backend)
		}
	}
	return &Chain{backends: active}
}

func (c *Chain) Name() string {
	if len(c.backends) == 0 {
		return "ml-chain(empty)"
	}
	names := make([]string, 0, len(c.backends))
	for _, backend := range c.backends {
		names = append(names, backend.Name())
	}
	return "ml-chain(" + strings.Join(names, ",") + ")"
}

func (c *Chain) Capabilities() []Capability {
	seen := map[Capability]bool{}
	var result []Capability
	for _, backend := range c.backends {
		for _, capability := range backend.Capabilities() {
			if !seen[capability] {
				seen[capability] = true
				result = append(result, capability)
			}
		}
	}
	return result
}

func (c *Chain) Infer(req Request) (Result, error) {
	if len(c.backends) == 0 {
		return Result{}, fmt.Errorf("%w: capability=%s", ErrAllBackendsFailed, req.Capability)
	}
	var errs []error
	for _, backend := range c.backends {
		if !supports(backend, req.Capability) {
			continue
		}
		result, err := backend.Infer(req)
		if err == nil {
			if err = validateResult(req.Capability, result); err == nil {
				return result, nil
			}
		}
		errs = append(errs, fmt.Errorf("%s: %w", backend.Name(), err))
	}
	if len(errs) == 0 {
		return Result{}, fmt.Errorf("%w: capability=%s", ErrUnsupported, req.Capability)
	}
	return Result{}, fmt.Errorf("%w: %w", ErrAllBackendsFailed, errors.Join(errs...))
}

type unavailable struct{ name string; capabilities []Capability }

func (b unavailable) Name() string { return b.name }
func (b unavailable) Capabilities() []Capability { return b.capabilities }
func (b unavailable) Infer(Request) (Result, error) { return Result{}, ErrUnavailable }

// Unavailable is the explicit terminal backend. It is allowed only as the
// last chain member and never represents a successful empty ML result.
func Unavailable(name string, capabilities ...Capability) Backend {
	return unavailable{name: name, capabilities: capabilities}
}
