package pass

import "github.com/yaklang/yaklang/common/yak/reify/core"

// RemoveStackFrame removes standard frame setup and teardown from one LowIR
// function: callee-saved pushes, the frame-pointer move, stack allocation, the
// epilogue pop and the matching stack release. It returns the number of removed
// instructions.
//
// The register names come from the frame model, so one implementation serves
// every architecture: rsp/rbp on amd64, esp/ebp on 386, sp/r11 on ARM, r15/r11 on
// s390x, r3/r22 on loong64. A model without a stack pointer removes nothing,
// because without it the pattern cannot be told apart from ordinary code.
//
// Frame-relative locals are not rewritten. Removing the frame therefore keeps
// the code readable but can make a frame-relative address lose its base.
func RemoveStackFrame(function *core.Function, facts core.ArchFacts) int {
	frame := facts.Frame
	if function == nil || len(function.Blocks) == 0 || frame.StackPointer == "" {
		return 0
	}
	removed := make(map[instructionRef]bool)
	removeBlockFrames(function, frame, removed)
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
func removeBlockFrames(function *core.Function, frame core.FrameModel, removed map[instructionRef]bool) {
	for blockIndex := range function.Blocks {
		groups := blockFrameGroups(function, blockIndex)
		if len(groups) == 0 {
			continue
		}
		// A prologue can be interleaved with unrelated setup instructions, so
		// inspect a small window instead of requiring contiguous groups.
		if blockUsesFrameBase(groups, frame) {
			markReturnEpilogue(groups, frame, removed)
			continue
		}
		window := len(groups)
		if window > 8 {
			window = 8
		}
		hasPrologue := blockIndex == 0
		for index := 0; index < window && !hasPrologue; index++ {
			if frameHasCalleePush(groups[index], frame) || frameHasFramePointer(groups[index], frame) {
				hasPrologue = true
			}
		}
		if hasPrologue {
			for index := 0; index < window; index++ {
				if frameSetup(groups[index], frame) {
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
			if !frameTeardown(group, frame) {
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

// markReturnEpilogue removes teardown groups even when frame-relative locals
// keep the prologue. These values only restore the stack and frame pointers and
// are not part of the function result.
func markReturnEpilogue(groups []frameGroup, frame core.FrameModel, removed map[instructionRef]bool) {
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
		if !frameTeardown(group, frame) {
			break
		}
		markFrameGroup(removed, group)
		end--
	}
}

func blockUsesFrameBase(groups []frameGroup, frame core.FrameModel) bool {
	if frame.FramePointer == "" {
		return false
	}
	for _, group := range groups {
		if frameNoise(group) || frameSetup(group, frame) || frameTeardown(group, frame) {
			continue
		}
		if frameReads(group, frame.FramePointer) {
			return true
		}
	}
	return false
}

func frameHasCalleePush(group frameGroup, frame core.FrameModel) bool {
	return frameWrites(group, frame.StackPointer) && frameHasStore(group) && frameReadsCalleeSaved(group, frame)
}

func frameHasFramePointer(group frameGroup, frame core.FrameModel) bool {
	if frame.FramePointer == "" {
		return false
	}
	return frameWrites(group, frame.FramePointer) && frameReads(group, frame.StackPointer)
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
		core.IsLandingPadIntrinsic(instruction.Operands[0].Name)
}

func frameTerminator(group frameGroup) bool {
	for _, item := range group.instructions {
		if item.instruction.Kind == core.OpReturn || item.instruction.Kind == core.OpTrap {
			return true
		}
	}
	return false
}

func frameSetup(group frameGroup, frame core.FrameModel) bool {
	writesStack := frameWrites(group, frame.StackPointer)
	readsStack := frameReads(group, frame.StackPointer)
	if frameHasFramePointer(group, frame) {
		return true
	}
	if writesStack && (frameHasKind(group, core.OpSub) || frameHasKind(group, core.OpAnd)) && readsStack {
		return true
	}
	return writesStack && frameHasStore(group) && frameReadsCalleeSaved(group, frame)
}

func frameTeardown(group frameGroup, frame core.FrameModel) bool {
	if frameHasLoad(group) && frameReads(group, frame.StackPointer) && frameWritesCalleeSaved(group, frame) {
		return true
	}
	if frame.FramePointer != "" && frameWrites(group, frame.StackPointer) && frameReads(group, frame.FramePointer) {
		return true
	}
	if frameHasKind(group, core.OpAdd) && frameReads(group, frame.StackPointer) && frameWritesOnlyStack(group, frame) {
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

// frameReadsCalleeSaved reports whether one group saves a register a callee must
// preserve. Saving the return address is part of the same prologue on a
// link-register target, where the callee has no stack slot for it by default.
func frameReadsCalleeSaved(group frameGroup, frame core.FrameModel) bool {
	for _, name := range frame.SavedRegisters() {
		if frameReads(group, name) {
			return true
		}
	}
	return false
}

func frameWritesCalleeSaved(group frameGroup, frame core.FrameModel) bool {
	for _, name := range frame.SavedRegisters() {
		if frameWrites(group, name) {
			return true
		}
	}
	return false
}

// frameWritesOnlyStack reports whether one group writes nothing but the stack
// pointer. A flag update may accompany the adjustment, because some targets
// compute the new stack pointer with a flag-setting subtract.
func frameWritesOnlyStack(group frameGroup, frame core.FrameModel) bool {
	for _, item := range group.instructions {
		instruction := item.instruction
		if instruction.Kind != core.OpWriteRegister || len(instruction.Operands) == 0 {
			continue
		}
		name := instruction.Operands[0].Name
		if name != frame.StackPointer && name != frame.Flags {
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
	// A teardown group can share its address with the terminator: the ARM
	// epilogue restores the frame pointer with a pop whose last register is pc,
	// and the return recorded for it carries the same address. Removing the
	// whole group would delete the terminator and leave the block without a
	// return, so the terminator always survives.
	for _, item := range group.instructions {
		if item.instruction.Kind == core.OpReturn || item.instruction.Kind == core.OpTrap {
			continue
		}
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
func RemoveFrame(function *core.Function, facts core.ArchFacts) int {
	return RemoveStackFrame(function, facts)
}
