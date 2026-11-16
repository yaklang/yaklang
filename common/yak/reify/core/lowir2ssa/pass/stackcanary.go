package pass

import (
	"strings"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// RemoveStackCanary removes stack-protector code from one LowIR function. It
// recognizes the TLS canary load, the stack spill, the reload-and-compare
// check, and the guarded failure call, then redirects the check branch to the
// success path. It returns the number of removed instructions.
//
// The pass works on LowIR, before conversion to YakSSA. It does not remove
// unrelated TLS accesses, and it leaves the function unchanged when the
// pattern is incomplete.
func RemoveStackCanary(function *core.Function, facts core.ArchFacts) int {
	if function == nil || len(function.Blocks) == 0 {
		return 0
	}
	state := newCanaryState(function, facts)
	if !state.scan() {
		return 0
	}
	return state.apply()
}

type instructionRef struct {
	block int
	index int
}

type canaryState struct {
	function *core.Function
	facts    core.ArchFacts
	defs     map[core.ValueID]core.Instruction
	aliases  map[core.ValueID]core.ValueID
	removed  map[instructionRef]bool
	rewrites map[instructionRef]core.BlockID
	fail     map[int]bool

	tlsLoads  map[core.ValueID]bool
	slots     map[string]bool
	reloads   map[core.ValueID]bool
	canary    map[core.ValueID]bool
	registers map[string]core.ValueID
}

func newCanaryState(function *core.Function, facts core.ArchFacts) *canaryState {
	return &canaryState{
		function:  function,
		facts:     facts,
		defs:      make(map[core.ValueID]core.Instruction),
		aliases:   make(map[core.ValueID]core.ValueID),
		removed:   make(map[instructionRef]bool),
		rewrites:  make(map[instructionRef]core.BlockID),
		fail:      make(map[int]bool),
		tlsLoads:  make(map[core.ValueID]bool),
		slots:     make(map[string]bool),
		reloads:   make(map[core.ValueID]bool),
		canary:    make(map[core.ValueID]bool),
		registers: make(map[string]core.ValueID),
	}
}

func (s *canaryState) scan() bool {
	for blockIndex := range s.function.Blocks {
		for _, instruction := range s.function.Blocks[blockIndex].Instructions {
			for _, result := range instruction.Results {
				s.defs[result.ID] = instruction
			}
		}
	}
	// Registers are tracked first: on a thread-pointer target the guard load
	// reads a register the thread-pointer instruction wrote, and the offset is
	// added after that.
	s.scanRegisters()
	if !s.scanCanary() {
		return false
	}
	if !s.scanSpills() {
		return false
	}
	checked := s.scanChecks()
	flagged := s.scanFlagChecks()
	if debugCanary {
		println("canary: tlsLoads", len(s.tlsLoads), "spillSlots", len(s.slots), "reloads", len(s.reloads), "checked", checked, "flagged", flagged)
	}
	return checked || flagged
}

// scanCanary finds the guard value. A lifter marks the thread pointer with the
// neutral tls.base intrinsic, so this scan needs no architecture knowledge: it
// looks for the marker, then for the model's offset, then for the load.
//
// Two shapes occur. A segment base folds the offset into the marker operand
// (x86: fs:0x28); a thread pointer register leaves the offset to an add
// (arm64: tpidr_el0, then ldr [x8,#0x28]).
func (s *canaryState) scanCanary() bool {
	offset := s.facts.Canary.Offset
	if s.facts.Canary.Kind == "" || offset == 0 {
		return false
	}
	addresses := make(map[core.ValueID]bool)
	bases := make(map[core.ValueID]bool)
	for blockIndex := range s.function.Blocks {
		for _, instruction := range s.function.Blocks[blockIndex].Instructions {
			if instruction.Kind != core.OpIntrinsic || len(instruction.Operands) == 0 {
				continue
			}
			if instruction.Operands[0].Name != core.TLSBaseIntrinsic {
				continue
			}
			if len(instruction.Results) == 0 {
				continue
			}
			// A segment base folds the offset into the marker; a thread pointer
			// register leaves it to an add after the marker.
			if len(instruction.Operands) >= 2 && s.constant(s.resolve(instruction.Operands[1].Value)) == offset {
				addresses[instruction.Results[0].ID] = true
				continue
			}
			bases[instruction.Results[0].ID] = true
		}
	}
	// The offset may be added after the marker, in which case the sum is the
	// guard address.
	for blockIndex := range s.function.Blocks {
		for _, instruction := range s.function.Blocks[blockIndex].Instructions {
			if instruction.Kind != core.OpAdd || len(instruction.Operands) < 2 || len(instruction.Results) == 0 {
				continue
			}
			left, right := s.resolve(instruction.Operands[0].Value), s.resolve(instruction.Operands[1].Value)
			if bases[left] && s.constant(right) == offset {
				addresses[instruction.Results[0].ID] = true
			}
			if bases[right] && s.constant(left) == offset {
				addresses[instruction.Results[0].ID] = true
			}
		}
	}
	if len(addresses) == 0 {
		return false
	}
	found := false
	for blockIndex := range s.function.Blocks {
		for _, instruction := range s.function.Blocks[blockIndex].Instructions {
			if instruction.Kind != core.OpLoad || len(instruction.Operands) == 0 || len(instruction.Results) == 0 {
				continue
			}
			if !addresses[s.resolve(instruction.Operands[0].Value)] {
				continue
			}
			s.tlsLoads[instruction.Results[0].ID] = true
			s.canary[instruction.Results[0].ID] = true
			found = true
		}
	}
	return found
}

// resolve follows the alias chain of one value so a scan matches the value a
// register read or a copy stands for, not only the instruction that produced it.
// A register read is linked by scanRegisters; a copy links through its operand.
func (s *canaryState) resolve(id core.ValueID) core.ValueID {
	for seen := 0; seen < 32; seen++ {
		if next, ok := s.aliases[id]; ok && next != id {
			id = next
			continue
		}
		definition, ok := s.defs[id]
		if !ok || definition.Kind != core.OpCopy || len(definition.Operands) == 0 {
			return id
		}
		next := definition.Operands[0].Value
		if next == id {
			return id
		}
		id = next
	}
	return id
}

func (s *canaryState) scanRegisters() {
	for blockIndex := range s.function.Blocks {
		for _, instruction := range s.function.Blocks[blockIndex].Instructions {
			switch instruction.Kind {
			case core.OpWriteRegister:
				if len(instruction.Operands) < 2 {
					continue
				}
				name := registerViewFor(instruction.Operands[0].Name, 0).canonical
				if name == "" {
					continue
				}
				s.registers[name] = s.trace(instruction.Operands[1].Value)
			case core.OpReadRegister:
				if len(instruction.Results) == 0 || len(instruction.Operands) == 0 {
					continue
				}
				name := registerViewFor(instruction.Operands[0].Name, 0).canonical
				if name == "" {
					continue
				}
				if value, ok := s.registers[name]; ok {
					s.aliases[instruction.Results[0].ID] = value
				}
			}
		}
	}
}

var debugCanary = false

func (s *canaryState) scanSpills() bool {
	found := false
	for blockIndex := range s.function.Blocks {
		for index, instruction := range s.function.Blocks[blockIndex].Instructions {
			if instruction.Kind != core.OpStore || len(instruction.Operands) < 2 {
				continue
			}
			if !s.isCanary(instruction.Operands[1].Value) {
				continue
			}
			key := s.addressKey(instruction.Operands[0].Value)
			if key == "" {
				continue
			}
			s.slots[key] = true
			s.markRemoved(blockIndex, index)
			found = true
		}
	}
	if debugCanary {
		println("canary: spill found", found)
	}
	if !found {
		return false
	}
	for blockIndex := range s.function.Blocks {
		for _, instruction := range s.function.Blocks[blockIndex].Instructions {
			if instruction.Kind != core.OpLoad || len(instruction.Operands) == 0 || len(instruction.Results) == 0 {
				continue
			}
			key := s.addressKey(instruction.Operands[0].Value)
			if key == "" || !s.slots[key] {
				continue
			}
			s.reloads[instruction.Results[0].ID] = true
			s.canary[instruction.Results[0].ID] = true
		}
	}
	return true
}

func (s *canaryState) scanChecks() bool {
	found := false
	for blockIndex := range s.function.Blocks {
		block := &s.function.Blocks[blockIndex]
		for index := range block.Instructions {
			instruction := &block.Instructions[index]
			if !s.isCanaryCompare(*instruction) {
				continue
			}
			if len(instruction.Results) == 0 {
				continue
			}
			compareID := instruction.Results[0].ID
			flags, flagsRef, ok := s.singleUserIntrinsic(blockIndex, index, compareID, core.FlagsIntrinsicPrefix)
			if !ok || len(flags.Results) == 0 {
				continue
			}
			condition, conditionRef, ok := s.singleUserIntrinsic(flagsRef.block, flagsRef.index, flags.Results[0].ID, core.ConditionIntrinsicPrefix)
			if !ok || len(condition.Results) == 0 {
				continue
			}
			branchRef, branch, ok := s.branchOn(conditionRef.block, conditionRef.index, condition.Results[0].ID)
			if !ok {
				continue
			}
			okBlock, failBlocks, ok := s.checkTargets(branch)
			if !ok {
				continue
			}
			for _, ref := range []instructionRef{{block: blockIndex, index: index}, flagsRef, conditionRef, branchRef} {
				s.markRemoved(ref.block, ref.index)
			}
			s.rewrites[branchRef] = okBlock
			for _, fail := range failBlocks {
				s.fail[fail] = true
			}
			found = true
		}
	}
	return found
}

// scanFlagChecks handles comparisons that the lifter folds directly into a
// flags intrinsic, such as cmp %fs:0x28, %rax.
func (s *canaryState) scanFlagChecks() bool {
	found := false
	for blockIndex := range s.function.Blocks {
		for index := range s.function.Blocks[blockIndex].Instructions {
			instruction := s.function.Blocks[blockIndex].Instructions[index]
			if instruction.Kind != core.OpIntrinsic || len(instruction.Operands) == 0 {
				continue
			}
			name := instruction.Operands[0].Name
			if !strings.HasPrefix(name, core.FlagsIntrinsicPrefix) {
				continue
			}
			if len(instruction.Results) == 0 || !s.flagCheckOperands(instruction) {
				continue
			}
			flagsRef := instructionRef{block: blockIndex, index: index}
			if s.removed[flagsRef] {
				continue
			}
			condition, conditionRef, ok := s.singleUserIntrinsic(blockIndex, index, instruction.Results[0].ID, core.ConditionIntrinsicPrefix)
			if !ok || len(condition.Results) == 0 {
				continue
			}
			branchRef, branch, ok := s.branchOn(conditionRef.block, conditionRef.index, condition.Results[0].ID)
			if !ok {
				continue
			}
			okBlock, failBlocks, ok := s.checkTargets(branch)
			if !ok {
				continue
			}
			s.markRemoved(flagsRef.block, flagsRef.index)
			s.markRemoved(conditionRef.block, conditionRef.index)
			s.markRemoved(branchRef.block, branchRef.index)
			s.rewrites[branchRef] = okBlock
			for _, fail := range failBlocks {
				s.fail[fail] = true
			}
			found = true
		}
	}
	return found
}

func (s *canaryState) flagCheckOperands(instruction core.Instruction) bool {
	hasCanary := false
	hasLoad := false
	for _, operand := range instruction.Operands[1:] {
		if operand.Kind != core.OperandValue {
			continue
		}
		value := s.trace(operand.Value)
		if s.isCanary(value) {
			hasCanary = true
		}
		if s.isLoadResult(value) {
			hasLoad = true
		}
	}
	return hasCanary && hasLoad
}

func (s *canaryState) isCanaryCompare(instruction core.Instruction) bool {
	switch instruction.Kind {
	case core.OpSub, core.OpXor, core.OpEqual:
	default:
		return false
	}
	if len(instruction.Operands) < 2 {
		return false
	}
	left := s.trace(instruction.Operands[0].Value)
	right := s.trace(instruction.Operands[1].Value)
	leftCanary := s.isCanary(left)
	rightCanary := s.isCanary(right)
	if !leftCanary && !rightCanary {
		return false
	}
	if leftCanary && rightCanary {
		return true
	}
	// One side is the TLS canary. The other side is the reloaded stack value;
	// accept any load so a different addressing form for the same slot does not
	// hide the check.
	other := left
	if rightCanary {
		other = right
	}
	return s.isLoadResult(other)
}

func (s *canaryState) isLoadResult(id core.ValueID) bool {
	instruction, ok := s.defs[id]
	return ok && instruction.Kind == core.OpLoad
}

func (s *canaryState) singleUserIntrinsic(blockIndex, index int, value core.ValueID, prefix string) (core.Instruction, instructionRef, bool) {
	for userBlock := blockIndex; userBlock < len(s.function.Blocks); userBlock++ {
		start := 0
		if userBlock == blockIndex {
			start = index + 1
		}
		for userIndex := start; userIndex < len(s.function.Blocks[userBlock].Instructions); userIndex++ {
			instruction := s.function.Blocks[userBlock].Instructions[userIndex]
			if instruction.Kind != core.OpIntrinsic || len(instruction.Operands) == 0 {
				continue
			}
			if len(instruction.Operands[0].Name) < len(prefix) || instruction.Operands[0].Name[:len(prefix)] != prefix {
				continue
			}
			for _, operand := range instruction.Operands[1:] {
				if operand.Kind != core.OperandValue {
					continue
				}
				if operand.Value == value || s.trace(operand.Value) == value {
					return instruction, instructionRef{block: userBlock, index: userIndex}, true
				}
			}
		}
	}
	return core.Instruction{}, instructionRef{}, false
}

func (s *canaryState) branchOn(blockIndex, index int, value core.ValueID) (instructionRef, core.Instruction, bool) {
	for userBlock := blockIndex; userBlock < len(s.function.Blocks); userBlock++ {
		start := 0
		if userBlock == blockIndex {
			start = index + 1
		}
		for userIndex := start; userIndex < len(s.function.Blocks[userBlock].Instructions); userIndex++ {
			instruction := s.function.Blocks[userBlock].Instructions[userIndex]
			if instruction.Kind != core.OpBranch || len(instruction.Operands) == 0 {
				continue
			}
			operand := instruction.Operands[0]
			if operand.Kind == core.OperandValue && operand.Value == value {
				return instructionRef{block: userBlock, index: userIndex}, instruction, true
			}
		}
	}
	return instructionRef{}, core.Instruction{}, false
}

func (s *canaryState) checkTargets(branch core.Instruction) (core.BlockID, []int, bool) {
	if len(branch.Targets) != 2 {
		return 0, nil, false
	}
	first, second := branch.Targets[0], branch.Targets[1]
	firstBlock := s.blockIndex(first)
	secondBlock := s.blockIndex(second)
	if firstBlock < 0 || secondBlock < 0 {
		return 0, nil, false
	}
	firstFail := s.isFailureBlock(firstBlock)
	secondFail := s.isFailureBlock(secondBlock)
	switch {
	case firstFail && !secondFail:
		return second, []int{firstBlock}, true
	case secondFail && !firstFail:
		return first, []int{secondBlock}, true
	default:
		return 0, nil, false
	}
}

func (s *canaryState) isFailureBlock(blockIndex int) bool {
	block := &s.function.Blocks[blockIndex]
	for _, instruction := range block.Instructions {
		// The failure handler does not return. The linear decoder may keep
		// decoding the next function into the same block, so the block does not
		// need a terminator after the call.
		if instruction.Kind == core.OpCall || instruction.Kind == core.OpTrap {
			return true
		}
	}
	return false
}

func (s *canaryState) blockIndex(id core.BlockID) int {
	for index := range s.function.Blocks {
		if s.function.Blocks[index].ID == id {
			return index
		}
	}
	return -1
}

func (s *canaryState) apply() int {
	s.markWritesOfCanaryValues()
	before := s.countInstructions()
	for blockIndex := range s.function.Blocks {
		block := &s.function.Blocks[blockIndex]
		instructions := make([]core.Instruction, 0, len(block.Instructions))
		for index, instruction := range block.Instructions {
			ref := instructionRef{block: blockIndex, index: index}
			if target, ok := s.rewrites[ref]; ok {
				instructions = append(instructions, core.Instruction{
					Kind:    core.OpJump,
					Targets: []core.BlockID{target},
					Address: instruction.Address,
				})
				continue
			}
			if s.removed[ref] {
				continue
			}
			instructions = append(instructions, instruction)
		}
		block.Instructions = instructions
	}
	s.deadCodeElimination()
	s.removeUnreachable()
	return before - s.countInstructions()
}

// markWritesOfCanaryValues removes register assignments whose source is a
// canary value or a value defined by an already removed instruction. Without
// this, the assignment keeps the canary load alive and the load keeps the TLS
// access alive.
func (s *canaryState) markWritesOfCanaryValues() {
	removedValues := make(map[core.ValueID]bool)
	for ref := range s.removed {
		for _, result := range s.function.Blocks[ref.block].Instructions[ref.index].Results {
			removedValues[result.ID] = true
		}
	}
	for blockIndex := range s.function.Blocks {
		for index, instruction := range s.function.Blocks[blockIndex].Instructions {
			if instruction.Kind != core.OpWriteRegister || len(instruction.Operands) < 2 {
				continue
			}
			value := instruction.Operands[1].Value
			if removedValues[value] || s.isCanary(value) {
				s.markRemoved(blockIndex, index)
			}
		}
	}
}

func (s *canaryState) deadCodeElimination() {
	for {
		used := make(map[core.ValueID]bool)
		for blockIndex := range s.function.Blocks {
			for _, instruction := range s.function.Blocks[blockIndex].Instructions {
				for _, operand := range instruction.Operands {
					if operand.Kind == core.OperandValue {
						used[operand.Value] = true
					}
				}
			}
		}
		changed := false
		for blockIndex := range s.function.Blocks {
			block := &s.function.Blocks[blockIndex]
			instructions := make([]core.Instruction, 0, len(block.Instructions))
			for _, instruction := range block.Instructions {
				if removableWhenUnused(instruction) && !hasUsedResult(instruction, used) {
					changed = true
					continue
				}
				instructions = append(instructions, instruction)
			}
			block.Instructions = instructions
		}
		if !changed {
			return
		}
	}
}

func removableWhenUnused(instruction core.Instruction) bool {
	if len(instruction.Results) == 0 {
		return false
	}
	// A thread-pointer or segment read carries EffectUnknown, but an address
	// that no remaining instruction uses has no observable effect of its own.
	if instruction.Kind == core.OpIntrinsic && baseAddressIntrinsic(instruction) {
		return true
	}
	const allowed = core.EffectReadMemory | core.EffectReadRegister
	return instruction.Effects&^allowed == 0
}

// baseAddressIntrinsic reports whether one intrinsic only computes the address of
// the thread pointer or the stack guard. Both are marked neutrally by the lifter.
func baseAddressIntrinsic(instruction core.Instruction) bool {
	if len(instruction.Operands) == 0 || instruction.Operands[0].Kind != core.OperandSymbol {
		return false
	}
	name := instruction.Operands[0].Name
	return name == core.TLSBaseIntrinsic || name == core.CanaryIntrinsic
}

func hasUsedResult(instruction core.Instruction, used map[core.ValueID]bool) bool {
	for _, result := range instruction.Results {
		if used[result.ID] {
			return true
		}
	}
	return false
}

func (s *canaryState) removeUnreachable() {
	if len(s.fail) == 0 {
		return
	}
	referenced := make(map[core.BlockID]bool)
	for blockIndex := range s.function.Blocks {
		for _, instruction := range s.function.Blocks[blockIndex].Instructions {
			for _, target := range instruction.Targets {
				referenced[target] = true
			}
		}
	}
	blocks := make([]core.Block, 0, len(s.function.Blocks))
	for index, block := range s.function.Blocks {
		if index != 0 && s.fail[index] && !referenced[block.ID] {
			continue
		}
		blocks = append(blocks, block)
	}
	s.function.Blocks = blocks
}

func (s *canaryState) countInstructions() int {
	total := 0
	for blockIndex := range s.function.Blocks {
		total += len(s.function.Blocks[blockIndex].Instructions)
	}
	return total
}

func (s *canaryState) markRemoved(block, index int) {
	s.removed[instructionRef{block: block, index: index}] = true
}

func (s *canaryState) isCanary(id core.ValueID) bool {
	resolved := s.trace(id)
	return s.canary[resolved] || s.tlsLoads[resolved] || s.reloads[resolved]
}

func (s *canaryState) trace(id core.ValueID) core.ValueID {
	for depth := 0; depth < 32; depth++ {
		next, ok := s.aliases[id]
		if !ok || next == id {
			return id
		}
		id = next
	}
	return id
}

func (s *canaryState) constant(id core.ValueID) uint64 {
	for depth := 0; depth < 32; depth++ {
		instruction, ok := s.defs[id]
		if !ok {
			return 0
		}
		switch instruction.Kind {
		case core.OpConstant:
			if len(instruction.Operands) == 0 {
				return 0
			}
			return literalUint64(instruction.Operands[0].Literal)
		case core.OpCopy:
			if len(instruction.Operands) == 0 {
				return 0
			}
			id = instruction.Operands[0].Value
		default:
			return 0
		}
	}
	return 0
}

func (s *canaryState) addressKey(id core.ValueID) string {
	return valueKey(s.defs, id)
}

func (s *canaryState) usersOf(blockIndex, index int, value core.ValueID, kind core.Opcode) []core.Instruction {
	var result []core.Instruction
	for block := blockIndex; block < len(s.function.Blocks); block++ {
		start := 0
		if block == blockIndex {
			start = index + 1
		}
		for i := start; i < len(s.function.Blocks[block].Instructions); i++ {
			instruction := s.function.Blocks[block].Instructions[i]
			if instruction.Kind != kind {
				continue
			}
			for _, operand := range instruction.Operands {
				if operand.Kind == core.OperandValue && operand.Value == value {
					result = append(result, instruction)
					break
				}
			}
		}
	}
	return result
}

// Remove is the registry entry point for stack-canary removal.
