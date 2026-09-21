package scannode

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
	"io"
	"net/http"
	"strings"
	"testing"
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
