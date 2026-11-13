package pass

import (
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa"
)

func TestRefineReturnTypesPublishesIntegerReturn(t *testing.T) {
	program := parseSSAForPass(t, `returns_int
type: () -> any
entry_0:
    <i64> t1 = <i64> 7
    ret <i64> t1`)
	RefineReturnTypes(program)
	if got := functionType(findFunctionForPass(t, program, "returns_int")).ReturnType.String(); got != "i64" {
		t.Fatalf("return type = %q, want i64", got)
	}
}

func TestRefineReturnTypesPublishesPointerReturn(t *testing.T) {
	program := parseSSAForPass(t, `returns_ptr
type: () -> any
entry_0:
    <ptr64> t1 = <ptr64> 0x1000
    ret <ptr64> t1`)
	RefineReturnTypes(program)
	if got := functionType(findFunctionForPass(t, program, "returns_ptr")).ReturnType.String(); got != "ptr64" {
		t.Fatalf("return type = %q, want ptr64", got)
	}
}

func TestRefineReturnTypesMarksVoid(t *testing.T) {
	program := parseSSAForPass(t, `returns_void
type: () -> any
entry_0:
    ret`)
	RefineReturnTypes(program)
	if got := functionType(findFunctionForPass(t, program, "returns_void")).ReturnType.String(); got != "null" {
		t.Fatalf("return type = %q, want null", got)
	}
}

func TestRefineReturnTypesPublishesCallType(t *testing.T) {
	program := parseSSAForPass(t, `callee
type: () -> any
entry_0:
    <i64> t1 = <i64> 1
    ret <i64> t1

caller
type: () -> void
entry_0:
    <any> t1 = call <any> callee () binding[] member[]
    ret`)
	call := findCallForPass(t, program, "callee")
	RefineReturnTypes(program)
	if got := call.GetType().String(); got != "i64" {
		t.Fatalf("call type = %q, want i64", got)
	}
}

func TestRefineReturnTypesSkipsKnownDefinitions(t *testing.T) {
	program := parseSSAForPass(t, `puts
type: () -> any
entry_0:
    ret`)
	RefineReturnTypes(program)
	if got := functionType(findFunctionForPass(t, program, "puts")).ReturnType.String(); strings.TrimSpace(got) == "" {
		t.Fatal("known definition lost its return type")
	}
}

func functionType(function *ssa.Function) *ssa.FunctionType {
	typ, _ := ssa.ToFunctionType(function.GetType())
	if typ == nil {
		return &ssa.FunctionType{}
	}
	return typ
}

func findFunctionForPass(t *testing.T, program *ssa.Program, name string) *ssa.Function {
	t.Helper()
	var found *ssa.Function
	program.Funcs.ForEach(func(_ string, function *ssa.Function) bool {
		if function != nil && function.GetName() == name {
			found = function
			return false
		}
		return true
	})
	if found == nil {
		t.Fatalf("function %q is missing", name)
	}
	return found
}
