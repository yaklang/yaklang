package arm

import (
	"golang.org/x/arch/arm/armasm"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// memoryOperand returns the effective pointer of one memory operand. Pre-indexed
// and post-indexed forms also update the base register.
func (l *lifter) memoryOperand(ins instruction, arg armasm.Arg) (core.ValueID, bool) {
	base, index, shift, displacement, memory, ok := memoryAddress(arg)
	if !ok {
		return 0, false
	}
	pointer := l.addressFromBase(ins, base, displacement)
	if index != "" {
		indexValue := l.readRegister(ins.address, index, 32)
		if shift != 0 {
			count := l.constant(ins.address, uint64(shift), 32)
			indexValue = l.operation(ins, core.OpShl, []core.Operand{l.value(indexValue), l.value(count)}, 32, core.EffectNone)
		}
		kind := core.OpAdd
		if isSignedIndex(memory) {
			kind = core.OpSub
		}
		pointer = l.operation(ins, kind, []core.Operand{l.value(pointer), l.value(indexValue)}, 32, core.EffectNone)
	}
	if memory.Mode == armasm.AddrPreIndex {
		l.writeRegister(ins.address, base, 32, l.pointerFrom(ins.address, pointer))
	}
	return pointer, true
}

// postUpdate writes the base register for a post-indexed operand.
func (l *lifter) postUpdate(ins instruction, arg armasm.Arg) {
	_, index, shift, displacement, memory, ok := memoryAddress(arg)
	if !ok || memory.Mode != armasm.AddrPostIndex {
		return
	}
	base, _, _ := registerName(memory.Base)
	if base == "" {
		return
	}
	amount := displacement
	if index != "" {
		// A register post-index adds a register, not an immediate.
		value := l.readRegister(ins.address, index, 32)
		if shift != 0 {
			count := l.constant(ins.address, uint64(shift), 32)
			value = l.operation(ins, core.OpShl, []core.Operand{l.value(value), l.value(count)}, 32, core.EffectNone)
		}
		current := l.readRegister(ins.address, base, 32)
		updated := l.operation(ins, core.OpAdd, []core.Operand{l.value(current), l.value(value)}, 32, core.EffectNone)
		l.writeRegister(ins.address, base, 32, updated)
		return
	}
	current := l.readRegister(ins.address, base, 32)
	kind := core.OpAdd
	if amount < 0 {
		kind = core.OpSub
		amount = -amount
	}
	offset := l.constant(ins.address, uint64(amount), 32)
	updated := l.operation(ins, kind, []core.Operand{l.value(current), l.value(offset)}, 32, core.EffectNone)
	l.writeRegister(ins.address, base, 32, updated)
}

// load lifts LDR and its byte, halfword and signed variants.
func (l *lifter) load(ins instruction) {
	args := args(ins)
	if len(args) != 2 {
		l.unsupported(ins)
		return
	}
	destination, bits, ok := argRegister(args[0])
	if !ok {
		l.unsupported(ins)
		return
	}
	width := accessWidth(baseOp(ins.inst.Op))
	if width == 0 {
		width = 32
	}
	pointer, ok := l.memoryOperand(ins, args[1])
	if !ok {
		l.unsupported(ins)
		return
	}
	value := l.loadValue(ins.address, pointer, width)
	switch baseOp(ins.inst.Op) {
	case armasm.LDRSB, armasm.LDRSH:
		value = l.operation(ins, core.OpSignExtend, []core.Operand{l.value(value)}, bits, core.EffectNone)
	}
	l.writeRegister(ins.address, destination, bits, value)
	l.postUpdate(ins, args[1])
}

// accessWidth returns the memory access width of a load or store opcode.
func accessWidth(op armasm.Op) uint16 {
	switch op {
	case armasm.LDRB, armasm.STRB, armasm.LDRSB, armasm.LDREXB, armasm.STREXB:
		return 8
	case armasm.LDRH, armasm.STRH, armasm.LDRSH, armasm.LDREXH, armasm.STREXH:
		return 16
	case armasm.LDRD, armasm.STRD, armasm.LDREXD, armasm.STREXD:
		return 64
	default:
		return 32
	}
}

// store lifts STR and its byte and halfword variants.
func (l *lifter) store(ins instruction) {
	args := args(ins)
	if len(args) < 2 || len(args) > 3 {
		l.unsupported(ins)
		return
	}
	value, ok := l.source(ins, args[0], 32)
	if !ok {
		l.unsupported(ins)
		return
	}
	pointer, ok := l.memoryOperand(ins, args[1])
	if !ok {
		l.unsupported(ins)
		return
	}
	width := accessWidth(baseOp(ins.inst.Op))
	if width != 32 {
		value = l.operation(ins, core.OpTruncate, []core.Operand{l.value(value)}, width, core.EffectNone)
	}
	l.storeValue(ins.address, pointer, value)
	l.postUpdate(ins, args[1])
}

// loadMultiple lifts LDM and its aliases. Each register in the list becomes a
// separate load with a consecutive address.
func (l *lifter) loadMultiple(ins instruction) {
	args := args(ins)
	if len(args) != 2 {
		l.unsupported(ins)
		return
	}
	list, ok := args[1].(armasm.RegList)
	if !ok {
		l.unsupported(ins)
		return
	}
	base, _, _, displacement, memory, ok := memoryAddress(args[0])
	if !ok {
		l.unsupported(ins)
		return
	}
	pointer := l.addressFromBase(ins, base, displacement)
	cursor := pointer
	for index, name := range registerList(list) {
		if index > 0 {
			offset := l.constant(ins.address, 4, 32)
			cursor = l.addAddress(ins, cursor, offset)
		}
		value := l.loadValue(ins.address, cursor, 32)
		l.writeRegister(ins.address, name, 32, value)
	}
	if memory.Mode == armasm.AddrLDM_WB {
		count := l.constant(ins.address, uint64(4*len(registerList(list))), 32)
		kind := core.OpAdd
		if baseOp(ins.inst.Op) == armasm.LDMDB {
			kind = core.OpSub
		}
		updated := l.operation(ins, kind, []core.Operand{l.value(pointer), l.value(count)}, 32, core.EffectNone)
		l.writeRegister(ins.address, base, 32, updated)
	}
}

// storeMultiple lifts STM and its aliases.
func (l *lifter) storeMultiple(ins instruction) {
	args := args(ins)
	if len(args) != 2 {
		l.unsupported(ins)
		return
	}
	list, ok := args[1].(armasm.RegList)
	if !ok {
		l.unsupported(ins)
		return
	}
	base, _, _, displacement, memory, ok := memoryAddress(args[0])
	if !ok {
		l.unsupported(ins)
		return
	}
	pointer := l.addressFromBase(ins, base, displacement)
	cursor := pointer
	names := registerList(list)
	for index, name := range names {
		if index > 0 {
			offset := l.constant(ins.address, 4, 32)
			cursor = l.addAddress(ins, cursor, offset)
		}
		value := l.readRegister(ins.address, name, 32)
		l.storeValue(ins.address, cursor, value)
	}
	if memory.Mode == armasm.AddrLDM_WB {
		count := l.constant(ins.address, uint64(4*len(names)), 32)
		kind := core.OpAdd
		if baseOp(ins.inst.Op) == armasm.STMDB {
			kind = core.OpSub
		}
		updated := l.operation(ins, kind, []core.Operand{l.value(pointer), l.value(count)}, 32, core.EffectNone)
		l.writeRegister(ins.address, base, 32, updated)
	}
}

// push lifts PUSH, which is STMDB sp! with the decoded register list.
func (l *lifter) push(ins instruction) {
	args := args(ins)
	if len(args) != 1 {
		l.unsupported(ins)
		return
	}
	list, ok := args[0].(armasm.RegList)
	if !ok {
		l.unsupported(ins)
		return
	}
	names := registerList(list)
	sp := l.readRegister(ins.address, "sp", 32)
	count := l.constant(ins.address, uint64(4*len(names)), 32)
	pointer := l.operation(ins, core.OpSub, []core.Operand{l.value(sp), l.value(count)}, 32, core.EffectNone)
	l.writeRegister(ins.address, "sp", 32, pointer)
	storePointer := pointer
	for index, name := range names {
		if index > 0 {
			offset := l.constant(ins.address, 4, 32)
			storePointer = l.addAddress(ins, storePointer, offset)
		}
		value := l.readRegister(ins.address, name, 32)
		l.storeValue(ins.address, storePointer, value)
	}
}

// pop lifts POP, which is LDMIA sp! with the decoded register list.
func (l *lifter) pop(ins instruction) {
	args := args(ins)
	if len(args) != 1 {
		l.unsupported(ins)
		return
	}
	list, ok := args[0].(armasm.RegList)
	if !ok {
		l.unsupported(ins)
		return
	}
	names := registerList(list)
	loadPointer := l.readRegister(ins.address, "sp", 32)
	for index, name := range names {
		if index > 0 {
			offset := l.constant(ins.address, 4, 32)
			loadPointer = l.addAddress(ins, loadPointer, offset)
		}
		value := l.loadValue(ins.address, loadPointer, 32)
		l.writeRegister(ins.address, name, 32, value)
	}
	sp := l.readRegister(ins.address, "sp", 32)
	count := l.constant(ins.address, uint64(4*len(names)), 32)
	updated := l.operation(ins, core.OpAdd, []core.Operand{l.value(sp), l.value(count)}, 32, core.EffectNone)
	l.writeRegister(ins.address, "sp", 32, updated)
}

// branchOrCall lifts B and BL. A conditional branch carries the condition code
// in the low four bits of the opcode.
func (l *lifter) branchOrCall(ins instruction) {
	target, ok := branchTarget(ins)
	if !ok {
		l.unsupported(ins)
		return
	}
	if baseOp(ins.inst.Op) == armasm.BL {
		l.emit(core.OpCall, []core.Operand{pointerOperand(target)}, nil, nil, core.EffectCall|core.EffectMayTrap|core.EffectUnknown, ins.address)
		return
	}
	code := condition(ins.inst.Op)
	if code != "al" && code != "nv" {
		conditionValue := l.condition(ins, code)
		targets := []core.BlockID{}
		if block, ok := l.blockIDs[target]; ok {
			targets = append(targets, block)
		}
		next := ins.address + uint64(ins.size)
		if block, ok := l.blockIDs[next]; ok && !containsBlock(targets, block) {
			targets = append(targets, block)
		}
		effects := core.EffectNone
		if len(targets) == 0 {
			effects = core.EffectUnknown
		}
		l.emit(core.OpBranch, []core.Operand{l.value(conditionValue)}, nil, targets, effects, ins.address)
		return
	}
	if block, ok := l.blockIDs[target]; ok {
		l.emit(core.OpJump, nil, nil, []core.BlockID{block}, core.EffectNone, ins.address)
		return
	}
	l.emit(core.OpJump, []core.Operand{pointerOperand(target)}, nil, nil, core.EffectUnknown, ins.address)
}

func pointerOperand(value uint64) core.Operand {
	return core.Operand{
		Kind:    core.OperandConstant,
		Type:    core.ValueType{Kind: core.TypePointer, Bits: 32},
		Literal: literal(value, 32),
	}
}

// registerTransfer lifts BX and BLX.
func (l *lifter) registerTransfer(ins instruction) {
	args := args(ins)
	if len(args) != 1 {
		l.unsupported(ins)
		return
	}
	name, bits, ok := argRegister(args[0])
	if !ok {
		l.unsupported(ins)
		return
	}
	value := l.pointerFrom(ins.address, l.readRegister(ins.address, name, bits))
	if baseOp(ins.inst.Op) == armasm.BLX {
		l.emit(core.OpCall, []core.Operand{l.value(value)}, nil, nil, core.EffectCall|core.EffectMayTrap|core.EffectUnknown, ins.address)
		return
	}
	if name == "lr" {
		l.emit(core.OpReturn, nil, nil, nil, core.EffectMayTrap|core.EffectUnknown, ins.address)
		return
	}
	l.emit(core.OpJump, []core.Operand{l.value(value)}, nil, nil, core.EffectUnknown, ins.address)
}

// condition materializes one condition code by reading cpsr.
func (l *lifter) condition(ins instruction, code string) core.ValueID {
	cpsr := l.readRegister(ins.address, "cpsr", 32)
	return l.operation(ins, core.OpIntrinsic, []core.Operand{symbolOperand("arm.cond." + code), l.value(cpsr)}, 1, core.EffectReadRegister)
}
