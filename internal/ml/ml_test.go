package ml

import (
	"errors"
	"testing"
)

type testBackend struct {
	name   string
	caps   []Capability
	result Result
	err    error
}

func (b testBackend) Name() string                  { return b.name }
func (b testBackend) Capabilities() []Capability    { return b.caps }
func (b testBackend) Infer(Request) (Result, error) { return b.result, b.err }

func TestChainFallsBackAndValidates(t *testing.T) {
	chain := NewChain(
		testBackend{name: "native", caps: []Capability{CapabilityEmbedding}, err: errors.New("no model")},
		testBackend{name: "community", caps: []Capability{CapabilityEmbedding}, result: Result{Embedding: &Vector{Values: []float32{1}}}},
	)
	result, err := chain.Infer(Request{Capability: CapabilityEmbedding})
	if err != nil || result.Embedding == nil {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestUnavailableIsTerminalError(t *testing.T) {
	_, err := NewChain(Unavailable("terminal", CapabilitySemanticSearch)).Infer(Request{Capability: CapabilitySemanticSearch})
	if !errors.Is(err, ErrAllBackendsFailed) {
		t.Fatalf("err=%v", err)
	}
}
