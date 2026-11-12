package arm

import (
	"golang.org/x/arch/arm/armasm"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

func args(ins instruction) []armasm.Arg {
	result := make([]armasm.Arg, 0, 4)
	for _, arg := range ins.inst.Args {
		if arg == nil {
			break
		}
		result = append(result, arg)
	}
	return result
}

// source reads one operand as a value. Shifted registers and immediates are
// materialized, and a memory operand becomes a load.
func (l *lifter) source(ins instruction, arg armasm.Arg, bits uint16) (core.ValueID, bool) {
	if value, ok := immediateValue(arg); ok {
		return l.constant(ins.address, value, bits), true
	}
	if name, shift, count, ok := shiftedRegister(arg); ok {
		value := l.readRegister(ins.address, name, 32)
		return l.applyShift(ins, value, shift, count), true
	}
	if offset, ok := pcRelative(arg); ok {
		return l.pointerConstant(ins.address, ins.address+8+uint64(offset)), true
	}
	if base, index, shift, displacement, memory, ok := memoryAddress(arg); ok {
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
		return l.loadValue(ins.address, pointer, bits), true
	}
	return 0, false
}

// applyShift materializes one shift by an immediate amount.
func (l *lifter) applyShift(ins instruction, value core.ValueID, shift string, count uint64) core.ValueID {
	switch shift {
	case "":
		return value
	case "rrx":
		// Rotate right through carry with an implicit count of one.
		count = 1
	}
	amount := l.constant(ins.address, count, 32)
	kind := core.OpShl
	switch shift {
	case "lsr":
		kind = core.OpLShr
	case "asr":
		kind = core.OpAShr
	case "ror":
		kind = core.OpIntrinsic
	}
	if kind == core.OpIntrinsic {
		return l.operation(ins, core.OpIntrinsic, []core.Operand{symbolOperand("arm.ror"), l.value(value), l.value(amount)}, 32, core.EffectNone)
	}
	return l.operation(ins, kind, []core.Operand{l.value(value), l.value(amount)}, 32, core.EffectNone)
}

func (l *lifter) binary(ins instruction) {
	args := args(ins)
	if len(args) < 2 || len(args) > 3 {
		l.unsupported(ins)
		return
	}
	destination, _, ok := argRegister(args[0])
	if !ok {
		l.unsupported(ins)
		return
	}
	left, ok := l.source(ins, args[1], 32)
	if !ok {
		l.unsupported(ins)
		return
	}
	var right core.ValueID
	if len(args) == 3 {
		right, ok = l.source(ins, args[2], 32)
		if !ok {
			l.unsupported(ins)
			return
		}
	} else {
		// Two-operand form: the destination is also the first input.
		right = left
		left = l.readRegister(ins.address, destination, 32)
	}
	kind := binaryOpcode(baseOp(ins.inst.Op))
	result := l.operation(ins, kind, []core.Operand{l.value(left), l.value(right)}, 32, core.EffectNone)
	l.writeRegister(ins.address, destination, 32, result)
	if setsFlags(ins) {
		l.setFlags(ins, "op", left, right, result)
	}
}

func binaryOpcode(op armasm.Op) core.Opcode {
	switch op {
	case armasm.ADD:
		return core.OpAdd
	case armasm.ADC:
		return core.OpAdd
	case armasm.SUB, armasm.RSB:
		return core.OpSub
	case armasm.SBC, armasm.RSC:
		return core.OpSub
	case armasm.AND, armasm.BIC:
		return core.OpAnd
	case armasm.ORR:
		return core.OpOr
	case armasm.EOR:
		return core.OpXor
	case armasm.SDIV:
		return core.OpSDiv
	case armasm.UDIV:
		return core.OpUDiv
	default:
		return core.OpAdd
	}
}

// setsFlags reports whether the opcode has the S bit set. armasm appends no
// suffix, so the encoded bit is the only reliable source.
func setsFlags(ins instruction) bool {
	return ins.inst.Enc&(1<<20) != 0
}

func (l *lifter) move(ins instruction) {
	args := args(ins)
	if len(args) != 2 {
		l.unsupported(ins)
		return
	}
	destination, _, ok := argRegister(args[0])
	if !ok {
		l.unsupported(ins)
		return
	}
	value, ok := l.source(ins, args[1], 32)
	if !ok {
		l.unsupported(ins)
		return
	}
	if baseOp(ins.inst.Op) == armasm.MVN {
		value = l.operation(ins, core.OpNot, []core.Operand{l.value(value)}, 32, core.EffectNone)
	}
	l.writeRegister(ins.address, destination, 32, value)
	if setsFlags(ins) {
		l.setFlags(ins, "op", value)
	}
}

// moveWide lifts MOVW and MOVT. MOVW fills the low halfword; MOVT keeps the low
// halfword and replaces the high one.
func (l *lifter) moveWide(ins instruction) {
	args := args(ins)
	if len(args) != 2 {
		l.unsupported(ins)
		return
	}
	destination, _, ok := argRegister(args[0])
	if !ok {
		l.unsupported(ins)
		return
	}
	immediate, ok := immediateValue(args[1])
	if !ok {
		l.unsupported(ins)
		return
	}
	if baseOp(ins.inst.Op) == armasm.MOVW {
		l.writeRegister(ins.address, destination, 32, l.constant(ins.address, immediate&0xffff, 32))
		return
	}
	current := l.readRegister(ins.address, destination, 32)
	mask := l.constant(ins.address, 0xffff, 32)
	low := l.operation(ins, core.OpAnd, []core.Operand{l.value(current), l.value(mask)}, 32, core.EffectNone)
	high := l.constant(ins.address, immediate<<16, 32)
	merged := l.operation(ins, core.OpOr, []core.Operand{l.value(low), l.value(high)}, 32, core.EffectNone)
	l.writeRegister(ins.address, destination, 32, merged)
}

func (l *lifter) multiply(ins instruction) {
	args := args(ins)
	if len(args) != 3 && len(args) != 4 {
		l.unsupported(ins)
		return
	}
	destination, _, ok := argRegister(args[0])
	if !ok {
		l.unsupported(ins)
		return
	}
	right, ok := l.source(ins, args[1], 32)
	if !ok {
		l.unsupported(ins)
		return
	}
	left, ok := l.source(ins, args[2], 32)
	if !ok {
		l.unsupported(ins)
		return
	}
	result := l.operation(ins, core.OpMul, []core.Operand{l.value(left), l.value(right)}, 32, core.EffectNone)
	if len(args) == 4 {
		addend, ok := l.source(ins, args[3], 32)
		if !ok {
			l.unsupported(ins)
			return
		}
		kind := core.OpAdd
		if baseOp(ins.inst.Op) == armasm.MLS {
			kind = core.OpSub
		}
		result = l.operation(ins, kind, []core.Operand{l.value(result), l.value(addend)}, 32, core.EffectNone)
	}
	l.writeRegister(ins.address, destination, 32, result)
	if setsFlags(ins) {
		l.setFlags(ins, "op", result)
	}
}

func (l *lifter) compare(ins instruction) {
	args := args(ins)
	if len(args) != 2 {
		l.unsupported(ins)
		return
	}
	left, ok := l.source(ins, args[0], 32)
	if !ok {
		l.unsupported(ins)
		return
	}
	right, ok := l.source(ins, args[1], 32)
	if !ok {
		l.unsupported(ins)
		return
	}
	name := "cmp"
	switch baseOp(ins.inst.Op) {
	case armasm.CMN:
		name = "cmn"
	case armasm.TST:
		name = "tst"
	case armasm.TEQ:
		name = "teq"
	}
	l.setFlags(ins, name, left, right)
}

func (l *lifter) shift(ins instruction) {
	args := args(ins)
	if len(args) != 3 {
		l.unsupported(ins)
		return
	}
	destination, _, ok := argRegister(args[0])
	if !ok {
		l.unsupported(ins)
		return
	}
	value, ok := l.source(ins, args[1], 32)
	if !ok {
		l.unsupported(ins)
		return
	}
	count, ok := l.source(ins, args[2], 32)
	if !ok {
		l.unsupported(ins)
		return
	}
	kind := core.OpShl
	switch baseOp(ins.inst.Op) {
	case armasm.LSR:
		kind = core.OpLShr
	case armasm.ASR:
		kind = core.OpAShr
	case armasm.ROR, armasm.RRX:
		result := l.operation(ins, core.OpIntrinsic, []core.Operand{symbolOperand("arm.ror"), l.value(value), l.value(count)}, 32, core.EffectNone)
		l.writeRegister(ins.address, destination, 32, result)
		return
	}
	result := l.operation(ins, kind, []core.Operand{l.value(value), l.value(count)}, 32, core.EffectNone)
	l.writeRegister(ins.address, destination, 32, result)
}

// intrinsicValue lifts an operation whose exact data layout is not modeled but
// whose result is a plain value, for example CLZ or REV.
func (l *lifter) intrinsicValue(ins instruction, name string) {
	args := args(ins)
	if len(args) != 2 {
		l.unsupported(ins)
		return
	}
	destination, _, ok := argRegister(args[0])
	if !ok {
		l.unsupported(ins)
		return
	}
	value, ok := l.source(ins, args[1], 32)
	if !ok {
		l.unsupported(ins)
		return
	}
	result := l.operation(ins, core.OpIntrinsic, []core.Operand{symbolOperand(name), l.value(value)}, 32, core.EffectNone)
	l.writeRegister(ins.address, destination, 32, result)
}

func (l *lifter) setFlags(ins instruction, name string, values ...core.ValueID) {
	operands := make([]core.Operand, 0, len(values)+1)
	operands = append(operands, symbolOperand("arm.flags."+name))
	for _, value := range values {
		operands = append(operands, l.value(value))
	}
	flags := l.operation(ins, core.OpIntrinsic, operands, 32, core.EffectWriteRegister)
	l.writeRegister(ins.address, "cpsr", 32, flags)
}

// extend lifts UXTB, UXTH, SXTB and SXTH.
func (l *lifter) extend(ins instruction) {
	args := args(ins)
	if len(args) != 2 {
		l.unsupported(ins)
		return
	}
	destination, _, ok := argRegister(args[0])
	if !ok {
		l.unsupported(ins)
		return
	}
	value, ok := l.source(ins, args[1], 32)
	if !ok {
		l.unsupported(ins)
		return
	}
	kind := core.OpZeroExtend
	switch baseOp(ins.inst.Op) {
	case armasm.SXTB, armasm.SXTH:
		kind = core.OpSignExtend
	}
	result := l.operation(ins, kind, []core.Operand{l.value(value)}, 32, core.EffectNone)
	l.writeRegister(ins.address, destination, 32, result)
}

// longMultiply lifts UMULL, UMLAL, SMULL and SMLAL. The result is a 64-bit value
// split across RdLo and RdHi; the low half is the low 32 bits and the high half
// is the next 32 bits.
func (l *lifter) longMultiply(ins instruction) {
	args := args(ins)
	if len(args) < 3 || len(args) > 4 {
		l.unsupported(ins)
		return
	}
	lowName, _, ok := argRegister(args[0])
	if !ok {
		l.unsupported(ins)
		return
	}
	highName, _, ok := argRegister(args[1])
	if !ok {
		l.unsupported(ins)
		return
	}
	left, ok := l.source(ins, args[2], 32)
	if !ok {
		l.unsupported(ins)
		return
	}
	right, ok := l.source(ins, args[3], 32)
	if !ok {
		l.unsupported(ins)
		return
	}
	product := l.operation(ins, core.OpMul, []core.Operand{l.value(left), l.value(right)}, 64, core.EffectNone)
	if baseOp(ins.inst.Op) == armasm.UMLAL || baseOp(ins.inst.Op) == armasm.SMLAL {
		low := l.readRegister(ins.address, lowName, 32)
		high := l.readRegister(ins.address, highName, 32)
		highWide := l.operation(ins, core.OpShl, []core.Operand{l.value(high), l.value(l.constant(ins.address, 32, 64))}, 64, core.EffectNone)
		lowWide := l.operation(ins, core.OpZeroExtend, []core.Operand{l.value(low)}, 64, core.EffectNone)
		accumulator := l.operation(ins, core.OpOr, []core.Operand{l.value(highWide), l.value(lowWide)}, 64, core.EffectNone)
		product = l.operation(ins, core.OpAdd, []core.Operand{l.value(product), l.value(accumulator)}, 64, core.EffectNone)
	}
	l.writeRegister(ins.address, lowName, 32, l.operation(ins, core.OpTruncate, []core.Operand{l.value(product)}, 32, core.EffectNone))
	shifted := l.operation(ins, core.OpLShr, []core.Operand{l.value(product), l.value(l.constant(ins.address, 32, 64))}, 64, core.EffectNone)
	l.writeRegister(ins.address, highName, 32, l.operation(ins, core.OpTruncate, []core.Operand{l.value(shifted)}, 32, core.EffectNone))
}

// bitfield lifts UBFX, SBFX, BFC and BFI as exact intrinsics: the field
// position and width stay visible instead of collapsing into shifts.
func (l *lifter) bitfield(ins instruction) {
	args := args(ins)
	if len(args) < 3 || len(args) > 4 {
		l.unsupported(ins)
		return
	}
	destination, _, ok := argRegister(args[0])
	if !ok {
		l.unsupported(ins)
		return
	}
	var operands []core.Operand
	operands = append(operands, symbolOperand("arm."+lower(baseOp(ins.inst.Op).String())))
	if len(args) == 3 {
		operands = append(operands, l.value(l.readRegister(ins.address, destination, 32)))
	} else {
		source, ok := l.source(ins, args[1], 32)
		if !ok {
			l.unsupported(ins)
			return
		}
		operands = append(operands, l.value(source))
	}
	position, ok := immediateValue(args[len(args)-2])
	if !ok {
		l.unsupported(ins)
		return
	}
	width, ok := immediateValue(args[len(args)-1])
	if !ok {
		l.unsupported(ins)
		return
	}
	operands = append(operands,
		l.value(l.constant(ins.address, position, 8)),
		l.value(l.constant(ins.address, width, 8)),
	)
	result := l.operation(ins, core.OpIntrinsic, operands, 32, core.EffectNone)
	if baseOp(ins.inst.Op) == armasm.BFC || baseOp(ins.inst.Op) == armasm.BFI {
		l.writeRegister(ins.address, destination, 32, result)
		return
	}
	l.writeRegister(ins.address, destination, 32, result)
}

// coprocessor lifts VFP, NEON and coprocessor instructions. The register file is
// separate from the general registers, so the operation stays one intrinsic
// with the raw operands instead of an invented data flow.
func (l *lifter) coprocessor(ins instruction) {
	operands := []core.Operand{symbolOperand("arm." + lower(baseOp(ins.inst.Op).String()))}
	for index, arg := range ins.inst.Args {
		if arg == nil {
			break
		}
		if name, bits, ok := argRegister(arg); ok {
			operands = append(operands, registerOperand(name, bits))
			continue
		}
		if operand, ok := l.rawOperand(ins, index, arg); ok {
			operands = append(operands, operand)
		}
	}
	l.emit(core.OpIntrinsic, operands, nil, nil, core.EffectReadRegister|core.EffectWriteRegister|core.EffectUnknown, ins.address)
}

// saturate lifts SSAT and USAT, which clamp a shifted value into a bit range.
func (l *lifter) saturate(ins instruction) {
	args := args(ins)
	if len(args) != 3 && len(args) != 4 {
		l.unsupported(ins)
		return
	}
	destination, _, ok := argRegister(args[0])
	if !ok {
		l.unsupported(ins)
		return
	}
	width, ok := immediateValue(args[1])
	if !ok {
		l.unsupported(ins)
		return
	}
	value, ok := l.source(ins, args[2], 32)
	if !ok {
		l.unsupported(ins)
		return
	}
	operands := []core.Operand{
		symbolOperand("arm." + lower(baseOp(ins.inst.Op).String())),
		l.value(value),
		l.value(l.constant(ins.address, width, 8)),
	}
	result := l.operation(ins, core.OpIntrinsic, operands, 32, core.EffectNone)
	l.writeRegister(ins.address, destination, 32, result)
}

// multiplyLongSigned lifts the halfword and dual signed multiply-accumulate
// forms. The lane layout differs per mnemonic, so the operation keeps its name.
func (l *lifter) multiplyLongSigned(ins instruction) {
	arguments := args(ins)
	if len(arguments) < 3 || len(arguments) > 4 {
		l.unsupported(ins)
		return
	}
	operation := []core.Operand{symbolOperand("arm." + lower(baseOp(ins.inst.Op).String()))}
	effects := core.EffectNone
	for index, arg := range arguments {
		if index == 0 {
			name, bits, ok := argRegister(arg)
			if !ok {
				l.unsupported(ins)
				return
			}
			value := l.newValue(core.TypeInteger, bits)
			operation = append(operation, core.Operand{Kind: core.OperandRegister, Type: value.Type, Name: name})
			effects |= core.EffectWriteRegister
			l.emit(core.OpIntrinsic, operation, []core.Value{value}, nil, effects|core.EffectReadRegister|core.EffectUnknown, ins.address)
			continue
		}
		value, ok := l.source(ins, arg, 32)
		if !ok {
			l.unsupported(ins)
			return
		}
		operation = append(operation, l.value(value))
	}
}

// statusRegister lifts MRS and MSR, which move the CPSR or an SPSR.
func (l *lifter) statusRegister(ins instruction) {
	arguments := args(ins)
	if len(arguments) != 2 {
		l.unsupported(ins)
		return
	}
	destination, bits, destinationOK := argRegister(arguments[0])
	source, sourceBits, sourceOK := argRegister(arguments[1])
	if !destinationOK || !sourceOK {
		l.unsupported(ins)
		return
	}
	if baseOp(ins.inst.Op) == armasm.MRS {
		value := l.readRegister(ins.address, source, sourceBits)
		l.writeRegister(ins.address, destination, bits, value)
		return
	}
	value := l.readRegister(ins.address, source, sourceBits)
	l.writeRegister(ins.address, destination, bits, value)
}

// exclusiveStore lifts STREX, STREXB, STREXH and STREXD. The first operand
// receives the monitor outcome, the following one or two registers hold the
// value, and the last operand is the address.
func (l *lifter) exclusiveStore(ins instruction) {
	arguments := args(ins)
	if len(arguments) < 3 {
		l.unsupported(ins)
		return
	}
	statusName, statusBits, ok := argRegister(arguments[0])
	if !ok {
		l.unsupported(ins)
		return
	}
	memoryIndex := len(arguments) - 1
	pointer, ok := l.memoryOperand(ins, arguments[memoryIndex])
	if !ok {
		l.unsupported(ins)
		return
	}
	width := uint16(32)
	valueCount := memoryIndex - 1
	if valueCount > 1 {
		width = 32
	}
	offsetValue := uint64(0)
	for index := 1; index < memoryIndex; index++ {
		value, ok := l.source(ins, arguments[index], width)
		if !ok {
			l.unsupported(ins)
			return
		}
		target := pointer
		if offsetValue != 0 {
			offset := l.constant(ins.address, offsetValue, 32)
			target = l.addAddress(ins, pointer, offset)
		}
		l.storeValue(ins.address, target, value)
		offsetValue += 4
	}
	outcome := l.operation(ins, core.OpIntrinsic, []core.Operand{
		symbolOperand("arm.exclusive.monitor"),
	}, statusBits, core.EffectAtomic|core.EffectUnknown)
	l.writeRegister(ins.address, statusName, statusBits, outcome)
	l.postUpdate(ins, arguments[memoryIndex])
}

// addAddress is defined in lift.go; this indirection keeps the helper local.
