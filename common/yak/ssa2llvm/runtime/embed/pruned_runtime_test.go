package embed

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
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
	assertContains(t, got, `"Sha256":`)
	assertContains(t, got, `codec.Sha256,`)
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
		{Module: "", Methods: []string{"definitelyMissingGlobal"}},
	})
	if !errors.Is(err, ErrUnsupportedPrunedRuntime) {
		t.Fatalf("expected ErrUnsupportedPrunedRuntime, got %v", err)
	}
}

func TestWritePrunedRuntimeImports_UnsupportedDependency(t *testing.T) {
	err := writePrunedRuntimeImports(t.TempDir(), []YaklibDependency{
		{Module: "definitelyMissingModule", Methods: []string{"Call"}},
	})
	if !errors.Is(err, ErrUnsupportedPrunedRuntime) {
		t.Fatalf("expected ErrUnsupportedPrunedRuntime, got %v", err)
	}
}

func TestScriptEngineLibRegistry(t *testing.T) {
	registry, err := scriptEngineRegistryFromLocalSource()
	if err != nil {
		t.Fatalf("load script engine registry failed: %v", err)
	}

	jsonExport, ok := registry.module("json")
	if !ok {
		t.Fatalf("missing json module in script engine registry")
	}
	if jsonExport.Expr != "yaklib.JsonExports" {
		t.Fatalf("unexpected json export expression: %q", jsonExport.Expr)
	}

	fileExport, ok := registry.module("file")
	if !ok {
		t.Fatalf("missing file module in script engine registry")
	}
	if fileExport.Expr != "yaklib.FileExport" {
		t.Fatalf("unexpected file export expression: %q", fileExport.Expr)
	}

	filesysExport, ok := registry.module("filesys")
	if !ok {
		t.Fatalf("missing filesys module in script engine registry")
	}
	if filesysExport.Expr != "filesys.Exports" {
		t.Fatalf("unexpected filesys export expression: %q", filesysExport.Expr)
	}

	ssaExport, ok := registry.module("ssa")
	if !ok {
		t.Fatalf("missing ssa module in script engine registry")
	}
	assertContains(t, ssaExport.Expr, "lo.Assign(")
	assertContains(t, ssaExport.Expr, "ssaapi.Exports")
	assertContains(t, ssaExport.Expr, "ssaproject.Exports")
	assertContains(t, ssaExport.Expr, "ssaconfig.Exports")
	assertNotContains(t, ssaExport.Expr, "ssaExports")

	sprintfExport, ok := registry.globalForMethod("sprintf")
	if !ok {
		t.Fatalf("missing sprintf global in script engine registry")
	}
	if sprintfExport.Expr != "builtin.YaklangBaseLib" {
		t.Fatalf("unexpected sprintf export expression: %q", sprintfExport.Expr)
	}

	atoiExport, ok := registry.globalForMethod("atoi")
	if !ok {
		t.Fatalf("missing atoi global in script engine registry")
	}
	if atoiExport.Expr != "yaklib.GlobalExport" {
		t.Fatalf("unexpected atoi export expression: %q", atoiExport.Expr)
	}
}

func TestLocalGoModuleRootFromSourcePathWhenOutsideModule(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("chdir temp: %v", err)
	}

	root, err := localGoModuleRoot()
	if err != nil {
		t.Fatalf("localGoModuleRoot outside module failed: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod from resolved root: %v", err)
	}
	assertContains(t, string(data), "module "+yaklangModulePath)
}

func TestWritePrunedRuntimeImports_ScriptEngineModuleDependencies(t *testing.T) {
	dir := t.TempDir()

	err := writePrunedRuntimeImports(dir, []YaklibDependency{
		{Module: "json", Methods: []string{"dumps", "loads"}},
		{Module: "file", Methods: []string{"GetExt"}},
		{Module: "filesys", Methods: []string{"Recursive"}},
		{Module: "ssa", Methods: []string{"NewConfig", "withProgramName"}},
	})
	if err != nil {
		t.Fatalf("writePrunedRuntimeImports failed: %v", err)
	}

	got := readGeneratedRuntimeImports(t, dir)
	assertContains(t, got, `yaklib "github.com/yaklang/yaklang/common/yak/yaklib"`)
	assertContains(t, got, `filesys "github.com/yaklang/yaklang/common/utils/filesys"`)
	assertContains(t, got, `lo "github.com/samber/lo"`)
	assertContains(t, got, `ssaapi "github.com/yaklang/yaklang/common/yak/ssaapi"`)
	assertContains(t, got, `ssaconfig "github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"`)
	assertContains(t, got, `ssaproject "github.com/yaklang/yaklang/common/yak/ssaproject"`)
	assertContains(t, got, `runtimeRegisterYaklibModule("json", yaklib.JsonExports)`)
	assertContains(t, got, `runtimeRegisterYaklibModule("file", yaklib.FileExport)`)
	assertContains(t, got, `runtimeRegisterYaklibModule("filesys", filesys.Exports)`)
	assertContains(t, got, `runtimeRegisterYaklibModule("ssa", lo.Assign(ssaapi.Exports,`)
	assertContains(t, got, `ssaproject.Exports,`)
	assertContains(t, got, `ssaconfig.Exports,`)
}

