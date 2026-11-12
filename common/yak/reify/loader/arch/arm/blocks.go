package arm

import (
	"fmt"
	"sort"

	"golang.org/x/arch/arm/armasm"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// buildFunctions splits one decoded region into basic blocks. ARM has no
// fall-through ambiguity: every instruction is four bytes, so the leader set is
// enough.
func buildFunctions(decoded []instruction, start, end uint64, diagnostics *diagnostics) ([]function, map[uint64]core.BlockID) {
	starts := make(map[uint64]bool, len(decoded))
	starts[decoded[0].address] = true
	for _, ins := range decoded {
		if ins.undefined {
			// An embedded data word ends the reachable code before it.
			starts[ins.address] = true
			continue
		}
		next := ins.address + uint64(ins.size)
		if target, ok := branchTarget(ins); ok {
			switch {
			case target >= start && target < end:
				starts[target] = true
			case baseOp(ins.inst.Op) == armasm.BL || baseOp(ins.inst.Op) == armasm.BLX:
				// A call into another function is normal; only jumps and
				// conditional branches must stay inside the region.
			default:
				diagnostics.add("branch-target-outside-region", fmt.Sprintf("control transfer at 0x%x targets 0x%x outside the decoded region", ins.address, target))
			}
		}
		switch baseOp(ins.inst.Op) {
		case armasm.B, armasm.BX, armasm.BLX, armasm.BKPT, armasm.SVC:
			starts[next] = true
		}
		if isConditionalBranch(ins) {
			starts[next] = true
		}
	}
	addresses := make([]uint64, 0, len(starts))
	for address := range starts {
		addresses = append(addresses, address)
	}
	sort.Slice(addresses, func(i, j int) bool { return addresses[i] < addresses[j] })
	blocks := make([]block, 0, len(addresses))
	var current *block
	for _, ins := range decoded {
		if starts[ins.address] || current == nil {
			blocks = append(blocks, block{start: ins.address})
			current = &blocks[len(blocks)-1]
		}
		current.instructions = append(current.instructions, ins)
	}
	blockIDs := make(map[uint64]core.BlockID, len(blocks))
	for index := range blocks {
		blockIDs[blocks[index].start] = core.BlockID(index + 1)
	}
	return []function{{start: blocks[0].start, blocks: blocks}}, blockIDs
}

// branchTarget returns the static target of a branch. armasm reports PCRel
// relative to the instruction address plus eight for ARM, while the Thumb
// program counter is the instruction address plus four.
func branchTarget(ins instruction) (uint64, bool) {
	if len(ins.inst.Args) == 0 {
		return 0, false
	}
	offset, ok := pcRelative(ins.inst.Args[0])
	if !ok {
		return 0, false
	}
	if ins.thumb {
		return ins.address + 4 + uint64(offset), true
	}
	return ins.address + 8 + uint64(offset), true
}

// isConditionalBranch reports whether the opcode carries a condition other than
// AL. The B family keeps its condition in the low four bits of Op.
func isConditionalBranch(ins instruction) bool {
	if baseOp(ins.inst.Op) != armasm.B {
		return false
	}
	switch condition(ins.inst.Op) {
	case "al", "nv":
		return false
	default:
		return true
	}
}

func controlTransfer(kind core.Opcode) bool {
	switch kind {
	case core.OpJump, core.OpBranch, core.OpReturn, core.OpTrap:
		return true
	default:
		return false
	}
}

func containsBlock(blocks []core.BlockID, target core.BlockID) bool {
	for _, block := range blocks {
		if block == target {
			return true
		}
	}
	return false
}
