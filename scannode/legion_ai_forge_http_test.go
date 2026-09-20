package scannode

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"

	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
)

func TestLegionForgeHTTPAddressPolicy(t *testing.T) {
	publicLookup := func(context.Context, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("203.0.113.10")}, nil
	}
	privateLookup := func(context.Context, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("10.0.0.8")}, nil
	}

	if _, err := newLegionForgeHTTPRuntime(context.Background(), "http://10.0.0.8:8080/owned", publicLookup); err != nil {
		t.Fatalf("explicit private target must remain available: %v", err)
	}
	for name, target := range map[string]string{
		"loopback":       "http://127.0.0.1:8080/",
		"link local":     "http://169.254.169.254/latest/meta-data",
		"cloud metadata": "http://100.100.100.200/latest/meta-data",
		"container host": "http://host.docker.internal/",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := newLegionForgeHTTPRuntime(context.Background(), target, publicLookup); err == nil {
				t.Fatalf("target %q unexpectedly passed", target)
			}
		})
	}
	if _, err := newLegionForgeHTTPRuntime(context.Background(), "https://example.test/", privateLookup); err == nil || !strings.Contains(err.Error(), "address") {
		t.Fatalf("hostname-to-private resolution error = %v", err)
	}

	runtimeValue, err := newLegionForgeHTTPRuntime(context.Background(), "https://example.test/", publicLookup)
	if err != nil {
		t.Fatal(err)
	}
	transport, ok := runtimeValue.client.Transport.(*http.Transport)
	if !ok || transport.DialContext == nil {
		t.Fatalf("HTTP transport is not address-pinned: %#v", runtimeValue.client.Transport)
	}
	if _, err := transport.DialContext(context.Background(), "tcp", "other.test:443"); err == nil || !strings.Contains(err.Error(), "escaped") {
		t.Fatalf("cross-origin dial error = %v", err)
	}
}

func TestLegionForgeHTTPRequestBoundaryAndCancellation(t *testing.T) {
	authorized, err := normalizeServerFocusURL("https://owned.example/start")
	if err != nil {
		t.Fatal(err)
	}
	requestSeen := false
	runtimeValue := &legionServerFocusRuntime{
		ctx: context.Background(), authorized: authorized,
		client: &http.Client{Transport: aiRuntimeRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			requestSeen = true
			return &http.Response{
				StatusCode: http.StatusOK, Status: "200 OK", Header: http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(`{"finding":"owned evidence"}`)), Request: request,
			}, nil
		})},
	}
	result, err := runtimeValue.executeHTTPRequestContext(context.Background(), map[string]any{
		"url": "https://owned.example/evidence", "method": "GET",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !requestSeen || result["status_code"] != http.StatusOK || !strings.Contains(result["body"].(string), "owned evidence") {
		t.Fatalf("controlled request did not return real evidence: %#v", result)
	}
	release := &aiv1.ContextForgeRelease{
		CapabilityProfile: legionForgeHTTPProfile,
		Parameters:        []*aiv1.ContextForgeParameter{{Key: "target-url", Value: "https://owned.example/start", ValueKind: "string"}},
	}
	refs, err := aiApplicationMaterialReferences(release, aiSessionBinding{}, runtimeValue)
	if err != nil || len(refs) != 2 || refs[1].Kind != "http_request" || refs[1].SHA256 != result["body_sha256"] ||
		len(refs[1].Operations) != 2 || refs[1].Operations[0] != "get" || refs[1].Operations[1] != "status:200" {
		t.Fatalf("bounded request provenance was not retained: refs=%#v err=%v", refs, err)
	}
	if _, err := aiApplicationMaterialReferences(release, aiSessionBinding{}, &legionServerFocusRuntime{}); err == nil || !strings.Contains(err.Error(), "no bounded request evidence") {
		t.Fatalf("HTTP result without request evidence was accepted: %v", err)
	}
	for name, params := range map[string]map[string]any{
		"cross origin": {"url": "https://other.example/evidence", "method": "GET"},
		"write method": {"url": "https://owned.example/evidence", "method": "POST"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := runtimeValue.executeHTTPRequestContext(context.Background(), params); err == nil {
				t.Fatalf("request unexpectedly passed: %#v", params)
			}
		})
	}

	cancelCtx, cancel := context.WithCancel(context.Background())
	cancelRuntime := &legionServerFocusRuntime{
		ctx: context.Background(), authorized: authorized,
		client: &http.Client{Transport: aiRuntimeRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			cancel()
			<-request.Context().Done()
			return nil, request.Context().Err()
		})},
	}
	if _, err := cancelRuntime.executeHTTPRequestContext(cancelCtx, map[string]any{"method": "GET"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled request error = %v", err)
	}
}

func TestLegionForgeHTTPClientDisablesRedirects(t *testing.T) {
	lookup := func(context.Context, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("203.0.113.10")}, nil
	}
	runtimeValue, err := newLegionForgeHTTPRuntime(context.Background(), "https://owned.example/", lookup)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtimeValue.client.CheckRedirect(&http.Request{}, nil); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("redirect policy error = %v", err)
	}
}

func TestLegionForgeHTTPAddressPolicyRejectsMixedDNSAnswers(t *testing.T) {
	lookup := func(context.Context, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("203.0.113.10"), net.ParseIP("10.0.0.8")}, nil
	}
	if _, err := newLegionForgeHTTPRuntime(context.Background(), "https://example.test/", lookup); err == nil {
		t.Fatal("mixed public/private DNS answers must fail closed")
	}
}
