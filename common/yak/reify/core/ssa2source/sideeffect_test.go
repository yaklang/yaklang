package ssa2source

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// TestRenderReportsSideEffect keeps the renderer honest: reify models no
// cross-procedure side effects, so a node here means the conversion broke an
// invariant. The renderer reports the node in a comment and keeps the rest of the
// function, because a diagnostic must not abort a whole program.
func TestRenderReportsSideEffect(t *testing.T) {
	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName("side-effect"))
	if err != nil {
		t.Fatal(err)
	}
	program := ssa.NewProgram(cfg, ssa.ProgramCacheMemory, ssa.Application, nil, "", 0)
	function := program.NewFunction("injected")
	builder := function.GetOrCreateBuilder()
	if builder == nil {
		t.Fatal("no builder")
	}
	value := builder.EmitConstInst(1)
	if value == nil {
		t.Fatal("no constant")
	}
	call := ssa.NewCall(value, nil, nil, builder.CurrentBlock)
	if call == nil {
		t.Fatal("no call")
	}
	builder.EmitCall(call)
	if builder.EmitSideEffect("injected", call, value) == nil {
		t.Fatal("no side effect")
	}
	builder.EmitReturn([]ssa.Value{value})

	source, renderErr := Render(context.Background(), program)
	if renderErr != nil {
		t.Fatalf("render failed: %v", renderErr)
	}
	if !strings.Contains(source, "/* unexpected side effect: injected */") {
		t.Fatalf("side effect was not reported:\n%s", source)
	}
	if !strings.Contains(source, "return ") {
		t.Fatalf("rendering stopped at the side effect:\n%s", source)
	}
}
