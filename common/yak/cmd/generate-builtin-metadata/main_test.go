package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/utils/builtinmetadata"
)

func TestUnchangedSetsSkipAnalysisAndWrites(t *testing.T) {
	for _, kind := range []string{"aitool", "coreplugin", "forge", "syntaxflow"} {
		t.Run(kind, func(t *testing.T) {
			source, output := t.TempDir(), t.TempDir()
			constant := "ExpectedHash"
			hashPath := filepath.Join(output, kind+"-hash.go")
			if err := os.WriteFile(hashPath, []byte("package consts\nconst "+constant+" string = \"initial\"\n"), 0644); err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(source, "demo.yak")
			if err := os.WriteFile(file, []byte("old"), 0644); err != nil {
				t.Fatal(err)
			}
			calls := 0
			build := func(files map[string]builtinmetadata.File) (*builtinmetadata.Bundle, error) {
				calls++
				return &builtinmetadata.Bundle{Version: builtinmetadata.Version, Kind: kind, Recipe: "recipe", Files: files, SourceHash: builtinmetadata.Fingerprint(files, "recipe"), Entries: map[string]builtinmetadata.Entry{}}, nil
			}
			run := func(want bool) {
				t.Helper()
				changed, err := updateSet(source, output, kind, constant, "recipe", false, false, build)
				if err != nil {
					t.Fatal(err)
				}
				if changed != want {
					t.Fatalf("changed=%v want=%v", changed, want)
				}
			}
			run(true)
			artifact := filepath.Join(output, kind+"-metadata.json")
			before, _ := os.ReadFile(artifact)
			stat, _ := os.Stat(artifact)
			run(false)
			after, _ := os.ReadFile(artifact)
			stat2, _ := os.Stat(artifact)
			if calls != 1 || !bytes.Equal(before, after) || !stat.ModTime().Equal(stat2.ModTime()) {
				t.Fatal("unchanged metadata was analyzed or rewritten")
			}
			packed, err := os.ReadFile(artifact + ".br")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(artifact + ".br"); err != nil {
				t.Fatal(err)
			}
			// A fresh checkout has only text. Restore the ignored build artifact
			// without invoking SSA/AST analysis or rewriting the tracked files.
			run(false)
			restored, err := os.ReadFile(artifact + ".br")
			if err != nil || !bytes.Equal(packed, restored) || calls != 1 {
				t.Fatal("packing repeated analysis or changed the artifact", err)
			}
			// A check-only run must also avoid the parser.
			if changed, err := updateSet(source, output, kind, constant, "recipe", true, false, build); err != nil || changed {
				t.Fatal("fresh check must only verify", err)
			}
			info, _ := os.Stat(file)
			_ = os.WriteFile(file, []byte("new"), 0644)
			_ = os.Chtimes(file, info.ModTime(), info.ModTime())
			run(true)
			_ = os.WriteFile(filepath.Join(source, "extra.yak"), []byte("extra"), 0644)
			run(true)
			_ = os.Remove(filepath.Join(source, "extra.yak"))
			run(true)
			_ = os.Rename(file, filepath.Join(source, "renamed.yak"))
			file = filepath.Join(source, "renamed.yak")
			run(true)
			if runtime.GOOS != "windows" {
				if err := os.Chmod(file, 0755); err != nil {
					t.Fatal(err)
				}
				run(true)
				if err := os.Chmod(source, 0700); err != nil {
					t.Fatal(err)
				}
				run(true)
			}
			// Corrupt readable metadata; source hashes alone must not allow reuse.
			data, _ := os.ReadFile(artifact)
			data[len(data)/2] ^= 0xff
			_ = os.WriteFile(artifact, data, 0644)
			run(true)
			good, _ := os.ReadFile(artifact)
			hash, _ := os.ReadFile(hashPath)
			_ = os.WriteFile(file, []byte("edited"), 0755)
			failure := func(map[string]builtinmetadata.File) (*builtinmetadata.Bundle, error) {
				return nil, fmt.Errorf("analysis failed")
			}
			if _, err := updateSet(source, output, kind, constant, "recipe", false, false, failure); err == nil {
				t.Fatal("generation failure hidden")
			}
			now, _ := os.ReadFile(artifact)
			nowHash, _ := os.ReadFile(hashPath)
			if !bytes.Equal(good, now) || !bytes.Equal(hash, nowHash) {
				t.Fatal("failed analysis changed the artifact or hash")
			}
		})
	}
}

func TestSyntaxFlowCompilerChangesInvalidateMetadata(t *testing.T) {
	root := t.TempDir()
	for _, file := range []string{"common/syntaxflow/sf/parser.go", "common/syntaxflow/sfvm/compiler.go", "common/schema/syntaxflow_rule.go"} {
		name := filepath.Join(root, file)
		if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte("package fixture"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	before, err := recipeFor(root, "syntaxflow")
	if err != nil {
		t.Fatal(err)
	}
	compiler := filepath.Join(root, "common/syntaxflow/sfvm/compiler.go")
	if err := os.WriteFile(compiler, []byte("package changed"), 0644); err != nil {
		t.Fatal(err)
	}
	after, err := recipeFor(root, "syntaxflow")
	if err != nil || before == after {
		t.Fatal("compiler change reused stale instructions", err)
	}
}

func TestCheckedInBundlesMatchCurrentScriptSets(t *testing.T) {
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	for _, spec := range resourceSpecs {
		recipe, err := recipeFor(root, spec.kind)
		if err != nil {
			t.Fatal(err)
		}
		called := false
		_, err = updateSet(filepath.Join(root, spec.dir), filepath.Join(root, "common/consts"), spec.kind, spec.constant, recipe, true, false, func(map[string]builtinmetadata.File) (*builtinmetadata.Bundle, error) {
			called = true
			return nil, fmt.Errorf("must not analyze unchanged scripts")
		})
		if err != nil {
			t.Fatal(err)
		}
		if called {
			t.Fatal("verification invoked the parser")
		}
	}
}

func TestRegistrationRecipeIgnoresUnrelatedGoEdits(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "common/coreplugin/coreplugin.go")
	_ = os.MkdirAll(filepath.Dir(p), 0755)
	write := func(code string) {
		t.Helper()
		if err := os.WriteFile(p, []byte(code), 0644); err != nil {
			t.Fatal(err)
		}
	}
	base := `package coreplugin;func init(){registerBuildInPlugin("yak","demo",withPluginEnableGenerateParam(true))}`
	write(base)
	old, err := recipeFor(root, "coreplugin")
	if err != nil {
		t.Fatal(err)
	}
	write(base + "\n// unrelated comment\n")
	same, _ := recipeFor(root, "coreplugin")
	if old != same {
		t.Fatal("unrelated edit invalidated metadata")
	}
	write(strings.Replace(base, `"demo"`, `"renamed"`, 1))
	changed, _ := recipeFor(root, "coreplugin")
	if old == changed {
		t.Fatal("registration changes must invalidate metadata")
	}
}
