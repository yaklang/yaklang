package ssa2source

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa"
)

func testProgram(t *testing.T) *ssa.Program {
	t.Helper()
	program := ssa.NewProgram(nil, ssa.ProgramCacheMemory, ssa.Application, nil, "", 0)
	function := program.NewFunction("sub_0x1000")
	builder := function.GetOrCreateBuilder()
	one := builder.EmitConstInst(1)
	two := builder.EmitConstInst(2)
	sum := builder.EmitBinOp(ssa.OpAdd, one, two)
	builder.EmitReturn(ssa.Values{sum})
	function.Finish()
	return program
}

func TestRenderArithmeticAndReturn(t *testing.T) {
	text, err := Render(context.Background(), testProgram(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"function sub_0x1000()", "1 + 2", "return"} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("source lacks %q:\n%s", fragment, text)
		}
	}
}

func TestRenderBranch(t *testing.T) {
	program := ssa.NewProgram(nil, ssa.ProgramCacheMemory, ssa.Application, nil, "", 0)
	function := program.NewFunction("sub_0x2000")
	builder := function.GetOrCreateBuilder()
	entry := builder.CurrentBlock
	yes := function.NewBasicBlock("yes")
	no := function.NewBasicBlock("no")
	yes.SetScope(ssa.NewScope(function, program.GetProgramName()))
	no.SetScope(ssa.NewScope(function, program.GetProgramName()))
	condition := builder.EmitConstInst(true)
	ifInstruction := builder.EmitIf()
	ifInstruction.Cond = condition.GetId()
	ifInstruction.True = yes.GetId()
	ifInstruction.False = no.GetId()
	entry.AddSucc(yes)
	entry.AddSucc(no)
	builder.CurrentBlock = yes
	builder.EmitReturn(ssa.Values{builder.EmitConstInst(1)})
	builder.CurrentBlock = no
	builder.EmitReturn(nil)
	function.Finish()

	text, err := Render(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"if (", "goto yes", "goto no", "return;"} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("source lacks %q:\n%s", fragment, text)
		}
	}
}

func TestRenderRejectsNilAndCancellation(t *testing.T) {
	if _, err := Render(context.Background(), nil); err == nil {
		t.Fatal("nil program accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Render(ctx, testProgram(t)); err == nil {
		t.Fatal("cancellation ignored")
	}
}

func TestRendererSatisfiesContract(t *testing.T) {
	type contract interface {
		RenderSource(context.Context, *ssa.Program) (string, error)
	}
	var _ contract = Renderer{}
	if text, err := (Renderer{}).RenderSource(context.Background(), testProgram(t)); err != nil || text == "" {
		t.Fatalf("renderer contract: %v %q", err, text)
	}
}
