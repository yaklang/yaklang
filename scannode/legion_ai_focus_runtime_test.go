package scannode

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/schema"
)

type recordingServerFocusSink struct {
	assets []aiFocusAssetResult
	risks  []*schema.Risk
}

func (s *recordingServerFocusSink) SubmitAsset(
	_ context.Context,
	asset aiFocusAssetResult,
) (aiFocusResultReceipt, error) {
	s.assets = append(s.assets, asset)
	return aiFocusResultReceipt{ResultID: fmt.Sprintf("asset-%d", len(s.assets))}, nil
}

func (s *recordingServerFocusSink) SubmitRisk(
	_ context.Context,
	risk *schema.Risk,
) (aiFocusResultReceipt, error) {
	s.risks = append(s.risks, risk)
	return aiFocusResultReceipt{ResultID: fmt.Sprintf("risk-%d", len(s.risks))}, nil
}

func TestLegionServerFocusRuntimeExecutesBoundedCapabilities(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/start":
			response.Header().Set("Content-Type", "text/html")
			_, _ = response.Write([]byte(`<a href="/api/users">users</a><script src="/app.js"></script><a href="https://outside.test/api">outside</a>`))
		case "/app.js":
			response.Header().Set("Content-Type", "application/javascript")
			_, _ = response.Write([]byte(`const endpoint = "/api/orders";`))
		default:
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{"ok":true}`))
		}
	}))
	defer server.Close()

	sink := &recordingServerFocusSink{}
	runtimeValue, err := newLegionServerFocusRuntime(context.Background(), server.URL+"/start", sink)
	if err != nil {
		t.Fatalf("new focus runtime: %v", err)
	}
	runtime := runtimeValue.(*legionServerFocusRuntime)

	seed, err := runtime.Execute(serverFocusCapabilityHTTPRequest, map[string]any{
		"headers": map[string]any{"Accept-Language": "en-US"},
	})
	if err != nil {
		t.Fatalf("request seed: %v", err)
	}
	if seed["status_code"] != http.StatusOK || !strings.Contains(seed["body"].(string), "/api/users") {
		t.Fatalf("unexpected seed response: %#v", seed)
	}

	refs, err := runtime.Execute(serverFocusCapabilityExtractReferences, map[string]any{
		"base_url": seed["url"],
		"document": seed["body"],
	})
	if err != nil {
		t.Fatalf("extract references: %v", err)
	}
	pages := refs["pages"].([]string)
	scripts := refs["scripts"].([]string)
	if len(pages) != 1 || pages[0] != server.URL+"/api/users" {
		t.Fatalf("unexpected pages: %#v", pages)
	}
	if len(scripts) != 1 || scripts[0] != server.URL+"/app.js" {
		t.Fatalf("unexpected scripts: %#v", scripts)
	}

	receipt, err := runtime.Execute(serverFocusCapabilitySubmitAsset, map[string]any{
		"kind":         "http_endpoint",
		"title":        "Discovered endpoint",
		"target":       server.URL + "/api/users",
		"identity_key": "http_endpoint:" + server.URL + "/api/users",
		"payload":      map[string]any{"status": "responded"},
	})
	if err != nil {
		t.Fatalf("submit asset: %v", err)
	}
	if receipt["result_id"] != "asset-1" || len(sink.assets) != 1 {
		t.Fatalf("unexpected asset receipt: %#v", receipt)
	}
}

func TestLegionServerFocusRuntimeRejectsBoundaryViolations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte("ok"))
	}))
	defer server.Close()

	sink := &recordingServerFocusSink{}
	runtimeValue, err := newLegionServerFocusRuntime(context.Background(), server.URL+"/", sink)
	if err != nil {
		t.Fatalf("new focus runtime: %v", err)
	}
	runtime := runtimeValue.(*legionServerFocusRuntime)

	if _, err := runtime.Execute(serverFocusCapabilityHTTPRequest, map[string]any{"url": "https://outside.test/"}); err == nil || !strings.Contains(err.Error(), "outside the authorized origin") {
		t.Fatalf("expected same-origin rejection, got %v", err)
	}
	if _, err := runtime.Execute(serverFocusCapabilityHTTPRequest, map[string]any{"headers": map[string]any{"Authorization": "secret"}}); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("expected header rejection, got %v", err)
	}
	if _, err := runtime.Execute(serverFocusCapabilitySubmitRisk, map[string]any{
		"verified":          false,
		"target":            server.URL,
		"title":             "unverified",
		"risk_type":         "test",
		"request_evidence":  "GET / HTTP/1.1",
		"response_evidence": "HTTP/1.1 200 OK",
	}); err == nil || !strings.Contains(err.Error(), "verified evidence") {
		t.Fatalf("expected evidence gate, got %v", err)
	}

	runtime.mu.Lock()
	runtime.requestCount = maxServerFocusRequests
	runtime.mu.Unlock()
	if _, err := runtime.Execute(serverFocusCapabilityHTTPRequest, nil); err == nil || !strings.Contains(err.Error(), "budget exhausted") {
		t.Fatalf("expected budget rejection, got %v", err)
	}
}
