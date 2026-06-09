package compiler

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/coreplugin"
	runtimeembed "github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/embed"
)

func TestCorePluginSSADetectPrunedRuntimeDependencyFallback(t *testing.T) {
	t.Setenv("YAKIT_HOME", t.TempDir())

	code := string(coreplugin.GetCorePluginData("SSA 项目探测"))
	require.NotEmpty(t, code)

	cfg := newCompileConfig(
		WithCompileSourceCode(code),
		WithCompileLanguage("yak"),
		WithCompileEntryFunction("main"),
		WithCompilePluginType(YakPluginTypeYak),
	)
	_, comp, _, err := compileInputWithConfig(cfg)
	require.NoError(t, err)
	require.NotNil(t, comp)
	defer comp.Dispose()

	deps := comp.YaklibDependencies()
	requireYaklibDependency(t, deps, "cli", "String")
	requireYaklibDependency(t, deps, "codec", "DecodeHex")
	requireYaklibDependency(t, deps, "json", "dumps")
	requireYaklibDependency(t, deps, "json", "loads")
	requireYaklibDependency(t, deps, "ssa", "NewConfig")
	requireYaklibDependency(t, deps, "ssa", "withProgramName")
	requireYaklibDependency(t, deps, "file", "GetExt")
	requireYaklibDependency(t, deps, "yakit", "Code")
	requireYaklibDependency(t, deps, "", "sprintf")

	runtimeDeps := runtimeYaklibDepsFromCompiler(comp)
	unsupported := runtimeembed.UnsupportedPrunedRuntimeDependencies(runtimeDeps)
	requireUnsupportedPrunedRuntimeDependency(t, unsupported, "json", "dumps")
	requireUnsupportedPrunedRuntimeDependency(t, unsupported, "ssa", "NewConfig")
	requireUnsupportedPrunedRuntimeDependency(t, unsupported, "file", "GetExt")
	requireUnsupportedPrunedRuntimeDependency(t, unsupported, "yakit", "Code")
	requireUnsupportedPrunedRuntimeDependency(t, unsupported, "", "sprintf")

	err = runtimeembed.ValidatePrunedRuntimeDependencies(runtimeDeps)
	require.True(t, errors.Is(err, runtimeembed.ErrUnsupportedPrunedRuntime), "expected unsupported pruned runtime dependency, got %v", err)
}

func requireYaklibDependency(t *testing.T, deps map[string][]string, module, method string) {
	t.Helper()
	methods, ok := deps[module]
	require.Truef(t, ok, "missing yaklib module %q in deps %#v", module, deps)
	require.Containsf(t, methods, method, "missing yaklib dependency %s.%s in %#v", module, method, deps)
}

func requireUnsupportedPrunedRuntimeDependency(t *testing.T, deps []runtimeembed.YaklibDependency, module, method string) {
	t.Helper()
	for _, dep := range deps {
		if dep.Module != module {
			continue
		}
		if containsString(dep.Methods, method) {
			return
		}
	}
	t.Fatalf("missing unsupported pruned runtime dependency %s.%s in %#v", module, method, deps)
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
