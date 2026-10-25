package ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// If the current block already has a terminator, EmitErrorHandler returns nil.
// TryBuilder.Finish must still tolerate syntactically present but unreachable
// try/except/finally code. Hue's deferred Python build hit this in txipc.py.
func TestTryBuilderFinishWithFinishedEntryBlock(t *testing.T) {
	prog := newDeferredBuildTestProgram(t, "try-builder-finished-entry")
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	require.NotNil(t, builder.CurrentBlock)
	builder.CurrentBlock.finish = true

	tb := builder.BuildTry()
	tb.BuildTryBlock(func() {
		builder.EmitConstInst("unreachable try body")
	})
	tb.BuildErrorCatch(func() string { return "exc" }, func() {
		builder.EmitConstInst("unreachable catch body")
	})
	tb.BuildFinally(func() {
		builder.EmitConstInst("unreachable finally body")
	})

	require.NotPanics(t, func() { tb.Finish() })
}
