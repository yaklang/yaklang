package pass

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

func blockOrder(function core.Function) []core.BlockID {
	order := make([]core.BlockID, 0, len(function.Blocks))
	for _, block := range function.Blocks {
		order = append(order, block.ID)
	}
	return order
}

func TestReorderReversePostorderPutsLoopHeaderBeforeBody(t *testing.T) {
	function := functionWithBlocks(
		core.Block{ID: 1, Address: 0x1000, Instructions: []core.Instruction{jumpTo(2, 0x1000)}},
		core.Block{ID: 3, Address: 0x1020, Instructions: []core.Instruction{jumpTo(2, 0x1020)}},
		core.Block{ID: 2, Address: 0x1010, Instructions: []core.Instruction{{Kind: core.OpBranch, Targets: []core.BlockID{3, 4}, Address: 0x1010}}},
		core.Block{ID: 4, Address: 0x1030, Instructions: []core.Instruction{{Kind: core.OpReturn, Address: 0x1030}}},
	)
	ReorderReversePostorder(&function)
	want := []core.BlockID{1, 2, 3, 4}
	got := blockOrder(function)
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("block order is %v, want %v", got, want)
		}
	}
}

func TestReorderReversePostorderKeepsUnreachableBlocks(t *testing.T) {
	function := functionWithBlocks(
		core.Block{ID: 1, Address: 0x1000, Instructions: []core.Instruction{{Kind: core.OpReturn, Address: 0x1000}}},
		core.Block{ID: 9, Address: 0x2000, Instructions: []core.Instruction{{Kind: core.OpReturn, Address: 0x2000}}},
	)
	ReorderReversePostorder(&function)
	want := []core.BlockID{1, 9}
	got := blockOrder(function)
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("block order is %v, want %v", got, want)
		}
	}
}

func TestReorderReversePostorderHandlesEmptyFunction(t *testing.T) {
	function := core.Function{}
	ReorderReversePostorder(&function)
	if len(function.Blocks) != 0 {
		t.Fatal("empty function gained blocks")
	}
}
