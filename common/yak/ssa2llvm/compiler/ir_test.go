package compiler

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func testInteropExternBindings() map[string]ExternBinding {
	return map[string]ExternBinding{
		"getObject": {
			Symbol: "yak_runtime_get_object",
			Return: ExternTypePtr,
		},
	}
}

func requireIRContainsInOrder(t *testing.T, ir string, parts ...string) {
	t.Helper()
	last := -1
	for _, part := range parts {
		searchFrom := last + 1
		if searchFrom < 0 {
			searchFrom = 0
		}
		offset := strings.Index(ir[searchFrom:], part)
		idx := -1
		if offset >= 0 {
			idx = searchFrom + offset
		}
		require.NotEqualf(t, -1, idx, "expected IR to contain %q", part)
		require.Greaterf(t, idx, last, "expected IR part %q after previous part", part)
		last = idx
	}
}

func requireIRAvoidsLegacyCallEntrypoints(t *testing.T, ir string) {
	t.Helper()
	require.NotContains(t, ir, "call void @yak_runtime_dispatch")
	require.NotContains(t, ir, "call void @yak_runtime_spawn")
	require.NotContains(t, ir, "call void @yak_runtime_invoke_async")
}

func TestIR_LocalFunctionCallUsesUnifiedInvoke(t *testing.T) {
	code := `
		func add(a, b) {
			return a + b
		}

		func main() {
			println(add(10, 20))
		}
		`
	_, _, ir, err := compileToIRFromCodeWithExternBindings(code, "yak", nil)
	require.NoError(t, err)
	require.GreaterOrEqual(t, strings.Count(ir, "call void @yak_runtime_invoke"), 2)
	require.Contains(t, ir, "@add")
	require.NotContains(t, ir, "call void @add")
	requireIRAvoidsLegacyCallEntrypoints(t, ir)
}

func TestIR_ObjectInteropCalls(t *testing.T) {
	code := `
		func main() {
			a = getObject(10)
			v = a.Number
			println(v)
		}
		`
	_, _, ir, err := compileToIRFromCodeWithExternBindings(code, "yak", testInteropExternBindings())
	require.NoError(t, err)
	requireIRContainsInOrder(t, ir,
		"@yak_runtime_get_object",
		"call void @yak_runtime_invoke",
		"call i64 @yak_runtime_get_field",
		"call void @yak_runtime_invoke",
	)
	requireIRAvoidsLegacyCallEntrypoints(t, ir)
}

func TestIR_LoopEmitsBranchesAndCalls(t *testing.T) {
	code := `
		func main() {
			i = 0
			for {
				if i > 3 { break }
				a = getObject(i)
				i = i + 1
			}
			println(999)
		}
		`
	_, _, ir, err := compileToIRFromCodeWithExternBindings(code, "yak", testInteropExternBindings())
	require.NoError(t, err)
	requireIRContainsInOrder(t, ir,
		"br i1",
		"@yak_runtime_get_object",
		"call void @yak_runtime_invoke",
		"call void @yak_runtime_invoke",
	)
	requireIRAvoidsLegacyCallEntrypoints(t, ir)
}

func TestIR_CustomExternBindingPointerReturn(t *testing.T) {
	code := `
		func main() {
			a = newObject(10)
			v = a.Number
			println(v)
		}
		`
	bindings := map[string]ExternBinding{
		"newObject": {
			Symbol: "yak_runtime_get_object",
			Return: ExternTypePtr,
		},
	}
	_, _, ir, err := compileToIRFromCodeWithExternBindings(code, "yak", bindings)
	require.NoError(t, err)
	requireIRContainsInOrder(t, ir,
		"@yak_runtime_get_object",
		"call void @yak_runtime_invoke",
		"call i64 @yak_runtime_get_field",
		"call void @yak_runtime_invoke",
	)
	requireIRAvoidsLegacyCallEntrypoints(t, ir)
}

