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
	assertContains(t, got, `yaklib "github.com/yaklang/yaklang/common/yak/yaklib"`)
	assertContains(t, got, `runtimeRegisterYaklibModule("codec", yaklib.CodecExports)`)
	assertNotContains(t, got, `_ "github.com/yaklang/yaklang/common/yak"`)
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
	assertContains(t, got, `ssaapi "github.com/yaklang/yaklang/common/yak/ssaapi"`)
	assertContains(t, got, `runtimeRegisterYaklibModule("json", yaklib.JsonExports)`)
	assertContains(t, got, `runtimeRegisterYaklibModule("file", yaklib.FileExport)`)
	assertContains(t, got, `runtimeRegisterYaklibModule("filesys", filesys.Exports)`)
	assertContains(t, got, `runtimeRegisterYaklibModule("ssa", ssaapi.YakExports)`)
}

func TestWritePrunedRuntimeImports_ScriptEngineGlobalDependencies(t *testing.T) {
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
	assertContains(t, got, `runtimeRegisterYaklibModule("yakit", loglite.YakitExports)`)
}

func TestUnsupportedPrunedRuntimeDependencies(t *testing.T) {
	got := UnsupportedPrunedRuntimeDependencies([]YaklibDependency{
		{Module: "codec", Methods: []string{"EncodeBase64", "Missing"}},
		{Module: "", Methods: []string{"definitelyMissingGlobal", "len"}},
		{Module: "json", Methods: []string{"dumps", "loads"}},
		{Module: "file", Methods: []string{"GetExt"}},
		{Module: "ssa", Methods: []string{"NewConfig"}},
		{Module: "yakit", Methods: []string{"Code"}},
		{Module: "cli", Methods: []string{"String", "check"}},
		{Module: "definitelyMissingModule", Methods: []string{"Call"}},
	})
	want := []YaklibDependency{
		{Module: "", Methods: []string{"definitelyMissingGlobal"}},
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
