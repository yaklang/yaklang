package embed

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompressedAssetsMatchSources(t *testing.T) {
	sources := map[string]bool{}
	for _, root := range []string{"data", "dataex"} {
		err := filepath.WalkDir(root, func(name string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			name = filepath.ToSlash(name)
			sources[name] = true
			want, err := os.ReadFile(name)
			if err != nil {
				return err
			}
			got, err := FS.ReadFile(name)
			if err != nil {
				return err
			}
			if !bytes.Equal(want, got) {
				t.Errorf("FS content differs from source: %s; regenerate release assets", name)
			}
			if strings.HasSuffix(name, ".gz") || strings.HasSuffix(name, ".gzip") {
				r, err := gzip.NewReader(bytes.NewReader(want))
				if err != nil {
					return err
				}
				want, err = io.ReadAll(r)
				r.Close()
				if err != nil {
					return err
				}
			}
			asset, err := Asset(name)
			if err != nil {
				return err
			}
			if !bytes.Equal(want, asset) {
				t.Errorf("Asset content differs: %s", name)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	err := fs.WalkDir(FS, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !sources[name] {
			t.Errorf("unexpected resource: %s", name)
		}
		delete(sources, name)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 0 {
		t.Fatalf("missing resources: %v", sources)
	}
	names, err := AssetDir("data/anti-crawler")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(names, ",") != "sannysoft.html,stealth.min.js" {
		t.Fatal(names)
	}
	if _, err := Asset("data/missing.gz"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing gzip error lost: %v", err)
	}
}
