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
	one := builder.EmitUndefined("left")
	two := builder.EmitUndefined("right")
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
	for _, fragment := range []string{"void sub_0x1000(void)", " + ", "return"} {
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
	for _, fragment := range []string{"if (true)", "} else {", "return 0x1;", "return;"} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("source lacks %q:\n%s", fragment, text)
		}
	}
	if strings.Contains(text, "goto ") || strings.Contains(text, "phi(") {
		t.Fatalf("source kept unstructured control flow or phi:\n%s", text)
	}
}

func TestRenderCastOfConstantIsInlined(t *testing.T) {
	program := ssa.NewProgram(nil, ssa.ProgramCacheMemory, ssa.Application, nil, "", 0)
	function := program.NewFunction("casts")
	builder := function.GetOrCreateBuilder()
	value := builder.EmitConstInst(int64(7))
	cast := builder.EmitTypeCast(value, ssa.NewBasicType(ssa.NumberTypeKind, "i32"))
	builder.EmitReturn(ssa.Values{cast})
	function.Finish()

	text, err := Render(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "return 0x7;") {
		t.Fatalf("constant cast was not folded:\n%s", text)
	}
	if strings.Contains(text, "cast(") || strings.Contains(text, "type-case") {
		t.Fatalf("temporary cast survived:\n%s", text)
	}
}

func TestRenderOpaqueMarker(t *testing.T) {
	program := ssa.NewProgram(nil, ssa.ProgramCacheMemory, ssa.Application, nil, "", 0)
	function := program.NewFunction("markers")
	builder := function.GetOrCreateBuilder()
	marker := builder.EmitConstInst("x86.flags.add")
	cast := builder.EmitTypeCast(marker, ssa.NewBasicType(ssa.NumberTypeKind, "i64"))
	builder.EmitReturn(ssa.Values{cast})
	function.Finish()

	text, err := Render(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "0x0 /* x86.flags.add */") {
		t.Fatalf("opaque marker is not inlined:\n%s", text)
	}
}

func TestRenderHexConstantWithComment(t *testing.T) {
	program := ssa.NewProgram(nil, ssa.ProgramCacheMemory, ssa.Application, nil, "", 0)
	function := program.NewFunction("hex")
	builder := function.GetOrCreateBuilder()
	value := builder.EmitConstInst(int64(0x401136))
	value.SetType(ssa.NewBasicType(ssa.NumberTypeKind, "i64"))
	value.SetVerboseName(`/* "main" */`)
	cast := builder.EmitTypeCast(value, ssa.NewBasicType(ssa.NumberTypeKind, "ptr64"))
	builder.EmitReturn(ssa.Values{cast})
	function.Finish()

	text, err := Render(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, `0x401136 /* "main" */`) {
		t.Fatalf("hex constant or comment is missing:\n%s", text)
	}
	if strings.Contains(text, "4198710") {
		t.Fatalf("decimal constant survived:\n%s", text)
	}
}

func TestRenderDropsUnusedCallAssignment(t *testing.T) {
	program := ssa.NewProgram(nil, ssa.ProgramCacheMemory, ssa.Application, nil, "", 0)
	function := program.NewFunction("caller")
	builder := function.GetOrCreateBuilder()
	target := program.NewFunction("puts")
	target.SetExtern(true)
	builder.EmitCall(builder.NewCall(target, nil))
	builder.EmitReturn(nil)
	function.Finish()

	text, err := Render(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "  puts();") {
		t.Fatalf("unused call keeps an assignment:\n%s", text)
	}
	if strings.Contains(text, "= puts();") {
		t.Fatalf("unused call keeps an assignment:\n%s", text)
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

func TestRenderFoldsCastsAndIdentities(t *testing.T) {
	program := ssa.NewProgram(nil, ssa.ProgramCacheMemory, ssa.Application, nil, "", 0)
	function := program.NewFunction("fold")
	builder := function.GetOrCreateBuilder()
	value := builder.EmitUndefined("x")
	i32 := ssa.NewBasicType(ssa.NumberTypeKind, "i32")
	i64 := ssa.NewBasicType(ssa.NumberTypeKind, "i64")
	cast32 := builder.EmitTypeCast(value, i32)
	cast64 := builder.EmitTypeCast(cast32, i64)
	back := builder.EmitTypeCast(cast64, i32)
	zero := builder.EmitBinOp(ssa.OpXor, cast32, cast32)
	and := builder.EmitBinOp(ssa.OpAnd, cast32, cast32)
	mul := builder.EmitBinOp(ssa.OpMul, cast32, builder.EmitConstInst(int64(1)))
	builder.EmitReturn(ssa.Values{back, zero, and, mul})
	function.Finish()

	text, err := Render(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "uint32_t)((uint64_t)") || strings.Contains(text, "uint64_t)((uint32_t)") {
		t.Fatalf("redundant casts survived:\n%s", text)
	}
	if strings.Contains(text, " ^ ") {
		t.Fatalf("self-xor was not folded:\n%s", text)
	}
}
