package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func explicitReturnCallOrders(t *testing.T, prog *ssaapi.Program, methodName string) [][]string {
	t.Helper()
	var orders [][]string
	prog.Program.EachFunction(func(function *ssa.Function) {
		if function.GetMethodName() != methodName {
			return
		}
		callNames := func(block *ssa.BasicBlock) []string {
			var names []string
			for _, instructionID := range block.Insts {
				instruction, ok := function.GetInstructionById(instructionID)
				if !ok || instruction == nil {
					continue
				}
				if call, ok := ssa.ToCall(instruction); ok {
					method, exists := call.GetValueById(call.Method)
					if exists && method != nil {
						names = append(names, method.GetVerboseName())
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
			current := callNames(block)
			if len(block.Preds) == 0 {
				return [][]string{current}
			}
			var paths [][]string
			for _, predecessorID := range block.Preds {
				predecessor, ok := function.GetBasicBlockByID(predecessorID)
				if !ok {
					continue
				}
				for _, prefix := range pathsTo(predecessor, seen) {
					path := append([]string(nil), prefix...)
					paths = append(paths, append(path, current...))
				}
			}
			return paths
		}
		for _, blockID := range function.Blocks {
			block, ok := function.GetBasicBlockByID(blockID)
			if !ok || block == nil || len(block.Insts) == 0 {
				continue
			}
			if _, ok := ssa.ToReturn(block.LastInst()); !ok {
				continue
			}
			for _, order := range pathsTo(block, make(map[int64]bool)) {
				orders = append(orders, append(order, "return"))
			}
		}
	})
	return orders
}

func requireReachableCallOrder(t *testing.T, prog *ssaapi.Program, methodName string, want ...string) {
	t.Helper()
	require.NotEmpty(t, want)
	terminal := want[len(want)-1]
	var paths [][]string
	prog.Program.EachFunction(func(function *ssa.Function) {
		if function.GetMethodName() != methodName || function.EnterBlock <= 0 {
			return
		}
		entry, ok := function.GetBasicBlockByID(function.EnterBlock)
		if !ok || entry == nil {
			return
		}
		var visit func(*ssa.BasicBlock, map[int64]bool, []string)
		visit = func(block *ssa.BasicBlock, seen map[int64]bool, calls []string) {
			if block == nil || seen[block.GetId()] {
				return
			}
			seen[block.GetId()] = true
			defer delete(seen, block.GetId())
			for _, instructionID := range block.Insts {
				instruction, exists := function.GetInstructionById(instructionID)
				if !exists || instruction == nil {
					continue
				}
				if call, isCall := ssa.ToCall(instruction); isCall {
					method, methodExists := call.GetValueById(call.Method)
					if methodExists && method != nil {
						calls = append(calls, method.GetVerboseName())
					}
				}
				if terminal == "return" {
					if _, isReturn := ssa.ToReturn(instruction); isReturn {
						paths = append(paths, append(append([]string(nil), calls...), "return"))
						return
					}
				} else if len(calls) > 0 && calls[len(calls)-1] == terminal {
					paths = append(paths, append([]string(nil), calls...))
					return
				}
			}
			for _, successorID := range block.Succs {
				successor, exists := function.GetBasicBlockByID(successorID)
				if exists {
					visit(successor, seen, append([]string(nil), calls...))
				}
			}
		}
		visit(entry, make(map[int64]bool), nil)
	})
	for _, path := range paths {
		index := 0
		for _, call := range path {
			if call == want[index] {
				index++
				if index == len(want) {
					return
				}
			}
		}
	}
	require.Failf(t, "reachable call order not found", "method=%s want=%v paths=%v", methodName, want, paths)
}

func parseCSharpSemantics(t *testing.T, code string) *ssaapi.Program {
	t.Helper()
	prog, err := ssaapi.Parse(code, ssaapi.WithLanguage(ssaconfig.CSHARP))
	require.NoError(t, err)
	requireCSharpCompileErrorFree(t, prog)
	return prog
}

func TestCSharp_ControlFlow_Phi(t *testing.T) {
	CheckCSharpPrintlnValue(`
int result = 1;
if (condition()) {
    result = 2;
} else {
    result = 3;
}
println(result);
`, []string{"phi(result)[2,3]"}, t)
}

func TestCSharp_ControlFlow_StatementFamiliesCompile(t *testing.T) {
	prog := parseCSharpSemantics(t, `
using System;
using System.Collections.Generic;

public sealed class Resource : IDisposable {
    public void Dispose() { }
}

public class Flow {
    public static IEnumerable<int> Generate(object gate) {
        int total = 0;
        using (var resource = new Resource()) {
            lock (gate) {
                for (int i = 0; i < 3; i++) {
                    if (i == 1) continue;
                    total += i;
                }
                foreach (var item in new int[] { 1, 2 }) {
                    total += item;
                }
                try {
                    if (total > 0) yield return total;
                    else throw new Exception("bad");
                } catch (Exception ex) when (ex != null) {
                    yield return -1;
                } finally {
                    cleanup(total);
                }
            }
        }
        yield break;
    }
}
`)
	require.NotEmpty(t, prog.Ref("cleanup"), "finally body must be emitted")
	require.NotEmpty(t, prog.Ref("Flow"), "class skeleton must be emitted")
}

func TestCSharp_ControlFlow_CatchWhenFalseDoesNotEmitBody(t *testing.T) {
	prog := parseCSharpSemantics(t, `
using System;

public class Flow {
    public static void Run() {
        try {
            risky();
        } catch (Exception ex) when (false) {
            rejected(source());
        }
    }
}
`)

	require.Empty(t, prog.Ref("rejected"), "a constant-false catch filter must make its body unreachable")
	require.Empty(t, prog.Ref("source"), "unreachable catch-filter body must not emit nested calls")
}

func TestCSharp_ControlFlow_DynamicCatchFilterBuildsConditionalRethrow(t *testing.T) {
	prog := parseCSharpSemantics(t, `
using System;

public class Flow {
    public static void Run() {
        try {
            risky();
        } catch (Exception ex) when (accept(ex)) {
            accepted(ex);
        }
    }
}
`)

	require.NotEmpty(t, prog.Ref("accept"))
	require.NotEmpty(t, prog.Ref("accepted"))
	var filterBranch *ssa.If
	var falseBranchHasPanic bool
	prog.Program.EachFunction(func(function *ssa.Function) {
		if function.GetMethodName() != "Run" {
			return
		}
		for _, blockID := range function.Blocks {
			block, ok := function.GetBasicBlockByID(blockID)
			if !ok || block == nil {
				continue
			}
			for _, instructionID := range block.Insts {
				instruction, exists := function.GetInstructionById(instructionID)
				if !exists || instruction == nil {
					continue
				}
				branch, isIf := ssa.ToIfInstruction(instruction)
				if !isIf {
					continue
				}
				filterBranch = branch
				falseBlock, exists := function.GetBasicBlockByID(branch.False)
				if !exists || falseBlock == nil {
					continue
				}
				for _, falseInstructionID := range falseBlock.Insts {
					falseInstruction, exists := function.GetInstructionById(falseInstructionID)
					if !exists {
						continue
					}
					if _, isPanic := falseInstruction.(*ssa.Panic); isPanic {
						falseBranchHasPanic = true
					}
				}
			}
		}
	})
	require.NotNil(t, filterBranch, "a dynamic catch filter must lower to an If instruction")
	require.True(t, falseBranchHasPanic, "the filter-false branch must rethrow the captured exception")
}

func TestCSharp_ControlFlow_ReturnRunsFinallyBeforeReturn(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class Flow {
    public static object Run() {
        try {
            return source();
        } finally {
            cleanup();
        }
    }
}
`)
	require.Contains(t, explicitReturnCallOrders(t, prog, "Run"), []string{"source", "cleanup", "return"})
}

func TestCSharp_ControlFlow_ReturnRunsNestedFinallyInsideOut(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class Flow {
    public static object Run() {
        try {
            try {
                return source();
            } finally {
                innerCleanup();
            }
        } finally {
            outerCleanup();
        }
    }
}
`)
	require.Contains(t, explicitReturnCallOrders(t, prog, "Run"), []string{"source", "innerCleanup", "outerCleanup", "return"})
}

func TestCSharp_ControlFlow_BreakRunsExitedFinally(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class Flow {
    public static void Run() {
        while (gate()) {
            try {
                beforeBreak();
                break;
            } finally {
                breakCleanup();
            }
        }
        afterBreak();
    }
}
`)
	requireReachableCallOrder(t, prog, "Run", "beforeBreak", "breakCleanup", "afterBreak")
}

func TestCSharp_ControlFlow_ContinueRunsExitedFinally(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class Flow {
    public static void Run() {
        for (int i = 0; gate(); step()) {
            try {
                beforeContinue();
                continue;
            } finally {
                continueCleanup();
            }
        }
    }
}
`)
	requireReachableCallOrder(t, prog, "Run", "beforeContinue", "continueCleanup", "step")
}

func TestCSharp_ControlFlow_YieldBreakRunsFinally(t *testing.T) {
	prog := parseCSharpSemantics(t, `
using System.Collections.Generic;

public class Flow {
    public static IEnumerable<int> Run() {
        try {
            beforeYieldBreak();
            yield break;
        } finally {
            yieldBreakCleanup();
        }
    }
}
`)
	requireReachableCallOrder(t, prog, "Run", "beforeYieldBreak", "yieldBreakCleanup", "return")
}

func TestCSharp_ControlFlow_BreakInsideTryDoesNotExitFinally(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class Flow {
    public static void Run() {
        try {
            while (gate()) {
                beforeInnerBreak();
                break;
            }
            afterInnerBreak();
        } finally {
            outerCleanup();
        }
    }
}
`)
	requireReachableCallOrder(t, prog, "Run", "beforeInnerBreak", "afterInnerBreak", "outerCleanup")
}
