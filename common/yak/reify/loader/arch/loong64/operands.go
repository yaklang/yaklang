package loong64

import (
	"golang.org/x/arch/loong64/loong64asm"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// load lifts one load: the destination is the first register and the base comes
// before the displacement, so the last three operands describe memory.
func (l *lifter) load(ins instruction) {
	operands := args(ins.inst)
	if len(operands) < 2 {
		l.unsupported(ins)
		return
	}
	destination, bits, ok := argRegister(operands[0])
	if !ok {
		l.unsupported(ins)
		return
	}
	pointer, ok := l.memoryPointer(ins, operands)
	if !ok {
		l.unsupported(ins)
		return
	}
	value := l.loadValue(ins.address, pointer, widthOf(nameOf(ins.inst.Op), bits))
	l.writeRegister(ins.address, destination, bits, value)
}

// store lifts one store.
func (l *lifter) store(ins instruction) {
	operands := args(ins.inst)
	if len(operands) < 2 {
		l.unsupported(ins)
		return
	}
	source, bits, ok := argRegister(operands[0])
	if !ok {
		l.unsupported(ins)
		return
	}
	pointer, ok := l.memoryPointer(ins, operands)
	if !ok {
		l.unsupported(ins)
		return
	}
	value := l.readRegister(ins.address, source, bits)
	l.storeValue(ins.address, pointer, value)
}

// memoryPointer builds the effective address of one load or store. LoongArch
// writes the base register and then the displacement, so both come from the
// operand list.
func (l *lifter) memoryPointer(ins instruction, operands []loong64asm.Arg) (core.ValueID, bool) {
	base, displacement, ok := memoryOperand(operands)
	if !ok {
		return 0, false
	}
	return l.addressFromBase(ins, base, displacement), true
}

// memoryOperand returns the base register and the displacement of one load or
// store. The base is the register immediately before the signed immediate.
func memoryOperand(operands []loong64asm.Arg) (string, int64, bool) {
	for index := len(operands) - 1; index >= 1; index-- {
		displacement, ok := operands[index].(loong64asm.Simm16)
		if !ok {
			continue
		}
		base, ok := operands[index-1].(loong64asm.Reg)
		if !ok {
			continue
		}
		name, _, ok := registerName(base)
		if !ok {
			continue
		}
		return name, int64(displacement.Imm), true
	}
	// A register-offset form keeps the index in the last register.
	if len(operands) >= 3 {
		base, ok := operands[1].(loong64asm.Reg)
		index, indexOK := operands[2].(loong64asm.Reg)
		if ok && indexOK {
			baseName, _, baseOK := registerName(base)
			if _, _, nameOK := registerName(index); baseOK && nameOK {
				return baseName, 0, true
			}
		}
	}
	return "", 0, false
}

// widthOf returns the access width of one load or store.
func widthOf(name string, fallback uint16) uint16 {
	switch name {
	case "ld.b", "ld.bu", "ldx.b", "ldx.bu", "st.b", "stx.b":
		return 8
	case "ld.h", "ld.hu", "ldx.h", "ldx.hu", "st.h", "stx.h":
		return 16
	case "ld.w", "ld.wu", "ldx.w", "ldx.wu", "st.w", "stx.w":
		return 32
	case "ld.d", "ldx.d", "st.d", "stx.d":
		return 64
	}
	return fallback
}

// arithmetic lifts the integer ALU instructions. LoongArch writes the result
// into the first operand, and the shift forms keep the count in the third.
func (l *lifter) arithmetic(ins instruction) (core.ValueID, bool) {
	name := nameOf(ins.inst.Op)
	operands := args(ins.inst)
	bits := addressBits
	switch name {
	case "lu12i.w":
		// The signed immediate occupies the high twenty bits of a word.
		destination, _, ok := argRegister(operands[0])
		if !ok {
			return 0, false
		}
		immediate, ok := signedImmediate(operands[1])
		if !ok {
			return 0, false
		}
		value := l.constant(ins.address, uint64(immediate)<<12, bits)
		l.writeRegister(ins.address, destination, bits, value)
		return value, true
	case "lu32i.d":
		destination, _, ok := argRegister(operands[0])
		if !ok {
			return 0, false
		}
		immediate, ok := signedImmediate(operands[1])
		if !ok {
			return 0, false
		}
		// LU32I.D replaces the middle 32 bits of a 64-bit value.
		shifted := (uint64(immediate) & 0xffffffff) << 32
		source, ok := l.source(ins, operands[0], bits)
		if !ok {
			return 0, false
		}
		mask := l.constant(ins.address, 0xffffffff, bits)
		low := l.operation(ins, core.OpAnd, []core.Operand{l.value(source), l.value(mask)}, bits, core.EffectNone)
		high := l.constant(ins.address, shifted, bits)
		value := l.operation(ins, core.OpOr, []core.Operand{l.value(low), l.value(high)}, bits, core.EffectNone)
		l.writeRegister(ins.address, destination, bits, value)
		return value, true
	case "lu52i.d":
		destination, _, ok := argRegister(operands[0])
		if !ok {
			return 0, false
		}
		immediate, ok := signedImmediate(operands[2])
		if !ok {
			return 0, false
		}
		source, ok := l.source(ins, operands[1], bits)
		if !ok {
			return 0, false
		}
		mask := l.constant(ins.address, 0x000fffffffffffff, bits)
		low := l.operation(ins, core.OpAnd, []core.Operand{l.value(source), l.value(mask)}, bits, core.EffectNone)
		high := l.constant(ins.address, (uint64(immediate)&0x0fff)<<52, bits)
		value := l.operation(ins, core.OpOr, []core.Operand{l.value(low), l.value(high)}, bits, core.EffectNone)
		l.writeRegister(ins.address, destination, bits, value)
		return value, true
	case "lu12i.w.old":
		// The immediate is shifted into place.
		if len(operands) != 2 {
			return 0, false
		}
		destination, _, ok := argRegister(operands[0])
		if !ok {
			return 0, false
		}
		immediate, ok := signedImmediate(operands[1])
		if !ok {
			return 0, false
		}
		_ = destination
		_ = immediate
		return 0, false
	case "pcaddi", "pcaddu12i", "pcalau12i", "pcaddu18i":
		// The PC-relative forms add a scaled immediate to the instruction
		// address. They are how a position-independent function reaches its
		// literal pool and its globals.
		if len(operands) != 2 {
			return 0, false
		}
		destination, _, ok := argRegister(operands[0])
		if !ok {
			return 0, false
		}
		immediate, ok := signedImmediate(operands[1])
		if !ok {
			return 0, false
		}
		shift := uint(2)
		base := ins.address
		switch name {
		case "pcalau12i":
			// The page-address form starts from the page of the instruction, not
			// from the instruction address: the low twelve bits of the result are
			// zero, and the following addi supplies the offset inside the page.
			shift = 12
			base = ins.address &^ 0xfff
		case "pcaddu12i":
			shift = 12
		case "pcaddu18i":
			shift = 18
		}
		value := l.pointerConstant(ins.address, base+uint64(immediate<<shift))
		l.writeRegister(ins.address, destination, bits, value)
		return value, true
	case "ori", "andi", "xori":
		// The logical immediates are zero-extended.
		if len(operands) != 3 {
			return 0, false
		}
		destination, _, ok := argRegister(operands[0])
		if !ok {
			return 0, false
		}
		left, ok := l.source(ins, operands[1], bits)
		if !ok {
			return 0, false
		}
		immediate, ok := signedImmediate(operands[2])
		if !ok {
			return 0, false
		}
		kind := core.OpAnd
		switch name {
		case "ori":
			kind = core.OpOr
		case "xori":
			kind = core.OpXor
		}
		right := l.constant(ins.address, uint64(uint16(immediate)), bits)
		result := l.operation(ins, kind, []core.Operand{l.value(left), l.value(right)}, bits, core.EffectNone)
		l.writeRegister(ins.address, destination, bits, result)
		return result, true
	case "addi.w", "addi.d", "addi.du":
		if len(operands) != 3 {
			return 0, false
		}
		destination, _, ok := argRegister(operands[0])
		if !ok {
			return 0, false
		}
		left, ok := l.source(ins, operands[1], bits)
		if !ok {
			return 0, false
		}
		immediate, ok := signedImmediate(operands[2])
		if !ok {
			return 0, false
		}
		right := l.constant(ins.address, uint64(immediate), bits)
		result := l.operation(ins, core.OpAdd, []core.Operand{l.value(left), l.value(right)}, bits, core.EffectNone)
		l.writeRegister(ins.address, destination, bits, result)
		return result, true
	case "slti", "sltui":
		if len(operands) != 3 {
			return 0, false
		}
		destination, _, ok := argRegister(operands[0])
		if !ok {
			return 0, false
		}
		left, ok := l.source(ins, operands[1], bits)
		if !ok {
			return 0, false
		}
		immediate, ok := signedImmediate(operands[2])
		if !ok {
			return 0, false
		}
		right := l.constant(ins.address, uint64(immediate), bits)
		kind := core.OpSLess
		if name == "sltui" {
			kind = core.OpULess
		}
		result := l.operation(ins, kind, []core.Operand{l.value(left), l.value(right)}, bits, core.EffectNone)
		l.writeRegister(ins.address, destination, bits, result)
		return result, true
	}

	kind, ok := binaryOpcode(name)
	if !ok {
		if isUnary(name) {
			return l.unary(ins, operands, name)
		}
		return 0, false
	}
	if len(operands) < 2 {
		return 0, false
	}
	destination, destinationBits, ok := argRegister(operands[0])
	if !ok {
		return 0, false
	}
	left, ok := l.source(ins, operands[1], destinationBits)
	if !ok {
		return 0, false
	}
	var right core.ValueID
	if len(operands) >= 3 {
		right, ok = l.source(ins, operands[2], destinationBits)
		if !ok {
			return 0, false
		}
	} else {
		right = left
	}
	result := l.operation(ins, kind, []core.Operand{l.value(left), l.value(right)}, destinationBits, core.EffectNone)
	l.writeRegister(ins.address, destination, destinationBits, result)
	return result, true
}

// unary lifts the single-source forms.
func (l *lifter) unary(ins instruction, operands []loong64asm.Arg, name string) (core.ValueID, bool) {
	if len(operands) < 2 {
		return 0, false
	}
	destination, bits, ok := argRegister(operands[0])
	if !ok {
		return 0, false
	}
	source, ok := l.source(ins, operands[1], bits)
	if !ok {
		return 0, false
	}
	result := l.operation(ins, core.OpIntrinsic, []core.Operand{
		symbolOperand("loong64." + name), l.value(source),
	}, bits, core.EffectNone)
	l.writeRegister(ins.address, destination, bits, result)
	return result, true
}

// isUnary reports whether the mnemonic transforms one register.
func isUnary(name string) bool {
	switch name {
	case "clz.w", "clz.d", "ctz.w", "ctz.d", "cto.w", "cto.d",
		"revb.2h", "revb.4h", "revb.2w", "revb.d", "ext.w.b", "ext.w.h",
		"bstrpick.w", "bstrpick.d", "bstrins.w", "bstrins.d",
		"movfcsr2gr", "movfr2gr.s", "movfr2gr.d", "movgr2fr.w", "movgr2fr.d",
		"movgr2frh.w", "movgr2cf", "movcf2gr", "movgr2fcsr", "move":
		return true
	}
	return false
}

// source materializes one operand as a value.
func (l *lifter) source(ins instruction, arg loong64asm.Arg, bits uint16) (core.ValueID, bool) {
	if name, width, ok := argRegister(arg); ok {
		if width == 0 {
			width = bits
		}
		return l.readRegister(ins.address, name, width), true
	}
	if value, ok := signedImmediate(arg); ok {
		return l.constant(ins.address, uint64(value), bits), true
	}
	return 0, false
}

// binaryOpcode maps one mnemonic to a LowIR opcode.
func binaryOpcode(name string) (core.Opcode, bool) {
	switch name {
	case "add.w", "add.d", "addi.w", "addi.d", "addu16i.d":
		return core.OpAdd, true
	case "sub.w", "sub.d":
		return core.OpSub, true
	case "mul.w", "mul.d", "mulh.w", "mulh.d", "mulh.wu", "mulh.du":
		return core.OpMul, true
	case "div.w", "div.d":
		return core.OpSDiv, true
	case "div.wu", "div.du", "mod.w", "mod.d":
		return core.OpUDiv, true
	case "mod.wu", "mod.du":
		return core.OpUDiv, true
	case "and", "andi":
		return core.OpAnd, true
	case "or", "ori":
		return core.OpOr, true
	case "nor":
		return core.OpOr, true
	case "xor", "xori":
		return core.OpXor, true
	case "sll.w", "sll.d", "slli.w", "slli.d":
		return core.OpShl, true
	case "srl.w", "srl.d", "srli.w", "srli.d":
		return core.OpLShr, true
	case "sra.w", "sra.d", "srai.w", "srai.d":
		return core.OpAShr, true
	case "rotri.w", "rotri.d", "rotri.b", "rotri.h":
		return core.OpIntrinsic, true
	case "slt", "sltu", "slti", "sltui":
		return core.OpSLess, true
	case "alsl.w", "alsl.d", "alsl.wu", "bytepick.w", "bytepick.d",
		"andn", "orn", "mulw.d.w", "mulw.d.wu":
		return core.OpIntrinsic, true
	}
	return "", false
}
