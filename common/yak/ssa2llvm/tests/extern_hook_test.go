package tests

import (
	"testing"
	"unsafe"

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/compiler"
)

func withJITTestAllocHook() compiler.RunOption {
	return compiler.WithRunExternalHooks(map[string]unsafe.Pointer{
		"yak_internal_print_int": getHookAddr(),
		"yak_test_alloc":         getTestAllocHookAddr(),
	})
}

func TestJIT_CustomExternBindingHook(t *testing.T) {
	teardown := SetupJITHook()

	if _, err := compiler.RunViaJIT(
		compiler.WithRunSourceCode(`
func check() {
    v = makeAlloc(23)
    println(v)
}
`),
		compiler.WithRunLanguage("yak"),
		compiler.WithRunFunction("check"),
		compiler.WithRunExternBindings(map[string]compiler.ExternBinding{
			"makeAlloc": {
				Symbol: "yak_test_alloc",
				Params: []compiler.LLVMExternType{compiler.ExternTypeI64},
				Return: compiler.ExternTypeI64,
			},
		}),
		withJITTestAllocHook(),
	); err != nil {
		t.Fatalf("JIT execution failed: %v", err)
	}

	vals := teardown()
	if len(vals) != 1 {
		t.Fatalf("expected 1 print call, got %d (%v)", len(vals), vals)
	}
	if vals[0] != 1023 {
		t.Fatalf("expected transformed value 1023, got %d", vals[0])
	}
}

func TestBinary_CustomExternBindingWithLinkedObject(t *testing.T) {
	code := `
func main() {
    v = getObject(7)
    println(v)
}
`
	goCode := `
import "fmt"

func getObject(x int64) int64 {
	return x * 3
}

func yak_internal_print_int(n int64) {
	fmt.Println(n)
}

func yak_runtime_gc() {}
`

	output := runBinaryWithEnv(t, code, "main", nil, withRuntimeCode(goCode))
	if output != "21\n" {
		t.Fatalf("expected output 21, got %q", output)
	}
}
