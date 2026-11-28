package pass

import (
	"strings"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// DropDeadFlags removes flags-register writes (and dead flags.* intrinsics that
// only feed them) whose flags value is overwritten before any read on every
// outgoing CFG path. This avoids lifting unused CPU condition-code computations
// into YakSSA, significantly reducing IR allocation and instruction counts.
func DropDeadFlags(function *core.Function, facts core.ArchFacts) int {
	if function == nil || len(function.Blocks) == 0 {
		return 0
	}
	flagsName := strings.ToLower(strings.TrimSpace(facts.Registers.Flags))
	if flagsName == "" && facts.Registers.Naming == core.RegisterNamingX86 {
		flagsName = "rflags"
	}
	if flagsName == "" {
		return 0
	}
	isFlagsReg := func(name string) bool {
		lower := strings.ToLower(strings.TrimSpace(name))
		if lower == "" {
			return false
		}
		if lower == flagsName {
			return true
		}
		if facts.Registers.Naming == core.RegisterNamingX86 {
			return lower == "rflags" || lower == "eflags" || lower == "flags"
		}
		return false
	}

	nBlocks := len(function.Blocks)
	blockIdx := make(map[core.BlockID]int, nBlocks)
	for i, b := range function.Blocks {
		blockIdx[b.ID] = i
	}

	// Backward 1-bit liveness for the flags register across blocks.
	// transfer[b] summarizes block b's upward-exposed use / kill before solving fixpoint.
	gen := make([]bool, nBlocks)  // reads flags before writing flags
	kill := make([]bool, nBlocks) // writes flags inside block
	succs := make([][]int, nBlocks)
	for i, b := range function.Blocks {
		// Walk backward within block to summarize gen/kill cleanly:
		// a write sets live=false; a read sets live=true.
		live := false
		hasKill := false
		for j := len(b.Instructions) - 1; j >= 0; j-- {
			inst := b.Instructions[j]
			if writesFlags(inst, isFlagsReg) {
				live = false
				hasKill = true
			} else if readsFlags(inst, isFlagsReg) {
				live = true
			}
		}
		gen[i] = live
		kill[i] = hasKill
		if len(b.Instructions) > 0 {
			last := b.Instructions[len(b.Instructions)-1]
			for _, t := range last.Targets {
				if idx, ok := blockIdx[t]; ok {
					succs[i] = append(succs[i], idx)
				}
			}
		}
	}

	liveIn := make([]bool, nBlocks)
	liveOut := make([]bool, nBlocks)
	for changed := true; changed; {
		changed = false
		for i := nBlocks - 1; i >= 0; i-- {
			out := false
			for _, s := range succs[i] {
				if liveIn[s] {
					out = true
					break
				}
			}
			in := gen[i] || (out && !kill[i])
			if out != liveOut[i] || in != liveIn[i] {
				liveOut[i] = out
				liveIn[i] = in
				changed = true
			}
		}
	}

	// Pass 1: mark dead flag-register writes and count surviving uses of ValueIDs.
	dead := make([][]bool, nBlocks)
	uses := make(map[core.ValueID]int)
	for i, b := range function.Blocks {
		dead[i] = make([]bool, len(b.Instructions))
		live := liveOut[i]
		for j := len(b.Instructions) - 1; j >= 0; j-- {
			inst := b.Instructions[j]
			if writesFlags(inst, isFlagsReg) {
				if !live {
					dead[i][j] = true
				}
				live = false
				continue
			}
			if readsFlags(inst, isFlagsReg) {
				live = true
			}
		}
		for j, inst := range b.Instructions {
			if dead[i][j] {
				continue
			}
			for _, op := range inst.Operands {
				if op.Kind == core.OperandValue && op.Value != 0 {
					uses[op.Value]++
				}
			}
		}
	}

	// Pass 2: remove flags.* intrinsics whose result ValueID now has 0 uses
	// (iterating to fixed point handles any copy/chain feeding only dead flag writes).
	for changed := true; changed; {
		changed = false
		for i, b := range function.Blocks {
			for j, inst := range b.Instructions {
				if dead[i][j] || !isDeadFlagIntrinsic(inst, uses) {
					continue
				}
				dead[i][j] = true
				changed = true
				for _, op := range inst.Operands {
					if op.Kind == core.OperandValue && op.Value != 0 {
						uses[op.Value]--
					}
				}
			}
		}
	}

	removed := 0
	for i := range function.Blocks {
		insts := function.Blocks[i].Instructions
		kept := insts[:0]
		for j, inst := range insts {
			if dead[i][j] {
				removed++
				continue
			}
			kept = append(kept, inst)
		}
		function.Blocks[i].Instructions = kept
	}
	return removed
}

func writesFlags(inst core.Instruction, isFlagsReg func(string) bool) bool {
	if inst.Kind != core.OpWriteRegister || len(inst.Operands) < 1 {
		return false
	}
	op := inst.Operands[0]
	return op.Kind == core.OperandRegister && isFlagsReg(op.Name)
}

func readsFlags(inst core.Instruction, isFlagsReg func(string) bool) bool {
	if inst.Kind == core.OpReadRegister && len(inst.Operands) >= 1 {
		op := inst.Operands[0]
		if op.Kind == core.OperandRegister && isFlagsReg(op.Name) {
			return true
		}
	}
	for idx, op := range inst.Operands {
		if idx == 0 && inst.Kind == core.OpWriteRegister {
			continue
		}
		if op.Kind == core.OperandRegister && isFlagsReg(op.Name) {
			return true
		}
	}
	// Intrinsics with EffectReadRegister or EffectUnknown (excluding flags.* generators)
	// may consume CPU flags (e.g. adc/sbb/rcl/pushf/cond.*).
	if inst.Kind == core.OpIntrinsic {
		name := ""
		if len(inst.Operands) > 0 && inst.Operands[0].Kind == core.OperandSymbol {
			name = inst.Operands[0].Name
		}
		if strings.HasPrefix(name, core.FlagsIntrinsicPrefix) {
			return false
		}
		if strings.HasPrefix(name, core.ConditionIntrinsicPrefix) ||
			inst.Effects&(core.EffectReadRegister|core.EffectUnknown) != 0 {
			return true
		}
	}
	return false
}

func isDeadFlagIntrinsic(inst core.Instruction, uses map[core.ValueID]int) bool {
	if inst.Kind != core.OpIntrinsic || len(inst.Results) != 1 {
		return false
	}
	if len(inst.Operands) == 0 || inst.Operands[0].Kind != core.OperandSymbol {
		return false
	}
	if !strings.HasPrefix(inst.Operands[0].Name, core.FlagsIntrinsicPrefix) {
		return false
	}
	return uses[inst.Results[0].ID] <= 0
}
