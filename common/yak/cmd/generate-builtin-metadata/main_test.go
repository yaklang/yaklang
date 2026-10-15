package main

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"

	"github.com/yaklang/yaklang/common/utils/builtinmetadata"
	"os"
	"path/filepath"
	"testing"
)

// Compare with fresh analysis, not just source hashes: extractor/schema changes
// also require regeneration even when every .yak file remains unchanged.
func TestBuiltinMetadataIsCurrent(t *testing.T) {
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	want, err := generate(root)
	if err != nil {
		t.Fatal(err)
	}
	if builtinmetadata.Enabled {
		var c builtinmetadata.Catalog
		if err := json.Unmarshal(want, &c); err != nil {
			t.Fatal(err)
		}
		for key, entry := range c.Entries {
			if !strings.HasPrefix(key, "plugin:") {
				continue
			}
			parts := strings.SplitN(key, ":", 3)
			code, err := os.ReadFile(filepath.Join(root, "common/coreplugin/base-yak-plugin", parts[2]+".yak"))
			if err != nil {
				t.Fatal(err)
			}
			got, hit := builtinmetadata.Lookup(key, string(code))
			if !hit || !reflect.DeepEqual(got, entry.Metadata) {
				t.Fatalf("builtin plugin cache differs from fresh dynamic analysis: %s", key)
			}
		}
	}
	want, err = catalogSource(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, "common/consts/builtin-metadata-hash.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("builtin metadata is stale; run go generate ./common/utils/builtinmetadata")
	}
}
