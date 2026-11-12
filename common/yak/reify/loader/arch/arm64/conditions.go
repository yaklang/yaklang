package arm64

import (
	"golang.org/x/arch/arm64/arm64asm"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// --- conditional selects ----------------------------------------------------

// conditionalSelect lifts CSET, CSETM, CINC, CINV and CNEG. Each one is an
// alias of CSINC, CSINV or CSNEG with the zero register as one input, so it
// becomes one select with an explicit constant.
func (l *lifter) conditionalSelect(ins instruction) {
	args := args(ins)
	if len(args) < 2 || len(args) > 4 {
		l.unsupported(ins)
		return
	}
	destination, bits, ok := argRegister(args[0])
	if !ok {
		l.unsupported(ins)
		return
	}
	// CSEL and CSINC take two sources and end with the condition; the alias
	// forms take one source, or none for CSET and CSETM.
	sourceIndex, source2Index, conditionIndex := 1, 2, 3
	if len(args) == 2 {
		sourceIndex, source2Index, conditionIndex = -1, -1, 1
	} else if len(args) == 3 {
		sourceIndex, source2Index, conditionIndex = 1, -1, 2
	}
	code, ok := conditionCode(args[conditionIndex])
	if !ok {
		l.unsupported(ins)
		return
	}
	condition := l.condition(ins, code)
	zero := l.constant(ins.address, 0, bits)
	whenTrue, whenFalse := zero, zero
	source := func(index int) (core.ValueID, bool) {
		if index < 0 {
			return 0, false
		}
		return l.readArg(ins, args[index], bits)
	}
	switch ins.inst.Op {
	case arm64asm.CSEL:
		first, ok := source(sourceIndex)
		if !ok {
			l.unsupported(ins)
			return
		}
		second, ok := source(source2Index)
		if !ok {
			l.unsupported(ins)
			return
		}
		whenTrue, whenFalse = first, second
	case arm64asm.CSINC, arm64asm.CSINV, arm64asm.CSNEG:
		first, ok := source(sourceIndex)
		if !ok {
			l.unsupported(ins)
			return
		}
		second, ok := source(source2Index)
		if !ok {
			l.unsupported(ins)
			return
		}
		whenTrue, whenFalse = first, l.transform(ins, second, bits)
	case arm64asm.CINC, arm64asm.CINV, arm64asm.CNEG:
		value, ok := source(sourceIndex)
		if !ok {
			l.unsupported(ins)
			return
		}
		whenTrue, whenFalse = l.transform(ins, value, bits), value
	case arm64asm.CSET:
		whenTrue = l.constant(ins.address, 1, bits)
	case arm64asm.CSETM:
		whenTrue = l.constant(ins.address, ^uint64(0), bits)
	default:
		l.unsupported(ins)
		return
	}
	result := l.operation(ins, core.OpSelect, []core.Operand{l.value(condition), l.value(whenTrue), l.value(whenFalse)}, bits, core.EffectNone)
	l.writeRegister(ins.address, destination, bits, result)
}

// transform applies the increment, invert or negate of the CSINC/CSINV/CSNEG
// family to one input.
func (l *lifter) transform(ins instruction, value core.ValueID, bits uint16) core.ValueID {
	switch ins.inst.Op {
	case arm64asm.CSINC, arm64asm.CINC:
		one := l.constant(ins.address, 1, bits)
		return l.operation(ins, core.OpAdd, []core.Operand{l.value(value), l.value(one)}, bits, core.EffectNone)
	case arm64asm.CSINV, arm64asm.CINV:
		return l.operation(ins, core.OpNot, []core.Operand{l.value(value)}, bits, core.EffectNone)
	default:
		return l.operation(ins, core.OpNeg, []core.Operand{l.value(value)}, bits, core.EffectNone)
	}
}

// --- conditional compare ----------------------------------------------------

// conditionalCompare lifts CCMP and CCMN. The flags are either the comparison
// result or the NZCV field, depending on the condition, so the whole update is
// one intrinsic.
func (l *lifter) conditionalCompare(ins instruction) {
	args := args(ins)
	if len(args) != 4 {
		l.unsupported(ins)
		return
	}
	bits := uint16(64)
	left, ok := l.readArg(ins, args[0], bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	if _, width, ok := argRegister(args[0]); ok {
		bits = width
	}
	right, ok := l.readArg(ins, args[1], bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	nzcv, ok := immediateValue(args[2])
	if !ok {
		l.unsupported(ins)
		return
	}
	code, ok := conditionCode(args[3])
	if !ok {
		l.unsupported(ins)
		return
	}
	condition := l.condition(ins, code)
	name := "arm64.flags.ccmp"
	if ins.inst.Op == arm64asm.CCMN {
		name = "arm64.flags.ccmn"
	}
	flags := l.operation(ins, core.OpIntrinsic, []core.Operand{
		symbolOperand(name),
		l.value(condition),
		l.value(left),
		l.value(right),
		l.value(l.constant(ins.address, nzcv, 4)),
	}, 4, core.EffectWriteRegister|core.EffectReadRegister)
	l.writeRegister(ins.address, "nzcv", 4, flags)
}

// --- bitfield ---------------------------------------------------------------

// bitfield lifts SBFIZ, SBFX, UBFX, UBFM, SBFM and BFXIL as exact intrinsics.
// Each one reinterprets a field of the source, so a named operation keeps the
// meaning without guessing an equivalent shift sequence.
func (l *lifter) bitfield(ins instruction) {
	args := args(ins)
	if len(args) != 4 {
		l.unsupported(ins)
		return
	}
	destination, bits, ok := argRegister(args[0])
	if !ok {
		l.unsupported(ins)
		return
	}
	source, ok := l.readArg(ins, args[1], bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	lsb, ok := immediateValue(args[2])
	if !ok {
		l.unsupported(ins)
		return
	}
	width, ok := immediateValue(args[3])
	if !ok {
		l.unsupported(ins)
		return
	}
	result := l.operation(ins, core.OpIntrinsic, []core.Operand{
		symbolOperand("arm64." + lower(ins.inst.Op)),
		l.value(source),
		l.value(l.constant(ins.address, lsb, 8)),
		l.value(l.constant(ins.address, width, 8)),
	}, bits, core.EffectNone)
	l.writeRegister(ins.address, destination, bits, result)
}

// --- long multiply ----------------------------------------------------------

// longMultiply lifts UMULL, SMULL, UMADDL and SMADDL. The sources are 32-bit
// registers widened to the 64-bit destination, so the widening is explicit.
func (l *lifter) longMultiply(ins instruction) {
	args := args(ins)
	if len(args) != 3 && len(args) != 4 {
		l.unsupported(ins)
		return
	}
	destination, bits, ok := argRegister(args[0])
	if !ok {
		l.unsupported(ins)
		return
	}
	signed := ins.inst.Op == arm64asm.SMULL || ins.inst.Op == arm64asm.SMADDL ||
		ins.inst.Op == arm64asm.SMSUBL
	left, ok := l.widenProduct(ins, args[1], bits, signed)
	if !ok {
		l.unsupported(ins)
		return
	}
	right, ok := l.widenProduct(ins, args[2], bits, signed)
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
		if ins.inst.Op == arm64asm.SMSUBL || ins.inst.Op == arm64asm.UMSUBL {
			kind = core.OpSub
		}
		result = l.operation(ins, kind, []core.Operand{l.value(result), l.value(addend)}, bits, core.EffectNone)
	}
	l.writeRegister(ins.address, destination, bits, result)
}

func (l *lifter) widenProduct(ins instruction, arg arm64asm.Arg, bits uint16, signed bool) (core.ValueID, bool) {
	name, width, ok := argRegister(arg)
	if !ok {
		return 0, false
	}
	value := l.readRegister(ins.address, name, width)
	if width == bits {
		return value, true
	}
	kind := core.OpZeroExtend
	if signed {
		kind = core.OpSignExtend
	}
	return l.operation(ins, kind, []core.Operand{l.value(value)}, bits, core.EffectNone), true
}

// --- vector ----------------------------------------------------------------

// isVectorRegister reports whether the register holds SIMD or floating-point
// lanes. Lane values are not modeled as integers, so such instructions become
// opaque register operations.
func isVectorRegister(name string) bool {
	if len(name) < 2 {
		return false
	}
	switch name[0] {
	case 'b', 'h', 's', 'd', 'q', 'v':
	default:
		return false
	}
	for _, current := range name[1:] {
		if current < '0' || current > '9' {
			return false
		}
	}
	return true
}

// vectorOperation lifts one SIMD or floating-point instruction. Results are one
// vector value per destination register, so data flow through the vector unit
// stays visible without pretending lanes are integers.
func (l *lifter) vectorOperation(ins instruction) {
	var operands []core.Operand
	var results []core.Value
	effects := core.EffectNone
	for index, arg := range ins.inst.Args {
		if arg == nil {
			break
		}
		name, bits, ok := argRegister(arg)
		if !ok {
			if operand, ok := l.rawOperand(ins, index, arg); ok {
				operands = append(operands, operand)
			}
			continue
		}
		if isVectorRegister(name) || name == "sp" || name == "wsp" {
			value := l.newValue(core.TypeInteger, bits)
			operands = append(operands, core.Operand{
				Kind: core.OperandRegister,
				Type: value.Type,
				Name: name,
			})
			if index == 0 {
				results = append(results, value)
				effects |= core.EffectWriteRegister
			} else {
				effects |= core.EffectReadRegister
			}
			continue
		}
		operands = append(operands, l.value(l.readRegister(ins.address, name, bits)))
	}
	operands = append([]core.Operand{symbolOperand("arm64." + lower(ins.inst.Op))}, operands...)
	l.emit(core.OpIntrinsic, operands, results, nil, effects|core.EffectUnknown, ins.address)
}

// --- bit reversal and rotation ---------------------------------------------

// reverse lifts REV, REV16, REV32 and RBIT as one intrinsic per operation. The
// byte layout matters for data, so no shift sequence replaces them.
func (l *lifter) reverse(ins instruction) {
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
	source, ok := l.readArg(ins, args[1], bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	result := l.operation(ins, core.OpIntrinsic, []core.Operand{
		symbolOperand("arm64." + lower(ins.inst.Op)),
		l.value(source),
	}, bits, core.EffectNone)
	l.writeRegister(ins.address, destination, bits, result)
}

// rotate lifts ROR, RORV and EXTR. EXTR concatenates two registers and extracts
// a window, so it keeps both inputs.
func (l *lifter) rotate(ins instruction) {
	args := args(ins)
	if len(args) < 3 || len(args) > 4 {
		l.unsupported(ins)
		return
	}
	destination, bits, ok := argRegister(args[0])
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
	operands := []core.Operand{symbolOperand("arm64." + lower(ins.inst.Op)), l.value(left), l.value(right)}
	if len(args) == 4 {
		amount, ok := immediateValue(args[3])
		if !ok {
			l.unsupported(ins)
			return
		}
		operands = append(operands, l.value(l.constant(ins.address, amount, 8)))
	}
	result := l.operation(ins, core.OpIntrinsic, operands, bits, core.EffectNone)
	l.writeRegister(ins.address, destination, bits, result)
}
