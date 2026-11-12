package s390x

import (
	"golang.org/x/arch/s390x/s390xasm"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// memory lifts one instruction with a memory operand: a load when the first
// operand is a register and the mnemonic starts with l, a store when it starts
// with st, and an opaque operation otherwise.
func (l *lifter) memory(ins instruction, base, index string, displacement int64) {
	arguments := args(ins.inst)
	name := nameOf(ins.inst.Op)
	pointer := l.addressFromBase(ins, base, index, displacement)
	if isStoreImmediate(name) {
		// The move and store-immediate forms carry no destination register.
		value, ok := l.storeSource(ins)
		if !ok {
			l.unsupported(ins)
			return
		}
		l.storeValue(ins.address, pointer, value)
		return
	}
	register, bits, ok := firstRegister(arguments)
	if !ok {
		l.unsupported(ins)
		return
	}
	switch {
	case isStore(name):
		value, ok := l.storeSource(ins)
		if !ok {
			l.unsupported(ins)
			return
		}
		l.storeValue(ins.address, pointer, value)
	case isLoad(name) && register != "":
		loaded := l.loadValue(ins.address, pointer, bits)
		value := loaded
		// The sign and zero extending loads widen a narrow value.
		if width := loadWidth(name); width != 0 && width < bits {
			kind := core.OpZeroExtend
			if signedLoad(name) {
				kind = core.OpSignExtend
			}
			value = l.operation(ins, kind, []core.Operand{l.value(loaded)}, bits, core.EffectNone)
		}
		l.writeRegister(ins.address, register, bits, value)
	default:
		// A memory operand on an ALU or compare form: keep the operation and
		// the effective address.
		operands := []core.Operand{symbolOperand("s390x." + name), l.value(pointer)}
		for position, arg := range arguments {
			if position == 0 {
				continue
			}
			if operand, ok := l.rawOperand(ins, arg); ok {
				operands = append(operands, operand)
			}
		}
		result := l.operation(ins, core.OpIntrinsic, operands, bits, core.EffectReadMemory|core.EffectUnknown)
		l.writeRegister(ins.address, register, bits, result)
	}
}

// isStoreImmediate reports whether the mnemonic writes a constant into memory:
// MVI, MVHI, MVGHI and the moves.
func isStoreImmediate(name string) bool {
	switch name {
	case "mvi", "mviy", "mvhi", "mvhhi", "mvghi", "mvc", "xc", "nc", "oc",
		"clc", "tr", "trt", "mvcle", "mvpg":
		return true
	}
	return false
}

// storeSource materializes the value one store writes: the register operand when
// there is one, otherwise the immediate.
func (l *lifter) storeSource(ins instruction) (core.ValueID, bool) {
	for _, arg := range args(ins.inst) {
		if register, ok := arg.(s390xasm.Reg); ok {
			name, bits, ok := registerName(register)
			if !ok {
				continue
			}
			return l.readRegister(ins.address, name, bits), true
		}
	}
	if value, ok := l.immediateValue(ins, append([]s390xasm.Arg{nil}, args(ins.inst)...), addressBits); ok {
		return value, true
	}
	// A move writes a byte, halfword or word count, which stays opaque.
	return l.operation(ins, core.OpIntrinsic, []core.Operand{symbolOperand("s390x." + nameOf(ins.inst.Op))},
		8, core.EffectUnknown), true
}

// loadWidth returns the width of the value a load transfers, or zero when the
// load moves a full register.
func loadWidth(name string) uint16 {
	switch name {
	case "lb", "llc", "ic", "icm", "stc", "lgb", "llgc":
		return 8
	case "lh", "lhy", "llh", "sth", "lgh", "llgh", "llch":
		return 16
	case "ly", "l", "st", "sty", "lg", "stg":
		return 0
	}
	// A trailing g means a 64-bit access, otherwise the default is a full
	// register move.
	return 0
}

// signedLoad reports whether a load sign-extends.
func signedLoad(name string) bool {
	switch name {
	case "lb", "lh", "lgb", "lgh", "lgf", "lgfr", "ltgf", "ltgfr":
		return true
	default:
		return false
	}
}

// registers lifts one register-only instruction: a binary or unary ALU
// operation, a compare, or an opaque operation.
func (l *lifter) registers(ins instruction) (core.ValueID, bool) {
	arguments := args(ins.inst)
	name := nameOf(ins.inst.Op)
	bits := addressBits
	if len(arguments) == 0 {
		return 0, false
	}
	kind, isBinary := binaryOpcode(name)
	if isBinary && len(arguments) >= 2 {
		destination, destinationBits, ok := firstRegister(arguments)
		if !ok {
			return 0, false
		}
		left, ok := l.readRegisterValue(ins, destination, destinationBits)
		if !ok {
			return 0, false
		}
		right, ok := l.sourceValue(ins, lastSource(arguments), destinationBits)
		if !ok {
			return 0, false
		}
		result := l.operation(ins, kind, []core.Operand{l.value(left), l.value(right)}, destinationBits, core.EffectNone)
		l.writeRegister(ins.address, destination, destinationBits, result)
		return result, true
	}
	// The single-register forms operate on the first operand: shifts, clears
	// and the load-address family.
	if isUnaryForm(name) {
		destination, destinationBits, ok := firstRegister(arguments)
		if !ok {
			return 0, false
		}
		value, ok := l.readRegisterValue(ins, destination, destinationBits)
		if !ok {
			return 0, false
		}
		result := l.operation(ins, core.OpIntrinsic, []core.Operand{
			symbolOperand("s390x." + name), l.value(value),
		}, destinationBits, core.EffectNone)
		l.writeRegister(ins.address, destination, destinationBits, result)
		return result, true
	}
	_ = bits
	return 0, false
}

// isUnaryForm reports whether the mnemonic transforms its first operand in
// place. The compilers emit these for sign and zero extension and for the
// shift-and-mask forms.
func isUnaryForm(name string) bool {
	switch name {
	case "lgr", "lgfr", "llgfr", "lgbr", "llgcr", "llghr", "llgtr",
		"lnr", "lpr", "lcr", "lcgr", "lngr", "lpgr",
		"sllg", "srlg", "srag", "sllk", "srlk", "srak", "rll", "rllg",
		"risbg", "rxsbg", "risbgn":
		return true
	}
	return false
}

// lastSource returns the source operand of one ALU form. The three-register
// forms write the result into the third operand, so the source is the last
// register rather than the first.
func lastSource(arguments []s390xasm.Arg) s390xasm.Arg {
	for index := len(arguments) - 1; index >= 1; index-- {
		if _, ok := arguments[index].(s390xasm.Reg); ok {
			return arguments[index]
		}
	}
	if len(arguments) > 1 {
		return arguments[1]
	}
	return nil
}

// binaryOpcode maps an s390x ALU mnemonic to a LowIR opcode. The z/Architecture
// names the register forms with an r suffix, so the table lists the base names
// and the caller strips the suffix.
func binaryOpcode(name string) (core.Opcode, bool) {
	switch name {
	case "agrk", "ark", "ar", "agr", "ag", "a", "ay", "alr", "algr", "alc",
		"alcr", "alcg", "alcgr":
		return core.OpAdd, true
	case "sgrk", "srk", "sr", "sgr", "sg", "s", "sy", "slr", "slgr":
		return core.OpSub, true
	case "msr", "msgr", "ms", "msy", "mr", "mgr", "m":
		return core.OpMul, true
	case "dsr", "dsgr", "dr", "dgr", "dlr", "dlgr", "d":
		return core.OpSDiv, true
	case "ngrk", "nrk", "nr", "ngr", "ng", "n", "ny":
		return core.OpAnd, true
	case "ogrk", "ork", "or", "ogr", "og", "o", "oy":
		return core.OpOr, true
	case "xgrk", "xrk", "xr", "xgr", "xg", "x", "xy":
		return core.OpXor, true
	}
	return "", false
}

// firstRegister returns the name and width of the first register operand.
func firstRegister(arguments []s390xasm.Arg) (string, uint16, bool) {
	for _, arg := range arguments {
		if register, ok := arg.(s390xasm.Reg); ok {
			return registerName(register)
		}
	}
	return "", 0, false
}

// readRegisterValue reads one register into a value.
func (l *lifter) readRegisterValue(ins instruction, name string, bits uint16) (core.ValueID, bool) {
	if name == "" {
		return 0, false
	}
	return l.readRegister(ins.address, name, bits), true
}

// sourceValue materializes one operand as a value: a register, an immediate or
// an absolute memory reference.
func (l *lifter) sourceValue(ins instruction, arg s390xasm.Arg, bits uint16) (core.ValueID, bool) {
	switch value := arg.(type) {
	case s390xasm.Reg:
		name, width, ok := registerName(value)
		if !ok {
			return 0, false
		}
		if width == 0 {
			width = bits
		}
		return l.readRegister(ins.address, name, width), true
	case s390xasm.Sign8:
		return l.constant(ins.address, uint64(int64(int8(value))), bits), true
	case s390xasm.Sign16:
		return l.constant(ins.address, uint64(int64(int16(value))), bits), true
	case s390xasm.Sign32:
		return l.constant(ins.address, uint64(int64(int32(value))), bits), true
	case s390xasm.Imm:
		return l.constant(ins.address, uint64(uint32(value)), bits), true
	case s390xasm.RegIm16:
		return l.pointerConstant(ins.address, ins.address+2*uint64(int16(value))), true
	case s390xasm.RegIm32:
		return l.pointerConstant(ins.address, ins.address+2*uint64(int32(value))), true
	default:
		return 0, false
	}
}
