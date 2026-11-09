package ssapass

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core/ssa2source"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func TestSimplifyPartialRegisterMerge(t *testing.T) {
	program := ssa.NewProgram(nil, ssa.ProgramCacheMemory, ssa.Application, nil, "", 0)
	function := program.NewFunction("partial")
	builder := function.GetOrCreateBuilder()
	old := builder.EmitConstInst(int64(0x41))
	old.SetType(ssa.NewBasicType(ssa.NumberTypeKind, "i64"))
	high := builder.EmitBinOp(ssa.OpAndNot, old, builder.EmitConstInst(int64(0xff)))
	low := builder.EmitBinOp(ssa.OpAnd, builder.EmitUndefined("low"), builder.EmitConstInst(int64(0xff)))
	merged := builder.EmitBinOp(ssa.OpOr, high, low)
	cast := builder.EmitTypeCast(merged, ssa.NewBasicType(ssa.NumberTypeKind, "i64"))
	builder.EmitReturn(ssa.Values{cast})
	function.Finish()

	SimplifyPartialWrites(program)
	text, err := ssa2source.Render(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "and-not 0xff") {
		t.Fatalf("partial register merge survived:\n%s", text)
	}
}
