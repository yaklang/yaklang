package compiler

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
)

func TestIR_ObjectMethodBeatsGlobalBuiltin(t *testing.T) {
	code := `check = () => {
		f = () => { this = {"key": 1, "set": (i) => { this.key = i }, "get": () => this.key}; return this }
		a = f(); a.set(2); return a.get()
	}`
	_, _, ir, err := compileToIRFromCodeWithExternBindings(code, "yak", nil)
	require.NoError(t, err)
	require.NotContains(t, ir, "@yaklib_method")
	require.Contains(t, ir, "call i64 @"+abi.RuntimeGetFieldSymbol)
}

func TestEntryFunctionCaptureOwnership(t *testing.T) {
	code := "leaf = () => { return 7 }\nmid = () => { return leaf() + 8 }\ncheck = () => { return mid() + leaf() }\n"
	_, comp, _, err := compileInput("", code, "yak", nil, "", nil)
	require.NoError(t, err)
	defer comp.Dispose()
	var selected *ssa.Function
	comp.Program.EachFunction(func(fn *ssa.Function) {
		if fn != nil && fn.GetName() == "check" {
			selected = fn
		}
	})
	require.NotNil(t, selected)
	t.Logf("free=%d blocks=%d id=%d", len(selected.FreeValues), len(selected.Blocks), selected.GetId())
	for variable := range selected.FreeValues {
		if variable == nil || variable.GetValue() == nil {
			t.Logf("variable nil value")
			continue
		}
		definer := variable.GetValue().GetFunc()
		definerID := int64(-1)
		if definer != nil {
			definerID = definer.GetId()
		}
		t.Logf("name=%s definer=%d", variable.GetName(), definerID)
	}
}

func TestIR_MainWrapperKeepsPlainEntryAsAddress(t *testing.T) {
	_, comp, _, err := compileInput("", `check = () => { return 42 }`, "yak", nil, "", nil)
	require.NoError(t, err)
	defer comp.Dispose()
	require.NoError(t, comp.addMainWrapperToModule("check", false))
	ir := comp.Mod.String()
	start := strings.Index(ir, "define i32 @main()")
	require.GreaterOrEqual(t, start, 0)
	require.NotContains(t, ir[start:], "@"+abi.MakeCallableSymbol)
}

func TestIR_MainWrapperMaterializesCapturedEntry(t *testing.T) {
	for _, source := range []string{
		`fib = (n) => { if n <= 2 { return 1 }; return fib(n-1)+fib(n-2) }; check = () => { return fib(10) }`,
		`value = 42; check = () => { return value }`,
	} {
		_, comp, _, err := compileInput("", source, "yak", nil, "", nil)
		require.NoError(t, err)
		func() {
			defer comp.Dispose()
			require.NoError(t, comp.addMainWrapperToModule("check", false))
			require.NoError(t, llvm.VerifyModule(comp.Mod, llvm.ReturnStatusAction))
			ir := comp.Mod.String()
			start := strings.Index(ir, "define i32 @main()")
			require.GreaterOrEqual(t, start, 0)
			require.Contains(t, ir[start:], "@"+abi.MakeCallableSymbol)
			require.Contains(t, ir[start:], "call void @"+abi.InvokeSymbol)
			// The closure belongs to check. Its captured function must not be
			// declared again as an external symbol.
			require.NotRegexp(t, ir, `declare void @yak_user_fib\(`)
		}()
	}
}
