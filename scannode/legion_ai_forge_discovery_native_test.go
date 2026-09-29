package scannode

import (
	"context"
	"errors"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
	"net"
	"strings"
	"sync"
	"testing"
)

func TestLegionForgeNativeSubdomainTool(t *testing.T) {
	release := &aiv1.ContextForgeRelease{CapabilityProfile: legionForgeDiscoveryProfileV2, Parameters: []*aiv1.ContextForgeParameter{{Key: "labels", ValueKind: "string", Value: "www,api"}, {Key: "target-host", ValueKind: "string", Value: "owned.example"}}}
	var mu sync.Mutex
	queries := []string{}
	lookup := func(ctx context.Context, host string) ([]net.IP, error) {
		mu.Lock()
		queries = append(queries, host)
		mu.Unlock()
		if host == "owned.example" || host == "www.owned.example" {
			return []net.IP{net.ParseIP("203.0.113.10")}, nil
		}
		if !strings.HasSuffix(host, ".owned.example") || strings.Count(host, ".") != 2 {
			t.Errorf("escaped scoped DNS: %s", host)
		}
		return nil, &net.DNSError{IsNotFound: true, Name: host, Err: "not found"}
	}
	opts, material, err := legionForgeDiscoveryOptionsWithLookup(context.Background(), release, lookup)
	if err != nil {
		t.Fatal(err)
	}
	cfg := aicommon.NewConfig(context.Background(), opts...)
	tool, err := cfg.GetAiToolManager().GetToolByName("dns_lookup")
	if err != nil {
		t.Fatal(err)
	}
	result, err := tool.InvokeWithParams(map[string]any{"runtime_id": "fixture"})
	if err != nil || !result.Success {
		t.Fatalf("invoke: %+v %v", result, err)
	}
	value := result.Data.(*aitool.ToolExecutionResult).Result.(map[string]any)
	if value["source"] != "native_subdomain_brute" || value["complete_dictionary"] != true || value["wildcard_probe_count"] != 2 {
		t.Fatal(value)
	}
	matches := value["matches"].([]map[string]any)
	if len(matches) != 1 || matches[0]["host"] != "www.owned.example" {
		t.Fatal(matches)
	}
	if len(queries) != 5 {
		t.Fatalf("expected root +2 wildcard +2 dictionary queries: %v", queries)
	}
	refs := material.applicationHTTPMaterialReferences()
	if len(refs) != 2 {
		t.Fatalf("expected actual positive and negative dictionary refs: %+v", refs)
	}
	for _, ref := range refs {
		if len(ref.Operations) != 2 || !strings.HasPrefix(ref.Operations[1], "label:") {
			t.Fatal(ref)
		}
	}
	// v1 never enumerates when label is absent.
	release.CapabilityProfile = legionForgeDiscoveryProfile
	opts, _, err = legionForgeDiscoveryOptionsWithLookup(context.Background(), release, lookup)
	if err != nil {
		t.Fatal(err)
	}
	cfg = aicommon.NewConfig(context.Background(), opts...)
	tool, _ = cfg.GetAiToolManager().GetToolByName("dns_lookup")
	result, err = tool.InvokeWithParams(map[string]any{"runtime_id": "v1"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Data.(*aitool.ToolExecutionResult).Result.(map[string]any)["source"] != "pinned_dns_resolution" {
		t.Fatal("v1 changed")
	}
}

func TestLegionNativeSubdomainFailsClosedOnResolverError(t *testing.T) {
	r, err := newLegionForgeDiscoveryRuntime(context.Background(), "owned.example", []int{80}, func(context.Context, string) ([]net.IP, error) { return []net.IP{net.ParseIP("203.0.113.10")}, nil })
	if err != nil {
		t.Fatal(err)
	}
	r.allowedLabels = map[string]struct{}{"www": {}}
	r.nativeEnumeration = true
	r.lookup = func(context.Context, string) ([]net.IP, error) { return nil, context.DeadlineExceeded }
	if _, err := r.execute(context.Background(), "dns_lookup", nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout became negative success: %v", err)
	}
	if len(r.applicationHTTPMaterialReferences()) != 0 {
		t.Fatal("failed queries emitted successful evidence")
	}
	r.lookup = func(context.Context, string) ([]net.IP, error) { return []net.IP{net.ParseIP("10.0.0.1")}, nil }
	if _, err := r.execute(context.Background(), "dns_lookup", nil); err == nil {
		t.Fatal("private DNS accepted")
	}
}

func TestLegionNativeDiscoveryProfileRequiresBoundedDictionary(t *testing.T) {
	r := testLegionContextForgeRelease(t)
	r.CapabilityProfile = legionForgeDiscoveryProfileV2
	r.DeclaredToolNames = append([]string(nil), legionForgeDiscoveryTools...)
	r.Parameters = []*aiv1.ContextForgeParameter{{Key: "labels", ValueKind: "string", Value: "www,api"}, {Key: "target-host", ValueKind: "string", Value: "owned.example"}}
	rehashLegionContextForgeRelease(t, r)
	if err := validateContextForgeRelease(r); err != nil {
		t.Fatal(err)
	}
	r.Parameters = r.Parameters[1:]
	rehashLegionContextForgeRelease(t, r)
	if err := validateContextForgeRelease(r); err == nil {
		t.Fatal("v2 accepted absent dictionary")
	}
	r.CapabilityProfile = legionForgeDiscoveryProfile
	rehashLegionContextForgeRelease(t, r)
	if err := validateContextForgeRelease(r); err != nil {
		t.Fatalf("v1 base-only compatibility changed: %v", err)
	}
}
