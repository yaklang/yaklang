package ssapass

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core/ssa2source"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func TestInferPointerFromMallocAndFree(t *testing.T) {
	program, function, builder := newProgram(t, "caller")
	allocation := builder.EmitCall(builder.NewCall(extern(program, "malloc"), []ssa.Value{builder.EmitConstInst(int64(8))}))
	zero := builder.EmitConstInst(int64(0))
	builder.EmitBinOp(ssa.OpEq, allocation, zero)
	builder.EmitCall(builder.NewCall(extern(program, "free"), []ssa.Value{allocation}))
	builder.EmitReturn(nil)
	function.Finish()

	ApplyFunctionDefine(program)
	InferTypes(program)
	if allocation.GetType().String() != "ptr64" {
		t.Fatalf("malloc result type = %s", allocation.GetType())
	}
	if zero.GetType().String() != "ptr64" {
		t.Fatalf("zero type = %s", zero.GetType())
	}
	text, err := ssa2source.Render(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "malloc(") || !strings.Contains(text, "free(") {
		t.Fatalf("source lacks inferred pointer values:\n%s", text)
	}
}
