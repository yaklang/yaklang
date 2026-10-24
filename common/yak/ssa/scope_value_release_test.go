package ssa

import (
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func newSplitWriteProgram(t *testing.T) *Program {
	t.Helper()
	programName := uuid.NewString()
	t.Cleanup(func() {
		ssadb.DeleteProgram(ssadb.GetDB(), programName)
	})
	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName(programName))
	require.NoError(t, err)
	cfg.SetCompileUnitSplit(true)
	return NewProgram(cfg, ProgramCacheDBWrite, Application, filesys.NewVirtualFs(), "", 0)
}

func assignLocal(t *testing.T, name string) (*Function, *Variable, Value) {
	t.Helper()
	prog := newSplitWriteProgram(t)
	fn := prog.NewFunction(name)
	builder := fn.GetOrCreateBuilder()
	value := builder.EmitConstInst(1)
	variable := builder.CreateVariable("local")
	builder.AssignVariable(variable, value)
	return fn, variable, value
}

func persistFinished(t *testing.T, fn *Function) {
	t.Helper()
	fn.Finish()
	fn.GetProgram().Cache.FlushCompileUnit("unit")
	fn.GetProgram().Cache.FlushInstructionSaver()
}

func TestReleasePersistedOrdinaryValueReloadsByID(t *testing.T) {
	fn, variable, value := assignLocal(t, "helper")
	persistFinished(t, fn)
	originalID := value.GetId()

	released := fn.GetProgram().ReleasePersistedOrdinaryValues()

	require.Greater(t, released, 0)
	require.Nil(t, variable.Value, "the scope must drop the strong value after a successful save barrier")
	reloaded := variable.GetValue()
	require.NotNil(t, reloaded)
	require.Equal(t, originalID, reloaded.GetId())
	require.False(t, reloaded == value, "reload must not resurrect the detached object")

	var wg sync.WaitGroup
	ids := make([]int64, 8)
	wg.Add(len(ids))
	for i := range ids {
		go func(i int) {
			defer wg.Done()
			got := variable.GetValue()
			if got != nil {
				ids[i] = got.GetId()
			}
		}(i)
	}
	wg.Wait()
	for _, id := range ids {
		require.Equal(t, originalID, id)
	}
}

func TestReleasePersistedOrdinaryValueVersions(t *testing.T) {
	prog := newSplitWriteProgram(t)
	fn := prog.NewFunction("helper")
	builder := fn.GetOrCreateBuilder()
	var variables []*Variable
	var ids []int64
	for i := 0; i < 3; i++ {
		value := builder.EmitConstInst(i)
		variable := builder.CreateVariableForce("local")
		builder.AssignVariable(variable, value)
		if i > 0 {
			require.NotSame(t, variables[i-1], variable, "fixture must create distinct lexical versions")
		}
		variables = append(variables, variable)
		ids = append(ids, value.GetId())
	}
	require.Len(t, builder.CurrentBlock.ScopeTable.GetAllVariablesByName("local", true), len(variables))
	persistFinished(t, fn)
	require.Equal(t, len(variables), prog.ReleasePersistedOrdinaryValues(), "release must visit every retained version, not only the newest")
	for _, variable := range variables {
		require.Nil(t, variable.Value, "older versions must not retain concrete IR through linkValues")
	}
	for i, variable := range variables {
		reloaded := variable.GetValue()
		require.NotNil(t, reloaded)
		require.Equal(t, ids[i], reloaded.GetId(), "historical variable lookup must keep its own saved instruction")
	}
	require.Nil(t, builder.CurrentBlock.ScopeTable, "a finished body with no closure capture must drop its scope table")
}

