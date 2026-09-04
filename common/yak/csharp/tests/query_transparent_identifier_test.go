package tests

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func requireQuerySinkMembers(t *testing.T, prog *ssaapi.Program, sink string, names ...string) {
	t.Helper()
	calls := csharpCallsToMethod(t, prog, sink)
	require.Len(t, calls, 1, "%s must be emitted exactly once", sink)
	call := calls[0]
	require.Len(t, call.Args, len(names), "%s must receive every in-scope range value", sink)
	for index, name := range names {
		argument, ok := call.GetValueById(call.Args[index])
		require.True(t, ok)
		_, isMember := ssa.ToParameterMember(argument)
		require.True(t, isMember, "argument %d must be a carrier parameter member, got %T", index, argument)
		require.Contains(t, argument.String(), "."+name,
			"argument %d must be read from transparent member %q, got %s", index, name, argument)
		instruction, ok := argument.(ssa.Instruction)
		require.True(t, ok)
		wrapped, err := prog.NewValue(instruction)
		require.NoError(t, err)
		topDefs := wrapped.GetTopDefs()
		require.NotEmpty(t, topDefs, "carrier member %q must retain a data-flow origin", name)
		hasParameterOrigin := false
		for _, topDef := range topDefs {
			if topDef.IsParameter() {
				hasParameterOrigin = true
				break
			}
		}
		require.True(t, hasParameterOrigin,
			"without a concrete Enumerable implementation, carrier member %q must top-def to its query parameter, got %s",
			name, topDefs)
	}
}

func requireNoQueryRangeCapture(t *testing.T, prog *ssaapi.Program, names ...string) {
	t.Helper()
	forbidden := make(map[string]struct{}, len(names))
	for _, name := range names {
		forbidden[name] = struct{}{}
	}
	prog.Program.EachFunction(func(function *ssa.Function) {
		for variable := range function.FreeValues {
			if _, bad := forbidden[variable.GetName()]; bad {
				require.Failf(t, "query range escaped as free value",
					"function %s captured %q instead of reading it from the transparent query carrier",
					function.GetName(), variable.GetName())
			}
		}
	})
}

func requireSingleCSharpCalls(t *testing.T, prog *ssaapi.Program, names ...string) {
	t.Helper()
	for _, name := range names {
		require.Len(t, csharpCallsToMethod(t, prog, name), 1, "%s must be evaluated exactly once", name)
	}
}

func csharpQueryCalls(prog *ssaapi.Program, methodName string) []*ssa.Call {
	var calls []*ssa.Call
	prog.Program.EachFunction(func(function *ssa.Function) {
		for _, blockID := range function.Blocks {
			block, exists := function.GetBasicBlockByID(blockID)
			if !exists || block == nil {
				continue
			}
			for _, instructionID := range block.Insts {
				instruction, exists := function.GetInstructionById(instructionID)
				if !exists {
					continue
				}
				call, ok := ssa.ToCall(instruction)
				if !ok {
					continue
				}
				callee, exists := call.GetValueById(call.Method)
				if !exists {
					continue
				}
				for _, candidate := range []string{callee.GetName(), callee.GetVerboseName(), callee.String()} {
					if candidate == methodName || strings.HasSuffix(candidate, "."+methodName) || strings.Contains(candidate, "."+methodName+"(") {
						calls = append(calls, call)
						break
					}
				}
			}
		}
	})
	return calls
}

func requireQueryResultSelector(t *testing.T, prog *ssaapi.Program, methodName string, argumentCount int) {
	t.Helper()
	calls := csharpQueryCalls(prog, methodName)
	require.Len(t, calls, 1, "%s must be emitted exactly once", methodName)
	call := calls[0]
	require.Len(t, call.Args, argumentCount)
	selectorValue, ok := call.GetValueById(call.Args[len(call.Args)-1])
	require.True(t, ok)
	selector, ok := ssa.ToFunction(selectorValue)
	require.True(t, ok, "%s final argument must be its result selector, got %T", methodName, selectorValue)
	require.Len(t, selector.Params, 2, "%s result selector must accept outer and introduced values", methodName)
}

