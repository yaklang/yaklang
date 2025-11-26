package yakcmds

import (
	"os"
	"reflect"
	"testing"
)

func TestParseKeyValue(t *testing.T) {
	key, value, err := parseKeyValue("A=B")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "A" || value != "B" {
		t.Fatalf("unexpected pair: %s=%s", key, value)
	}

	if _, _, err := parseKeyValue("invalid"); err == nil {
		t.Fatalf("expected error for invalid format")
	}
}

func TestLoadVarsFromFile(t *testing.T) {
	content := "Alpha: one\n# comment\nBeta: two\nCount: 10\nEnabled: true\nMeta: {\"foo\": \"bar\"}\n\n"
	f, err := os.CreateTemp("", "vars.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()

	vars, err := loadVarsFromFile(f.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vars["Alpha"] != "one" || vars["Beta"] != "two" {
		t.Fatalf("unexpected vars: %#v", vars)
	}
	if _, ok := vars["Count"].(int); !ok {
		t.Fatalf("expected Count to be int, got %#v", vars["Count"])
	}
	if enabled, ok := vars["Enabled"].(bool); !ok || !enabled {
		t.Fatalf("expected Enabled to be true, got %#v", vars["Enabled"])
	}
	if meta, ok := vars["Meta"].(map[string]interface{}); !ok || meta["foo"] != "bar" {
		t.Fatalf("expected Meta map, got %#v", vars["Meta"])
	}
}

func TestCollectCustomVars(t *testing.T) {
	content := "FileKey: fileValue\n"
	f, err := os.CreateTemp("", "vars.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()

	result, err := collectCustomVars([]string{"Flag=123", "Threshold=1.5"}, f.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["FileKey"] != "fileValue" {
		t.Fatalf("file var missing: %#v", result)
	}
	if flag, ok := result["Flag"].(int); !ok || flag != 123 {
		t.Fatalf("cli int var missing: %#v", result["Flag"])
	}
	if threshold, ok := result["Threshold"].(float64); !ok || threshold != 1.5 {
		t.Fatalf("cli float var missing: %#v", result["Threshold"])
	}
}

func TestParseVarValue(t *testing.T) {
	tests := []struct {
		input  string
		expect any
	}{
		{"true", true},
		{"false", false},
		{"10", 10},
		{"3.14", 3.14},
		{"{\"k\":1}", map[string]any{"k": float64(1)}},
		{"plain", "plain"},
	}
	for _, tt := range tests {
		got := parseVarValue(tt.input)
		if !reflect.DeepEqual(got, tt.expect) {
			t.Fatalf("unexpected value for %s: got %#v want %#v", tt.input, got, tt.expect)
		}
	}
}