func TestIR_CustomExternBindingOverrideGetObject(t *testing.T) {
	code := `
		func main() {
			v = getObject(16)
			println(v)
		}
		`
	bindings := map[string]ExternBinding{
		"getObject": {
			Symbol: "yak_hook_get_object",
			Return: ExternTypeI64,
		},
	}
	_, _, ir, err := compileToIRFromCodeWithExternBindings(code, "yak", bindings)
	require.NoError(t, err)
	requireIRContainsInOrder(t, ir,
		"@yak_hook_get_object",
		"call void @yak_runtime_invoke",
		"call void @yak_runtime_invoke",
	)
	requireIRAvoidsLegacyCallEntrypoints(t, ir)
}

func TestIR_LegacyExternBindingParamsRejected(t *testing.T) {
	code := `
		func main() {
			v = getObject(16)
			println(v)
		}
		`
	bindings := map[string]ExternBinding{
		"getObject": {
			Symbol: "yak_hook_get_object",
			Params: []LLVMExternType{ExternTypeI64},
			Return: ExternTypeI64,
		},
	}
	_, _, _, err := compileInput("", code, "yak", bindings, nil, nil)
	require.Error(t, err)
	require.ErrorContains(t, err, "legacy parameter ABI")
}

func TestIR_GoStmtUsesAsyncInvoke(t *testing.T) {
	code := `
		func main() {
			go println(1)
		}
		`
	_, _, ir, err := compileToIRFromCodeWithExternBindings(code, "yak", nil)
	require.NoError(t, err)
	requireIRContainsInOrder(t, ir,
		"store i64 1",
		"call void @yak_runtime_invoke",
	)
	requireIRAvoidsLegacyCallEntrypoints(t, ir)
}

func TestIR_GoStmtCallableUsesAsyncInvoke(t *testing.T) {
	code := `
		func f(x) {
			println(x)
		}

		func main() {
			go f(10)
		}
		`
	_, _, ir, err := compileToIRFromCodeWithExternBindings(code, "yak", nil)
	require.NoError(t, err)
	requireIRContainsInOrder(t, ir,
		"@f",
		"store i64 1",
		"call void @yak_runtime_invoke",
	)
	requireIRAvoidsLegacyCallEntrypoints(t, ir)
}

func TestIR_MainWrapperUsesUnifiedInvoke(t *testing.T) {
	_, comp, _, err := compileInput("", `check = () => { return 42 }`, "yak", nil, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, comp)
	defer comp.Dispose()

	require.NoError(t, comp.addMainWrapperToModule("check", true))

	ir := comp.Mod.String()
	requireIRContainsInOrder(t, ir,
		"define i32 @main()",
		"call void @yak_runtime_invoke",
		"call void @yak_runtime_wait_async",
		"call void @yak_runtime_gc",
	)
	requireIRAvoidsLegacyCallEntrypoints(t, ir)
}

func TestIR_SyncWaitGroupUsesDispatch(t *testing.T) {
	code := `
		func main() {
			wg = sync.NewWaitGroup()
			wg.Add(1)
			wg.Done()
			wg.Wait()
		}
		`
	_, _, ir, err := compileToIRFromCodeWithExternBindings(code, "yak", nil)
	require.NoError(t, err)
	require.GreaterOrEqual(t, strings.Count(ir, "call void @yak_runtime_invoke"), 4)
	require.Contains(t, ir, "@yak_runtime_invoke")
	requireIRAvoidsLegacyCallEntrypoints(t, ir)
}

func TestIR_MakeSliceUsesRuntimeHelper(t *testing.T) {
	code := `
		func main() {
			a = make([]int, 10)
			println(a[1])
		}
		`
	_, _, ir, err := compileToIRFromCodeWithExternBindings(code, "yak", nil)
	require.NoError(t, err)
	require.Contains(t, ir, "@yak_runtime_make_slice")
	require.Contains(t, ir, "@yak_runtime_get_field")
}

func TestIR_AppendUsesDispatch(t *testing.T) {
	code := `
		func main() {
			a = make([]int, 10)
			a = append(a, 1)
			print(a)
		}
		`
	_, _, ir, err := compileToIRFromCodeWithExternBindings(code, "yak", nil)
	require.NoError(t, err)
	require.Contains(t, ir, "@yak_runtime_make_slice")
	require.GreaterOrEqual(t, strings.Count(ir, "call void @yak_runtime_invoke"), 2)
	requireIRAvoidsLegacyCallEntrypoints(t, ir)
}
