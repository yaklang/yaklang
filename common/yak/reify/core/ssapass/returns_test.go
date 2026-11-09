package ssapass

import (
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa"
)

func TestMarkVoidReturns(t *testing.T) {
	program := ssa.NewProgram(nil, ssa.ProgramCacheMemory, ssa.Application, nil, "", 0)
	voidFunction := program.NewFunction("voidish")
	voidBuilder := voidFunction.GetOrCreateBuilder()
	marker := voidBuilder.EmitConstInst("load:1")
	cast := voidBuilder.EmitTypeCast(marker, ssa.NewBasicType(ssa.NumberTypeKind, "i64"))
	voidBuilder.EmitReturn(ssa.Values{cast})
	voidFunction.Finish()

	valueFunction := program.NewFunction("value")
	valueBuilder := valueFunction.GetOrCreateBuilder()
	valueConstant := valueBuilder.EmitConstInst(int64(0))
	valueConstant.SetType(ssa.NewBasicType(ssa.NumberTypeKind, "i64"))
	valueBuilder.EmitReturn(ssa.Values{valueConstant})
	valueFunction.Finish()

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
	program := ssa.NewProgram(nil, ssa.ProgramCacheMemory, ssa.Application, nil, "", 0)
	pointerFunction := program.NewFunction("pointer")
	pointerBuilder := pointerFunction.GetOrCreateBuilder()
	pointerBuilder.EmitReturn(ssa.Values{pointerBuilder.EmitTypeCast(pointerBuilder.EmitConstInst(int64(0)), ssa.NewBasicType(ssa.NumberTypeKind, "ptr64"))})
	pointerFunction.Finish()

	valueFunction := program.NewFunction("value")
	valueBuilder := valueFunction.GetOrCreateBuilder()
	valueConstant := valueBuilder.EmitConstInst(int64(0))
	valueConstant.SetType(ssa.NewBasicType(ssa.NumberTypeKind, "i64"))
	valueBuilder.EmitReturn(ssa.Values{valueConstant})
	valueFunction.Finish()

	voidFunction := program.NewFunction("void")
	voidFunction.GetOrCreateBuilder().EmitReturn(nil)
	voidFunction.Finish()

	RefineReturnTypes(program)
	if got := pointerFunction.GetType().String(); !strings.Contains(got, "-> ptr64") {
		t.Fatalf("pointer return type = %s", got)
	}
	if got := valueFunction.GetType().String(); !strings.Contains(got, "-> i64") {
		t.Fatalf("integer return type = %s", got)
	}
	if got := voidFunction.GetType().String(); !strings.Contains(got, "-> null") {
		t.Fatalf("void return type = %s", got)
	}
}
