package tests

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func patternCallMatches(actual, expected string) bool {
	return actual == expected || strings.HasSuffix(actual, "."+expected)
}

func patternCallIndex(path []string, name string) int {
	for index, call := range path {
		if patternCallMatches(call, name) {
			return index
		}
	}
	return -1
}

func requirePatternGetterGate(t *testing.T, prog *ssaapi.Program, methodName, first, second string) {
	t.Helper()
	paths := explicitReturnCallOrders(t, prog, methodName)
	require.NotEmpty(t, paths)
	firstBlocks := make(map[int64]struct{})
	secondBlocks := make(map[int64]struct{})
	firstCalls, secondCalls := 0, 0
	prog.Program.EachFunction(func(function *ssa.Function) {
		if function.GetMethodName() != methodName {
			return
		}
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
				name := callee.GetVerboseName()
				switch {
				case patternCallMatches(name, first):
					firstCalls++
					firstBlocks[block.GetId()] = struct{}{}
				case patternCallMatches(name, second):
					secondCalls++
					secondBlocks[block.GetId()] = struct{}{}
				}
			}
		}
	})
	require.Equal(t, 1, firstCalls, "%s must be evaluated once in the emitted pattern", first)
	require.Equal(t, 1, secondCalls, "%s must be evaluated once in the emitted pattern", second)
	for blockID := range secondBlocks {
		_, sameBlock := firstBlocks[blockID]
		require.False(t, sameBlock, "%s must be emitted in a gated block after %s", second, first)
	}

	sawFirstFailure := false
	sawSuccessfulPrefix := false
	for _, path := range paths {
		firstIndex := patternCallIndex(path, first)
		secondIndex := patternCallIndex(path, second)
		if firstIndex >= 0 && secondIndex < 0 {
			sawFirstFailure = true
		}
		if firstIndex >= 0 && secondIndex > firstIndex {
			sawSuccessfulPrefix = true
		}
	}
	require.True(t, sawFirstFailure, "a false %s result must reach a return without evaluating %s; paths=%v", first, second, paths)
	require.True(t, sawSuccessfulPrefix, "the successful prefix must evaluate %s before %s; paths=%v", first, second, paths)
}

func patternMemberKeyPaths(prog *ssaapi.Program, methodName string) [][]string {
	var paths [][]string
	prog.Program.EachFunction(func(function *ssa.Function) {
		if function.GetMethodName() != methodName {
			return
		}
		markers := func(block *ssa.BasicBlock) []string {
			var names []string
			for _, instructionID := range block.Insts {
				instruction, exists := function.GetInstructionById(instructionID)
				if !exists {
					continue
				}
				constant, ok := ssa.ToConstInst(instruction)
				if ok && constant.ConstType == ssa.ConstTypePlaceholder {
					switch constant.String() {
					case "A", "B":
						names = append(names, constant.String())
					}
				}
			}
			return names
		}
		var pathsTo func(*ssa.BasicBlock, map[int64]bool) [][]string
		pathsTo = func(block *ssa.BasicBlock, seen map[int64]bool) [][]string {
			if block == nil || seen[block.GetId()] {
				return nil
			}
			seen[block.GetId()] = true
			defer delete(seen, block.GetId())
			current := markers(block)
			if len(block.Preds) == 0 {
				return [][]string{current}
			}
			var found [][]string
			for _, predecessorID := range block.Preds {
				predecessor, exists := function.GetBasicBlockByID(predecessorID)
				if !exists {
					continue
				}
				for _, prefix := range pathsTo(predecessor, seen) {
					found = append(found, append(append([]string(nil), prefix...), current...))
				}
			}
			return found
		}
		for _, blockID := range function.Blocks {
			block, exists := function.GetBasicBlockByID(blockID)
			if !exists || block == nil || len(block.Insts) == 0 {
				continue
			}
			if _, isReturn := ssa.ToReturn(block.LastInst()); !isReturn {
				continue
			}
			paths = append(paths, pathsTo(block, make(map[int64]bool))...)
		}
	})
	return paths
}

func requirePositionalMemberGate(t *testing.T, prog *ssaapi.Program, methodName string) {
	t.Helper()
	paths := patternMemberKeyPaths(prog, methodName)
	require.NotEmpty(t, paths)
	sawFirstFailure := false
	sawSuccessfulPrefix := false
	firstCount, secondCount := 0, 0
	for _, path := range paths {
		first := patternCallIndex(path, "A")
		second := patternCallIndex(path, "B")
		if first >= 0 {
			firstCount++
		}
		if second >= 0 {
			secondCount++
		}
		if first >= 0 && second < 0 {
			sawFirstFailure = true
		}
		if first >= 0 && second > first {
			sawSuccessfulPrefix = true
		}
	}
	require.Positive(t, firstCount)
	require.Positive(t, secondCount)
	require.True(t, sawFirstFailure, "the second positional member must be unreachable after the first fails: %v", paths)
	require.True(t, sawSuccessfulPrefix, "positional members must be evaluated in source order: %v", paths)
}

