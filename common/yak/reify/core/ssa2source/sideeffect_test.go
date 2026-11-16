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
// invariant. Rendering a statement for it would hide that.
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

	_, renderErr := Render(context.Background(), program)
	if renderErr == nil {
		t.Fatal("Render accepted a side effect")
	}
	if !strings.Contains(renderErr.Error(), "side effect") {
		t.Fatalf("unexpected error: %v", renderErr)
	}
}
