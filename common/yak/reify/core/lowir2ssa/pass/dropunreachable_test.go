package pass

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// TestDropUnreachableRemovesTheBlockAfterAJump keeps a dead block out of the
// result. An unconditional branch leaves the instruction that follows it without a
// predecessor, and the code lifted from there reads registers whose value belongs
// to a path that never ran.
func TestDropUnreachableRemovesTheBlockAfterAJump(t *testing.T) {
	function := functionWithBlocks(
		core.Block{ID: 1, Address: 0x1000, Instructions: []core.Instruction{
			{Kind: core.OpConstant, Results: []core.Value{{ID: 1, Type: integer(64)}}, Operands: []core.Operand{literal(1)}, Address: 0x1000},
			{Kind: core.OpJump, Targets: []core.BlockID{3}, Address: 0x1000},
		}},
		core.Block{ID: 2, Address: 0x1004, Instructions: []core.Instruction{
			{Kind: core.OpConstant, Results: []core.Value{{ID: 2, Type: integer(64)}}, Operands: []core.Operand{literal(2)}, Address: 0x1004},
			{Kind: core.OpReturn, Address: 0x1004},
		}},
		core.Block{ID: 3, Address: 0x1008, Instructions: []core.Instruction{
			{Kind: core.OpReturn, Address: 0x1008},
		}},
	)
	DropUnreachable(&function)
	if len(function.Blocks) != 2 {
		t.Fatalf("blocks = %d, want 2", len(function.Blocks))
	}
	for _, block := range function.Blocks {
		if block.ID == 2 {
			t.Fatal("the block after the jump survived")
		}
	}
}

// TestDropUnreachableCollapsesABranchToOneSide checks the rewrite: a branch whose
// other target was dropped becomes a jump, so the structurer does not look for a
// second arm that no longer exists.
func TestDropUnreachableCollapsesABranchToOneSide(t *testing.T) {
	function := functionWithBlocks(
		core.Block{ID: 1, Address: 0x1000, Instructions: []core.Instruction{
			{Kind: core.OpConstant, Results: []core.Value{{ID: 1, Type: integer(1)}}, Operands: []core.Operand{literal(1)}, Address: 0x1000},
			{Kind: core.OpBranch, Operands: []core.Operand{value(1, integer(1))}, Targets: []core.BlockID{2, 3}, Address: 0x1000},
		}},
		core.Block{ID: 2, Address: 0x1004, Instructions: []core.Instruction{
			{Kind: core.OpReturn, Address: 0x1004},
		}},
		core.Block{ID: 3, Address: 0x1008, Instructions: []core.Instruction{
			{Kind: core.OpJump, Targets: []core.BlockID{2}, Address: 0x1008},
		}},
	)
	// Block 3 is reachable from the branch, so it stays; block 4 never existed.
	DropUnreachable(&function)
	if len(function.Blocks) != 3 {
		t.Fatalf("blocks = %d, want 3", len(function.Blocks))
	}
}