const patternPayload = `
public class PatternPayload {
    public int A { get { return firstGetter(); } }
    public int B { get { return secondGetter(); } }
}
`

func TestCSharp_PatternShortCircuit_PropertyIs(t *testing.T) {
	prog := parseCSharpSemantics(t, patternPayload+`
public class PatternCases {
    public static bool Run(PatternPayload value) {
        if (value is { A: 0, B: 1 }) {
            matched();
            return true;
        }
        return false;
    }
}`)

	requirePatternGetterGate(t, prog, "Run", "value.get_A", "value.get_B")
}

func TestCSharp_PatternShortCircuit_PropertySwitch(t *testing.T) {
	prog := parseCSharpSemantics(t, patternPayload+`
public class PatternCases {
    public static bool Run(PatternPayload value) {
        switch (value) {
        case { A: 0, B: 1 }:
            matched();
            return true;
        default:
            return false;
        }
    }
}`)

	requirePatternGetterGate(t, prog, "Run", "value.get_A", "value.get_B")
}

func TestCSharp_PatternShortCircuit_PositionalIs(t *testing.T) {
	prog := parseCSharpSemantics(t, patternPayload+`
public class PatternCases {
    public static bool Run(PatternPayload value) {
        if (value is PatternPayload(A: 0, B: 1)) {
            matched();
            return true;
        }
        return false;
    }
}`)

	requirePositionalMemberGate(t, prog, "Run")
}

func TestCSharp_PatternDesignationAndGuardAreSuccessGated(t *testing.T) {
	prog := parseCSharpSemantics(t, patternPayload+`
public class PatternCases {
    public static bool Run(PatternPayload value) {
        switch (value) {
        case { A: 0, B: var captured } when guard(captured):
            sink(captured);
            return true;
        default:
            return false;
        }
    }
}`)

	requirePatternGetterGate(t, prog, "Run", "value.get_A", "value.get_B")

	paths := explicitReturnCallOrders(t, prog, "Run")
	require.NotEmpty(t, paths)
	sawFirstFailure := false
	sawGuardedSuccess := false
	for _, path := range paths {
		first := patternCallIndex(path, "value.get_A")
		guard := patternCallIndex(path, "guard")
		sink := patternCallIndex(path, "sink")
		if first >= 0 && guard < 0 {
			sawFirstFailure = true
		}
		if first >= 0 && guard > first && sink > guard {
			sawGuardedSuccess = true
		}
	}
	require.True(t, sawFirstFailure, "guard must be unreachable when the property pattern fails: %v", paths)
	require.True(t, sawGuardedSuccess, "guard and body must follow the successful pattern edge: %v", paths)

	flow, err := prog.SyntaxFlowWithError(`sink(* as $captured)`)
	require.NoError(t, err)
	captured := flow.GetValues("captured")
	require.NotEmpty(t, captured)
	require.NotContains(t, captured.String(), "Undefined-captured")
}

func TestCSharp_PatternDesignationFeedsLogicalAndOnSuccess(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class PatternCases {
    public static bool Run(object subject) {
        if (subject is string captured && guard(captured)) {
            sink(captured);
            return true;
        }
        return false;
    }
}`)

	paths := explicitReturnCallOrders(t, prog, "Run")
	require.NotEmpty(t, paths)
	sawTypeFailure := false
	sawGuardedSuccess := false
	for _, path := range paths {
		typeCheck := patternCallIndex(path, "is")
		guard := patternCallIndex(path, "guard")
		sink := patternCallIndex(path, "sink")
		if typeCheck >= 0 && guard < 0 {
			sawTypeFailure = true
		}
		if typeCheck >= 0 && guard > typeCheck && sink > guard {
			sawGuardedSuccess = true
		}
	}
	require.True(t, sawTypeFailure, "guard must be unreachable when the declaration pattern fails: %v", paths)
	require.True(t, sawGuardedSuccess, "guard and body must receive the successful declaration binding: %v", paths)

	flow, err := prog.SyntaxFlowWithError(`guard(* as $guard); sink(* as $sink)`)
	require.NoError(t, err)
	guard := flow.GetValues("guard")
	sink := flow.GetValues("sink")
	require.NotEmpty(t, guard)
	require.NotEmpty(t, sink)
	require.NotContains(t, guard.String(), "Undefined-captured")
	require.NotContains(t, sink.String(), "Undefined-captured")
	require.Contains(t, guard.GetTopDefs().String(), "subject")
	require.Contains(t, sink.GetTopDefs().String(), "subject")
}
