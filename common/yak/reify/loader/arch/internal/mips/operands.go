package mips

import (
	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// arithmetic lifts the integer ALU instructions. MIPS uses two-operand forms
// for the immediate variants and three-operand forms for the register variants.
func (l *lifter) arithmetic(ins instruction) (core.ValueID, bool) {
	// The named cases below cover forms that are not a plain binary operation,
	// so they are matched before the opcode table.
	bits := l.profile.Bits
	kind, _ := binaryOpcode(ins.op)
	switch ins.op {
	case "addi", "addiu", "daddi", "daddiu", "slti", "sltiu", "andi", "ori", "xori", "lui":
		if len(ins.operands) < 2 {
			return 0, false
		}
		destination := registerName(ins.operands[0].register)
		if ins.op == "lui" {
			value := l.constant(ins.address, uint64(ins.operands[1].immediat)<<16, bits)
			l.writeRegister(ins.address, destination, bits, value)
			return value, true
		}
		if len(ins.operands) != 3 {
			return 0, false
		}
		left := l.readRegister(ins.address, registerName(ins.operands[1].register), bits)
		right := l.constant(ins.address, uint64(ins.operands[2].immediat), bits)
		result := l.operation(ins, kind, []core.Operand{l.value(left), l.value(right)}, bits, core.EffectNone)
		l.writeRegister(ins.address, destination, bits, result)
		return result, true
	case "sll", "srl", "sra", "dsll", "dsrl", "dsra", "dsll32", "dsrl32", "dsra32":
		if len(ins.operands) != 3 {
			return 0, false
		}
		destination := registerName(ins.operands[0].register)
		source := l.readRegister(ins.address, registerName(ins.operands[1].register), bits)
		count := l.constant(ins.address, uint64(ins.operands[2].immediat), bits)
		result := l.operation(ins, kind, []core.Operand{l.value(source), l.value(count)}, bits, core.EffectNone)
		l.writeRegister(ins.address, destination, bits, result)
		return result, true
	case "sllv", "srlv", "srav", "dsllv", "dsrlv", "dsrav":
		if len(ins.operands) != 3 {
			return 0, false
		}
		destination := registerName(ins.operands[0].register)
		source := l.readRegister(ins.address, registerName(ins.operands[1].register), bits)
		count := l.readRegister(ins.address, registerName(ins.operands[2].register), bits)
		result := l.operation(ins, kind, []core.Operand{l.value(source), l.value(count)}, bits, core.EffectNone)
		l.writeRegister(ins.address, destination, bits, result)
		return result, true
	case "mfhi", "mflo":
		if len(ins.operands) != 1 {
			return 0, false
		}
		destination := registerName(ins.operands[0].register)
		register := "hi"
		if ins.op == "mflo" {
			register = "lo"
		}
		value := l.readRegister(ins.address, register, bits)
		l.writeRegister(ins.address, destination, bits, value)
		return value, true
	case "rdhwr":
		// RDHWR reads one hardware register: the thread pointer, the CPU
		// number, or the cycle counter. The hardware register stays a symbol
		// because its value is defined by the platform, not by the program.
		if len(ins.operands) != 2 {
			return 0, false
		}
		destination := registerName(ins.operands[0].register)
		value := l.operation(ins, core.OpIntrinsic, []core.Operand{
			symbolOperand("mips.rdhwr"),
			symbolOperand(registerName(ins.operands[1].register)),
		}, bits, core.EffectReadRegister|core.EffectUnknown)
		l.writeRegister(ins.address, destination, bits, value)
		return value, true
	case "mthi", "mtlo":
		if len(ins.operands) != 1 {
			return 0, false
		}
		value := l.readRegister(ins.address, registerName(ins.operands[0].register), bits)
		register := "hi"
		if ins.op == "mtlo" {
			register = "lo"
		}
		l.writeRegister(ins.address, register, bits, value)
		return value, true
	case "mult", "multu", "div", "divu", "dmult", "dmultu", "ddiv", "ddivu":
		if len(ins.operands) != 2 {
			return 0, false
		}
		left := l.readRegister(ins.address, registerName(ins.operands[0].register), bits)
		right := l.readRegister(ins.address, registerName(ins.operands[1].register), bits)
		low := l.operation(ins, kind, []core.Operand{l.value(left), l.value(right)}, bits, core.EffectNone)
		high := l.operation(ins, core.OpIntrinsic, []core.Operand{
			symbolOperand("mips." + ins.op + ".hi"), l.value(left), l.value(right),
		}, bits, core.EffectNone)
		l.writeRegister(ins.address, "lo", bits, low)
		l.writeRegister(ins.address, "hi", bits, high)
		return low, true
	case "movz", "movn", "movt", "movf":
		// A conditional move: the destination keeps its value when the
		// condition is false, so the operation stays visible as an intrinsic.
		if len(ins.operands) != 3 {
			return 0, false
		}
		destination := registerName(ins.operands[0].register)
		source := l.readRegister(ins.address, registerName(ins.operands[1].register), bits)
		condition := l.readRegister(ins.address, registerName(ins.operands[2].register), bits)
		result := l.operation(ins, core.OpIntrinsic, []core.Operand{
			symbolOperand("mips." + ins.op), l.value(source), l.value(condition),
		}, bits, core.EffectReadRegister)
		l.writeRegister(ins.address, destination, bits, result)
		return result, true
	case "clz", "clo", "dclz", "dclo":
		if len(ins.operands) != 2 {
			return 0, false
		}
		destination := registerName(ins.operands[0].register)
		source := l.readRegister(ins.address, registerName(ins.operands[1].register), bits)
		result := l.operation(ins, core.OpIntrinsic, []core.Operand{
			symbolOperand("mips." + ins.op), l.value(source),
		}, bits, core.EffectNone)
		l.writeRegister(ins.address, destination, bits, result)
		return result, true
	}

	kind, ok := binaryOpcode(ins.op)
	if !ok {
		return 0, false
	}
	if len(ins.operands) < 2 {
		return 0, false
	}
	destination := registerName(ins.operands[0].register)
	left := l.readRegister(ins.address, registerName(ins.operands[1].register), bits)
	var right core.ValueID
	if len(ins.operands) >= 3 {
		right = l.readRegister(ins.address, registerName(ins.operands[2].register), bits)
	} else {
		right = left
	}
	result := l.operation(ins, kind, []core.Operand{l.value(left), l.value(right)}, bits, core.EffectNone)
	l.writeRegister(ins.address, destination, bits, result)
	return result, true
}

func binaryOpcode(name string) (core.Opcode, bool) {
	switch name {
	case "add", "addu", "addi", "addiu", "dadd", "daddu", "daddi", "daddiu":
		return core.OpAdd, true
	case "sub", "subu", "dsub", "dsubu":
		return core.OpSub, true
	case "and", "andi":
		return core.OpAnd, true
	case "or", "ori":
		return core.OpOr, true
	case "xor", "xori", "nor":
		return core.OpXor, true
	case "sll", "sllv", "dsll", "dsllv", "dsll32":
		return core.OpShl, true
	case "srl", "srlv", "dsrl", "dsrlv", "dsrl32":
		return core.OpLShr, true
	case "sra", "srav", "dsra", "dsrav", "dsra32":
		return core.OpAShr, true
	case "slt", "slti", "sltu", "sltiu":
		return core.OpSLess, true
	case "mult", "multu", "mul", "madd", "maddu", "msub", "msubu", "dmult", "dmultu":
		return core.OpMul, true
	case "div", "divu", "ddiv", "ddivu":
		return core.OpSDiv, true
	}
	return "", false
}

// load lifts the integer and floating-point loads. MIPS keeps the destination,
// the displacement and the base register in that order.
func (l *lifter) load(ins instruction) {
	bits := l.profile.Bits
	if len(ins.operands) < 2 {
		l.unsupported(ins)
		return
	}
	destination := registerName(ins.operands[0].register)
	memory := ins.operands[1]
	if memory.kind != operandMemory {
		l.unsupported(ins)
		return
	}
	pointer := l.addressFromBase(ins, registerName(memory.base), memory.immediat)
	width := accessWidth(ins.op, l.profile)
	if isFloatOpcode(ins.op) {
		value := l.newValue(core.TypeFloat, width)
		l.emit(core.OpLoad, []core.Operand{l.value(pointer)}, []core.Value{value}, nil, core.EffectReadMemory, ins.address)
		l.emit(core.OpWriteRegister, []core.Operand{
			{Kind: core.OperandRegister, Type: value.Type, Name: destination}, l.value(value.ID),
		}, nil, nil, core.EffectWriteRegister, ins.address)
		return
	}
	value := l.loadValue(ins.address, pointer, width)
	if isSignExtendingLoad(ins.op) {
		value = l.operation(ins, core.OpSignExtend, []core.Operand{l.value(value)}, bits, core.EffectNone)
	} else if width < bits {
		value = l.operation(ins, core.OpZeroExtend, []core.Operand{l.value(value)}, bits, core.EffectNone)
	}
	l.writeRegister(ins.address, destination, bits, value)
}

// store lifts the integer and floating-point stores.
func (l *lifter) store(ins instruction) {
	if len(ins.operands) < 2 {
		l.unsupported(ins)
		return
	}
	source := registerName(ins.operands[0].register)
	memory := ins.operands[1]
	if memory.kind != operandMemory {
		l.unsupported(ins)
		return
	}
	pointer := l.addressFromBase(ins, registerName(memory.base), memory.immediat)
	if isFloatOpcode(ins.op) {
		value := l.newValue(core.TypeFloat, accessWidth(ins.op, l.profile))
		l.emit(core.OpReadRegister, []core.Operand{{
			Kind: core.OperandRegister, Type: value.Type, Name: source,
		}}, []core.Value{value}, nil, core.EffectReadRegister, ins.address)
		l.storeValue(ins.address, pointer, value.ID)
		return
	}
	value := l.readRegister(ins.address, source, l.profile.Bits)
	l.storeValue(ins.address, pointer, value)
}

// accessWidth returns the memory access width of one load or store.
func accessWidth(name string, profile Profile) uint16 {
	switch name {
	case "lb", "lbu", "sb":
		return 8
	case "lh", "lhu", "sh":
		return 16
	case "lw", "lwu", "sw", "ll", "sc", "lwc1", "swc1":
		return 32
	case "ld", "sd", "lld", "scd", "ldc1", "sdc1":
		return 64
	default:
		return profile.Bits
	}
}

func isSignExtendingLoad(name string) bool {
	switch name {
	case "lb", "lh", "lw", "ld", "ll", "lld":
		return true
	}
	return false
}

func isFloatOpcode(name string) bool {
	switch name {
	case "lwc1", "swc1", "ldc1", "sdc1", "mfc1", "mtc1", "cfc1", "ctc1":
		return true
	}
	return false
}

// call lifts JAL and JALR. The return address goes to the link register.
func (l *lifter) call(ins instruction) {
	name := ins.op
	if name == "jal" {
		target, _ := branchTarget(ins)
		l.emit(core.OpCall, []core.Operand{pointerOperand(target, l.profile.Bits)}, nil, nil,
			core.EffectCall|core.EffectMayTrap|core.EffectUnknown, ins.address)
		l.writeRegister(ins.address, "ra", l.profile.Bits, l.constant(ins.address, ins.address+uint64(ins.size), l.profile.Bits))
		return
	}
	// JALR <rd>, <rs>: the target is the register value.
	destination := "ra"
	target := ins.operands[0].register
	if len(ins.operands) >= 2 {
		destination = registerName(ins.operands[0].register)
		target = ins.operands[1].register
	}
	pointer := l.pointerFrom(ins.address, l.readRegister(ins.address, registerName(target), l.profile.Bits))
	l.emit(core.OpCall, []core.Operand{l.value(pointer)}, nil, nil,
		core.EffectCall|core.EffectMayTrap|core.EffectUnknown, ins.address)
	l.writeRegister(ins.address, destination, l.profile.Bits, l.constant(ins.address, ins.address+uint64(ins.size), l.profile.Bits))
}

// jump lifts J and JR.
func (l *lifter) jump(ins instruction) {
	if ins.op == "jr" {
		target := ins.operands[0].register
		if registerName(target) == "ra" {
			l.emit(core.OpReturn, nil, nil, nil, core.EffectMayTrap|core.EffectUnknown, ins.address)
			return
		}
		pointer := l.pointerFrom(ins.address, l.readRegister(ins.address, registerName(target), l.profile.Bits))
		l.emit(core.OpJump, []core.Operand{l.value(pointer)}, nil, nil, core.EffectUnknown, ins.address)
		return
	}
	target, ok := branchTarget(ins)
	if !ok {
		l.unsupported(ins)
		return
	}
	if block, ok := l.blockIDs[target]; ok {
		l.emit(core.OpJump, nil, nil, []core.BlockID{block}, core.EffectNone, ins.address)
		return
	}
	l.emit(core.OpJump, []core.Operand{pointerOperand(target, l.profile.Bits)}, nil, nil, core.EffectUnknown, ins.address)
}

// branch lifts the conditional branches. MIPS keeps the target as the last
// operand.
func (l *lifter) branch(ins instruction) {
	target, ok := branchTarget(ins)
	if !ok {
		l.unsupported(ins)
		return
	}
	// The COP1 branches test a floating-point condition and take only the
	// target; the condition flag itself lives in the FP control register.
	if len(ins.operands) == 1 && ins.operands[0].kind == operandImmediate {
		condition := l.operation(ins, core.OpIntrinsic, []core.Operand{
			symbolOperand("mips." + ins.op + ".condition"),
		}, 1, core.EffectReadRegister|core.EffectUnknown)
		l.emitBranch(ins, condition, target)
		return
	}
	if len(ins.operands) < 2 {
		l.unsupported(ins)
		return
	}
	bits := l.profile.Bits
	name := ins.op
	left := l.readRegister(ins.address, registerName(ins.operands[0].register), bits)
	var right core.ValueID
	if len(ins.operands) == 3 {
		right = l.readRegister(ins.address, registerName(ins.operands[1].register), bits)
	} else {
		right = l.constant(ins.address, 0, bits)
	}
	kind := core.OpEqual
	switch name {
	case "beq", "beql":
		kind = core.OpEqual
	case "bne", "bnel":
		kind = core.OpEqual
	case "blez", "blezl":
		kind = core.OpSLess
	case "bgtz", "bgtzl":
		kind = core.OpSLess
	case "bltz", "bltzl", "bltzal", "bltzall":
		kind = core.OpSLess
	case "bgez", "bgezl", "bgezal", "bgezall":
		kind = core.OpSLess
	default:
		kind = core.OpEqual
	}
	lhs, rhs := left, right
	if kind == core.OpEqual {
		if name == "bne" || name == "bnel" {
			condition := l.operation(ins, core.OpEqual, []core.Operand{l.value(lhs), l.value(rhs)}, 1, core.EffectNone)
			condition = l.operation(ins, core.OpNot, []core.Operand{l.value(condition)}, 1, core.EffectNone)
			l.emitBranch(ins, condition, target)
			return
		}
		condition := l.operation(ins, core.OpEqual, []core.Operand{l.value(lhs), l.value(rhs)}, 1, core.EffectNone)
		l.emitBranch(ins, condition, target)
		return
	}
	// The less-than forms compare against zero with the register on the
	// correct side.
	switch name {
	case "bgtz", "bgtzl":
		lhs, rhs = rhs, lhs
	case "bgez", "bgezl", "bgezal", "bgezall":
		lhs, rhs = rhs, lhs
	}
	condition := l.operation(ins, kind, []core.Operand{l.value(lhs), l.value(rhs)}, 1, core.EffectNone)
	if name == "bgez" || name == "bgezl" || name == "bgezal" || name == "bgezall" {
		condition = l.operation(ins, core.OpNot, []core.Operand{l.value(condition)}, 1, core.EffectNone)
	}
	l.emitBranch(ins, condition, target)
}

// emitBranch writes one branch with both successors.
func (l *lifter) emitBranch(ins instruction, condition core.ValueID, target uint64) {
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
	l.emit(core.OpBranch, []core.Operand{l.value(condition)}, nil, targets, effects, ins.address)
}

func pointerOperand(value uint64, bits uint16) core.Operand {
	return core.Operand{
		Kind:    core.OperandConstant,
		Type:    core.ValueType{Kind: core.TypePointer, Bits: bits},
		Literal: literal(value, bits),
	}
}

// system lifts synchronization, cache and trap instructions.
func (l *lifter) system(ins instruction) {
	effects := core.EffectUnknown
	switch ins.op {
	case "syscall", "break", "sdbbp", "teq", "tne", "tlt", "tltu", "tge", "tgeu":
		effects |= core.EffectMayTrap | core.EffectCall
	case "sync", "cache", "pref":
		effects |= core.EffectReadMemory | core.EffectWriteMemory
	}
	l.emit(core.OpIntrinsic, []core.Operand{symbolOperand("mips." + ins.op)}, nil, nil, effects, ins.address)
}

// float lifts one floating-point instruction as an opaque register operation.
func (l *lifter) float(ins instruction) {
	operands := []core.Operand{symbolOperand("mips." + ins.op)}
	var results []core.Value
	effects := core.EffectNone
	for index, item := range ins.operands {
		if item.kind != operandRegister {
			continue
		}
		name := registerName(item.register)
		bits := uint16(64)
		if l.profile.Bits == 32 {
			bits = 32
		}
		if index == 0 {
			value := l.newValue(core.TypeFloat, bits)
			results = append(results, value)
			operands = append(operands, core.Operand{Kind: core.OperandRegister, Type: value.Type, Name: name})
			effects |= core.EffectWriteRegister
			continue
		}
		operands = append(operands, core.Operand{
			Kind: core.OperandRegister,
			Type: core.ValueType{Kind: core.TypeFloat, Bits: bits},
			Name: name,
		})
		effects |= core.EffectReadRegister
	}
	l.emit(core.OpIntrinsic, operands, results, nil, effects|core.EffectUnknown, ins.address)
}
