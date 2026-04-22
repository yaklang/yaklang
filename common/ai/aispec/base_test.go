package aispec

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

func TestChatBase_RawHTTPRequestResponseCallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body failed: %v", err)
		}
		if !bytes.Contains(body, []byte(`"model":"test-model"`)) {
			t.Fatalf("unexpected request body: %s", string(body))
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hello from callback"}}]}`))
	}))
	defer server.Close()

	var gotRequest []byte
	var gotResponseHeader []byte
	var gotBodyPreview []byte

	result, err := ChatBase(
		server.URL,
		"test-model",
		"hello",
		WithChatBase_DisableStream(true),
		WithChatBase_StreamHandler(func(reader io.Reader) {
			_, _ = io.Copy(io.Discard, reader)
		}),
		WithChatBase_PoCOptions(func() ([]poc.PocConfigOption, error) {
			return nil, nil
		}),
		WithChatBase_RawHTTPRequestResponseCallback(func(requestBytes []byte, responseHeaderBytes []byte, bodyPreview []byte) {
			gotRequest = append([]byte(nil), requestBytes...)
			gotResponseHeader = append([]byte(nil), responseHeaderBytes...)
			gotBodyPreview = append([]byte(nil), bodyPreview...)
		}),
	)
	if err != nil {
		t.Fatalf("chat base failed: %v", err)
	}

	if result != "hello from callback" {
		t.Fatalf("unexpected result: %s", result)
	}
	if !bytes.Contains(gotRequest, []byte("POST / HTTP/1.1")) {
		t.Fatalf("request callback not captured: %q", string(gotRequest))
	}
	if !bytes.Contains(gotRequest, []byte(`"messages":[{"role":"user","content":"hello"}]`)) {
		t.Fatalf("request payload not captured: %q", string(gotRequest))
	}
	if !bytes.Contains(gotResponseHeader, []byte("200 OK")) {
		t.Fatalf("response header not captured: %q", string(gotResponseHeader))
	}
	if !bytes.Contains(gotBodyPreview, []byte(`"hello from callback"`)) {
		t.Fatalf("response body preview not captured: %q", string(gotBodyPreview))
	}
}

func TestChatBase_RequestIncludesGenerationParameters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body failed: %v", err)
		}

		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("unmarshal request body failed: %v", err)
		}

		if payload["temperature"] != 0.25 {
			t.Fatalf("unexpected temperature: %v", payload["temperature"])
		}
		if payload["top_p"] != 0.9 {
			t.Fatalf("unexpected top_p: %v", payload["top_p"])
		}
		if payload["top_k"] != float64(12) {
			t.Fatalf("unexpected top_k: %v", payload["top_k"])
		}
		if payload["max_tokens"] != float64(256) {
			t.Fatalf("unexpected max_tokens: %v", payload["max_tokens"])
		}
		if payload["presence_penalty"] != 0.4 {
			t.Fatalf("unexpected presence_penalty: %v", payload["presence_penalty"])
		}
		if payload["frequency_penalty"] != 0.2 {
			t.Fatalf("unexpected frequency_penalty: %v", payload["frequency_penalty"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()

	result, err := ChatBase(
		server.URL,
		"test-model",
		"hello",
		WithChatBase_DisableStream(true),
		WithChatBase_StreamHandler(func(reader io.Reader) {
			_, _ = io.Copy(io.Discard, reader)
		}),
		WithChatBase_PoCOptions(func() ([]poc.PocConfigOption, error) {
			return nil, nil
		}),
		WithChatBase_Temperature(0.25),
		WithChatBase_TopP(0.9),
		WithChatBase_TopK(12),
		WithChatBase_MaxTokens(256),
		WithChatBase_PresencePenalty(0.4),
		WithChatBase_FrequencyPenalty(0.2),
	)
	if err != nil {
		t.Fatalf("chat base failed: %v", err)
	}
	if result != "ok" {
		t.Fatalf("unexpected result: %s", result)
	}
}
