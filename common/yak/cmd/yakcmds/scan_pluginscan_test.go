package yakcmds

import (
	"os"
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
	content := "Alpha: one\n# comment\nBeta: two\n\n"
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

	result, err := collectCustomVars([]string{"Flag=cliValue"}, f.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["FileKey"] != "fileValue" {
		t.Fatalf("file var missing: %#v", result)
	}
	if result["Flag"] != "cliValue" {
		t.Fatalf("cli var missing: %#v", result)
	}
}
