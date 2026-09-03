package yaklib

import (
	"strings"
	"testing"
	"time"
)

func TestYAMLV3DynamicTypes(t *testing.T) {
	got, err := yamlUnmarshal([]byte("name: yak\n1: numeric-key\nwhen: 2026-09-01T08:30:00Z\n"))
	if err != nil {
		t.Fatal(err)
	}
	values, ok := got.(map[interface{}]interface{})
	if !ok {
		t.Fatalf("unexpected dynamic map type: %T", got)
	}
	if values["name"] != "yak" || values[1] != "numeric-key" {
		t.Fatalf("map key behavior changed: %#v", values)
	}
	if _, ok := values["when"].(time.Time); !ok {
		t.Fatalf("timestamp should use yaml.v3 time semantics, got %T", values["when"])
	}
}

func TestYAMLV3InlineAndStrictBehavior(t *testing.T) {
	type commonFields struct {
		Name string `yaml:"name"`
	}
	type document struct {
		commonFields `yaml:",inline"`
		Extra        map[string]any `yaml:",inline"`
	}

	raw, err := yamlMarshal(document{
		commonFields: commonFields{Name: "yak"},
		Extra:        map[string]any{"enabled": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, "name: yak") || !strings.Contains(text, "enabled: true") {
		t.Fatalf("inline fields missing:\n%s", text)
	}

	if _, err := yamlUnmarshalStrict([]byte("name: first\nname: second\n")); err == nil {
		t.Fatal("strict YAML decoding must reject duplicate keys")
	}
}
