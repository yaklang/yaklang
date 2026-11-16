package riscv

import (
	"golang.org/x/arch/riscv64/riscv64asm"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// arithmetic lifts the integer ALU instructions. The result goes to the first
// operand and the inputs follow.
func (l *lifter) arithmetic(ins instruction) (core.ValueID, bool) {
	arguments := args(ins)
	name := opName(ins.inst.Op)
	bits := l.profile.Bits

	switch name {
	case "lui":
		// LUI writes the immediate shifted left by twelve.
		if len(arguments) != 2 {
			return 0, false
		}
		destination, _, ok := registerName(argRegister(arguments[0]))
		if !ok {
			return 0, false
		}
		immediate, ok := upperImmediate(arguments[1])
		if !ok {
			return 0, false
		}
		value := l.constant(ins.address, uint64(immediate)<<12, bits)
		l.writeRegister(ins.address, destination, bits, value)
		return value, true
	case "auipc":
		// AUIPC adds the immediate, shifted left by twelve, to the address of
		// the instruction.
		if len(arguments) != 2 {
			return 0, false
		}
		destination, _, ok := registerName(argRegister(arguments[0]))
		if !ok {
			return 0, false
		}
		immediate, ok := upperImmediate(arguments[1])
		if !ok {
			return 0, false
		}
		value := l.pointerConstant(ins.address, ins.address+uint64(immediate)<<12)
		l.writeRegister(ins.address, destination, bits, value)
		return value, true
	case "nop":
		l.emit(core.OpNop, nil, nil, nil, core.EffectNone, ins.address)
		return 0, true
	case "mv", "li":
		if len(arguments) != 2 {
			return 0, false
		}
		destination, _, ok := registerName(argRegister(arguments[0]))
		if !ok {
			return 0, false
		}
		value, ok := l.readOperand(ins, arguments[1])
		if !ok {
			return 0, false
		}
		l.writeRegister(ins.address, destination, bits, value)
		return value, true
	case "seqz", "snez", "sltz", "sgtz", "neg", "not", "negw", "sext.w":
		return l.unaryPseudo(ins, arguments, name)
	}

	kind, ok := binaryOpcode(name)
	if !ok {
		return 0, false
	}
	if len(arguments) < 2 {
		return 0, false
	}
	destination, _, ok := registerName(argRegister(arguments[0]))
	if !ok {
		return 0, false
	}
	left, ok := l.readOperand(ins, arguments[1])
	if !ok {
		return 0, false
	}
	var right core.ValueID
	if len(arguments) >= 3 {
		right, ok = l.readOperand(ins, arguments[2])
		if !ok {
			return 0, false
		}
	} else {
		right = left
	}
	result := l.operation(ins, kind, []core.Operand{l.value(left), l.value(right)}, bits, core.EffectNone)
	l.writeRegister(ins.address, destination, bits, result)
	return result, true
}

// unaryPseudo lifts the pseudo instructions the assembler emits for single
// operand comparisons and sign handling.
func (l *lifter) unaryPseudo(ins instruction, arguments []riscv64asm.Arg, name string) (core.ValueID, bool) {
	if len(arguments) < 2 {
		return 0, false
	}
	destination, _, ok := registerName(argRegister(arguments[0]))
	if !ok {
		return 0, false
	}
	value, ok := l.readOperand(ins, arguments[1])
	if !ok {
		return 0, false
	}
	bits := l.profile.Bits
	var result core.ValueID
	switch name {
	case "seqz":
		zero := l.constant(ins.address, 0, bits)
		result = l.operation(ins, core.OpEqual, []core.Operand{l.value(value), l.value(zero)}, 1, core.EffectNone)
	case "snez":
		zero := l.constant(ins.address, 0, bits)
		condition := l.operation(ins, core.OpEqual, []core.Operand{l.value(value), l.value(zero)}, 1, core.EffectNone)
		result = l.operation(ins, core.OpNot, []core.Operand{l.value(condition)}, 1, core.EffectNone)
	case "sltz":
		zero := l.constant(ins.address, 0, bits)
		result = l.operation(ins, core.OpSLess, []core.Operand{l.value(value), l.value(zero)}, 1, core.EffectNone)
	case "sgtz":
		zero := l.constant(ins.address, 0, bits)
		result = l.operation(ins, core.OpSLess, []core.Operand{l.value(zero), l.value(value)}, 1, core.EffectNone)
	case "neg", "negw":
		result = l.operation(ins, core.OpNeg, []core.Operand{l.value(value)}, bits, core.EffectNone)
	case "not":
		result = l.operation(ins, core.OpNot, []core.Operand{l.value(value)}, bits, core.EffectNone)
	case "sext.w":
		result = l.operation(ins, core.OpSignExtend, []core.Operand{l.value(value)}, bits, core.EffectNone)
	default:
		return 0, false
	}
	l.writeRegister(ins.address, destination, bits, result)
	return result, true
}

func binaryOpcode(name string) (core.Opcode, bool) {
	switch trimmed(name) {
	case "add", "addi", "addw", "addiw", "add.uw":
		return core.OpAdd, true
	case "sub", "subw":
		return core.OpSub, true
	case "and", "andi":
		return core.OpAnd, true
	case "or", "ori":
		return core.OpOr, true
	case "xor", "xori":
		return core.OpXor, true
	case "sll", "slli", "sllw", "slliw":
		return core.OpShl, true
	case "srl", "srli", "srlw", "srliw":
		return core.OpLShr, true
	case "sra", "srai", "sraw", "sraiw":
		return core.OpAShr, true
	case "mul", "mulw", "mulh", "mulhu", "mulhsu":
		return core.OpMul, true
	case "div", "divw", "divu", "divuw":
		return core.OpSDiv, true
	case "rem", "remw", "remu", "remuw":
		return core.OpIntrinsic, true
	case "slt", "slti", "sltu", "sltiu":
		return core.OpSLess, true
	}
	return "", false
}

// readOperand reads one register or immediate operand as a value.
func (l *lifter) readOperand(ins instruction, arg riscv64asm.Arg) (core.ValueID, bool) {
	if reg, ok := arg.(riscv64asm.Reg); ok {
		name, _, ok := registerName(reg)
		if !ok {
			return 0, false
		}
		if isFloatRegister(name) {
			return 0, false
		}
		return l.readRegister(ins.address, name, l.profile.Bits), true
	}
	if value, ok := signedImmediate(arg); ok {
		return l.constant(ins.address, uint64(value), l.profile.Bits), true
	}
	return 0, false
}

// load lifts the scalar loads. The address is the base register plus the
// displacement, and the destination receives the loaded value.
func (l *lifter) load(ins instruction) {
	arguments := args(ins)
	name := trimmed(opName(ins.inst.Op))
	if name == "lr" || name == "lr.w" || name == "lr.d" {
		_ = name
		// LR.W / LR.D: the address is the only operand, the result is the value.
		if len(arguments) != 2 {
			l.unsupported(ins)
			return
		}
		destination, _, ok := registerName(argRegister(arguments[0]))
		if !ok {
			l.unsupported(ins)
			return
		}
		base, displacement, ok := memoryOperand(arguments[1])
		if !ok {
			l.unsupported(ins)
			return
		}
		pointer := l.addressFromBase(ins, base, displacement)
		value := l.loadValue(ins.address, pointer, accessWidth(ins.inst.Op, l.profile))
		l.writeRegister(ins.address, destination, l.profile.Bits, value)
		return
	}
	if len(arguments) != 2 {
		l.unsupported(ins)
		return
	}
	destination, _, ok := registerName(argRegister(arguments[0]))
	if !ok {
		l.unsupported(ins)
		return
	}
	base, displacement, ok := memoryOperand(arguments[1])
	if !ok {
		l.unsupported(ins)
		return
	}
	pointer := l.addressFromBase(ins, base, displacement)
	width := accessWidth(ins.inst.Op, l.profile)
	value := l.loadValue(ins.address, pointer, width)
	if isSignExtendingLoad(name) {
		value = l.operation(ins, core.OpSignExtend, []core.Operand{l.value(value)}, l.profile.Bits, core.EffectNone)
	} else if width < l.profile.Bits {
		value = l.operation(ins, core.OpZeroExtend, []core.Operand{l.value(value)}, l.profile.Bits, core.EffectNone)
	}
	l.writeRegister(ins.address, destination, l.profile.Bits, value)
}

// store lifts the scalar stores. SC.W and SC.D report their outcome in the first
// operand, so the value is the second one.
func (l *lifter) store(ins instruction) {
	arguments := args(ins)
	name := trimmed(opName(ins.inst.Op))
	if name == "sc" || name == "sc.w" || name == "sc.d" {
		if len(arguments) != 3 {
			l.unsupported(ins)
			return
		}
		status, _, ok := registerName(argRegister(arguments[0]))
		if !ok {
			l.unsupported(ins)
			return
		}
		value, ok := l.readOperand(ins, arguments[1])
		if !ok {
			l.unsupported(ins)
			return
		}
		base, displacement, ok := memoryOperand(arguments[2])
		if !ok {
			l.unsupported(ins)
			return
		}
		pointer := l.addressFromBase(ins, base, displacement)
		l.storeValue(ins.address, pointer, value)
		outcome := l.operation(ins, core.OpIntrinsic, []core.Operand{
			symbolOperand("riscv.exclusive.monitor"),
		}, l.profile.Bits, core.EffectAtomic|core.EffectUnknown)
		l.writeRegister(ins.address, status, l.profile.Bits, outcome)
		return
	}
	if len(arguments) != 2 {
		l.unsupported(ins)
		return
	}
	// Floating-point stores move a float register to memory. The value is read
	// as an opaque register, because the lane layout is not modeled.
	if reg, ok := arguments[0].(riscv64asm.Reg); ok {
		if name, _, ok := registerName(reg); ok && isFloatRegister(name) {
			base, displacement, ok := memoryOperand(arguments[1])
			if !ok {
				l.unsupported(ins)
				return
			}
			pointer := l.addressFromBase(ins, base, displacement)
			value := l.newValue(core.TypeFloat, floatWidth(ins.inst.Op, l.profile))
			l.emit(core.OpReadRegister, []core.Operand{core.Operand{
				Kind: core.OperandRegister,
				Type: value.Type,
				Name: name,
			}}, []core.Value{value}, nil, core.EffectReadRegister, ins.address)
			l.storeValue(ins.address, pointer, value.ID)
			return
		}
	}
	value, ok := l.readOperand(ins, arguments[0])
	if !ok {
		l.unsupported(ins)
		return
	}
	base, displacement, ok := memoryOperand(arguments[1])
	if !ok {
		l.unsupported(ins)
		return
	}
	pointer := l.addressFromBase(ins, base, displacement)
	l.storeValue(ins.address, pointer, value)
}

// accessWidth returns the memory access width of one load or store.
func accessWidth(op riscv64asm.Op, profile Profile) uint16 {
	name := trimmed(opName(op))
	switch {
	case len(name) >= 2 && name[len(name)-1] == 'b':
		return 8
	case len(name) >= 2 && name[len(name)-1] == 'h':
		return 16
	case len(name) >= 2 && name[len(name)-1] == 'w':
		return 32
	case len(name) >= 2 && name[len(name)-1] == 'd':
		return 64
	}
	return profile.Bits
}

// floatWidth returns the width of one floating-point memory access.
func floatWidth(op riscv64asm.Op, profile Profile) uint16 {
	name := opName(op)
	if len(name) > 0 && name[len(name)-1] == 's' {
		return 32
	}
	return 64
}

func isSignExtendingLoad(name string) bool {
	switch name {
	case "lb", "lh", "lw", "ld":
		return true
	}
	return false
}

// branch lifts one conditional branch. RISC-V compares two registers and jumps
// to the immediate offset.
func (l *lifter) branch(ins instruction) {
	target, ok := branchTarget(ins)
	if !ok {
		l.unsupported(ins)
		return
	}
	arguments := args(ins)
	name := trimmed(opName(ins.inst.Op))
	if len(arguments) < 2 {
		l.unsupported(ins)
		return
	}
	left, ok := l.readOperand(ins, arguments[0])
	if !ok {
		l.unsupported(ins)
		return
	}
	right, ok := l.readOperand(ins, arguments[1])
	if !ok {
		l.unsupported(ins)
		return
	}
	kind := core.OpEqual
	switch name {
	case "beq", "beqz":
		kind = core.OpEqual
	case "bne", "bnez":
		kind = core.OpEqual
	case "blt", "bltz":
		kind = core.OpSLess
	case "bge", "bgez":
		kind = core.OpSLess
	case "bltu":
		kind = core.OpULess
	case "bgeu":
		kind = core.OpULess
	}
	condition := l.operation(ins, kind, []core.Operand{l.value(left), l.value(right)}, 1, core.EffectNone)
	if name == "bne" || name == "bnez" || name == "bge" || name == "bgez" || name == "bgeu" {
		condition = l.operation(ins, core.OpNot, []core.Operand{l.value(condition)}, 1, core.EffectNone)
	}
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

// jumpOrCall lifts JAL, JALR and their pseudo forms. JAL with the zero
// destination register is an unconditional jump; otherwise it is a call.
func (l *lifter) jumpOrCall(ins instruction) {
	name := opName(ins.inst.Op)
	arguments := args(ins)
	switch name {
	case "jal":
		target, ok := branchTarget(ins)
		if !ok {
			l.unsupported(ins)
			return
		}
		if len(arguments) == 0 {
			l.unsupported(ins)
			return
		}
		destination := ""
		if len(arguments) > 0 {
			destination, _, _ = registerName(argRegister(arguments[0]))
		}
		if destination == "" || destination == "x0" {
			if block, ok := l.blockIDs[target]; ok {
				l.emit(core.OpJump, nil, nil, []core.BlockID{block}, core.EffectNone, ins.address)
				return
			}
			l.emit(core.OpJump, []core.Operand{pointerOperand(target, l.profile.Bits)}, nil, nil, core.EffectUnknown, ins.address)
			return
		}
		l.emit(core.OpCall, []core.Operand{pointerOperand(target, l.profile.Bits)}, nil, nil,
			core.EffectCall|core.EffectMayTrap|core.EffectUnknown, ins.address)
		l.writeRegister(ins.address, destination, l.profile.Bits, l.constant(ins.address, ins.address+uint64(ins.size), l.profile.Bits))
	case "jalr", "c.jalr":
		if len(arguments) == 0 {
			l.unsupported(ins)
			return
		}
		destination, _, _ := registerName(argRegister(arguments[0]))
		// JALR <rd>, <rs1>, <imm>: the base and the displacement are packed
		// into one RegOffset operand.
		base, displacement, ok := memoryOperand(arguments[1])
		if !ok {
			l.unsupported(ins)
			return
		}
		pointer := l.addressFromBase(ins, base, displacement)
		if destination == "" || destination == "x0" {
			if base == "x1" {
				l.emit(core.OpReturn, nil, nil, nil, core.EffectMayTrap|core.EffectUnknown, ins.address)
				return
			}
			l.emit(core.OpJump, []core.Operand{l.value(l.pointerFrom(ins.address, pointer))}, nil, nil, core.EffectUnknown, ins.address)
			return
		}
		l.emit(core.OpCall, []core.Operand{l.value(l.pointerFrom(ins.address, pointer))}, nil, nil,
			core.EffectCall|core.EffectMayTrap|core.EffectUnknown, ins.address)
		l.writeRegister(ins.address, destination, l.profile.Bits, l.constant(ins.address, ins.address+uint64(ins.size), l.profile.Bits))
	default:
		l.unsupported(ins)
	}
}

func pointerOperand(value uint64, bits uint16) core.Operand {
	return core.Operand{
		Kind:    core.OperandConstant,
		Type:    core.ValueType{Kind: core.TypePointer, Bits: bits},
		Literal: literal(value, bits),
	}
}

// system lifts the environment calls and fences. None of them carry a data
// result the decompiler models.
func (l *lifter) system(ins instruction) {
	name := opName(ins.inst.Op)
	effects := core.EffectUnknown
	switch name {
	case "ecall", "ebreak", "mret", "sret", "uret":
		effects |= core.EffectCall | core.EffectMayTrap
	case "fence", "fence.i", "fence.tso", "sfence.vma":
		effects |= core.EffectReadMemory | core.EffectWriteMemory
	}
	l.emit(core.OpIntrinsic, []core.Operand{symbolOperand("riscv." + name)}, nil, nil, effects, ins.address)
	if name == "ecall" || name == "ebreak" {
		l.emit(core.OpTrap, []core.Operand{symbolOperand("riscv." + name)}, nil, nil,
			core.EffectMayTrap|core.EffectUnknown, ins.address)
	}
}

// atomic lifts the AMO instructions. The destination receives the previous
// memory value; the source is folded into memory. The ordering is not modeled.
func (l *lifter) atomic(ins instruction) {
	arguments := args(ins)
	if len(arguments) != 3 {
		l.unsupported(ins)
		return
	}
	destination, _, ok := registerName(argRegister(arguments[0]))
	if !ok {
		l.unsupported(ins)
		return
	}
	source, ok := l.readOperand(ins, arguments[1])
	if !ok {
		l.unsupported(ins)
		return
	}
	base, displacement, ok := memoryOperand(arguments[2])
	if !ok {
		l.unsupported(ins)
		return
	}
	pointer := l.addressFromBase(ins, base, displacement)
	width := accessWidth(ins.inst.Op, l.profile)
	result := l.operation(ins, core.OpIntrinsic, []core.Operand{
		symbolOperand("riscv." + opName(ins.inst.Op)),
		l.value(l.pointerFrom(ins.address, pointer)),
		l.value(source),
	}, width, core.EffectReadMemory|core.EffectWriteMemory|core.EffectAtomic|core.EffectUnknown)
	l.writeRegister(ins.address, destination, l.profile.Bits, result)
}

// float lifts one floating-point instruction as an opaque vector operation: the
// destination register receives a value, the sources are read, and the lane
// semantics stay unmodeled.
func (l *lifter) float(ins instruction) {
	operands := []core.Operand{symbolOperand("riscv." + opName(ins.inst.Op))}
	var results []core.Value
	effects := core.EffectNone
	for index, arg := range ins.inst.Args {
		if arg == nil {
			break
		}
		if reg, ok := arg.(riscv64asm.Reg); ok {
			name, _, ok := registerName(reg)
			if !ok {
				continue
			}
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
			continue
		}
		if operand, ok := l.rawOperand(ins, index, arg); ok {
			operands = append(operands, operand)
		}
	}
	l.emit(core.OpIntrinsic, operands, results, nil, effects|core.EffectUnknown, ins.address)
}
