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

func TestSyntaxFlowNestedDataflowContext(t *testing.T) {
	prog, err := Parse(`a = source(); b = a + "suffix"; if cond { b = b + a }; sink(b); sink(a)`)
	require.NoError(t, err)
	result, err := prog.SyntaxFlowWithError(`
 source() as $extraValue
 sink(* as $param)
 $param?{<self> #{include: <<<CODE
* & $extraValue
CODE}->} as $sink
 $sink<dataflow(include=<<<CODE
* & $extraValue as $__next__
CODE,exclude=<<<CODE
*?{opcode: call} as $__next__
CODE)> as $high
 $sink<dataflow(include=<<<CODE
* & $extraValue as $__next__
CODE,exclude=<<<CODE
<self>?{opcode: call && <self><getCallee> & $function} as $__next__
CODE)> as $highAndMid
 alert $high
 `)
	require.NoError(t, err)
	require.NotNil(t, result)
}

func TestSyntaxFlowNestedQuerySharesRuleLifetime(t *testing.T) {
	prog, err := Parse("a = 1")
	require.NoError(t, err)
	var childContext context.Context
	sfvm.RegisterNativeCall("testCaptureRuleContext", func(v sfvm.Values, f *sfvm.SFFrame, _ *sfvm.NativeCallActualParams) (bool, sfvm.Values, error) {
		childContext = f.GetContext()
		return true, v, nil
	})
	sfvm.RegisterNativeCall("testNestedRuleContext", func(v sfvm.Values, f *sfvm.SFFrame, _ *sfvm.NativeCallActualParams) (bool, sfvm.Values, error) {
		_, err := prog.SyntaxFlowWithError("a<testCaptureRuleContext>", QueryWithSFConfig(f.GetConfig()))
		require.NoError(t, err)
		require.NotNil(t, childContext)
		require.NoError(t, childContext.Err(), "nested result contexts must remain usable until the owning rule ends")
		return true, v, nil
	})
	_, err = prog.SyntaxFlowWithError("a<testNestedRuleContext>")
	require.NoError(t, err)
	require.ErrorIs(t, childContext.Err(), context.Canceled, "rule exit releases the whole query lifetime")
}
