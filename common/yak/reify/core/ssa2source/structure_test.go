package ssa2source

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestStructurerLargeDiamondCFGPerformance(t *testing.T) {
	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName("large-diamond"))
	if err != nil {
		t.Fatal(err)
	}
	prog := ssa.NewProgram(cfg, ssa.ProgramCacheMemory, ssa.Application, nil, "", 0)
	fn := prog.NewFunction("big_diamond")
	builder := fn.GetOrCreateBuilder()

	const stages = 500
	cur := builder.CurrentBlock
	zero := builder.EmitConstInst(int64(0))
	one := builder.EmitConstInst(int64(1))
	for i := 0; i < stages; i++ {
		thenB := fn.NewBasicBlock("t")
		thenB.SetScope(ssa.NewScope(fn, prog.GetProgramName()))
		elseB := fn.NewBasicBlock("f")
		elseB.SetScope(ssa.NewScope(fn, prog.GetProgramName()))
		joinB := fn.NewBasicBlock("j")
		joinB.SetScope(ssa.NewScope(fn, prog.GetProgramName()))
		builder.CurrentBlock = cur
		cond := builder.EmitBinOp(ssa.OpEq, zero, one)
		ifInst := builder.EmitIf()
		ifInst.Cond = cond.GetId()
		ifInst.True = thenB.GetId()
		ifInst.False = elseB.GetId()
		cur.AddSucc(thenB)
		cur.AddSucc(elseB)
		cur.SetConditionInstID(ifInst.GetId())
		builder.EmitReturn(nil)

		builder.CurrentBlock = thenB
		builder.EmitJump(joinB)
		builder.EmitReturn(nil)

		builder.CurrentBlock = elseB
		builder.EmitJump(joinB)
		builder.EmitReturn(nil)

		cur = joinB
	}
	builder.CurrentBlock = cur
	builder.EmitReturn([]ssa.Value{zero})
	fn.Finish()

	start := time.Now()
	src, err := Render(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("Render took %v on %d-block CFG", elapsed, len(fn.Blocks))
	}
	if !strings.Contains(src, "big_diamond") || !strings.Contains(src, "return 0x0;") {
		t.Fatalf("unexpected source output:\n%s", src[:min(len(src), 300)])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
