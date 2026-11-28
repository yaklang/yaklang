package ssa2source

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssairtext"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestStructurerLargeDiamondCFGPerformance(t *testing.T) {
	var b strings.Builder
	b.WriteString("big_cfg_1500 <i1> (1) cond\n")
	b.WriteString("type: (i1) -> i64\n")
	b.WriteString("entry_0: (<nil condition>)\n")
	b.WriteString("<i64> t2 = <i64> 1\n")
	const diamonds = 500 // 1 + 500*3 = 1501 blocks
	for i := 0; i < diamonds; i++ {
		fmt.Fprintf(&b, "If [<i1> cond] true -> t%d, false -> f%d\n", i, i)
		fmt.Fprintf(&b, "t%d:\n\tjump -> j%d\n", i, i)
		fmt.Fprintf(&b, "f%d:\n\tjump -> j%d\n", i, i)
		fmt.Fprintf(&b, "j%d:\n", i)
	}
	b.WriteString("ret <i64> t2\n")

	program, err := ssairtext.Parse(b.String())
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	text, err := Render(context.Background(), program)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if text == "" {
		t.Fatal("Render returned empty output")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("1500-block Render took %v, expected < 2s", elapsed)
	}
}

func TestSingleBlockSelfLoopNoBodyDuplication(t *testing.T) {
	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName("self-loop"))
	if err != nil {
		t.Fatal(err)
	}
	program := ssa.NewProgram(cfg, ssa.ProgramCacheMemory, ssa.Application, nil, "", 0)
	fn := program.NewFunction("self_loop")
	b := fn.GetOrCreateBuilder()
	entry := b.CurrentBlock
	loop := fn.NewBasicBlock("loop")
	exit := fn.NewBasicBlock("exit")
	loop.SetScope(ssa.NewScope(fn, program.GetProgramName()))
	exit.SetScope(ssa.NewScope(fn, program.GetProgramName()))

	b.CurrentBlock = entry
	zero := b.EmitConstInst(0)
	one := b.EmitConstInst(1)
	limit := b.EmitConstInst(10)
	b.EmitJump(loop)

	b.CurrentBlock = loop
	phi := ssa.NewPhi(loop, "i")
	phi.SetType(ssa.NewBasicType(ssa.NumberTypeKind, "i64"))
	loop.Phis = append(loop.Phis, phi.GetId())
	program.SetVirtualRegister(phi)
	next := b.EmitBinOp(ssa.OpAdd, phi, one)
	cond := b.EmitBinOp(ssa.OpLt, next, limit)
	phi.Edge = []int64{zero.GetId(), next.GetId()}
	zero.AddUser(phi)
	next.AddUser(phi)
	ifInst := b.EmitIf()
	ifInst.Cond = cond.GetId()
	cond.AddUser(ifInst)
	ifInst.True = loop.GetId()
	ifInst.False = exit.GetId()
	loop.AddSucc(loop)
	loop.AddSucc(exit)

	b.CurrentBlock = exit
	b.EmitReturn([]ssa.Value{next})
	fn.Finish()

	src, err := Render(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(src, "while (true) {") || !strings.Contains(src, "break;") {
		t.Fatalf("expected structured while loop with break:\n%s", src)
	}
	if count := strings.Count(src, "+ 0x1;"); count != 1 {
		t.Fatalf("loop body duplicated (count=%d):\n%s", count, src)
	}
}

func TestInlineFrameBaseAliasesWordBoundary(t *testing.T) {
	lines := []string{
		"void fn(uint64_t arg0) {",
		"    v1 = entry_rsp - 0x10;",
		"    v10 = v1 + 0x8;",
		"    v11 = v10 + arg0;",
		"    return;",
		"}",
	}
	got := strings.Join(inlineFrameBaseAliases(lines), "\n")
	if strings.Contains(got, "entry_rsp0") || strings.Contains(got, "entry_rsp1") {
		t.Fatalf("inlineFrameBaseAliases corrupted v10/v11:\n%s", got)
	}
	if !strings.Contains(got, "v10 = entry_rsp + 0x8;") {
		t.Fatalf("expected v1 replaced inside v10:\n%s", got)
	}
}

func TestCleanSourceInsertsForwardDeclInsideFunctionBody(t *testing.T) {
	src := "uint64_t fn(uint64_t arg0) {\n    if (v9 != 0x0) {\n        return v9;\n    }\n    v9 = arg0 + 0x2;\n    return v9;\n}"
	program, err := ssairtext.Parse("fn <i64> (1) a0\ntype: (i64) -> i64\nentry_0: (<nil condition>)\n\tret <i64> a0")
	if err != nil {
		t.Fatal(err)
	}
	fn := program.Funcs.Values()[0]
	cleaned, err := cleanSource(fn, src)
	if err != nil {
		t.Fatalf("cleanSource failed: %v", err)
	}
	openIdx := strings.Index(cleaned, ") {")
	declIdx := strings.Index(cleaned, "forward declaration")
	if openIdx < 0 || declIdx < openIdx {
		t.Fatalf("forward declaration placed before function header:\n%s", cleaned)
	}
}
