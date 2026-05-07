package aispec

import (
	"bytes"
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
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hello from callback"}}],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`))
	}))
	defer server.Close()

	var gotRequest []byte
	var gotResponseHeader []byte
	var gotBodyPreview []byte
	var gotUsage *ChatUsage

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
		WithChatBase_RawHTTPRequestResponseCallback(func(requestBytes []byte, responseHeaderBytes []byte, bodyPreview []byte, usageInfo *ChatUsage) {
			gotRequest = append([]byte(nil), requestBytes...)
			gotResponseHeader = append([]byte(nil), responseHeaderBytes...)
			gotBodyPreview = append([]byte(nil), bodyPreview...)
			gotUsage = usageInfo
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
	if gotUsage == nil {
		t.Fatal("usage should be captured for non-stream response")
	}
	if gotUsage.PromptTokens != 11 || gotUsage.CompletionTokens != 7 || gotUsage.TotalTokens != 18 {
		t.Fatalf("unexpected usage captured: %+v", gotUsage)
	}
}
