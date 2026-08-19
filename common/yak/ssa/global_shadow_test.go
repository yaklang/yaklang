package ssa

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// TestLocalShadowDoesNotOverwriteGlobalStaticMember reproduces the review A6
// hazard: a function-local variable that shadows a global name must not
// overwrite the GlobalVariablesBlueprint StaticMember, or later functions
// would read the local value as the global one.
func TestLocalShadowDoesNotOverwriteGlobalStaticMember(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName(programName))
	require.NoError(t, err)
	prog := NewProgram(cfg, ProgramCacheMemory, Application, filesys.NewVirtualFs(), "", 0)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))

	// Register the real global "x" with a distinct value.
	globalVal := builder.EmitUndefined("global-x")
	globalVal.SetName("x")
	builder.AddGlobalVariable("x", func() Value { return globalVal })

	// Global must be registered BEFORE the local assignment so the hazard is
	// real: at assign time StaticMember already contains "x", and the old code
	// would overwrite it with the local value.
	prog.GlobalVariablesBlueprint.Build()

	// A function-local "x" shadows the global name inside one function.
	local := builder.CreateLocalVariable("x")
	localVal := builder.EmitUndefined("local-x")
	localVal.SetName("x")
	builder.AssignVariable(local, localVal)

	got, ok := prog.GetGlobalVariable("x")
	require.True(t, ok, "global x must still be registered")
	require.Equal(t, globalVal.GetId(), got.GetId(),
		"a local shadow must not overwrite the global StaticMember")
}
