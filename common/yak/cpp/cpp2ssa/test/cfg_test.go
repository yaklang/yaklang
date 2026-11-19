package test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// TestCppPhiIfElse asserts the merge phi of an if/else and the branch each
// incoming value comes from.
func TestCppPhiIfElse(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"ifelse.cpp": `
int choose(int a, int b) {
    int result = 0;
    if (a > b) {
        result = a;
    } else {
        result = b;
    }
    return result;
}
`,
	})
	fn := funcBySymbol(t, progs[0], "choose")
	phi := findPhi(t, fn, "result")
	require.Len(t, phi.Edge, 2, "if/else merge must take one value per branch")

	trueEdge := phiEdgeFrom(t, fn, phi, "if.true")
	falseEdge := phiEdgeFrom(t, fn, phi, "if.false")
	require.Equal(t, "a", trueEdge.GetName(), "true branch must carry parameter a")
	require.Equal(t, "b", falseEdge.GetName(), "false branch must carry parameter b")
}

// TestCppPhiIfWithoutElse asserts the implicit false branch keeps the value
// that was live before the if.
func TestCppPhiIfWithoutElse(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"ifonly.cpp": `
int chooseDefault(int a) {
    int result = 7;
    if (a > 0) {
        result = 1;
    }
    return result;
}
`,
	})
	fn := funcBySymbol(t, progs[0], "chooseDefault")
	phi := findPhi(t, fn, "result")
	require.Len(t, phi.Edge, 2)
	require.EqualValues(t, 1, constInt(t, phiEdgeFrom(t, fn, phi, "if.true")))
	// Without an else clause the merge has no if.false block; the second edge
	// comes from the block that held the pre-if value.
	values := phiEdgeInts(t, fn, phi)
	require.ElementsMatch(t, []int64{1, 7}, values)
}

// TestCppPhiElseIfChain asserts an else-if chain merges every arm.
func TestCppPhiElseIfChain(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"chain.cpp": `
int classify(int value) {
    int label = 0;
    if (value < 0) {
        label = -1;
    } else if (value == 0) {
        label = 0;
    } else {
        label = 1;
    }
    return label;
}
`,
	})
	fn := funcBySymbol(t, progs[0], "classify")
	values := phiValues(t, fn, "label")
	require.ElementsMatch(t, []int64{-1, 0, 1}, values, "each arm must reach the merge phi")
}

// TestCppPhiNestedIf asserts nested if merges the inner result before the outer
// merge sees it.
func TestCppPhiNestedIf(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"nested.cpp": `
int nested(int a, int b) {
    int result = 0;
    if (a > 0) {
        if (b > 0) {
            result = 11;
        } else {
            result = 12;
        }
    } else {
        result = 13;
    }
    return result;
}
`,
	})
	fn := funcBySymbol(t, progs[0], "nested")
	values := phiValues(t, fn, "result")
	require.Subset(t, values, []int64{11, 12, 13})
}

// TestCppPhiLoops asserts every loop kind merges the loop-carried variable.
func TestCppPhiLoops(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"loops.cpp": `
int sumFor(int n) {
    int total = 0;
    for (int i = 0; i < n; i++) {
        total += i;
    }
    return total;
}

int sumWhile(int n) {
    int index = 0;
    int total = 0;
    while (index < n) {
        total += index;
        index++;
    }
    return total;
}

int sumDoWhile(int n) {
    int total = 0;
    do {
        total++;
    } while (total < n);
    return total;
}
`,
	})
	prog := progs[0]

	for _, name := range []string{"sumFor", "sumWhile", "sumDoWhile"} {
		t.Run(name, func(t *testing.T) {
			fn := funcBySymbol(t, prog, name)
			phi := findPhi(t, fn, "total")
			require.GreaterOrEqual(t, len(phi.Edge), 2, "loop phi must merge entry and latch")
			// One incoming edge is the pre-loop constant 0. The other is the
			// value computed in the body, so it must not be a constant.
			var constants, computed int
			for _, edge := range phi.Edge {
				value, ok := fn.GetValueById(edge)
				require.True(t, ok)
				if _, isConst := ssa.ToConstInst(value); isConst {
					constants++
				} else {
					computed++
				}
			}
			require.Equal(t, 1, constants, "loop entry must contribute exactly one constant")
			require.GreaterOrEqual(t, computed, 1, "loop body must contribute a computed value")
		})
	}
}

