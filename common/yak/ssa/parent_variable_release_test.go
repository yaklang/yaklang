package ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// A released variable keeps its id and reloads through GetValue. A closure that
// reads its parent scope must use that read path: after ReleasePersistedOrdinaryValues
// the bare Value field is nil while the scope entry is still there.
//
// Returning (nil, true) from getParentFunctionVariable fed that nil into
// Parameter.SetDefault, which dereferenced it (panic count 7-23 per Hue run).
func TestGetParentFunctionVariableReloadsReleasedValue(t *testing.T) {
	prog := newSplitWriteProgram(t)
	// @main stays unfinished for the whole compile, so its builder and scope are
	// kept while its already-saved ordinary values are released. That is the
	// state the panic came from: the scope entry is readable, the value is not.
	mainBuilder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	value := mainBuilder.EmitConstInst(7)
	variable := mainBuilder.CreateVariable("captured")
	mainBuilder.AssignVariable(variable, value)
	parentScope := mainBuilder.CurrentBlock.ScopeTable
	require.NotNil(t, parentScope)
	originalID := value.GetId()

	prog.Cache.FlushCompileUnit("unit")
	prog.Cache.FlushInstructionSaver()
	require.True(t, prog.Cache.instructions.isPersisted(originalID), "the value must be saved before it can be released")

	require.Greater(t, prog.ReleasePersistedOrdinaryValues(), 0)
	require.Nil(t, variable.Value, "release must drop the strong value and keep the id")
	require.NotNil(t, parentScope, "the live container scope stays")

	child := prog.NewFunction("childFn")
	childBuilder := child.GetOrCreateBuilder()
	childBuilder.parentScope = &ParentScope{scope: parentScope}

	got, ok := childBuilder.getParentFunctionVariable("captured")
	require.True(t, ok, "the parent scope still holds the variable")
	require.NotNil(t, got, "a released parent value must reload instead of reporting a nil value as found")
	require.Equal(t, originalID, got.GetId())
}

// SetDefault binds a value into a free value. A released parent variable used to
// reach it as a nil interface, so the call itself must stay safe.
func TestSetDefaultIgnoresNilValue(t *testing.T) {
	prog := newSplitWriteProgram(t)
	fn := prog.NewFunction("withParam")
	builder := fn.GetOrCreateBuilder()
	param := builder.NewParam("p")
	require.NotPanics(t, func() { param.SetDefault(nil) })
}
