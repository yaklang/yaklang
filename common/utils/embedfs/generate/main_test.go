package main

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateReproducibilityAndFreshness(t *testing.T) {
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(old) })
	if err = os.Mkdir("assets", 0755); err != nil {
		t.Fatal(err)
	}
	source := []byte(strings.Repeat("compressible test resource\n", 100))
	if err = os.WriteFile("assets/large.txt", source, 0644); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile("assets/small.txt", []byte("small"), 0644); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile("assets/archive.gz", source, 0644); err != nil {
		t.Fatal(err)
	}
	run := func(check bool) error {
		return generate("assets", "FS", "resources_embed.go", []string{"assets"}, check, "")
	}
	if err = run(false); err != nil {
		t.Fatal(err)
	}
	if err = run(true); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("assets/large.txt" + suffix)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(reader)
	reader.Close()
	if err != nil || !bytes.Equal(data, source) {
		t.Fatalf("%v", err)
	}
	code, err := os.ReadFile("resources_embed.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(code), "//go:embed \"assets/large.txt\"") || !strings.Contains(string(code), "//go:embed \"assets/archive.gz\"") {
		t.Fatal(string(code))
	}
	if err = run(false); err != nil {
		t.Fatal(err)
	}
	again, _ := os.ReadFile("assets/large.txt" + suffix)
	if !bytes.Equal(raw, again) {
		t.Fatal("non-deterministic gzip")
	}
	if err = os.WriteFile("assets/large.txt", []byte("now small"), 0644); err != nil {
		t.Fatal(err)
	}
	if err = run(true); err == nil {
		t.Fatal("stale source accepted")
	}
	if err = run(false); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join("assets", "large.txt"+suffix)); !os.IsNotExist(err) {
		t.Fatal("stale sidecar retained")
	}
	if err = run(true); err != nil {
		t.Fatal(err)
	}
}
