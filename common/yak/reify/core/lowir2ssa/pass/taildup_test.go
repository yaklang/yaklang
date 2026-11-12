package pass

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

func jumpTo(target core.BlockID, address uint64) core.Instruction {
	return core.Instruction{Kind: core.OpJump, Targets: []core.BlockID{target}, Address: address}
}

func returnBlock(id core.BlockID, address uint64) core.Block {
	return core.Block{
		ID:      id,
		Address: address,
		Instructions: []core.Instruction{
			{Kind: core.OpReadRegister, Results: []core.Value{{ID: core.ValueID(id), Type: integer(64)}}, Operands: []core.Operand{register("rax", 64)}, Address: address},
			{Kind: core.OpReturn, Operands: []core.Operand{value(core.ValueID(id), integer(64))}, Address: address},
		},
	}
}

func TestDuplicateReturnBlocksCopiesSharedEpilogue(t *testing.T) {
	function := functionWithBlocks(
		core.Block{ID: 1, Address: 0x1000, Instructions: []core.Instruction{{Kind: core.OpBranch, Targets: []core.BlockID{2, 3}, Address: 0x1000}}},
		core.Block{ID: 2, Address: 0x1001, Instructions: []core.Instruction{jumpTo(4, 0x1001)}},
		core.Block{ID: 3, Address: 0x1002, Instructions: []core.Instruction{jumpTo(4, 0x1002)}},
		returnBlock(4, 0x1003),
	)
	DuplicateReturnBlocks(&function)
	if len(function.Blocks) != 3 {
		t.Fatalf("shared epilogue survived: %d blocks", len(function.Blocks))
	}
	for _, block := range function.Blocks {
		if block.ID == 4 {
			t.Fatal("epilogue block was not removed")
		}
		if block.ID != 2 && block.ID != 3 {
			continue
		}
		last := block.Instructions[len(block.Instructions)-1]
		if last.Kind != core.OpReturn {
			t.Fatalf("block %d does not end in return: %s", block.ID, last.Kind)
		}
	}
}

func TestDuplicateReturnBlocksKeepsCallEpilogue(t *testing.T) {
	function := functionWithBlocks(
		core.Block{ID: 1, Address: 0x1000, Instructions: []core.Instruction{{Kind: core.OpBranch, Targets: []core.BlockID{2, 3}, Address: 0x1000}}},
		core.Block{ID: 2, Address: 0x1001, Instructions: []core.Instruction{jumpTo(4, 0x1001)}},
		core.Block{ID: 3, Address: 0x1002, Instructions: []core.Instruction{jumpTo(4, 0x1002)}},
		core.Block{ID: 4, Address: 0x1003, Instructions: []core.Instruction{
			{Kind: core.OpCall, Operands: []core.Operand{{Kind: core.OperandSymbol, Name: "puts"}}, Address: 0x1003},
			{Kind: core.OpReturn, Address: 0x1003},
		}},
	)
	DuplicateReturnBlocks(&function)
	if len(function.Blocks) != 4 {
		t.Fatalf("epilogue with a call was duplicated: %d blocks", len(function.Blocks))
	}
}

func TestDuplicateReturnBlocksIgnoresSinglePredecessor(t *testing.T) {
	function := functionWithBlocks(
		core.Block{ID: 1, Address: 0x1000, Instructions: []core.Instruction{jumpTo(2, 0x1000)}},
		returnBlock(2, 0x1001),
	)
	before := len(function.Blocks)
	DuplicateReturnBlocks(&function)
	if len(function.Blocks) != before {
		t.Fatal("single-predecessor epilogue was copied")
	}
}
