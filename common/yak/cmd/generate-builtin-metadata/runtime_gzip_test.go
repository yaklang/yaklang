//go:build gzip_embed

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils/builtinmetadata"
)

func TestBuiltinGzipMetadataMatchesArchives(t *testing.T) {
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	for _, spec := range resourceSpecs {
		t.Run(spec.kind, func(t *testing.T) {
			data, expected, err := consts.BuiltinMetadata(spec.kind)
			if err != nil {
				t.Fatal(err)
			}
			bundle, err := builtinmetadata.Decode(data, spec.kind, expected)
			if err != nil {
				t.Fatal(err)
			}
			var archivePath, prefix string
			switch spec.kind {
			case "aitool":
				archivePath = "common/ai/aid/aitool/buildinaitools/yakscripttools/yakscriptforai.tar.gz"
			case "coreplugin":
				archivePath = "common/coreplugin/base-yak-plugin.tar.gz"
				prefix = "base-yak-plugin"
			case "forge":
				archivePath = "common/ai/aiforge/buildinforge.tar.gz"
				prefix = "buildinforge"
			case "syntaxflow":
				archivePath = "common/syntaxflow/sfbuildin/buildin.tar.gz"
				prefix = "buildin"
			}
			bytes, err := os.ReadFile(filepath.Join(root, archivePath))
			if err != nil {
				t.Fatal(err)
			}
			builtinmetadata.Register(spec.kind, prefix, func() ([]byte, error) { return bytes, nil })
			got, err := builtinmetadata.Hash(spec.kind)
			if err != nil {
				t.Fatal(err)
			}
			if got != expected {
				t.Fatal("sync hash must be exactly the compressed metadata hash")
			}
			for key, entry := range bundle.Entries {
				source, err := os.ReadFile(filepath.Join(root, spec.dir, filepath.FromSlash(entry.Script)))
				if err != nil {
					t.Fatal(err)
				}
				if _, hit := builtinmetadata.Lookup(key, string(source)); !hit {
					t.Fatalf("missing source-bound metadata: %s", key)
				}
			}
		})
	}
}
