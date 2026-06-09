package embed

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestWritePrunedRuntimeImports_CodecDependency(t *testing.T) {
	dir := t.TempDir()

	err := writePrunedRuntimeImports(dir, []YaklibDependency{
		{Module: "codec", Methods: []string{"EncodeBase64", "EncodeBase64", "Sha256"}},
	})
	if err != nil {
		t.Fatalf("writePrunedRuntimeImports failed: %v", err)
	}

	got := readGeneratedRuntimeImports(t, dir)
	assertContains(t, got, "//go:build ssa2llvm_pruned_runtime")
	assertContains(t, got, `codec "github.com/yaklang/yaklang/common/yak/yaklib/codec"`)
	assertContains(t, got, `runtimeRegisterYaklibModule("codec", map[string]any{`)
	assertContains(t, got, `"EncodeBase64": codec.EncodeBase64,`)
	assertContains(t, got, `"Sha256": codec.Sha256,`)
	assertNotContains(t, got, `_ "github.com/yaklang/yaklang/common/yak"`)
	if count := strings.Count(got, `"EncodeBase64": codec.EncodeBase64,`); count != 1 {
		t.Fatalf("EncodeBase64 should be generated once, got %d occurrences in:\n%s", count, got)
	}
}

func TestWritePrunedRuntimeImports_WholeModuleDependency(t *testing.T) {
	dir := t.TempDir()

	err := writePrunedRuntimeImports(dir, []YaklibDependency{
		{Module: "cli", Methods: []string{"String"}},
	})
	if err != nil {
		t.Fatalf("writePrunedRuntimeImports failed: %v", err)
	}

	got := readGeneratedRuntimeImports(t, dir)
	assertContains(t, got, `cli "github.com/yaklang/yaklang/common/utils/cli"`)
	assertContains(t, got, `runtimeRegisterYaklibModule("cli", cli.CliExports)`)
	assertNotContains(t, got, `map[string]any{`)
}

func TestWritePrunedRuntimeImports_GlobalBuiltinDependency(t *testing.T) {
	dir := t.TempDir()

	err := writePrunedRuntimeImports(dir, []YaklibDependency{
		{Module: "", Methods: []string{"len"}},
	})
	if err != nil {
		t.Fatalf("writePrunedRuntimeImports failed: %v", err)
	}

	got := readGeneratedRuntimeImports(t, dir)
	assertContains(t, got, `runtimeRegisterYaklibGlobals(map[string]any{`)
	assertContains(t, got, `"len": runtimeYakBuiltinLen,`)
	assertNotContains(t, got, `github.com/yaklang/yaklang/common/yak/yaklang/lib/builtin`)
	assertNotContains(t, got, `_ "github.com/yaklang/yaklang/common/yak"`)
}

func TestWritePrunedRuntimeImports_UnsupportedGlobalDependency(t *testing.T) {
	err := writePrunedRuntimeImports(t.TempDir(), []YaklibDependency{
		{Module: "", Methods: []string{"sprint"}},
	})
	if !errors.Is(err, ErrUnsupportedPrunedRuntime) {
		t.Fatalf("expected ErrUnsupportedPrunedRuntime, got %v", err)
	}
}

func TestWritePrunedRuntimeImports_UnsupportedDependency(t *testing.T) {
	err := writePrunedRuntimeImports(t.TempDir(), []YaklibDependency{
		{Module: "json", Methods: []string{"dumps"}},
	})
	if !errors.Is(err, ErrUnsupportedPrunedRuntime) {
		t.Fatalf("expected ErrUnsupportedPrunedRuntime, got %v", err)
	}
}

func TestUnsupportedPrunedRuntimeDependencies(t *testing.T) {
	got := UnsupportedPrunedRuntimeDependencies([]YaklibDependency{
		{Module: "codec", Methods: []string{"EncodeBase64", "Missing"}},
		{Module: "", Methods: []string{"len", "sprintf"}},
		{Module: "json", Methods: []string{"dumps", "loads"}},
		{Module: "cli", Methods: []string{"String", "check"}},
	})
	want := []YaklibDependency{
		{Module: "", Methods: []string{"sprintf"}},
		{Module: "codec", Methods: []string{"Missing"}},
		{Module: "json", Methods: []string{"dumps", "loads"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected unsupported deps:\nwant=%#v\n got=%#v", want, got)
	}

	err := ValidatePrunedRuntimeDependencies(got)
	if !errors.Is(err, ErrUnsupportedPrunedRuntime) {
		t.Fatalf("expected ErrUnsupportedPrunedRuntime, got %v", err)
	}
}

func readGeneratedRuntimeImports(t *testing.T, dir string) string {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(dir, "runtime_imports_generated.go"))
	if err != nil {
		t.Fatalf("read generated imports failed: %v", err)
	}
	return string(got)
}

func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Fatalf("expected generated imports to contain %q:\n%s", substr, s)
	}
}

func assertNotContains(t *testing.T, s, substr string) {
	t.Helper()
	if strings.Contains(s, substr) {
		t.Fatalf("expected generated imports not to contain %q:\n%s", substr, s)
	}
}