func TestReleaseTrimsSettledOrdinaryVersionChain(t *testing.T) {
	prog := newSplitWriteProgram(t)
	fn := prog.NewFunction("helper")
	builder := fn.GetOrCreateBuilder()
	var variables []*Variable
	var ids []int64
	for i := 0; i < 3; i++ {
		value := builder.EmitConstInst(i)
		variable := builder.CreateVariableForce("local")
		builder.AssignVariable(variable, value)
		variables = append(variables, variable)
		ids = append(ids, value.GetId())
	}
	capturedValue := builder.EmitConstInst("captured")
	captured := builder.CreateVariableForce("captured")
	builder.AssignVariable(captured, capturedValue)
	noteCapturedByClosure(capturedValue, "captured")
	persistFinished(t, fn)

	require.Equal(t, len(variables), prog.ReleasePersistedOrdinaryValues())
	remaining := builder.CurrentBlock.ScopeTable.GetAllVariablesByName("local", true)
	require.Len(t, remaining, 1, "persisted ordinary history must leave the scope chain")
	require.Same(t, variables[len(variables)-1], remaining[0], "the lexical head stays reachable")
	for i, variable := range variables {
		reloaded := variable.GetValue()
		require.NotNil(t, reloaded)
		require.Equal(t, ids[i], reloaded.GetId(), "a caller holding an older version still reloads that saved instruction")
	}
	capturedVersions := builder.CurrentBlock.ScopeTable.GetAllVariablesByName("captured", true)
	require.Len(t, capturedVersions, 1)
	require.Same(t, captured, capturedVersions[0], "a captured version stays in the scope chain")
	require.Same(t, capturedValue, captured.GetValue())
}

func TestReleaseKeepsCapturedHistoricalVersion(t *testing.T) {
	prog := newSplitWriteProgram(t)
	fn := prog.NewFunction("Helper")
	builder := fn.GetOrCreateBuilder()
	capturedValue := builder.EmitConstInst("captured")
	captured := builder.CreateVariable("local")
	builder.AssignVariable(captured, capturedValue)
	ordinaryValue := builder.EmitConstInst("ordinary")
	ordinary := builder.CreateVariableForce("local")
	builder.AssignVariable(ordinary, ordinaryValue)
	require.NotSame(t, captured, ordinary)
	noteCapturedByClosure(capturedValue, "local")
	persistFinished(t, fn)

	require.Equal(t, 1, prog.ReleasePersistedOrdinaryValues())
	require.Same(t, capturedValue, captured.GetValue(), "a historical closure binding keeps its concrete value")
	require.Nil(t, ordinary.Value, "an unrelated ordinary version can still be detached")
	require.Equal(t, ordinaryValue.GetId(), ordinary.GetValue().GetId())
}

func TestReleaseSavedLocalsFromResidentFastPath(t *testing.T) {
	prog := newResidentDeltaProgram(t)
	defer closeUnsavedFlushFixture(t, prog)()
	builder := prog.GetAndCreateFunctionBuilder("", "Helper")
	value := builder.EmitConstInst("local")
	variable := builder.CreateVariable("local")
	builder.AssignVariable(variable, value)
	builder.Finish()
	require.NotNil(t, prog.Cache.instructions.residentCache)
	require.NoError(t, prog.Cache.instructions.residentCache.Flush(false))
	require.True(t, prog.Cache.hasResidentInstruction(value.GetId()))

	require.Greater(t, prog.ReleasePersistedOrdinaryValues(), 0)
	require.Nil(t, variable.Value)
	require.False(t, prog.Cache.hasResidentInstruction(value.GetId()), "detaching the scope must also drop clean fast-path residency")
	require.Equal(t, value.GetId(), variable.GetValue().GetId())
	require.False(t, variable.GetValue() == value)
}

func TestReleaseDropsOffsetPointerAndReloadsByID(t *testing.T) {
	fn, variable, value := assignLocal(t, "helper")
	prog := fn.GetProgram()
	editor := memedit.NewMemEditor("local")
	prog.SetOffsetVariable(variable, editor.GetFullRange())
	persistFinished(t, fn)
	end := editor.GetFullRange().GetEndOffset()
	prog.offsetMu.RLock()
	item := prog.OffsetMap[end]
	prog.offsetMu.RUnlock()
	require.NotNil(t, item)

	released := prog.ReleasePersistedOrdinaryValues()
	require.Greater(t, released, 0)
	require.Nil(t, item.value, "the offset table must not keep the saved value object")
	reloaded := item.GetValue()
	require.NotNil(t, reloaded)
	require.Equal(t, value.GetId(), reloaded.GetId())
	require.False(t, reloaded == value)
}

