package arm64

import (
	"fmt"
	"sort"

	"golang.org/x/arch/arm64/arm64asm"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

type block struct {
	start        uint64
	instructions []instruction
}

// function is one decoded unit: a start address and its basic blocks in
// address order. A ParseRequest region becomes one function.
type function struct {
	start  uint64
	blocks []block
}

func buildFunctions(decoded []instruction, start, end uint64, diagnostics *diagnostics) ([]function, map[uint64]core.BlockID) {
	starts := make(map[uint64]bool, len(decoded))
	starts[decoded[0].address] = true
	for _, ins := range decoded {
		next := ins.address + uint64(ins.size)
		if target, ok := directTarget(ins); ok {
			if target >= start && target < end {
				starts[target] = true
			} else if isBranch(ins.inst.Op) {
				diagnostics.add("branch-target-outside-region", fmt.Sprintf("control transfer at 0x%x targets 0x%x outside the decoded region", ins.address, target))
			}
		}
		switch ins.inst.Op {
		case arm64asm.B, arm64asm.BR, arm64asm.RET, arm64asm.BRK, arm64asm.HLT:
			// Terminators end a block; a call returns to the next instruction
			// and keeps the linear block together, like the AMD64 parser.
			starts[next] = true
		}
		if conditional(ins) {
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

// directTarget returns the static target of a direct branch or call. PCRel is
// already a signed offset from the instruction address. A conditional branch
// carries the condition first and the label second, so every operand is tried.
func directTarget(ins instruction) (uint64, bool) {
	for _, arg := range ins.inst.Args {
		if arg == nil {
			break
		}
		if offset, ok := pcRelative(arg); ok {
			return ins.address + uint64(offset), true
		}
	}
	return 0, false
}

func isBranch(op arm64asm.Op) bool {
	switch op {
	case arm64asm.B, arm64asm.BL, arm64asm.CBZ, arm64asm.CBNZ, arm64asm.TBZ, arm64asm.TBNZ:
		return true
	default:
		return false
	}
}

// conditional reports whether the branch has two successors: the direct target
// and the next instruction.
func conditional(ins instruction) bool {
	switch ins.inst.Op {
	case arm64asm.CBZ, arm64asm.CBNZ, arm64asm.TBZ, arm64asm.TBNZ:
		return true
	case arm64asm.B:
		_, ok := conditionCode(ins.inst.Args[0])
		return ok
	default:
		return false
	}
}
