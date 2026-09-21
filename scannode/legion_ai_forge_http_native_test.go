package scannode

import (
	"context"
	"errors"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestLegionNativeHTTPToolsUseGuardedEvidence(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	release := &aiv1.ContextForgeRelease{CapabilityProfile: legionForgeHTTPProfileV2, Parameters: []*aiv1.ContextForgeParameter{{Key: "target-url", ValueKind: "string", Value: "http://10.0.0.8/"}}}
	opts, runtime, err := legionForgeHTTPOptions(ctx, release)
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{}
	runtime.client = &http.Client{Transport: aiRuntimeRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "10.0.0.8" || req.Method != "GET" {
			t.Fatalf("escaped egress: %s %s", req.Method, req.URL)
		}
		paths = append(paths, req.URL.Path)
		body := `<html>/AV732E/setup.exe <a href="/child">child</a><a href="http://other.example/">external</a><a href="http://127.0.0.1/">loopback</a></html>`
		if req.URL.Path == "/child" {
			body = `actual child marker <a href="/grandchild">depth two</a>`
		}
		return &http.Response{StatusCode: 200, Status: "200 OK", Header: http.Header{"Content-Type": []string{"text/html"}}, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
	})}
	cfg := aicommon.NewConfig(ctx, opts...)
	fp, _ := cfg.GetAiToolManager().GetToolByName("web_fingerprint")
	result, err := fp.InvokeWithParams(map[string]any{"runtime_id": "native-fp"}, aitool.WithContext(ctx))
	if err != nil || !result.Success {
		t.Fatalf("fp: %+v %v", result, err)
	}
	value := result.Data.(*aitool.ToolExecutionResult).Result.(map[string]any)
	products := value["fingerprints"].(map[string]any)["products"].([]string)
	found := false
	for _, p := range products {
		if p == "AVTech-Video-Web-Server" {
			found = true
		}
	}
	if !found {
		t.Fatalf("native embedded rule did not match fixture: %v", products)
	}
	crawler, _ := cfg.GetAiToolManager().GetToolByName("simple_crawler")
	result, err = crawler.InvokeWithParams(map[string]any{"runtime_id": "native-crawl"}, aitool.WithContext(ctx))
	if err != nil || !result.Success {
		t.Fatalf("crawl: %+v %v", result, err)
	}
	value = result.Data.(*aitool.ToolExecutionResult).Result.(map[string]any)
	pages := value["pages"].([]map[string]any)
	if len(pages) != 2 || !strings.Contains(pages[1]["body"].(string), "actual child marker") {
		t.Fatalf("not actually crawled: %+v", value)
	}
	for _, path := range paths {
		if path == "/grandchild" {
			t.Fatal("crawler exceeded depth")
		}
	}
	if len(runtime.applicationHTTPMaterialReferences()) != 3 {
		t.Fatal("missing per-request evidence")
	}
}

type legionNativeCancelOnCloseBody struct {
	io.Reader
	cancel context.CancelFunc
}

func (b legionNativeCancelOnCloseBody) Close() error { b.cancel(); return nil }

func TestLegionNativeFingerprintFailureDoesNotPublishRequestEvidence(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	release := &aiv1.ContextForgeRelease{CapabilityProfile: legionForgeHTTPProfileV2, Parameters: []*aiv1.ContextForgeParameter{{Key: "target-url", ValueKind: "string", Value: "http://10.0.0.8/"}}}
	opts, runtime, err := legionForgeHTTPOptions(ctx, release)
	if err != nil {
		t.Fatal(err)
	}
	runtime.client = &http.Client{Transport: aiRuntimeRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Status: "200 OK", Header: http.Header{}, Body: legionNativeCancelOnCloseBody{Reader: strings.NewReader("/AV732E/setup.exe"), cancel: cancel}, Request: req}, nil
	})}
	cfg := aicommon.NewConfig(context.Background(), opts...)
	tool, err := cfg.GetAiToolManager().GetToolByName("web_fingerprint")
	if err != nil {
		t.Fatal(err)
	}
	_, err = tool.InvokeWithParams(map[string]any{"runtime_id": "failed-fingerprint"})
	if err == nil {
		t.Fatal("canceled postprocessing unexpectedly succeeded")
	}
	if len(runtime.applicationHTTPMaterialReferences()) != 0 {
		t.Fatal("failed fingerprint published fetched response evidence")
	}
	if runtime.requestCount != 1 {
		t.Fatalf("failed tool must still consume request budget: %d", runtime.requestCount)
	}
	if _, err := aiApplicationMaterialReferences(release, aiSessionBinding{}, runtime); err == nil {
		t.Fatal("HTTP v2 final gate accepted no successful tool evidence")
	}
}

func TestLegionNativeCrawlerTimeoutPreservesConcurrentSuccessfulEvidence(t *testing.T) {
	authorized, err := normalizeServerFocusURL("http://10.0.0.8/")
	if err != nil {
		t.Fatal(err)
	}
	childStarted := make(chan struct{})
	runtime := &legionServerFocusRuntime{ctx: context.Background(), authorized: authorized}
	runtime.client = &http.Client{Transport: aiRuntimeRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/child" {
			close(childStarted)
			<-req.Context().Done()
			return nil, req.Context().Err()
		}
		body := "independent successful observation"
		if req.URL.Path == "/crawl" {
			body = `<a href="/child">child</a>`
		}
		return &http.Response{StatusCode: 200, Status: "200 OK", Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
	})}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	failed := make(chan error, 1)
	go func() {
		_, err := legionForgeNativeHTTPCall(ctx, context.Background(), runtime, "simple_crawler", map[string]any{"url": "http://10.0.0.8/crawl"})
		failed <- err
	}()
	select {
	case <-childStarted:
	case <-ctx.Done():
		t.Fatal("crawler did not reach child")
	}
	success, err := legionForgeNativeHTTPCall(context.Background(), context.Background(), runtime, "do_http_request", map[string]any{"url": "http://10.0.0.8/success"})
	if err != nil {
		t.Fatal(err)
	}
	if err := <-failed; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected child timeout: %v", err)
	}
	refs := runtime.applicationHTTPMaterialReferences()
	if len(refs) != 1 || refs[0].SHA256 != success["body_sha256"] {
		t.Fatalf("failed call leaked evidence or removed concurrent success: %+v", refs)
	}
	if runtime.requestCount != 3 {
		t.Fatalf("request budget refunded: %d", runtime.requestCount)
	}
}
