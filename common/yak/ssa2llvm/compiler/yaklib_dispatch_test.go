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
