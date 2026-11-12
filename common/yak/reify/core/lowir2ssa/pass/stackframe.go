package pass

import "github.com/yaklang/yaklang/common/yak/reify/core"

// RemoveStackFrame removes standard frame setup and teardown from one LowIR
// function: callee-saved pushes, `mov rbp, rsp`, stack allocation, `leave`,
// callee-saved pops, and matching stack release. It returns the number of
// removed instructions.
//
// Frame-relative locals are not rewritten. Removing the frame therefore keeps
// the code readable but can make an rbp-relative address lose its base.
func RemoveStackFrame(function *core.Function) int {
	if function == nil || len(function.Blocks) == 0 {
		return 0
	}
	removed := make(map[instructionRef]bool)
	removeBlockFrames(function, removed)
	if len(removed) == 0 {
		return 0
	}
	before := countFrameInstructions(function)
	for ref := range removed {
		if ref.block >= 0 && ref.block < len(function.Blocks) && ref.index >= 0 && ref.index < len(function.Blocks[ref.block].Instructions) {
			function.Blocks[ref.block].Instructions[ref.index].Effects |= core.EffectFrameRestore
		}
	}
	for blockIndex := range function.Blocks {
		block := &function.Blocks[blockIndex]
		instructions := make([]core.Instruction, 0, len(block.Instructions))
		for index, instruction := range block.Instructions {
			if removed[instructionRef{block: blockIndex, index: index}] {
				continue
			}
			instructions = append(instructions, instruction)
		}
		block.Instructions = instructions
	}
	return before - countFrameInstructions(function)
}

type frameGroup struct {
	address      uint64
	instructions []frameGroupInstruction
}

type frameGroupInstruction struct {
	ref         instructionRef
	instruction core.Instruction
}

// removeBlockFrames trims frame setup and teardown at every block boundary, so
// functions recovered inside one linear scan also lose their frames.
func removeBlockFrames(function *core.Function, removed map[instructionRef]bool) {
	for blockIndex := range function.Blocks {
		groups := blockFrameGroups(function, blockIndex)
		if len(groups) == 0 {
			continue
		}
		// A prologue can be interleaved with unrelated setup instructions, so
		// inspect a small window instead of requiring contiguous groups.
		if blockUsesFrameBase(groups) {
			markReturnEpilogue(groups, removed)
			continue
		}
		window := len(groups)
		if window > 8 {
			window = 8
		}
		hasPrologue := blockIndex == 0
		for index := 0; index < window && !hasPrologue; index++ {
			if frameHasCalleePush(groups[index]) || frameHasFramePointer(groups[index]) {
				hasPrologue = true
			}
		}
		if hasPrologue {
			for index := 0; index < window; index++ {
				if frameSetup(groups[index]) {
					markFrameGroup(removed, groups[index])
				}
			}
		}
		terminator := frameTerminatorKind(function, blockIndex)
		if terminator != core.OpReturn && terminator != core.OpTrap {
			continue
		}
		tailStart := len(groups) - window
		end := len(groups) - 1
		if frameTerminator(groups[end]) {
			end--
		}
		for end >= tailStart {
			group := groups[end]
			if frameNoise(group) {
				end--
				continue
			}
			if !frameTeardown(group) {
				break
			}
			markFrameGroup(removed, group)
			end--
		}
	}
}

func blockFrameGroups(function *core.Function, blockIndex int) []frameGroup {
	var groups []frameGroup
	for index, instruction := range function.Blocks[blockIndex].Instructions {
		if len(groups) == 0 || groups[len(groups)-1].address != instruction.Address {
			groups = append(groups, frameGroup{address: instruction.Address})
		}
		group := &groups[len(groups)-1]
		group.instructions = append(group.instructions, frameGroupInstruction{
			ref:         instructionRef{block: blockIndex, index: index},
			instruction: instruction,
		})
	}
	return groups
}

func frameTerminatorKind(function *core.Function, blockIndex int) core.Opcode {
	instructions := function.Blocks[blockIndex].Instructions
	if len(instructions) == 0 {
		return ""
	}
	return instructions[len(instructions)-1].Kind
}

// markReturnEpilogue removes teardown groups even when rbp-relative locals
// keep the prologue. These values only restore rsp/rbp and are not part of the
// function result.
func markReturnEpilogue(groups []frameGroup, removed map[instructionRef]bool) {
	if len(groups) == 0 || !frameTerminator(groups[len(groups)-1]) {
		return
	}
	end := len(groups) - 1
	if frameTerminator(groups[end]) {
		end--
	}
	for end >= 0 {
		group := groups[end]
		if frameNoise(group) {
			end--
			continue
		}
		if !frameTeardown(group) {
			break
		}
		markFrameGroup(removed, group)
		end--
	}
}

