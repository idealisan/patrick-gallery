package ocr

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPProcessor(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer token" {
			t.Fatalf("unexpected request: %s %s auth=%q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
		}
		var body struct {
			Name string `json:"name"`
			MIME string `json:"mime"`
			Data []byte `json:"data"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Name != "a.jpg" || body.MIME != "image/jpeg" || string(body.Data) != "image" {
			t.Fatalf("unexpected payload: %+v", body)
		}
		_ = json.NewEncoder(w).Encode(Result{Text: "hello", Words: []Word{{Text: "hello"}}})
	}))
	defer server.Close()

	processor, err := NewHTTP(HTTPConfig{Endpoint: server.URL, Token: "token"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := processor.Recognize(Request{Context: context.Background(), Data: []byte("image"), MIME: "image/jpeg", Name: "a.jpg"})
	if err != nil || result.Text != "hello" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestHTTPRejectsEmptyResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Result{})
	}))
	defer server.Close()
	processor, _ := NewHTTP(HTTPConfig{Endpoint: server.URL})
	_, err := processor.Recognize(Request{Data: []byte("image")})
	if !errors.Is(err, ErrInvalidResult) {
		t.Fatalf("err=%v, want ErrInvalidResult", err)
	}
}

func TestUnavailable(t *testing.T) {
	chain := NewChain()
	_, err := chain.Recognize(Request{})
	if !errors.Is(err, ErrAllBackendsFailed) {
		t.Fatalf("err=%v, want ErrAllBackendsFailed", err)
	}
}

func TestChainFallsBackInOrder(t *testing.T) {
	first := testProcessor{name: "first", err: errors.New("failed")}
	second := testProcessor{name: "second", result: Result{Text: "ok"}}
	chain := NewChain(first, nil, second)
	result, err := chain.Recognize(Request{})
	if err != nil || result.Text != "ok" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

type testProcessor struct {
	name   string
	result Result
	err    error
}

func (p testProcessor) Name() string                      { return p.name }
func (p testProcessor) Recognize(Request) (Result, error) { return p.result, p.err }