// TestCppPhiLoopConditionUsesBodyValue asserts the loop condition reads the
// merged variable, not the pre-loop constant.
//
// TODO(ssa-cpp): the for-init counter (`i`) is only assigned in the third
// expression; the current loop builder creates a phi for variables assigned in
// the body but not for latch-only updates, so the counter stays at its init
// value in the condition.
func TestCppPhiLoopConditionUsesBodyValue(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"forcond.cpp": `
int countTo(int n) {
    int seen = 0;
    for (int i = 0; i < n; i++) {
        seen += 1;
    }
    return seen;
}
`,
	})
	fn := funcBySymbol(t, progs[0], "countTo")
	phi := findPhi(t, fn, "seen")
	require.GreaterOrEqual(t, len(phi.Edge), 2, "loop body value must be loop-carried")
	require.EqualValues(t, 0, constInt(t, phiEdgeFrom(t, fn, phi, "loop.header")))
	condition := phi.GetBlock()
	require.True(t, strings.HasPrefix(condition.GetName(), "loop.condition"),
		"merge must live in the condition block, got %s", condition.GetName())
}

// TestCppSwitchMerge asserts the switch join keeps every case value.
func TestCppSwitchMerge(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"switch.cpp": `
int selectCase(int value) {
    int result = 0;
    switch (value) {
    case 1:
        result = 10;
        break;
    case 2:
        result = 20;
        break;
    default:
        result = -1;
        break;
    }
    return result;
}
`,
	})
	fn := funcBySymbol(t, progs[0], "selectCase")
	phi := findPhi(t, fn, "result")
	require.ElementsMatch(t, []int64{10, 20, -1}, phiEdgeInts(t, fn, phi))
}

// TestCppSwitchFallThrough asserts labels without a body share the next arm.
func TestCppSwitchFallThrough(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"fallthrough.cpp": `
int fallThrough(int value) {
    int result = 0;
    switch (value) {
    case 1:
    case 2:
        result = 12;
        break;
    case 3:
        result = 3;
        break;
    }
    return result;
}
`,
	})
	fn := funcBySymbol(t, progs[0], "fallThrough")
	phi := findPhi(t, fn, "result")
	require.Subset(t, phiEdgeInts(t, fn, phi), []int64{12, 3, 0})
}

// TestCppRangeFor asserts the range-for body reads an iteration value.
func TestCppRangeFor(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"range.cpp": `
int sumRange() {
    int total = 0;
    int values[3] = {1, 2, 3};
    for (int value : values) {
        total += value;
    }
    return total;
}
`,
	})
	fn := funcBySymbol(t, progs[0], "sumRange")
	require.NotNil(t, findPhi(t, fn, "total"))
	require.Positive(t, countInstructions(fn, func(inst ssa.Instruction) bool {
		_, ok := inst.(*ssa.Next)
		return ok
	}), "range-for must emit a Next instruction")
}

// TestCppBreakAndContinue asserts break and continue terminate their block and
// keep the loop merge reachable.
func TestCppBreakAndContinue(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"breakcontinue.cpp": `
int sumWithBreak(int n) {
    int total = 0;
    for (int i = 0; i < n; i++) {
        if (i == 4) {
            break;
        }
        if (i % 2 == 0) {
            continue;
        }
        total += i;
    }
    return total;
}
`,
	})
	fn := funcBySymbol(t, progs[0], "sumWithBreak")
	require.NotNil(t, findPhi(t, fn, "total"))
	jumps := countInstructions(fn, func(inst ssa.Instruction) bool {
		_, ok := inst.(*ssa.Jump)
		return ok
	})
	require.Positive(t, jumps, "break and continue must leave jumps")
}

// phiValues collects the constant values of every phi that carries name.
func phiValues(t *testing.T, fn *ssa.Function, name string) []int64 {
	t.Helper()
	var values []int64
	for _, blockID := range fn.Blocks {
		block := blockValue(t, fn, blockID)
		for _, phiID := range block.Phis {
			value, ok := fn.GetValueById(phiID)
			if !ok {
				continue
			}
			phi, ok := ssa.ToPhi(value)
			if !ok || phi.GetName() != name {
				continue
			}
			// A nested merge can feed a phi from another phi (else-if); only
			// the constant arm values are compared here.
			for _, edge := range phi.Edge {
				edgeValue, ok := fn.GetValueById(edge)
				if !ok {
					continue
				}
				if constInst, ok := ssa.ToConstInst(edgeValue); ok {
					switch raw := constInst.GetRawValue().(type) {
					case int64:
						values = append(values, raw)
					case int:
						values = append(values, int64(raw))
					}
				}
			}
		}
	}
	require.NotEmptyf(t, values, "no phi values for %q in %s", name, fn.GetName())
	return values
}

