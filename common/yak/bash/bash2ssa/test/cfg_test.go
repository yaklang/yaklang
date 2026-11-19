package test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

// bashPhiInfo is a flattened view of one Phi instruction, used to assert where
// the merge lives (if.done, loop.condition, loop.exit, ...) and which values it
// merges.
type bashPhiInfo struct {
	Name  string
	Block string
	Edges []string
}

func (p bashPhiInfo) inBlock(prefix string) bool {
	return strings.HasPrefix(p.Block, prefix)
}

func checkBash(t *testing.T, code string, handler func(prog *ssaapi.Program)) {
	t.Helper()
	ssatest.CheckWithNameOnlyInMemory("", t, code, func(prog *ssaapi.Program) error {
		handler(prog)
		return nil
	}, ssaapi.WithLanguage(ssaconfig.BASH), ssaapi.WithMemory())
}

// bashPhis lists every Phi emitted by the file-level SSA function.
func bashPhis(t *testing.T, prog *ssaapi.Program) []bashPhiInfo {
	t.Helper()
	fn := prog.Program.GetFunction(string(ssa.MainFunctionName), "")
	require.NotNil(t, fn)
	var phis []bashPhiInfo
	for _, blockID := range fn.Blocks {
		block, ok := fn.GetBasicBlockByID(blockID)
		if !ok || block == nil {
			continue
		}
		for _, phiID := range block.Phis {
			value, ok := fn.GetValueById(phiID)
			if !ok || value == nil {
				continue
			}
			phi, ok := ssa.ToPhi(value)
			if !ok {
				continue
			}
			info := bashPhiInfo{Name: phi.GetName(), Block: block.GetName()}
			for _, edgeID := range phi.Edge {
				edge, ok := fn.GetValueById(edgeID)
				if !ok || edge == nil {
					info.Edges = append(info.Edges, "<nil>")
					continue
				}
				info.Edges = append(info.Edges, strings.TrimSpace(edge.String()))
			}
			phis = append(phis, info)
		}
	}
	return phis
}

func findPhi(phis []bashPhiInfo, blockPrefix, name string) (bashPhiInfo, bool) {
	for _, phi := range phis {
		if phi.Name == name && phi.inBlock(blockPrefix) {
			return phi, true
		}
	}
	return bashPhiInfo{}, false
}

func requirePhi(t *testing.T, phis []bashPhiInfo, blockPrefix, name string) bashPhiInfo {
	t.Helper()
	phi, ok := findPhi(phis, blockPrefix, name)
	require.Truef(t, ok, "missing phi %q in %s block, got %+v", name, blockPrefix, phis)
	return phi
}

// requirePhiArgument asserts the value passed to a command argument is a Phi.
// SyntaxFlow is the stable way to reach the value at a sink.
func requirePhiArgument(t *testing.T, prog *ssaapi.Program, callee string) *ssa.Phi {
	t.Helper()
	res, err := prog.SyntaxFlowWithError(callee + `(* as $argv)`)
	require.NoError(t, err)
	values := res.GetValues("argv")
	require.NotEmpty(t, values, "no %s arguments", callee)
	for _, value := range values {
		if phi, ok := ssa.ToPhi(value.GetSSAInst()); ok {
			return phi
		}
	}
	t.Fatalf("no Phi in %s arguments: %v", callee, values)
	return nil
}

func requirePhiInBlock(t *testing.T, phi *ssa.Phi, blockPrefix string) {
	t.Helper()
	block := phi.GetBlock()
	require.NotNil(t, block)
	require.Truef(t, strings.HasPrefix(block.GetName(), blockPrefix),
		"phi %q expected in %s, got %s", phi.GetName(), blockPrefix, block.GetName())
}