func TestCSharp_QueryTransparentIdentifier_Let(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class QueryCases {
    public static object Run() {
        return from x in outerSource()
               let y = transformSource(x)
               select sinkPair(x, y);
    }
}`)

	requireQuerySinkMembers(t, prog, "sinkPair", "x", "y")
	requireNoQueryRangeCapture(t, prog, "x", "y")
	requireSingleCSharpCalls(t, prog, "outerSource", "transformSource", "sinkPair")

	transform := csharpCallsToMethod(t, prog, "transformSource")[0]
	require.Len(t, transform.Args, 1)
	argument, ok := transform.GetValueById(transform.Args[0])
	require.True(t, ok)
	require.True(t, argument.IsParameter(), argument.String())
}

func TestCSharp_QueryTransparentIdentifier_AdditionalFrom(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class QueryCases {
    public static object Run() {
        return from x in outerSource()
               from y in innerSource(x)
               select sinkPair(x, y);
    }
}`)

	requireQuerySinkMembers(t, prog, "sinkPair", "x", "y")
	requireNoQueryRangeCapture(t, prog, "x", "y")
	requireSingleCSharpCalls(t, prog, "outerSource", "innerSource", "sinkPair")

	requireQueryResultSelector(t, prog, "selectMany", 3)
}

func TestCSharp_QueryTransparentIdentifier_MultipleLet(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class QueryCases {
    public static object Run() {
        return from x in outerSource()
               let y = firstTransform(x)
               let z = secondTransform(x, y)
               select sinkTriple(x, y, z);
    }
}`)

	requireQuerySinkMembers(t, prog, "sinkTriple", "x", "y", "z")
	requireNoQueryRangeCapture(t, prog, "x", "y", "z")
	requireSingleCSharpCalls(t, prog, "outerSource", "firstTransform", "secondTransform", "sinkTriple")
}

func TestCSharp_QueryTransparentIdentifier_Join(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class QueryCases {
    public static object Run() {
        return from x in outerSource()
               join y in innerSource() on outerKey(x) equals innerKey(y)
               select sinkPair(x, y);
    }
}`)

	requireQuerySinkMembers(t, prog, "sinkPair", "x", "y")
	requireNoQueryRangeCapture(t, prog, "x", "y")
	requireSingleCSharpCalls(t, prog, "outerSource", "innerSource", "outerKey", "innerKey", "sinkPair")
	requireQueryResultSelector(t, prog, "join", 5)
}

func TestCSharp_QueryTransparentIdentifier_GroupJoin(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class QueryCases {
    public static object Run() {
        return from x in outerSource()
               join y in innerSource() on outerKey(x) equals innerKey(y) into matches
               select sinkPair(x, matches);
    }
}`)

	requireQuerySinkMembers(t, prog, "sinkPair", "x", "matches")
	requireNoQueryRangeCapture(t, prog, "x", "y", "matches")
	requireSingleCSharpCalls(t, prog, "outerSource", "innerSource", "outerKey", "innerKey", "sinkPair")
	requireQueryResultSelector(t, prog, "groupJoin", 5)
}

func TestCSharp_QueryTransparentIdentifier_GroupContinuation(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class QueryCases {
    public static object Run() {
        return from x in outerSource()
               let y = transformSource(x)
               group sinkPair(x, y) by keyPair(x, y) into grouped
               select sinkContinuation(grouped);
    }
}`)

	requireQuerySinkMembers(t, prog, "sinkPair", "x", "y")
	requireQuerySinkMembers(t, prog, "keyPair", "x", "y")
	requireNoQueryRangeCapture(t, prog, "x", "y", "grouped")
	requireSingleCSharpCalls(t, prog, "outerSource", "transformSource", "sinkPair", "keyPair", "sinkContinuation")

	continuation := csharpCallsToMethod(t, prog, "sinkContinuation")
	require.Len(t, continuation, 1)
	require.Len(t, continuation[0].Args, 1)
	grouped, ok := continuation[0].GetValueById(continuation[0].Args[0])
	require.True(t, ok)
	require.True(t, grouped.IsParameter(), grouped.String())
}