func TestWritePrunedRuntimeImports_ScriptEngineGlobalDependencies(t *testing.T) {
	dir := t.TempDir()

	err := writePrunedRuntimeImports(dir, []YaklibDependency{
		{Module: "", Methods: []string{"atoi", "sprintf"}},
	})
	if err != nil {
		t.Fatalf("writePrunedRuntimeImports failed: %v", err)
	}

	got := readGeneratedRuntimeImports(t, dir)
	assertContains(t, got, `builtin "github.com/yaklang/yaklang/common/yak/yaklang/lib/builtin"`)
	assertContains(t, got, `yaklib "github.com/yaklang/yaklang/common/yak/yaklib"`)
	assertContains(t, got, `runtimeRegisterYaklibGlobals(builtin.YaklangBaseLib)`)
	assertContains(t, got, `runtimeRegisterYaklibGlobals(yaklib.GlobalExport)`)
}

func TestWritePrunedRuntimeImports_YakitDependencyUsesRuntimeClient(t *testing.T) {
	dir := t.TempDir()

	err := writePrunedRuntimeImports(dir, []YaklibDependency{
		{Module: "yakit", Methods: []string{"Code"}},
	})
	if err != nil {
		t.Fatalf("writePrunedRuntimeImports failed: %v", err)
	}

	got := readGeneratedRuntimeImports(t, dir)
	assertContains(t, got, `runtimeRegisterYaklibModule("yakit", runtimePrunedYakitExports())`)
	assertNotContains(t, got, `yaklib "github.com/yaklang/yaklang/common/yak/yaklib"`)
}

func TestPrunedRuntimeBuildTags_ExcludePocByDefault(t *testing.T) {
	tags := prunedRuntimeBuildTags(PrunedRuntimeDependencies{})
	if !containsString(tags, "ssa2llvm_pruned_runtime") {
		t.Fatalf("expected base pruned runtime tag in %#v", tags)
	}
	if containsString(tags, "ssa2llvm_runtime_poc") {
		t.Fatalf("did not expect POC runtime tag by default: %#v", tags)
	}
}

func TestPrunedRuntimeBuildTags_IncludePocForPocDispatch(t *testing.T) {
	tags := prunedRuntimeBuildTags(PrunedRuntimeDependencies{
		RuntimeDispatch: []abi.FuncID{abi.IDPrintln, abi.IDPocGet},
	})
	if !containsString(tags, "ssa2llvm_runtime_poc") {
		t.Fatalf("expected POC runtime tag for POC dispatch: %#v", tags)
	}
}

func TestPrunedRuntimeBuildTags_IncludeCliForCliModule(t *testing.T) {
	tags := prunedRuntimeBuildTags(PrunedRuntimeDependencies{
		Yaklib: []YaklibDependency{{Module: "cli", Methods: []string{"String"}}},
	})
	if !containsString(tags, "ssa2llvm_runtime_cli") {
		t.Fatalf("expected cli runtime tag for cli module: %#v", tags)
	}
}

func TestPrunedRuntimeBuildTags_IncludeYakitForYakitModule(t *testing.T) {
	tags := prunedRuntimeBuildTags(PrunedRuntimeDependencies{
		Yaklib: []YaklibDependency{{Module: "yakit", Methods: []string{"Code"}}},
	})
	if !containsString(tags, "ssa2llvm_runtime_yakit") {
		t.Fatalf("expected yakit runtime tag for yakit module: %#v", tags)
	}
}

func TestUnsupportedPrunedRuntimeDependencies(t *testing.T) {
	got := UnsupportedPrunedRuntimeDependencies([]YaklibDependency{
		{Module: "codec", Methods: []string{"EncodeBase64", "Missing"}},
		{Module: "", Methods: []string{"definitelyMissingGlobal", "len", "sprintf"}},
		{Module: "json", Methods: []string{"dumps", "loads"}},
		{Module: "file", Methods: []string{"GetExt"}},
		{Module: "ssa", Methods: []string{"NewConfig"}},
		{Module: "yakit", Methods: []string{"Code"}},
		{Module: "cli", Methods: []string{"String", "check"}},
		{Module: "definitelyMissingModule", Methods: []string{"Call"}},
	})
	want := []YaklibDependency{
		{Module: "", Methods: []string{"definitelyMissingGlobal"}},
		{Module: "codec", Methods: []string{"Missing"}},
		{Module: "definitelyMissingModule", Methods: []string{"Call"}},
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

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
