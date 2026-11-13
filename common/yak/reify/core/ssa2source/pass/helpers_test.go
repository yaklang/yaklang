package pass_test

import (
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssairtext"
)

func parseSSA(t *testing.T, lines ...string) *ssa.Program {
	t.Helper()
	program, err := ssairtext.Parse(strings.Join(lines, "\n"))
	if err != nil {
		t.Fatal(err)
	}
	return program
}

func findCall(t *testing.T, program *ssa.Program, name string) *ssa.Call {
	t.Helper()
	var found *ssa.Call
	program.Funcs.ForEach(func(_ string, function *ssa.Function) bool {
		if function == nil || function.IsExtern() {
			return true
		}
		for _, blockID := range function.Blocks {
			block, ok := function.GetBasicBlockByID(blockID)
			if !ok || block == nil {
				continue
			}
			for _, instructionID := range block.Insts {
				instruction, ok := function.GetInstructionById(instructionID)
				if !ok || instruction == nil {
					continue
				}
				call, ok := ssa.ToCall(instruction)
				if !ok || call == nil {
					continue
				}
				target, ok := call.GetValueById(call.Method)
				if !ok || target == nil || target.GetName() != name {
					continue
				}
				found = call
				return false
			}
		}
		return true
	})
	if found == nil {
		t.Fatalf("call to %q is missing", name)
	}
	return found
}
