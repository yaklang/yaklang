package pass

import (
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa"
)

func TestMarkVoidReturns(t *testing.T) {
	program := parseSSAForPass(t, `voidish
type: () -> any
entry_0:
    <string> t1 = <string> "load:1"
    <i64> t2 = type_cast[i64] <string> t1
    ret <i64> t2

value
type: () -> i64
entry_0:
    <i64> t1 = <i64> 0
    ret <i64> t1`)
	voidFunction := findFunctionForPass(t, program, "voidish")
	valueFunction := findFunctionForPass(t, program, "value")

	if count := MarkVoidReturns(program); count != 1 {
		t.Fatalf("void functions = %d", count)
	}
	if got := voidFunction.GetType().String(); !strings.Contains(got, "-> null") {
		t.Fatalf("void return type = %s", got)
	}
	for _, blockID := range voidFunction.Blocks {
		block, _ := voidFunction.GetBasicBlockByID(blockID)
		for _, instructionID := range block.Insts {
			instruction, _ := voidFunction.GetInstructionById(instructionID)
			if ret, ok := instruction.(*ssa.Return); ok && len(ret.Results) != 0 {
				t.Fatalf("void return kept %d values", len(ret.Results))
			}
		}
	}
	if got := valueFunction.GetType().String(); strings.Contains(got, "-> null") {
		t.Fatal("value-returning function was marked void")
	}
}

func TestRefineReturnTypes(t *testing.T) {
	program := parseSSAForPass(t, `pointer
type: () -> any
entry_0:
    <ptr64> t1 = type_cast[ptr64] <i64> 0
    ret <ptr64> t1

value
type: () -> i64
entry_0:
    <i64> t1 = <i64> 0
    ret <i64> t1

void
type: () -> any
entry_0:
    ret`)
	RefineReturnTypes(program)
	if got := findFunctionForPass(t, program, "pointer").GetType().String(); !strings.Contains(got, "-> ptr64") {
		t.Fatalf("pointer return type = %s", got)
	}
	if got := findFunctionForPass(t, program, "value").GetType().String(); !strings.Contains(got, "-> i64") {
		t.Fatalf("integer return type = %s", got)
	}
	if got := findFunctionForPass(t, program, "void").GetType().String(); !strings.Contains(got, "-> null") {
		t.Fatalf("void return type = %s", got)
	}
}
