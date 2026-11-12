package arm64

import (
	"golang.org/x/arch/arm64/arm64asm"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// --- data moves -------------------------------------------------------------

func (l *lifter) move(ins instruction) {
	args := args(ins)
	if len(args) != 2 {
		l.unsupported(ins)
		return
	}
	destinationName, destinationBits, ok := argRegister(args[0])
	if !ok {
		l.unsupported(ins)
		return
	}
	switch ins.inst.Op {
	case arm64asm.MOVZ, arm64asm.MOVN, arm64asm.MOVK:
		l.moveWide(ins, args[1], destinationName, destinationBits)
		return
	}
	value, ok := l.source(ins, args[1], destinationBits)
	if !ok {
		l.unsupported(ins)
		return
	}
	l.writeRegister(ins.address, destinationName, destinationBits, value)
}

// moveWide lifts MOVZ, MOVN and MOVK. The immediate occupies one 16-bit lane
// at the encoded shift, and MOVK keeps the other lanes of the destination.
func (l *lifter) moveWide(ins instruction, arg arm64asm.Arg, destinationName string, bits uint16) {
	immediate, ok := immediateValue(arg)
	if !ok {
		l.unsupported(ins)
		return
	}
	shift := uint8(0)
	if raw, amount, ok := immediateShift(arg); ok {
		immediate, shift = raw, amount
	}
	lane := uint64(0xffff) << shift
	value := (immediate << shift) & lane
	switch ins.inst.Op {
	case arm64asm.MOVN:
		value = ^value & (laneMask(bits))
	case arm64asm.MOVK:
		current := l.readRegister(ins.address, destinationName, bits)
		masked := l.operation(ins, core.OpAnd, []core.Operand{l.value(current), l.value(l.constant(ins.address, ^lane, bits))}, bits, core.EffectNone)
		inserted := l.operation(ins, core.OpOr, []core.Operand{l.value(masked), l.value(l.constant(ins.address, value, bits))}, bits, core.EffectNone)
		l.writeRegister(ins.address, destinationName, bits, inserted)
		return
	}
	l.writeRegister(ins.address, destinationName, bits, l.constant(ins.address, value, bits))
}

// laneMask returns the mask of a value wide enough for bits.
func laneMask(bits uint16) uint64 {
	if bits == 0 || bits >= 64 {
		return ^uint64(0)
	}
	return (uint64(1) << bits) - 1
}

// orrMove recognizes the ORR alias that moves a register with an optional LSL.
func (l *lifter) orrMove(ins instruction) bool {
	args := args(ins)
	if len(args) != 3 {
		return false
	}
	if _, ok := args[0].(arm64asm.RegSP); !ok {
		return false
	}
	if zero, ok := args[1].(arm64asm.RegSP); !ok || arm64asm.Reg(zero) != arm64asm.XZR && arm64asm.Reg(zero) != arm64asm.WZR {
		return false
	}
	l.move(ins)
	return true
}

func (l *lifter) extend(ins instruction) {
	args := args(ins)
	if len(args) != 2 {
		l.unsupported(ins)
		return
	}
	destinationName, destinationBits, ok := argRegister(args[0])
	if !ok {
		l.unsupported(ins)
		return
	}
	sourceName, sourceBits, ok := argRegister(args[1])
	if !ok {
		l.unsupported(ins)
		return
	}
	value := l.readRegister(ins.address, sourceName, sourceBits)
	kind := extSign32
	switch ins.inst.Op {
	case arm64asm.UXTB:
		kind = extZero8
	case arm64asm.UXTH:
		kind = extZero16
	case arm64asm.SXTB:
		kind = extSign8
	case arm64asm.SXTH:
		kind = extSign16
	}
	extended, ok := l.extendValue(ins, value, kind, 0, destinationBits)
	if !ok {
		l.unsupported(ins)
		return
	}
	l.writeRegister(ins.address, destinationName, destinationBits, extended)
}

// --- arithmetic -------------------------------------------------------------

func (l *lifter) binary(ins instruction, kind core.Opcode, flag string) {
	args := args(ins)
	if len(args) < 2 || len(args) > 4 {
		l.unsupported(ins)
		return
	}
	destinationName, bits, ok := argRegister(args[0])
	if !ok {
		l.unsupported(ins)
		return
	}
	left, ok := l.readArg(ins, args[1], bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	if len(args) == 2 {
		// Two-operand form: the destination is also the left operand.
		left = l.readRegister(ins.address, destinationName, bits)
		result := l.operation(ins, kind, []core.Operand{l.value(left), l.value(left)}, bits, core.EffectNone)
		l.writeRegister(ins.address, destinationName, bits, result)
		return
	}
	right, ok := l.readOperandArg(ins, args[2], bits, left)
	if !ok {
		l.unsupported(ins)
		return
	}
	if len(args) == 4 {
		// Shifted-register forms carry their shift in the fourth operand.
		if _, shift, ok := immediateShift(args[3]); ok && shift != 0 {
			count := l.constant(ins.address, uint64(shift), bits)
			right = l.operation(ins, core.OpShl, []core.Operand{l.value(right), l.value(count)}, bits, core.EffectNone)
		}
	}
	result := l.operation(ins, kind, []core.Operand{l.value(left), l.value(right)}, bits, core.EffectNone)
	l.writeRegister(ins.address, destinationName, bits, result)
	if flag != "" && (ins.inst.Op == arm64asm.ADDS || ins.inst.Op == arm64asm.SUBS || ins.inst.Op == arm64asm.ANDS) {
		l.setFlags(ins, flag, left, right, result)
	}
}

func (l *lifter) multiply(ins instruction) {
	args := args(ins)
	if len(args) != 3 && len(args) != 4 {
		l.unsupported(ins)
		return
	}
	destinationName, bits, ok := argRegister(args[0])
	if !ok {
		l.unsupported(ins)
		return
	}
	left, ok := l.readArg(ins, args[1], bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	right, ok := l.readArg(ins, args[2], bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	result := l.operation(ins, core.OpMul, []core.Operand{l.value(left), l.value(right)}, bits, core.EffectNone)
	if len(args) == 4 {
		addend, ok := l.readArg(ins, args[3], bits)
		if !ok {
			l.unsupported(ins)
			return
		}
		kind := core.OpAdd
		if ins.inst.Op == arm64asm.MSUB {
			kind = core.OpSub
		}
		result = l.operation(ins, kind, []core.Operand{l.value(result), l.value(addend)}, bits, core.EffectNone)
	}
	l.writeRegister(ins.address, destinationName, bits, result)
}

func (l *lifter) unary(ins instruction, kind core.Opcode, flag string) {
	args := args(ins)
	if len(args) != 2 {
		l.unsupported(ins)
		return
	}
	destinationName, bits, ok := argRegister(args[0])
	if !ok {
		l.unsupported(ins)
		return
	}
	value, ok := l.readArg(ins, args[1], bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	result := l.operation(ins, kind, []core.Operand{l.value(value)}, bits, core.EffectNone)
	l.writeRegister(ins.address, destinationName, bits, result)
	if flag != "" {
		l.setFlags(ins, flag, value)
	}
}

func (l *lifter) compare(ins instruction) {
	args := args(ins)
	if len(args) != 2 {
		l.unsupported(ins)
		return
	}
	bits := uint16(64)
	if _, width, ok := argRegister(args[0]); ok {
		bits = width
	}
	left, ok := l.readArg(ins, args[0], bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	right, ok := l.readOperandArg(ins, args[1], bits, left)
	if !ok {
		l.unsupported(ins)
		return
	}
	name := "cmp"
	switch ins.inst.Op {
	case arm64asm.CMN:
		name = "cmn"
	case arm64asm.TST:
		name = "tst"
	}
	l.setFlags(ins, name, left, right)
}

// --- control flow -----------------------------------------------------------

func (l *lifter) branchOrCall(ins instruction) {
	target, ok := directTarget(ins)
	if !ok {
		l.unsupported(ins)
		return
	}
	if ins.inst.Op == arm64asm.BL {
		l.emit(core.OpCall, []core.Operand{pointerOperand(target)}, nil, nil, core.EffectCall|core.EffectMayTrap|core.EffectUnknown, ins.address)
		return
	}
	if code, ok := conditionCode(ins.inst.Args[0]); ok && ins.inst.Op == arm64asm.B {
		condition := l.condition(ins, code)
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
		return
	}
	if block, ok := l.blockIDs[target]; ok {
		l.emit(core.OpJump, nil, nil, []core.BlockID{block}, core.EffectNone, ins.address)
		return
	}
	l.emit(core.OpJump, []core.Operand{pointerOperand(target)}, nil, nil, core.EffectUnknown, ins.address)
}

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
	if ins.inst.Op == arm64asm.BLR {
		l.emit(core.OpCall, []core.Operand{l.value(value)}, nil, nil, core.EffectCall|core.EffectMayTrap|core.EffectUnknown, ins.address)
		return
	}
	l.emit(core.OpJump, []core.Operand{l.value(value)}, nil, nil, core.EffectUnknown, ins.address)
}

func (l *lifter) conditionalBranch(ins instruction) {
	args := args(ins)
	if len(args) < 2 {
		l.unsupported(ins)
		return
	}
	target, ok := directTarget(ins)
	if !ok {
		l.unsupported(ins)
		return
	}
	var condition core.ValueID
	switch ins.inst.Op {
	case arm64asm.CBZ, arm64asm.CBNZ:
		name, bits, ok := argRegister(args[0])
		if !ok {
			l.unsupported(ins)
			return
		}
		value := l.readRegister(ins.address, name, bits)
		zero := l.constant(ins.address, 0, bits)
		condition = l.operation(ins, core.OpEqual, []core.Operand{l.value(value), l.value(zero)}, 1, core.EffectNone)
		if ins.inst.Op == arm64asm.CBNZ {
			condition = l.operation(ins, core.OpNot, []core.Operand{l.value(condition)}, 1, core.EffectNone)
		}
	default:
		value, ok := l.readArg(ins, args[0], 64)
		if !ok {
			l.unsupported(ins)
			return
		}
		bit, ok := immediateValue(args[1])
		if !ok {
			l.unsupported(ins)
			return
		}
		shift := l.constant(ins.address, bit, 64)
		shifted := l.operation(ins, core.OpLShr, []core.Operand{l.value(value), l.value(shift)}, 64, core.EffectNone)
		mask := l.constant(ins.address, 1, 64)
		and := l.operation(ins, core.OpAnd, []core.Operand{l.value(shifted), l.value(mask)}, 64, core.EffectNone)
		zero := l.constant(ins.address, 0, 64)
		condition = l.operation(ins, core.OpEqual, []core.Operand{l.value(and), l.value(zero)}, 1, core.EffectNone)
		if ins.inst.Op == arm64asm.TBNZ {
			condition = l.operation(ins, core.OpNot, []core.Operand{l.value(condition)}, 1, core.EffectNone)
		}
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

func (l *lifter) condition(ins instruction, code string) core.ValueID {
	if code == "al" || code == "nv" {
		// Always-taken: fold the condition to a non-zero constant.
		return l.constant(ins.address, 1, 1)
	}
	nzcv := l.readRegister(ins.address, "nzcv", 4)
	return l.operation(ins, core.OpIntrinsic, []core.Operand{symbolOperand("arm64.cond." + code), l.value(nzcv)}, 1, core.EffectReadRegister)
}

func (l *lifter) setFlags(ins instruction, name string, values ...core.ValueID) {
	operands := make([]core.Operand, 0, len(values)+1)
	operands = append(operands, symbolOperand("arm64.flags."+name))
	for _, value := range values {
		operands = append(operands, l.value(value))
	}
	flags := l.operation(ins, core.OpIntrinsic, operands, 4, core.EffectWriteRegister)
	l.writeRegister(ins.address, "nzcv", 4, flags)
}

// --- memory -----------------------------------------------------------------

func (l *lifter) load(ins instruction) {
	args := args(ins)
	if len(args) < 2 {
		l.unsupported(ins)
		return
	}
	bits := l.destinationWidth(ins, args[0])
	if ins.inst.Op == arm64asm.LDPSW {
		// LDPSW loads two sign-extended words into 64-bit registers.
		if len(args) != 3 {
			l.unsupported(ins)
			return
		}
		firstName, _, ok := argRegister(args[0])
		if !ok {
			l.unsupported(ins)
			return
		}
		secondName, _, ok := argRegister(args[1])
		if !ok {
			l.unsupported(ins)
			return
		}
		l.loadPairSigned(ins, args[2], firstName, secondName)
		return
	}
	if ins.inst.Op == arm64asm.LDP {
		if len(args) != 3 {
			l.unsupported(ins)
			return
		}
		firstName, firstBits, ok := argRegister(args[0])
		if !ok {
			l.unsupported(ins)
			return
		}
		secondName, secondBits, ok := argRegister(args[1])
		if !ok {
			l.unsupported(ins)
			return
		}
		l.loadPair(ins, args[2], firstName, firstBits, secondName, secondBits)
		return
	}
	name, bitsFromRegister, ok := argRegister(args[0])
	if !ok {
		l.unsupported(ins)
		return
	}
	bits = bitsFromRegister
	switch ins.inst.Op {
	case arm64asm.LDRSW:
		bits = 32
	case arm64asm.LDRSB, arm64asm.LDRSH, arm64asm.LDURSB, arm64asm.LDURSH:
		// The load reads one byte or halfword; the sign extension afterwards
		// widens it to the destination register.
		bits = loadWidth(ins.inst.Op)
	}
	pointer := l.memoryOperand(ins, args[1])
	if pointer == 0 {
		// A literal-pool load addresses memory through the program counter:
		// LDR <Wt|Xt>, <label>. The target is an absolute image address.
		if offset, ok := pcRelative(args[1]); ok {
			pointer = l.pointerConstant(ins.address, ins.address+uint64(offset))
		}
	}
	if pointer == 0 {
		l.unsupported(ins)
		return
	}
	value := l.loadValue(ins.address, pointer, bits)
	switch ins.inst.Op {
	case arm64asm.LDRSW, arm64asm.LDRSB, arm64asm.LDRSH, arm64asm.LDURSB, arm64asm.LDURSH:
		value = l.operation(ins, core.OpSignExtend, []core.Operand{l.value(value)}, bitsFromRegister, core.EffectNone)
	}
	l.writeRegister(ins.address, name, bitsFromRegister, value)
	l.postUpdate(ins, args[1])
}

// loadWidth returns the memory access width of a sign-extending load.
func loadWidth(op arm64asm.Op) uint16 {
	switch op {
	case arm64asm.LDRSB, arm64asm.LDURSB:
		return 8
	case arm64asm.LDRSH, arm64asm.LDURSH:
		return 16
	case arm64asm.LDRSW:
		return 32
	default:
		return 0
	}
}

func (l *lifter) loadPair(ins instruction, operand arm64asm.Arg, firstName string, firstBits uint16, secondName string, secondBits uint16) {
	base, displacement, mode, ok := memoryAddress(operand)
	if !ok {
		l.unsupported(ins)
		return
	}
	pointer := l.addressFromBase(ins, base, displacement)
	first := l.loadValue(ins.address, pointer, firstBits)
	l.writeRegister(ins.address, firstName, firstBits, first)
	if mode == arm64asm.AddrPostIndex {
		pointer = l.addressFromBase(ins, base, 0)
	}
	offset := l.constant(ins.address, uint64(firstBits/8), 64)
	secondPointer := l.addAddress(ins, pointer, offset)
	second := l.loadValue(ins.address, secondPointer, secondBits)
	l.writeRegister(ins.address, secondName, secondBits, second)
	l.postUpdate(ins, operand)
}

func (l *lifter) store(ins instruction) {
	args := args(ins)
	if len(args) < 2 {
		l.unsupported(ins)
		return
	}
	bits := l.sourceWidth(ins, args[0])
	if ins.inst.Op == arm64asm.STP {
		if len(args) != 3 {
			l.unsupported(ins)
			return
		}
		l.storePair(ins, args[2], args[0], args[1])
		return
	}
	value, ok := l.readArg(ins, args[0], bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	if ins.inst.Op == arm64asm.STRB || ins.inst.Op == arm64asm.STURB {
		value = l.operation(ins, core.OpTruncate, []core.Operand{l.value(value)}, 8, core.EffectNone)
	}
	if ins.inst.Op == arm64asm.STRH || ins.inst.Op == arm64asm.STURH {
		value = l.operation(ins, core.OpTruncate, []core.Operand{l.value(value)}, 16, core.EffectNone)
	}
	pointer := l.memoryOperand(ins, args[1])
	if pointer == 0 {
		l.unsupported(ins)
		return
	}
	l.storeValue(ins.address, pointer, value)
	l.postUpdate(ins, args[1])
}

func (l *lifter) storePair(ins instruction, operand arm64asm.Arg, first, second arm64asm.Arg) {
	base, displacement, mode, ok := memoryAddress(operand)
	if !ok {
		l.unsupported(ins)
		return
	}
	bits := l.sourceWidth(ins, first)
	firstValue, ok := l.readArg(ins, first, bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	secondValue, ok := l.readArg(ins, second, bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	pointer := l.addressFromBase(ins, base, displacement)
	l.storeValue(ins.address, pointer, firstValue)
	if mode == arm64asm.AddrPostIndex {
		pointer = l.addressFromBase(ins, base, 0)
	}
	offset := l.constant(ins.address, uint64(bits/8), 64)
	secondPointer := l.addAddress(ins, pointer, offset)
	l.storeValue(ins.address, secondPointer, secondValue)
	l.postUpdate(ins, operand)
}

// postUpdate writes the adjusted base register for pre- and post-indexed
// addressing. The offset form leaves the base register unchanged.
func (l *lifter) postUpdate(ins instruction, operand arm64asm.Arg) {
	base, displacement, mode, ok := memoryAddress(operand)
	if !ok || mode == arm64asm.AddrOffset {
		return
	}
	bits := uint16(64)
	if name, width, ok := registerNameText(base); ok {
		_, bits = name, width
	}
	if displacement == 0 {
		return
	}
	current := l.readRegister(ins.address, base, bits)
	kind := core.OpAdd
	amount := uint64(displacement)
	if displacement < 0 {
		kind = core.OpSub
		amount = uint64(-displacement)
	}
	offset := l.constant(ins.address, amount, bits)
	value := l.operation(ins, kind, []core.Operand{l.value(current), l.value(offset)}, bits, core.EffectNone)
	l.writeRegister(ins.address, base, bits, value)
}

func (l *lifter) memoryOperand(ins instruction, operand arm64asm.Arg) core.ValueID {
	base, displacement, mode, ok := memoryAddress(operand)
	if !ok {
		return 0
	}
	if displacement != 0 && mode == arm64asm.AddrPostIndex {
		displacement = 0
	}
	return l.addressFromBase(ins, base, displacement)
}

func (l *lifter) addressFromBase(ins instruction, base string, displacement int64) core.ValueID {
	bits := uint16(64)
	if _, width, ok := registerNameText(base); ok {
		bits = width
	}
	address := l.pointerFrom(ins.address, l.readRegister(ins.address, base, bits))
	if displacement == 0 {
		return address
	}
	kind := core.OpAdd
	amount := uint64(displacement)
	if displacement < 0 {
		kind = core.OpSub
		amount = uint64(-displacement)
	}
	offset := l.constant(ins.address, amount, 64)
	return l.operation(ins, kind, []core.Operand{l.value(address), l.value(offset)}, 64, core.EffectNone)
}

func (l *lifter) addAddress(ins instruction, left, right core.ValueID) core.ValueID {
	result := l.newValue(core.TypePointer, 64)
	l.emit(core.OpAdd, []core.Operand{l.value(left), l.value(right)}, []core.Value{result}, nil, core.EffectNone, ins.address)
	return result.ID
}

// address materializes an ADR or ADRP target.
func (l *lifter) address(ins instruction) {
	args := args(ins)
	if len(args) != 2 {
		l.unsupported(ins)
		return
	}
	name, bits, ok := argRegister(args[0])
	if !ok {
		l.unsupported(ins)
		return
	}
	offset, ok := pcRelative(args[1])
	if !ok {
		l.unsupported(ins)
		return
	}
	target := ins.address + uint64(offset)
	if ins.inst.Op == arm64asm.ADRP {
		target = (ins.address &^ 0xfff) + uint64(offset)
	}
	value := l.pointerConstant(ins.address, target)
	l.writeRegister(ins.address, name, bits, value)
}

// --- operand helpers --------------------------------------------------------

func args(ins instruction) []arm64asm.Arg {
	result := make([]arm64asm.Arg, 0, 5)
	for _, arg := range ins.inst.Args {
		if arg == nil {
			break
		}
		result = append(result, arg)
	}
	return result
}

func (l *lifter) source(ins instruction, arg arm64asm.Arg, bits uint16) (core.ValueID, bool) {
	if name, width, ok := argRegister(arg); ok {
		value := l.readRegister(ins.address, name, width)
		if bits != 0 && width != bits {
			value = l.coerce(ins, value, width, bits, true)
		}
		return value, true
	}
	if value, ok := immediateValue(arg); ok {
		return l.constant(ins.address, value, bits), true
	}
	if base, displacement, mode, ok := memoryAddress(arg); ok {
		pointer := l.addressFromBase(ins, base, displacement)
		if mode == arm64asm.AddrPostIndex {
			l.postUpdate(ins, arg)
		}
		return l.loadValue(ins.address, pointer, bits), true
	}
	if offset, ok := pcRelative(arg); ok {
		return l.pointerConstant(ins.address, ins.address+uint64(offset)), true
	}
	return 0, false
}

func (l *lifter) readArg(ins instruction, arg arm64asm.Arg, bits uint16) (core.ValueID, bool) {
	return l.source(ins, arg, bits)
}

// readOperandArg reads one operand that may carry a shift or an extension.
func (l *lifter) readOperandArg(ins instruction, arg arm64asm.Arg, bits uint16, current core.ValueID) (core.ValueID, bool) {
	value, ok := l.source(ins, arg, bits)
	if !ok {
		return 0, false
	}
	kind, shift, ok := extensionOf(arg)
	if !ok {
		return 0, false
	}
	if kind == extCopy && shift == 0 {
		return value, true
	}
	return l.extendValue(ins, value, kind, shift, bits)
}

func (l *lifter) destinationWidth(ins instruction, arg arm64asm.Arg) uint16 {
	if _, bits, ok := argRegister(arg); ok {
		return bits
	}
	return 64
}

func (l *lifter) sourceWidth(ins instruction, arg arm64asm.Arg) uint16 {
	if _, bits, ok := argRegister(arg); ok {
		return bits
	}
	if value, ok := immediateValue(arg); ok {
		_ = value
		return 64
	}
	return 64
}

func containsBlock(blocks []core.BlockID, target core.BlockID) bool {
	for _, block := range blocks {
		if block == target {
			return true
		}
	}
	return false
}

// exclusiveStore lifts STXR and STLXR. The status register receives the outcome
// of the exclusive monitor; the monitor itself is not modeled, so the store is
// emitted with an explicit status write.
func (l *lifter) exclusiveStore(ins instruction) {
	arguments := args(ins)
	if len(arguments) != 3 {
		l.unsupported(ins)
		return
	}
	statusName, statusBits, ok := argRegister(arguments[0])
	if !ok {
		l.unsupported(ins)
		return
	}
	valueBits := uint16(32)
	if _, bits, ok := argRegister(arguments[1]); ok {
		valueBits = bits
	}
	value, ok := l.source(ins, arguments[1], valueBits)
	if !ok {
		l.unsupported(ins)
		return
	}
	pointer := l.memoryOperand(ins, arguments[2])
	if pointer == 0 {
		l.unsupported(ins)
		return
	}
	l.storeValue(ins.address, pointer, value)
	// The monitor succeeds unless another core intervened, which is not
	// visible statically; the outcome is left as an opaque zero-or-one value.
	outcome := l.operation(ins, core.OpIntrinsic, []core.Operand{
		symbolOperand("arm64.exclusive.monitor"),
	}, statusBits, core.EffectAtomic|core.EffectUnknown)
	l.writeRegister(ins.address, statusName, statusBits, outcome)
	l.postUpdate(ins, arguments[2])
}

// loadPairSigned lifts LDPSW: two 32-bit loads, each sign-extended to 64 bits.
func (l *lifter) loadPairSigned(ins instruction, operand arm64asm.Arg, firstName, secondName string) {
	base, displacement, mode, ok := memoryAddress(operand)
	if !ok {
		l.unsupported(ins)
		return
	}
	pointer := l.addressFromBase(ins, base, displacement)
	first := l.loadValue(ins.address, pointer, 32)
	first = l.operation(ins, core.OpSignExtend, []core.Operand{l.value(first)}, 64, core.EffectNone)
	l.writeRegister(ins.address, firstName, 64, first)
	if mode == arm64asm.AddrPostIndex {
		pointer = l.addressFromBase(ins, base, 0)
	}
	offset := l.constant(ins.address, 4, 64)
	second := l.loadValue(ins.address, l.addAddress(ins, pointer, offset), 32)
	second = l.operation(ins, core.OpSignExtend, []core.Operand{l.value(second)}, 64, core.EffectNone)
	l.writeRegister(ins.address, secondName, 64, second)
	l.postUpdate(ins, operand)
}