func blockUsesFrameBase(groups []frameGroup) bool {
	for _, group := range groups {
		if frameNoise(group) || frameSetup(group) || frameTeardown(group) {
			continue
		}
		if frameReads(group, "rbp") {
			return true
		}
	}
	return false
}

func frameHasCalleePush(group frameGroup) bool {
	return frameWrites(group, "rsp") && frameHasStore(group) && frameReadsCalleeSaved(group)
}

func frameHasFramePointer(group frameGroup) bool {
	return frameWrites(group, "rbp") && frameReads(group, "rsp")
}

func frameNoise(group frameGroup) bool {
	if len(group.instructions) != 1 {
		return false
	}
	instruction := group.instructions[0].instruction
	if instruction.Kind == core.OpNop {
		return true
	}
	return instruction.Kind == core.OpIntrinsic && len(instruction.Operands) > 0 &&
		instruction.Operands[0].Name == "x86.endbr64"
}

func frameTerminator(group frameGroup) bool {
	for _, item := range group.instructions {
		if item.instruction.Kind == core.OpReturn || item.instruction.Kind == core.OpTrap {
			return true
		}
	}
	return false
}

func frameSetup(group frameGroup) bool {
	writesRSP := frameWrites(group, "rsp")
	if frameWrites(group, "rbp") && frameReads(group, "rsp") {
		return true
	}
	if writesRSP && (frameHasKind(group, core.OpSub) || frameHasKind(group, core.OpAnd)) && frameReads(group, "rsp") {
		return true
	}
	return writesRSP && frameHasStore(group) && frameReadsCalleeSaved(group)
}

func frameTeardown(group frameGroup) bool {
	if frameHasLoad(group) && frameReads(group, "rsp") && frameWritesCalleeSaved(group) {
		return true
	}
	if frameWrites(group, "rsp") && frameReads(group, "rbp") {
		return true
	}
	if frameHasKind(group, core.OpAdd) && frameReads(group, "rsp") && frameWritesOnlyRSP(group) {
		return true
	}
	return false
}

func frameWrites(group frameGroup, name string) bool {
	for _, item := range group.instructions {
		instruction := item.instruction
		if instruction.Kind != core.OpWriteRegister || len(instruction.Operands) == 0 {
			continue
		}
		if instruction.Operands[0].Name == name {
			return true
		}
	}
	return false
}

func frameReads(group frameGroup, name string) bool {
	for _, item := range group.instructions {
		instruction := item.instruction
		if instruction.Kind != core.OpReadRegister || len(instruction.Operands) == 0 {
			continue
		}
		if instruction.Operands[0].Name == name {
			return true
		}
	}
	return false
}

func frameReadsCalleeSaved(group frameGroup) bool {
	for _, name := range []string{"rbp", "rbx", "r12", "r13", "r14", "r15"} {
		if frameReads(group, name) {
			return true
		}
	}
	return false
}

func frameWritesCalleeSaved(group frameGroup) bool {
	for _, name := range []string{"rbp", "rbx", "r12", "r13", "r14", "r15"} {
		if frameWrites(group, name) {
			return true
		}
	}
	return false
}

func frameWritesOnlyRSP(group frameGroup) bool {
	for _, item := range group.instructions {
		instruction := item.instruction
		if instruction.Kind != core.OpWriteRegister || len(instruction.Operands) == 0 {
			continue
		}
		name := instruction.Operands[0].Name
		if name != "rsp" && name != "rflags" {
			return false
		}
	}
	return true
}

func frameHasKind(group frameGroup, kind core.Opcode) bool {
	for _, item := range group.instructions {
		if item.instruction.Kind == kind {
			return true
		}
	}
	return false
}

func frameHasStore(group frameGroup) bool { return frameHasKind(group, core.OpStore) }
func frameHasLoad(group frameGroup) bool  { return frameHasKind(group, core.OpLoad) }

func markFrameGroup(removed map[instructionRef]bool, group frameGroup) {
	for _, item := range group.instructions {
		removed[item.ref] = true
	}
}

func countFrameInstructions(function *core.Function) int {
	total := 0
	for blockIndex := range function.Blocks {
		total += len(function.Blocks[blockIndex].Instructions)
	}
	return total
}

// RemoveFrame is the registry entry point for stack-frame removal.
func RemoveFrame(function *core.Function) int { return RemoveStackFrame(function) }
