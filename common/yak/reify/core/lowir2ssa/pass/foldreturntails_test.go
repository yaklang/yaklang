package pass

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

func returnWithRAX(address uint64) core.Instruction {
	return core.Instruction{
		Kind:     core.OpReturn,
		Operands: []core.Operand{register("rax", 64)},
		Address:  address,
	}
}

func TestFoldReturnTailsReplacesJumpToEpilogue(t *testing.T) {
	function := functionWithBlocks(
		core.Block{ID: 1, Address: 0x1000, Instructions: []core.Instruction{
			{Kind: core.OpConstant, Results: []core.Value{{ID: 1, Type: integer(64)}}, Operands: []core.Operand{literal(0x2a)}, Address: 0x1000},
			{Kind: core.OpWriteRegister, Operands: []core.Operand{register("rax", 64), value(1, integer(64))}, Address: 0x1000},
			jumpTo(2, 0x1000),
		}},
		core.Block{ID: 2, Address: 0x1004, Instructions: []core.Instruction{returnWithRAX(0x1004)}},
	)
	FoldReturnTails(&function)
	last := function.Blocks[0].Instructions[len(function.Blocks[0].Instructions)-1]
	if last.Kind != core.OpReturn {
		t.Fatalf("tail jump survived: %s", last.Kind)
	}
	if len(last.Operands) != 1 || last.Operands[0].Name != "rax" {
		t.Fatalf("return does not use rax: %+v", last.Operands)
	}
}

func TestFoldReturnTailsKeepsLoopBackEdge(t *testing.T) {
	function := functionWithBlocks(
		core.Block{ID: 1, Address: 0x1000, Instructions: []core.Instruction{jumpTo(2, 0x1000)}},
		core.Block{ID: 2, Address: 0x1002, Instructions: []core.Instruction{{Kind: core.OpBranch, Targets: []core.BlockID{3, 1}, Address: 0x1002}}},
		core.Block{ID: 3, Address: 0x1004, Instructions: []core.Instruction{returnWithRAX(0x1004)}},
	)
	FoldReturnTails(&function)
	last := function.Blocks[1].Instructions[len(function.Blocks[1].Instructions)-1]
	if last.Kind != core.OpBranch {
		t.Fatalf("loop branch was folded away: %s", last.Kind)
	}
}

func TestFoldReturnTailsSkipsTailWithReturningCall(t *testing.T) {
	function := functionWithBlocks(
		core.Block{ID: 1, Address: 0x1000, Instructions: []core.Instruction{jumpTo(2, 0x1000)}},
		core.Block{ID: 2, Address: 0x1004, Instructions: []core.Instruction{
			{Kind: core.OpCall, Operands: []core.Operand{{Kind: core.OperandSymbol, Name: "puts"}}, Address: 0x1004},
			returnWithRAX(0x1004),
		}},
	)
	FoldReturnTails(&function)
	last := function.Blocks[0].Instructions[len(function.Blocks[0].Instructions)-1]
	if last.Kind != core.OpJump {
		t.Fatalf("tail with a returning call was folded: %s", last.Kind)
	}
}

func TestFoldReturnTailsFoldsAfterNonReturningCall(t *testing.T) {
	function := functionWithBlocks(
		core.Block{ID: 1, Address: 0x1000, Instructions: []core.Instruction{jumpTo(2, 0x1000)}},
		core.Block{ID: 2, Address: 0x1004, Instructions: []core.Instruction{
			{Kind: core.OpCall, Operands: []core.Operand{{Kind: core.OperandSymbol, Name: "exit"}}, Address: 0x1004},
			returnWithRAX(0x1004),
		}},
	)
	FoldReturnTails(&function)
	last := function.Blocks[0].Instructions[len(function.Blocks[0].Instructions)-1]
	if last.Kind != core.OpReturn {
		t.Fatalf("tail after exit was not folded: %s", last.Kind)
	}
}
