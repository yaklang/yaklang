package compiler

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
	"github.com/yaklang/yaklang/common/yak/yaklang"
)

// Ensure yaklang stdlib registration matches compile-time dispatch resolution.
import _ "github.com/yaklang/yaklang/common/yak"

func TestYaklibDispatch_CodecUsesGenericYaklibPath(t *testing.T) {
	code := `check = () => { return len(codec.EncodeToHex("ab")) }`
	_, _, ir, err := compileToIRFromCodeWithExternBindings(code, "yak", nil)
	require.NoError(t, err)

	requireIRContainsGlobalCString(t, ir, "codec")
	requireIRContainsGlobalCString(t, ir, "EncodeToHex")
	require.Contains(t, ir, "yaklib_pkg_")
	require.Contains(t, ir, "call void @"+abi.InvokeSymbol)
	requireIRAvoidsLegacyCallEntrypoints(t, ir)
}

func TestYaklibDispatch_RecordsRuntimeDependencies(t *testing.T) {
	deps := compileYaklibDependencies(t, `check = () => { if codec.EncodeBase64("yak") == "eWFr" { return 0 }; return 1 }`)
	require.Equal(t, map[string][]string{
		"codec": {"EncodeBase64"},
	}, deps)
}

func TestYaklibDispatch_RecordsGlobalBuiltinRuntimeDependencies(t *testing.T) {
	deps := compileYaklibDependencies(t, `check = () => { return len("yak") }`)
	require.Equal(t, map[string][]string{
		"": {"len"},
	}, deps)
}

func TestYaklibDispatch_BuiltinsDoNotRecordRuntimeDependencies(t *testing.T) {
	deps := compileYaklibDependencies(t, `check = () => { println(1); return 0 }`)
	require.Empty(t, deps)
}

func TestYaklibExtern_ModeAllConstantDoesNotRecordRuntimeDependency(t *testing.T) {
	deps := compileYaklibDependencies(t, `check = () => { if ssa.ModeAll == 0 { return -1 }; return ssa.ModeAll }`)
	require.Empty(t, deps)
}

func TestYaklibDispatch_YakitInfoUsesDedicatedFuncID(t *testing.T) {
	code := `check = () => { yakit.Info("ok"); return 1 }`
	_, _, ir, err := compileToIRFromCodeWithExternBindings(code, "yak", nil)
	require.NoError(t, err)

	binding, ok := defaultExternBindings["yakit.Info"]
	require.True(t, ok)
	require.Equal(t, abi.IDYakitInfo, binding.DispatchID)

	require.NotContains(t, ir, "yaklib_pkg_")
	require.Contains(t, ir, "call void @"+abi.InvokeSymbol)
}

func TestYaklibDispatch_NonStdlibGlobalDoesNotUseGenericYaklibPath(t *testing.T) {
	// Boundary: unresolved globals may still compile via dynamic callable fallback,
	// but must not be misclassified as yaklib stdlib dispatch.
	code := `check = () => { return totallyMissingSymbol() }`
	_, _, ir, err := compileToIRFromCodeWithExternBindings(code, "yak", nil)
	require.NoError(t, err)
	require.NotContains(t, ir, "yaklib_pkg_")
	require.Contains(t, ir, "call void @"+abi.InvokeSymbol)
}

func TestYaklibDispatch_LookupExportMatchesRegisteredStdlib(t *testing.T) {
	_, ok := yaklang.LookupExport("codec", "EncodeToHex")
	require.True(t, ok, "codec.EncodeToHex must be registered for generic yaklib dispatch")

	_, ok = yaklang.LookupExport("yakit", "AutoInitYakit")
	require.False(t, ok, "AutoInitYakit must not be exported via YakitExports")
}

func TestYaklibDispatch_PrintlnUsesBuiltinFuncIDNotGenericPath(t *testing.T) {
	code := `check = () => { println(1); return 0 }`
	_, _, ir, err := compileToIRFromCodeWithExternBindings(code, "yak", nil)
	require.NoError(t, err)

	require.NotContains(t, ir, "yaklib_pkg_")
	require.GreaterOrEqual(t, strings.Count(ir, "call void @"+abi.InvokeSymbol), 1)
}

func TestYaklibDispatch_FunctionReturnArgsAreRooted(t *testing.T) {
	code := `
check = () => {
	opt = ssa.withProjectName("probe")
	config, err = ssa.NewConfig(ssa.ModeAll, opt)
	if err != nil { return -1 }
	if config == nil { return -2 }
	return 0
}
`
	_, _, ir, err := compileToIRFromCodeWithExternBindings(code, "yak", nil)
	require.NoError(t, err)

	require.Regexp(t, `store i64 %yak_ctx_arg_tag[0-9]*, ptr %[0-9]+, align 4
  %[0-9]+ = getelementptr i64, ptr %yak_yaklib_ctx_i64p[0-9]*, i64 [0-9]+
  store i64 %yak_load_[0-9]+, ptr %[0-9]+, align 4`, ir)
}

func TestYaklibDispatch_StringEqualityUsesRuntimeDispatch(t *testing.T) {
	code := `check = () => { if "php" == "php" { return 1 }; return 0 }`
	_, _, ir, err := compileToIRFromCodeWithExternBindings(code, "yak", nil)
	require.NoError(t, err)

	require.Contains(t, ir, "i64 27")
	require.Contains(t, ir, "yak_eq_ctx")
}

func compileYaklibDependencies(t *testing.T, code string) map[string][]string {
	t.Helper()
	_, comp, _, err := compileInput("", code, "yak", nil, "", nil)
	require.NoError(t, err)
	require.NotNil(t, comp)
	defer comp.Dispose()
	return comp.YaklibDependencies()
}

func TestYaklibExtern_ModeAllConstantInIR(t *testing.T) {
	code := `check = () => { if ssa.ModeAll == 0 { return -1 }; return ssa.ModeAll }`
	_, _, ir, err := compileToIRFromCodeWithExternBindings(code, "yak", nil)
	require.NoError(t, err)
	require.Contains(t, ir, "i64 127")
}

func TestYaklibExtern_FunctionMemberValueDoesNotRecurse(t *testing.T) {
	code := `
check = () => {
	callable = codec.EncodeToHex
	if callable == nil {
		return 0
	}
	return 1
}
`
	_, _, ir, err := compileToIRFromCodeWithExternBindings(code, "yak", nil)
	require.NoError(t, err)
	require.Contains(t, ir, "yak_eq_ctx")
}
