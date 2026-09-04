package ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/memedit"
)

func TestAssignVariableAllowsExplicitLocalToShadowExtern(t *testing.T) {
	prog, builder := newTestBuilder(t)
	builder.WithExternValue(map[string]any{"name": "extern"})
	builder.CurrentRange = memedit.NewMemEditor("name").GetFullRange()

	local := builder.CreateLocalVariable("name")
	builder.AssignVariable(local, builder.EmitConstInst("local"))

	for _, err := range prog.GetErrors() {
		require.NotEqual(t, ContAssignExtern("name"), err.Message)
	}
}

func TestAssignVariableStillWarnsWhenOverwritingExtern(t *testing.T) {
	prog, builder := newTestBuilder(t)
	builder.WithExternValue(map[string]any{"name": "extern"})
	builder.CurrentRange = memedit.NewMemEditor("name").GetFullRange()
	require.NotNil(t, builder.TryBuildExternValue("name"))

	variable := builder.CreateVariable("name")
	require.False(t, variable.GetLocal())
	builder.AssignVariable(variable, builder.EmitConstInst("replacement"))

	require.Contains(t, prog.GetErrors().String(), ContAssignExtern("name"))
}
