package aihttp

import (
	"encoding/json"
	"testing"
)

func TestBuildListAiModelGRPCRequest_WithStructuredConfig(t *testing.T) {
	req, err := buildListAiModelGRPCRequest(map[string]any{
		"config": map[string]any{
			"Type":   "openai",
			"APIKey": "test-key",
			"Domain": "api.openai.com",
			"ExtraParams": []any{
				map[string]any{"Key": "proxy", "Value": "http://127.0.0.1:7890"},
				map[string]any{"Key": "no_https", "Value": "true"},
			},
		},
	})
	if err != nil {
		t.Fatalf("build request failed: %v", err)
	}

	var cfg map[string]any
	if err := json.Unmarshal([]byte(req.GetConfig()), &cfg); err != nil {
		t.Fatalf("unmarshal grpc config failed: %v", err)
	}
	if cfg["Type"] != "openai" {
		t.Fatalf("unexpected type: %v", cfg["Type"])
	}
	if cfg["api_key"] != "test-key" {
		t.Fatalf("unexpected api_key: %v", cfg["api_key"])
	}
	if cfg["proxy"] != "http://127.0.0.1:7890" {
		t.Fatalf("unexpected proxy: %v", cfg["proxy"])
	}
	if cfg["no_https"] != true {
		t.Fatalf("unexpected no_https: %v", cfg["no_https"])
	}
}

func TestBuildListAiModelGRPCRequest_WithLegacyStringType(t *testing.T) {
	req, err := buildListAiModelGRPCRequest(map[string]any{
		"Config": "openai",
	})
	if err != nil {
		t.Fatalf("build request failed: %v", err)
	}

	var cfg map[string]any
	if err := json.Unmarshal([]byte(req.GetConfig()), &cfg); err != nil {
		t.Fatalf("unmarshal grpc config failed: %v", err)
	}
	if cfg["Type"] != "openai" {
		t.Fatalf("unexpected type: %v", cfg["Type"])
	}
}

func TestBuildListAiModelGRPCRequest_WithLegacyJSONString(t *testing.T) {
	rawConfig := `{"Type":"openai","api_key":"legacy-key"}`
	req, err := buildListAiModelGRPCRequest(map[string]any{
		"Config": rawConfig,
	})
	if err != nil {
		t.Fatalf("build request failed: %v", err)
	}
	if req.GetConfig() != rawConfig {
		t.Fatalf("legacy json config should be preserved")
	}
}

func TestBuildListAiModelGRPCRequest_MissingConfig(t *testing.T) {
	_, err := buildListAiModelGRPCRequest(map[string]any{})
	if err == nil {
		t.Fatalf("expected error when config missing")
	}
}
