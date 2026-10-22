package ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildSyntaxBlockRestoresMissingEntryScope(t *testing.T) {
	prog := newDeferredBuildTestProgram(t, "missing-cfg-scope")
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	require.NotNil(t, builder.CurrentBlock)
	builder.CurrentBlock.ScopeTable = nil

	require.NotPanics(t, func() {
		builder.BuildSyntaxBlock(func() {})
	})
	require.NotNil(t, builder.CurrentBlock.ScopeTable)
}

func TestIfBuilderKeepsScopeAfterFinishedEntry(t *testing.T) {
	prog := newDeferredBuildTestProgram(t, "finished-if-entry")
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	require.NotNil(t, builder.CurrentBlock)
	builder.CurrentBlock.finish = true

	require.NotPanics(t, func() {
		builder.CreateIfBuilder().AppendItem(
			func() Value { return builder.EmitConstInst(true) },
			func() {},
		).Build()
	})
	require.NotNil(t, builder.CurrentBlock.ScopeTable)
}
