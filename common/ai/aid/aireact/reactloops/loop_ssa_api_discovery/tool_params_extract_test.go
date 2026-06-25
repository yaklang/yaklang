package loop_ssa_api_discovery

import (
	"strings"
	"testing"
)

func TestAugmentDoHTTPParams_ContentTypeAlias(t *testing.T) {
	out, notes := augmentDoHTTPParams(aitoolInvoke("content_type", "application/x-www-form-urlencoded", "method", "POST", "body", "username=a&password=b"))
	if out["content-type"] != "application/x-www-form-urlencoded" {
		t.Fatalf("content-type=%v", out["content-type"])
	}
	if _, ok := out["content_type"]; ok {
		t.Fatal("content_type should be removed")
	}
	if len(notes) == 0 {
		t.Fatal("expected normalization notes")
	}
}

func TestAugmentDoHTTPParams_InferFormFromBody(t *testing.T) {
	out, notes := augmentDoHTTPParams(aitoolInvoke("method", "POST", "body", "username=admin&password=x&secureLogin=true"))
	if out["content-type"] != "application/x-www-form-urlencoded" {
		t.Fatalf("content-type=%v", out["content-type"])
	}
	if len(notes) == 0 {
		t.Fatal("expected inference notes")
	}
}

func TestAugmentDoHTTPParams_InferJSONFromBody(t *testing.T) {
	out, _ := augmentDoHTTPParams(aitoolInvoke("method", "POST", "body", `{"username":"admin","password":"x"}`))
	if out["content-type"] != "application/json" {
		t.Fatalf("content-type=%v", out["content-type"])
	}
}

func TestAugmentDoHTTPParams_PostParamsSetsContentType(t *testing.T) {
	out, _ := augmentDoHTTPParams(aitoolInvoke("method", "POST", "post-params", "username=a&password=b"))
	if out["content-type"] != "application/x-www-form-urlencoded" {
		t.Fatalf("content-type=%v", out["content-type"])
	}
}

func TestLooksLikeFormBody(t *testing.T) {
	if !looksLikeFormBody("username=a&password=b") {
		t.Fatal("expected form body")
	}
	if looksLikeFormBody(`{"a":1}`) {
		t.Fatal("json should not be form")
	}
}

func TestFormatDoHTTPParamNormalizationHint(t *testing.T) {
	h := formatDoHTTPParamNormalizationHint([]string{"normalized param \"content_type\" -> \"content-type\""})
	if !strings.Contains(h, "content-type") {
		t.Fatalf("hint=%q", h)
	}
}

func aitoolInvoke(kv ...string) map[string]any {
	m := make(map[string]any)
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i]] = kv[i+1]
	}
	return m
}