func TestBashCFG_IfElsePhi(t *testing.T) {
	checkBash(t, `if [ "$1" = a ]; then X=1; else X=2; fi; echo "$X"`, func(prog *ssaapi.Program) {
		phi := requirePhiArgument(t, prog, "echo")
		requirePhiInBlock(t, phi, ssa.IfDone)
		require.Len(t, phi.Edge, 2)

		merged := requirePhi(t, bashPhis(t, prog), ssa.IfDone, "X")
		require.ElementsMatch(t, []string{"1", "2"}, merged.Edges)
	})
}

func TestBashCFG_IfWithoutElsePhi(t *testing.T) {
	checkBash(t, `X=0; if [ "$1" = a ]; then X=1; fi; echo "$X"`, func(prog *ssaapi.Program) {
		phi := requirePhiArgument(t, prog, "echo")
		requirePhiInBlock(t, phi, ssa.IfDone)

		merged := requirePhi(t, bashPhis(t, prog), ssa.IfDone, "X")
		require.ElementsMatch(t, []string{"0", "1"}, merged.Edges)
	})
}

// Bash assigns without declaration, so a variable first written inside both
// branches must still merge: the pre-declared Undefined is what makes the scope
// merge emit the Phi instead of dropping the name.
func TestBashCFG_FirstAssignmentInBranchesPhi(t *testing.T) {
	checkBash(t, `if [ "$1" = a ]; then Y=1; else Y=2; fi; echo "$Y"`, func(prog *ssaapi.Program) {
		phi := requirePhiArgument(t, prog, "echo")
		requirePhiInBlock(t, phi, ssa.IfDone)

		merged := requirePhi(t, bashPhis(t, prog), ssa.IfDone, "Y")
		require.ElementsMatch(t, []string{"1", "2"}, merged.Edges)
	})
}

func TestBashCFG_ElifPhi(t *testing.T) {
	checkBash(t, `if [ "$1" = a ]; then X=1; elif [ "$1" = b ]; then X=2; else X=3; fi; echo "$X"`, func(prog *ssaapi.Program) {
		phi := requirePhiArgument(t, prog, "echo")
		requirePhiInBlock(t, phi, ssa.IfDone)

		merged := requirePhi(t, bashPhis(t, prog), ssa.IfDone, "X")
		require.Len(t, merged.Edges, 3)
		require.ElementsMatch(t, []string{"1", "2", "3"}, merged.Edges)
	})
}

func TestBashCFG_ForLoopPhi(t *testing.T) {
	checkBash(t, `for i in a b; do X=$i; done; echo "$X"`, func(prog *ssaapi.Program) {
		phis := bashPhis(t, prog)

		// The accumulator is loop-carried: its Phi sits on the loop header path.
		accumulator := requirePhi(t, phis, ssa.LoopCondition, "X")
		require.Len(t, accumulator.Edges, 2)

		// The item selection has actual predecessors; the loop head Phi
		// joins the entering variable with the selected item on the latch.
		choice := requirePhi(t, phis, ssa.IfDone, "i")
		require.ElementsMatch(t, []string{"a", "b"}, choice.Edges)
		require.Len(t, requirePhi(t, phis, ssa.LoopCondition, "i").Edges, 2)
	})
}

func TestBashCFG_CStyleForPhi(t *testing.T) {
	checkBash(t, `for ((i=0; i<3; i++)); do Y=$i; done; echo "$Y"`, func(prog *ssaapi.Program) {
		phis := bashPhis(t, prog)

		// i is carried through init, condition and the i++ latch step.
		index := requirePhi(t, phis, ssa.LoopCondition, "i")
		require.Len(t, index.Edges, 2)
		require.Contains(t, index.Edges, "0")
		require.True(t, strings.Contains(strings.Join(index.Edges, " "), "add"),
			"expected the latch edge to be the i++ update, got %v", index.Edges)

		// Y accumulates the loop body value.
		requirePhi(t, phis, ssa.LoopCondition, "Y")

		phi := requirePhiArgument(t, prog, "echo")
		requirePhiInBlock(t, phi, ssa.LoopCondition)
	})
}

