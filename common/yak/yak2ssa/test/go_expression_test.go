package test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func TestCoreGoExpressionSSA(t *testing.T) {
	for _, tc := range []struct {
		source      string
		workerCalls int
	}{
		{`go 1+1`, 0},
		{`go sink()+1`, 1},
		{`go func(){sink()}`, 1},
		{`go ((func(){sink()}))`, 1},
		{`go ()=>sink()`, 1},
		{`go func task(){sink()}`, 1},
		{`factory=()=>{sink(); return ()=>sink()}; go factory()`, 1},
		{`factory=()=>{sink(); return ()=>sink()}; go ((factory()))`, 1},
		{`go func(){sink(); return ()=>sink()}`, 1},
		{`f=()=>sink(); go f`, 0},
		{`a=[()=>sink()]; go a[0]`, 0},
	} {
		t.Run(tc.source, func(t *testing.T) {
			prog, err := ssaapi.Parse(`sink=()=>1; ` + tc.source)
			require.NoError(t, err)
			require.Empty(t, prog.GetErrors())
			main := prog.Program.GetFunction(string(ssa.MainFunctionName), "")
			require.NotNil(t, main)
			calls := coreFunctionCalls(main)
			require.Len(t, calls, 1, "the submitting function must only invoke one worker")
			require.True(t, calls[0].Async)
			method, ok := main.GetValueById(calls[0].Method)
			require.True(t, ok)
			worker, ok := ssa.ToFunction(method)
			require.True(t, ok)
			workerCalls := coreFunctionCalls(worker)
			require.Len(t, workerCalls, tc.workerCalls, "do not call a returned closure or function-valued expression")
			for _, call := range workerCalls {
				require.False(t, call.Async, "nested expression calls belong to the worker body")
			}
		})
	}
}

func coreFunctionCalls(fn *ssa.Function) []*ssa.Call {
	var calls []*ssa.Call
	for _, id := range fn.Blocks {
		block, ok := fn.GetBasicBlockByID(id)
		if !ok {
			continue
		}
		for _, instruction := range block.GetInstructionsByIDs(block.Insts) {
			if call, ok := ssa.ToCall(instruction); ok {
				calls = append(calls, call)
			}
		}
	}
	return calls
}
