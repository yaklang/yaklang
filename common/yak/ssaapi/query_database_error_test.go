package ssaapi

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func TestSyntaxFlowSurfacesDatabaseCancellation(t *testing.T) {
	called := false
	sfvm.RegisterNativeCall("testDatabaseCancellation", func(v sfvm.Values, frame *sfvm.SFFrame, _ *sfvm.NativeCallActualParams) (bool, sfvm.Values, error) {
		called = true
		ctx, cancel := context.WithCancel(frame.GetContext())
		cancel()
		for range ssadb.SearchVariable(ssadb.GetDB(), ctx, "cancelled-rule", nil, ssadb.ExactCompare, ssadb.ConstType, "value") {
			t.Error("unexpected query result")
		}
		return true, v, nil // the legacy channel API cannot return the SQL error
	})
	prog, err := Parse("a = 1")
	require.NoError(t, err)
	result, err := prog.SyntaxFlowWithError(`a<testDatabaseCancellation> as $result`)
	require.True(t, called)
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorContains(t, err, "SSA database search failed")
	require.Nil(t, result, "database failure must not be a successful empty scan")
}

func TestSyntaxFlowQueryRestoresSharedVMContext(t *testing.T) {
	prog, err := Parse("a = 1")
	require.NoError(t, err)
	vm := sfvm.NewSyntaxFlowVirtualMachine()
	parent := vm.GetConfig().GetContext()
	for i := 0; i < 2; i++ {
		result, err := prog.SyntaxFlowWithError(`a as $value`, QueryWithVM(vm))
		require.NoError(t, err)
		require.Len(t, result.GetValues("value"), 1)
		require.Equal(t, parent, vm.GetConfig().GetContext())
		require.NoError(t, parent.Err())
	}
}