func TestBashCFG_WhilePhi(t *testing.T) {
	checkBash(t, `X=0; while read -r line; do X=$line; done < /tmp/f; echo "$X"`, func(prog *ssaapi.Program) {
		phi := requirePhiArgument(t, prog, "echo")
		requirePhiInBlock(t, phi, ssa.LoopCondition)

		accumulator := requirePhi(t, bashPhis(t, prog), ssa.LoopCondition, "X")
		require.Len(t, accumulator.Edges, 2)
		require.Contains(t, accumulator.Edges, "0")
	})
}

func TestBashCFG_UntilPhi(t *testing.T) {
	checkBash(t, `N=0; until [ "$N" -gt 3 ]; do N=$((N+1)); done; echo "$N"`, func(prog *ssaapi.Program) {
		phi := requirePhiArgument(t, prog, "echo")
		requirePhiInBlock(t, phi, ssa.LoopCondition)

		accumulator := requirePhi(t, bashPhis(t, prog), ssa.LoopCondition, "N")
		require.Len(t, accumulator.Edges, 2)
		require.Contains(t, accumulator.Edges, "0")
	})
}

func TestBashCFG_BreakPhi(t *testing.T) {
	checkBash(t, `while true; do X=1; break; done; echo "$X"`, func(prog *ssaapi.Program) {
		phi := requirePhiArgument(t, prog, "echo")
		requirePhiInBlock(t, phi, ssa.LoopExit)

		merged := requirePhi(t, bashPhis(t, prog), ssa.LoopExit, "X")
		require.Len(t, merged.Edges, 2)
		require.Contains(t, merged.Edges, "1")
	})
}

func TestBashCFG_NestedIfInLoop(t *testing.T) {
	checkBash(t, `for i in a b; do if [ "$i" = a ]; then X=1; else X=2; fi; echo "$X"; done`, func(prog *ssaapi.Program) {
		phis := bashPhis(t, prog)
		requirePhi(t, phis, ssa.IfDone, "X")
		requirePhi(t, phis, ssa.LoopCondition, "X")
	})
}

func TestBashCFG_NestedLoopInIf(t *testing.T) {
	checkBash(t, `if true; then for i in a b; do X=$i; done; fi; echo "$X"`, func(prog *ssaapi.Program) {
		phi := requirePhiArgument(t, prog, "echo")
		requirePhiInBlock(t, phi, ssa.IfDone)

		phis := bashPhis(t, prog)
		requirePhi(t, phis, ssa.LoopCondition, "X")
		requirePhi(t, phis, ssa.IfDone, "X")
	})
}

func TestBashCFG_ShortCircuitPhi(t *testing.T) {
	t.Run("and", func(t *testing.T) {
		checkBash(t, `A=1; true && A=2; echo "$A"`, func(prog *ssaapi.Program) {
			phi := requirePhiArgument(t, prog, "echo")
			requirePhiInBlock(t, phi, ssa.IfDone)

			merged := requirePhi(t, bashPhis(t, prog), ssa.IfDone, "A")
			require.ElementsMatch(t, []string{"1", "2"}, merged.Edges)
		})
	})

	t.Run("or", func(t *testing.T) {
		checkBash(t, `A=1; false || A=3; echo "$A"`, func(prog *ssaapi.Program) {
			phi := requirePhiArgument(t, prog, "echo")
			requirePhiInBlock(t, phi, ssa.IfDone)

			merged := requirePhi(t, bashPhis(t, prog), ssa.IfDone, "A")
			require.ElementsMatch(t, []string{"1", "3"}, merged.Edges)
		})
	})
}

func TestBashCFG_CommandSubstitutionPhi(t *testing.T) {
	checkBash(t, `X=$(if true; then echo 1; else echo 2; fi); echo "$X"`, func(prog *ssaapi.Program) {
		phi := requirePhiArgument(t, prog, "echo")
		requirePhiInBlock(t, phi, ssa.IfDone)
		require.Len(t, phi.Edge, 2)
	})
}

