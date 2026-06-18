package mcp

import "testing"

func TestCreateWebFuzzerTabToolRegistered(t *testing.T) {
	set, ok := globalToolSets["http_fuzzer"]
	if !ok {
		t.Fatalf("http_fuzzer tool set not registered")
	}
	if _, exists := set.Tools["create_web_fuzzer_tab"]; !exists {
		t.Fatalf("tool not registered: create_web_fuzzer_tab")
	}
	if _, exists := set.Tools["http_fuzzer"]; !exists {
		t.Fatalf("tool not registered: http_fuzzer")
	}
}
