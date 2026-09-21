package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"go/parser"
	"go/token"
	"io"
	"strconv"
	"strings"

	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/utils/gzip_embed"
)

func TestArchiveGeneration(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"data", "dataex"} {
		if err := os.Mkdir(filepath.Join(root, dir), 0755); err != nil {
			t.Fatal(err)
		}
	}
	sources := map[string][]byte{"data/plain.txt": bytes.Repeat([]byte("original resource\n"), 100), "data/existing.gz": {0x1f, 0x8b, 1, 2, 3}, "dataex/config.json": []byte(`{"enabled":true}`)}
	for name, data := range sources {
		if err := os.WriteFile(filepath.Join(root, name), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	archive := filepath.Join(root, "resources.tar.gz")
	pack := func() {
		t.Helper()
		if err := targz([]string{filepath.Join(root, "data"), filepath.Join(root, "dataex")}, root, archive, false, true); err != nil {
			t.Fatal(err)
		}
	}
	pack()
	first, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	for name := range sources {
		if err := os.Chtimes(filepath.Join(root, name), time.Now(), time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	pack()
	second, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("archive depends on source mtime")
	}
	reader, err := gzip.NewReader(bytes.NewReader(second))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	tr := tar.NewReader(reader)
	seen := 0
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		got, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		want, exists := sources[h.Name]
		if !exists || !bytes.Equal(got, want) {
			t.Fatalf("changed source content: %s", h.Name)
		}
		seen++
	}
	if seen != len(sources) {
		t.Fatalf("missing resources: %d", seen)
	}
	if err := targz([]string{filepath.Join(root, "dataex/config.json")}, "", archive, false, false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	one, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer one.Close()
	header, err := tar.NewReader(one).Next()
	if err != nil || header.Name != "config.json" {
		t.Fatalf("single file omitted: %v %v", header, err)
	}
	if err := targz([]string{filepath.Join(root, "missing")}, root, archive, false, false); err == nil {
		t.Fatal("missing source accepted")
	}
}

func TestArchiveCLIDefaultsAndOverrides(t *testing.T) {
	for _, tc := range []struct {
		name, key string
		args      []string
		cache     bool
	}{
		{"default", gzip_embed.DefaultXORKey, nil, true},
		{"plain", "", []string{"--xor-key=", "--cache=false"}, false},
		{"custom", "key-with-quotes-\"-and-backslash-\\", []string{"--xor-key=key-with-quotes-\"-and-backslash-\\"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "fixture")
			if err := os.Mkdir(root, 0755); err != nil {
				t.Fatal(err)
			}
			old, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			if err = os.Chdir(root); err != nil {
				t.Fatal(err)
			}
			defer os.Chdir(old)
			if err = os.WriteFile("file with spaces.txt", []byte("original content"), 0644); err != nil {
				t.Fatal(err)
			}
			args := append([]string{"gzip-embed", "--source", "file with spaces.txt", "--gz", "resources.tar.gz"}, tc.args...)
			if err = newApp().Run(args); err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile("resources.tar.gz")
			if err != nil {
				t.Fatal(err)
			}
			if tc.key != "" {
				if bytes.HasPrefix(raw, []byte{0x1f, 0x8b}) {
					t.Fatal("archive was not encoded")
				}
				raw = XORKeyStream(raw, []byte(tc.key))
			}
			r, err := gzip.NewReader(bytes.NewReader(raw))
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			tr := tar.NewReader(r)
			h, err := tr.Next()
			if err != nil || h.Name != "file with spaces.txt" {
				t.Fatalf("path mismatch: %v %v", h, err)
			}
			content, err := io.ReadAll(tr)
			if err != nil || string(content) != "original content" {
				t.Fatal("content changed")
			}
			code, err := os.ReadFile("embed.go")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = parser.ParseFile(token.NewFileSet(), "embed.go", code, parser.AllErrors); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(code), strconv.FormatBool(tc.cache)) {
				t.Fatal("cache flag lost")
			}
			if tc.key != "" && tc.key != gzip_embed.DefaultXORKey && !strings.Contains(string(code), strconv.Quote(tc.key)) {
				t.Fatal("custom decoder key lost")
			}
		})
	}
}