// Specialization retains both concrete calls, but never puts a two-input
// Phi in the predecessor-free entry block.
func TestBashCFG_LiteralForUnrollPhi(t *testing.T) {
	checkBash(t, `for i in a b; do echo "$i"; done`, func(prog *ssaapi.Program) {
		merged := requirePhi(t, bashPhis(t, prog), ssa.IfDone, "i")
		require.ElementsMatch(t, []string{"a", "b"}, merged.Edges)

		res, err := prog.SyntaxFlowWithError(`echo(* as $argv) as $call`)
		require.NoError(t, err)
		require.Len(t, res.GetValues("call"), 2, "each literal iteration must keep its own call")
	})
}

func TestBashCFG_CasePhi(t *testing.T) {
	checkBash(t, `case "$1" in a) X=1;; b) X=2;; esac; echo "$X"`, func(prog *ssaapi.Program) {
		phi := requirePhiArgument(t, prog, "echo")
		requirePhiInBlock(t, phi, ssa.IfDone)
		require.True(t, len(phi.Edge) >= 2)
	})
}

func TestBashCFG_SelectLoop(t *testing.T) {
	checkBash(t, `select choice in a b; do X=$choice; done; echo "$X"`, func(prog *ssaapi.Program) {
		requirePhi(t, bashPhis(t, prog), ssa.LoopCondition, "X")
	})
}

func TestBashCFG_NestedBreakDepth(t *testing.T) {
	checkBash(t, `for i in a; do for j in b; do X=$i$j; break 2; done; done; echo "$X"`, func(prog *ssaapi.Program) {
		phi := requirePhiArgument(t, prog, "echo")
		require.NotNil(t, phi)
	})
}

func TestBashCFG_BackgroundJob(t *testing.T) {
	checkBash(t, `sleep 1 & wait; X=done; echo "$X"`, func(prog *ssaapi.Program) {
		res, err := prog.SyntaxFlowWithError(`echo(* as $argv)`)
		require.NoError(t, err)
		require.NotEmpty(t, res.GetValues("argv"))
	})
}

func TestBashPrintlnCFG(t *testing.T) {
	ssatest.CheckPrintlnValue(sink+`i=old; for i in; do i=bad; done; println "$i"; for i in a b; do :; done; println "$i"`, []string{`"old"`, `"b"`}, t)
	for _, code := range []string{`if probe; then x=a; elif probe2; then x=b; else x=c; fi`, `case "$1" in a|b) x=a;; c*) x=b;; *) x=c;; esac`} {
		ssatest.CheckPrintlnValueRegexp(sink+code+`; println "$x"`, []string{`^phi\([^)]*\)\["a","b","c"\]$`}, t)
	}
	ssatest.CheckPrintlnValueRegexp(sink+`x=a; probe && x=b; probe2 || x=c; println "$x"`, []string{`^phi\([^)]*\)\["c",phi\([^)]*\)\["b","a"\]\]$`}, t)
	checkBash(t, `for i in "$@"; do continue; never; done; until done_test; do break; done; select choice in a b; do break; done`, func(p *ssaapi.Program) {
		r, err := p.SyntaxFlowWithError(`never() as $dead`)
		require.NoError(t, err)
		require.Empty(t, r.GetValues("dead"))
		fn := p.Program.GetFunction(string(ssa.MainFunctionName), "")
		loops := 0
		for _, id := range fn.Blocks {
			b, _ := fn.GetBasicBlockByID(id)
			for _, id := range b.Insts {
				inst, _ := fn.GetInstructionById(id)
				if inst.GetOpcode() == ssa.SSAOpcodeLoop {
					loops++
				}
			}
		}
		require.Equal(t, 3, loops)
	})
}
