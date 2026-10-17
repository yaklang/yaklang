package compiler

import (
	"github.com/yaklang/go-llvm"
	"testing"
)

func TestInstructionParentBlockDynamicTypeCheck(t *testing.T) {
	ctx := llvm.NewContext()
	defer ctx.Dispose()
	mod := ctx.NewModule("parent-check")
	defer mod.Dispose()
	b := ctx.NewBuilder()
	defer b.Dispose()
	i64 := ctx.Int64Type()
	fn := llvm.AddFunction(mod, "f", llvm.FunctionType(i64, []llvm.Type{i64}, false))
	entry := ctx.AddBasicBlock(fn, "entry")
	b.SetInsertPointAtEnd(entry)
	sum := b.CreateAdd(fn.Param(0), llvm.ConstInt(i64, 1, false), "sum")
	for name, v := range map[string]llvm.Value{"nil": {}, "constant": llvm.ConstInt(i64, 1, false), "argument": fn.Param(0), "function": fn} {
		if got := instructionParentBlock(v); !got.IsNil() {
			t.Fatalf("%s has unexpected instruction parent", name)
		}
	}
	if got := instructionParentBlock(sum); got.IsNil() || got.C != entry.C {
		t.Fatal("instruction parent missing")
	}
}
