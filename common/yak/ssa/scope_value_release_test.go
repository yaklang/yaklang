package ssa

import (
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
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

func TestReleaseKeepsExportedAndCapturedValues(t *testing.T) {
	exportedFn, exportedVar, exportedValue := assignLocal(t, "Helper")
	persistFinished(t, exportedFn)
	require.Equal(t, 0, exportedFn.GetProgram().ReleasePersistedOrdinaryValues())
	require.True(t, exportedVar.GetValue() == exportedValue)

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