func TestReleaseKeepsExportedAndCapturedValues(t *testing.T) {
	exportedFn, exportedVar, exportedValue := assignLocal(t, "Helper")
	param := exportedFn.builder.NewParam("arg")
	persistFinished(t, exportedFn)
	originalID := exportedValue.GetId()

	released := exportedFn.GetProgram().ReleasePersistedOrdinaryValues()
	require.Greater(t, released, 0, "a finished exported function must drop its saved ordinary locals")
	require.Nil(t, exportedVar.Value)
	reloaded := exportedVar.GetValue()
	require.NotNil(t, reloaded)
	require.Equal(t, originalID, reloaded.GetId())
	require.False(t, reloaded == exportedValue, "reload must not keep the detached local")
	require.Nil(t, exportedFn.builder, "a finished exported body must not keep its builder")
	if variable := param.GetLastVariable(); variable != nil {
		require.Nil(t, variable.GetScope(), "a kept parameter must not pin the finished body scope")
	}
	require.NotZero(t, exportedFn.EnterBlock)
	require.False(t, exportedFn.GetProgram().Cache.hasResidentInstruction(exportedFn.EnterBlock), "a finished exported body block must leave the resident cache")
	require.True(t, exportedFn.GetProgram().Cache.hasResidentInstruction(exportedFn.GetId()), "the function object stays resident for cross-unit calls")
	require.False(t, exportedFn.GetProgram().Cache.hasResidentInstruction(param.GetId()), "a finished exported parameter is reloaded by id")
	gotParam, ok := exportedFn.GetValueById(param.GetId())
	require.True(t, ok)
	require.Equal(t, param.GetId(), gotParam.GetId())

	capturedFn, capturedVar, capturedValue := assignLocal(t, "helper")
	noteCapturedByClosure(capturedValue, "local")
	persistFinished(t, capturedFn)
	require.Equal(t, 0, capturedFn.GetProgram().ReleasePersistedOrdinaryValues())
	require.True(t, capturedVar.GetValue() == capturedValue)

	draftFn, draftVar, draftValue := assignLocal(t, "helper")
	draftFn.GetProgram().Cache.FlushCompileUnit("unit")
	draftFn.GetProgram().Cache.FlushInstructionSaver()
	require.Equal(t, 0, draftFn.GetProgram().ReleasePersistedOrdinaryValues())
	require.True(t, draftVar.GetValue() == draftValue)
}

// A captured bit lives on the value. Loading that same value into another
// finished function must not keep the second function's scope.
func TestReleaseDropsScopeThatOnlyMentionsCapturedValue(t *testing.T) {
	prog := newSplitWriteProgram(t)
	owner := prog.NewFunction("owner")
	ownerB := owner.GetOrCreateBuilder()
	shared := ownerB.EmitConstInst("shared")
	ownerVar := ownerB.CreateVariable("shared")
	ownerB.AssignVariable(ownerVar, shared)
	noteCapturedByClosure(shared, "shared")

	other := prog.NewFunction("other")
	otherB := other.GetOrCreateBuilder()
	otherVar := otherB.CreateVariable("shared")
	otherB.AssignVariable(otherVar, shared)
	local := otherB.EmitConstInst("local")
	localVar := otherB.CreateVariable("local")
	otherB.AssignVariable(localVar, local)

	persistFinished(t, owner)
	persistFinished(t, other)
	require.Greater(t, prog.ReleasePersistedOrdinaryValues(), 0)
	require.True(t, ownerVar.GetValue() == shared, "the function that captured the value keeps it")
	require.Nil(t, otherB.CurrentBlock.ScopeTable, "loading a value captured elsewhere must not pin this scope")
}
