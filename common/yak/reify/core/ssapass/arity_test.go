package ssapass

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa"
)

func TestTrimInternalCallsUsesRecoveredArity(t *testing.T) {
	program := ssa.NewProgram(nil, ssa.ProgramCacheMemory, ssa.Application, nil, "", 0)
	caller := program.NewFunction("caller")
	builder := caller.GetOrCreateBuilder()

	zero := program.NewFunction("zero")
	zero.GetOrCreateBuilder().EmitReturn(nil)
	zero.Finish()

	six := program.NewFunction("six")
	sixBuilder := six.GetOrCreateBuilder()
	params := make(ssa.Values, 0, 6)
	for index := 0; index < 6; index++ {
		param := ssa.NewParam("arg", false, sixBuilder)
		param.FormalParameterIndex = index
		six.Params = append(six.Params, param.GetId())
		params = append(params, param)
	}
	sixBuilder.EmitReturn(params)
	six.Finish()

	zeroCall := builder.EmitCall(builder.NewCall(zero, []ssa.Value{builder.EmitConstInst(int64(1)), builder.EmitConstInst(int64(2))}))
	sixCall := builder.EmitCall(builder.NewCall(six, []ssa.Value{
		builder.EmitConstInst(int64(1)), builder.EmitConstInst(int64(2)),
		builder.EmitConstInst(int64(3)), builder.EmitConstInst(int64(4)),
		builder.EmitConstInst(int64(5)), builder.EmitConstInst(int64(6)),
		builder.EmitConstInst(int64(7)),
	}))
	builder.EmitReturn(nil)
	caller.Finish()

	TrimInternalCalls(program)
	if len(zeroCall.Args) != 0 {
		t.Fatalf("zero-parameter call args = %d", len(zeroCall.Args))
	}
	if len(sixCall.Args) != 6 {
		t.Fatalf("six-parameter call args = %d", len(sixCall.Args))
	}
}