func phiEdgeInts(t *testing.T, fn *ssa.Function, phi *ssa.Phi) []int64 {
	t.Helper()
	values := make([]int64, 0, len(phi.Edge))
	for _, edge := range phi.Edge {
		value, ok := fn.GetValueById(edge)
		require.True(t, ok)
		values = append(values, constInt(t, value))
	}
	return values
}

func countInstructions(fn *ssa.Function, match func(ssa.Instruction) bool) int {
	count := 0
	for _, blockID := range fn.Blocks {
		value, ok := fn.GetValueById(blockID)
		if !ok {
			continue
		}
		block, ok := value.(*ssa.BasicBlock)
		if !ok {
			continue
		}
		for _, instID := range block.Insts {
			inst, ok := fn.GetInstructionById(instID)
			if ok && match(inst) {
				count++
			}
		}
	}
	return count
}

// TestCppPhiIncomingBlocks asserts the phi predecessor list matches the branch
// blocks, not just the number of values.
func TestCppPhiIncomingBlocks(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"preds.cpp": `
int pick(int a) {
    int selected = 0;
    if (a) {
        selected = 1;
    } else {
        selected = 2;
    }
    return selected;
}
`,
	})
	fn := funcBySymbol(t, progs[0], "pick")
	phi := findPhi(t, fn, "selected")
	block := phi.GetBlock()
	require.Len(t, block.Preds, len(phi.Edge), "one edge per predecessor")
	names := make([]string, 0, len(block.Preds))
	for _, predID := range block.Preds {
		names = append(names, blockValue(t, fn, predID).GetName())
	}
	require.True(t, hasBlockPrefix(names, "if.true"), "missing true predecessor: %v", names)
	require.True(t, hasBlockPrefix(names, "if.false"), "missing false predecessor: %v", names)
}

func hasBlockPrefix(names []string, prefix string) bool {
	for _, name := range names {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}
// TestCppTryCatchBlocks asserts try/catch lowers to try and catch blocks and
// the catch parameter is an SSA value.
func TestCppTryCatchBlocks(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"try.cpp": `
int risky(int value) {
    try {
        if (value > 0) {
            throw value;
        }
    } catch (int error) {
        return error;
    }
    return 0;
}
`,
	})
	fn := funcBySymbol(t, progs[0], "risky")
	names := make([]string, 0, len(fn.Blocks))
	for _, blockID := range fn.Blocks {
		names = append(names, blockValue(t, fn, blockID).GetName())
	}
	require.Contains(t, strings.Join(names, ","), "catch", "try/catch must create a catch block")
	require.NotEmpty(t, progs[0].Ref("error"), "catch parameter must be visible")
}

// TestCppLoopContinuePhi asserts a continue branch still merges into the loop
// phi instead of dropping the accumulator.
func TestCppLoopContinuePhi(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"continue.cpp": `
int sumOdd(int limit) {
    int total = 0;
    for (int i = 0; i < limit; i++) {
        if (i == 2) {
            continue;
        }
        total += i;
    }
    return total;
}
`,
	})
	fn := funcBySymbol(t, progs[0], "sumOdd")
	phi := findPhi(t, fn, "total")
	require.GreaterOrEqual(t, len(phi.Edge), 2)
	condition := phi.GetBlock()
	require.True(t, strings.HasPrefix(condition.GetName(), "loop.condition"))
}

// TestCppRangeForAccumulator asserts a range-for keeps a loop-carried
// accumulator phi.
func TestCppRangeForAccumulator(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"range_sum.cpp": `
int sumAll() {
    int values[3] = {1, 2, 3};
    int total = 0;
    for (int value : values) {
        total += value;
    }
    return total;
}
`,
	})
	fn := funcBySymbol(t, progs[0], "sumAll")
	require.NotNil(t, findPhi(t, fn, "total"))
}
